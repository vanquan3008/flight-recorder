package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment parsing is kept separate from the field-to-env mapping in
// env.go. These helpers are deliberately small and side-effect free apart
// from reading one explicitly named variable.

func splitCommaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func environmentValue(name, fallback string) string {
	if value, exists := os.LookupEnv(name); exists {
		return value
	}
	return fallback
}

func readDuration(name string, fallback time.Duration) (time.Duration, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsedDuration, parseError := time.ParseDuration(rawValue)
	if parseError != nil || parsedDuration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsedDuration, nil
}

func readNonNegativeDuration(name string, fallback time.Duration) (time.Duration, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsedDuration, parseError := time.ParseDuration(rawValue)
	if parseError != nil || parsedDuration < 0 {
		return 0, fmt.Errorf("%s must be a non-negative duration", name)
	}
	return parsedDuration, nil
}

func readBoolean(name string, fallback bool) (bool, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsed, parseError := strconv.ParseBool(rawValue)
	if parseError != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return parsed, nil
}

func readPositiveInteger(name string, fallback int) (int, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsedInteger, parseError := strconv.Atoi(rawValue)
	if parseError != nil || parsedInteger <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsedInteger, nil
}

func readPositiveInt64(name string, fallback int64) (int64, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsedInteger, parseError := strconv.ParseInt(rawValue, 10, 64)
	if parseError != nil || parsedInteger <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsedInteger, nil
}

func readPositiveInt32(name string, fallback int32) (int32, error) {
	parsedInteger, parseError := readInt32(name, fallback)
	if parseError != nil || parsedInteger <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsedInteger, nil
}

func readNonNegativeInt32(name string, fallback int32) (int32, error) {
	parsedInteger, parseError := readInt32(name, fallback)
	if parseError != nil || parsedInteger < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return parsedInteger, nil
}

func readInt32(name string, fallback int32) (int32, error) {
	rawValue, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	parsedInteger, parseError := strconv.ParseInt(rawValue, 10, 32)
	return int32(parsedInteger), parseError
}

func parseLogLevel(value string) (slog.Level, error) {
	var level slog.Level
	if parseError := level.UnmarshalText([]byte(strings.ToUpper(value))); parseError != nil {
		return 0, errors.New("TRACEPILOT_LOG_LEVEL/log_level must be debug, info, warn, or error")
	}
	return level, nil
}
