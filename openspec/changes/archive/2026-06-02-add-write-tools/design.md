## Context

The server is read-only today: `internal/proxmox/client.go` only exposes `do(ctx, method, path, query)` used through a `get` helper, and `internal/tools/tools.go` registers six GET-backed tools. Adding write tools touches three concerns: the HTTP client (new verbs + form bodies + task handling), the configuration (a write-enable flag), and tool registration (gating + new mutating handlers). The Proxmox API models almost every mutation as an asynchronous worker task and replies with a UPID string rather than the final result, which shapes how tools report success.

## Goals / Non-Goals

**Goals:**
- Provide guest lifecycle, snapshot and management tools mapped to Proxmox `POST`/`PUT`/`DELETE` endpoints.
- Keep the default deployment read-only; writes require an explicit opt-in.
- Reuse the existing error-mapping, validation, and redaction behavior — write tools should feel like the read tools.
- Return the task UPID so the caller can track long-running operations.

**Non-Goals:**
- Waiting/polling a task to completion inside a tool call by default (the API is async; blocking risks MCP timeouts). A status-read helper is provided; synchronous wait is out of scope for this change.
- A generic "run any pvesh command" escape hatch.
- LXC clone (`clone_vm` is QEMU-only, matching the Proxmox endpoint shape); LXC creation/restore from template.
- Fine-grained per-tool permission flags — a single global `PROXMOX_ALLOW_WRITE` gate is sufficient for now.

## Decisions

**Single global write gate (`PROXMOX_ALLOW_WRITE`) over per-tool flags.**
Mutating tools are registered only when the flag is `true`. Rationale: defense-in-depth that is independent of the Proxmox token's privileges — a read-only deployment cannot accidentally mutate even if the token is over-privileged, and the tool surface stays minimal by default. Alternative considered: rely solely on Proxmox ACLs. Rejected because that couples safety to remote config and still advertises mutating tools the operator may not want exposed.

**Form-encoded bodies for POST/PUT.**
Proxmox expects `application/x-www-form-urlencoded`. Extend the client's `do` to accept an optional body (`url.Values`) and set the content type when present. Alternative: JSON bodies — rejected, Proxmox's `pvesh`/API convention is form encoding and JSON support is inconsistent across endpoints.

**Return the UPID, don't block.**
Each mutating tool returns the task UPID (and the node) as its JSON result. A separate client helper `TaskStatus(node, upid)` reads `/nodes/{node}/tasks/{upid}/status`. We do not expose a blocking tool now; if needed later a `wait_task` tool can be added without touching existing tools. Rationale: MCP calls should stay short; a clone or migration can run for minutes.

**Shared guest handler keyed by `type`.**
Lifecycle, snapshot and management tools take `type ∈ {qemu, lxc}` and build the path `/nodes/{node}/{type}/{vmid}/...`, validating `type` up front (mirrors the existing read-tool validation for `get_cluster_resources` `type`). `clone_vm` is the exception — QEMU-only, so it hardcodes `qemu`.

**`vmid` typed as integer in the tool schema.** The read tools already decode `vmid` as `json.Number`; for inputs we accept an integer and render it into the path. Keeps schemas explicit and avoids string/number ambiguity.

## Risks / Trade-offs

- **Destructive operations exposed to an LLM** (`delete_guest`, `rollback_snapshot`, `stop_guest`) → Mitigated by the `PROXMOX_ALLOW_WRITE` gate (off by default), reliance on Proxmox ACLs for the token, and the MCP client's own tool-confirmation UX. Document the risk in the README.
- **UPID-only result reads as "success" while the task later fails** → The tool reports the task was *started*, not that it *succeeded*; document this and provide the task-status helper. A future `wait_task` can close the gap.
- **Over-privileged token** → If the operator grants broad write ACLs, the gate still limits exposure, but the token should be scoped to the minimum roles (`VM.PowerMgmt`, `VM.Snapshot`, `VM.Allocate`, `VM.Migrate`, `VM.Config.Disk`). Documented.
- **Type validation drift** → Centralize the `qemu`/`lxc` check in one helper so all tools stay consistent.

## Migration Plan

Additive change. Existing read-only deployments are unaffected: without `PROXMOX_ALLOW_WRITE=true` the tool surface is identical to today. To enable writes, an operator sets the flag and grants the token the required Proxmox privileges. Rollback is simply unsetting the flag (and restarting the server).

## Open Questions

- Should `create_snapshot`'s `vmstate` (RAM snapshot) default on for QEMU? Leaning no — opt-in via the optional param to avoid surprising disk/time cost.
- Future: a `wait_task` / blocking option once real usage shows it's needed.
