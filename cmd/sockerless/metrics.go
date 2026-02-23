package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func cmdMetrics() {
	frontendAddr, backendAddr := activeAddrs()
	if frontendAddr == "" && backendAddr == "" {
		fmt.Fprintln(os.Stderr, "error: no server addresses configured in active context")
		os.Exit(1)
	}

	if frontendAddr != "" {
		data, err := mgmtGet(frontendAddr, "/metrics")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Frontend metrics unavailable: %v\n", err)
		} else {
			var m map[string]any
			json.Unmarshal(data, &m)
			fmt.Println("=== Frontend Metrics ===")
			printMetricsMap(m)
			fmt.Println()
		}
	}

	if backendAddr != "" {
		data, err := mgmtGet(backendAddr, "/internal/v1/metrics")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Backend metrics unavailable: %v\n", err)
		} else {
			var m map[string]any
			json.Unmarshal(data, &m)
			fmt.Println("=== Backend Metrics ===")
			printMetricsMap(m)
		}
	}
}

func printMetricsMap(m map[string]any) {
	for k, v := range m {
		switch val := v.(type) {
		case map[string]any:
			fmt.Printf("  %s:\n", k)
			for sk, sv := range val {
				fmt.Printf("    %-30s %v\n", sk, sv)
			}
		case float64:
			if val == float64(int64(val)) {
				fmt.Printf("  %-30s %d\n", k, int64(val))
			} else {
				fmt.Printf("  %-30s %.2f\n", k, val)
			}
		default:
			fmt.Printf("  %-30s %v\n", k, v)
		}
	}
}
