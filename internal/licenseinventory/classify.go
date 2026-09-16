package licenseinventory

import (
	"path/filepath"
	"regexp"
	"strings"
)

var spdxHeader = regexp.MustCompile(`(?i)SPDX-License-Identifier:\s*([A-Za-z0-9.+-]+(?:\s+(?:AND|OR)\s+[A-Za-z0-9.+-]+)*)`)

func classifyLicense(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if match := spdxHeader.FindStringSubmatch(text); len(match) == 2 {
		return strings.Join(strings.Fields(match[1]), " ")
	}
	upper := strings.ToUpper(text)
	switch {
	case strings.Contains(upper, "SIL OPEN FONT LICENSE") && strings.Contains(upper, "VERSION 1.1"):
		return "OFL-1.1"
	case strings.Contains(upper, "MOZILLA PUBLIC LICENSE") && strings.Contains(upper, "2.0"):
		return "MPL-2.0"
	case strings.Contains(upper, "APACHE LICENSE") && strings.Contains(upper, "VERSION 2.0"):
		return "Apache-2.0"
	case strings.Contains(upper, "GNU GENERAL PUBLIC LICENSE") && (strings.Contains(upper, "VERSION 2") || strings.Contains(upper, "GPL VERSION 2")):
		return "GPL-2.0-only"
	case strings.Contains(upper, "BLUE OAK MODEL LICENSE"):
		return "BlueOak-1.0.0"
	case strings.Contains(upper, "THE UNLICENSE"):
		return "Unlicense"
	case strings.Contains(upper, "CREATIVE COMMONS") && strings.Contains(upper, "CC0"):
		return "CC0-1.0"
	case strings.Contains(upper, "ZLIB LICENSE") || (strings.Contains(upper, "ALADDIN ENTERPRISES") && strings.Contains(upper, "ZLIB")):
		return "Zlib"
	case strings.Contains(upper, "ISC LICENSE") || (strings.Contains(upper, "PERMISSION TO USE, COPY, MODIFY, AND/OR DISTRIBUTE") && strings.Contains(upper, "THE AUTHOR OR CONTRIBUTORS BE LIABLE")):
		return "ISC"
	case strings.Contains(upper, "PERMISSION IS HEREBY GRANTED, FREE OF CHARGE") && strings.Contains(upper, "THE SOFTWARE IS PROVIDED \"AS IS\""):
		return "MIT"
	case strings.Contains(upper, "REDISTRIBUTION AND USE IN SOURCE AND BINARY FORMS"):
		advertising := strings.Contains(upper, "ALL ADVERTISING MATERIALS")
		endorsement := strings.Contains(upper, "NEITHER THE NAME") || strings.Contains(upper, "THE NAMES OF ITS CONTRIBUTORS")
		switch {
		case advertising:
			return "BSD-4-Clause"
		case endorsement:
			return "BSD-3-Clause"
		default:
			return "BSD-2-Clause"
		}
	default:
		return ""
	}
}

func licenseFileNames() []string {
	return []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "LICENCE", "LICENCE.md", "COPYING", "COPYING.md", "LICENSE.MIT", "LICENSE.APACHE"}
}

func readLicenseFile(dir string) (string, string, error) {
	for _, name := range licenseFileNames() {
		path := filepath.Join(dir, name)
		data, err := readFileIfExists(path)
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(data) != "" {
			return name, data, nil
		}
	}
	return "", "", nil
}
