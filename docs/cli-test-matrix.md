# neuDrive CLI Test Matrix

This matrix maps every user-facing `neudrive` command to its automated coverage layer.

- `L1`: command surface, usage, exit codes
- `L2`: SQLite local CLI integration with a real built `neudrive` binary
- `L3`: platform adapter contract tests with isolated HOME and shim binaries

## Root Commands

| Command | Coverage | Primary test files | Real execution | Platform shim |
| --- | --- | --- | --- | --- |
| `neudrive status` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/daemon_integration_test.go` | Yes | No |
| `neudrive doctor` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/daemon_integration_test.go` | Yes | No |
| `neudrive platform ls` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go` | Yes | Yes |
| `neudrive platform show <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `neudrive ls` | L2 | `internal/cli/platform_integration_test.go` | Yes | Yes |
| `neudrive ls <platform>` | L2 | `internal/cli/platform_integration_test.go` | Yes | Yes |
| `neudrive connect <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `neudrive disconnect <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `neudrive import <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `neudrive export <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `neudrive login` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neudrive logout` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neudrive use` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neudrive whoami` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neudrive profiles` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neudrive daemon status` | L2 | `internal/cli/daemon_integration_test.go` | Yes | No |
| `neudrive daemon stop` | L2 | `internal/cli/daemon_integration_test.go` | Yes | No |
| `neudrive daemon logs` | L2 | `internal/cli/daemon_integration_test.go` | Yes | No |
| `neudrive server` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/server_mcp_integration_test.go` | Yes | No |
| `neudrive mcp stdio` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/server_mcp_integration_test.go` | Yes | No |

## `neudrive sync`

| Command | Coverage | Primary test files | Real execution | Platform shim |
| --- | --- | --- | --- | --- |
| `neudrive sync export` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neudrive sync preview` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neudrive sync push` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neudrive sync pull` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neudrive sync resume` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neudrive sync history` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neudrive sync diff` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |

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
