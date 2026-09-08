package page

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

const configOperationTimeout = 60 * time.Second

type ConfigCommand string

const (
	ConfigRefresh ConfigCommand = "config.refresh"
	ConfigEdit    ConfigCommand = "config.edit"
	ConfigVerify  ConfigCommand = "config.verify"
	ConfigReload  ConfigCommand = "config.reload"
	ConfigMigrate ConfigCommand = "config.migrate"
	ConfigConvert ConfigCommand = "config.convert"
	ConfigExport  ConfigCommand = "config.export"
	ConfigImport  ConfigCommand = "config.import"
)

type ConfigCommandMsg struct {
	Command    ConfigCommand
	ResourceID string
}

type configOverlay uint8

const (
	configOverlayNone configOverlay = iota
	configOverlayForm
	configOverlayOperation
)

type configLoadMsg struct {
	overview application.ConfigOverview
	err      error
}

type configOperationMsg struct {
	operationID uint64
	command     ConfigCommand
	mutation    application.ConfigMutationResult
	verify      config.VerifyResult
	reload      application.ConfigReloadResult
	format      configformat.Format
	converted   int
	files       int
	secrets     int
	path        string
	err         error
}

type ConfigPage struct {
	ctx             context.Context
	resourceID      string
	overview        application.ConfigOverview
	browser         component.Browser
	detail          component.DetailPage
	loaded          bool
	loading         bool
	overlay         configOverlay
	form            component.Form
	command         ConfigCommand
	targetKey       string
	fieldForm       *configFieldFormData
	convertForm     *configConvertFormData
	bundleForm      *configBundleFormData
	operationCancel context.CancelFunc
	operationID     uint64
	progress        *component.Progress
	notice          string
	err             error
	width           int
	height          int
}

type configDomain struct {
	ID          string
	Title       string
	Description string
}

var configDomains = []configDomain{
	{ID: "runtime", Title: "Runtime & Network", Description: "MCP HTTP and admin server configuration"},
	{ID: "access", Title: "Access & Security", Description: "Authentication and filesystem access"},
	{ID: "shell", Title: "Shell & Execution", Description: "Approval, sandbox, environment, and network policy"},
	{ID: "features", Title: "Features", Description: "Ponytail and Caveman behavior"},
	{ID: "tunnel", Title: "Tunnel", Description: "OpenAI Secure MCP Tunnel configuration"},
	{ID: "storage", Title: "Storage & Maintenance", Description: "Storage, verification, reload, import, export, and migration"},
}

func NewConfig(ctx context.Context) (*ConfigPage, error) {
	return NewConfigRoute(ctx, "")
}

func NewConfigRoute(ctx context.Context, resourceID string) (*ConfigPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	page := &ConfigPage{ctx: ctx, resourceID: strings.TrimSpace(resourceID)}
	page.rebuildBrowser("")
	return page, nil
}

func (page *ConfigPage) Init() tea.Cmd {
	if page == nil {
		return nil
	}
	page.loading = true
	return page.loadCmd()
}

func (page *ConfigPage) OverlayActive() bool {
	return page != nil && page.overlay != configOverlayNone
}

func (page *ConfigPage) InputActive() bool {
	return page != nil && (page.overlay == configOverlayForm || page.isBrowserRoute() && page.browser.InputActive())
}

func (page *ConfigPage) Notice() string {
	if page == nil {
		return ""
	}
	return page.notice
}

func (page *ConfigPage) SetNotice(value string) {
	if page != nil {
		page.notice = strings.TrimSpace(value)
	}
}

