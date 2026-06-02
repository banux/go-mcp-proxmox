package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func testConfig(serverURL string) Config {
	return Config{
		URL:         serverURL,
		TokenID:     "mcp@pve!claude",
		TokenSecret: "secret-uuid",
	}
}

func TestClientSendsTokenHeaderOnly(t *testing.T) {
	var gotAuth string
	var hadCookie, hadCSRF bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, err := r.Cookie("PVEAuthCookie")
		hadCookie = err == nil
		hadCSRF = r.Header.Get("CSRFPreventionToken") != ""
		w.Write([]byte(`{"data": []}`))
	}))
	defer srv.Close()

	client := NewClient(testConfig(srv.URL))
	if _, err := client.ListNodes(context.Background()); err != nil {
		t.Fatalf("ListNodes: %v", err)
	}

	want := "PVEAPIToken=mcp@pve!claude=secret-uuid"
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
	if hadCookie || hadCSRF {
		t.Errorf("request must not carry ticket auth: cookie=%t csrf=%t", hadCookie, hadCSRF)
	}
}

func TestClientUnwrapsDataEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Write([]byte(`{"data": [{"node": "pve1", "status": "online"}]}`))
	}))
	defer srv.Close()

	client := NewClient(testConfig(srv.URL))
	nodes, err := client.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Node != "pve1" || nodes[0].Status != "online" {
		t.Errorf("unexpected nodes: %+v", nodes)
	}
}

func TestClientMapsPermissionErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		client := NewClient(testConfig(srv.URL))
		_, err := client.ListNodes(context.Background())
		srv.Close()

		if err == nil {
			t.Fatalf("status %d: want error, got nil", status)
		}
		msg := err.Error()
		if !strings.Contains(msg, "permission denied") || !strings.Contains(msg, "privileges") {
			t.Errorf("status %d: error should mention permission/privileges, got %q", status, msg)
		}
	}
}

func TestClientMapsServerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"errors": {"node": "no such node 'pve9'"}}`))
	}))
	defer srv.Close()

	client := NewClient(testConfig(srv.URL))
	_, err := client.GetNodeStatus(context.Background(), "pve9")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "500") || !strings.Contains(msg, "no such node") {
		t.Errorf("error should contain status and Proxmox body, got %q", msg)
	}
}

func TestClientTLSPolicy(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data": []}`))
	}))
	defer srv.Close()

	t.Run("default rejects untrusted cert", func(t *testing.T) {
		client := NewClient(testConfig(srv.URL))
		_, err := client.ListNodes(context.Background())
		if err == nil {
			t.Fatal("want TLS error against self-signed cert, got nil")
		}
	})

	t.Run("insecure opt-in accepts untrusted cert", func(t *testing.T) {
		cfg := testConfig(srv.URL)
		cfg.InsecureTLS = true
		client := NewClient(cfg)
		if _, err := client.ListNodes(context.Background()); err != nil {
			t.Fatalf("ListNodes with InsecureTLS: %v", err)
		}
	})
}

func TestClientQueryParameters(t *testing.T) {
	var gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/cluster/resources" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		gotType = r.URL.Query().Get("type")
		w.Write([]byte(`{"data": [{"id": "qemu/100", "type": "qemu"}]}`))
	}))
	defer srv.Close()

	client := NewClient(testConfig(srv.URL))
	raw, err := client.GetClusterResources(context.Background(), "vm")
	if err != nil {
		t.Fatalf("GetClusterResources: %v", err)
	}
	if gotType != "vm" {
		t.Errorf("type query = %q, want %q", gotType, "vm")
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil || len(items) != 1 {
		t.Errorf("unexpected payload %s (err=%v)", raw, err)
	}
}

// captureRequest records the method, path and parsed form of the request, and
// replies with a UPID payload.
func captureRequest(t *testing.T, method, path *string, form *url.Values) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*method = r.Method
		*path = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		*form = r.PostForm
		w.Write([]byte(`{"data": "UPID:thor:00001234:..:task:"}`))
	}
}

