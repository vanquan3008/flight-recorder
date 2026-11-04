package config

import (
	"errors"
	"log/slog"
	"strings"
)

type validator struct {
	errs []string
}

func (v *validator) add(field, reason string) {
	slog.Error("invalid configuration", "field", field, "reason", reason)
	v.errs = append(v.errs, field+": "+reason)
}

func (v *validator) err() error {
	if len(v.errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(v.errs, "; "))
}

// Validate checks the configuration for semantic errors, invalid ranges and production prerequisites.
func (configuration Config) Validate() error {
	var v validator

	configuration.validateApp(&v)
	configuration.validateHTTP(&v)
	configuration.validateDatabase(&v)
	configuration.validateAuth(&v)
	configuration.validateCache(&v)
	configuration.validateStorage(&v)
	configuration.validateQueue(&v)
	configuration.validateObservability(&v)

	return v.err()
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	nonZero := false
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return false
		}
		if character != '0' {
			nonZero = true
		}
	}
	return nonZero
}
