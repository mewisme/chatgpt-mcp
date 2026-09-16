package page

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func (page *TunnelPage) resizeEditor() {
	if page == nil || page.editor == nil || page.width <= 0 || page.height <= 0 {
		return
	}
	page.editor.Resize(page.width, page.height)
}

func (page *TunnelPage) editorView(width, height int) string {
	if page == nil || page.editor == nil {
		return component.StateView(component.PageError, "Tunnel editor unavailable", "")
	}
	page.width, page.height = width, height
	page.resizeEditor()
	return page.editor.View()
}

func (page *TunnelPage) editorParentNavigation() tea.Cmd {
	if page != nil && page.resourceID != "" {
		id := page.resourceID
		return func() tea.Msg { return NavigateMsg{Path: []string{"tunnels", id}} }
	}
	return func() tea.Msg { return NavigateMsg{Path: []string{"tunnels"}} }
}

func (page *TunnelPage) submitEditor() tea.Cmd {
	if page == nil || page.editor == nil {
		return nil
	}
	if err := page.editor.Validate(); err != nil {
		page.editor.SetFeedback("", err)
		return nil
	}
	page.editor.SetFeedback("", nil)
	return page.submitManagedEditor()
}

func (page *TunnelPage) initManagedEditorRoute() error {
	if page == nil {
		return fmt.Errorf("managed tunnel editor is unavailable")
	}
	switch page.action {
	case "create":
		if page.resourceID != "" || page.section != "" {
			return fmt.Errorf("managed tunnel create editor does not accept a resource or section")
		}
		editor, data := newManagedTunnelEditor(tunnel.Metadata{}, true, page.adminProfiles)
		page.editor, page.managedForm = &editor, data
		page.command = TunnelManagedCreate
	case "edit":
		if page.resourceID == "" || page.section != "" {
			return fmt.Errorf("managed tunnel edit editor requires a tunnel resource")
		}
		metadata, err := page.cachedManagedTunnel(page.resourceID)
		if err != nil {
			return err
		}
		editor, data := newManagedTunnelEditor(metadata, false, page.profilesForManaged(page.resourceID, true))
		page.editor, page.managedForm = &editor, data
		page.command, page.targetID = TunnelManagedUpdate, page.resourceID
	case "configure":
		if page.resourceID == "" || page.section != "" {
			return fmt.Errorf("managed tunnel attach editor requires a tunnel resource")
		}
		editor, data := newManagedConfigureEditor(page.profilesForManaged(page.resourceID, false))
		page.editor, page.configureForm = &editor, data
		page.command, page.targetID = TunnelManagedConfigure, page.resourceID
	case "delete":
		if page.resourceID == "" || page.section != "" {
			return fmt.Errorf("managed tunnel delete editor requires a tunnel resource")
		}
		editor, data := newManagedDeleteEditor(page.profilesForManaged(page.resourceID, true))
		page.editor, page.deleteForm = &editor, data
		page.command, page.targetID = TunnelManagedDelete, page.resourceID
	default:
		return fmt.Errorf("unsupported managed tunnel editor action: %s", page.action)
	}
	page.resizeEditor()
	return nil
}

