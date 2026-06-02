package proxmox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// apiBase is the common prefix of every Proxmox VE API endpoint.
const apiBase = "/api2/json"

// Client is a minimal Proxmox VE API client authenticated with an API token.
type Client struct {
	baseURL    string
	authHeader string
	httpClient *http.Client
}

// NewClient builds a Client from a validated Config.
//
// When cfg.InsecureTLS is true, TLS certificate verification is disabled and a
// warning is written to stderr.
func NewClient(cfg Config) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		fmt.Fprintln(os.Stderr, "go-mcp-proxmox: WARNING: TLS certificate verification disabled (PROXMOX_INSECURE_TLS=true)")
	}
	return &Client{
		baseURL:    strings.TrimRight(cfg.URL, "/"),
		authHeader: fmt.Sprintf("PVEAPIToken=%s=%s", cfg.TokenID, cfg.TokenSecret),
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// apiError is the subset of a Proxmox error response we care about.
type apiError struct {
	Errors map[string]string `json:"errors"`
}

// do performs a request against the Proxmox API and returns the raw JSON
// payload, with the {"data": ...} envelope already unwrapped.
func (c *Client) do(ctx context.Context, method, path string, query url.Values) (json.RawMessage, error) {
	return c.doForm(ctx, method, path, query, nil)
}

// doForm is like do but, when form is non-empty, sends it as an
// application/x-www-form-urlencoded request body — the Proxmox convention for
// POST/PUT mutations.
func (c *Client) doForm(ctx context.Context, method, path string, query, form url.Values) (json.RawMessage, error) {
	reqURL := c.baseURL + apiBase + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	var reqBody io.Reader
	if len(form) > 0 {
		reqBody = strings.NewReader(form.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("building request %s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")
	if len(form) > 0 {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Proxmox API %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("reading Proxmox API response for %s %s: %w", method, path, err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("permission denied by Proxmox for %s %s (HTTP %d %s): the API token lacks the required privileges, check its roles/ACL in Proxmox",
			method, path, resp.StatusCode, strings.TrimSpace(resp.Status))
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("Proxmox API error for %s %s: HTTP %d: %s", method, path, resp.StatusCode, summarizeErrorBody(body))
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decoding Proxmox API response for %s %s: %w", method, path, err)
	}
	return envelope.Data, nil
}

// summarizeErrorBody extracts the useful part of a Proxmox error body.
func summarizeErrorBody(body []byte) string {
	var apiErr apiError
	if err := json.Unmarshal(body, &apiErr); err == nil && len(apiErr.Errors) > 0 {
		parts := make([]string, 0, len(apiErr.Errors))
		for field, msg := range apiErr.Errors {
			parts = append(parts, field+": "+msg)
		}
		return strings.Join(parts, "; ")
	}
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "(empty body)"
	}
	if len(s) > 500 {
		s = s[:500] + "…"
	}
	return s
}

// get performs a GET request and decodes the unwrapped payload into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	data, err := c.do(ctx, http.MethodGet, path, query)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decoding payload of GET %s: %w", path, err)
	}
	return nil
}

// upid sends a mutating request whose payload is a Proxmox task identifier and
// returns that UPID string. form may be nil for parameterless mutations.
func (c *Client) upid(ctx context.Context, method, path string, form url.Values) (string, error) {
	data, err := c.doForm(ctx, method, path, nil, form)
	if err != nil {
		return "", err
	}
	// Most mutations reply with the UPID as a JSON string; some reply null.
	if len(data) == 0 || string(data) == "null" {
		return "", nil
	}
	var upid string
	if err := json.Unmarshal(data, &upid); err != nil {
		return "", fmt.Errorf("decoding task UPID of %s %s: %w", method, path, err)
	}
	return upid, nil
}

// post issues a POST whose result is a task UPID.
func (c *Client) post(ctx context.Context, path string, form url.Values) (string, error) {
	return c.upid(ctx, http.MethodPost, path, form)
}

// put issues a PUT whose result is a task UPID.
func (c *Client) put(ctx context.Context, path string, form url.Values) (string, error) {
	return c.upid(ctx, http.MethodPut, path, form)
}

// delete issues a DELETE whose result is a task UPID.
func (c *Client) delete(ctx context.Context, path string, form url.Values) (string, error) {
	return c.upid(ctx, http.MethodDelete, path, form)
}

// Node is an entry of GET /nodes.
type Node struct {
	Node   string  `json:"node"`
	Status string  `json:"status"`
	CPU    float64 `json:"cpu,omitempty"`
	MaxCPU int     `json:"maxcpu,omitempty"`
	Mem    int64   `json:"mem,omitempty"`
	MaxMem int64   `json:"maxmem,omitempty"`
	Uptime int64   `json:"uptime,omitempty"`
}

// ListNodes returns the cluster nodes (GET /nodes).
func (c *Client) ListNodes(ctx context.Context) ([]Node, error) {
	var nodes []Node
	if err := c.get(ctx, "/nodes", nil, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

// GetNodeStatus returns the detailed status of one node
// (GET /nodes/{node}/status). The payload shape varies across PVE versions,
// so it is returned as raw JSON.
func (c *Client) GetNodeStatus(ctx context.Context, node string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/status", nil)
}

// VM is an entry of GET /nodes/{node}/qemu or /nodes/{node}/lxc.
type VM struct {
	VMID   json.Number `json:"vmid"`
	Name   string      `json:"name,omitempty"`
	Status string      `json:"status"`
	CPU    float64     `json:"cpu,omitempty"`
	CPUs   int         `json:"cpus,omitempty"`
	Mem    int64       `json:"mem,omitempty"`
	MaxMem int64       `json:"maxmem,omitempty"`
	Uptime int64       `json:"uptime,omitempty"`
	Tags   string      `json:"tags,omitempty"`
	// Node is filled by callers when aggregating across nodes.
	Node string `json:"node,omitempty"`
}

// ListQemu returns the QEMU VMs of one node (GET /nodes/{node}/qemu).
func (c *Client) ListQemu(ctx context.Context, node string) ([]VM, error) {
	var vms []VM
	if err := c.get(ctx, "/nodes/"+url.PathEscape(node)+"/qemu", nil, &vms); err != nil {
		return nil, err
	}
	return vms, nil
}

// ListLXC returns the LXC containers of one node (GET /nodes/{node}/lxc).
func (c *Client) ListLXC(ctx context.Context, node string) ([]VM, error) {
	var cts []VM
	if err := c.get(ctx, "/nodes/"+url.PathEscape(node)+"/lxc", nil, &cts); err != nil {
		return nil, err
	}
	return cts, nil
}

// ListStorage returns the storage configuration (GET /storage), or the
// storages of one node (GET /nodes/{node}/storage) when node is non-empty.
// The payload shape differs between the two endpoints, so it is returned as
// raw JSON.
func (c *Client) ListStorage(ctx context.Context, node string) (json.RawMessage, error) {
	path := "/storage"
	if node != "" {
		path = "/nodes/" + url.PathEscape(node) + "/storage"
	}
	return c.do(ctx, http.MethodGet, path, nil)
}

// GetClusterResources returns the cluster resources (GET /cluster/resources),
// optionally filtered by type (vm, storage, node, sdn).
func (c *Client) GetClusterResources(ctx context.Context, resourceType string) (json.RawMessage, error) {
	var query url.Values
	if resourceType != "" {
		query = url.Values{"type": {resourceType}}
	}
	return c.do(ctx, http.MethodGet, "/cluster/resources", query)
}

// guestPath builds the API path for a guest, e.g. /nodes/thor/qemu/103. typ is
// "qemu" or "lxc"; callers are responsible for validating it.
func guestPath(node, typ string, vmid int) string {
	return fmt.Sprintf("/nodes/%s/%s/%d", url.PathEscape(node), typ, vmid)
}

// TaskStatus returns the status of a Proxmox task
// (GET /nodes/{node}/tasks/{upid}/status) as raw JSON.
func (c *Client) TaskStatus(ctx context.Context, node, upid string) (json.RawMessage, error) {
	path := "/nodes/" + url.PathEscape(node) + "/tasks/" + url.PathEscape(upid) + "/status"
	return c.do(ctx, http.MethodGet, path, nil)
}

// --- Guest lifecycle ---

// StartGuest starts a guest and returns the started task UPID.
func (c *Client) StartGuest(ctx context.Context, node, typ string, vmid int) (string, error) {
	return c.post(ctx, guestPath(node, typ, vmid)+"/status/start", nil)
}

// StopGuest stops (hard) a guest and returns the started task UPID.
func (c *Client) StopGuest(ctx context.Context, node, typ string, vmid int) (string, error) {
	return c.post(ctx, guestPath(node, typ, vmid)+"/status/stop", nil)
}

// ShutdownGuest gracefully shuts down a guest and returns the started task UPID.
func (c *Client) ShutdownGuest(ctx context.Context, node, typ string, vmid int) (string, error) {
	return c.post(ctx, guestPath(node, typ, vmid)+"/status/shutdown", nil)
}

// RebootGuest reboots a guest and returns the started task UPID.
func (c *Client) RebootGuest(ctx context.Context, node, typ string, vmid int) (string, error) {
	return c.post(ctx, guestPath(node, typ, vmid)+"/status/reboot", nil)
}

// --- Snapshots ---

// ListSnapshots returns the snapshots of a guest as raw JSON.
func (c *Client) ListSnapshots(ctx context.Context, node, typ string, vmid int) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, guestPath(node, typ, vmid)+"/snapshot", nil)
}

// CreateSnapshot creates a snapshot and returns the started task UPID.
// description may be empty; vmstate (QEMU RAM snapshot) is included only when true.
func (c *Client) CreateSnapshot(ctx context.Context, node, typ string, vmid int, snapname, description string, vmstate bool) (string, error) {
	form := url.Values{"snapname": {snapname}}
	if description != "" {
		form.Set("description", description)
	}
	if vmstate {
		form.Set("vmstate", "1")
	}
	return c.post(ctx, guestPath(node, typ, vmid)+"/snapshot", form)
}

// RollbackSnapshot rolls a guest back to a snapshot and returns the task UPID.
func (c *Client) RollbackSnapshot(ctx context.Context, node, typ string, vmid int, snapname string) (string, error) {
	return c.post(ctx, guestPath(node, typ, vmid)+"/snapshot/"+url.PathEscape(snapname)+"/rollback", nil)
}

// DeleteSnapshot deletes a snapshot and returns the started task UPID.
func (c *Client) DeleteSnapshot(ctx context.Context, node, typ string, vmid int, snapname string) (string, error) {
	return c.delete(ctx, guestPath(node, typ, vmid)+"/snapshot/"+url.PathEscape(snapname), nil)
}

// --- Guest management ---

// CloneVM clones a QEMU VM to newid and returns the started task UPID.
// name and target may be empty; full requests a full (non-linked) clone.
func (c *Client) CloneVM(ctx context.Context, node string, vmid, newid int, name, target string, full bool) (string, error) {
	form := url.Values{"newid": {strconv.Itoa(newid)}}
	if name != "" {
		form.Set("name", name)
	}
	if target != "" {
		form.Set("target", target)
	}
	if full {
		form.Set("full", "1")
	}
	return c.post(ctx, guestPath(node, "qemu", vmid)+"/clone", form)
}

// DeleteGuest destroys a guest and returns the started task UPID.
func (c *Client) DeleteGuest(ctx context.Context, node, typ string, vmid int) (string, error) {
	return c.delete(ctx, guestPath(node, typ, vmid), nil)
}

// MigrateGuest migrates a guest to target and returns the started task UPID.
func (c *Client) MigrateGuest(ctx context.Context, node, typ string, vmid int, target string, online bool) (string, error) {
	form := url.Values{"target": {target}}
	if online {
		form.Set("online", "1")
	}
	return c.post(ctx, guestPath(node, typ, vmid)+"/migrate", form)
}

// ResizeDisk grows a guest disk (e.g. disk "scsi0", size "+10G") and returns
// the started task UPID. Proxmox uses PUT for the resize endpoint.
func (c *Client) ResizeDisk(ctx context.Context, node, typ string, vmid int, disk, size string) (string, error) {
	form := url.Values{"disk": {disk}, "size": {size}}
	return c.put(ctx, guestPath(node, typ, vmid)+"/resize", form)
}
