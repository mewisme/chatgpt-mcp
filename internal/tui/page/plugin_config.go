package page

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type pluginConfigFormData struct {
	ID     pluginpkg.PluginID
	Schema pluginpkg.SettingsSchema
	Bools  map[string]*bool
	Enums  map[string]*string
	Texts  map[string]*string
}

func (page *PluginPage) initPluginConfigEditor() error {
	id, err := pluginID(page.resourceID)
	if err != nil {
		return err
	}
	schema, values, err := page.service.PluginSettings(id)
	if err != nil {
		return err
	}
	if len(schema.Fields) == 0 {
		return fmt.Errorf("%w: %s", pluginpkg.ErrNoPluginConfig, id)
	}
	data := &pluginConfigFormData{
		ID: id, Schema: schema,
		Bools: map[string]*bool{}, Enums: map[string]*string{}, Texts: map[string]*string{},
	}
	fields := make([]huh.Field, 0, len(schema.Fields))
	for _, field := range schema.Fields {
		title := pluginSettingTitle(field)
		switch field.Kind {
		case pluginpkg.FieldBool:
			value := boolSetting(values[field.Key], field.Default)
			data.Bools[field.Key] = &value
			fields = append(fields, component.Switch(title, data.Bools[field.Key], "TRUE", "FALSE"))
		case pluginpkg.FieldEnum:
			value := stringSetting(values[field.Key], field.Default)
			data.Enums[field.Key] = &value
			options := make([]huh.Option[string], 0, len(field.Enum))
			for _, option := range field.Enum {
				options = append(options, huh.NewOption(option, option))
			}
			selectField := component.Select(title, data.Enums[field.Key], options...)
			if field.Description != "" {
				selectField = selectField.Description(field.Description)
			}
			fields = append(fields, selectField)
		default:
			value := stringSetting(values[field.Key], nil)
			if !field.Sensitive && value == "" {
				value = stringSetting(field.Default, nil)
			}
			if field.Sensitive {
				value = ""
			}
			data.Texts[field.Key] = &value
			captured := field
			if field.Sensitive {
				fields = append(fields, component.PasswordInput(title, data.Texts[field.Key]).Placeholder("leave blank to keep").Validate(func(raw string) error {
					if strings.TrimSpace(raw) == "" {
						return nil
					}
					_, err := pluginpkg.ParseSettingValue(captured, raw)
					return err
				}))
				continue
			}
			input := component.Input(title, data.Texts[field.Key]).Validate(func(raw string) error {
				_, err := pluginpkg.ParseSettingValue(captured, raw)
				return err
			})
			if field.Description != "" {
				input = input.Description(field.Description)
			}
			fields = append(fields, input)
		}
	}
	editor := component.NewEditor("save", component.EditorSection{
		ID: "config", Title: "Plugin Configuration",
		Description: "Global plugin configuration. Secret fields stay masked; leave them blank to keep the current value.",
		Form:        component.NewEditorForm(component.Group(fields...)),
	})
	page.configForm, page.editor = data, &editor
	return nil
}

func (page *PluginPage) submitPluginConfigEditor() tea.Cmd {
	if page.editor == nil || page.configForm == nil {
		return nil
	}
	if err := page.editor.Validate(); err != nil {
		page.editor.SetFeedback("", err)
		return nil
	}
	data := page.configForm
	for _, field := range data.Schema.Fields {
		raw, skip, err := data.rawValue(field)
		if err != nil {
			page.editor.SetFeedback("", err)
			return nil
		}
		if skip {
			continue
		}
		if err := page.service.SetPluginSetting(page.ctx, data.ID, field.Key, raw); err != nil {
			page.editor.SetFeedback("", err)
			return nil
		}
	}
	page.editor.Accept()
	return tea.Batch(page.editorParentNavigation(), func() tea.Msg {
		return OperationResult("plugin.config.save", "Plugins", "Plugin configuration saved", nil)
	})
}

func (data *pluginConfigFormData) rawValue(field pluginpkg.SettingField) (string, bool, error) {
	switch field.Kind {
	case pluginpkg.FieldBool:
		if data.Bools[field.Key] != nil && *data.Bools[field.Key] {
			return "true", false, nil
		}
		return "false", false, nil
	case pluginpkg.FieldEnum:
		if data.Enums[field.Key] != nil {
			return *data.Enums[field.Key], false, nil
		}
		return "", false, fmt.Errorf("%w: %s", pluginpkg.ErrInvalidConfigValue, field.Key)
	default:
		raw := ""
		if data.Texts[field.Key] != nil {
			raw = *data.Texts[field.Key]
		}
		if field.Sensitive && strings.TrimSpace(raw) == "" {
			return "", true, nil
		}
		return raw, false, nil
	}
}

func pluginSettingTitle(field pluginpkg.SettingField) string {
	title := strings.TrimSpace(field.Title)
	if title == "" {
		title = field.Key
	}
	if field.Default != nil {
		title += " (default: " + pluginpkg.FormatSettingValue(field.Default) + ")"
	}
	return title
}

func boolSetting(value, fallback any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	}
	switch typed := fallback.(type) {
	case bool:
		return typed
	}
	return false
}

func stringSetting(value, fallback any) string {
	if text := formatSetting(value); text != "" {
		return text
	}
	return formatSetting(fallback)
}

func formatSetting(value any) string {
	if value == nil {
		return ""
	}
	return pluginpkg.FormatSettingValue(value)
}
