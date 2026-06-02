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
	reqURL := c.baseURL + apiBase + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request %s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")

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
