package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"astral-tools-update/internal/selfupdate"
	"astral-tools-update/internal/updater"
)

const version = "1.1.0"

func main() {
	logger := log.New(os.Stderr, "", 0)

	var noSelfUpdate bool
	var noGithubSelfUpdate bool
	var showVersion bool
	flag.BoolVar(&noSelfUpdate, "no-self-update", false, "Skip updating uv itself")
	flag.BoolVar(&noGithubSelfUpdate, "no-github-self-update", false, "Skip checking GitHub for a newer astral-update release")
	flag.BoolVar(&showVersion, "version", false, "Print the program version and exit")
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [--no-github-self-update] [--no-self-update] [--version] [tools...]\n\n", os.Args[0])
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), "Update and install Astral tools (uv, ruff, ty, etc.).")
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), "If no tools are provided, defaults to: ruff ty zizmor")
		_, _ = fmt.Fprintln(flag.CommandLine.Output())
		flag.PrintDefaults()
	}
	flag.Parse()

	if showVersion {
		_, _ = fmt.Fprintln(os.Stdout, version)
		return
	}

	if !noGithubSelfUpdate {
		selfUpdater := selfupdate.New(version, logger)
		result, err := selfUpdater.CheckAndInstall(context.Background())
		if err != nil {
			if result.InstallStarted {
				logger.Printf("ERROR: GitHub self-update failed: %v", err)
				os.Exit(1)
			}
			logger.Printf("WARN: GitHub self-update skipped: %v", err)
		}
	}

	tools := flag.Args()
	if len(tools) == 0 {
		tools = []string{"ruff", "ty", "zizmor"}
	}

	toolUpdater := updater.NewReal(logger)
	if err := toolUpdater.Update(tools, noSelfUpdate); err != nil {
		logger.Printf("ERROR: %v", err)
		os.Exit(1)
	}
}
