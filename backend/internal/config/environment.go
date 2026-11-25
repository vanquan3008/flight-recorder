package config

import (
	"encoding"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const environmentPrefix = "TRACEPILOT_"

var (
	durationType        = reflect.TypeFor[time.Duration]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

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
	return nil, fmt.Errorf("%s: invalid or unreadable file", path)
}

func environmentValue(name string, fileValues map[string]string) (string, bool) {
	if value, exists := os.LookupEnv(name); exists {
		return value, true
	}
	value, exists := fileValues[name]
	return value, exists
}

func applyEnvironment(configuration *Config, fileValues map[string]string, scopes ...string) error {
	if configuration == nil {
		return errors.New("configuration must not be nil")
	}

	return bindEnvironment(reflect.ValueOf(configuration).Elem(), nil, fileValues, scopes)
}

func bindEnvironment(value reflect.Value, path []string, fileValues map[string]string, scopes []string) error {
	valueType := value.Type()

	for index := range value.NumField() {
		field := value.Field(index)
		fieldType := valueType.Field(index)

		name := configName(fieldType)
		if !fieldType.IsExported() || name == "-" {
			continue
		}

		fieldPath := appendPath(path, name)
		if !scopeSelected(fieldPath, scopes) {
			continue
		}

		if field.Kind() == reflect.Struct && !isScalarStruct(field) {
			if err := bindEnvironment(field, fieldPath, fileValues, scopes); err != nil {
				return err
			}
			continue
		}

		if err := bindField(field, fieldPath, fileValues); err != nil {
			return err
		}
	}

	return nil
}

func bindField(field reflect.Value, path []string, fileValues map[string]string) error {
	name := environmentName(path)

	rawValue, found := environmentValue(name, fileValues)
	if !found {
		return nil
	}

	if err := setValue(field, rawValue); err != nil {
		return fmt.Errorf("%s (%s): %w", strings.Join(path, "."), name, err)
	}

	return nil
}

func configName(field reflect.StructField) string {
	name := strings.TrimSpace(strings.SplitN(field.Tag.Get("config"), ",", 2)[0])
	if name == "" {
		return strings.ToLower(field.Name)
	}
	return name
}

func environmentName(path []string) string {
	return environmentPrefix + strings.ToUpper(strings.Join(path, "_"))
}

func isScalarStruct(field reflect.Value) bool {
	return field.Addr().Type().Implements(textUnmarshalerType)
}

func scopeSelected(path, scopes []string) bool {
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

func appendPath(path []string, name string) []string {
	result := make([]string, len(path), len(path)+1)
	copy(result, path)
	return append(result, name)
}

func setValue(field reflect.Value, rawValue string) error {
	trimmed := strings.TrimSpace(rawValue)
	fieldType := field.Type()

	switch {
	case fieldType == durationType:
		value, err := time.ParseDuration(trimmed)
		if err != nil {
			return errors.New("expected a duration such as 500ms, 30s or 1h")
		}
		field.SetInt(int64(value))

	case field.Addr().Type().Implements(textUnmarshalerType):
		unmarshaler := field.Addr().Interface().(encoding.TextUnmarshaler)
		if err := unmarshaler.UnmarshalText([]byte(trimmed)); err != nil {
			return fmt.Errorf("expected a valid %s", fieldType)
		}

	case field.Kind() == reflect.String:
		field.SetString(rawValue)

	case field.Kind() == reflect.Bool:
		value, err := strconv.ParseBool(trimmed)
		if err != nil {
			return errors.New("expected a boolean")
		}
		field.SetBool(value)

	case field.CanInt():
		value, err := strconv.ParseInt(trimmed, 10, fieldType.Bits())
		if err != nil {
			return fmt.Errorf("expected a %d-bit integer", fieldType.Bits())
		}
		field.SetInt(value)

	case field.CanUint():
		value, err := strconv.ParseUint(trimmed, 10, fieldType.Bits())
		if err != nil {
			return fmt.Errorf("expected a %d-bit unsigned integer", fieldType.Bits())
		}
		field.SetUint(value)

	case field.CanFloat():
		value, err := strconv.ParseFloat(trimmed, fieldType.Bits())
		if err != nil {
			return errors.New("expected a number")
		}
		field.SetFloat(value)

	case field.Kind() == reflect.Slice && fieldType.Elem().Kind() == reflect.String:
		values := parseList(rawValue)
		slice := reflect.MakeSlice(fieldType, len(values), len(values))
		for index, value := range values {
			slice.Index(index).SetString(value)
		}
		field.Set(slice)

	default:
		return fmt.Errorf("unsupported configuration type %s", fieldType)
	}

	return nil
}

func parseList(rawValue string) []string {
	values := []string{}
	for part := range strings.SplitSeq(rawValue, ",") {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}