func (page *ConfigPage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	switch msg := message.(type) {
	case configLoadMsg:
		page.loading = false
		if msg.err != nil {
			page.err = msg.err
			return page, nil
		}
		page.loaded, page.err = true, nil
		page.overview = msg.overview
		if page.isFieldRoute() {
			page.syncDetail()
		} else {
			page.rebuildBrowser(page.selectedKey())
		}
		return page, nil
	case configOperationMsg:
		return page, page.finishOperation(msg)
	case tea.WindowSizeMsg:
		page.width, page.height = msg.Width, msg.Height
		var browserCmd tea.Cmd
		if page.isFieldRoute() {
			page.detail.Resize(msg.Width, msg.Height)
		} else {
			browserCmd = page.resizeBrowser()
		}
		if page.overlay == configOverlayForm {
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
		if page.overlay == configOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		return page, nil
	case ConfigCommandMsg:
		cmd, err := page.openCommand(msg.Command, msg.ResourceID)
		if err != nil {
			page.err = err
		}
		return page, cmd
	case component.BrowserOpenMsg:
		if msg.Row.ID != "" && page.isBrowserRoute() {
			if page.resourceID == "" {
				return page, func() tea.Msg { return NavigateMsg{Path: []string{"config", msg.Row.ID}} }
			}
			return page, func() tea.Msg { return NavigateMsg{Path: []string{"config", msg.Row.ID}} }
		}
		return page, nil
	case tea.KeyPressMsg:
		if page.overlay == configOverlayOperation {
			if msg.String() == "esc" {
				page.cancelOperation()
				return page, nil
			}
			if page.progress != nil {
				updated, cmd := page.progress.Update(msg)
				page.progress = &updated
				return page, cmd
			}
			return page, nil
		}
		if page.overlay == configOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		if page.isBrowserRoute() && page.browser.InputActive() {
			updated, cmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			return page, cmd
		}
		if page.isFieldRoute() {
			updated, cmd := page.detail.Update(msg)
			page.detail = updated
			return page, cmd
		} else if cmd, handled := page.handleKey(msg); handled {
			return page, cmd
		}
	}
	if page.overlay == configOverlayForm {
		updated, cmd := page.form.Update(message)
		page.form = updated
		return page, cmd
	}
	if page.isFieldRoute() {
		updated, cmd := page.detail.Update(message)
		page.detail = updated
		return page, cmd
	}
	updated, cmd := page.browser.Update(message)
	page.browser = updated.(component.Browser)
	return page, cmd
}

func (page *ConfigPage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "Config page unavailable", "")
	}
	page.width, page.height = width, height
	if !page.loaded && page.loading {
		return component.StateView(component.PageLoading, "Loading configuration", "")
	}
	var content string
	if page.isFieldRoute() {
		page.detail.SetFeedback(page.notice, page.err)
		page.detail.Resize(width, height)
		content = page.detail.View()
	} else {
		pageTitle := "Configuration"
		overview := page.overviewView(width)
		if page.isDomainRoute() {
			pageTitle = "Configuration / " + page.domainTitle()
			overview = component.WrapKeyValue("", page.domainSummary(page.resourceID), width)
		}
		title := component.PageTitleNotice(pageTitle, page.notice, width)
		headerHeight := lipgloss.Height(title) + lipgloss.Height(overview)
		feedback := ""
		if page.err != nil {
			feedback = component.BannerWidth(page.err.Error(), component.ToneDanger, width)
		}
		browserHeight := max(1, height-headerHeight-pageFeedbackHeight(feedback))
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: width, Height: browserHeight})
		page.browser = updated.(component.Browser)
		content = title + "\n" + overview + "\n" + prependPageFeedback(feedback, page.browser.Content())
	}
	switch page.overlay {
	case configOverlayForm:
		content = component.CenterOverlay(content, component.Modal(page.form.View(), overlayWidth(width, 80)), width, height)
	case configOverlayOperation:
		body := ""
		if page.progress != nil {
			body = page.progress.View()
		}
		body += "\n\n" + component.Muted("Esc cancel")
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 72)), width, height)
	}
	return content
}

func (page *ConfigPage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	switch page.overlay {
	case configOverlayForm:
		return formOverlayMouseTargets(page.form, overlayWidth(page.width, 80), page.width, page.height, originX, originY, z+20)
	case configOverlayOperation:
		return []component.MouseTarget{mouseBlocker(originX, originY, page.width, page.height, z+20)}
	default:
		if page.isFieldRoute() {
			return page.detail.MouseTargets(originX, originY, z)
		}
		feedback := ""
		if page.err != nil {
			feedback = component.BannerWidth(page.err.Error(), component.ToneDanger, page.width)
		}
		offsetY := lipgloss.Height(component.PageTitleNotice("Configuration", page.notice, page.width)) + lipgloss.Height(page.overviewView(page.width)) + pageFeedbackHeight(feedback)
		return page.browser.MouseTargets(originX, originY+offsetY, z)
	}
}

