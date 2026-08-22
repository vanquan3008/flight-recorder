package main

import (
	"fmt"
	"os"
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
	if len(args) == 0 {
		usage()
		return 2
	}

	switch args[0] {
	case "serve":
		return runServer(args[1:])
	case "migrate":
		return runMigrations(args[1:])
	case "version":
		fmt.Printf("Flight %s\ncommit: %s\nbuilt: %s\n", version, commit, buildTime)
		return 0
	case "healthcheck":
		return healthcheck()
	default:
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: flight <serve|migrate|healthcheck|version>")
}
