# Rotki Sync

A CLI tool for syncing rotki data from various sources.

## Overview

Rotki Sync is a Go CLI tool that interacts with the rotki-core API to perform various synchronization tasks:

- Fetch and process balances (take a snapshot if needed)
- Fetch and decode EVM transactions
- Fetch staking events
- Fetch exchange trades
- Download the latest rotki-core binary
- Create backups of rotki's data directory

## Installation

### Prerequisites

- Go 1.24 or later
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

### Development

#### Setup

```bash
# Download golangci-lint (if not already installed)
make download-golangci-lint

# Tidy Go modules
make mod-tidy
```

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

- **Lint**: Runs golangci-lint to ensure code quality
- **Test**: Runs all Go tests and generates coverage reports
- **Build**: Builds the application for multiple platforms (Linux, Windows, macOS)

The CI pipeline is triggered on push and pull requests to the main branch, but only when Go-related files are changed. TypeScript code is ignored by the CI pipeline.

To see the current CI status, check the Actions tab in the GitHub repository.

## Usage

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
# Run the sync process with default settings
./rotki-sync

# Run with custom settings
./rotki-sync --port 59002 --bin-path /path/to/rotki-core
```

### Environment Variables

For each user in rotki, you need to set an environment variable with the password:

```bash
export USERNAME_PASSWORD=your_password
```

Replace `USERNAME` with the actual username in uppercase.

### Command Line Options

#### Global Options

- `--port, -p`: Port to run rotki-core on (default: 59001)
- `--bin-path, -b`: Path to rotki-core binary (default: bin/rotki-core)
- `--data-dir`: Directory where rotki's data resides. (default: depends on the system)
- `--max-retries, -r`: Maximum number of balance fetch retries (default: 10)
- `--retry-delay, -d`: Delay between retries in milliseconds (default: 2000)
- `--api-ready-timeout, -t`: Maximum attempts to check API readiness (default: 30)

#### Backup Command Options

- `--backup-dir`: Directory where the backup will be stored (default: ~/backups)

## Project Structure

- `cmd/sync`: Main CLI entry point
- `internal/models`: Data models for API requests and responses
- `internal/utils`: Utility functions for HTTP requests and validation
- `internal/blockchain`: Functionality for interacting with blockchain data
- `internal/download`: Functionality for downloading the rotki-core binary
- `internal/backup`: Functionality for creating backups of rotki's data directory

## License

[MIT](./LICENSE.md)
