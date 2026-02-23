package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func cmdServer(args []string) {
	if len(args) < 1 {
		serverUsage()
		os.Exit(1)
	}
	switch args[0] {
	case "start":
		serverStart(args[1:])
	case "stop":
		serverStop()
	case "restart":
		serverStop()
		time.Sleep(500 * time.Millisecond)
		serverStart(args[1:])
	default:
		serverUsage()
		os.Exit(1)
	}
}

func serverUsage() {
	fmt.Fprintln(os.Stderr, `Usage: sockerless server <subcommand>

Subcommands:
  start     Start frontend and backend servers
  stop      Stop running servers
  restart   Restart servers`)
}

func serverStart(args []string) {
	fs := flag.NewFlagSet("server start", flag.ExitOnError)
	backendBin := fs.String("backend-bin", "", "path to backend binary (default: sockerless-backend-{type})")
	frontendBin := fs.String("frontend-bin", "", "path to frontend binary (default: sockerless-frontend-docker)")
	backendAddr := fs.String("backend-addr", ":9100", "backend listen address")
	frontendAddr := fs.String("frontend-addr", ":2375", "frontend Docker API listen address")
	mgmtAddr := fs.String("mgmt-addr", ":9080", "frontend management API listen address")
	fs.Parse(args)

	name := activeContextName()
	if name == "" {
		fmt.Fprintln(os.Stderr, "error: no active context; run 'sockerless context use <name>' first")
		os.Exit(1)
	}

	// Read context to get backend type
	_, bAddr := activeAddrs()
	_ = bAddr

	data, err := os.ReadFile(filepath.Join(contextDir(name), "config.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	var cfg contextConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	runDir := serverRunDir(name)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Resolve backend binary
	bBin := *backendBin
	if bBin == "" {
		bBin = "sockerless-backend-" + cfg.Backend
	}
	bBinPath, err := exec.LookPath(bBin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: backend binary %q not found in PATH\n", bBin)
		os.Exit(1)
	}

	// Resolve frontend binary
	fBin := *frontendBin
	if fBin == "" {
		fBin = "sockerless-frontend-docker"
	}
	fBinPath, err := exec.LookPath(fBin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: frontend binary %q not found in PATH\n", fBin)
		os.Exit(1)
	}

	// Start backend
	backendCmd := exec.Command(bBinPath, "-addr", *backendAddr)
	backendCmd.Stdout = os.Stdout
	backendCmd.Stderr = os.Stderr
	if err := backendCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting backend: %v\n", err)
		os.Exit(1)
	}
	os.WriteFile(filepath.Join(runDir, "backend.pid"), []byte(strconv.Itoa(backendCmd.Process.Pid)), 0o644)
	fmt.Printf("Backend started (PID %d) on %s\n", backendCmd.Process.Pid, *backendAddr)

	// Start frontend
	frontendCmd := exec.Command(fBinPath, "-addr", *frontendAddr, "-backend", "http://localhost"+*backendAddr, "-mgmt-addr", *mgmtAddr)
	frontendCmd.Stdout = os.Stdout
	frontendCmd.Stderr = os.Stderr
	if err := frontendCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting frontend: %v\n", err)
		os.Exit(1)
	}
	os.WriteFile(filepath.Join(runDir, "frontend.pid"), []byte(strconv.Itoa(frontendCmd.Process.Pid)), 0o644)
	fmt.Printf("Frontend started (PID %d) on %s (mgmt: %s)\n", frontendCmd.Process.Pid, *frontendAddr, *mgmtAddr)

	// Brief health check wait
	time.Sleep(1 * time.Second)
	fmt.Println("Servers started. Use 'sockerless status' to verify.")
}

func serverStop() {
	name := activeContextName()
	if name == "" {
		fmt.Fprintln(os.Stderr, "error: no active context")
		os.Exit(1)
	}

	runDir := serverRunDir(name)
	stopped := 0

	for _, proc := range []string{"frontend", "backend"} {
		pidFile := filepath.Join(runDir, proc+".pid")
		data, err := os.ReadFile(pidFile)
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			os.Remove(pidFile)
			continue
		}
		p, err := os.FindProcess(pid)
		if err != nil {
			os.Remove(pidFile)
			continue
		}
		if err := p.Signal(syscall.SIGTERM); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not signal %s (PID %d): %v\n", proc, pid, err)
		} else {
			fmt.Printf("Sent SIGTERM to %s (PID %d)\n", proc, pid)
			stopped++
		}
		os.Remove(pidFile)
	}

	if stopped == 0 {
		fmt.Println("No running servers found")
	}
}

func serverRunDir(contextName string) string {
	return filepath.Join(sockerlessDir(), "run", contextName)
}
