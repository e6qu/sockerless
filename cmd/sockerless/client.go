package main

import (
	"fmt"
	"io"
	"net/http"
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

// activeAddr reads the server address from the active context in config.yaml.
// Legacy per-context JSON files are no longer consulted; operators on older
// state must run `sockerless config migrate`.
func activeAddr() string {
	name := activeContextName()
	if name == "" {
		return ""
	}
	if !configFileExists() {
		return ""
	}
	cfg, err := loadConfigFile()
	if err != nil {
		return ""
	}
	env, ok := cfg.Environments[name]
	if !ok {
		return ""
	}
	return env.Addr
}
