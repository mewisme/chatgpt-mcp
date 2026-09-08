package commandpattern

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode"
)

const pathSeparatorSentinel = '\ue000'

type Pattern struct {
	raw    string
	tokens []string
}

func Parse(raw string) (Pattern, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Pattern{}, errors.New("command pattern is empty")
	}
	tokens, err := words(raw)
	if err != nil {
		return Pattern{}, err
	}
	if len(tokens) == 0 {
		return Pattern{}, errors.New("command pattern is empty")
	}
	for _, token := range tokens {
		if token == "**" {
			continue
		}
		if _, err := matchToken(token, ""); err != nil {
			return Pattern{}, fmt.Errorf("invalid command pattern %q: %w", raw, err)
		}
	}
	tokens[0] = strings.ToLower(tokens[0])
	return Pattern{raw: raw, tokens: tokens}, nil
}

func Compile(values []string) ([]Pattern, error) {
	result := make([]Pattern, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		pattern, err := Parse(value)
		if err != nil {
			return nil, err
		}
		result = append(result, pattern)
	}
	return result, nil
}

func Validate(value string) error {
	_, err := Parse(value)
	return err
}

func (p Pattern) Raw() string { return p.raw }

func (p Pattern) Match(argv []string) bool {
	if len(p.tokens) == 0 || len(argv) == 0 {
		return false
	}
	values := append([]string(nil), argv...)
	values[0] = strings.ToLower(values[0])
	return matchTokens(p.tokens, values)
}

func MatchAny(patterns []Pattern, argv []string) bool {
	for _, pattern := range patterns {
		if pattern.Match(argv) {
			return true
		}
	}
	return false
}

func matchTokens(pattern, values []string) bool {
	if len(pattern) == 0 {
		return true
	}
	if pattern[0] == "**" {
		if len(pattern) == 1 {
			return true
		}
		for index := 0; index <= len(values); index++ {
			if matchTokens(pattern[1:], values[index:]) {
				return true
			}
		}
		return false
	}
	if len(values) == 0 {
		return false
	}
	matched, err := matchToken(pattern[0], values[0])
	return err == nil && matched && matchTokens(pattern[1:], values[1:])
}

func matchToken(pattern, value string) (bool, error) {
	pattern = strings.NewReplacer("/", string(pathSeparatorSentinel), `\`, string(pathSeparatorSentinel)).Replace(pattern)
	value = strings.NewReplacer("/", string(pathSeparatorSentinel), `\`, string(pathSeparatorSentinel)).Replace(value)
	return path.Match(pattern, value)
}

func words(value string) ([]string, error) {
	var result []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			result = append(result, current.String())
			current.Reset()
		}
	}
	for _, r := range value {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	if quote != 0 || escaped {
		return nil, errors.New("unbalanced command pattern quoting")
	}
	flush()
	return result, nil
}
