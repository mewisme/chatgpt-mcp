package pluginbuild

import "strings"

const (
	EnvUPX           = "CGM_PLUGIN_UPX"
	EnvUPXOff        = "off"
	DigestSentinel   = "0000000000000000000000000000000000000000000000000000000000000000"
	PinnedUPXVersion = "5.2.0"
)

func ShouldCompress(goos string) bool {
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "linux", "windows":
		return true
	default:
		return false
	}
}

func upxDisabled(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), EnvUPXOff)
}
