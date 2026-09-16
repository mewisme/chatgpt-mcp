package page

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"go.mewis.me/chatgpt-mcp/internal/application"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

const pluginOperationTimeout = 2 * time.Minute
const pluginHostInstallTimeout = 10 * time.Minute

type PluginCommand string

const (
	PluginRefresh        PluginCommand = "plugin.refresh"
	PluginInstall        PluginCommand = "plugin.install"
	PluginUpdate         PluginCommand = "plugin.update"
	PluginRollback       PluginCommand = "plugin.rollback"
	PluginPrune          PluginCommand = "plugin.prune"
	PluginUninstall      PluginCommand = "plugin.uninstall"
	PluginForceUninstall PluginCommand = "plugin.uninstall.force"
	PluginEnable         PluginCommand = "plugin.enable"
	PluginDisable        PluginCommand = "plugin.disable"
	PluginVerify         PluginCommand = "plugin.verify"
	PluginRegistryRemove PluginCommand = "plugin.registry.remove"
	PluginConfigReset    PluginCommand = "plugin.config.reset"
)

type PluginCommandMsg struct {
	Command  PluginCommand
	TargetID string
}

type pluginOverlay uint8

const (
	pluginOverlayNone pluginOverlay = iota
	pluginOverlayConfirm
	pluginOverlayOperation
	pluginOverlayHostInstall
)

type pluginHostInstallOption struct {
	portable bool
	hint     pluginpkg.HostInstallHint
	label    string
}

type pluginLoadMsg struct {
	section     string
	installed   []application.InstalledPluginInfo
	marketplace []application.MarketplacePluginInfo
	updates     []pluginpkg.OutdatedPlugin
	registries  []application.PluginRegistryInfo
	detail      *application.PluginDetail
	err         error
}

type pluginOperationMsg struct {
	command PluginCommand
	target  string
	notice  string
	err     error
}

type pluginRegistryFormData struct {
	Name        string
	URL         string
	Issuer      string
	Repository  string
	Unqualified bool
}

type PluginPage struct {
	ctx              context.Context
	service          *application.PluginService
	resourceID       string
	section          string
	action           string
	browser          component.Browser
	detail           component.DetailPage
	installed        map[pluginpkg.PluginID]application.InstalledPluginInfo
	marketplace      map[string]application.MarketplacePluginInfo
	updates          map[pluginpkg.PluginID]pluginpkg.OutdatedPlugin
	registries       map[string]application.PluginRegistryInfo
	detailValue      application.PluginDetail
	loaded           bool
	loading          bool
	overlay          pluginOverlay
	confirm          component.ConfirmButtons
	command          PluginCommand
	targetID         string
	progress         *component.Progress
	operationCancel  context.CancelFunc
	hostPrerequisite *pluginpkg.HostPrerequisiteError
	hostOptions      []pluginHostInstallOption
	hostIndex        int
	editor           *component.Editor
	registryForm     *pluginRegistryFormData
	configForm       *pluginConfigFormData
	notice           string
	err              error
	width            int
	height           int
}

func NewPlugins(ctx context.Context, resourceID, section string) (*PluginPage, error) {
	return NewPluginsRouteAction(ctx, resourceID, section, "")
}

func NewPluginsRouteAction(ctx context.Context, resourceID, section, action string) (*PluginPage, error) {
	service, err := application.NewPluginService()
	if err != nil {
		return nil, err
	}
	return newPluginsRouteAction(ctx, resourceID, section, action, service)
}

func newPluginsRouteAction(ctx context.Context, resourceID, section, action string, service *application.PluginService) (*PluginPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if service == nil {
		return nil, fmt.Errorf("plugin service is required")
	}
	page := &PluginPage{
		ctx: ctx, service: service, resourceID: strings.TrimSpace(resourceID), section: strings.TrimSpace(section), action: strings.TrimSpace(action),
		installed: map[pluginpkg.PluginID]application.InstalledPluginInfo{}, marketplace: map[string]application.MarketplacePluginInfo{}, updates: map[pluginpkg.PluginID]pluginpkg.OutdatedPlugin{}, registries: map[string]application.PluginRegistryInfo{},
	}
	page.browser = component.NewBrowser(ctx, page.browserTitle(), nil, nil).WithTitleVisible(false).WithHelpBindings(page.sectionBindings()...)
	if page.action != "" {
		if err := page.initEditorRoute(); err != nil {
			return nil, err
		}
	}
	return page, nil
}

func (page *PluginPage) Init() tea.Cmd {
	if page == nil {
		return nil
	}
	if page.editor != nil {
		return page.editor.Init()
	}
	page.loading = true
	return page.loadCmd()
}

func (page *PluginPage) Close() {
	if page != nil && page.operationCancel != nil {
		page.operationCancel()
		page.operationCancel = nil
	}
}

func (page *PluginPage) OverlayActive() bool { return page != nil && page.overlay != pluginOverlayNone }

func (page *PluginPage) InputActive() bool {
	return page != nil && (page.editor != nil || page.resourceID == "" && page.browser.InputActive())
}

func (page *PluginPage) Dirty() bool { return page != nil && page.editor != nil && page.editor.Dirty() }

func (page *PluginPage) Submitting() bool {
	return page != nil && page.editor != nil && page.editor.Submitting()
}

func (page *PluginPage) Notice() string {
	if page == nil {
		return ""
	}
	return page.notice
}

func (page *PluginPage) SetNotice(value string) {
	if page != nil {
		page.notice = strings.TrimSpace(value)
	}
}

