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

	frontendAddr, backendAddr := activeAddrs()
	fmt.Printf("Context: %s\n", name)

	if frontendAddr == "" && backendAddr == "" {
		fmt.Println("No server addresses configured (offline mode)")
		return
	}

	if frontendAddr != "" {
		data, err := mgmtGet(frontendAddr, "/healthz")
		if err != nil {
			fmt.Printf("Frontend (%s): DOWN (%v)\n", frontendAddr, err)
		} else {
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not parse frontend health response: %v\n", err)
			}
			uptime, _ := resp["uptime_seconds"].(float64)
			fmt.Printf("Frontend (%s): UP (uptime: %ds)\n", frontendAddr, int(uptime))

			// Get detailed status
			if sdata, err := mgmtGet(frontendAddr, "/status"); err == nil {
				var status map[string]any
				if err := json.Unmarshal(sdata, &status); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not parse frontend status response: %v\n", err)
				}
				if da, ok := status["docker_addr"].(string); ok && da != "" {
					fmt.Printf("  Docker API: %s\n", da)
				}
			}
		}
	}

	if backendAddr != "" {
		data, err := mgmtGet(backendAddr, "/internal/v1/healthz")
		if err != nil {
			fmt.Printf("Backend  (%s): DOWN (%v)\n", backendAddr, err)
		} else {
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not parse backend health response: %v\n", err)
			}
			uptime, _ := resp["uptime_seconds"].(float64)
			fmt.Printf("Backend  (%s): UP (uptime: %ds)\n", backendAddr, int(uptime))

			// Get detailed status
			if sdata, err := mgmtGet(backendAddr, "/internal/v1/status"); err == nil {
				var status map[string]any
				if err := json.Unmarshal(sdata, &status); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not parse backend status response: %v\n", err)
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
		}
	}
}
