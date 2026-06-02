package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
