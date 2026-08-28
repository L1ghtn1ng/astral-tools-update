# astral-tools-update
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/L1ghtn1ng/astral-tools-update)

`astral-tools-update` is a small Go CLI for keeping Astral tools up to date through `uv`.
It will:

- check GitHub for a newer `astral-update` release and install the matching Linux package,
- make sure `uv` is available,
- optionally run `uv self update`,
- upgrade tools that are already installed, and
- install missing tools at their latest version.

By default, it updates `ruff`, `ty`, and `zizmor` when you run it without arguments.

## What this repo does

This repository contains a command-line utility at `cmd/astral-update` and the core update logic in `internal/updater`.

Current behavior:

- accepts a list of Astral-managed tools such as `ruff` and `ty`,
- defaults to `ruff ty zizmor` when no tool names are provided,
- checks GitHub for a newer stable `astral-update` release before updating tools,
- selects release assets for the detected Linux package family and CPU architecture,
- looks for `uv` on `PATH` first,
- checks `uv`'s configured tool bin directory when checking for installed tools,
- falls back to `~/.local/bin/<tool>` if that lookup is unavailable,
- attempts to install `uv` automatically if it is missing, and
- rejects invalid tool names before running commands.

## Requirements

- Go `1.27.x` to build from source
- `curl` and `sh` if you want the program to attempt automatic `uv` installation

## Usage

Run the binary with an optional list of tool names:

```bash
astral-update [--no-github-self-update] [--no-self-update] [--version] [tools...]
```

Examples:

```bash
# Default behavior: updates ruff, ty, and zizmor
astral-update

# Update a specific set of tools
astral-update ruff ty

# Skip updating uv itself
astral-update --no-self-update ruff

# Skip checking GitHub for a newer astral-update release
astral-update --no-github-self-update ruff

# Print the program version
astral-update --version
```

What happens during a run:

1. Unless `--no-github-self-update` is set, the program checks GitHub for a newer stable `astral-update` release.
2. If a newer release exists, it selects the release asset matching the Linux CPU architecture and how the running executable was installed, then installs it.
3. The program validates the tool names.
4. It locates `uv` or installs it with Astral's install script.
5. Unless `--no-self-update` is set, it runs `uv self update`.
6. For each requested tool:
   - if the tool already exists, it runs `uv tool upgrade <tool>`
   - if the tool is missing, it runs `uv tool install <tool>@latest`

GitHub check or download failures before installation starts are logged as warnings and do not stop the tool update workflow. If installation starts and fails, the command exits with an error.

## Building manually

### With Go directly

Build the local binary into `bin/astral-update`:

```bash
mkdir -p bin
go build -buildmode=pie -trimpath -ldflags "-s -w" -o bin/astral-update ./cmd/astral-update
```

Local builds report version `dev` and do not self-update. GoReleaser injects the
Git tag into release binaries so the installed version and update comparison
always use the same release identifier.

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

During self-update on Linux, an executable running from the native package path (`/usr/bin/astral-update`) prefers `.deb` on Debian/Ubuntu-like systems and `.rpm` on Fedora/RHEL/SUSE-like systems. Executables running from any other path use the matching `.tar.gz` archive so the currently running installation is replaced directly. The updater also falls back to the archive when no native package format can be detected or matched.

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
