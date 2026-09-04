package update

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidVersion = errors.New("invalid semantic version")

type semanticVersion struct {
	major, minor, patch uint64
	prerelease          []string
}

func NormalizeVersion(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidVersion
	}
	if value[0] != 'v' && value[0] != 'V' {
		value = "v" + value
	} else if value[0] == 'V' {
		value = "v" + value[1:]
	}
	parsed, err := parseVersion(value)
	if err != nil {
		return "", err
	}
	result := fmt.Sprintf("v%d.%d.%d", parsed.major, parsed.minor, parsed.patch)
	if len(parsed.prerelease) > 0 {
		result += "-" + strings.Join(parsed.prerelease, ".")
	}
	return result, nil
}

func CompareVersions(left, right string) (int, error) {
	leftVersion, err := parseVersionNormalized(left)
	if err != nil {
		return 0, err
	}
	rightVersion, err := parseVersionNormalized(right)
	if err != nil {
		return 0, err
	}
	for _, pair := range [][2]uint64{{leftVersion.major, rightVersion.major}, {leftVersion.minor, rightVersion.minor}, {leftVersion.patch, rightVersion.patch}} {
		if pair[0] < pair[1] {
			return -1, nil
		}
		if pair[0] > pair[1] {
			return 1, nil
		}
	}
	return comparePrerelease(leftVersion.prerelease, rightVersion.prerelease), nil
}

func parseVersionNormalized(value string) (semanticVersion, error) {
	normalized, err := NormalizeVersion(value)
	if err != nil {
		return semanticVersion{}, err
	}
	return parseVersion(normalized)
}

func parseVersion(value string) (semanticVersion, error) {
	if !strings.HasPrefix(value, "v") || len(value) < 2 {
		return semanticVersion{}, fmt.Errorf("%w: %q", ErrInvalidVersion, value)
	}
	body := value[1:]
	if index := strings.IndexByte(body, '+'); index >= 0 {
		metadata := body[index+1:]
		if !validIdentifiers(metadata, false) {
			return semanticVersion{}, fmt.Errorf("%w: %q", ErrInvalidVersion, value)
		}
		body = body[:index]
	}
	var prerelease []string
	if index := strings.IndexByte(body, '-'); index >= 0 {
		pre := body[index+1:]
		if !validIdentifiers(pre, true) {
			return semanticVersion{}, fmt.Errorf("%w: %q", ErrInvalidVersion, value)
		}
		prerelease = strings.Split(pre, ".")
		body = body[:index]
	}
	parts := strings.Split(body, ".")
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("%w: %q", ErrInvalidVersion, value)
	}
	values := [3]uint64{}
	for i, part := range parts {
		if !validNumeric(part) {
			return semanticVersion{}, fmt.Errorf("%w: %q", ErrInvalidVersion, value)
		}
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semanticVersion{}, fmt.Errorf("%w: %q", ErrInvalidVersion, value)
		}
		values[i] = number
	}
	return semanticVersion{major: values[0], minor: values[1], patch: values[2], prerelease: prerelease}, nil
}

func validNumeric(value string) bool {
	if value == "" || len(value) > 1 && value[0] == '0' {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func validIdentifiers(value string, rejectNumericLeadingZero bool) bool {
	if value == "" {
		return false
	}
	for _, identifier := range strings.Split(value, ".") {
		if identifier == "" {
			return false
		}
		numeric := true
		for _, char := range identifier {
			if char < '0' || char > '9' {
				numeric = false
			}
			if char != '-' && (char < '0' || char > '9') && (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') {
				return false
			}
		}
		if rejectNumericLeadingZero && numeric && len(identifier) > 1 && identifier[0] == '0' {
			return false
		}
	}
	return true
}

func comparePrerelease(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for i := 0; i < limit; i++ {
		leftNumber, leftNumeric := numericIdentifier(left[i])
		rightNumber, rightNumeric := numericIdentifier(right[i])
		switch {
		case leftNumeric && rightNumeric:
			if leftNumber < rightNumber {
				return -1
			}
			if leftNumber > rightNumber {
				return 1
			}
		case leftNumeric:
			return -1
		case rightNumeric:
			return 1
		default:
			if left[i] < right[i] {
				return -1
			}
			if left[i] > right[i] {
				return 1
			}
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func numericIdentifier(value string) (uint64, bool) {
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
	}
	number, err := strconv.ParseUint(value, 10, 64)
	return number, err == nil
}
