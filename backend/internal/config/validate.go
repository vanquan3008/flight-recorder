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

func (configuration Config) Validate() error {
	var validator validator

	configuration.validateApp(&validator)
	configuration.validateHTTP(&validator)
	configuration.validateDatabase(&validator)
	configuration.validateClickHouse(&validator)
	configuration.validateAuth(&validator)
	configuration.Cache.validate(&validator)
	configuration.validateStorage(&validator)
	configuration.validateQueue(&validator)
	configuration.validateIngestion(&validator)
	configuration.validateObservability(&validator)
	configuration.validateReplay(&validator)
	configuration.validateNotifications(&validator)

	return validator.err()
}
