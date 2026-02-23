package main

import (
	"os"
	"path/filepath"
	"strings"
)

func sockerlessDir() string {
	if d := os.Getenv("SOCKERLESS_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sockerless")
}

func contextDir(name string) string {
	return filepath.Join(sockerlessDir(), "contexts", name)
}

func activeFile() string {
	return filepath.Join(sockerlessDir(), "active")
}

func activeContextName() string {
	if name := os.Getenv("SOCKERLESS_CONTEXT"); name != "" {
		return name
	}
	data, err := os.ReadFile(activeFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