func (page *PluginPage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	switch msg := message.(type) {
	case pluginLoadMsg:
		return page, page.finishLoad(msg)
	case pluginOperationMsg:
		return page, page.finishOperation(msg)
	case component.EditorSubmitMsg:
		if page.editor != nil {
			return page, page.submitEditor()
		}
	case component.EditorCancelMsg:
		if page.editor != nil {
			return page, page.editorParentNavigation()
		}
	case component.ConfirmChoiceMsg:
		if page.overlay == pluginOverlayConfirm {
			page.confirm.Select(msg.Affirmative)
			return page, page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
	case PluginCommandMsg:
		cmd, err := page.openCommand(msg.Command, msg.TargetID)
		if err != nil {
			page.err = err
		}
		return page, cmd
	case component.BrowserOpenMsg:
		if page.resourceID == "" && msg.Row.ID != "" {
			return page, page.openRow(msg.Row.ID)
		}
	}

	if page.overlay == pluginOverlayOperation {
		if key, ok := message.(tea.KeyPressMsg); ok && key.String() == "esc" {
			if page.operationCancel != nil {
				page.operationCancel()
			}
			page.notice = "Plugin operation cancellation requested"
			return page, nil
		}
		if page.progress != nil {
			updated, cmd := page.progress.Update(message)
			page.progress = &updated
			return page, cmd
		}
		return page, nil
	}
	if page.overlay == pluginOverlayHostInstall {
		if key, ok := message.(tea.KeyPressMsg); ok {
			return page, page.updateHostInstall(key)
		}
		return page, nil
	}
	if page.overlay == pluginOverlayConfirm {
		if key, ok := message.(tea.KeyPressMsg); ok {
			return page, page.updateConfirm(key)
		}
		return page, nil
	}
	if page.editor != nil {
		updated, cmd := page.editor.Update(message)
		page.editor = &updated
		return page, cmd
	}
	if key, ok := message.(tea.KeyPressMsg); ok {
		if page.resourceID == "" && page.browser.InputActive() {
			updated, cmd := page.browser.Update(key)
			page.browser = updated.(component.Browser)
			return page, cmd
		}
		if cmd, handled := page.handleKey(key); handled {
			return page, cmd
		}
	}
	if page.resourceID != "" {
		updated, cmd := page.detail.Update(message)
		page.detail = updated
		return page, cmd
	}
	updated, cmd := page.browser.Update(message)
	page.browser = updated.(component.Browser)
	return page, cmd
}

func (page *PluginPage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "Plugin page unavailable", "")
	}
	page.width, page.height = width, height
	if page.editor == nil && !page.loaded && page.loading {
		return component.StateView(component.PageLoading, "Loading plugins", "")
	}
	var content string
	if page.editor != nil {
		page.editor.Resize(width, height)
		page.editor.SetFeedback(page.notice, page.err)
		content = page.editor.View()
	} else if page.resourceID != "" {
		page.detail.SetFeedback(page.notice, page.err)
		page.detail.Resize(width, height)
		content = page.detail.View()
	} else {
		page.browser.SetTitleNotice(page.notice)
		feedback := ""
		if page.err != nil {
			feedback = component.BannerWidth(page.err.Error(), component.ToneDanger, width)
		}
		browserHeight := max(1, height-pageFeedbackHeight(feedback))
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: width, Height: browserHeight})
		page.browser = updated.(component.Browser)
		content = prependPageFeedback(feedback, page.browser.Content())
	}
	switch page.overlay {
	case pluginOverlayConfirm:
		modalWidth := overlayWidth(width, 72)
		body := confirmOverlayBody(page.confirm, page.confirmTitle(), page.confirmDescription(), modalWidth)
		content = component.CenterOverlay(content, component.Modal(body, modalWidth), width, height)
	case pluginOverlayOperation:
		modalWidth := overlayWidth(width, 76)
		body := component.Muted("Plugin operation in progress")
		if page.progress != nil {
			body = page.progress.View()
		}
		body += "\n\n" + component.Muted("Esc cancel")
		content = component.CenterOverlay(content, component.Modal(component.WrapModalBody(body, modalWidth), modalWidth), width, height)
	case pluginOverlayHostInstall:
		modalWidth := overlayWidth(width, 84)
		content = component.CenterOverlay(content, component.Modal(component.WrapModalBody(page.hostInstallBody(), modalWidth), modalWidth), width, height)
	}
	return content
}

func (page *PluginPage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	if page.editor != nil {
		return page.editor.MouseTargets(originX, originY, z)
	}
	switch page.overlay {
	case pluginOverlayConfirm:
		return confirmOverlayMouseTargets(page.confirm, page.confirmTitle(), page.confirmDescription(), overlayWidth(page.width, 72), page.width, page.height, originX, originY, z+20)
	case pluginOverlayOperation, pluginOverlayHostInstall:
		return []component.MouseTarget{mouseBlocker(originX, originY, page.width, page.height, z+20)}
	default:
		if page.resourceID != "" {
			return page.detail.MouseTargets(originX, originY, z)
		}
		feedback := ""
		if page.err != nil {
			feedback = component.BannerWidth(page.err.Error(), component.ToneDanger, page.width)
		}
		return page.browser.MouseTargets(originX, originY+pageFeedbackHeight(feedback), z)
	}
}

func (page *PluginPage) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "1", "alt+1":
		return navigatePluginSection(""), true
	case "2", "alt+2":
		return navigatePluginSection("marketplace"), true
	case "3", "alt+3":
		return navigatePluginSection("updates"), true
	case "4", "alt+4":
		return navigatePluginSection("registries"), true
	case "r":
		page.loading = true
		return page.loadCmd(), true
	case "a":
		if page.resourceID == "" && page.section == "registries" {
			return func() tea.Msg { return NavigateMsg{Path: []string{"plugins", "registries", "add"}} }, true
		}
	}
	return nil, false
}

