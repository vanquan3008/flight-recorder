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

func environmentValue(name string, fileValues map[string]string) (string, bool) {
	if value, exists := os.LookupEnv(name); exists {
		return value, true
	}
	value, exists := fileValues[name]
	return value, exists
}

func applyEnvironment(configuration *Config, fileValues map[string]string) error {
	return bindEnvironment(reflect.ValueOf(configuration).Elem(), nil, fileValues)
}

func bindEnvironment(value reflect.Value, path []string, fileValues map[string]string) error {
	valueType := value.Type()
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		fieldType := valueType.Field(index)
		name := strings.Split(fieldType.Tag.Get("config"), ",")[0]
		if name == "-" {
			continue
		}
		fieldPath := append(path, name)
		if field.Kind() == reflect.Struct {
			if bindError := bindEnvironment(field, fieldPath, fileValues); bindError != nil {
				return bindError
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
		if parseError := setEnvironmentValue(field, rawValue); parseError != nil {
			return fmt.Errorf("%s: %w", environmentName, parseError)
		}
	}
	return nil
}

func setEnvironmentValue(field reflect.Value, rawValue string) error {
	switch {
	case field.Type() == durationType:
		value, parseError := time.ParseDuration(rawValue)
		if parseError != nil {
			return fmt.Errorf("must be a duration: %w", parseError)
		}
		field.SetInt(int64(value))
	case field.Type() == logLevelType:
		var value slog.Level
		if parseError := value.UnmarshalText([]byte(strings.ToUpper(rawValue))); parseError != nil {
			return fmt.Errorf("must be debug, info, warn, or error: %w", parseError)
		}
		field.SetInt(int64(value))
	case field.Kind() == reflect.String:
		field.SetString(rawValue)
	case field.Kind() == reflect.Bool:
		value, parseError := strconv.ParseBool(rawValue)
		if parseError != nil {
			return fmt.Errorf("must be a boolean: %w", parseError)
		}
		field.SetBool(value)
	case field.Kind() >= reflect.Int && field.Kind() <= reflect.Int64:
		value, parseError := strconv.ParseInt(rawValue, 10, field.Type().Bits())
		if parseError != nil {
			return fmt.Errorf("must be an integer: %w", parseError)
		}
		field.SetInt(value)
	case field.Kind() == reflect.Slice && field.Type().Elem().Kind() == reflect.String:
		parts := strings.Split(rawValue, ",")
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				values = append(values, part)
			}
		}
		field.Set(reflect.ValueOf(values))
	default:
		return fmt.Errorf("unsupported configuration type %s", field.Type())
	}
	return nil
}