func (page *ConfigPage) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if page.isDomainRoute() && msg.String() == "e" {
		cmd, err := page.openCommand(ConfigEdit, page.selectedKey())
		if err != nil {
			page.err = err
		}
		return cmd, true
	}
	commands := map[string]ConfigCommand{"r": ConfigRefresh}
	command, ok := commands[msg.String()]
	if !ok {
		return nil, false
	}
	cmd, err := page.openCommand(command, "")
	if err != nil {
		page.err = err
	}
	return cmd, true
}

func (page *ConfigPage) openCommand(command ConfigCommand, resourceID string) (tea.Cmd, error) {
	page.err, page.notice = nil, ""
	page.command, page.targetKey = command, strings.TrimSpace(resourceID)
	switch command {
	case ConfigRefresh:
		page.loading = true
		return page.loadCmd(), nil
	case ConfigEdit:
		if !page.loaded {
			return nil, fmt.Errorf("configuration is still loading")
		}
		if page.targetKey == "" {
			page.targetKey = page.selectedKey()
		}
		if page.targetKey == "" {
			return nil, fmt.Errorf("select a configuration field first")
		}
		spec, ok := config.FieldByKey(page.targetKey)
		if !ok {
			return nil, fmt.Errorf("unknown config field: %s", page.targetKey)
		}
		if !spec.Editable {
			page.notice = spec.Guidance
			if page.notice == "" {
				page.notice = spec.Key + " is read-only"
			}
			return nil, nil
		}
		form, data, err := newConfigFieldForm(page.overview.Config, spec)
		if err != nil {
			return nil, err
		}
		page.form, page.fieldForm, page.overlay = form, data, configOverlayForm
		return page.form.Init(), nil
	case ConfigVerify:
		return page.startOperation(command, "Verifying configuration", func(context.Context) configOperationMsg {
			result, err := application.VerifyConfig()
			return configOperationMsg{command: command, verify: result, err: err}
		}), nil
	case ConfigReload:
		if !page.overview.RuntimeRunning {
			return nil, fmt.Errorf("runtime is not running; the next start will load persisted configuration")
		}
		return page.startOperation(command, "Reloading runtime configuration", func(ctx context.Context) configOperationMsg {
			result, err := application.ReloadConfig(ctx)
			return configOperationMsg{command: command, reload: result, err: err}
		}), nil
	case ConfigMigrate:
		return page.startOperation(command, "Migrating stored credentials", func(context.Context) configOperationMsg {
			err := application.MigrateLegacySecrets()
			return configOperationMsg{command: command, err: err}
		}), nil
	case ConfigConvert:
		page.form, page.convertForm = newConfigConvertForm(page.overview.Source.Format)
		page.overlay = configOverlayForm
		return page.form.Init(), nil
	case ConfigExport:
		page.form, page.bundleForm = newConfigBundleForm(true)
		page.overlay = configOverlayForm
		return page.form.Init(), nil
	case ConfigImport:
		page.form, page.bundleForm = newConfigBundleForm(false)
		page.overlay = configOverlayForm
		return page.form.Init(), nil
	default:
		return nil, fmt.Errorf("unsupported config action: %s", command)
	}
}

func (page *ConfigPage) submitForm() tea.Cmd {
	switch page.command {
	case ConfigEdit:
		key, raw := page.targetKey, configFieldFormValue(page.fieldForm)
		page.fieldForm = nil
		return page.startOperation(page.command, "Saving configuration", func(context.Context) configOperationMsg {
			result, err := application.SetConfigField(key, raw)
			return configOperationMsg{command: ConfigEdit, mutation: result, err: err}
		})
	case ConfigConvert:
		data := page.convertForm
		page.convertForm = nil
		if data == nil || !data.Confirm {
			page.closeOverlay()
			page.notice = "Format conversion cancelled"
			return nil
		}
		format, err := configformat.Parse(data.Format)
		if err != nil {
			page.err = err
			return nil
		}
		return page.startOperation(page.command, "Converting configuration format", func(context.Context) configOperationMsg {
			count, err := application.ConvertConfig(format)
			return configOperationMsg{command: ConfigConvert, format: format, converted: count, err: err}
		})
	case ConfigExport:
		data := page.bundleForm
		page.bundleForm = nil
		return page.startOperation(page.command, "Exporting configuration bundle", func(context.Context) configOperationMsg {
			result, err := application.ExportConfig(data.Path, data.Force)
			return configOperationMsg{command: ConfigExport, path: result.Path, files: result.Files, secrets: result.Secrets, err: err}
		})
	case ConfigImport:
		data := page.bundleForm
		page.bundleForm = nil
		if data == nil || !data.Confirm {
			page.closeOverlay()
			page.notice = "Import cancelled"
			return nil
		}
		return page.startOperation(page.command, "Importing configuration bundle", func(ctx context.Context) configOperationMsg {
			result, err := application.ImportConfig(ctx, data.Path, data.Force)
			return configOperationMsg{command: ConfigImport, files: result.Files, secrets: result.Secrets, err: err}
		})
	default:
		page.err = fmt.Errorf("unsupported config form action: %s", page.command)
		return nil
	}
}