func navigatePluginSection(section string) tea.Cmd {
	path := []string{"plugins"}
	if section != "" {
		path = append(path, section)
	}
	return func() tea.Msg { return NavigateMsg{Path: path} }
}

func (page *PluginPage) openRow(id string) tea.Cmd {
	path := []string{"plugins", id}
	if page.section != "" {
		path = []string{"plugins", page.section, id}
	}
	return func() tea.Msg { return NavigateMsg{Path: path} }
}

func (page *PluginPage) loadCmd() tea.Cmd {
	section, resourceID := page.section, page.resourceID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(page.ctx, 30*time.Second)
		defer cancel()
		msg := pluginLoadMsg{section: section}
		if resourceID != "" {
			switch section {
			case "marketplace":
				detail, err := page.service.MarketplaceDetail(ctx, resourceID)
				msg.detail, msg.err = &detail, err
			case "registries":
				msg.registries, msg.err = page.service.Registries()
			case "updates", "":
				_, id, _, err := pluginpkg.ParseReference(resourceID)
				if err == nil {
					detail, detailErr := page.service.InstalledDetail(id)
					msg.detail, msg.err = &detail, detailErr
				} else {
					msg.err = err
				}
			default:
				msg.err = fmt.Errorf("unsupported plugin section: %s", section)
			}
			return msg
		}
		switch section {
		case "":
			msg.installed, msg.err = page.service.Installed()
		case "marketplace":
			msg.marketplace, msg.err = page.service.Marketplace(ctx)
		case "updates":
			msg.updates, msg.err = page.service.Outdated(ctx)
		case "registries":
			msg.registries, msg.err = page.service.Registries()
		default:
			msg.err = fmt.Errorf("unsupported plugin section: %s", section)
		}
		return msg
	}
}

func (page *PluginPage) finishLoad(msg pluginLoadMsg) tea.Cmd {
	page.loading = false
	if msg.err != nil {
		page.err = msg.err
		page.loaded = false
		if page.resourceID != "" {
			page.detail = component.NewDetailPage("Unavailable", page.resourceID, component.Muted(msg.err.Error())).WithTitleVisible(false)
		}
		return nil
	}
	page.loaded, page.err = true, nil
	if msg.detail != nil {
		page.detailValue = *msg.detail
		page.syncPluginDetail()
		return nil
	}
	if msg.registries != nil {
		page.registries = map[string]application.PluginRegistryInfo{}
		for _, item := range msg.registries {
			page.registries[item.Registry.Name] = item
		}
		if page.resourceID != "" {
			page.syncRegistryDetail()
			return nil
		}
	}
	page.installed = map[pluginpkg.PluginID]application.InstalledPluginInfo{}
	for _, item := range msg.installed {
		page.installed[item.ID] = item
	}
	page.marketplace = map[string]application.MarketplacePluginInfo{}
	for _, item := range msg.marketplace {
		page.marketplace[item.Reference] = item
	}
	page.updates = map[pluginpkg.PluginID]pluginpkg.OutdatedPlugin{}
	for _, item := range msg.updates {
		page.updates[item.ID] = item
	}
	return page.syncBrowser()
}

func (page *PluginPage) syncBrowser() tea.Cmd {
	selectedID := ""
	if selected, ok := page.browser.Selected(); ok {
		selectedID = selected.ID
	}
	helpExpanded := page.browser.HelpExpanded()
	rows := page.rows()
	browser := component.NewBrowser(page.ctx, page.browserTitle(), rows, nil).WithTitleVisible(false).WithHelpBindings(page.sectionBindings()...)
	for _, action := range page.rowActions() {
		browser = browser.WithAction(action)
	}
	browser.SetHelpExpanded(helpExpanded)
	page.browser = browser
	if selectedID != "" {
		page.browser.SelectID(selectedID)
	}
	if page.width > 0 && page.height > 0 {
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: page.height})
		page.browser = updated.(component.Browser)
	}
	return nil
}

func (page *PluginPage) rows() []component.Row {
	switch page.section {
	case "marketplace":
		keys := sortedStringKeys(page.marketplace)
		rows := make([]component.Row, 0, len(keys))
		for _, key := range keys {
			item := page.marketplace[key]
			state := "available"
			if item.Installed != nil {
				state = "installed " + string(item.Installed.Version)
			}
			trust := "untrusted"
			if item.Publisher.Trusted {
				trust = "trusted"
			}
			rows = append(rows, component.Row{ID: item.Reference, Title: item.Entry.Name, Description: item.Reference + " · " + item.Entry.Description, Meta: string(item.Entry.Stable) + " · " + string(item.Entry.Type) + " · " + trust + " · " + state, Search: strings.Join([]string{item.Reference, item.Entry.Name, item.Entry.Description, item.Publisher.Name, string(item.Entry.Type)}, " ")})
		}
		return rows
	case "updates":
		ids := sortedPluginIDsFromUpdates(page.updates)
		rows := make([]component.Row, 0, len(ids))
		for _, id := range ids {
			item := page.updates[id]
			rows = append(rows, component.Row{ID: string(id), Title: string(id), Description: string(item.Current) + " -> " + string(item.Latest), Meta: "update available", Search: string(id)})
		}
		return rows
	case "registries":
		keys := sortedStringKeys(page.registries)
		rows := make([]component.Row, 0, len(keys))
		for _, key := range keys {
			item := page.registries[key]
			state := "custom"
			if item.Official {
				state = "official"
			}
			resolution := "qualified only"
			if item.Registry.UnqualifiedResolution {
				resolution = "unqualified enabled"
			}
			rows = append(rows, component.Row{ID: key, Title: key, Description: item.Registry.URL, Meta: state + " · " + resolution, Search: key + " " + item.Registry.URL})
		}
		return rows
	default:
		ids := sortedPluginIDsFromInstalled(page.installed)
		rows := make([]component.Row, 0, len(ids))
		for _, id := range ids {
			item := page.installed[id]
			state := "disabled"
			if item.Lock.Enabled {
				state = "enabled"
			}
			origin := item.Lock.Registry + "/" + item.Lock.Publisher
			if item.Origin == pluginpkg.OriginBuiltin {
				origin = item.Origin.Label()
			}
			rows = append(rows, component.Row{ID: string(id), Title: item.Installed.Manifest.Name, Description: string(id) + " · " + origin, Meta: string(item.Lock.Version) + " · " + string(item.Installed.Manifest.Type) + " · " + state, Search: strings.Join([]string{string(id), item.Installed.Manifest.Name, string(item.Lock.Version), origin}, " ")})
		}
		return rows
	}
}

