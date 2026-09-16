package tunnel

import "strings"

func DisplayLabel(id, name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return strings.TrimSpace(id)
}

func ShortID(id string) string {
	id = strings.TrimPrefix(strings.TrimSpace(id), "tunnel_")
	if len(id) <= 8 {
		return id
	}
	return id[len(id)-8:]
}

func UniqueLabel(id, name string, duplicate bool) string {
	name = strings.TrimSpace(name)
	if duplicate && name != "" {
		if short := ShortID(id); short != "" {
			return name + " · " + short
		}
	}
	return DisplayLabel(id, name)
}

func UniqueLabels(ids, names []string) []string {
	counts := make(map[string]int, len(names))
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			counts[name]++
		}
	}
	labels := make([]string, len(ids))
	for i, id := range ids {
		name := ""
		if i < len(names) {
			name = names[i]
		}
		labels[i] = UniqueLabel(id, name, counts[strings.TrimSpace(name)] > 1)
	}
	return labels
}
