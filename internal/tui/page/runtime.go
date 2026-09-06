package page

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/install"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	updatepkg "go.mewis.me/chatgpt-mcp/internal/update"
)

const systemOperationTimeout = 2 * time.Minute

type SystemCommand string

const (
	SystemRefresh        SystemCommand = "system.refresh"
	RuntimeUpUser        SystemCommand = "runtime.up.user"
	RuntimeUpSystem      SystemCommand = "runtime.up.system"
	RuntimeDownUser      SystemCommand = "runtime.down.user"
	RuntimeDownSystem    SystemCommand = "runtime.down.system"
	RuntimeRestartUser   SystemCommand = "runtime.restart.user"
	RuntimeRestartSystem SystemCommand = "runtime.restart.system"
	RuntimeReload        SystemCommand = "runtime.reload"
	RuntimeForeground    SystemCommand = "runtime.foreground"
	ConfigInitialize     SystemCommand = "config.initialize.external"
	ConfigUninitialize   SystemCommand = "config.uninitialize.external"
	AuthMCPEnable        SystemCommand = "auth.mcp.enable"
	AuthMCPDisable       SystemCommand = "auth.mcp.disable"
	AuthMCPRotate        SystemCommand = "auth.mcp.rotate"
	AuthAdminEnable      SystemCommand = "auth.admin.enable"
	AuthAdminDisable     SystemCommand = "auth.admin.disable"
	AuthAdminRotate      SystemCommand = "auth.admin.rotate"
	AliasInstall         SystemCommand = "alias.install"
	AliasRemove          SystemCommand = "alias.remove"
	InstallRun           SystemCommand = "install.run"
	InstallCleanup       SystemCommand = "install.cleanup"
	UpdateCheck          SystemCommand = "update.check"
	UpdateApply          SystemCommand = "update.apply"
)

type SystemCommandMsg struct{ Command SystemCommand }

type systemOverlay uint8

const (
	systemOverlayNone systemOverlay = iota
	systemOverlayForm
	systemOverlayConfirm
	systemOverlayOperation
	systemOverlaySecret
	systemOverlayExternal
)

type systemLoadMsg struct {
	runtime application.RuntimeOverview
	auth    application.AuthStatus
	install application.InstallationOverview
	about   application.AboutInfo
	err     error
}

type systemOperationMsg struct {
	id       uint64
	command  SystemCommand
	token    string
	external *application.ExternalCommand
	update   updatepkg.CheckResult
	notice   string
	err      error
}

type systemCopyMsg struct{ err error }

type RuntimePage struct {
	ctx             context.Context
	browser         component.Browser
	runtime         application.RuntimeOverview
	auth            application.AuthStatus
	install         application.InstallationOverview
	about           application.AboutInfo
	loaded          bool
	loading         bool
	overlay         systemOverlay
	form            component.Form
	installForm     *installFormData
	updateForm      *updateFormData
	confirm         component.ConfirmButtons
	pending         SystemCommand
	progress        *component.Progress
	operationCancel context.CancelFunc
	operationID     uint64
	secret          string
	secretKind      string
	external        *application.ExternalCommand
	notice          string
	err             error
	width           int
	height          int
}

func NewRuntime(ctx context.Context) (*RuntimePage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	page := &RuntimePage{ctx: ctx}
	page.browser = component.NewBrowser(ctx, "Runtime", nil, nil).WithTitleVisible(false)
	return page, nil
}

func (page *RuntimePage) Init() tea.Cmd {
	if page == nil {
		return nil
	}
	page.loading = true
	return page.loadCmd()
}

func (page *RuntimePage) Close() { page.cancelOperation() }

func (page *RuntimePage) OverlayActive() bool {
	return page != nil && (page.overlay != systemOverlayNone || page.browser.DetailOpen())
}

func (page *RuntimePage) InputActive() bool {
	return page != nil && (page.overlay == systemOverlayForm || page.browser.InputActive())
}