func (page *PluginPage) rowActions() []component.RowAction {
	command := func(key, desc string, value PluginCommand, when func(component.Row) bool) component.RowAction {
		return component.RowAction{Key: key, Desc: desc, When: when, Run: func(row component.Row) (string, tea.Cmd, error) {
			return "", func() tea.Msg { return PluginCommandMsg{Command: value, TargetID: row.ID} }, nil
		}}
	}
	switch page.section {
	case "marketplace":
		return []component.RowAction{
			command("i", "install", PluginInstall, func(row component.Row) bool { return page.marketplace[row.ID].Installed == nil }),
			command("u", "update", PluginUpdate, func(row component.Row) bool { return page.marketplace[row.ID].Installed != nil }),
		}
	case "updates":
		return []component.RowAction{command("u", "update", PluginUpdate, nil)}
	case "registries":
		return []component.RowAction{command("d", "remove", PluginRegistryRemove, func(row component.Row) bool { return row.ID != pluginpkg.OfficialRegistryName })}
	default:
		return []component.RowAction{
			command("space", "toggle", PluginEnable, func(row component.Row) bool {
				return page.pluginLifecycle(row.ID).Enable || page.pluginLifecycle(row.ID).Disable
			}),
			{
				Key: "e", Desc: "configure",
				When: func(row component.Row) bool { return page.pluginLifecycle(row.ID).Configure },
				Run: func(row component.Row) (string, tea.Cmd, error) {
					id := row.ID
					return "", func() tea.Msg { return NavigateMsg{Path: []string{"plugins", id, "configure"}} }, nil
				},
			},
			command("x", "reset config", PluginConfigReset, func(row component.Row) bool { return page.pluginLifecycle(row.ID).Configure }),
			command("u", "update", PluginUpdate, func(row component.Row) bool { return page.pluginLifecycle(row.ID).Update }),
			command("b", "rollback", PluginRollback, func(row component.Row) bool { return page.pluginLifecycle(row.ID).Rollback }),
			command("p", "prune", PluginPrune, func(row component.Row) bool { return page.pluginLifecycle(row.ID).Prune }),
			command("v", "verify", PluginVerify, func(row component.Row) bool { return page.pluginLifecycle(row.ID).Verify }),
			command("d", "uninstall", PluginUninstall, func(row component.Row) bool { return page.pluginLifecycle(row.ID).Uninstall }),
			command("D", "force uninstall", PluginForceUninstall, func(row component.Row) bool { return page.pluginLifecycle(row.ID).Uninstall }),
		}
	}
}

func (page *PluginPage) sectionBindings() []key.Binding {
	bindings := []key.Binding{
		component.Binding([]string{"1", "alt+1"}, "1", "installed"), component.Binding([]string{"2", "alt+2"}, "2", "marketplace"), component.Binding([]string{"3", "alt+3"}, "3", "updates"), component.Binding([]string{"4", "alt+4"}, "4", "registries"), component.Binding([]string{"r"}, "r", "refresh"),
	}
	if page.section == "registries" && page.resourceID == "" {
		bindings = append(bindings, component.Binding([]string{"a"}, "a", "add"))
	}
	return bindings
}

func (page *PluginPage) browserTitle() string {
	switch page.section {
	case "marketplace":
		return "Plugin Marketplace"
	case "updates":
		return "Plugin Updates"
	case "registries":
		return "Plugin Registries"
	default:
		return "Installed Plugins"
	}
}

func (page *PluginPage) pluginLifecycle(id string) pluginpkg.Lifecycle {
	if item, ok := page.installed[pluginpkg.PluginID(id)]; ok {
		if item.Origin == pluginpkg.OriginBuiltin {
			return item.Lifecycle
		}
		if item.Lifecycle != (pluginpkg.Lifecycle{}) {
			return item.Lifecycle
		}
	}
	return pluginpkg.ArtifactLifecycle()
}

func (page *PluginPage) detailLifecycle(detail application.PluginDetail) pluginpkg.Lifecycle {
	if detail.Origin == pluginpkg.OriginBuiltin {
		return detail.Lifecycle
	}
	if detail.Lifecycle != (pluginpkg.Lifecycle{}) {
		return detail.Lifecycle
	}
	return pluginpkg.ArtifactLifecycle()
}

