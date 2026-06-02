// Package tools registers the MCP tools exposed by go-mcp-proxmox.
//
// All tools in this package are read-only: they only ever issue GET requests
// against the Proxmox VE API. Handler errors are returned as regular Go
// errors; the MCP SDK converts them into tool results with isError=true, so a
// failing tool never crashes the server.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/banux/go-mcp-proxmox/internal/proxmox"
)

// Register adds every available tool to the server.
func Register(server *mcp.Server, client *proxmox.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_nodes",
		Description: "List the Proxmox cluster nodes with their status and resource usage.",
	}, listNodes(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_node_status",
		Description: "Get the detailed status of one Proxmox node (CPU, memory, load, kernel...).",
	}, getNodeStatus(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_vms",
		Description: "List QEMU virtual machines. Scoped to one node if 'node' is given, otherwise aggregated over every online node.",
	}, listGuests(client, (*proxmox.Client).ListQemu))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_lxc",
		Description: "List LXC containers. Scoped to one node if 'node' is given, otherwise aggregated over every online node.",
	}, listGuests(client, (*proxmox.Client).ListLXC))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_storage",
		Description: "List the storage configuration of the cluster, or the storages of one node if 'node' is given.",
	}, listStorage(client))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_cluster_resources",
		Description: "List cluster resources (VMs, containers, storages, nodes), optionally filtered by type: vm, storage, node or sdn.",
	}, getClusterResources(client))
}

// jsonResult marshals v as indented JSON into a tool result.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding tool result: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(out)}},
	}, nil
}

type emptyArgs struct{}

type nodeRequiredArgs struct {
	Node string `json:"node" jsonschema:"name of the Proxmox node"`
}

type nodeOptionalArgs struct {
	Node string `json:"node,omitempty" jsonschema:"optional name of a Proxmox node"`
}

type clusterResourcesArgs struct {
	Type string `json:"type,omitempty" jsonschema:"optional resource type filter: vm, storage, node or sdn"`
}

func listNodes(client *proxmox.Client) mcp.ToolHandlerFor[emptyArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
		nodes, err := client.ListNodes(ctx)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(nodes)
		return res, nil, err
	}
}

func getNodeStatus(client *proxmox.Client) mcp.ToolHandlerFor[nodeRequiredArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args nodeRequiredArgs) (*mcp.CallToolResult, any, error) {
		if args.Node == "" {
			return nil, nil, fmt.Errorf("missing required parameter: node")
		}
		status, err := client.GetNodeStatus(ctx, args.Node)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(json.RawMessage(status))
		return res, nil, err
	}
}

// listGuests builds the handler shared by list_vms and list_lxc. listFn is the
// per-node listing method (ListQemu or ListLXC).
func listGuests(client *proxmox.Client, listFn func(*proxmox.Client, context.Context, string) ([]proxmox.VM, error)) mcp.ToolHandlerFor[nodeOptionalArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args nodeOptionalArgs) (*mcp.CallToolResult, any, error) {
		if args.Node != "" {
			guests, err := listFn(client, ctx, args.Node)
			if err != nil {
				return nil, nil, err
			}
			for i := range guests {
				guests[i].Node = args.Node
			}
			res, err := jsonResult(guests)
			return res, nil, err
		}

		nodes, err := client.ListNodes(ctx)
		if err != nil {
			return nil, nil, err
		}
		all := []proxmox.VM{}
		for _, node := range nodes {
			if node.Status != "online" {
				continue
			}
			guests, err := listFn(client, ctx, node.Node)
			if err != nil {
				return nil, nil, fmt.Errorf("listing guests on node %q: %w", node.Node, err)
			}
			for i := range guests {
				guests[i].Node = node.Node
			}
			all = append(all, guests...)
		}
		res, err := jsonResult(all)
		return res, nil, err
	}
}

func listStorage(client *proxmox.Client) mcp.ToolHandlerFor[nodeOptionalArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args nodeOptionalArgs) (*mcp.CallToolResult, any, error) {
		storage, err := client.ListStorage(ctx, args.Node)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(json.RawMessage(storage))
		return res, nil, err
	}
}

func getClusterResources(client *proxmox.Client) mcp.ToolHandlerFor[clusterResourcesArgs, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args clusterResourcesArgs) (*mcp.CallToolResult, any, error) {
		switch args.Type {
		case "", "vm", "storage", "node", "sdn":
		default:
			return nil, nil, fmt.Errorf("invalid type %q: must be one of vm, storage, node, sdn", args.Type)
		}
		resources, err := client.GetClusterResources(ctx, args.Type)
		if err != nil {
			return nil, nil, err
		}
		res, err := jsonResult(json.RawMessage(resources))
		return res, nil, err
	}
}
