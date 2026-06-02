package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/banux/go-mcp-proxmox/internal/proxmox"
)

// buildBinary compiles the server once per test run.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "go-mcp-proxmox")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func TestStdioSmoke(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data": []}`))
	}))
	defer fake.Close()

	bin := buildBinary(t)
	cmd := exec.Command(bin)
	cmd.Env = []string{
		proxmox.EnvURL + "=" + fake.URL,
		proxmox.EnvTokenID + "=mcp@pve!smoke",
		proxmox.EnvTokenSecret + "=secret",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := mcp.NewClient(&mcp.Implementation{Name: "smoke-test"}, nil).
		Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("initialize over stdio failed: %v", err)
	}
	defer session.Close()

	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"list_nodes", "get_node_status", "list_vms", "list_lxc", "list_storage", "get_cluster_resources"} {
		if !got[want] {
			t.Errorf("tools/list missing %q (got %v)", want, res.Tools)
		}
	}
}

func TestFailFastOnMissingConfig(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin)
	cmd.Env = []string{} // no PROXMOX_* variables at all

	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
		t.Fatalf("want non-zero exit, got err=%v output=%s", err, out)
	}
	for _, want := range []string{proxmox.EnvURL, proxmox.EnvTokenID, proxmox.EnvTokenSecret} {
		if !strings.Contains(string(out), want) {
			t.Errorf("stderr should name missing variable %s, got:\n%s", want, out)
		}
	}
}
