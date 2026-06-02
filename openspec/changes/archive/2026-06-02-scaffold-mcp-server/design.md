## Context

Empty repository. The goal is an MCP server in Go that exposes the Proxmox VE REST API (`/api2/json`) as MCP tools, authenticated exclusively by Proxmox API token. This change scaffolds the whole vertical slice: configuration → HTTP client → MCP server → first read-only tools, so later changes only add tools.

Constraints (from `openspec/project.md`):
- Token auth only, header `Authorization: PVEAPIToken=USER@REALM!TOKENID=UUID`
- Config via environment variables, no config file
- TLS verification on by default
- Secrets never logged or returned in tool output

## Goals / Non-Goals

**Goals:**
- Working binary `go-mcp-proxmox` speaking MCP over stdio
- Reusable Proxmox client in `internal/proxmox` decoupled from MCP concerns
- Six read-only tools covering cluster inventory: `list_nodes`, `get_node_status`, `list_vms`, `list_lxc`, `list_storage`, `get_cluster_resources`
- Fail-fast startup: invalid/missing config aborts with a clear message before serving
- Tests that don't require a real Proxmox cluster

**Non-Goals:**
- Mutating tools (start/stop/create/delete) — later change
- HTTP/SSE transport — stdio only for now
- Ticket/cookie (username+password) authentication
- Caching, rate limiting, or task polling helpers

## Decisions

### 1. Official MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`)
Use the official SDK with `mcp.NewServer` + stdio transport.
- *Alternative considered*: `mark3labs/mcp-go` — more popular historically, but the official SDK is now maintained by Anthropic/Google and is the long-term safe bet. Typed tool handlers with generated JSON schemas from Go structs.

### 2. Hand-rolled Proxmox client over `net/http`
A thin client in `internal/proxmox` with a single `do(ctx, method, path, query) ([]byte, error)` core plus typed methods (`ListNodes`, `ListQemu`, ...).
- *Alternative considered*: existing libs (`luthermonson/go-proxmox`, Telmate's client) — they pull in large APIs, mix auth flows, and hide the request layer; we need only a handful of GET endpoints and full control over auth headers and error mapping.
- The Proxmox `{"data": ...}` envelope is unwrapped in `do`; typed methods decode into structs.

### 3. Configuration: env vars parsed at startup into a `Config` struct
`LoadConfig()` reads `PROXMOX_URL`, `PROXMOX_TOKEN_ID`, `PROXMOX_TOKEN_SECRET`, `PROXMOX_INSECURE_TLS`, validates (URL parseable, token ID matches `user@realm!tokenid`), and returns errors listing every missing/invalid var at once. The token secret lives only inside the client; `Config.String()` redacts it.

### 4. Error mapping surfaces Proxmox semantics
HTTP 401/403 → explicit "permission denied by Proxmox (check token privileges)" errors; other non-2xx → status + Proxmox error body. Tools return these as MCP tool errors (`isError: true`), never panics. RBAC is enforced server-side by Proxmox; the MCP layer just reports it clearly.

### 5. Tool layer in `internal/tools`, one registration function
`tools.Register(server, client)` registers all tools; each tool is a typed handler struct (input params → JSON output). Tool results return the decoded JSON from Proxmox re-marshaled, so assistants get structured data. Names: snake_case verb-first per project convention.

### 6. TLS policy
Default `http.Transport` (full verification). `PROXMOX_INSECURE_TLS=true` swaps in `TLSClientConfig{InsecureSkipVerify: true}` and logs a warning to stderr (stdout is reserved for MCP stdio framing — all logging goes to stderr).

## Risks / Trade-offs

- [Official Go SDK is young, API may shift] → Pin the version in go.mod; client layer is SDK-agnostic so churn is confined to `cmd/` and `internal/tools`
- [Self-signed certs are common on Proxmox] → Explicit `PROXMOX_INSECURE_TLS` escape hatch with stderr warning; document recommending proper CA setup
- [Token with too-broad privileges] → Read-only tools only in this change; docs recommend a `PVEAuditor` token
- [Large clusters → large tool outputs] → Acceptable for v1; pagination/filtering params can be added per-tool later without breaking changes

## Migration Plan

Greenfield — no migration. Rollback = delete the scaffold.

## Open Questions

(none)

## Resolved

- Module path confirmed: `github.com/banux/go-mcp-proxmox` (repo https://github.com/banux/go-mcp-proxmox)
- Minimum supported Proxmox VE version: **8.x** (confirmed by user)
