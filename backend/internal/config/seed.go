package config

// SeedConfig contains settings for bootstrapping a development administrator account.
type SeedConfig struct {
	AdminEmail    string `config:"admin_email"`
	AdminPassword string `config:"admin_password"`
	AdminName     string `config:"admin_name"`
}