func (page *RuntimePage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	if msg, ok := message.(systemOperationMsg); ok {
		return page, page.finishOperation(msg)
	}
	if msg, ok := message.(systemCopyMsg); ok {
		if msg.err != nil {
			page.notice = "Clipboard unavailable: " + msg.err.Error()
		} else {
			page.notice = "Copied to clipboard"
		}
		return page, nil
	}
	if page.overlay == systemOverlayOperation {
		if key, ok := message.(tea.KeyPressMsg); ok && key.String() == "esc" {
			page.cancelOperation()
			page.closeOverlay()
			return page, nil
		}
		if page.progress != nil {
			updated, cmd := page.progress.Update(message)
			page.progress = &updated
			return page, cmd
		}
		return page, nil
	}

	switch msg := message.(type) {
	case systemLoadMsg:
		page.loading = false
		if msg.err != nil {
			page.err = msg.err
			return page, nil
		}
		page.loaded, page.err = true, nil
		page.runtime, page.auth, page.install, page.about = msg.runtime, msg.auth, msg.install, msg.about
		cmd := page.rebuildBrowser(page.selectedID())
		page.syncBrowserHelp()
		return page, cmd
	case tea.WindowSizeMsg:
		page.width, page.height = msg.Width, msg.Height
		browserCmd := page.resizeBrowser()
		if page.overlay == systemOverlayForm {
			form, formCmd := page.form.Update(msg)
			page.form = form
			return page, tea.Batch(browserCmd, formCmd)
		}
		return page, browserCmd
	case component.FormSubmittedMsg:
		return page, page.submitForm()
	case component.FormCancelledMsg:
		page.closeOverlay()
		return page, nil
	case component.FormMouseMsg:
		if page.overlay == systemOverlayForm {
			form, cmd := page.form.Update(msg)
			page.form = form
			return page, cmd
		}
		return page, nil
	case component.ConfirmChoiceMsg:
		if page.overlay == systemOverlayConfirm {
			page.confirm.Select(msg.Affirmative)
			return page, page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		return page, nil
	case SystemCommandMsg:
		cmd, err := page.openCommand(msg.Command)
		if err != nil {
			page.err = err
		}
		return page, cmd
	case tea.KeyPressMsg:
		if page.overlay == systemOverlayForm {
			form, cmd := page.form.Update(msg)
			page.form = form
			return page, cmd
		}
		if page.overlay == systemOverlayConfirm {
			return page, page.updateConfirm(msg)
		}
		if page.overlay == systemOverlaySecret || page.overlay == systemOverlayExternal {
			switch msg.String() {
			case "esc":
				page.closeOverlay()
				return page, nil
			case "c":
				return page, page.copyOverlayValue()
			}
			return page, nil
		}
		if page.browser.InputActive() {
			updated, cmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			page.syncBrowserHelp()
			return page, cmd
		}
		if cmd, handled := page.handleKey(msg); handled {
			return page, cmd
		}
	}
	if page.overlay == systemOverlayForm {
		form, cmd := page.form.Update(message)
		page.form = form
		return page, cmd
	}
	updated, cmd := page.browser.Update(message)
	page.browser = updated.(component.Browser)
	page.syncBrowserHelp()
	return page, cmd
}

func (page *RuntimePage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "Runtime page unavailable", "")
	}
	page.width, page.height = width, height
	if !page.loaded && page.loading {
		return component.StateView(component.PageLoading, "Loading runtime and system state", "")
	}
	title := component.PageTitle("Runtime & System", width)
	status := page.statusView(width)
	feedback := ""
	if page.err != nil {
		feedback = component.Banner(page.err.Error(), component.ToneDanger)
	} else if page.notice != "" {
		feedback = component.Muted(page.notice)
	}
	browserHeight := max(1, height-lipgloss.Height(title)-lipgloss.Height(status)-pageFeedbackHeight(feedback))
	updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: width, Height: browserHeight})
	page.browser = updated.(component.Browser)
	content := title + "\n" + status + "\n" + prependPageFeedback(feedback, page.browser.Content())
	switch page.overlay {
	case systemOverlayForm:
		content = component.CenterOverlay(content, component.Modal(page.form.View(), overlayWidth(width, 82)), width, height)
	case systemOverlayConfirm:
		body := component.Title(page.confirmTitle()) + "\n\n" + component.Muted(page.confirmDescription()) + "\n\n" + page.confirm.View() + "\n" + component.Muted("Enter confirm · Esc cancel")
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 72)), width, height)
	case systemOverlayOperation:
		body := ""
		if page.progress != nil {
			body = page.progress.View()
		}
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 64)), width, height)
	case systemOverlaySecret:
		body := component.Title(strings.ToUpper(page.secretKind)+" token") + "\n\n" + page.secret + "\n\n" + component.Muted("Shown once · c copy · Esc close")
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 88)), width, height)
	case systemOverlayExternal:
		body := component.Title("Run outside the TUI") + "\n\n" + component.Muted(page.external.Reason) + "\n\n" + page.external.Command + "\n\n" + component.Muted("c copy command · Esc close")
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 88)), width, height)
	}
	return content
}

