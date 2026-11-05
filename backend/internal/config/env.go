package config

import (
	"errors"
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
	durationType = reflect.TypeOf(time.Duration(0))
	logLevelType = reflect.TypeOf(slog.Level(0))
)

// readEnvironmentFile reads only backend/.env. The Go command can be launched
// from either the repository root or the backend module directory.
func readEnvironmentFile() (map[string]string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		slog.Error("failed to locate working directory for environment file", "error", err)
		return nil, errors.New("locate backend environment file: " + err.Error())
	}

	backendDirectory := filepath.Join(workingDirectory, "backend")
	if hasGoModule(backendDirectory) {
		return readEnvironmentFileAt(filepath.Join(backendDirectory, ".env"))
	}
	if filepath.Base(workingDirectory) == "backend" && hasGoModule(workingDirectory) {
		return readEnvironmentFileAt(filepath.Join(workingDirectory, ".env"))
	}
	return nil, nil
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
	return nil, errors.New("read environment file \"" + path + "\": invalid or unreadable file")
}

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
		slog.Error("configuration must not be nil")
		return errors.New("configuration must not be nil")
	}

	value := reflect.ValueOf(configuration)

	if value.Kind() != reflect.Pointer || value.IsNil() {
		slog.Error("configuration must be a non-nil pointer")
		return errors.New("configuration must be a non-nil pointer")
	}

	value = value.Elem()

	if value.Kind() != reflect.Struct {
		slog.Error("configuration must point to a struct")
		return errors.New("configuration must point to a struct")
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
		pathStr := strings.Join(path, ".")
		slog.Error("expected struct at configuration path", "path", pathStr)
		return errors.New("expected struct at " + pathStr)
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
			fieldStr := strings.Join(fieldPath, ".")
			slog.Error("configuration field cannot be set", "field", fieldStr)
			return errors.New("configuration field " + fieldStr + " cannot be set")
		}

		if err := setEnvironmentValue(field, rawValue); err != nil {
			fieldStr := strings.Join(fieldPath, ".")
			slog.Error("invalid configuration environment value", "field", fieldStr, "env", environmentName, "error", err)
			return errors.New("invalid configuration " + fieldStr + " (environment variable " + environmentName + "): " + err.Error())
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
			return errors.New("must be a valid duration: " + err.Error())
		}

		field.SetInt(int64(value))

	case fieldType == logLevelType:
		var value slog.Level

		if err := value.UnmarshalText(
			[]byte(strings.ToUpper(strings.TrimSpace(rawValue))),
		); err != nil {
			return errors.New("must be debug, info, warn, or error: " + err.Error())
		}

		field.SetInt(int64(value))

	case field.Kind() == reflect.String:
		field.SetString(rawValue)

	case field.Kind() == reflect.Bool:
		value, err := strconv.ParseBool(strings.TrimSpace(rawValue))
		if err != nil {
			return errors.New("must be a valid boolean: " + err.Error())
		}

		field.SetBool(value)

	case isIntegerKind(field.Kind()):
		value, err := strconv.ParseInt(
			strings.TrimSpace(rawValue),
			10,
			fieldType.Bits(),
		)
		if err != nil {
			return errors.New("must be a valid integer: " + err.Error())
		}

		field.SetInt(value)

	case field.Kind() == reflect.Slice &&
		fieldType.Elem().Kind() == reflect.String:

		field.Set(reflect.ValueOf(parseStringSlice(rawValue)))

	default:
		return errors.New("unsupported configuration type " + fieldType.String())
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
