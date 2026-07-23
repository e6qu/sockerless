package tfsim

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

type Env struct {
	BaseURL    string
	Endpoint   string
	State      string
	CACertFile string
	Client     *http.Client
	cmd        *exec.Cmd
	gatewayCmd *exec.Cmd
	dir        string
}

func Start(t *testing.T, configDir string) *Env {
	t.Helper()

	stateDir, err := os.MkdirTemp("", "sockerless-aws-tf-state-*")
	if err != nil {
		t.Fatalf("create terraform state dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })

	binaryPath := filepath.Join(stateDir, "simulator-aws")
	simDir, err := filepath.Abs(filepath.Join(configDir, "..", ".."))
	if err != nil {
		t.Fatalf("resolve simulator dir: %v", err)
	}
	if configured := os.Getenv("SOCKERLESS_AWS_SIMULATOR_BINARY"); configured != "" {
		binaryPath = requireExecutable(t, configured, "AWS Terraform tests")
	} else {
		build := exec.Command("go", "build", "-tags", "noui", "-o", binaryPath, ".")
		build.Dir = simDir
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build simulator: %v\n%s", err, out)
		}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("free-port listener address is not TCP: %T", ln.Addr())
	}
	port := tcpAddr.Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close free-port listener: %v", err)
	}

	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), fmt.Sprintf("SIM_LISTEN_ADDR=:%d", port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Own process group so the whole simulator subtree can be reaped with one
	// kill(-pgid): by t.Cleanup on the normal path, by the deadline watchdog
	// just before a hard timeout, and by the signal reaper on Ctrl-C / SIGQUIT.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start simulator: %v", err)
	}
	trackForReaping(cmd)
	t.Cleanup(func() {
		reapProcessGroup(cmd)
		untrackForReaping(cmd)
	})

	env := &Env{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		State:   filepath.Join(stateDir, "terraform.tfstate"),
		Client:  http.DefaultClient,
		cmd:     cmd,
		dir:     configDir,
	}
	env.Endpoint = env.BaseURL
	if err := waitForHealth(env.BaseURL + "/health"); err != nil {
		t.Fatalf("simulator health: %v", err)
	}
	if os.Getenv("SOCKERLESS_TF_HTTPS_GATEWAY") == "1" {
		startHTTPSGateway(t, env, stateDir, simDir, port)
	}
	env.armDeadlineReaper(t)
	return env
}

// armDeadlineReaper reaps the simulator (and HTTPS gateway) process groups a
// few seconds before the test's own deadline. go test's hard timeout aborts the
// binary without running t.Cleanup, so without this the subprocesses would
// orphan past the run; firing early reaps them while there is still time.
func (e *Env) armDeadlineReaper(t *testing.T) {
	dl, ok := t.Deadline()
	if !ok {
		return
	}
	d := time.Until(dl) - 15*time.Second
	if d <= 0 {
		return
	}
	timer := time.AfterFunc(d, func() {
		reapProcessGroup(e.gatewayCmd)
		reapProcessGroup(e.cmd)
	})
	t.Cleanup(func() { timer.Stop() })
}