func (page *RuntimePage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	switch page.overlay {
	case systemOverlayForm:
		return formOverlayMouseTargets(page.form, overlayWidth(page.width, 82), page.width, page.height, originX, originY, z+20)
	case systemOverlayConfirm:
		return confirmOverlayMouseTargets(page.confirm, page.confirmTitle(), page.confirmDescription(), overlayWidth(page.width, 72), page.width, page.height, originX, originY, z+20)
	case systemOverlayOperation, systemOverlaySecret, systemOverlayExternal:
		return []component.MouseTarget{mouseBlocker(originX, originY, page.width, page.height, z+20)}
	}
	feedback := ""
	if page.err != nil {
		feedback = component.Banner(page.err.Error(), component.ToneDanger)
	} else if page.notice != "" {
		feedback = component.Muted(page.notice)
	}
	y := originY + lipgloss.Height(component.PageTitle("Runtime & System", page.width)) + lipgloss.Height(page.statusView(page.width)) + pageFeedbackHeight(feedback)
	return page.browser.MouseTargets(originX, y, z)
}

func (page *RuntimePage) loadCmd() tea.Cmd {
	ctx := page.ctx
	return func() tea.Msg {
		runtimeState, err := application.LoadRuntimeOverview(ctx)
		if err != nil {
			return systemLoadMsg{err: err}
		}
		auth, err := application.GetAuthStatus()
		if err != nil {
			return systemLoadMsg{err: err}
		}
		installation, err := application.LoadInstallationOverview()
		if err != nil {
			return systemLoadMsg{err: err}
		}
		about, err := application.LoadAbout(ctx)
		return systemLoadMsg{runtime: runtimeState, auth: auth, install: installation, about: about, err: err}
	}
}

func (page *RuntimePage) openCommand(command SystemCommand) (tea.Cmd, error) {
	page.err, page.notice = nil, ""
	switch command {
	case SystemRefresh:
		page.loading = true
		return page.loadCmd(), nil
	case InstallRun:
		page.form, page.installForm = newInstallForm()
		page.pending, page.overlay = command, systemOverlayForm
		return page.form.Init(), nil
	case UpdateApply:
		page.form, page.updateForm = newUpdateForm()
		page.pending, page.overlay = command, systemOverlayForm
		return page.form.Init(), nil
	case RuntimeForeground:
		page.external = &application.ExternalCommand{Command: "cgm serve", Reason: "The foreground runtime owns the terminal. Exit the TUI before starting it."}
		page.overlay = systemOverlayExternal
		return nil, nil
	case ConfigInitialize:
		page.external = &application.ExternalCommand{Command: "cgm init", Reason: "Initialization creates new plaintext MCP/admin tokens. Run it outside the TUI so the CLI can present the one-time credentials directly."}
		page.overlay = systemOverlayExternal
		return nil, nil
	case ConfigUninitialize:
		page.external = &application.ExternalCommand{Command: "cgm uninit", Reason: "Uninitialize permanently removes local ChatGPT MCP configuration and state. Run this destructive command explicitly outside the TUI."}
		page.overlay = systemOverlayExternal
		return nil, nil
	case AuthMCPRotate, AuthAdminRotate, InstallCleanup, AliasRemove, RuntimeDownUser, RuntimeDownSystem, RuntimeRestartUser, RuntimeRestartSystem:
		page.pending = command
		page.confirm = component.NewConfirmButtons(page.confirmActionLabel(), "Cancel", false)
		page.overlay = systemOverlayConfirm
		return nil, nil
	case RuntimeUpUser, RuntimeUpSystem, RuntimeReload, AuthMCPEnable, AuthMCPDisable, AuthAdminEnable, AuthAdminDisable, AliasInstall, UpdateCheck:
		return page.startOperation(command), nil
	default:
		return nil, fmt.Errorf("unsupported system action: %s", command)
	}
}

func (page *RuntimePage) submitForm() tea.Cmd {
	switch page.pending {
	case InstallRun:
		if page.installForm == nil || !page.installForm.Confirm {
			page.closeOverlay()
			page.err = fmt.Errorf("installation was not confirmed")
			return nil
		}
	case UpdateApply:
		if page.updateForm == nil || !page.updateForm.Confirm {
			page.closeOverlay()
			page.err = fmt.Errorf("update was not confirmed")
			return nil
		}
	default:
		return nil
	}
	command := page.pending
	page.overlay = systemOverlayNone
	return page.startOperation(command)
}

func (page *RuntimePage) updateConfirm(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		page.closeOverlay()
		return nil
	case "enter":
		if !page.confirm.AffirmativeSelected() {
			page.closeOverlay()
			return nil
		}
		command := page.pending
		page.overlay = systemOverlayNone
		return page.startOperation(command)
	default:
		return page.confirm.Update(msg)
	}
}