func lifecycleAllows(life pluginpkg.Lifecycle, command PluginCommand) bool {
	switch command {
	case PluginInstall:
		return life.Install
	case PluginUpdate:
		return life.Update
	case PluginRollback:
		return life.Rollback
	case PluginPrune:
		return life.Prune
	case PluginUninstall, PluginForceUninstall:
		return life.Uninstall
	case PluginEnable:
		return life.Enable
	case PluginDisable:
		return life.Disable
	case PluginVerify:
		return life.Verify
	case PluginConfigReset:
		return life.Configure
	default:
		return true
	}
}

func (page *PluginPage) openCommand(command PluginCommand, target string) (tea.Cmd, error) {
	page.err, page.notice = nil, ""
	page.command, page.targetID = command, strings.TrimSpace(target)
	if command == PluginRefresh {
		page.loading = true
		return page.loadCmd(), nil
	}
	if command == PluginConfigReset {
		id, err := pluginID(page.targetID)
		if err != nil {
			return nil, err
		}
		if _, _, err := page.service.PluginSettings(id); err != nil {
			return nil, err
		}
		page.confirm = component.NewConfirmButtons(page.confirmAffirmative(), "Cancel", false)
		page.overlay = pluginOverlayConfirm
		return nil, nil
	}
	if command != PluginRegistryRemove && !lifecycleAllows(page.pluginLifecycle(page.targetID), command) {
		return nil, fmt.Errorf("%w: %s", pluginpkg.ErrBuiltinPlugin, page.targetID)
	}
	if command == PluginVerify {
		return page.startOperation(command, page.targetID), nil
	}
	if command == PluginEnable || command == PluginDisable {
		id, err := pluginID(page.targetID)
		if err != nil {
			return nil, err
		}
		if item, ok := page.installed[id]; ok {
			if item.Lock.Enabled {
				page.command = PluginDisable
			} else {
				page.command = PluginEnable
			}
		}
	}
	page.confirm = component.NewConfirmButtons(page.confirmAffirmative(), "Cancel", false)
	page.overlay = pluginOverlayConfirm
	return nil, nil
}

func (page *PluginPage) updateConfirm(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "esc" {
		page.closeOverlay()
		return nil
	}
	if msg.String() != "enter" {
		return page.confirm.Update(msg)
	}
	if !page.confirm.AffirmativeSelected() {
		page.closeOverlay()
		return nil
	}
	command, target := page.command, page.targetID
	page.closeOverlay()
	return page.startOperation(command, target)
}

func (page *PluginPage) startOperation(command PluginCommand, target string) tea.Cmd {
	ctx, cancel := context.WithTimeout(page.ctx, pluginOperationTimeout)
	page.operationCancel = cancel
	page.command, page.targetID = command, target
	progress := component.NewProgress(page.operationTitle(command))
	page.progress = &progress
	page.overlay = pluginOverlayOperation
	return func() tea.Msg {
		msg := pluginOperationMsg{command: command, target: target}
		switch command {
		case PluginInstall:
			_, msg.err = page.service.Install(ctx, target)
			msg.notice = "Plugin installed"
		case PluginUpdate:
			id, err := pluginID(target)
			if err != nil {
				msg.err = err
			} else {
				_, msg.err = page.service.Update(ctx, id)
			}
			msg.notice = "Plugin updated"
		case PluginRollback:
			id, err := pluginID(target)
			if err != nil {
				msg.err = err
			} else {
				_, msg.err = page.service.Rollback(ctx, id)
			}
			msg.notice = "Plugin rolled back"
		case PluginPrune:
			id, err := pluginID(target)
			if err != nil {
				msg.err = err
			} else {
				_, msg.err = page.service.Prune(id)
			}
			msg.notice = "Plugin versions pruned"
		case PluginUninstall, PluginForceUninstall:
			id, err := pluginID(target)
			if err != nil {
				msg.err = err
			} else {
				msg.err = page.service.Uninstall(ctx, id, command == PluginForceUninstall)
			}
			msg.notice = "Plugin uninstalled"
		case PluginEnable, PluginDisable:
			id, err := pluginID(target)
			if err != nil {
				msg.err = err
			} else {
				msg.err = page.service.SetEnabled(ctx, id, command == PluginEnable)
			}
			msg.notice = "Plugin disabled"
			if command == PluginEnable {
				msg.notice = "Plugin enabled"
			}
		case PluginVerify:
			id, err := pluginID(target)
			if err != nil {
				msg.err = err
			} else {
				msg.err = page.service.Verify(ctx, id)
			}
			msg.notice = "Plugin verified"
		case PluginRegistryRemove:
			msg.err = page.service.RemoveRegistry(ctx, target)
			msg.notice = "Plugin registry removed"
		case PluginConfigReset:
			id, err := pluginID(target)
			if err != nil {
				msg.err = err
			} else {
				msg.err = page.service.ResetPluginSetting(ctx, id, "")
			}
			msg.notice = "Plugin configuration reset"
		default:
			msg.err = fmt.Errorf("unsupported plugin operation: %s", command)
		}
		return msg
	}
}

func (page *PluginPage) finishOperation(msg pluginOperationMsg) tea.Cmd {
	if page.operationCancel != nil {
		page.operationCancel()
	}
	page.operationCancel = nil
	page.overlay, page.progress = pluginOverlayNone, nil
	if msg.command == PluginInstall && msg.err != nil {
		var prerequisite *pluginpkg.HostPrerequisiteError
		if errors.As(msg.err, &prerequisite) && page.openHostInstall(prerequisite) {
			return nil
		}
	}
	page.err = msg.err
	if msg.err != nil {
		return nil
	}
	page.notice = msg.notice
	if (msg.command == PluginUninstall || msg.command == PluginForceUninstall) && page.resourceID != "" && page.section == "" {
		return func() tea.Msg { return NavigateMsg{Path: []string{"plugins"}, Replace: true} }
	}
	if msg.command == PluginRegistryRemove && page.resourceID != "" {
		return func() tea.Msg { return NavigateMsg{Path: []string{"plugins", "registries"}, Replace: true} }
	}
	page.loading = true
	return page.loadCmd()
}

