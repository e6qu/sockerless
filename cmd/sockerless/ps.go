package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func cmdPs() {
	_, backendAddr := activeAddrs()
	if backendAddr == "" {
		fmt.Fprintln(os.Stderr, "error: no backend_addr configured in active context")
		os.Exit(1)
	}

	data, err := mgmtGet(backendAddr, "/internal/v1/containers/summary")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var entries []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Image   string `json:"image"`
		State   string `json:"state"`
		Created string `json:"created"`
		PodName string `json:"pod_name"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid response: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("No containers")
		return
	}

	fmt.Printf("%-12s  %-20s  %-30s  %-10s  %s\n", "ID", "NAME", "IMAGE", "STATE", "POD")
	for _, e := range entries {
		id := e.ID
		if len(id) > 12 {
			id = id[:12]
		}
		name := e.Name
		if len(name) > 20 {
			name = name[:20]
		}
		image := e.Image
		if len(image) > 30 {
			image = image[:30]
		}
		fmt.Printf("%-12s  %-20s  %-30s  %-10s  %s\n", id, name, image, e.State, e.PodName)
	}
}
