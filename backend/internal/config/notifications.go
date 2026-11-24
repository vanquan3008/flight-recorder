package config

import (
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/notification"
)

type NotificationsConfig struct {
	Enabled        bool          `config:"enabled"`
	EncryptionKey  string        `config:"encryption_key"`
	PollInterval   time.Duration `config:"poll_interval"`
	Workers        int           `config:"workers"`
	RequestTimeout time.Duration `config:"request_timeout"`
}

func (configuration Config) validateNotifications(validator *validator) {
	notifications := configuration.Notifications
	if !notifications.Enabled {
		return
	}

	if _, err := notification.DecodeEncryptionKey(notifications.EncryptionKey); err != nil {
		validator.add("notifications.encryption_key", err.Error())
	}
	if notifications.PollInterval <= 0 {
		validator.add("notifications.poll_interval", "must be positive")
	}
	if notifications.Workers <= 0 || notifications.Workers > 64 {
		validator.add("notifications.workers", "must be between 1 and 64")
	}
	if notifications.RequestTimeout <= 0 || notifications.RequestTimeout > 5*time.Minute {
		validator.add("notifications.request_timeout", "must be between 1 and 5 minutes")
	}
}
