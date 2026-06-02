package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/banux/go-mcp-proxmox/internal/proxmox"
)

// registerWriteTools adds the mutating tools. It is only called when write
// access is enabled (PROXMOX_ALLOW_WRITE=true).
func registerWriteTools(server *mcp.Server, client *proxmox.Client) {
	// Lifecycle
	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_guest",
		Description: "Start a VM or container. Returns the Proxmox task UPID.",
	}, lifecycle(client, (*proxmox.Client).StartGuest))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "stop_guest",
		Description: "Stop (hard power-off) a VM or container. Returns the Proxmox task UPID.",
	}, lifecycle(client, (*proxmox.Client).StopGuest))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "shutdown_guest",
		Description: "Gracefully shut down a VM or container. Returns the Proxmox task UPID.",
	}, lifecycle(client, (*proxmox.Client).ShutdownGuest))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "reboot_guest",
		Description: "Reboot a VM or container. Returns the Proxmox task UPID.",
	}, lifecycle(client, (*proxmox.Client).RebootGuest))

	// Snapshots
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_snapshots",
		Description: "List the snapshots of a VM or container.",
	}, listSnapshots(client))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_snapshot",
		Description: "Create a snapshot of a VM or container. Returns the Proxmox task UPID.",
	}, createSnapshot(client))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rollback_snapshot",
		Description: "Roll a VM or container back to a snapshot. Returns the Proxmox task UPID.",
	}, rollbackSnapshot(client))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_snapshot",
		Description: "Delete a snapshot of a VM or container. Returns the Proxmox task UPID.",
	}, deleteSnapshot(client))

	// Management
	mcp.AddTool(server, &mcp.Tool{
		Name:        "clone_vm",
		Description: "Clone a QEMU VM to a new VMID. Returns the Proxmox task UPID.",
	}, cloneVM(client))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_guest",
		Description: "Destroy a VM or container. Returns the Proxmox task UPID.",
	}, deleteGuest(client))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "migrate_guest",
		Description: "Migrate a VM or container to another node. Returns the Proxmox task UPID.",
	}, migrateGuest(client))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "resize_disk",
		Description: "Grow a VM or container disk (e.g. disk \"scsi0\", size \"+10G\"). Returns the Proxmox task UPID.",
	}, resizeDisk(client))
}

// guestArgs identifies a single guest. type is qemu or lxc.
type guestArgs struct {
	Node string `json:"node" jsonschema:"name of the Proxmox node"`
	VMID int    `json:"vmid" jsonschema:"numeric id of the guest"`
	Type string `json:"type" jsonschema:"guest type: qemu or lxc"`
}

type snapshotArgs struct {
	guestArgs
	Snapname    string `json:"snapname" jsonschema:"name of the snapshot"`
	Description string `json:"description,omitempty" jsonschema:"optional snapshot description"`
	VMState     bool   `json:"vmstate,omitempty" jsonschema:"include the VM RAM state in the snapshot (QEMU only)"`
}

type snapshotNameArgs struct {
	guestArgs
	Snapname string `json:"snapname" jsonschema:"name of the snapshot"`
}

type cloneArgs struct {
	Node   string `json:"node" jsonschema:"name of the Proxmox node"`
	VMID   int    `json:"vmid" jsonschema:"numeric id of the source VM"`
	NewID  int    `json:"newid" jsonschema:"numeric id of the new VM"`
	Name   string `json:"name,omitempty" jsonschema:"optional name for the new VM"`
	Target string `json:"target,omitempty" jsonschema:"optional target node for the clone"`
	Full   bool   `json:"full,omitempty" jsonschema:"create a full (non-linked) clone"`
}

type migrateArgs struct {
	guestArgs
	Target string `json:"target" jsonschema:"target node to migrate to"`
	Online bool   `json:"online,omitempty" jsonschema:"perform an online (live) migration"`
}

type resizeArgs struct {
	guestArgs
	Disk string `json:"disk" jsonschema:"disk to resize, e.g. scsi0 or rootfs"`
	Size string `json:"size" jsonschema:"new size, absolute (e.g. 32G) or relative (e.g. +10G)"`
}

// validateGuest checks the common guest parameters.
func (g guestArgs) validate() error {
	if g.Node == "" {
		return fmt.Errorf("missing required parameter: node")
	}
	if g.VMID == 0 {
		return fmt.Errorf("missing required parameter: vmid")
	}
	switch g.Type {
	case "qemu", "lxc":
	default:
		return fmt.Errorf("invalid type %q: must be one of qemu, lxc", g.Type)
	}
	return nil
}

// taskResult reports a started task to the caller.
type taskResult struct {
	Node string `json:"node"`
	UPID string `json:"upid"`
}

