package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func cmdResources(args []string) {
	if len(args) < 1 {
		resourcesUsage()
		os.Exit(1)
	}
	switch args[0] {
	case "list", "ls":
		resourcesList()
	case "orphaned":
		resourcesOrphaned()
	case "cleanup":
		resourcesCleanup()
	default:
		resourcesUsage()
		os.Exit(1)
	}
}

func resourcesUsage() {
	fmt.Fprintln(os.Stderr, `Usage: sockerless resources <subcommand>

Subcommands:
  list       List active cloud resources
  orphaned   List orphaned cloud resources
  cleanup    Clean up orphaned resources`)
}

func resourcesList() {
	_, backendAddr := activeAddrs()
	if backendAddr == "" {
		fmt.Fprintln(os.Stderr, "error: no backend_addr configured in active context")
		os.Exit(1)
	}

	data, err := mgmtGet(backendAddr, "/internal/v1/resources?active=true")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var entries []map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("No active resources")
		return
	}

	fmt.Printf("%-40s  %-12s  %-12s  %s\n", "RESOURCE ID", "TYPE", "BACKEND", "CONTAINER")
	for _, e := range entries {
		rid, _ := e["resourceId"].(string)
		rtype, _ := e["resourceType"].(string)
		backend, _ := e["backend"].(string)
		cid, _ := e["containerId"].(string)
		if len(rid) > 40 {
			rid = rid[:40]
		}
		if len(cid) > 12 {
			cid = cid[:12]
		}
		fmt.Printf("%-40s  %-12s  %-12s  %s\n", rid, rtype, backend, cid)
	}
}

func resourcesOrphaned() {
	_, backendAddr := activeAddrs()
	if backendAddr == "" {
		fmt.Fprintln(os.Stderr, "error: no backend_addr configured in active context")
		os.Exit(1)
	}

	data, err := mgmtGet(backendAddr, "/internal/v1/resources/orphaned")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var entries []map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("No orphaned resources")
		return
	}

	fmt.Printf("Found %d orphaned resource(s):\n", len(entries))
	for _, e := range entries {
		rid, _ := e["resourceId"].(string)
		rtype, _ := e["resourceType"].(string)
		fmt.Printf("  %s (%s)\n", rid, rtype)
	}
}

func resourcesCleanup() {
	_, backendAddr := activeAddrs()
	if backendAddr == "" {
		fmt.Fprintln(os.Stderr, "error: no backend_addr configured in active context")
		os.Exit(1)
	}

	data, err := mgmtPost(backendAddr, "/internal/v1/resources/cleanup")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var resp map[string]any
	json.Unmarshal(data, &resp)
	cleaned, _ := resp["cleaned"].(float64)
	fmt.Printf("Cleaned up %d resource(s)\n", int(cleaned))
}
