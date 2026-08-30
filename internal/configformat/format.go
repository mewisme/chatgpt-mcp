package configformat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type Format string

const (
	JSON Format = "json"
	YAML Format = "yaml"
	TOML Format = "toml"
)

func Parse(value string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "json":
		return JSON, nil
	case "yaml", "yml":
		return YAML, nil
	case "toml":
		return TOML, nil
	default:
		return "", fmt.Errorf("unsupported format %q; expected json, yaml, or toml", value)
	}
}

func Detect(path string) (Format, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return JSON, nil
	case ".yaml", ".yml":
		return YAML, nil
	case ".toml":
		return TOML, nil
	default:
		return "", fmt.Errorf("unsupported structured file extension: %s", filepath.Ext(path))
	}
}

func Extension(format Format) string {
	switch format {
	case YAML:
		return ".yaml"
	case TOML:
		return ".toml"
	default:
		return ".json"
	}
}

func Marshal(format Format, value any) ([]byte, error) {
	if format == JSON {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(data, '\n'), nil
	}
	raw, err := toGeneric(value)
	if err != nil {
		return nil, err
	}
	switch format {
	case YAML:
		data, err := yaml.Marshal(raw)
		if err != nil {
			return nil, err
		}
		return data, nil
	case TOML:
		data, err := toml.Marshal(raw)
		if err != nil {
			return nil, err
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

func Unmarshal(format Format, data []byte, value any) error {
	if format == JSON {
		return json.Unmarshal(data, value)
	}
	var raw any
	switch format {
	case YAML:
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return err
		}
		raw = normalizeYAML(raw)
	case TOML:
		var object map[string]any
		if err := toml.Unmarshal(data, &object); err != nil {
			return err
		}
		raw = object
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, value)
}

func MarshalPath(path string, value any) ([]byte, error) {
	format, err := Detect(path)
	if err != nil {
		return nil, err
	}
	return Marshal(format, value)
}

func UnmarshalPath(path string, data []byte, value any) error {
	format, err := Detect(path)
	if err != nil {
		return err
	}
	return Unmarshal(format, data, value)
}

func DecodeGeneric(format Format, data []byte) (any, error) {
	var raw any
	if format == JSON {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		return normalizeJSONNumbers(raw), nil
	}
	if format == YAML {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		return normalizeYAML(raw), nil
	}
	if format == TOML {
		var object map[string]any
		if err := toml.Unmarshal(data, &object); err != nil {
			return nil, err
		}
		return object, nil
	}
	return nil, fmt.Errorf("unsupported format: %s", format)
}

func EncodeGeneric(format Format, value any) ([]byte, error) {
	switch format {
	case JSON:
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(value); err != nil {
			return nil, err
		}
		return buffer.Bytes(), nil
	case YAML:
		return yaml.Marshal(value)
	case TOML:
		if _, ok := value.([]any); ok {
			return nil, errors.New("TOML cannot encode a root array without a named table")
		}
		return toml.Marshal(value)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

func toGeneric(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	return normalizeJSONNumbers(raw), nil
}

func normalizeJSONNumbers(value any) any {
	switch current := value.(type) {
	case json.Number:
		text := current.String()
		if !strings.ContainsAny(text, ".eE") {
			if integer, err := strconv.ParseInt(text, 10, 64); err == nil {
				return integer
			}
		}
		if decimal, err := strconv.ParseFloat(text, 64); err == nil {
			return decimal
		}
		return text
	case []any:
		for i := range current {
			current[i] = normalizeJSONNumbers(current[i])
		}
		return current
	case map[string]any:
		for key := range current {
			current[key] = normalizeJSONNumbers(current[key])
		}
		return current
	default:
		return current
	}
}

func normalizeYAML(value any) any {
	switch current := value.(type) {
	case map[string]any:
		for key := range current {
			current[key] = normalizeYAML(current[key])
		}
		return current
	case map[any]any:
		result := make(map[string]any, len(current))
		for key, nested := range current {
			result[fmt.Sprint(key)] = normalizeYAML(nested)
		}
		return result
	case []any:
		for i := range current {
			current[i] = normalizeYAML(current[i])
		}
		return current
	default:
		return current
	}
}
