package tfsim

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	BaseURL string
	State   string
	cmd     *exec.Cmd
	dir     string
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
	build := exec.Command("go", "build", "-tags", "noui", "-o", binaryPath, ".")
	build.Dir = simDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build simulator: %v\n%s", err, out)
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
		cmd:     cmd,
		dir:     configDir,
	}
	if err := waitForHealth(env.BaseURL + "/health"); err != nil {
		t.Fatalf("simulator health: %v", err)
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
	cmd.Env = append(os.Environ(), "TF_VAR_endpoint="+e.BaseURL)
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
	req, err := http.NewRequest(http.MethodPost, e.BaseURL+"/", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build %s request: %v", target, err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	resp, err := http.DefaultClient.Do(req)
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
	resp, err := http.PostForm(e.BaseURL+"/", values)
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

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
