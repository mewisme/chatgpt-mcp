package page

import (
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type requestResolveFormData struct {
	Reason  string
	Confirm bool
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
