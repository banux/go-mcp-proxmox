# Project Context

## Purpose

`go-mcp-proxmox` is an MCP (Model Context Protocol) server written in Go that exposes the Proxmox VE REST API as MCP tools. It lets AI assistants (Claude, etc.) inspect and manage a Proxmox cluster: nodes, VMs (QEMU), containers (LXC), storage, tasks, etc.

## Tech Stack

- **Language**: Go (latest stable)
- **MCP**: official Go SDK `github.com/modelcontextprotocol/go-sdk` (stdio transport first)
- **HTTP client**: standard library `net/http` against the Proxmox VE API (`/api2/json`)
- **Config**: environment variables (no config file required)

## Authentication

- **API token only** (no username/password, no ticket/cookie flow)
- Header: `Authorization: PVEAPIToken=USER@REALM!TOKENID=UUID`
- Configured via environment variables, e.g.:
  - `PROXMOX_URL` — base URL of the Proxmox API (e.g. `https://pve.example.com:8006`)
  - `PROXMOX_TOKEN_ID` — `USER@REALM!TOKENID`
  - `PROXMOX_TOKEN_SECRET` — the token UUID
  - `PROXMOX_INSECURE_TLS` — optional, allow self-signed certificates (default: false)
- Secrets must never be logged or returned in tool output

## Project Conventions

- Idiomatic Go: `gofmt`, `go vet`, errors wrapped with `%w`
- Standard layout: `cmd/` for the binary entrypoint, `internal/` for packages (Proxmox client, MCP tools)
- Table-driven tests with the standard `testing` package; Proxmox API mocked with `httptest`
- Read-only tools first (list/status), then mutating tools (start/stop/create) gated carefully
- Tool names follow MCP conventions: snake_case, verb-first (e.g. `list_vms`, `get_node_status`, `start_vm`)

## Domain Context

- Proxmox VE API reference: https://pve.proxmox.com/pve-docs/api-viewer/
- All endpoints live under `/api2/json/...`; responses wrap payloads in a `data` field
- Key resources: `/nodes`, `/nodes/{node}/qemu`, `/nodes/{node}/lxc`, `/storage`, `/cluster/resources`, `/nodes/{node}/tasks`
- Mutating operations return a task UPID that can be polled for completion

- Minimum supported Proxmox VE version: 8.x
- Repository: https://github.com/banux/go-mcp-proxmox — Go module `github.com/banux/go-mcp-proxmox`

## Important Constraints

- Token permissions are defined server-side in Proxmox (RBAC); the MCP server must surface permission errors clearly, not work around them
- TLS verification on by default; opt-out only via explicit env var
- Destructive operations (delete VM, etc.) should be opt-in / clearly separated from read-only tools
