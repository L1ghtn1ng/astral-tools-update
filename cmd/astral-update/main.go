package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"astral-tools-update/internal/updater"
)

const version = "1.0.0"

func main() {
	logger := log.New(os.Stderr, "", 0)

	var noSelfUpdate bool
	var showVersion bool
	flag.BoolVar(&noSelfUpdate, "no-self-update", false, "Skip updating uv itself")
	flag.BoolVar(&showVersion, "version", false, "Print the program version and exit")
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [--no-self-update] [--version] [tools...]\n\n", os.Args[0])
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), "Update and install Astral tools (uv, ruff, ty, etc.).")
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), "If no tools are provided, defaults to: ruff ty")
		_, _ = fmt.Fprintln(flag.CommandLine.Output())
		flag.PrintDefaults()
	}
	flag.Parse()

	if showVersion {
		_, _ = fmt.Fprintln(os.Stdout, version)
		return
	}

	tools := flag.Args()
	if len(tools) == 0 {
		tools = []string{"ruff", "ty"}
	}

	toolUpdater := updater.NewReal(logger)
	if err := toolUpdater.Update(tools, noSelfUpdate); err != nil {
		logger.Printf("ERROR: %v", err)
		os.Exit(1)
	}
}
