package patch

import "strings"

func ApplyText(original, replacement string) string {
	return strings.Replace(original, replacement, replacement, 1)
}
