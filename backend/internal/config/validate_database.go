package config

import "errors"

func (configuration Config) validateDatabaseSessionLimits() error {
	database := configuration.Database
	if database.StatementTimeout < 0 || database.LockTimeout < 0 || database.IdleInTransactionTimeout < 0 {
		return errors.New("database.statement_timeout, lock_timeout and idle_in_transaction_timeout must not be negative (0 keeps the server default)")
	}
	if database.StatementTimeout > 0 && configuration.HTTP.RequestTimeout > 0 && database.StatementTimeout < configuration.HTTP.RequestTimeout {
		return errors.New("database.statement_timeout must not be below http.request_timeout, or queries are cut off before the API can answer")
	}
	return nil
}
