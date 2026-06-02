## ADDED Requirements

### Requirement: Write tools gated by an explicit flag
The mutating MCP tools of this capability SHALL only be registered on the server when the environment variable `PROXMOX_ALLOW_WRITE` is set to `true`. When the flag is absent or any other value, the server SHALL expose only the read-only tools, and no mutating tool SHALL be callable.

#### Scenario: Write disabled by default
- **WHEN** the server starts without `PROXMOX_ALLOW_WRITE` set
- **THEN** none of the mutating tools are listed in `tools/list` and the read-only tools are unaffected

#### Scenario: Write explicitly enabled
- **WHEN** the server starts with `PROXMOX_ALLOW_WRITE=true`
- **THEN** the mutating tools are listed in `tools/list` alongside the read-only tools

### Requirement: Guest lifecycle tools
The server SHALL expose lifecycle tools that act on a single guest identified by `node`, `vmid` and `type` (`qemu` or `lxc`), each mapped to the matching Proxmox status endpoint:

- `start_guest` → `POST /nodes/{node}/{type}/{vmid}/status/start`
- `stop_guest` → `POST /nodes/{node}/{type}/{vmid}/status/stop`
- `shutdown_guest` → `POST /nodes/{node}/{type}/{vmid}/status/shutdown`
- `reboot_guest` → `POST /nodes/{node}/{type}/{vmid}/status/reboot`

Each tool SHALL return the identifier of the Proxmox task (UPID) it started.

#### Scenario: Start a stopped VM
- **WHEN** `start_guest` is called with `node: "thor"`, `vmid: 103`, `type: "qemu"`
- **THEN** a `POST` is sent to `/nodes/thor/qemu/103/status/start` and the result contains the started task UPID

#### Scenario: Graceful shutdown of a container
- **WHEN** `shutdown_guest` is called with `type: "lxc"`
- **THEN** a `POST` is sent to the LXC `status/shutdown` endpoint for that guest

#### Scenario: Invalid guest type rejected
- **WHEN** any lifecycle tool is called with `type` other than `qemu` or `lxc`
- **THEN** the tool returns an error and no API request is made

### Requirement: Snapshot tools
The server SHALL expose snapshot tools acting on a guest (`node`, `vmid`, `type`):

- `list_snapshots` → `GET /nodes/{node}/{type}/{vmid}/snapshot`
- `create_snapshot` (params `snapname` required, `description` optional, `vmstate` optional for QEMU) → `POST /nodes/{node}/{type}/{vmid}/snapshot`
- `rollback_snapshot` (param `snapname` required) → `POST /nodes/{node}/{type}/{vmid}/snapshot/{snapname}/rollback`
- `delete_snapshot` (param `snapname` required) → `DELETE /nodes/{node}/{type}/{vmid}/snapshot/{snapname}`

#### Scenario: Create a snapshot
- **WHEN** `create_snapshot` is called with `snapname: "before-upgrade"`
- **THEN** a `POST` is sent to the guest's `snapshot` endpoint with the snapshot name and the result contains the task UPID

#### Scenario: Roll back to a snapshot
- **WHEN** `rollback_snapshot` is called with `snapname: "before-upgrade"`
- **THEN** a `POST` is sent to `.../snapshot/before-upgrade/rollback`

#### Scenario: List returns existing snapshots
- **WHEN** `list_snapshots` is called for a guest
- **THEN** the result contains the JSON array of that guest's snapshots

### Requirement: Guest management tools
The server SHALL expose management tools:

- `clone_vm` (params `node`, `vmid`, `newid` required; `name`, `target`, `full` optional) → `POST /nodes/{node}/qemu/{vmid}/clone`
- `delete_guest` (params `node`, `vmid`, `type`) → `DELETE /nodes/{node}/{type}/{vmid}`
- `migrate_guest` (params `node`, `vmid`, `type`, `target` required; `online` optional) → `POST /nodes/{node}/{type}/{vmid}/migrate`
- `resize_disk` (params `node`, `vmid`, `type`, `disk`, `size` required) → `PUT /nodes/{node}/{type}/{vmid}/resize`

#### Scenario: Clone a VM to a new id
- **WHEN** `clone_vm` is called with `vmid: 100` and `newid: 200`
- **THEN** a `POST` is sent to `/nodes/{node}/qemu/100/clone` with `newid=200` and the result contains the task UPID

#### Scenario: Migrate a guest online
- **WHEN** `migrate_guest` is called with `target: "loki"` and `online: true`
- **THEN** a `POST` is sent to the guest's `migrate` endpoint with `target=loki` and `online=1`

#### Scenario: Grow a disk
- **WHEN** `resize_disk` is called with `disk: "scsi0"` and `size: "+10G"`
- **THEN** a `PUT` is sent to the guest's `resize` endpoint with `disk=scsi0` and `size=+10G`

### Requirement: Mutating tools reuse client error and validation behavior
The mutating tools SHALL validate required parameters before contacting the API and surface Proxmox errors as MCP tool errors (`isError: true`) without crashing the server, consistent with the read tools.

#### Scenario: Missing required parameter
- **WHEN** a mutating tool is called without a required parameter (e.g. `start_guest` without `vmid`)
- **THEN** the tool returns an error naming the missing parameter and no API request is made

#### Scenario: Permission denied surfaced
- **WHEN** a mutating tool call hits a `403` from Proxmox (token lacks write privileges)
- **THEN** the tool result is an error explaining the token lacks privileges and the server keeps serving