// lifecycle builds a handler for a guest lifecycle operation. op is one of the
// client's lifecycle methods (StartGuest, StopGuest, ...).
func lifecycle(client *proxmox.Client, op func(*proxmox.Client, context.Context, string, string, int) (string, error)) mcp.ToolHandlerFor[guestArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args guestArgs) (*mcp.CallToolResult, any, error) {
		if err := args.validate(); err != nil {
			return nil, nil, err
		}
		upid, err := op(client, ctx, args.Node, args.Type, args.VMID)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(taskResult{Node: args.Node, UPID: upid})
		return res, nil, err
	}
}

func listSnapshots(client *proxmox.Client) mcp.ToolHandlerFor[guestArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args guestArgs) (*mcp.CallToolResult, any, error) {
		if err := args.validate(); err != nil {
			return nil, nil, err
		}
		snaps, err := client.ListSnapshots(ctx, args.Node, args.Type, args.VMID)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(snaps)
		return res, nil, err
	}
}

func createSnapshot(client *proxmox.Client) mcp.ToolHandlerFor[snapshotArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args snapshotArgs) (*mcp.CallToolResult, any, error) {
		if err := args.validate(); err != nil {
			return nil, nil, err
		}
		if args.Snapname == "" {
			return nil, nil, fmt.Errorf("missing required parameter: snapname")
		}
		upid, err := client.CreateSnapshot(ctx, args.Node, args.Type, args.VMID, args.Snapname, args.Description, args.VMState)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(taskResult{Node: args.Node, UPID: upid})
		return res, nil, err
	}
}

func rollbackSnapshot(client *proxmox.Client) mcp.ToolHandlerFor[snapshotNameArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args snapshotNameArgs) (*mcp.CallToolResult, any, error) {
		if err := args.validate(); err != nil {
			return nil, nil, err
		}
		if args.Snapname == "" {
			return nil, nil, fmt.Errorf("missing required parameter: snapname")
		}
		upid, err := client.RollbackSnapshot(ctx, args.Node, args.Type, args.VMID, args.Snapname)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(taskResult{Node: args.Node, UPID: upid})
		return res, nil, err
	}
}

func deleteSnapshot(client *proxmox.Client) mcp.ToolHandlerFor[snapshotNameArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args snapshotNameArgs) (*mcp.CallToolResult, any, error) {
		if err := args.validate(); err != nil {
			return nil, nil, err
		}
		if args.Snapname == "" {
			return nil, nil, fmt.Errorf("missing required parameter: snapname")
		}
		upid, err := client.DeleteSnapshot(ctx, args.Node, args.Type, args.VMID, args.Snapname)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(taskResult{Node: args.Node, UPID: upid})
		return res, nil, err
	}
}

func cloneVM(client *proxmox.Client) mcp.ToolHandlerFor[cloneArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args cloneArgs) (*mcp.CallToolResult, any, error) {
		if args.Node == "" {
			return nil, nil, fmt.Errorf("missing required parameter: node")
		}
		if args.VMID == 0 {
			return nil, nil, fmt.Errorf("missing required parameter: vmid")
		}
		if args.NewID == 0 {
			return nil, nil, fmt.Errorf("missing required parameter: newid")
		}
		upid, err := client.CloneVM(ctx, args.Node, args.VMID, args.NewID, args.Name, args.Target, args.Full)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(taskResult{Node: args.Node, UPID: upid})
		return res, nil, err
	}
}

func deleteGuest(client *proxmox.Client) mcp.ToolHandlerFor[guestArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args guestArgs) (*mcp.CallToolResult, any, error) {
		if err := args.validate(); err != nil {
			return nil, nil, err
		}
		upid, err := client.DeleteGuest(ctx, args.Node, args.Type, args.VMID)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(taskResult{Node: args.Node, UPID: upid})
		return res, nil, err
	}
}

func migrateGuest(client *proxmox.Client) mcp.ToolHandlerFor[migrateArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args migrateArgs) (*mcp.CallToolResult, any, error) {
		if err := args.validate(); err != nil {
			return nil, nil, err
		}
		if args.Target == "" {
			return nil, nil, fmt.Errorf("missing required parameter: target")
		}
		upid, err := client.MigrateGuest(ctx, args.Node, args.Type, args.VMID, args.Target, args.Online)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(taskResult{Node: args.Node, UPID: upid})
		return res, nil, err
	}
}

func resizeDisk(client *proxmox.Client) mcp.ToolHandlerFor[resizeArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args resizeArgs) (*mcp.CallToolResult, any, error) {
		if err := args.validate(); err != nil {
			return nil, nil, err
		}
		if args.Disk == "" {
			return nil, nil, fmt.Errorf("missing required parameter: disk")
		}
		if args.Size == "" {
			return nil, nil, fmt.Errorf("missing required parameter: size")
		}
		upid, err := client.ResizeDisk(ctx, args.Node, args.Type, args.VMID, args.Disk, args.Size)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(taskResult{Node: args.Node, UPID: upid})
		return res, nil, err
	}
}
