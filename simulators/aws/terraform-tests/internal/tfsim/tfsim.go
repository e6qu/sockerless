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
	"path/filepath"
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
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close free-port listener: %v", err)
	}

	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), fmt.Sprintf("SIM_LISTEN_ADDR=:%d", port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start simulator: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
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
	return env
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
	start := time.Now()
	out, err := cmd.CombinedOutput()
	t.Logf("terraform %v duration=%s", args, time.Since(start).Round(time.Millisecond))
	if err != nil {
		t.Fatalf("terraform %v failed: %v\n%s", args, err, out)
	}
	return out
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
	if err := env.gatewayCmd.Start(); err != nil {
		t.Fatalf("start HTTPS gateway: %v", err)
	}
	t.Cleanup(func() {
		_ = env.gatewayCmd.Process.Kill()
		_ = env.gatewayCmd.Wait()
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
	for i := 0; i < 50; i++ {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", url)
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
	port := ln.Addr().(*net.TCPAddr).Port
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
