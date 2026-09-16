package tunnel

import "strings"

func DisplayLabel(id, name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return strings.TrimSpace(id)
}