func (page *PluginPage) openHostInstall(prerequisite *pluginpkg.HostPrerequisiteError) bool {
	if prerequisite == nil {
		return false
	}
	options := make([]pluginHostInstallOption, 0, len(prerequisite.Install)+1)
	if prerequisite.Portable != nil {
		options = append(options, pluginHostInstallOption{portable: true, label: "Install verified portable binary locally in plugin data"})
	}
	for _, hint := range prerequisite.Install {
		if hint.Runnable() {
			options = append(options, pluginHostInstallOption{hint: hint, label: "Install globally: " + hint.DisplayCommand()})
		}
	}
	if len(options) == 0 {
		return false
	}
	page.hostPrerequisite, page.hostOptions, page.hostIndex = prerequisite, options, 0
	page.err = nil
	page.overlay = pluginOverlayHostInstall
	return true
}

func (page *PluginPage) updateHostInstall(msg tea.KeyPressMsg) tea.Cmd {
	if len(page.hostOptions) == 0 {
		page.closeHostInstall()
		return nil
	}
	switch msg.String() {
	case "esc":
		page.closeHostInstall()
		return nil
	case "up", "k":
		page.hostIndex = (page.hostIndex - 1 + len(page.hostOptions)) % len(page.hostOptions)
		return nil
	case "down", "j":
		page.hostIndex = (page.hostIndex + 1) % len(page.hostOptions)
		return nil
	case "enter":
		choice := page.hostOptions[page.hostIndex]
		page.closeHostInstall()
		return page.startHostInstall(choice)
	default:
		return nil
	}
}

func (page *PluginPage) startHostInstall(choice pluginHostInstallOption) tea.Cmd {
	ctx, cancel := context.WithTimeout(page.ctx, pluginHostInstallTimeout)
	page.operationCancel = cancel
	progress := component.NewProgress("Installing host dependency")
	page.progress = &progress
	page.overlay = pluginOverlayOperation
	target := page.targetID
	return func() tea.Msg {
		msg := pluginOperationMsg{command: PluginInstall, target: target, notice: "Plugin installed"}
		if choice.portable {
			_, msg.err = page.service.InstallPortable(ctx, target)
			return msg
		}
		if _, msg.err = page.service.RunHostInstallHint(ctx, choice.hint); msg.err != nil {
			return msg
		}
		_, msg.err = page.service.Install(ctx, target)
		return msg
	}
}