func (page *TunnelPage) submitManagedEditor() tea.Cmd {
	switch page.command {
	case TunnelManagedCreate:
		if page.managedForm == nil {
			page.editor.SetFeedback("", fmt.Errorf("managed tunnel draft is unavailable"))
			return nil
		}
		request, profileID := managedCreateInput(page.managedForm), page.managedForm.AdminProfileID
		return page.startOperation(page.command, "", "Creating managed tunnel", func(ctx context.Context) tunnelOperationMsg {
			discovery, err := application.CreateManagedTunnelByProfile(ctx, profileID, request)
			return tunnelOperationMsg{command: TunnelManagedCreate, result: application.ManagedTunnelResult{Metadata: discovery.Metadata}, err: err}
		})
	case TunnelManagedUpdate:
		if page.managedForm == nil || page.targetID == "" {
			page.editor.SetFeedback("", fmt.Errorf("managed tunnel update draft is unavailable"))
			return nil
		}
		request, id, profileID := managedUpdateInput(page.managedForm), page.targetID, page.managedForm.AdminProfileID
		return page.startOperation(page.command, id, "Updating managed tunnel", func(ctx context.Context) tunnelOperationMsg {
			discovery, err := application.UpdateManagedTunnelByProfile(ctx, id, profileID, request)
			return tunnelOperationMsg{command: TunnelManagedUpdate, targetID: id, result: application.ManagedTunnelResult{Metadata: discovery.Metadata}, err: err}
		})
	case TunnelManagedConfigure:
		if page.configureForm == nil || page.targetID == "" {
			page.editor.SetFeedback("", fmt.Errorf("managed tunnel attach draft is unavailable"))
			return nil
		}
		data, id := page.configureForm, page.targetID
		options := application.AttachManagedTunnelOptions{AdminProfileID: data.AdminProfileID, ProjectID: data.ProjectID, Enabled: true}
		switch data.RuntimeKeyMode {
		case "auto":
			options.AutoGenerateRuntimeKey = true
		case "manual":
			options.RuntimeAPIKey = data.RuntimeAPIKey
		default:
			page.editor.SetFeedback("", fmt.Errorf("unsupported runtime credential mode: %s", data.RuntimeKeyMode))
			return nil
		}
		return page.startOperation(page.command, id, "Attaching managed tunnel", func(ctx context.Context) tunnelOperationMsg {
			local, err := application.AttachManagedTunnelWithOptions(ctx, id, options)
			result := application.ManagedTunnelResult{Configured: err == nil}
			if local.Status.Metadata != nil {
				result.Metadata = *local.Status.Metadata
			}
			return tunnelOperationMsg{command: TunnelManagedConfigure, targetID: id, result: result, err: err}
		})
	case TunnelManagedDelete:
		if page.deleteForm == nil || page.targetID == "" {
			page.editor.SetFeedback("", fmt.Errorf("managed tunnel delete draft is unavailable"))
			return nil
		}
		id, profileID := page.targetID, page.deleteForm.AdminProfileID
		return page.startOperation(page.command, id, "Deleting managed tunnel", func(ctx context.Context) tunnelOperationMsg {
			metadata, err := application.DeleteManagedTunnelByProfile(ctx, id, profileID)
			return tunnelOperationMsg{command: TunnelManagedDelete, targetID: id, result: application.ManagedTunnelResult{Metadata: metadata}, err: err}
		})
	default:
		page.editor.SetFeedback("", fmt.Errorf("unsupported managed tunnel editor action: %s", page.command))
		return nil
	}
}

func (page *TunnelPage) managedEditorSuccess(message, id string) tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return NavigateMsg{Path: []string{"tunnels", id}} },
		func() tea.Msg {
			return OperationResult("tunnel.managed.save", "Managed Tunnel", message, nil)
		},
	)
}

func (page *TunnelPage) acceptManagedEditorSuccess(metadata tunnel.Metadata) {
	if page == nil {
		return
	}
	editor, data := newManagedTunnelEditor(metadata, false, page.profilesForManaged(metadata.ID, true))
	page.editor, page.managedForm = &editor, data
	page.resizeEditor()
}

func (page *TunnelPage) acceptManagedConfigureSuccess() {
	if page == nil {
		return
	}
	editor, data := newManagedConfigureEditor(page.profilesForManaged(page.resourceID, false))
	page.editor, page.configureForm = &editor, data
	page.resizeEditor()
}

func (page *TunnelPage) cachedManagedTunnel(id string) (tunnel.Metadata, error) {
	for _, item := range page.items {
		if item.ID == id {
			return item, nil
		}
	}
	return tunnel.Metadata{}, fmt.Errorf("managed tunnel not found in local cache: %s", id)
}