func (page *RuntimePage) startOperation(command SystemCommand) tea.Cmd {
	page.cancelOperation()
	page.operationID++
	id := page.operationID
	ctx, cancel := context.WithTimeout(page.ctx, systemOperationTimeout)
	page.operationCancel = cancel
	progress := component.NewProgress(systemOperationTitle(command))
	page.progress, page.pending, page.overlay = &progress, command, systemOverlayOperation
	installOptions := application.InstallCurrentOptions{}
	if page.installForm != nil {
		installOptions = page.installForm.Options()
	}
	updateOptions := application.UpdateApplyOptions{}
	if page.updateForm != nil {
		updateOptions = page.updateForm.Options()
	}
	operation := func() tea.Msg {
		msg := systemOperationMsg{id: id, command: command}
		switch command {
		case RuntimeUpUser:
			result, err := application.ManagedRuntimeAction(ctx, "up", managed.ScopeUser)
			msg.err, msg.external = err, result.External
		case RuntimeUpSystem:
			result, err := application.ManagedRuntimeAction(ctx, "up", managed.ScopeSystem)
			msg.err, msg.external = err, result.External
		case RuntimeDownUser:
			result, err := application.ManagedRuntimeAction(ctx, "down", managed.ScopeUser)
			msg.err, msg.external = err, result.External
		case RuntimeDownSystem:
			result, err := application.ManagedRuntimeAction(ctx, "down", managed.ScopeSystem)
			msg.err, msg.external = err, result.External
		case RuntimeRestartUser:
			result, err := application.ManagedRuntimeAction(ctx, "restart", managed.ScopeUser)
			msg.err, msg.external = err, result.External
		case RuntimeRestartSystem:
			result, err := application.ManagedRuntimeAction(ctx, "restart", managed.ScopeSystem)
			msg.err, msg.external = err, result.External
		case RuntimeReload:
			_, msg.err = application.ReloadConfig(ctx)
		case AuthMCPEnable:
			_, msg.err = application.SetAuthEnabled("mcp", true)
		case AuthMCPDisable:
			_, msg.err = application.SetAuthEnabled("mcp", false)
		case AuthAdminEnable:
			_, msg.err = application.SetAuthEnabled("admin", true)
		case AuthAdminDisable:
			_, msg.err = application.SetAuthEnabled("admin", false)
		case AuthMCPRotate:
			msg.token, _, msg.err = application.RotateAuthToken("mcp")
		case AuthAdminRotate:
			msg.token, _, msg.err = application.RotateAuthToken("admin")
		case AliasInstall:
			_, msg.err = application.SetAliasInstalled(true)
		case AliasRemove:
			_, msg.err = application.SetAliasInstalled(false)
		case InstallRun:
			_, msg.err = application.InstallCurrent(installOptions)
		case InstallCleanup:
			_, msg.err = application.CleanupLegacyInstallations()
		case UpdateCheck:
			msg.update, msg.err = application.CheckForUpdate(ctx)
		case UpdateApply:
			result, err := application.ApplyUpdate(ctx, updateOptions)
			msg.err, msg.external, msg.notice = err, result.External, result.Notice
		}
		return msg
	}
	return tea.Batch(progress.Init(), operation)
}

func (page *RuntimePage) finishOperation(msg systemOperationMsg) tea.Cmd {
	if msg.id != page.operationID {
		return nil
	}
	page.cancelOperation()
	page.progress = nil
	toast := updateToastCmd(msg)
	if msg.err != nil {
		page.overlay = systemOverlayNone
		page.err = msg.err
		return toast
	}
	if msg.token != "" {
		page.secret = msg.token
		page.secretKind = "admin"
		if msg.command == AuthMCPRotate {
			page.secretKind = "mcp"
		}
		page.overlay = systemOverlaySecret
	} else if msg.external != nil {
		page.external, page.overlay = msg.external, systemOverlayExternal
	} else {
		page.overlay = systemOverlayNone
		if msg.command == UpdateCheck || msg.command == UpdateApply {
			page.notice = ""
		} else {
			page.notice = operationNotice(msg)
		}
	}
	page.installForm, page.updateForm = nil, nil
	return tea.Batch(page.loadCmd(), toast)
}

func (page *RuntimePage) cancelOperation() {
	if page.operationCancel != nil {
		page.operationCancel()
		page.operationCancel = nil
	}
}

func (page *RuntimePage) closeOverlay() {
	page.cancelOperation()
	page.overlay = systemOverlayNone
	page.form = component.Form{}
	page.confirm = component.ConfirmButtons{}
	page.progress = nil
	page.pending = ""
	page.installForm, page.updateForm = nil, nil
	page.secret, page.secretKind = "", ""
	page.external = nil
}

