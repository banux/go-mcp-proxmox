## Why

The project is empty: there is no Go module, no MCP server, and no way to talk to a Proxmox VE cluster. This change lays the foundation — an MCP server in Go backed by a Proxmox API client authenticated by API token — so that AI assistants can start inspecting a Proxmox cluster and later changes can add more tools on top of a solid base.

## What Changes

- Initialize the Go module and standard project layout (`cmd/`, `internal/`)
- Add a Proxmox VE API client (`internal/proxmox`) using `net/http`:
  - API token authentication via the `Authorization: PVEAPIToken=...` header (no ticket/cookie flow)
  - Configuration from environment variables (`PROXMOX_URL`, `PROXMOX_TOKEN_ID`, `PROXMOX_TOKEN_SECRET`, `PROXMOX_INSECURE_TLS`)
  - TLS verification on by default, opt-out via explicit env var
  - Unwrapping of the Proxmox `data` response envelope and clear error mapping (401/403 permission errors surfaced as-is)
- Add the MCP server entrypoint (`cmd/go-mcp-proxmox`) using the official `github.com/modelcontextprotocol/go-sdk` with stdio transport
- Register the first set of **read-only** MCP tools: `list_nodes`, `get_node_status`, `list_vms`, `list_lxc`, `list_storage`, `get_cluster_resources`
- Add table-driven tests with `httptest` mocking the Proxmox API

No mutating tools (start/stop/create/delete) in this change — they come later, gated separately.

## Capabilities

### New Capabilities
- `proxmox-client`: HTTP client for the Proxmox VE API — token authentication, env-based configuration, TLS policy, response envelope handling, error mapping
- `mcp-server`: MCP server lifecycle — stdio transport, startup validation of configuration, tool registration
- `proxmox-read-tools`: read-only MCP tools exposing cluster inventory and status (nodes, VMs, LXC, storage, cluster resources)

### Modified Capabilities

(none — first change of the project)

## Impact

- New Go module `github.com/banux/go-mcp-proxmox` (go.mod, go.sum)
- New directories: `cmd/go-mcp-proxmox/`, `internal/proxmox/`, `internal/tools/`
- New dependency: `github.com/modelcontextprotocol/go-sdk`
- No existing code affected (empty repository)
- Requires a Proxmox VE API token with at least `PVEAuditor`-level permissions to use the tools
