package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func cmdMetrics() {
	addr := activeAddr()
	if addr == "" {
		fmt.Fprintln(os.Stderr, "error: no server address configured in active context")
		os.Exit(1)
	}

	data, err := mgmtGet(addr, "/internal/v1/metrics")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Metrics unavailable: %v\n", err)
		os.Exit(1)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not parse metrics: %v\n", err)
		os.Exit(1)
	}
	printMetricsMap(m)
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
