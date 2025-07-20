# vault-sync

A CLI tool to synchronize secrets between HashiCorp Vault instances.

## Features

- Synchronize secrets between different Vault instances
- Support for dry-run mode to preview changes
- Configuration via file or command-line arguments
- Comprehensive error handling and validation

## Usage

```bash
# Basic usage with command-line arguments
vault-sync --source http://source-vault:8200 --target http://target-vault:8200 --token your-token

# Dry run to see what would be synced
vault-sync --source http://source-vault:8200 --target http://target-vault:8200 --token your-token --dry-run

# Using a configuration file
vault-sync --config config.json
```

## Configuration File

```json
{
  "source": {
    "address": "http://source-vault:8200",
    "token": "source-token",
    "prefix": "secret"
  },
  "target": {
    "address": "http://target-vault:8200", 
    "token": "target-token",
    "prefix": "secret"
  }
}
```

## Build

```bash
go build -o vault-sync .
```

## Test

```bash
go test ./internal/... -v
```