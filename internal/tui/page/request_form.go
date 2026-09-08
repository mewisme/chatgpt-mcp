package page

import (
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type requestCreateFormData struct {
	WorkspaceID string
	Title       string
	Command     string
}

type requestResolveFormData struct {
	Reason  string
	Confirm bool
}

func newRequestCreateForm() (component.Form, *requestCreateFormData) {
	data := &requestCreateFormData{WorkspaceID: "ws_dummy", Title: "Allow test command", Command: "echo test approval"}
	form := component.NewForm(component.Group(
		component.Input("Workspace ID", &data.WorkspaceID).Description("Workspace label used by the synthetic approval request"),
		component.Input("Title", &data.Title).Description("Human-readable approval title"),
		component.Input("Command", &data.Command).Description("Command shown in the request arguments; it is not executed"),
	))
	return form, data
}

func newRequestResolveForm(request approval.Request, approve bool) (component.Form, *requestResolveFormData) {
	data := &requestResolveFormData{}
	action := "Deny"
	if approve {
		action = "Approve"
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = request.ID
	}
	confirm := component.Confirm(fmt.Sprintf("%s this exact request", action), &data.Confirm).Description(fmt.Sprintf("%s · %s · %s", request.ID, request.TargetTool, title))
	form := component.NewForm(component.Group(component.Input("Reason (optional)", &data.Reason), confirm))
	return form, data
}