func (e *Env) Terraform(t *testing.T, args ...string) []byte {
	t.Helper()
	if len(args) > 0 {
		switch args[0] {
		case "apply", "destroy", "output", "plan", "refresh":
			out := make([]string, 0, len(args)+1)
			out = append(out, args[0], "-state="+e.State)
			out = append(out, args[1:]...)
			args = out
		}
	}
	cmd := exec.Command("terraform", args...)
	cmd.Dir = e.dir
	// Own process group so a stuck terraform (and the provider-plugin
	// grandchildren it spawns) can be reaped with one kill(-pgid) instead of
	// orphaning a spinning process tree past the test deadline.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(), "TF_VAR_endpoint="+e.Endpoint)
	if e.CACertFile != "" {
		cmd.Env = append(cmd.Env, "SSL_CERT_FILE="+e.CACertFile)
	}
	if v := os.Getenv("TF_LOG"); v != "" {
		cmd.Env = append(cmd.Env, "TF_LOG="+v)
	}
	if v := os.Getenv("TF_LOG_PATH"); v != "" {
		cmd.Env = append(cmd.Env, "TF_LOG_PATH="+v)
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatalf("terraform %v: failed to start: %v", args, err)
	}
	killGroup := func() {
		if cmd.Process != nil {
			// Negative pid targets the whole process group (Setpgid'd tree).
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	// Safety net for the pass/fail/panic paths; the watchdog below covers the
	// hard-timeout path that t.Cleanup does not run on.
	t.Cleanup(killGroup)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Fire ~20s before the test deadline so the tree is reaped and the command
	// fails diagnosably rather than being orphaned by the hard SIGQUIT timeout.
	var watchdog <-chan time.Time
	if dl, ok := t.Deadline(); ok {
		if d := time.Until(dl) - 20*time.Second; d > 0 {
			timer := time.NewTimer(d)
			defer timer.Stop()
			watchdog = timer.C
		}
	}

	select {
	case err := <-done:
		t.Logf("terraform %v duration=%s", args, time.Since(start).Round(time.Millisecond))
		if err != nil {
			t.Fatalf("terraform %v failed: %v\n%s", args, err, buf.Bytes())
		}
		return buf.Bytes()
	case <-watchdog:
		killGroup()
		<-done // reap the killed process so no zombie/orphan remains
		t.Fatalf("terraform %v timed out near the test deadline (process group killed to avoid orphans)\n%s",
			args, buf.Bytes())
		return nil
	}
}

// reapProcessGroup SIGKILLs the whole process group of cmd (started Setpgid, so
// it leads its own group) and reaps it, leaving no orphan or zombie. Safe to
// call more than once and on a nil / not-yet-started command.
func reapProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Wait()
}

var (
	reaperMu   sync.Mutex
	reaperCmds []*exec.Cmd
	reaperOnce sync.Once
)

// trackForReaping registers a Setpgid'd subprocess so the signal reaper can
// kill its process group if the test binary is aborted by Ctrl-C or the
// go-test hard-timeout SIGQUIT — neither of which runs t.Cleanup.
func trackForReaping(cmd *exec.Cmd) {
	reaperMu.Lock()
	reaperCmds = append(reaperCmds, cmd)
	reaperMu.Unlock()
	reaperOnce.Do(installSignalReaper)
}

func untrackForReaping(cmd *exec.Cmd) {
	reaperMu.Lock()
	for i, c := range reaperCmds {
		if c == cmd {
			reaperCmds = append(reaperCmds[:i], reaperCmds[i+1:]...)
			break
		}
	}
	reaperMu.Unlock()
}

func installSignalReaper() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		sig := <-ch
		reaperMu.Lock()
		for _, c := range reaperCmds {
			if c != nil && c.Process != nil {
				_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
			}
		}
		reaperMu.Unlock()
		// Restore default disposition and re-deliver so the exit status
		// reflects the terminating signal. The channel only carries the
		// syscall signals registered above.
		if s, ok := sig.(syscall.Signal); ok {
			signal.Reset(s)
			_ = syscall.Kill(syscall.Getpid(), s)
		}
	}()
}

func (e *Env) AWSJSON(t *testing.T, target string, in, out any) {
	t.Helper()
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal %s request: %v", target, err)
	}
	req, err := http.NewRequest(http.MethodPost, e.Endpoint+"/", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build %s request: %v", target, err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var b bytes.Buffer
		_, _ = b.ReadFrom(resp.Body)
		t.Fatalf("%s status=%d body=%s", target, resp.StatusCode, b.String())
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s response: %v", target, err)
		}
	}
}

