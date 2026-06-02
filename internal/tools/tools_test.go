package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/banux/go-mcp-proxmox/internal/proxmox"
)

// fakeProxmox is an httptest server mimicking the Proxmox VE API.
type fakeProxmox struct {
	*httptest.Server
	requests atomic.Int64 // number of API requests received
}

func newFakeProxmox(t *testing.T, handler http.HandlerFunc) *fakeProxmox {
	t.Helper()
	f := &fakeProxmox{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		handler(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

// defaultHandler serves a small two-node cluster.
func defaultHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api2/json/nodes":
		w.Write([]byte(`{"data": [
			{"node": "pve1", "status": "online"},
			{"node": "pve2", "status": "online"},
			{"node": "pve3", "status": "offline"}
		]}`))
	case "/api2/json/nodes/pve1/qemu":
		w.Write([]byte(`{"data": [{"vmid": 100, "name": "web", "status": "running"}]}`))
	case "/api2/json/nodes/pve2/qemu":
		w.Write([]byte(`{"data": [{"vmid": 200, "name": "db", "status": "stopped"}]}`))
	case "/api2/json/nodes/pve1/lxc", "/api2/json/nodes/pve2/lxc":
		w.Write([]byte(`{"data": []}`))
	case "/api2/json/nodes/pve1/status":
		w.Write([]byte(`{"data": {"uptime": 12345, "loadavg": ["0.1", "0.2", "0.3"]}}`))
	case "/api2/json/storage":
		w.Write([]byte(`{"data": [{"storage": "local", "type": "dir"}]}`))
	case "/api2/json/cluster/resources":
		w.Write([]byte(`{"data": [{"id": "qemu/100", "type": "qemu", "node": "pve1"}]}`))
	default:
		http.NotFound(w, r)
	}
}

// newSession wires a fake Proxmox API to a server with all tools registered,
// and returns a connected in-memory MCP client session.
func newSession(t *testing.T, fake *fakeProxmox) *mcp.ClientSession {
	t.Helper()
	client := proxmox.NewClient(proxmox.Config{
		URL:         fake.URL,
		TokenID:     "mcp@pve!test",
		TokenSecret: "secret",
	})
	server := mcp.NewServer(&mcp.Implementation{Name: "go-mcp-proxmox", Version: "test"}, nil)
	Register(server, client)

	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), st, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("tool result has no content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *mcp.TextContent", res.Content[0])
	}
	return text.Text
}

func TestToolsAreRegistered(t *testing.T) {
	session := newSession(t, newFakeProxmox(t, defaultHandler))
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	want := map[string]bool{
		"list_nodes": false, "get_node_status": false, "list_vms": false,
		"list_lxc": false, "list_storage": false, "get_cluster_resources": false,
	}
	for _, tool := range res.Tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestHappyPaths(t *testing.T) {
	tests := []struct {
		tool string
		args map[string]any
		want []string // substrings expected in the JSON output
	}{
		{"list_nodes", nil, []string{"pve1", "pve2", "online"}},
		{"get_node_status", map[string]any{"node": "pve1"}, []string{"12345", "loadavg"}},
		{"list_vms", map[string]any{"node": "pve1"}, []string{"web", `"node": "pve1"`}},
		{"list_storage", nil, []string{"local", "dir"}},
		{"get_cluster_resources", map[string]any{"type": "vm"}, []string{"qemu/100"}},
	}
	session := newSession(t, newFakeProxmox(t, defaultHandler))
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			res := callTool(t, session, tt.tool, tt.args)
			if res.IsError {
				t.Fatalf("unexpected tool error: %s", textOf(t, res))
			}
			text := textOf(t, res)
			for _, want := range tt.want {
				if !strings.Contains(text, want) {
					t.Errorf("output does not contain %q:\n%s", want, text)
				}
			}
		})
	}
}

func TestListVMsAggregatesAcrossOnlineNodes(t *testing.T) {
	session := newSession(t, newFakeProxmox(t, defaultHandler))
	res := callTool(t, session, "list_vms", nil)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", textOf(t, res))
	}
	var vms []map[string]any
	if err := json.Unmarshal([]byte(textOf(t, res)), &vms); err != nil {
		t.Fatalf("output is not a JSON array: %v", err)
	}
	if len(vms) != 2 {
		t.Fatalf("got %d VMs, want 2 (offline node pve3 must be skipped)", len(vms))
	}
	nodes := map[string]bool{}
	for _, vm := range vms {
		nodes[vm["node"].(string)] = true
	}
	if !nodes["pve1"] || !nodes["pve2"] {
		t.Errorf("entries must be tagged with their node, got %v", nodes)
	}
}

func TestMissingRequiredParamSkipsAPI(t *testing.T) {
	fake := newFakeProxmox(t, defaultHandler)
	session := newSession(t, fake)
	before := fake.requests.Load()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_node_status", Arguments: map[string]any{},
	})
	// The SDK may reject the call at schema validation (protocol error) or the
	// handler may return a tool error; both are acceptable as long as the
	// Proxmox API is never contacted.
	if err == nil && !res.IsError {
		t.Fatal("call without required 'node' must fail")
	}
	if got := fake.requests.Load(); got != before {
		t.Errorf("Proxmox API was contacted %d time(s); want 0", got-before)
	}
}

func TestPermissionErrorSurfacedAsToolError(t *testing.T) {
	fake := newFakeProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	session := newSession(t, fake)

	res := callTool(t, session, "list_nodes", nil)
	if !res.IsError {
		t.Fatal("403 from Proxmox must surface as a tool error")
	}
	if text := textOf(t, res); !strings.Contains(text, "privileges") {
		t.Errorf("tool error should mention token privileges, got: %s", text)
	}

	// The server must keep serving after a failing tool call.
	res = callTool(t, session, "get_cluster_resources", map[string]any{"type": "bogus"})
	if !res.IsError {
		t.Error("invalid type must produce a tool error")
	}
}
