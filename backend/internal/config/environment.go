package config

import (
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	durationType = reflect.TypeOf(time.Duration(0))
	logLevelType = reflect.TypeOf(slog.Level(0))
)

// environmentValue resolves a configuration value.
// OS environment variables take precedence over file values.
func environmentValue(name string, fileValues map[string]string) (string, bool) {
	if value, exists := os.LookupEnv(name); exists {
		return value, true
	}

	value, exists := fileValues[name]
	return value, exists
}

// applyEnvironment binds environment variables to configuration fields.
func applyEnvironment(configuration *Config, fileValues map[string]string) error {
	if configuration == nil {
		return fmt.Errorf("configuration must not be nil")
	}

	value := reflect.ValueOf(configuration)

	if value.Kind() != reflect.Pointer || value.IsNil() {
		return fmt.Errorf("configuration must be a non-nil pointer")
	}

	value = value.Elem()

	if value.Kind() != reflect.Struct {
		return fmt.Errorf("configuration must point to a struct")
	}

	return bindEnvironment(value, nil, fileValues)
}

// bindEnvironment recursively binds configuration fields.
func bindEnvironment(
	value reflect.Value,
	path []string,
	fileValues map[string]string,
) error {
	if value.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct at %s", strings.Join(path, "."))
	}

	valueType := value.Type()

	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		fieldType := valueType.Field(index)

		if fieldType.PkgPath != "" {
			continue
		}

		configTag := fieldType.Tag.Get("config")
		configName := strings.TrimSpace(strings.SplitN(configTag, ",", 2)[0])

		if configName == "-" {
			continue
		}

		if configName == "" {
			configName = fieldType.Name
		}

		fieldPath := appendPath(path, configName)

		if field.Kind() == reflect.Struct && field.Type() != durationType {
			if err := bindEnvironment(field, fieldPath, fileValues); err != nil {
				return err
			}

			continue
		}

		environmentName := fieldType.Tag.Get("env")

		if environmentName == "-" {
			continue
		}

		if environmentName == "" {
			environmentName = "TRACEPILOT_" +
				strings.ToUpper(strings.Join(fieldPath, "_"))
		}

		rawValue, exists := environmentValue(environmentName, fileValues)
		if !exists {
			continue
		}

		if !field.CanSet() {
			return fmt.Errorf(
				"configuration field %s cannot be set",
				strings.Join(fieldPath, "."),
			)
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

// appendPath creates a new configuration path without modifying its parent.
func appendPath(path []string, name string) []string {
	result := make([]string, len(path), len(path)+1)
	copy(result, path)

	return append(result, name)
}

// setEnvironmentValue parses and assigns a configuration value.
func setEnvironmentValue(field reflect.Value, rawValue string) error {
	fieldType := field.Type()

	switch {
	case fieldType == durationType:
		value, err := time.ParseDuration(strings.TrimSpace(rawValue))
		if err != nil {
			return fmt.Errorf("must be a valid duration: %w", err)
		}

		field.SetInt(int64(value))

	case fieldType == logLevelType:
		var value slog.Level

		if err := value.UnmarshalText(
			[]byte(strings.ToUpper(strings.TrimSpace(rawValue))),
		); err != nil {
			return fmt.Errorf("must be debug, info, warn, or error: %w", err)
		}

		field.SetInt(int64(value))

	case field.Kind() == reflect.String:
		field.SetString(rawValue)

	case field.Kind() == reflect.Bool:
		value, err := strconv.ParseBool(strings.TrimSpace(rawValue))
		if err != nil {
			return fmt.Errorf("must be a valid boolean: %w", err)
		}

		field.SetBool(value)

	case isIntegerKind(field.Kind()):
		value, err := strconv.ParseInt(
			strings.TrimSpace(rawValue),
			10,
			fieldType.Bits(),
		)
		if err != nil {
			return fmt.Errorf("must be a valid integer: %w", err)
		}

		field.SetInt(value)

	case field.Kind() == reflect.Slice &&
		fieldType.Elem().Kind() == reflect.String:

		field.Set(reflect.ValueOf(parseStringSlice(rawValue)))

	default:
		return fmt.Errorf("unsupported configuration type %s", fieldType)
	}

	return nil
}

// isIntegerKind reports whether the kind is a signed integer type.
func isIntegerKind(kind reflect.Kind) bool {
	return kind >= reflect.Int && kind <= reflect.Int64
}

// parseStringSlice parses a comma-separated string.
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