func (page *RuntimePage) copyOverlayValue() tea.Cmd {
	value := page.secret
	if page.overlay == systemOverlayExternal && page.external != nil {
		value = page.external.Command
	}
	return func() tea.Msg { return systemCopyMsg{err: component.CopyText(value)} }
}

func (page *RuntimePage) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if msg.String() == "r" {
		page.loading = true
		return page.loadCmd(), true
	}
	switch page.selectedID() {
	case "runtime":
		scope := page.runtimeScope()
		switch msg.String() {
		case "u":
			return page.openRuntimeAction("up", scope), true
		case "d":
			return page.openRuntimeAction("down", scope), true
		case "x":
			return page.openRuntimeAction("restart", scope), true
		case "l":
			cmd, _ := page.openCommand(RuntimeReload)
			return cmd, true
		case "f":
			cmd, _ := page.openCommand(RuntimeForeground)
			return cmd, true
		}
	case "service.user":
		return page.handleServiceKey(msg, managed.ScopeUser)
	case "service.system":
		return page.handleServiceKey(msg, managed.ScopeSystem)
	case "auth.mcp":
		if msg.String() == "e" {
			command := AuthMCPEnable
			if page.auth.MCPEnabled {
				command = AuthMCPDisable
			}
			cmd, _ := page.openCommand(command)
			return cmd, true
		}
		if msg.String() == "t" {
			cmd, _ := page.openCommand(AuthMCPRotate)
			return cmd, true
		}
	case "auth.admin":
		if msg.String() == "e" {
			command := AuthAdminEnable
			if page.auth.AdminEnabled {
				command = AuthAdminDisable
			}
			cmd, _ := page.openCommand(command)
			return cmd, true
		}
		if msg.String() == "t" {
			cmd, _ := page.openCommand(AuthAdminRotate)
			return cmd, true
		}
	case "installation":
		if msg.String() == "i" {
			cmd, _ := page.openCommand(InstallRun)
			return cmd, true
		}
		if msg.String() == "c" {
			cmd, _ := page.openCommand(InstallCleanup)
			return cmd, true
		}
	case "alias":
		if msg.String() == "a" {
			command := AliasInstall
			if page.install.Alias.State == install.AliasInstalled {
				command = AliasRemove
			}
			cmd, _ := page.openCommand(command)
			return cmd, true
		}
	case "update":
		if msg.String() == "k" {
			cmd, _ := page.openCommand(UpdateCheck)
			return cmd, true
		}
		if msg.String() == "u" {
			cmd, _ := page.openCommand(UpdateApply)
			return cmd, true
		}
	}
	return nil, false
}

func (page *RuntimePage) handleServiceKey(msg tea.KeyPressMsg, scope managed.Scope) (tea.Cmd, bool) {
	switch msg.String() {
	case "u":
		return page.openRuntimeAction("up", scope), true
	case "d":
		return page.openRuntimeAction("down", scope), true
	case "x":
		return page.openRuntimeAction("restart", scope), true
	default:
		return nil, false
	}
}

func (page *RuntimePage) openRuntimeAction(action string, scope managed.Scope) tea.Cmd {
	cmd, _ := page.openCommand(runtimeCommand(action, scope))
	return cmd
}

func runtimeCommand(action string, scope managed.Scope) SystemCommand {
	system := scope == managed.ScopeSystem
	switch action {
	case "up":
		if system {
			return RuntimeUpSystem
		}
		return RuntimeUpUser
	case "down":
		if system {
			return RuntimeDownSystem
		}
		return RuntimeDownUser
	default:
		if system {
			return RuntimeRestartSystem
		}
		return RuntimeRestartUser
	}
}

func (page *RuntimePage) runtimeScope() managed.Scope {
	if page.runtime.Running && page.runtime.Status.Managed && page.runtime.Status.ServiceScope == string(managed.ScopeSystem) {
		return managed.ScopeSystem
	}
	return managed.ScopeUser
}

func (page *RuntimePage) selectedID() string {
	row, ok := page.browser.Selected()
	if !ok {
		return ""
	}
	return row.ID
}

func (page *RuntimePage) rebuildBrowser(selected string) tea.Cmd {
	rows := []component.Row{page.runtimeRow(), page.serviceRow(page.runtime.UserService)}
	if page.runtime.SystemService.Supported {
		rows = append(rows, page.serviceRow(page.runtime.SystemService))
	}
	rows = append(rows, page.authRow("mcp"), page.authRow("admin"), page.installRow(), page.aliasRow(), page.updateRow(), page.aboutRow())
	cmd := page.browser.ReplaceRows(rows, selected)
	page.syncBrowserHelp()
	return cmd
}