func (page *PluginPage) hostInstallBody() string {
	if page.hostPrerequisite == nil || len(page.hostOptions) == 0 {
		return component.Muted("Host dependency options are unavailable")
	}
	var builder strings.Builder
	builder.WriteString(component.ToneText("Host dependency required", component.ToneWarning))
	builder.WriteString("\n\n")
	builder.WriteString(strings.TrimSpace(page.hostPrerequisite.Reason))
	builder.WriteString("\n\nChoose how to install the required host dependency:\n")
	for index, option := range page.hostOptions {
		line := "  " + option.label
		if index == page.hostIndex {
			line = component.ToneText("> "+option.label, component.ToneAccent)
		}
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	manual := make([]string, 0, len(page.hostPrerequisite.Install))
	for _, hint := range page.hostPrerequisite.Install {
		if !hint.Runnable() {
			manual = append(manual, hint.DisplayCommand())
		}
	}
	if len(manual) > 0 {
		builder.WriteString("\n")
		builder.WriteString(component.Muted("Manual install commands:"))
		builder.WriteByte('\n')
		for _, command := range manual {
			builder.WriteString(component.Muted("- " + command))
			builder.WriteByte('\n')
		}
	}
	builder.WriteString("\n")
	builder.WriteString(component.Muted("↑/↓ select  Enter install  Esc cancel"))
	return strings.TrimSpace(builder.String())
}

func (page *PluginPage) closeHostInstall() {
	page.overlay = pluginOverlayNone
	page.hostPrerequisite = nil
	page.hostOptions = nil
	page.hostIndex = 0
}

func (page *PluginPage) operationTitle(command PluginCommand) string {
	switch command {
	case PluginInstall:
		return "Installing plugin"
	case PluginUpdate:
		return "Updating plugin"
	case PluginRollback:
		return "Rolling back plugin"
	case PluginPrune:
		return "Pruning plugin versions"
	case PluginUninstall:
		return "Uninstalling plugin"
	case PluginForceUninstall:
		return "Force uninstalling plugin"
	case PluginEnable:
		return "Enabling plugin"
	case PluginDisable:
		return "Disabling plugin"
	case PluginVerify:
		return "Verifying plugin"
	case PluginRegistryRemove:
		return "Removing plugin registry"
	case PluginConfigReset:
		return "Resetting plugin configuration"
	default:
		return "Plugin operation"
	}
}

func (page *PluginPage) confirmAffirmative() string {
	switch page.command {
	case PluginInstall:
		return "Install"
	case PluginUpdate:
		return "Update"
	case PluginRollback:
		return "Rollback"
	case PluginPrune:
		return "Prune"
	case PluginUninstall:
		return "Uninstall"
	case PluginForceUninstall:
		return "Force uninstall"
	case PluginEnable:
		return "Enable"
	case PluginDisable:
		return "Disable"
	case PluginRegistryRemove:
		return "Remove"
	case PluginConfigReset:
		return "Reset"
	default:
		return "Confirm"
	}
}

func (page *PluginPage) confirmTitle() string {
	switch page.command {
	case PluginRegistryRemove:
		return "Remove plugin registry?"
	case PluginConfigReset:
		return "Reset plugin configuration?"
	default:
		return page.confirmAffirmative() + " plugin?"
	}
}

func (page *PluginPage) confirmDescription() string {
	switch page.command {
	case PluginInstall:
		return "Install and enable the signed plugin " + page.targetID + ". Manifest and artifact signatures are verified before activation."
	case PluginUpdate:
		return "Install the latest stable signed version and atomically switch the active plugin: " + page.targetID
	case PluginRollback:
		return "Verify and activate the newest retained version older than the current plugin: " + page.targetID
	case PluginPrune:
		return "Keep the active version and the default rollback retention, removing older inactive versions for plugin " + page.targetID + "."
	case PluginUninstall:
		return "Disable and remove the active plugin version: " + page.targetID + ". Active dependents will prevent removal."
	case PluginForceUninstall:
		return "Force removal of plugin " + page.targetID + ". Active dependents will be disabled before the provider is removed."
	case PluginEnable:
		return "Enable plugin " + page.targetID + " after revalidating integrity, compatibility, and dependencies."
	case PluginDisable:
		return "Disable plugin " + page.targetID + ". Its capabilities will stop resolving immediately."
	case PluginRegistryRemove:
		return "Remove plugin registry " + page.targetID + " from local configuration. Installed payloads are not deleted."
	case PluginConfigReset:
		return "Reset all configuration for plugin " + page.targetID + " to schema defaults."
	default:
		return page.targetID
	}
}

func (page *PluginPage) closeOverlay() {
	page.overlay = pluginOverlayNone
	page.confirm = component.ConfirmButtons{}
}

func (page *PluginPage) syncPluginDetail() {
	detail := page.detailValue
	manifest := detail.Manifest
	state := "not installed"
	if detail.Installed {
		state = "disabled"
		if detail.Enabled {
			state = "enabled"
		}
	}
	trust := "untrusted"
	if detail.Publisher.Trusted {
		trust = "trusted"
	}
	content := detailFields(
		[2]string{"ID", string(manifest.ID)}, [2]string{"Name", manifest.Name}, [2]string{"Version", string(manifest.Version)}, [2]string{"Type", string(manifest.Type)}, [2]string{"Origin", detail.Origin.Label()}, [2]string{"State", state},
		[2]string{"Registry", detail.Registry.Name}, [2]string{"Publisher", detail.Publisher.Name}, [2]string{"Publisher trust", trust}, [2]string{"Signature", detail.SignatureStatus}, [2]string{"Source", detail.Publisher.Source}, [2]string{"Signing repository", detail.Publisher.Sigstore.Repository},
		[2]string{"Capabilities", pluginCapabilities(manifest.Provides)}, [2]string{"Permissions", pluginPermissions(manifest.Permissions)}, [2]string{"Dependencies", pluginCapabilities(manifest.Dependencies.Capabilities)}, [2]string{"Core requirement", manifest.Requires.ChatGPTMCP}, [2]string{"Core compatibility", detail.CoreCompatibility}, [2]string{"Platforms", pluginPlatforms(manifest.Platforms)},
	)
	page.detail = component.NewDetailPage(manifest.Name, string(manifest.Version)+" · "+string(manifest.Type)+" · "+state, content).WithTitleVisible(false)
	bindings := []component.DetailPageBinding{{Key: "r", Desc: "refresh", Message: PluginCommandMsg{Command: PluginRefresh}}}
	life := page.detailLifecycle(detail)
	if detail.Installed {
		if life.Enable || life.Disable {
			toggle := PluginEnable
			if detail.Enabled {
				toggle = PluginDisable
			}
			bindings = append(bindings, component.DetailPageBinding{Key: "space", HelpKey: "space", Desc: "toggle", Message: PluginCommandMsg{Command: toggle, TargetID: string(manifest.ID)}})
		}
		if life.Configure {
			bindings = append(bindings, component.DetailPageBinding{Key: "e", Desc: "configure", Message: NavigateMsg{Path: []string{"plugins", string(manifest.ID), "configure"}}})
			bindings = append(bindings, component.DetailPageBinding{Key: "x", Desc: "reset config", Message: PluginCommandMsg{Command: PluginConfigReset, TargetID: string(manifest.ID)}})
		}
		if life.Update {
			bindings = append(bindings, component.DetailPageBinding{Key: "u", Desc: "update", Message: PluginCommandMsg{Command: PluginUpdate, TargetID: string(manifest.ID)}})
		}
		if life.Rollback {
			bindings = append(bindings, component.DetailPageBinding{Key: "b", Desc: "rollback", Message: PluginCommandMsg{Command: PluginRollback, TargetID: string(manifest.ID)}})
		}
		if life.Prune {
			bindings = append(bindings, component.DetailPageBinding{Key: "p", Desc: "prune", Message: PluginCommandMsg{Command: PluginPrune, TargetID: string(manifest.ID)}})
		}
		if life.Verify {
			bindings = append(bindings, component.DetailPageBinding{Key: "v", Desc: "verify", Message: PluginCommandMsg{Command: PluginVerify, TargetID: string(manifest.ID)}})
		}
		if life.Uninstall {
			bindings = append(bindings, component.DetailPageBinding{Key: "d", Desc: "uninstall", Message: PluginCommandMsg{Command: PluginUninstall, TargetID: string(manifest.ID)}})
			bindings = append(bindings, component.DetailPageBinding{Key: "D", Desc: "force uninstall", Message: PluginCommandMsg{Command: PluginForceUninstall, TargetID: string(manifest.ID)}})
		}
	} else if life.Install {
		bindings = append(bindings, component.DetailPageBinding{Key: "i", Desc: "install", Message: PluginCommandMsg{Command: PluginInstall, TargetID: detail.Reference}})
	}
	page.detail.SetBindings(bindings...)
}

func (page *PluginPage) syncRegistryDetail() {
	item, ok := page.registries[page.resourceID]
	if !ok {
		page.err = fmt.Errorf("plugin registry not found: %s", page.resourceID)
		page.detail = component.NewDetailPage("Unavailable", page.resourceID, component.Muted(page.err.Error())).WithTitleVisible(false)
		return
	}
	trustIssuer, trustRepo := "", ""
	if item.Registry.Trust != nil {
		trustIssuer, trustRepo = item.Registry.Trust.Issuer, item.Registry.Trust.Repository
	}
	content := detailFields(
		[2]string{"Name", item.Registry.Name}, [2]string{"URL", item.Registry.URL}, [2]string{"Official", fmt.Sprint(item.Official)}, [2]string{"Unqualified resolution", fmt.Sprint(item.Registry.UnqualifiedResolution)}, [2]string{"Sigstore issuer", trustIssuer}, [2]string{"Signing repository", trustRepo},
	)
	page.detail = component.NewDetailPage(item.Registry.Name, item.Registry.URL, content).WithTitleVisible(false)
	bindings := []component.DetailPageBinding{{Key: "r", Desc: "refresh", Message: PluginCommandMsg{Command: PluginRefresh}}}
	if !item.Official {
		bindings = append(bindings, component.DetailPageBinding{Key: "d", Desc: "remove", Message: PluginCommandMsg{Command: PluginRegistryRemove, TargetID: item.Registry.Name}})
	}
	page.detail.SetBindings(bindings...)
}

func (page *PluginPage) initEditorRoute() error {
	if page.action == "configure" && page.section == "" && page.resourceID != "" {
		return page.initPluginConfigEditor()
	}
	if page.section != "registries" || page.action != "add" || page.resourceID != "" {
		return fmt.Errorf("unsupported plugin editor route")
	}
	data := &pluginRegistryFormData{Issuer: pluginpkg.OfficialSigstoreIssuer}
	form := component.NewEditorForm(component.Group(
		component.Input("Registry name", &data.Name).Validate(requiredValue("registry name")),
		component.Input("Registry HTTPS URL", &data.URL).Validate(requiredValue("registry URL")),
		component.Input("Pinned Sigstore issuer", &data.Issuer).Validate(requiredValue("Sigstore issuer")),
		component.Input("Pinned signing repository", &data.Repository).Validate(requiredValue("signing repository")),
		component.Switch("Allow unqualified plugin resolution", &data.Unqualified, "Yes", "No"),
	))
	editor := component.NewEditor("add", component.EditorSection{ID: "registry", Title: "Plugin Registry", Description: "Add a signed registry with an explicitly pinned Sigstore issuer and signing repository.", Form: form})
	page.registryForm, page.editor = data, &editor
	return nil
}

func (page *PluginPage) submitEditor() tea.Cmd {
	if page.configForm != nil {
		return page.submitPluginConfigEditor()
	}
	if page.editor == nil || page.registryForm == nil {
		return nil
	}
	if err := page.editor.Validate(); err != nil {
		page.editor.SetFeedback("", err)
		return nil
	}
	data := page.registryForm
	err := page.service.AddRegistry(page.ctx, data.Name, data.URL, data.Unqualified, pluginpkg.SigstoreIdentity{Issuer: data.Issuer, Repository: data.Repository})
	if err != nil {
		page.editor.SetFeedback("", err)
		return nil
	}
	page.editor.Accept()
	return tea.Batch(page.editorParentNavigation(), func() tea.Msg {
		return ToastMsg{Title: "Plugins", Message: "Plugin registry added", Tone: component.ToneSuccess}
	})
}

func (page *PluginPage) editorParentNavigation() tea.Cmd {
	if page.action == "configure" && page.resourceID != "" {
		id := page.resourceID
		return func() tea.Msg { return NavigateMsg{Path: []string{"plugins", id}} }
	}
	return func() tea.Msg { return NavigateMsg{Path: []string{"plugins", "registries"}} }
}

func pluginID(value string) (pluginpkg.PluginID, error) {
	_, id, _, err := pluginpkg.ParseReference(strings.TrimSpace(value))
	return id, err
}

func pluginCapabilities(values []pluginpkg.Capability) string {
	if len(values) == 0 {
		return "none"
	}
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = string(value)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

func pluginPermissions(values []pluginpkg.Permission) string {
	if len(values) == 0 {
		return "none"
	}
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = string(value)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

func pluginPlatforms(values map[string]pluginpkg.PlatformArtifact) string {
	if len(values) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\n")
}

func sortedPluginIDsFromInstalled(values map[pluginpkg.PluginID]application.InstalledPluginInfo) []pluginpkg.PluginID {
	ids := make([]pluginpkg.PluginID, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedPluginIDsFromUpdates(values map[pluginpkg.PluginID]pluginpkg.OutdatedPlugin) []pluginpkg.PluginID {
	ids := make([]pluginpkg.PluginID, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedStringKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
