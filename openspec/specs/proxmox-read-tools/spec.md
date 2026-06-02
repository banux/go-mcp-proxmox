# proxmox-read-tools Specification

## Purpose

Read-only MCP tools exposing cluster inventory and status: nodes, VMs, LXC containers, storage and cluster resources.

## Requirements

### Requirement: Read-only inventory tools
The server SHALL expose the following read-only MCP tools, each mapped to a GET endpoint of the Proxmox VE API and returning the decoded Proxmox payload as structured JSON text:

- `list_nodes` → `GET /nodes` — cluster nodes with status
- `get_node_status` (param `node`, required) → `GET /nodes/{node}/status` — detailed node status
- `list_vms` (param `node`, optional) → `GET /nodes/{node}/qemu` for one node, or aggregated over all nodes when omitted
- `list_lxc` (param `node`, optional) → `GET /nodes/{node}/lxc` for one node, or aggregated over all nodes when omitted
- `list_storage` (param `node`, optional) → `GET /storage`, or `GET /nodes/{node}/storage` when a node is given
- `get_cluster_resources` (param `type`, optional: `vm`, `storage`, `node`, `sdn`) → `GET /cluster/resources`

#### Scenario: List nodes
- **WHEN** the `list_nodes` tool is called
- **THEN** the result contains the JSON array of cluster nodes returned by Proxmox

#### Scenario: List VMs on one node
- **WHEN** `list_vms` is called with `node: "pve1"`
- **THEN** the result contains the QEMU VMs of node `pve1`

#### Scenario: List VMs across the cluster
- **WHEN** `list_vms` is called without a node parameter
- **THEN** the result contains the VMs of every online node, each entry indicating its node

#### Scenario: Filtered cluster resources
- **WHEN** `get_cluster_resources` is called with `type: "vm"`
- **THEN** the result contains only resources of type VM

### Requirement: Tool input validation
Each tool SHALL declare a JSON schema for its parameters; required parameters missing from a call MUST produce a tool error without contacting the Proxmox API.

#### Scenario: Missing required parameter
- **WHEN** `get_node_status` is called without `node`
- **THEN** the tool returns an error identifying the missing parameter and no API request is made

### Requirement: API errors returned as tool errors
When the Proxmox API returns an error, the tool SHALL return an MCP tool error (`isError: true`) containing the mapped error message. Tools MUST NOT crash the server on API failures.

#### Scenario: Permission denied surfaced
- **WHEN** a tool call hits a `403` from Proxmox
- **THEN** the tool result is an error explaining the token lacks privileges, and the server keeps serving

### Requirement: No mutation
Tools in this capability SHALL only perform GET requests against the Proxmox API.

#### Scenario: Read-only guarantee
- **WHEN** any tool of this capability is executed
- **THEN** only HTTP GET requests are sent to the Proxmox API
