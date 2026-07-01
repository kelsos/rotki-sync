# Rotki Sync

A CLI tool for syncing rotki data from various sources.

## Overview

Rotki Sync is a Go CLI tool that interacts with the rotki-core API to perform various synchronization tasks:

- Fetch and process balances (take a snapshot if needed)
- Fetch and decode transactions (EVM and Bitcoin)
- Fetch staking and other online events
- Fetch exchange trades
- Download the latest rotki-core binary
- Create backups of rotki's data directory
- Store user login passwords in an age-encrypted secret store
- Run unattended on a schedule via a systemd `--user` timer

## Installation

### Prerequisites

- Go 1.26 or later (the exact toolchain is pinned in `go.mod`)
- rotki-core binary (can be downloaded using the CLI)

### Building from Source

```bash
# Clone the repository
git clone https://github.com/kelsos/rotki-sync.git
cd rotki-sync

# Build the CLI
go build -o rotki-sync ./cmd/sync

# Or use the Makefile
make build
```

### Installing

```bash
# Build and install to ~/.local/bin (override with INSTALL_DIR=...)
make install

# Remove it again
make uninstall
```

### Data Directory

Downloaded binaries, logs, and the secret store live under a single data home,
resolved in this order:

1. `ROTKI_SYNC_HOME`
2. `$XDG_DATA_HOME/rotki-sync`
3. `~/.local/share/rotki-sync`

The layout is `<home>/bin` (rotki-core), `<home>/logs`, and
`<home>/secrets.age`. Set `ROTKI_SYNC_HOME=<repo>` if you want the old
run-from-the-checkout behavior.

### Development

#### Setup

```bash
# Download golangci-lint (if not already installed)
make download-golangci-lint

# Tidy Go modules
make mod-tidy

# Enable the git pre-commit/pre-push hooks (one-time per clone)
make hooks
```

`make hooks` points `core.hooksPath` at `.githooks/`, enabling:

- **pre-commit**: `gofmt -s` check on staged Go files, then `go vet`.
- **pre-push**: `make lint` (skipped with a warning if golangci-lint is absent),
  `make test`, and `make build` — mirroring CI.

Both are bypassable with `--no-verify`; disable with
`git config --unset core.hooksPath`.

#### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Run tests
make test

# Generate test coverage report
make coverage
```

#### Build Options

```bash
# Build for Linux
make build-linux

# Build for Windows
make build-windows

# Build for macOS
make build-darwin
```

#### Available Make Commands

Run `make help` to see all available commands.

## Continuous Integration

This project uses GitHub Actions for continuous integration:

- **Lint**: Runs golangci-lint (via the pinned `golangci-lint-action`).
- **Test**: Runs all Go tests and generates coverage reports.
- **Build**: Builds the application for multiple platforms (Linux, Windows, macOS).

The pipeline runs on push and pull requests to `main`, but only when
Go-related files change. The Go toolchain is pinned via `go.mod`
(`go-version-file`), and every action is pinned to a commit SHA. Renovate keeps
dependencies and action digests current on a 7-day cooldown.

To see the current CI status, check the Actions tab in the GitHub repository.

## Usage

### Storing User Passwords

Login passwords are kept in an age-encrypted store — there is no
`USERNAME_PASSWORD` environment variable. Before the first sync each user must
have a stored password, or login will fail.

```bash
# Create the store and its age identity (one-time)
./rotki-sync secret init

# Store or update a user's password (prompts with no echo if omitted)
./rotki-sync secret set <username>

# List users with a stored password (names only)
./rotki-sync secret list

# Remove a user's password
./rotki-sync secret rm <username>

# Verify stored passwords authenticate against a live rotki-core
./rotki-sync secret check
```

The age identity is resolved from `ROTKI_SYNC_AGE_KEY`, then the OS keyring,
then `<data-home>/identity.key`. Secrets are stripped from the rotki-core child
process environment.

### Downloading rotki-core

```bash
# Download the latest rotki-core binary
./rotki-sync download
```

### Creating a Backup

```bash
# Create a backup of rotki's data directory with default settings
./rotki-sync backup

# Create a backup with custom data and backup directories
./rotki-sync backup --data-dir /path/to/rotki/data --backup-dir /path/to/backup/location
```

### Running the Sync Process

```bash
# Run the sync process with default settings (interactive TUI)
./rotki-sync

# Run without the TUI (for scripts and timers)
./rotki-sync --no-tui

# Run with custom settings
./rotki-sync --port 59002 --bin-path /path/to/rotki-core
```

On completion of a non-interactive run, a desktop notification is sent via
`notify-send` (best-effort). Failures also trigger a webhook if
`ROTKI_SYNC_ALERT_WEBHOOK` is set.

### Preflight Check

```bash
# Boot rotki-core and verify the environment without running a full sync
./rotki-sync preflight
```

### Scheduling (systemd --user timer)

```bash
# Install and enable a daily timer (default 09:30)
./rotki-sync service install

# Use a custom OnCalendar expression
./rotki-sync service install --schedule '*-*-* 06:00:00'

# Disable and remove the timer
./rotki-sync service uninstall
```

The unit files are written to `$XDG_CONFIG_HOME/systemd/user`. The timer is
`Persistent=true` and runs after login/unlock (no lingering).

### Shell Completion

```bash
# Install completion for your shell
./rotki-sync completion install [bash|zsh|fish]
```

### Version

```bash
# Print the build version, commit, and date
./rotki-sync version
```

### Command Line Options

#### Global / Sync Options

- `--port, -p`: Port to run rotki-core on (default: 59001)
- `--bin-path, -b`: Path to rotki-core binary (default: under the data home's `bin/` directory)
- `--data-dir`: Directory where rotki's data resides (default: depends on the system)
- `--max-retries, -r`: Maximum number of balance fetch retries (default: 10)
- `--retry-delay, -d`: Delay between retries in milliseconds (default: 2000)
- `--api-ready-timeout, -t`: Maximum attempts to check API readiness (default: 30)
- `--no-tui`: Disable the interactive TUI monitoring mode
- `--yes, -y`: Skip the rotki-core version confirmation prompt

#### Backup Command Options

- `--backup-dir`: Directory where the backup will be stored (default: ~/backups)

### Environment Variables

- `ROTKI_SYNC_HOME`: Override the data home (bin/logs/secrets).
- `ROTKI_SYNC_AGE_KEY`: age identity used to decrypt the secret store.
- `ROTKI_SYNC_ALERT_WEBHOOK`: URL notified on a failed run.
- `ROTKI_SYNC_LOG_KEEP`: Number of per-run logs to retain (default: 20, `0` disables pruning).

## Project Structure

- `cmd/sync`: Main CLI entry point and commands
- `internal/client`: HTTP client for the rotki-core API
- `internal/services`: Sync logic (balances, transactions, online events)
- `internal/models`: Data models for API requests and responses
- `internal/secrets`: age-encrypted password store
- `internal/paths`: XDG-aware data-home resolution
- `internal/progress`: Live decode/rate-limit progress via websocket + log tail
- `internal/process`: rotki-core process lifecycle management
- `internal/download`: Downloading the rotki-core binary
- `internal/backup`: Creating backups of rotki's data directory
- `internal/tui`: Interactive terminal UI

## License

[MIT](./LICENSE.md)
