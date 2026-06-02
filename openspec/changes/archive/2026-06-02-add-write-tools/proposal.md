## Why

The server today is strictly read-only: an operator can inspect the cluster but cannot act on it. To make go-mcp-proxmox useful for day-to-day administration through an MCP client, it needs to drive the full lifecycle of guests and the common management operations (snapshots, creation, deletion, migration, resize) that today still require the web UI or `pvesh`.

## What Changes

- Extend the Proxmox client with write verbs (`POST`, `PUT`, `DELETE`) on top of the existing `GET` support.
- Add asynchronous-task handling: most Proxmox mutations return a UPID; the client gains the ability to read a task's status (`GET /nodes/{node}/tasks/{upid}/status`) and optionally wait for completion.
- Introduce a new `proxmox-write-tools` capability exposing mutating MCP tools:
  - **Guest lifecycle** (QEMU + LXC): `start_guest`, `stop_guest`, `shutdown_guest`, `reboot_guest`.
  - **Snapshots**: `list_snapshots`, `create_snapshot`, `rollback_snapshot`, `delete_snapshot`.
  - **Guest management**: `clone_vm`, `delete_guest`, `migrate_guest`, `resize_disk`.
- Add a global write guard: mutating tools are only registered/usable when `PROXMOX_ALLOW_WRITE=true`, so the default deployment stays read-only. **BREAKING** for nobody (additive), but changes default tool surface only when the flag is set.
- Document the additional Proxmox privileges (`VM.PowerMgmt`, `VM.Snapshot`, `VM.Allocate`, `VM.Migrate`, `VM.Config.Disk`) the API token needs.

## Capabilities

### New Capabilities
- `proxmox-write-tools`: Mutating MCP tools for guest lifecycle, snapshots and guest management, gated behind an explicit write-enable flag, mapping each tool to a Proxmox `POST`/`PUT`/`DELETE` endpoint and surfacing the resulting task.

### Modified Capabilities
- `proxmox-client`: Add `POST`/`PUT`/`DELETE` request methods, form-encoded request bodies, and Proxmox task (UPID) status reading / optional wait-for-completion. The existing read, auth, TLS, envelope, error-mapping and redaction requirements are unchanged.

## Impact

- Code: `internal/proxmox/client.go` (new verbs + task helpers), new `internal/proxmox` methods for each operation, `internal/tools/tools.go` (register write tools behind the flag), `internal/proxmox/config.go` (read `PROXMOX_ALLOW_WRITE`).
- Config: new optional env var `PROXMOX_ALLOW_WRITE` (default `false`).
- Ops: the API token must be granted write privileges in Proxmox for these tools to succeed; otherwise they surface the existing `403` permission error.
- Docs: README updated with the write flag, the new tools and the required token roles.