func (page *ConfigPage) startOperation(command ConfigCommand, title string, run func(context.Context) configOperationMsg) tea.Cmd {
	ctx, cancel := context.WithTimeout(page.ctx, configOperationTimeout)
	page.operationID++
	operationID := page.operationID
	page.operationCancel = cancel
	page.command = command
	progress := component.NewProgress(title)
	page.progress = &progress
	page.overlay = configOverlayOperation
	page.err = nil
	return func() tea.Msg {
		message := run(ctx)
		message.operationID = operationID
		return message
	}
}

func (page *ConfigPage) finishOperation(msg configOperationMsg) tea.Cmd {
	if msg.operationID != 0 && msg.operationID != page.operationID {
		return nil
	}
	if page.operationCancel != nil {
		page.operationCancel()
	}
	page.operationCancel = nil
	page.overlay, page.progress = configOverlayNone, nil
	if msg.err != nil {
		page.err = msg.err
		return nil
	}
	page.err = nil
	switch msg.command {
	case ConfigEdit:
		page.overview.Config = msg.mutation.Config
		page.notice = application.ConfigOperationNotice(page.overview.RuntimeRunning)
	case ConfigVerify:
		page.notice = fmt.Sprintf("Configuration verified · %s · %d structured files", msg.verify.Format, msg.verify.Files)
	case ConfigReload:
		page.overview.RuntimeRunning = true
		page.notice = fmt.Sprintf("Runtime configuration reloaded · pid %d · network restarted %t", msg.reload.PID, msg.reload.NetworkRestarted)
	case ConfigMigrate:
		page.notice = "Legacy credentials migrated to the secret store"
	case ConfigConvert:
		page.notice = fmt.Sprintf("Configuration converted to %s · %d files", msg.format, msg.converted)
	case ConfigExport:
		page.notice = fmt.Sprintf("Configuration exported · %d files · %d secrets · %s", msg.files, msg.secrets, msg.path)
	case ConfigImport:
		page.notice = fmt.Sprintf("Configuration imported · %d files · %d secrets", msg.files, msg.secrets)
	}
	return func() tea.Msg {
		overview, err := application.LoadConfigOverview(page.ctx)
		return configLoadMsg{overview: overview, err: err}
	}
}

func (page *ConfigPage) cancelOperation() {
	if page.operationCancel != nil {
		page.operationCancel()
	}
	page.operationCancel = nil
	page.operationID++
	page.overlay, page.progress = configOverlayNone, nil
	page.notice = "Configuration operation cancellation requested"
}

func (page *ConfigPage) closeOverlay() {
	page.overlay = configOverlayNone
	page.form = component.Form{}
	page.fieldForm, page.convertForm, page.bundleForm = nil, nil, nil
	page.command, page.targetKey = "", ""
}

func (page *ConfigPage) loadCmd() tea.Cmd {
	return func() tea.Msg {
		overview, err := application.LoadConfigOverview(page.ctx)
		return configLoadMsg{overview: overview, err: err}
	}
}

