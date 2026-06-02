## 1. Configuration

- [x] 1.1 Add `EnvAllowWrite = "PROXMOX_ALLOW_WRITE"` constant and an `AllowWrite bool` field to `Config` in `internal/proxmox/config.go`.
- [x] 1.2 Parse `PROXMOX_ALLOW_WRITE` in `LoadConfig` with the same boolean handling as `PROXMOX_INSECURE_TLS` (default `false`, error on non-boolean).
- [x] 1.3 Add config tests for default-off, `true`, and invalid-value cases in `config_test.go`.

## 2. Client write verbs and task handling

- [x] 2.1 Extend `do` (or add a sibling) in `client.go` to accept an optional `url.Values` form body; set `Content-Type: application/x-www-form-urlencoded` when a body is present.
- [x] 2.2 Add `post`, `put`, `delete` helpers that return the unwrapped payload.
- [x] 2.3 Add `TaskStatus(ctx, node, upid)` reading `GET /nodes/{node}/tasks/{upid}/status` and returning the raw status JSON.
- [x] 2.4 Add client methods for each operation: `StartGuest`, `StopGuest`, `ShutdownGuest`, `RebootGuest`, `ListSnapshots`, `CreateSnapshot`, `RollbackSnapshot`, `DeleteSnapshot`, `CloneVM`, `DeleteGuest`, `MigrateGuest`, `ResizeDisk` — each building the `/{type}/` path and returning the UPID (or payload for `ListSnapshots`).
- [x] 2.5 Add `client_test.go` cases with an `httptest` server asserting method, path, form body, and UPID extraction for representative operations (start, create_snapshot, clone, delete, resize, task status).

## 3. Write tools registration and handlers

- [x] 3.1 Add a `guestType` validation helper (accepts `qemu`/`lxc`) and shared arg structs (`guestArgs`, `snapshotArgs`, etc.) in `internal/tools`.
- [x] 3.2 Implement handlers for the lifecycle tools (`start_guest`, `stop_guest`, `shutdown_guest`, `reboot_guest`), returning `{node, upid}` JSON.
- [x] 3.3 Implement snapshot handlers (`list_snapshots`, `create_snapshot`, `rollback_snapshot`, `delete_snapshot`).
- [x] 3.4 Implement management handlers (`clone_vm`, `delete_guest`, `migrate_guest`, `resize_disk`).
- [x] 3.5 In `Register`, register the mutating tools only when `cfg.AllowWrite` is true; thread the flag (or `Config`) into `Register`/`tools` as needed.
- [x] 3.6 Add `tools_test.go` cases: required-parameter validation, invalid `type` rejection, and that mutating tools are absent when the flag is off / present when on.

## 4. Wiring and docs

- [x] 4.1 Pass the write flag from `cmd/go-mcp-proxmox/main.go` through to `tools.Register` (update `Register` signature or pass `cfg`).
- [x] 4.2 Update `README.md`: document `PROXMOX_ALLOW_WRITE`, list the new tools, note UPID/async semantics, and the required Proxmox token privileges (`VM.PowerMgmt`, `VM.Snapshot`, `VM.Allocate`, `VM.Migrate`, `VM.Config.Disk`).
- [x] 4.3 Run `go build ./...` and `go test ./...`; verify the read-only tool surface is unchanged when the flag is off.

## 5. Manual end-to-end verification

- [x] 5.1 With `PROXMOX_ALLOW_WRITE=true`, drive the server over stdio against a non-critical guest: create_snapshot → list_snapshots → delete_snapshot, and a start/stop cycle, confirming UPIDs and task status.
