# go-mcp-proxmox

MCP (Model Context Protocol) server for [Proxmox VE](https://www.proxmox.com/), written in Go. It lets AI assistants (Claude, etc.) inspect a Proxmox cluster through MCP tools, authenticated exclusively with a Proxmox **API token**.

Requires Proxmox VE **8.x** or later.

## Tools

All tools are **read-only** (GET requests only):

| Tool | Description |
|---|---|
| `list_nodes` | List cluster nodes with status |
| `get_node_status` | Detailed status of one node (`node` required) |
| `list_vms` | QEMU VMs of one node, or the whole cluster if `node` is omitted |
| `list_lxc` | LXC containers of one node, or the whole cluster if `node` is omitted |
| `list_storage` | Storage configuration, optionally scoped to a node |
| `get_cluster_resources` | Cluster resources, optionally filtered by `type` (`vm`, `storage`, `node`, `sdn`) |

## Configuration

Configuration is done entirely through environment variables:

| Variable | Required | Description |
|---|---|---|
| `PROXMOX_URL` | yes | Base URL of the Proxmox API, e.g. `https://pve.example.com:8006` |
| `PROXMOX_TOKEN_ID` | yes | API token ID in the form `user@realm!tokenid`, e.g. `mcp@pve!claude` |
| `PROXMOX_TOKEN_SECRET` | yes | The token secret (UUID) |
| `PROXMOX_INSECURE_TLS` | no | Set to `true` to skip TLS verification (self-signed certs). Default: `false` |

## Creating an API token

A token with the built-in **`PVEAuditor`** role is enough for all current tools (read-only):

```sh
# On the Proxmox host (or via the web UI: Datacenter → Permissions → API Tokens)
pveum user add mcp@pve
pveum acl modify / --users mcp@pve --roles PVEAuditor
pveum user token add mcp@pve claude --privsep 0
```

Keep the printed secret — it is shown only once.

## Usage

```sh
go build ./cmd/go-mcp-proxmox
```

Example Claude Code configuration (`.mcp.json`):

```json
{
  "mcpServers": {
    "proxmox": {
      "command": "/path/to/go-mcp-proxmox",
      "env": {
        "PROXMOX_URL": "https://pve.example.com:8006",
        "PROXMOX_TOKEN_ID": "mcp@pve!claude",
        "PROXMOX_TOKEN_SECRET": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
      }
    }
  }
}
```

## Security notes

- Authentication is **token-only**: no username/password or ticket/cookie flow.
- TLS verification is **on by default**; `PROXMOX_INSECURE_TLS=true` is an explicit opt-out and logs a warning. Prefer installing a proper CA-signed certificate.
- Permissions are enforced by Proxmox RBAC: the server only ever has the privileges granted to the token.
- The token secret is never logged or included in tool output.

## Development

```sh
go test ./...
go vet ./...
```

Project specs live in [`openspec/`](openspec/) (spec-driven workflow with [OpenSpec](https://github.com/Fission-AI/OpenSpec)).
