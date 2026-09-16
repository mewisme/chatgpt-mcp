package runtimeplugin

import (
	"os"
	"sort"
	"strings"
)

func safeProcessEnvironment(extra ...string) []string {
	allowed := map[string]struct{}{"home": {}, "lang": {}, "path": {}, "systemroot": {}, "temp": {}, "tmp": {}, "userprofile": {}, "windir": {}}
	result := make([]string, 0, len(allowed)+len(extra))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, keep := allowed[strings.ToLower(key)]; keep {
			result = append(result, entry)
		}
	}
	result = append(result, extra...)
	sort.Strings(result)
	return result
}
