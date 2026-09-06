package page

import (
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type installFormData struct {
	NoAlias       bool
	Force         bool
	MigrateLegacy bool
	Confirm       bool
}

type updateFormData struct {
	TargetVersion string
	NoRestart     bool
	Confirm       bool
}

func newInstallForm() (component.Form, *installFormData) {
	data := &installFormData{MigrateLegacy: true}
	form := component.NewForm(component.Group(
		component.Switch("Skip cgm alias", &data.NoAlias),
		component.Switch("Allow development build", &data.Force),
		component.Switch("Clean verified legacy installations", &data.MigrateLegacy),
		component.Confirm("Install this binary into the managed layout", &data.Confirm),
	).Title("Managed install"))
	return form, data
}

func (data *installFormData) Options() application.InstallCurrentOptions {
	if data == nil {
		return application.InstallCurrentOptions{}
	}
	return application.InstallCurrentOptions{NoAlias: data.NoAlias, Force: data.Force, MigrateLegacy: data.MigrateLegacy}
}

func newUpdateForm() (component.Form, *updateFormData) {
	data := &updateFormData{}
	form := component.NewForm(component.Group(
		component.Input("Target version", &data.TargetVersion).Description("Leave empty for latest release; explicit versions may downgrade"),
		component.Switch("Skip managed runtime restart", &data.NoRestart),
		component.Confirm("Apply the verified update", &data.Confirm),
	).Title("Update"))
	return form, data
}

func (data *updateFormData) Options() application.UpdateApplyOptions {
	if data == nil {
		return application.UpdateApplyOptions{}
	}
	return application.UpdateApplyOptions{TargetVersion: strings.TrimSpace(data.TargetVersion), NoRestart: data.NoRestart}
}
