package config

import (
	"errors"
	"strings"
)

type validator struct {
	errs []string
}

func (validator *validator) add(field, reason string) {
	validator.errs = append(validator.errs, field+": "+reason)
}

func (validator *validator) err() error {
	if len(validator.errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(validator.errs, "; "))
}

// Validate checks the configuration for semantic errors, invalid ranges and production prerequisites.
func (configuration Config) Validate() error {
	var validator validator

	configuration.validateApp(&validator)
	configuration.validateHTTP(&validator)
	configuration.validateDatabase(&validator)
	configuration.validateClickHouse(&validator)
	configuration.validateAuth(&validator)
	configuration.validateCache(&validator)
	configuration.validateStorage(&validator)
	configuration.validateQueue(&validator)
	configuration.validateObservability(&validator)

	return validator.err()
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
