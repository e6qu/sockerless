package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func cmdCheck() {
	_, backendAddr := activeAddrs()
	if backendAddr == "" {
		fmt.Fprintln(os.Stderr, "error: no backend_addr configured in active context")
		os.Exit(1)
	}

	data, err := mgmtGet(backendAddr, "/internal/v1/check")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var resp struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	allOk := true
	for _, c := range resp.Checks {
		icon := "OK"
		if c.Status != "ok" {
			icon = "FAIL"
			allOk = false
		}
		detail := ""
		if c.Detail != "" {
			detail = " (" + c.Detail + ")"
		}
		fmt.Printf("  [%s] %s%s\n", icon, c.Name, detail)
	}

	if !allOk {
		os.Exit(1)
	}
}
