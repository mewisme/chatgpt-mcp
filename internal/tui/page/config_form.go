package page

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type configFieldFormData struct {
	Raw  string
	Bool bool
	Enum string
	Key  string
	Kind config.FieldKind
}

type configConvertFormData struct {
	Format  string
	Confirm bool
}

type configBundleFormData struct {
	Path    string
	Force   bool
	Confirm bool
}

func newConfigFieldForm(cfg config.Config, spec config.FieldSpec) (component.Form, *configFieldFormData, error) {
	if !spec.Editable {
		return component.Form{}, nil, fmt.Errorf("%s is read-only", spec.Key)
	}
	raw, err := config.RawValue(cfg, spec.Key)
	if err != nil {
		return component.Form{}, nil, err
	}
	data := &configFieldFormData{Raw: raw, Key: spec.Key, Kind: spec.Kind}
	validate := func(value string) error {
		next := cfg
		return config.SetValueValidated(&next, spec.Key, value)
	}
	var field huh.Field
	switch spec.Kind {
	case config.FieldBool:
		data.Bool = strings.EqualFold(raw, "true")
		field = component.Confirm(spec.Key, &data.Bool).Description(spec.Description)
	case config.FieldEnum:
		data.Enum = raw
		options := make([]huh.Option[string], 0, len(spec.Options))
		for _, option := range spec.Options {
			options = append(options, huh.NewOption(option, option))
		}
		field = component.Select(spec.Key, &data.Enum, options...).Description(spec.Description)
	case config.FieldList:
		data.Raw = strings.ReplaceAll(raw, ",", "\n")
		field = component.Text(spec.Key+" (one per line)", &data.Raw).Description(spec.Description).Validate(validate)
	case config.FieldInt, config.FieldString:
		field = component.Input(spec.Key, &data.Raw).Description(spec.Description).Validate(validate)
	default:
		return component.Form{}, nil, fmt.Errorf("unsupported config field type: %s", spec.Kind)
	}
	return component.NewForm(component.Group(field)), data, nil
}

func configFieldFormValue(data *configFieldFormData) string {
	if data == nil {
		return ""
	}
	switch data.Kind {
	case config.FieldBool:
		if data.Bool {
			return "true"
		}
		return "false"
	case config.FieldEnum:
		return data.Enum
	default:
		return data.Raw
	}
}

func newConfigConvertForm(current configformat.Format) (component.Form, *configConvertFormData) {
	data := &configConvertFormData{Format: string(current)}
	form := component.NewForm(component.Group(
		component.Select("Target format", &data.Format, huh.NewOption("JSON", "json"), huh.NewOption("YAML", "yaml"), huh.NewOption("TOML", "toml")),
		component.Confirm("Convert all structured config/state files", &data.Confirm),
	))
	return form, data
}

func newConfigBundleForm(export bool) (component.Form, *configBundleFormData) {
	data := &configBundleFormData{Path: "chatgpt-mcp-config.cgm"}
	title := "Bundle file"
	confirmTitle := "Import this bundle and replace existing configuration/state"
	if export {
		confirmTitle = "Overwrite the destination if it already exists"
	}
	form := component.NewForm(component.Group(
		component.Input(title, &data.Path).Validate(func(value string) error {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("bundle file is required")
			}
			if filepath.Clean(value) == "." {
				return fmt.Errorf("bundle file must name a file")
			}
			return nil
		}),
		component.Confirm(confirmTitle, &data.Force),
	))
	if !export {
		data.Confirm = false
		form = component.NewForm(component.Group(
			component.Input(title, &data.Path).Validate(requiredValue("bundle file")),
			component.Confirm("Replace existing configuration/state", &data.Force),
			component.Confirm("I understand the current configuration may be replaced", &data.Confirm),
		))
	}
	return form, data
}
