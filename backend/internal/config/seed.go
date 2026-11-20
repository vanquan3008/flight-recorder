package config

// SeedConfig contains settings for bootstrapping a development administrator account.
type SeedConfig struct {
	AdminEmail    string `config:"admin_email" env:"TRACEPILOT_SEED_ADMIN_EMAIL"`
	AdminPassword string `config:"admin_password" env:"TRACEPILOT_SEED_ADMIN_PASSWORD"`
	AdminName     string `config:"admin_name" env:"TRACEPILOT_SEED_ADMIN_NAME"`
}
