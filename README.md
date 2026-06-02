# go-mcp-proxmox

MCP (Model Context Protocol) server for [Proxmox VE](https://www.proxmox.com/), written in Go. It lets AI assistants (Claude, etc.) inspect a Proxmox cluster through MCP tools, authenticated exclusively with a Proxmox **API token**.

Requires Proxmox VE **8.x** or later.

## Tools

### Read-only tools

Always available (GET requests only):

| Tool | Description |
|---|---|
| `list_nodes` | List cluster nodes with status |
| `get_node_status` | Detailed status of one node (`node` required) |
| `list_vms` | QEMU VMs of one node, or the whole cluster if `node` is omitted |
| `list_lxc` | LXC containers of one node, or the whole cluster if `node` is omitted |
| `list_storage` | Storage configuration, optionally scoped to a node |
| `get_cluster_resources` | Cluster resources, optionally filtered by `type` (`vm`, `storage`, `node`, `sdn`) |

### Write tools

Registered **only when `PROXMOX_ALLOW_WRITE=true`** (see [Configuration](#configuration)). They are off by default, so a standard deployment stays read-only.

All write tools take `node`, `vmid` and `type` (`qemu` or `lxc`), except `clone_vm` which is QEMU-only. Proxmox executes most mutations as asynchronous tasks, so these tools return the **task UPID** that was *started* — not the final outcome. The task may still fail afterwards; track it with the task UPID in the Proxmox UI.

| Tool | Description |
|---|---|
| `start_guest` | Start a VM/container |
| `stop_guest` | Hard power-off |
| `shutdown_guest` | Graceful shutdown |
| `reboot_guest` | Reboot |
| `list_snapshots` | List a guest's snapshots |
| `create_snapshot` | Create a snapshot (`snapname` required; optional `description`, `vmstate`) |
| `rollback_snapshot` | Roll back to a snapshot (`snapname` required) |
| `delete_snapshot` | Delete a snapshot (`snapname` required) |
| `clone_vm` | Clone a QEMU VM (`newid` required; optional `name`, `target`, `full`) |
| `delete_guest` | Destroy a VM/container |
| `migrate_guest` | Migrate to another node (`target` required; optional `online`) |
| `resize_disk` | Grow a disk (`disk` and `size` required, e.g. `scsi0` / `+10G`) |

## Configuration

Configuration is done entirely through environment variables:

| Variable | Required | Description |
|---|---|---|
| `PROXMOX_URL` | yes | Base URL of the Proxmox API, e.g. `https://pve.example.com:8006` |
| `PROXMOX_TOKEN_ID` | yes | API token ID in the form `user@realm!tokenid`, e.g. `mcp@pve!claude` |
| `PROXMOX_TOKEN_SECRET` | yes | The token secret (UUID) |
| `PROXMOX_INSECURE_TLS` | no | Set to `true` to skip TLS verification (self-signed certs). Default: `false` |
| `PROXMOX_ALLOW_WRITE` | no | Set to `true` to register the mutating tools. Default: `false` (read-only) |

## Creating an API token

A token with the built-in **`PVEAuditor`** role is enough for the read-only tools:

```sh
# On the Proxmox host (or via the web UI: Datacenter → Permissions → API Tokens)
pveum user add mcp@pve
pveum acl modify / --users mcp@pve --roles PVEAuditor
pveum user token add mcp@pve claude --privsep 0
```

Keep the printed secret — it is shown only once.

To use the **write tools** (with `PROXMOX_ALLOW_WRITE=true`), the token additionally needs the relevant privileges. Grant only what you need:

| Tools | Required privilege |
|---|---|
| `start_/stop_/shutdown_/reboot_guest` | `VM.PowerMgmt` |
| `*_snapshot` | `VM.Snapshot` |
| `clone_vm`, `delete_guest` | `VM.Allocate` |
| `migrate_guest` | `VM.Migrate` |
| `resize_disk` | `VM.Config.Disk` |

The built-in `PVEVMAdmin` role covers all of the above. For example, to allow VM administration on the whole cluster:

```sh
pveum acl modify / --users mcp@pve --roles PVEVMAdmin
```

`PROXMOX_ALLOW_WRITE` and the token's Proxmox privileges are independent gates: a mutation is only possible when **both** allow it.

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
- Mutating tools are **disabled by default**; `PROXMOX_ALLOW_WRITE=true` is an explicit opt-in that is independent of the token's privileges, so a read-only deployment cannot mutate even with an over-privileged token.
- The token secret is never logged or included in tool output.

## Development

```sh
go test ./...
go vet ./...
```

Project specs live in [`openspec/`](openspec/) (spec-driven workflow with [OpenSpec](https://github.com/Fission-AI/OpenSpec)).