func TestClientMutationsSendCorrectRequests(t *testing.T) {
	const wantUPID = "UPID:thor:00001234:..:task:"
	tests := []struct {
		name       string
		call       func(c *Client) (string, error)
		wantMethod string
		wantPath   string
		wantForm   map[string]string
	}{
		{
			name:       "start guest",
			call:       func(c *Client) (string, error) { return c.StartGuest(context.Background(), "thor", "qemu", 103) },
			wantMethod: http.MethodPost,
			wantPath:   "/api2/json/nodes/thor/qemu/103/status/start",
		},
		{
			name: "create snapshot",
			call: func(c *Client) (string, error) {
				return c.CreateSnapshot(context.Background(), "thor", "qemu", 103, "before-upgrade", "pre", true)
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api2/json/nodes/thor/qemu/103/snapshot",
			wantForm:   map[string]string{"snapname": "before-upgrade", "description": "pre", "vmstate": "1"},
		},
		{
			name: "rollback snapshot",
			call: func(c *Client) (string, error) {
				return c.RollbackSnapshot(context.Background(), "thor", "lxc", 200, "before-upgrade")
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api2/json/nodes/thor/lxc/200/snapshot/before-upgrade/rollback",
		},
		{
			name: "clone vm",
			call: func(c *Client) (string, error) {
				return c.CloneVM(context.Background(), "loki", 100, 200, "clone", "thor", true)
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api2/json/nodes/loki/qemu/100/clone",
			wantForm:   map[string]string{"newid": "200", "name": "clone", "target": "thor", "full": "1"},
		},
		{
			name:       "delete guest",
			call:       func(c *Client) (string, error) { return c.DeleteGuest(context.Background(), "thor", "qemu", 103) },
			wantMethod: http.MethodDelete,
			wantPath:   "/api2/json/nodes/thor/qemu/103",
		},
		{
			name: "migrate guest online",
			call: func(c *Client) (string, error) {
				return c.MigrateGuest(context.Background(), "thor", "qemu", 103, "loki", true)
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api2/json/nodes/thor/qemu/103/migrate",
			wantForm:   map[string]string{"target": "loki", "online": "1"},
		},
		{
			name: "resize disk",
			call: func(c *Client) (string, error) {
				return c.ResizeDisk(context.Background(), "thor", "qemu", 103, "scsi0", "+10G")
			},
			wantMethod: http.MethodPut,
			wantPath:   "/api2/json/nodes/thor/qemu/103/resize",
			wantForm:   map[string]string{"disk": "scsi0", "size": "+10G"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			var gotForm url.Values
			srv := httptest.NewServer(captureRequest(t, &gotMethod, &gotPath, &gotForm))
			defer srv.Close()

			upid, err := tt.call(NewClient(testConfig(srv.URL)))
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if upid != wantUPID {
				t.Errorf("UPID = %q, want %q", upid, wantUPID)
			}
			if gotMethod != tt.wantMethod {
				t.Errorf("method = %q, want %q", gotMethod, tt.wantMethod)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
			for k, want := range tt.wantForm {
				if got := gotForm.Get(k); got != want {
					t.Errorf("form[%q] = %q, want %q", k, got, want)
				}
			}
		})
	}
}

func TestClientTaskStatus(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"data": {"status": "stopped", "exitstatus": "OK"}}`))
	}))
	defer srv.Close()

	client := NewClient(testConfig(srv.URL))
	raw, err := client.TaskStatus(context.Background(), "thor", "UPID:thor:00001234:..:task:")
	if err != nil {
		t.Fatalf("TaskStatus: %v", err)
	}
	if want := "/api2/json/nodes/thor/tasks/UPID:thor:00001234:..:task:/status"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if !strings.Contains(string(raw), "exitstatus") {
		t.Errorf("status payload missing exitstatus: %s", raw)
	}
}
