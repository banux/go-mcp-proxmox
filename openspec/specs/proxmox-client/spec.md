# proxmox-client Specification

## Purpose

HTTP client for the Proxmox VE API: API token authentication, environment-based configuration, TLS policy, response envelope handling and error mapping.

## Requirements

### Requirement: API token authentication
The client SHALL authenticate every request to the Proxmox VE API using an API token sent in the `Authorization` header with the format `PVEAPIToken=<USER@REALM!TOKENID>=<SECRET>`. The client MUST NOT implement or fall back to the ticket/cookie (username+password) authentication flow.

#### Scenario: Token header on every request
- **WHEN** the client performs any request to the Proxmox API
- **THEN** the request carries the header `Authorization: PVEAPIToken=<token-id>=<secret>` and no `Cookie` or `CSRFPreventionToken` header

### Requirement: Configuration from environment variables
The client configuration SHALL be loaded from the environment variables `PROXMOX_URL`, `PROXMOX_TOKEN_ID`, `PROXMOX_TOKEN_SECRET` (required) and `PROXMOX_INSECURE_TLS` (optional, default `false`). Loading MUST fail with an error naming every missing or invalid variable.

#### Scenario: Valid configuration
- **WHEN** all required variables are set with valid values (parseable URL, token ID matching `user@realm!tokenid`)
- **THEN** loading succeeds and returns a usable configuration

#### Scenario: Missing variables reported together
- **WHEN** `PROXMOX_URL` and `PROXMOX_TOKEN_SECRET` are both unset
- **THEN** loading fails with a single error that names both missing variables

#### Scenario: Invalid token ID format
- **WHEN** `PROXMOX_TOKEN_ID` does not match the `user@realm!tokenid` format
- **THEN** loading fails with an error describing the expected format

### Requirement: TLS verification by default
The client SHALL verify TLS certificates by default. Verification MAY be disabled only when `PROXMOX_INSECURE_TLS=true`, in which case the client MUST emit a warning on stderr at startup.

#### Scenario: Default TLS policy
- **WHEN** `PROXMOX_INSECURE_TLS` is unset or `false`
- **THEN** requests to a server with an untrusted certificate fail with a TLS error

#### Scenario: Insecure opt-in
- **WHEN** `PROXMOX_INSECURE_TLS=true`
- **THEN** certificate verification is skipped and a warning is written to stderr

### Requirement: Response envelope handling
The client SHALL unwrap the Proxmox `{"data": ...}` JSON envelope and return only the payload to callers.

#### Scenario: Successful response
- **WHEN** the API responds `200` with body `{"data": [{"node": "pve1"}]}`
- **THEN** the caller receives the decoded `[{"node": "pve1"}]` payload

### Requirement: Error mapping
The client SHALL map HTTP errors to descriptive Go errors: `401`/`403` responses MUST produce a permission error mentioning that the token lacks privileges in Proxmox; other non-2xx responses MUST include the HTTP status and the Proxmox error body.

#### Scenario: Permission denied
- **WHEN** the API responds `403`
- **THEN** the returned error states that the Proxmox token lacks the required privileges

#### Scenario: Other API error
- **WHEN** the API responds `500` with an error body
- **THEN** the returned error contains the status code and the response body

### Requirement: Secret redaction
The client and its configuration SHALL never expose the token secret in logs, error messages, or string representations.

#### Scenario: Config printed
- **WHEN** the configuration is formatted as a string (logging, error context)
- **THEN** the token secret is redacted