func (e *Env) AWSQuery(t *testing.T, values url.Values) {
	t.Helper()
	resp, err := e.Client.PostForm(e.Endpoint+"/", values)
	if err != nil {
		t.Fatalf("post query action %s: %v", values.Get("Action"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var b bytes.Buffer
		_, _ = b.ReadFrom(resp.Body)
		t.Fatalf("query action %s status=%d body=%s", values.Get("Action"), resp.StatusCode, b.String())
	}
}

func startHTTPSGateway(t *testing.T, env *Env, stateDir, simDir string, simPort int) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join(simDir, "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	caddyBin := os.Getenv("CADDY")
	if caddyBin == "" {
		caddyBin = "caddy"
	}
	caddyBin = requireExecutable(t, caddyBin, "AWS Terraform HTTPS gateway tests")

	gatewayDir := filepath.Join(stateDir, "https-gateway")
	if err := os.MkdirAll(gatewayDir, 0o755); err != nil {
		t.Fatalf("create HTTPS gateway state dir: %v", err)
	}
	gatewayPort := freeTCPPort(t)
	gatewayAdminPort := freeTCPPort(t)
	env.CACertFile = filepath.Join(gatewayDir, "data", "caddy", "pki", "authorities", "local", "root.crt")
	env.gatewayCmd = exec.Command(caddyBin, "run", "--config", filepath.Join(repoRoot, "make", "https-gateway", "Caddyfile"), "--adapter", "caddyfile")
	env.gatewayCmd.Env = append(os.Environ(),
		"XDG_DATA_HOME="+filepath.Join(gatewayDir, "data"),
		"XDG_CONFIG_HOME="+filepath.Join(gatewayDir, "config"),
		fmt.Sprintf("SOCKERLESS_HTTPS_GATEWAY_PORT=%d", gatewayPort),
		fmt.Sprintf("SOCKERLESS_HTTPS_GATEWAY_ADMIN_PORT=%d", gatewayAdminPort),
		fmt.Sprintf("SOCKERLESS_AWS_SIM_PORT=%d", simPort),
		"SOCKERLESS_GCP_SIM_PORT=1",
		"SOCKERLESS_AZURE_SIM_PORT=1",
		fmt.Sprintf("SOCKERLESS_HTTPS_GATEWAY_DEFAULT_SIM_PORT=%d", simPort),
	)
	env.gatewayCmd.Stdout = os.Stdout
	env.gatewayCmd.Stderr = os.Stderr
	// Own process group, reaped the same way as the simulator (see Start).
	env.gatewayCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := env.gatewayCmd.Start(); err != nil {
		t.Fatalf("start HTTPS gateway: %v", err)
	}
	trackForReaping(env.gatewayCmd)
	t.Cleanup(func() {
		reapProcessGroup(env.gatewayCmd)
		untrackForReaping(env.gatewayCmd)
	})

	env.Endpoint = fmt.Sprintf("https://localhost:%d", gatewayPort)
	if err := waitForFile(env.CACertFile, 10*time.Second); err != nil {
		t.Fatalf("HTTPS gateway CA: %v", err)
	}
	client, err := trustedHTTPClient(env.CACertFile)
	if err != nil {
		t.Fatalf("HTTPS gateway trust: %v", err)
	}
	env.Client = client
	if err := waitForHTTPSHealth(env.Endpoint+"/health", client); err != nil {
		t.Fatalf("HTTPS gateway health: %v", err)
	}
}

func waitForHealth(url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			resp.Body.Close()
		} else if err != nil {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s: %v", url, lastErr)
}

func waitForHTTPSHealth(raw string, client *http.Client) error {
	for i := 0; i < 50; i++ {
		resp, err := client.Get(raw)
		if err == nil && resp.StatusCode == http.StatusOK {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", raw)
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", path)
}

func trustedHTTPClient(caCert string) (*http.Client, error) {
	caPEM, err := os.ReadFile(caCert)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("parse CA cert %s", caCert)
	}
	return &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}, nil
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("free-port listener address is not TCP: %T", ln.Addr())
	}
	port := tcpAddr.Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close free-port listener: %v", err)
	}
	return port
}

func requireExecutable(t *testing.T, name, purpose string) string {
	t.Helper()
	if filepath.Base(name) != name {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatalf("%s requires executable %q: %v", purpose, name, err)
		}
		if info.IsDir() {
			t.Fatalf("%s requires executable %q, but it is a directory", purpose, name)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("%s requires executable %q, but it is not executable", purpose, name)
		}
		return name
	}
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("%s requires %q in PATH: %v", purpose, name, err)
	}
	return path
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