func (page *RuntimePage) resizeBrowser() tea.Cmd {
	feedback := ""
	if page.err != nil {
		feedback = component.Banner(page.err.Error(), component.ToneDanger)
	} else if page.notice != "" {
		feedback = component.Muted(page.notice)
	}
	height := max(1, page.height-lipgloss.Height(component.PageTitle("Runtime & System", page.width))-lipgloss.Height(page.statusView(page.width))-pageFeedbackHeight(feedback))
	updated, cmd := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: height})
	page.browser = updated.(component.Browser)
	page.syncBrowserHelp()
	return cmd
}

func (page *RuntimePage) runtimeRow() component.Row {
	state := "stopped"
	fields := [][2]string{{"State", state}}
	if page.runtime.Running {
		status := page.runtime.Status
		mode := "foreground"
		if status.Managed {
			mode = "managed / " + status.ServiceScope
		}
		state = "running"
		fields = [][2]string{{"State", state}, {"PID", fmt.Sprint(status.PID)}, {"Session", status.RunID}, {"Mode", mode}, {"Service", status.ServiceID}, {"Started", timeLabel(status.StartedAt)}, {"MCP", endpoint(status.ServerPort, "/mcp")}, {"Admin", adminEndpoint(status)}, {"Exposure", string(status.Exposure)}, {"Tunnel", runtimeTunnelStatus(status)}}
	}
	return component.Row{ID: "runtime", Title: "Runtime", Description: state, Search: "runtime status service server", DetailTitle: "Runtime", Detail: detailFields(fields...)}
}

func (page *RuntimePage) serviceRow(service application.ServiceOverview) component.Row {
	state := "not installed"
	if service.Installed {
		state = "installed"
	}
	if service.Running {
		state = "running"
	}
	if service.Err != "" {
		state = "unavailable"
	}
	title := "User service"
	if service.Scope == managed.ScopeSystem {
		title = "System service"
	}
	fields := [][2]string{{"Scope", string(service.Scope)}, {"State", state}, {"Backend", service.Backend}, {"Service", service.ID}, {"PID", valueInt(service.PID)}, {"Config", service.ConfigRoot}, {"Persistence", service.Warning}, {"Error", service.Err}}
	return component.Row{ID: "service." + string(service.Scope), Title: title, Description: state, Search: "service " + string(service.Scope) + " " + service.Backend, DetailTitle: title, Detail: detailFields(fields...)}
}

func (page *RuntimePage) authRow(kind string) component.Row {
	enabled, configured := page.auth.MCPEnabled, page.auth.MCPConfigured
	if kind == "admin" {
		enabled, configured = page.auth.AdminEnabled, page.auth.AdminConfigured
	}
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	configuredText := "missing"
	if configured {
		configuredText = "configured"
	}
	return component.Row{ID: "auth." + kind, Title: strings.ToUpper(kind) + " authentication", Description: state + " · token " + configuredText, Search: "auth token " + kind, DetailTitle: strings.ToUpper(kind) + " authentication", Detail: detailFields([2]string{"Enabled", fmt.Sprint(enabled)}, [2]string{"Token", configuredText}, [2]string{"Security", "Token hashes are persisted; plaintext is shown once after rotation."})}
}

func (page *RuntimePage) installRow() component.Row {
	method := string(page.install.Detection.Method)
	state := method
	if page.install.Managed {
		state = "managed direct · " + page.install.ManagedVersion
	}
	return component.Row{ID: "installation", Title: "Installation", Description: state, Search: "install managed layout cleanup migration", DetailTitle: "Installation", Detail: detailFields([2]string{"Method", method}, [2]string{"Executable", page.install.Detection.Executable}, [2]string{"Root", page.install.Detection.Root}, [2]string{"Managed", fmt.Sprint(page.install.Managed)}, [2]string{"Current", page.install.ManagedVersion}, [2]string{"Update policy", string(page.install.Policy.Action)}, [2]string{"Guidance", page.install.Policy.Message})}
}

func (page *RuntimePage) aliasRow() component.Row {
	state, path, target := "unavailable", "", ""
	if page.install.AliasAvailable {
		state, path, target = string(page.install.Alias.State), page.install.Alias.Path, page.install.Alias.Target
	}
	return component.Row{ID: "alias", Title: "cgm alias", Description: state, Search: "alias cgm command", DetailTitle: "cgm alias", Detail: detailFields([2]string{"State", state}, [2]string{"Path", path}, [2]string{"Target", target})}
}

