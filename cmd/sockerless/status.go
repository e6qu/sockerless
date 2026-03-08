package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func cmdStatus() {
	name := activeContextName()
	if name == "" {
		fmt.Println("No active context")
		os.Exit(1)
	}

	addr := activeAddr()
	fmt.Printf("Context: %s\n", name)

	if addr == "" {
		fmt.Println("No server address configured (offline mode)")
		return
	}

	data, err := mgmtGet(addr, "/internal/v1/healthz")
	if err != nil {
		fmt.Printf("Server (%s): DOWN (%v)\n", addr, err)
		return
	}

	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not parse health response: %v\n", err)
	}
	uptime, _ := resp["uptime_seconds"].(float64)
	fmt.Printf("Server (%s): UP (uptime: %ds)\n", addr, int(uptime))

	sdata, err := mgmtGet(addr, "/internal/v1/status")
	if err != nil {
		return
	}
	var status map[string]any
	if err := json.Unmarshal(sdata, &status); err != nil {
		return
	}
	if bt, ok := status["backend_type"].(string); ok {
		fmt.Printf("  Backend type: %s\n", bt)
	}
	if c, ok := status["containers"].(float64); ok {
		fmt.Printf("  Containers: %d\n", int(c))
	}
	if r, ok := status["active_resources"].(float64); ok {
		fmt.Printf("  Active resources: %d\n", int(r))
	}
}
