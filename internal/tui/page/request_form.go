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
		component.Input("Workspace ID (synthetic request label)", &data.WorkspaceID),
		component.Input("Title (human-readable approval title)", &data.Title),
		component.Input("Command (displayed only; not executed)", &data.Command),
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
