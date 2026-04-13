# astral-tools-update
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/L1ghtn1ng/astral-tools-update)

`astral-tools-update` is a small Go CLI for keeping Astral tools up to date through `uv`.
It will:

- make sure `uv` is available,
- optionally run `uv self update`,
- upgrade tools that are already installed, and
- install missing tools at their latest version.

By default, it updates `ruff` and `ty` when you run it without arguments.

## What this repo does

This repository contains a command-line utility at `cmd/astral-update` and the core update logic in `internal/updater`.

Current behavior:

- accepts a list of Astral-managed tools such as `ruff` and `ty`,
- defaults to `ruff ty` when no tool names are provided,
- looks for `uv` on `PATH` first,
- checks `uv`'s configured tool bin directory when checking for installed tools,
- falls back to `~/.local/bin/<tool>` if that lookup is unavailable,
- attempts to install `uv` automatically if it is missing, and
- rejects invalid tool names before running commands.

## Requirements

- Go `1.26` to build from source
- `curl` and `sh` if you want the program to attempt automatic `uv` installation

## Usage

Run the binary with an optional list of tool names:

```bash
astral-update [--no-self-update] [--version] [tools...]
```

Examples:

```bash
# Default behavior: updates ruff and ty
astral-update

# Update a specific set of tools
astral-update ruff ty

# Skip updating uv itself
astral-update --no-self-update ruff

# Print the program version
astral-update --version
```

What happens during a run:

1. The program validates the tool names.
2. It locates `uv` or installs it with Astral's install script.
3. Unless `--no-self-update` is set, it runs `uv self update`.
4. For each requested tool:
   - if the tool already exists, it runs `uv tool upgrade <tool>`
   - if the tool is missing, it runs `uv tool install <tool>@latest`

## Building manually

### With Go directly

Build the local binary into `bin/astral-update`:

```bash
mkdir -p bin
go build -ldflags "-s -w" -o bin/astral-update ./cmd/astral-update
```

### With `make`

The repository includes a `Makefile` with common development tasks:

```bash
make build            # build bin/astral-update
make test             # run go test -v ./...
make ci               # run format check, vet, and tests
make build-linux      # build Linux amd64 + arm64 binaries
make clean            # remove bin/
```

Linux cross-build outputs:

- `bin/astral-update_x86_64`
- `bin/astral-update_arm64`

## Running from source

You can also run it without producing a binary first:

```bash
go run ./cmd/astral-update --no-self-update ruff
```

## Project layout

- `cmd/astral-update/main.go` — CLI entrypoint and flag parsing
- `internal/updater/updater.go` — update workflow, command execution, and environment checks
- `internal/updater/updater_test.go` — unit tests for success paths and edge cases
- `Makefile` — local development and build helpers
- `.github/workflows/ci.yml` — CI build and test workflow
- `.github/workflows/release.yml` — tagged release workflow
- `.goreleaser.yaml` — Linux archives and package release configuration

## Release notes

This repo is configured to publish releases from Git tags using GoReleaser.
The release pipeline currently targets:

- Linux `amd64`
- Linux `arm64`
- archive output as `.tar.gz`
- package output as `.deb` and `.rpm`

## Helpful notes

- Automatic `uv` installation uses: `curl -LsSf https://astral.sh/uv/install.sh | sh`
- Installed tools may be detected from `PATH`, from `uv tool dir --bin`, or from `~/.local/bin`
- The repository ignores generated build output such as `bin/` and `dist/`
- CI uses `make build` and `make ci`, so keeping those targets green is a good local check before pushing changes

## Development

For day-to-day work, a typical loop is:

```bash
make fmt
make ci
make build
```

If you change the updater behavior, update or add tests in `internal/updater/updater_test.go` so the documented behavior stays accurate.