func (page *ConfigPage) rebuildBrowser(selected string) {
	if page.isFieldRoute() {
		page.syncDetail()
		return
	}
	helpExpanded := page.browser.HelpExpanded()
	rows := page.configRows()
	title := "Configuration domains"
	bindings := []key.Binding{component.Binding([]string{"s"}, "s", "search"), component.Binding([]string{"r"}, "r", "refresh")}
	if page.isDomainRoute() {
		title = page.domainTitle() + " fields"
		bindings = []key.Binding{component.Binding([]string{"e"}, "e", "edit"), component.Binding([]string{"/"}, "/", "filter"), component.Binding([]string{"r"}, "r", "refresh")}
	}
	page.browser = component.NewBrowser(page.ctx, title, rows, nil).WithTitleVisible(false).WithHelpBindings(bindings...)
	page.browser.SetHelpExpanded(helpExpanded)
	if page.width > 0 && page.height > 0 {
		_ = page.resizeBrowser()
	}
	if selected != "" {
		page.browser.SelectID(selected)
	}
}

func (page *ConfigPage) resizeBrowser() tea.Cmd {
	pageTitle := "Configuration"
	overview := page.overviewView(page.width)
	if page.isDomainRoute() {
		pageTitle = "Configuration / " + page.domainTitle()
		overview = component.WrapKeyValue("", page.domainSummary(page.resourceID), page.width)
	}
	headerHeight := lipgloss.Height(component.PageTitleNotice(pageTitle, page.notice, page.width)) + lipgloss.Height(overview)
	feedback := ""
	if page.err != nil {
		feedback = component.BannerWidth(page.err.Error(), component.ToneDanger, page.width)
	}
	height := max(1, page.height-headerHeight-pageFeedbackHeight(feedback))
	updated, cmd := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: height})
	page.browser = updated.(component.Browser)
	return cmd
}

func (page *ConfigPage) configRows() []component.Row {
	if page.isDomainRoute() {
		return page.domainRows()
	}
	rows := make([]component.Row, 0, len(configDomains))
	for _, domain := range configDomains {
		summary := "loading"
		if page.loaded {
			summary = page.domainSummary(domain.ID)
		}
		rows = append(rows, component.Row{ID: domain.ID, Title: domain.Title, Description: domain.Description, Meta: summary, Search: domain.Title + " " + domain.Description + " " + summary})
	}
	return rows
}

func (page *ConfigPage) domainRows() []component.Row {
	section, ok := configSectionForRoute(page.resourceID)
	if !ok {
		return nil
	}
	rows := make([]component.Row, 0)
	for _, spec := range config.Fields() {
		if spec.Section != section {
			continue
		}
		value := "loading"
		state := config.FieldStateDefault
		if page.loaded {
			if display, err := config.DisplayValue(page.overview.Config, spec); err == nil {
				value = display
			}
			if current, err := config.State(page.overview.Config, spec); err == nil {
				state = current
			}
		}
		rows = append(rows, component.Row{ID: spec.Key, Title: spec.Label, Description: spec.Description, Meta: value + " · " + string(state), Search: strings.Join(append([]string{spec.Label, spec.Key, spec.Description, value, string(state), string(spec.Section)}, spec.Options...), " ")})
	}
	return rows
}

func (page *ConfigPage) syncDetail() {
	key := strings.TrimSpace(page.resourceID)
	spec, ok := config.FieldByKey(key)
	if !ok {
		page.err = fmt.Errorf("unknown config field: %s", key)
		page.detail = component.NewDetailPage("Config · "+key, "unavailable", component.Muted("Configuration field not found."))
		page.detail.SetBindings(component.DetailPageBinding{Key: "f", Desc: "refresh", Message: ConfigCommandMsg{Command: ConfigRefresh, ResourceID: key}})
		return
	}
	value := "loading"
	if page.loaded {
		if display, err := config.DisplayValue(page.overview.Config, spec); err == nil {
			value = display
		}
	}
	mode := string(spec.Kind)
	if !spec.Editable {
		mode = "read-only"
	}
	detail := detailFields([2]string{"Key", spec.Key}, [2]string{"Value", value}, [2]string{"Type", string(spec.Kind)}, [2]string{"Access", mode}, [2]string{"Description", spec.Description})
	if len(spec.Options) > 0 {
		detail += "\n" + detailFields([2]string{"Options", strings.Join(spec.Options, ", ")})
	}
	if spec.Guidance != "" {
		detail += "\n" + detailFields([2]string{"Manage via", spec.Guidance})
	}
	page.detail = component.NewDetailPage("Config · "+spec.Key, value+" · "+mode, detail)
	bindings := []component.DetailPageBinding{{Key: "f", Desc: "refresh", Message: ConfigCommandMsg{Command: ConfigRefresh, ResourceID: spec.Key}}}
	if spec.Editable {
		bindings = append([]component.DetailPageBinding{{Key: "e", Desc: "edit", Message: ConfigCommandMsg{Command: ConfigEdit, ResourceID: spec.Key}}}, bindings...)
	}
	page.detail.SetBindings(bindings...)
	if page.width > 0 && page.height > 0 {
		page.detail.Resize(page.width, page.height)
	}
}

