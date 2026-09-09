package parser

import (
	"strconv"
	"strings"
)

func splitEscaped(value string, delimiter rune, limit int) []string {
	parts := make([]string, 0, limit)
	var current strings.Builder
	escaped := false
	for _, character := range value {
		if escaped {
			current.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		if character == delimiter && len(parts) < limit-1 {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(character)
	}
	parts = append(parts, current.String())
	return parts
}

func parseKeyValues(value string, delimiter rune) map[string]any {
	if delimiter == ' ' {
		return parseSpaceKeyValues(value)
	}
	fields := make(map[string]any)
	for _, token := range strings.FieldsFunc(value, func(character rune) bool { return character == delimiter }) {
		key, candidate, found := strings.Cut(strings.TrimSpace(token), "=")
		if !found || key == "" {
			continue
		}
		fields[key] = strings.Trim(strings.TrimSpace(candidate), "\"")
	}
	return fields
}

// CEF extensions separate fields with a space but values may contain spaces.
// A following token is a delimiter only when it has the shape key=value.
func parseSpaceKeyValues(value string) map[string]any {
	fields := make(map[string]any)
	for position := 0; position < len(value); {
		for position < len(value) && value[position] == ' ' {
			position++
		}
		keyStart := position
		for position < len(value) && value[position] != '=' && value[position] != ' ' {
			position++
		}
		if position == keyStart || position == len(value) || value[position] != '=' {
			for position < len(value) && value[position] != ' ' {
				position++
			}
			continue
		}
		key := value[keyStart:position]
		position++
		valueStart := position
		escaped := false
		for position < len(value) {
			if escaped {
				escaped = false
				position++
				continue
			}
			if value[position] == '\\' {
				escaped = true
				position++
				continue
			}
			if value[position] == ' ' && startsKeyValue(value, position+1) {
				break
			}
			position++
		}
		fields[key] = strings.Trim(strings.ReplaceAll(strings.TrimSpace(value[valueStart:position]), "\\ ", " "), "\"")
	}
	return fields
}

func startsKeyValue(value string, position int) bool {
	if position >= len(value) {
		return false
	}
	for current := position; current < len(value); current++ {
		character := value[current]
		if character == '=' {
			return current > position
		}
		if character == ' ' || character == '\\' {
			return false
		}
	}
	return false
}

func first(fields map[string]any, names ...string) string {
	for _, name := range names {
		if value, ok := fields[name].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
func integer(fields map[string]any, names ...string) int {
	value := first(fields, names...)
	result, _ := strconv.Atoi(value)
	if result < 0 || result > 65535 {
		return 0
	}
	return result
}