func (page *RuntimePage) updateRow() component.Row {
	status, latest, checked := string(page.install.Policy.Action), "", ""
	if page.install.CachedUpdate != nil {
		status, latest, checked = string(page.install.CachedUpdate.Status), page.install.CachedUpdate.Latest, page.install.CachedUpdate.CheckedAt.Local().Format(time.RFC3339)
	}
	return component.Row{ID: "update", Title: "Update", Description: status, Search: "update release version latest", DetailTitle: "Update", Detail: detailFields([2]string{"Current", page.about.Version}, [2]string{"Status", status}, [2]string{"Latest", latest}, [2]string{"Checked", checked}, [2]string{"Policy", page.install.Policy.Message}, [2]string{"External command", page.install.Policy.Command})}
}

func (page *RuntimePage) aboutRow() component.Row {
	serverUptime := "stopped"
	if page.about.RuntimeRunning {
		serverUptime = page.about.ServerUptime.String()
	}
	machine := "unavailable"
	if page.about.MachineUptimeOK {
		machine = page.about.MachineUptime.String()
	}
	return component.Row{ID: "about", Title: "About", Description: page.about.Version + " · " + runtime.GOOS + "/" + runtime.GOARCH, Search: "about version commit build uptime paths", DetailTitle: "About", Detail: detailFields([2]string{"Version", page.about.Version}, [2]string{"Commit", page.about.Commit}, [2]string{"Build time", page.about.BuildTime}, [2]string{"Server uptime", serverUptime}, [2]string{"Machine uptime", machine}, [2]string{"Executable", page.about.Executable}, [2]string{"Config", page.about.ConfigPath}, [2]string{"Config root", page.about.ConfigRoot}, [2]string{"Logs", page.about.LogsPath}, [2]string{"Install method", string(page.about.InstallMethod)}, [2]string{"Install root", page.about.InstallRoot})}
}

func (page *RuntimePage) statusView(width int) string {
	state := component.ToneText("● RUNNING", component.ToneSuccess)
	if !page.runtime.Running {
		state = component.Muted("○ STOPPED")
	}
	mode := "no active runtime"
	if page.runtime.Running {
		mode = "foreground"
		if page.runtime.Status.Managed {
			mode = page.runtime.Status.ServiceScope + " · " + page.runtime.Status.ServiceID
		}
	}
	return component.TwoColumn(component.KeyValue("Runtime", state), component.KeyValue("Mode", mode), width)
}

func (page *RuntimePage) syncBrowserHelp() {
	refresh := component.Binding([]string{"r"}, "r", "refresh")
	switch page.selectedID() {
	case "runtime", "service.user", "service.system":
		bindings := []key.Binding{refresh, component.Binding([]string{"u"}, "u", "up"), component.Binding([]string{"x"}, "x", "restart"), component.Binding([]string{"d"}, "d", "down")}
		if page.selectedID() == "runtime" {
			if page.runtime.Running {
				bindings = append(bindings, component.Binding([]string{"l"}, "l", "reload"))
			}
			bindings = append(bindings, component.Binding([]string{"f"}, "f", "foreground"))
		}
		page.browser.SetHelpBindings(bindings...)
	case "auth.mcp":
		label := "enable"
		if page.auth.MCPEnabled {
			label = "disable"
		}
		bindings := []key.Binding{refresh, component.Binding([]string{"t"}, "t", "rotate token")}
		if page.auth.MCPConfigured || page.auth.MCPEnabled {
			bindings = append(bindings, component.Binding([]string{"e"}, "e", label))
		}
		page.browser.SetHelpBindings(bindings...)
	case "auth.admin":
		label := "enable"
		if page.auth.AdminEnabled {
			label = "disable"
		}
		bindings := []key.Binding{refresh, component.Binding([]string{"t"}, "t", "rotate token")}
		if page.auth.AdminConfigured || page.auth.AdminEnabled {
			bindings = append(bindings, component.Binding([]string{"e"}, "e", label))
		}
		page.browser.SetHelpBindings(bindings...)
	case "installation":
		page.browser.SetHelpBindings(refresh, component.Binding([]string{"i"}, "i", "install"), component.Binding([]string{"c"}, "c", "cleanup"))
	case "alias":
		label := "install"
		if page.install.Alias.State == install.AliasInstalled {
			label = "remove"
		}
		if page.install.AliasAvailable {
			page.browser.SetHelpBindings(refresh, component.Binding([]string{"a"}, "a", label))
		} else {
			page.browser.SetHelpBindings(refresh)
		}
	case "update":
		page.browser.SetHelpBindings(refresh, component.Binding([]string{"k"}, "k", "check"), component.Binding([]string{"u"}, "u", "update"))
	default:
		page.browser.SetHelpBindings(refresh)
	}
}

