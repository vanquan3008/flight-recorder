package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var (
	durationType = reflect.TypeFor[time.Duration]()
	logLevelType = reflect.TypeFor[slog.Level]()
)

// readEnvironmentFile reads backend/.env from the repository root
// or the backend module directory.
func readEnvironmentFile() (map[string]string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	directory := filepath.Join(workingDirectory, "backend")
	if filepath.Base(workingDirectory) == "backend" {
		directory = workingDirectory
	}

	if !hasGoModule(directory) {
		return nil, nil
	}

	return readEnvironmentFileAt(filepath.Join(directory, ".env"))
}

func hasGoModule(directory string) bool {
	info, err := os.Stat(filepath.Join(directory, "go.mod"))
	return err == nil && !info.IsDir()
}

func readEnvironmentFileAt(path string) (map[string]string, error) {
	values, err := godotenv.Read(path)
	if err == nil {
		return values, nil
	}
	if os.IsNotExist(err) {
		return nil, nil
	}

	slog.Error("failed to read environment file", "path", path, "error", err)
	return nil, fmt.Errorf("read environment file %q: invalid or unreadable file", path)
}

// environmentValue prioritizes OS environment variables over .env.
func environmentValue(name string, fileValues map[string]string) (string, bool) {
	if value, exists := os.LookupEnv(name); exists {
		return value, true
	}

	value, exists := fileValues[name]
	return value, exists
}

// applyEnvironment binds environment values to Config.
func applyEnvironment(configuration *Config, fileValues map[string]string, scopes ...string) error {
	if configuration == nil {
		return fmt.Errorf("configuration must not be nil")
	}

	return bindEnvironment(reflect.ValueOf(configuration).Elem(), nil, fileValues, scopes...)
}

// bindEnvironment recursively binds exported configuration fields.
func bindEnvironment(value reflect.Value, path []string, fileValues map[string]string, scopes ...string) error {
	if value.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct at %s", strings.Join(path, "."))
	}

	valueType := value.Type()

	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		fieldType := valueType.Field(index)

		if !fieldType.IsExported() {
			continue
		}

		configName := strings.TrimSpace(strings.SplitN(fieldType.Tag.Get("config"), ",", 2)[0])
		if configName == "-" {
			continue
		}
		if configName == "" {
			configName = fieldType.Name
		}

		fieldPath := appendPath(path, configName)
		if !environmentScopeSelected(fieldPath, scopes) {
			continue
		}

		if field.Kind() == reflect.Struct {
			if err := bindEnvironment(field, fieldPath, fileValues, scopes...); err != nil {
				return err
			}
			continue
		}

		environmentName := fieldType.Tag.Get("env")
		if environmentName == "-" {
			continue
		}
		if environmentName == "" {
			environmentName = "TRACEPILOT_" + strings.ToUpper(strings.Join(fieldPath, "_"))
		}

		rawValue, exists := environmentValue(environmentName, fileValues)
		if !exists {
			continue
		}

		if !field.CanSet() {
			return fmt.Errorf("configuration field %s cannot be set", strings.Join(fieldPath, "."))
		}

		if err := setEnvironmentValue(field, rawValue); err != nil {
			return fmt.Errorf(
				"invalid configuration %s (environment variable %s): %w",
				strings.Join(fieldPath, "."),
				environmentName,
				err,
			)
		}
	}

	return nil
}

func environmentScopeSelected(path, scopes []string) bool {
	if len(scopes) == 0 {
		return true
	}
	field := strings.Join(path, ".")
	for _, scope := range scopes {
		if field == scope || strings.HasPrefix(field, scope+".") || strings.HasPrefix(scope, field+".") {
			return true
		}
	}
	return false
}

// appendPath creates a child path without modifying its parent.
func appendPath(path []string, name string) []string {
	result := make([]string, len(path), len(path)+1)
	copy(result, path)
	return append(result, name)
}

// setEnvironmentValue parses and assigns supported configuration types.
func setEnvironmentValue(field reflect.Value, rawValue string) error {
	fieldType := field.Type()
	trimmed := strings.TrimSpace(rawValue)

	switch {
	case fieldType == durationType:
		value, err := time.ParseDuration(trimmed)
		if err != nil {
			return fmt.Errorf("invalid duration: %w", err)
		}
		field.SetInt(int64(value))

	case fieldType == logLevelType:
		var level slog.Level
		if err := level.UnmarshalText([]byte(strings.ToUpper(trimmed))); err != nil {
			return fmt.Errorf("invalid log level: %w", err)
		}
		field.SetInt(int64(level))

	case field.Kind() == reflect.String:
		field.SetString(rawValue)

	case field.Kind() == reflect.Bool:
		value, err := strconv.ParseBool(trimmed)
		if err != nil {
			return fmt.Errorf("invalid boolean: %w", err)
		}
		field.SetBool(value)

	case isIntegerKind(field.Kind()):
		value, err := strconv.ParseInt(trimmed, 10, fieldType.Bits())
		if err != nil {
			return fmt.Errorf("invalid integer: %w", err)
		}
		field.SetInt(value)

	case field.Kind() == reflect.Slice && fieldType.Elem().Kind() == reflect.String:
		values := parseStringSlice(rawValue)
		slice := reflect.MakeSlice(fieldType, 0, len(values))

		for _, value := range values {
			element := reflect.ValueOf(value).Convert(fieldType.Elem())
			slice = reflect.Append(slice, element)
		}

		field.Set(slice)

	default:
		return fmt.Errorf("unsupported configuration type %s", fieldType)
	}

	return nil
}

func isIntegerKind(kind reflect.Kind) bool {
	return kind >= reflect.Int && kind <= reflect.Int64
}

// parseStringSlice parses comma-separated values.
func parseStringSlice(rawValue string) []string {
	if strings.TrimSpace(rawValue) == "" {
		return []string{}
	}

	parts := strings.Split(rawValue, ",")
	values := make([]string, 0, len(parts))

	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}

	return values
}
