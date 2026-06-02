## ADDED Requirements

### Requirement: Mutating request verbs
The client SHALL support `POST`, `PUT` and `DELETE` requests against the Proxmox VE API in addition to `GET`. Request parameters for mutating verbs SHALL be sent as `application/x-www-form-urlencoded` bodies, matching the Proxmox API convention.

#### Scenario: POST with form body
- **WHEN** the client issues a `POST` with parameters `{newid: 200}`
- **THEN** the request carries `Content-Type: application/x-www-form-urlencoded` and a body encoding `newid=200`

#### Scenario: DELETE without body
- **WHEN** the client issues a `DELETE` with no parameters
- **THEN** the request is sent with the configured auth header and no form body

### Requirement: Task (UPID) result handling
Most Proxmox mutations respond with a task identifier (UPID) as their `data` payload. The client SHALL expose the started task's UPID to callers and SHALL provide a way to read a task's status via `GET /nodes/{node}/tasks/{upid}/status`.

#### Scenario: Mutation returns a UPID
- **WHEN** a mutating call responds `200` with body `{"data": "UPID:thor:..."}`
- **THEN** the caller receives the UPID string

#### Scenario: Read task status
- **WHEN** the task-status helper is called with a node and a UPID
- **THEN** a `GET` is sent to `/nodes/{node}/tasks/{upid}/status` and the decoded status payload is returned

### Requirement: Write-enable configuration
The configuration SHALL read the optional environment variable `PROXMOX_ALLOW_WRITE` (default `false`). Loading MUST fail with an error if the value is present but not a parseable boolean, consistent with `PROXMOX_INSECURE_TLS`.

#### Scenario: Write disabled by default
- **WHEN** `PROXMOX_ALLOW_WRITE` is unset
- **THEN** the loaded configuration reports write access as disabled

#### Scenario: Invalid write flag rejected
- **WHEN** `PROXMOX_ALLOW_WRITE` is set to a non-boolean value
- **THEN** loading fails with an error naming the variable
