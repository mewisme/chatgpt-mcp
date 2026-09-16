package redact

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	secretTokenPattern = regexp.MustCompile(`(?i)\b(?:mcp|admin|runtime)_[A-Za-z0-9_-]{24,}\b`)
	bearerPattern      = regexp.MustCompile(`(?i)(\bbearer\s+)[^\s,;]+`)
	assignmentPattern  = regexp.MustCompile(`(?i)\b(authorization|api[_-]?key|apikey|password|client[_-]?secret|secret|access[_-]?token|refresh[_-]?token|bearer[_-]?token|token|credential|signature)\b["']?(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
)

func SensitiveKey(key string) bool {
	key = normalizeKey(key)
	if key == "authorization" || key == "cookie" || key == "set_cookie" || key == "token" || key == "oauth_code" || key == "oauth_state" || key == "secret" {
		return true
	}
	for _, fragment := range []string{"access_token", "refresh_token", "bearer_token", "client_secret", "admin_key", "runtime_api_key", "api_key", "apikey", "token_hash", "password", "signature", "credential"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func URL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.User = nil
	query := parsed.Query()
	for key, values := range query {
		if !sensitiveQueryKey(key) {
			continue
		}
		for index := range values {
			values[index] = "<redacted>"
		}
		query[key] = values
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func Text(value string) string {
	value = sanitizeURLs(value)
	value = bearerPattern.ReplaceAllString(value, `${1}<redacted>`)
	value = secretTokenPattern.ReplaceAllString(value, `<redacted>`)
	return assignmentPattern.ReplaceAllStringFunc(value, redactAssignment)
}

func redactAssignment(value string) string {
	if alreadyRedacted(value) {
		return value
	}
	match := assignmentPattern.FindStringSubmatch(value)
	if len(match) < 3 {
		return value
	}
	return match[1] + match[2] + "<redacted>"
}

func alreadyRedacted(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "<redacted>") || strings.Contains(lower, "[redacted]")
}

func Value(key string, value any) any {
	if SensitiveKey(key) {
		if text, ok := value.(string); ok && alreadyRedacted(text) {
			return text
		}
		return "<redacted>"
	}
	switch typed := value.(type) {
	case string:
		return Text(typed)
	case []string:
		out := make([]string, len(typed))
		for index, item := range typed {
			out[index] = Text(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = Value("", item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, item := range typed {
			out[childKey] = Value(childKey, item)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(typed))
		for childKey, item := range typed {
			if SensitiveKey(childKey) {
				out[childKey] = "<redacted>"
			} else {
				out[childKey] = Text(item)
			}
		}
		return out
	case error:
		return Text(typed.Error())
	default:
		return value
	}
}

func sanitizeURLs(text string) string {
	for _, marker := range []string{"http://", "https://"} {
		start := 0
		for {
			index := strings.Index(text[start:], marker)
			if index < 0 {
				break
			}
			index += start
			end := index
			for end < len(text) && !strings.ContainsRune(" \t\r\n)]}\"'", rune(text[end])) {
				end++
			}
			raw := text[index:end]
			clean := URL(raw)
			text = text[:index] + clean + text[end:]
			start = index + len(clean)
		}
	}
	return text
}

func sensitiveQueryKey(key string) bool {
	key = normalizeKey(key)
	if SensitiveKey(key) {
		return true
	}
	return key == "token" || key == "code" || key == "state" || key == "sig" || strings.HasSuffix(key, "_token") || strings.HasSuffix(key, "_signature")
}

func normalizeKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(key)
}

func String(value any) string { return Text(fmt.Sprint(value)) }
