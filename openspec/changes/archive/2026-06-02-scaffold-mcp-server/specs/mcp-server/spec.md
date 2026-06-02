## ADDED Requirements

### Requirement: MCP server over stdio
The binary SHALL run an MCP server using the official `modelcontextprotocol/go-sdk`, communicating over stdio. All logging MUST go to stderr; stdout is reserved for MCP protocol framing.

#### Scenario: Server starts and responds to initialize
- **WHEN** the binary is started with a valid configuration and an MCP client sends `initialize`
- **THEN** the server responds with its server info and advertised tool capability

#### Scenario: Logs do not corrupt the protocol
- **WHEN** the server logs anything (warnings, request traces)
- **THEN** the output is written to stderr only

### Requirement: Fail-fast startup validation
The server SHALL validate the Proxmox configuration before serving. On invalid or missing configuration it MUST exit with a non-zero status and a clear message on stderr, without starting the MCP transport.

#### Scenario: Missing configuration
- **WHEN** the binary is started without `PROXMOX_URL`
- **THEN** it exits non-zero and stderr names the missing variable

### Requirement: Tool registration
The server SHALL register all available tools at startup through a single registration function, so an MCP client listing tools sees the complete set.

#### Scenario: Tools listed
- **WHEN** an MCP client sends `tools/list`
- **THEN** the response contains `list_nodes`, `get_node_status`, `list_vms`, `list_lxc`, `list_storage`, and `get_cluster_resources`
