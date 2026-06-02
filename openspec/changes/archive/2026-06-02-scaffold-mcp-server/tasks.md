## 1. Project Setup

- [x] 1.1 Initialize Go module `github.com/banux/go-mcp-proxmox` and create `cmd/go-mcp-proxmox/`, `internal/proxmox/`, `internal/tools/` layout
- [x] 1.2 Add dependency `github.com/modelcontextprotocol/go-sdk` (pinned) and a minimal `main.go` that compiles
- [x] 1.3 Add `.gitignore` (binary, coverage files) and a basic `README.md` (purpose, env vars, token setup with PVEAuditor recommendation)

## 2. Configuration

- [x] 2.1 Implement `internal/proxmox` `Config` + `LoadConfig()`: read `PROXMOX_URL`, `PROXMOX_TOKEN_ID`, `PROXMOX_TOKEN_SECRET`, `PROXMOX_INSECURE_TLS`; validate URL and `user@realm!tokenid` format; aggregate all missing/invalid vars in one error
- [x] 2.2 Implement secret redaction (`Config.String()` redacts the token secret)
- [x] 2.3 Table-driven tests for `LoadConfig()`: valid config, multiple missing vars reported together, invalid token ID format, redaction

## 3. Proxmox Client

- [x] 3.1 Implement client core `do(ctx, method, path, query)`: base URL joining under `/api2/json`, `Authorization: PVEAPIToken=...` header on every request, `data` envelope unwrapping
- [x] 3.2 Implement TLS policy: default verification; `InsecureSkipVerify` only when `PROXMOX_INSECURE_TLS=true` with stderr warning
- [x] 3.3 Implement error mapping: 401/403 → permission error mentioning token privileges; other non-2xx → status + Proxmox error body
- [x] 3.4 Implement typed methods: `ListNodes`, `GetNodeStatus(node)`, `ListQemu(node)`, `ListLXC(node)`, `ListStorage(node?)`, `GetClusterResources(type?)` with response structs
- [x] 3.5 `httptest`-based tests: auth header presence (no Cookie/CSRF), envelope unwrapping, 403 mapping, 500 mapping, TLS failure on untrusted cert vs insecure opt-in

## 4. MCP Tools

- [x] 4.1 Implement `internal/tools` with `Register(server, client)` and typed input structs (JSON schemas) for the 6 tools
- [x] 4.2 Implement `list_nodes` and `get_node_status` (required `node` param validated before any API call)
- [x] 4.3 Implement `list_vms` and `list_lxc` with optional `node`: single-node call or aggregation across online nodes (entries tagged with their node)
- [x] 4.4 Implement `list_storage` (optional `node`) and `get_cluster_resources` (optional `type` filter)
- [x] 4.5 Map client errors to MCP tool errors (`isError: true`); ensure a failing tool never crashes the server
- [x] 4.6 Tests for tools against a mocked client/API: happy paths, missing required param (no API call made), 403 surfaced as tool error

## 5. Server Entrypoint

- [x] 5.1 Implement `cmd/go-mcp-proxmox/main.go`: load config (fail-fast, non-zero exit, message on stderr), build client, create MCP server, `tools.Register`, serve over stdio
- [x] 5.2 Route all logging to stderr (stdout reserved for MCP framing)
- [x] 5.3 Smoke test: `initialize` + `tools/list` over stdio returns the 6 tools (in-process SDK transport)

## 6. Verification

- [x] 6.1 `gofmt`, `go vet`, `go test ./...` all clean
- [x] 6.2 Manual check against a real or mocked Proxmox: README quick-start works end-to-end with a token
