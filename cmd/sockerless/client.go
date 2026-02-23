package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// mgmtGet performs a GET request to the given addr+path with a 5s timeout.
func mgmtGet(addr, path string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(addr + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// mgmtPost performs a POST request to the given addr+path with a 5s timeout.
func mgmtPost(addr, path string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(addr+path, "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// activeAddrs reads frontend_addr and backend_addr from the active context.
func activeAddrs() (frontendAddr, backendAddr string) {
	name := activeContextName()
	if name == "" {
		return "", ""
	}
	data, err := os.ReadFile(filepath.Join(contextDir(name), "config.json"))
	if err != nil {
		return "", ""
	}
	var cfg contextConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", ""
	}
	return cfg.FrontendAddr, cfg.BackendAddr
}
