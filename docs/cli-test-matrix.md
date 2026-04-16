# neuDrive CLI Test Matrix

This matrix maps every user-facing `neu` command to its automated coverage layer. The compatible `neudrive` alias follows the same coverage.

- `L1`: command surface, usage, exit codes
- `L2`: SQLite local CLI integration with a real built `neudrive` binary
- `L3`: platform adapter contract tests with isolated HOME and shim binaries

## Root Commands

| Command | Coverage | Primary test files | Real execution | Platform shim |
| --- | --- | --- | --- | --- |
| `neu status` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/daemon_integration_test.go` | Yes | No |
| `neu doctor` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/daemon_integration_test.go` | Yes | No |
| `neu platform ls` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go` | Yes | Yes |
| `neu platform show <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `neu ls` | L2 | `internal/cli/platform_integration_test.go` | Yes | Yes |
| `neu ls <platform>` | L2 | `internal/cli/platform_integration_test.go` | Yes | Yes |
| `neu connect <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `neu disconnect <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `neu import <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `neu export <platform>` | L1, L2, L3 | `internal/cli/root_commands_test.go`, `internal/cli/platform_integration_test.go`, `internal/platforms/adapter_contract_test.go` | Yes | Yes |
| `neu login` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neu logout` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neu use` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neu whoami` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neu profiles` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neu daemon status` | L2 | `internal/cli/daemon_integration_test.go` | Yes | No |
| `neu daemon stop` | L2 | `internal/cli/daemon_integration_test.go` | Yes | No |
| `neu daemon logs` | L2 | `internal/cli/daemon_integration_test.go` | Yes | No |
| `neu server` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/server_mcp_integration_test.go` | Yes | No |
| `neu mcp stdio` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/server_mcp_integration_test.go` | Yes | No |

## `neu sync`

| Command | Coverage | Primary test files | Real execution | Platform shim |
| --- | --- | --- | --- | --- |
| `neu sync export` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neu sync preview` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neu sync push` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neu sync pull` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neu sync resume` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neu sync history` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |
| `neu sync diff` | L1, L2 | `internal/cli/root_commands_test.go`, `internal/cli/sync_integration_test.go` | Yes | No |

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