func (page *ConfigPage) selectedKey() string {
	selected, ok := page.browser.Selected()
	if !ok {
		return ""
	}
	return selected.ID
}

func (page *ConfigPage) isBrowserRoute() bool { return page.resourceID == "" || page.isDomainRoute() }

func (page *ConfigPage) isDomainRoute() bool {
	_, ok := configSectionForRoute(page.resourceID)
	return ok
}

func (page *ConfigPage) isFieldRoute() bool { return configFieldRouteCompat(page.resourceID) }

func configFieldRouteCompat(resourceID string) bool {
	if resourceID == "" {
		return false
	}
	if _, ok := configSectionForRoute(resourceID); ok || resourceID == "storage" {
		return false
	}
	_, ok := config.FieldByKey(resourceID)
	return ok
}

func configSectionForRoute(resourceID string) (config.FieldSection, bool) {
	switch strings.TrimSpace(resourceID) {
	case "runtime":
		return config.FieldSectionRuntime, true
	case "access":
		return config.FieldSectionAccess, true
	case "shell":
		return config.FieldSectionShell, true
	case "features":
		return config.FieldSectionFeatures, true
	case "tunnel":
		return config.FieldSectionTunnel, true
	default:
		return "", false
	}
}

func (page *ConfigPage) domainTitle() string {
	for _, domain := range configDomains {
		if domain.ID == page.resourceID {
			return domain.Title
		}
	}
	return "Configuration"
}

func (page *ConfigPage) overviewView(width int) string {
	if !page.loaded {
		if page.err != nil {
			return component.StateView(component.PageError, "Configuration unavailable", page.err.Error())
		}
		return component.StateView(component.PageEmpty, "Configuration not loaded", "")
	}
	status := "runtime stopped"
	if page.overview.RuntimeRunning {
		status = "runtime running"
	}
	initialized := "no"
	if page.overview.Source.Exists {
		initialized = "yes"
	}
	return component.WrapKeyValue("", fmt.Sprintf("%s · initialized %s · %s", page.overview.Source.Format, initialized, status), width)
}

func (page *ConfigPage) domainSummary(domain string) string {
	cfg := page.overview.Config
	switch domain {
	case "runtime":
		return fmt.Sprintf("MCP HTTP %s :%d · Admin %s :%d · exposure %s", configOnOff(cfg.Server.Enabled), cfg.Server.Port, configOnOff(cfg.Admin.Enabled), cfg.Admin.Port, config.NormalizeExposure(cfg.Server.Expose).Mode)
	case "access":
		return fmt.Sprintf("MCP auth %s · Admin auth %s · %d extra filesystem roots", configOnOff(cfg.Auth.MCPEnabled), configOnOff(cfg.Auth.AdminEnabled), len(cfg.Permissions.AllowDirs))
	case "shell":
		return fmt.Sprintf("%s · sandbox %s · network %s · %d command overrides", cfg.Shell.ApprovalPolicy, cfg.Shell.SandboxPolicy, cfg.Shell.NetworkPolicy, len(cfg.Shell.ApprovalAllowCommands)+len(cfg.Shell.ApprovalDenyCommands))
	case "features":
		return fmt.Sprintf("Ponytail %s · Caveman %s", configOnOff(cfg.Features.Ponytail.Active), configOnOff(cfg.Features.Caveman.Active))
	case "tunnel":
		return fmt.Sprintf("%s · runtime key %s · admin key %s", configOnOff(cfg.Tunnel.Enabled), configuredState(cfg.Tunnel.APIKey), configuredState(cfg.Tunnel.AdminKey))
	case "storage":
		return fmt.Sprintf("%s · verify / reload / convert / import / export", page.overview.Source.Format)
	default:
		return ""
	}
}

func configuredState(value string) string {
	if strings.TrimSpace(value) == "" {
		return "not configured"
	}
	return "configured"
}

func configOnOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}
