package config

import "time"

type AppConfig struct {
	Production      bool          `config:"production"`
	ShutdownTimeout time.Duration `config:"shutdown_timeout"`
	TLS             TLSConfig     `config:"tls"`
}

type TLSConfig struct {
	CertificateFile string `config:"certificate_file"`
	PrivateKeyFile  string `config:"private_key_file"`
}

func (configuration Config) validateApp(validator *validator) {
	app := configuration.App

	if (app.TLS.CertificateFile == "") != (app.TLS.PrivateKeyFile == "") {
		validator.add("app.tls", "certificate_file and private_key_file must be set together")
	}
	if app.ShutdownTimeout <= 0 {
		validator.add("app.shutdown_timeout", "must be positive")
	}
}
