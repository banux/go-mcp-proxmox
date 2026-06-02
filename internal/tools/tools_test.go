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

// newSession wires a fake Proxmox API to a server with the read-only tools
// registered, and returns a connected in-memory MCP client session.
func newSession(t *testing.T, fake *fakeProxmox) *mcp.ClientSession {
	return newSessionWrite(t, fake, false)
}

// newSessionWrite is like newSession but lets the test choose whether the
// mutating tools are registered.
func newSessionWrite(t *testing.T, fake *fakeProxmox, allowWrite bool) *mcp.ClientSession {
	t.Helper()
	client := proxmox.NewClient(proxmox.Config{
		URL:         fake.URL,
		TokenID:     "mcp@pve!test",
		TokenSecret: "secret",
	})
	server := mcp.NewServer(&mcp.Implementation{Name: "go-mcp-proxmox", Version: "test"}, nil)
	Register(server, client, allowWrite)

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

// writeToolNames are the mutating tools registered only when write is enabled.
var writeToolNames = []string{
	"start_guest", "stop_guest", "shutdown_guest", "reboot_guest",
	"list_snapshots", "create_snapshot", "rollback_snapshot", "delete_snapshot",
	"clone_vm", "delete_guest", "migrate_guest", "resize_disk",
}

func listToolNames(t *testing.T, session *mcp.ClientSession) map[string]bool {
	t.Helper()
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	return names
}

func TestWriteToolsAbsentWhenDisabled(t *testing.T) {
	session := newSessionWrite(t, newFakeProxmox(t, defaultHandler), false)
	names := listToolNames(t, session)
	for _, name := range writeToolNames {
		if names[name] {
			t.Errorf("mutating tool %q must not be registered when write is disabled", name)
		}
	}
	// Read-only tools remain available.
	if !names["list_nodes"] {
		t.Error("read-only tools must stay registered")
	}
}

func TestWriteToolsPresentWhenEnabled(t *testing.T) {
	session := newSessionWrite(t, newFakeProxmox(t, defaultHandler), true)
	names := listToolNames(t, session)
	for _, name := range writeToolNames {
		if !names[name] {
			t.Errorf("mutating tool %q must be registered when write is enabled", name)
		}
	}
}

func TestWriteToolHappyPath(t *testing.T) {
	var gotMethod, gotPath string
	fake := newFakeProxmox(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Write([]byte(`{"data": "UPID:thor:00001234:..:task:"}`))
	})
	session := newSessionWrite(t, fake, true)

	res := callTool(t, session, "start_guest", map[string]any{"node": "thor", "vmid": 103, "type": "qemu"})
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", textOf(t, res))
	}
	if gotMethod != http.MethodPost || gotPath != "/api2/json/nodes/thor/qemu/103/status/start" {
		t.Errorf("unexpected request %s %s", gotMethod, gotPath)
	}
	if text := textOf(t, res); !strings.Contains(text, "UPID:thor") {
		t.Errorf("result should contain the task UPID, got: %s", text)
	}
}

func TestWriteToolInvalidTypeSkipsAPI(t *testing.T) {
	fake := newFakeProxmox(t, defaultHandler)
	session := newSessionWrite(t, fake, true)
	before := fake.requests.Load()

	res := callTool(t, session, "stop_guest", map[string]any{"node": "thor", "vmid": 103, "type": "bogus"})
	if !res.IsError {
		t.Fatal("invalid guest type must produce a tool error")
	}
	if got := fake.requests.Load(); got != before {
		t.Errorf("Proxmox API was contacted %d time(s) for an invalid type; want 0", got-before)
	}
}

func TestWriteToolMissingParamSkipsAPI(t *testing.T) {
	fake := newFakeProxmox(t, defaultHandler)
	session := newSessionWrite(t, fake, true)
	before := fake.requests.Load()

	// vmid omitted: rejected at schema validation or by the handler, never reaching the API.
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "start_guest", Arguments: map[string]any{"node": "thor", "type": "qemu"},
	})
	if err == nil && !res.IsError {
		t.Fatal("call without required 'vmid' must fail")
	}
	if got := fake.requests.Load(); got != before {
		t.Errorf("Proxmox API was contacted %d time(s); want 0", got-before)
	}
}
