# Agent Hub CLI Test Matrix

This matrix maps every user-facing `agenthub` command to its automated coverage layer.

- `L1`: command surface, usage, exit codes
- `L2`: SQLite local CLI integration with a real built `agenthub` binary
- `L3`: platform adapter contract tests with isolated HOME and shim binaries

## Root Commands

| Command | Coverage | Primary test files | Real execution | Platform shim |
| --- | --- | --- | --- | --- |
| `agenthub status` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/daemon_integration_test.go` | Yes | No |
| `agenthub doctor` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/daemon_integration_test.go` | Yes | No |
| `agenthub platform ls` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go` | Yes | Yes |
| `agenthub platform show <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `agenthub ls` | L2 | `internal/cli/platform_integration_test.go` | Yes | Yes |
| `agenthub ls <platform>` | L2 | `internal/cli/platform_integration_test.go` | Yes | Yes |
| `agenthub connect <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `agenthub disconnect <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `agenthub import <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `agenthub export <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `agenthub daemon status` | L2 | `internal/cli/daemon_integration_test.go` | Yes | No |
| `agenthub daemon stop` | L2 | `internal/cli/daemon_integration_test.go` | Yes | No |
| `agenthub daemon logs` | L2 | `internal/cli/daemon_integration_test.go` | Yes | No |
| `agenthub server` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/server_mcp_integration_test.go` | Yes | No |
| `agenthub mcp stdio` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/server_mcp_integration_test.go` | Yes | No |

## `agenthub sync`

| Command | Coverage | Primary test files | Real execution | Platform shim |
| --- | --- | --- | --- | --- |
| `agenthub sync login` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub sync profiles` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub sync use` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub sync whoami` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub sync logout` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub sync export` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub sync preview` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub sync push` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub sync pull` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub sync resume` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub sync history` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub sync diff` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |

## `agenthub remote`

| Command | Coverage | Primary test files | Real execution | Platform shim |
| --- | --- | --- | --- | --- |
| `agenthub remote ls` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub remote login` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub remote use` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub remote logout` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `agenthub remote whoami` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |

## Platform Adapters

| Platform | Coverage | Contract file | Detect | Connect/Disconnect | Import/Export |
| --- | --- | --- | --- | --- | --- |
| `claude-code` | L2, L3 | `internal/platforms/adapter_contract_test.go` | Yes | Yes | Yes |
| `codex` | L2, L3 | `internal/platforms/adapter_contract_test.go` | Yes | Yes | Yes |
| `gemini-cli` | L2, L3 | `internal/platforms/adapter_contract_test.go` | Yes | Yes | Yes |
| `cursor-agent` | L2, L3 | `internal/platforms/adapter_contract_test.go` | Yes | Yes | Yes |

## Notes

- L1/L2/L3 are designed to run under `go test ./...`.
- All platform-facing tests use isolated HOME/XDG-style directories and fixture data under `internal/platforms/testdata/`.
- No L1/L2/L3 test depends on real user data or writes to a live platform configuration on purpose.