func (page *RuntimePage) confirmActionLabel() string {
	switch page.pending {
	case AuthMCPRotate, AuthAdminRotate:
		return "Rotate"
	case InstallCleanup:
		return "Clean"
	case AliasRemove:
		return "Remove"
	case RuntimeDownUser, RuntimeDownSystem:
		return "Stop & remove"
	case RuntimeRestartUser, RuntimeRestartSystem:
		return "Restart"
	default:
		return "Confirm"
	}
}

func (page *RuntimePage) confirmTitle() string {
	switch page.pending {
	case AuthMCPRotate:
		return "Rotate MCP token?"
	case AuthAdminRotate:
		return "Rotate admin token?"
	case InstallCleanup:
		return "Clean legacy installations?"
	case AliasRemove:
		return "Remove cgm alias?"
	case RuntimeDownUser, RuntimeDownSystem:
		return "Stop and remove managed service?"
	case RuntimeRestartUser, RuntimeRestartSystem:
		return "Restart managed service?"
	default:
		return "Confirm action?"
	}
}

func (page *RuntimePage) confirmDescription() string {
	switch page.pending {
	case AuthMCPRotate, AuthAdminRotate:
		return "The previous token stops working immediately. The new plaintext token is shown once and is not persisted by the TUI."
	case InstallCleanup:
		return "Only verified legacy standalone installations are eligible for removal; the current executable is preserved."
	case AliasRemove:
		return "The managed installation remains intact; only the cgm alias is removed."
	case RuntimeDownUser, RuntimeDownSystem:
		return "The managed service is stopped and uninstalled. Configuration and runtime logs are preserved."
	case RuntimeRestartUser, RuntimeRestartSystem:
		return "The managed runtime is stopped and started again with the current configuration."
	default:
		return "Review the action before continuing."
	}
}

func operationNotice(msg systemOperationMsg) string {
	if msg.notice != "" {
		return msg.notice
	}
	if msg.command == UpdateCheck {
		return fmt.Sprintf("Update check: %s · latest %s", msg.update.Status, msg.update.Latest)
	}
	switch msg.command {
	case RuntimeReload:
		return "Runtime configuration reloaded"
	case AliasInstall:
		return "cgm alias installed"
	case AliasRemove:
		return "cgm alias removed"
	case InstallRun:
		return "Managed installation updated"
	case InstallCleanup:
		return "Legacy installation cleanup completed"
	case UpdateApply:
		return "Update completed"
	case AuthMCPEnable, AuthAdminEnable:
		return "Authentication enabled"
	case AuthMCPDisable, AuthAdminDisable:
		return "Authentication disabled"
	default:
		return "Runtime/system action completed"
	}
}

func updateToast(msg systemOperationMsg) (ToastMsg, bool) {
	if msg.command != UpdateCheck && msg.command != UpdateApply || msg.external != nil {
		return ToastMsg{}, false
	}
	title := "Update"
	if msg.command == UpdateCheck {
		title = "Update check"
	}
	tone, message := component.ToneSuccess, operationNotice(msg)
	if msg.err != nil {
		tone, message = component.ToneDanger, msg.err.Error()
	}
	return ToastMsg{Title: title, Message: message, Tone: tone}, true
}

func updateToastCmd(msg systemOperationMsg) tea.Cmd {
	toast, ok := updateToast(msg)
	if !ok {
		return nil
	}
	return func() tea.Msg { return toast }
}

func systemOperationTitle(command SystemCommand) string {
	switch command {
	case UpdateCheck:
		return "Checking for updates"
	case UpdateApply:
		return "Verifying and applying update"
	case InstallRun:
		return "Installing managed binary"
	case InstallCleanup:
		return "Cleaning legacy installations"
	case RuntimeReload:
		return "Reloading runtime configuration"
	case AuthMCPRotate, AuthAdminRotate:
		return "Rotating authentication token"
	default:
		return "Applying system action"
	}
}

func timeLabel(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format(time.RFC3339)
}

func endpoint(port int, path string) string {
	if port <= 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
}

func adminEndpoint(status runtimecontrol.RuntimeStatus) string {
	if !status.AdminEnabled {
		return "disabled"
	}
	return endpoint(status.AdminPort, "/")
}

func valueInt(value int) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprint(value)
}

func runtimeTunnelStatus(status runtimecontrol.RuntimeStatus) string {
	if !status.TunnelEnabled {
		return "disabled"
	}
	if !status.TunnelConfigured {
		return "enabled · not configured"
	}
	switch {
	case status.TunnelReady:
		return "connected"
	case status.TunnelRestarting:
		return "reconnecting"
	case status.TunnelRunning:
		return "connecting"
	default:
		return "starting"
	}
}
