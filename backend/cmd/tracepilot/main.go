package main

import (
	"log/slog"
	"os"

	"github.com/vanquan3008/tracepilot/internal/observability/logging"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	slog.SetDefault(logging.NewLogger(os.Stdout, &logging.Options{ServiceName: "tracepilot", ServiceVersion: version}))
	if len(args) == 0 {
		slog.Error("missing sub-command", "usage", "tracepilot <serve|migrate|create-admin|healthcheck|version>")
		return 2
	}
	switch args[0] {
	case "serve":
		return runServer(args[1:])
	case "migrate":
		return runMigrations(args[1:])
	case "create-admin":
		return runCreateAdmin(args[1:], os.Stdin)
	case "version":
		slog.Info("TracePilot version", "version", version, "commit", commit, "built", buildTime)
		return 0
	case "healthcheck":
		return healthcheck()
	default:
		slog.Error("unknown sub-command", "subcommand", args[0], "usage", "tracepilot <serve|migrate|create-admin|healthcheck|version>")
		return 2
	}
}
