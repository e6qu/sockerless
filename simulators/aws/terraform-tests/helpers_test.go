package aws_tf_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	baseURL     string
	simCmd      *exec.Cmd
	gatewayCmd  *exec.Cmd
	binaryPath  string
	simPort     int
	gatewayPort int
	caCertFile  string
	tfState     string
	tfEndpoint  string
)

func TestMain(m *testing.M) {
	noProxy := mergeNoProxy(os.Getenv("NO_PROXY"),
		"localhost",
		"127.0.0.1",
		"::1",
		"sockerless.localhost",
		".sockerless.localhost",
		"*.sockerless.localhost",
	)
	os.Setenv("NO_PROXY", noProxy)
	os.Setenv("no_proxy", noProxy)

	stateDir, err := os.MkdirTemp("", "sockerless-aws-tf-state-*")
	if err != nil {
		log.Fatalf("Failed to create terraform state dir: %v", err)
	}
	defer os.RemoveAll(stateDir)
	tfState = filepath.Join(stateDir, "terraform.tfstate")
	binaryPath = filepath.Join(stateDir, "simulator-aws")

	if configured := os.Getenv("SOCKERLESS_AWS_SIMULATOR_BINARY"); configured != "" {
		binaryPath = requireExecutable(configured, "AWS Terraform tests")
	} else {
		simDir, _ := filepath.Abs("..")
		build := exec.Command("go", "build", "-tags", "noui", "-o", binaryPath, ".")
		build.Dir = simDir
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := build.CombinedOutput(); err != nil {
			log.Fatalf("Failed to build simulator: %v\n%s", err, out)
		}
	}

	lambdaHandlerDir, _ := filepath.Abs("../../testdata/lambda-runtime-handler")
	buildGoScratchImage("sockerless-lambda-runtime-handler:test", lambdaHandlerDir, "lambda-runtime-handler", nativeDockerPlatform())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free port: %v", err)
	}
	simPort = ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	simCmd = exec.Command(binaryPath)
	simCmd.Env = append(os.Environ(), fmt.Sprintf("SIM_LISTEN_ADDR=:%d", simPort))
	simCmd.Stdout = os.Stdout
	simCmd.Stderr = os.Stderr
	if err := simCmd.Start(); err != nil {
		log.Fatalf("Failed to start simulator: %v", err)
	}

	baseURL = fmt.Sprintf("http://127.0.0.1:%d", simPort)
	tfEndpoint = baseURL

	if err := waitForHealth(baseURL + "/health"); err != nil {
		simCmd.Process.Kill()
		log.Fatalf("Simulator did not become healthy: %v", err)
	}

	if os.Getenv("SOCKERLESS_TF_HTTPS_GATEWAY") == "1" {
		gatewayDir, err := os.MkdirTemp("", "aws-https-gateway-*")
		if err != nil {
			simCmd.Process.Kill()
			log.Fatalf("Failed to create HTTPS gateway state dir: %v", err)
		}
		defer os.RemoveAll(gatewayDir)

		repoRoot, err := filepath.Abs("../../..")
		if err != nil {
			simCmd.Process.Kill()
			log.Fatalf("Failed to resolve repository root: %v", err)
		}
		caddyBin := os.Getenv("CADDY")
		if caddyBin == "" {
			caddyBin = "caddy"
		}
		caddyBin = requireExecutable(caddyBin, "AWS Terraform HTTPS gateway tests")

		gatewayPort = freeTCPPort()
		gatewayAdminPort := freeTCPPort()
		caCertFile = filepath.Join(gatewayDir, "data", "caddy", "pki", "authorities", "local", "root.crt")
		gatewayCmd = exec.Command(caddyBin, "run", "--config", filepath.Join(repoRoot, "make", "https-gateway", "Caddyfile"), "--adapter", "caddyfile")
		gatewayCmd.Env = append(os.Environ(),
			fmt.Sprintf("XDG_DATA_HOME=%s", filepath.Join(gatewayDir, "data")),
			fmt.Sprintf("XDG_CONFIG_HOME=%s", filepath.Join(gatewayDir, "config")),
			fmt.Sprintf("SOCKERLESS_HTTPS_GATEWAY_PORT=%d", gatewayPort),
			fmt.Sprintf("SOCKERLESS_HTTPS_GATEWAY_ADMIN_PORT=%d", gatewayAdminPort),
			fmt.Sprintf("SOCKERLESS_AWS_SIM_PORT=%d", simPort),
			"SOCKERLESS_GCP_SIM_PORT=1",
			"SOCKERLESS_AZURE_SIM_PORT=1",
			fmt.Sprintf("SOCKERLESS_HTTPS_GATEWAY_DEFAULT_SIM_PORT=%d", simPort),
		)
		gatewayCmd.Stdout = os.Stdout
		gatewayCmd.Stderr = os.Stderr
		if err := gatewayCmd.Start(); err != nil {
			simCmd.Process.Kill()
			log.Fatalf("Failed to start HTTPS gateway: %v", err)
		}

		tfEndpoint = fmt.Sprintf("https://localhost:%d", gatewayPort)
		requireHTTPSURL(tfEndpoint, "AWS Terraform HTTPS gateway endpoint")
		if err := waitForFile(caCertFile, 10*time.Second); err != nil {
			gatewayCmd.Process.Kill()
			simCmd.Process.Kill()
			log.Fatalf("HTTPS gateway did not publish its local CA: %v", err)
		}
		if err := waitForHTTPSHealth(tfEndpoint+"/health", caCertFile); err != nil {
			gatewayCmd.Process.Kill()
			simCmd.Process.Kill()
			log.Fatalf("HTTPS gateway did not become healthy: %v", err)
		}
	}

	code := m.Run()
	if gatewayCmd != nil {
		gatewayCmd.Process.Kill()
		gatewayCmd.Wait()
	}
	simCmd.Process.Kill()
	simCmd.Wait()
	os.Exit(code)
}

func waitForHealth(url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 50; i++ {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == 200 {
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

func nativeDockerPlatform() string {
	return "linux/" + runtime.GOARCH
}

func buildGoScratchImage(imageName, sourceDir, binaryName, platform string) {
	buildDir, err := os.MkdirTemp("", "sockerless-aws-image-*")
	if err != nil {
		log.Fatalf("Failed to create image build dir: %v", err)
	}
	defer os.RemoveAll(buildDir)

	binaryPath := filepath.Join(buildDir, binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Dir = sourceDir
	build.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH="+runtime.GOARCH,
	)
	if out, err := build.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build %s binary: %v\n%s", binaryName, err, out)
	}

	dockerfile := fmt.Sprintf(`FROM scratch
COPY %s /usr/local/bin/%s
ENTRYPOINT ["/usr/local/bin/%s"]
`, binaryName, binaryName, binaryName)
	dockerBuild := exec.Command("docker", "build",
		"--platform", platform,
		"-t", imageName, "-f", "-", buildDir)
	dockerBuild.Stdin = strings.NewReader(dockerfile)
	if out, err := dockerBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build %s Docker image: %v\n%s", binaryName, err, out)
	}
}

func terraformCmd(args ...string) *exec.Cmd {
	args = terraformArgs(args...)
	cmd := exec.Command("terraform", args...)
	cmd.Dir = filepath.Dir(mustAbs("main.tf"))
	// Put terraform and the provider-plugin subprocesses it spawns into their
	// own process group so runTimed can reap the whole tree with one
	// kill(-pgid). Without this, a timed-out command leaves orphaned, spinning
	// provider plugins (reparented to init) that starve later runs into
	// cascading timeouts.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("TF_VAR_endpoint=%s", tfEndpoint),
	)
	if caCertFile != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("SSL_CERT_FILE=%s", caCertFile))
	}
	// Pass through TF_LOG / TF_LOG_PATH when the operator wants
	// terraform-provider-aws's request/response trace (debugging sim
	// handler gaps surfaced by the provider's call sequence).
	if v := os.Getenv("TF_LOG"); v != "" {
		cmd.Env = append(cmd.Env, "TF_LOG="+v)
	}
	if v := os.Getenv("TF_LOG_PATH"); v != "" {
		cmd.Env = append(cmd.Env, "TF_LOG_PATH="+v)
	}
	return cmd
}

func terraformArgs(args ...string) []string {
	if len(args) == 0 || tfState == "" {
		return args
	}
	switch args[0] {
	case "apply", "destroy", "output", "plan", "refresh":
		out := make([]string, 0, len(args)+1)
		out = append(out, args[0], "-state="+tfState)
		out = append(out, args[1:]...)
		return out
	default:
		return args
	}
}

func mustAbs(name string) string {
	p, err := filepath.Abs(name)
	if err != nil {
		log.Fatal(err)
	}
	return p
}

func mergeNoProxy(existing string, entries ...string) string {
	seen := map[string]bool{}
	var merged []string
	for _, part := range strings.Split(existing, ",") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		merged = append(merged, part)
	}
	for _, entry := range entries {
		if entry == "" || seen[entry] {
			continue
		}
		seen[entry] = true
		merged = append(merged, entry)
	}
	return strings.Join(merged, ",")
}

func freeTCPPort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func requireExecutable(name, purpose string) string {
	if strings.ContainsRune(name, rune(os.PathSeparator)) {
		info, err := os.Stat(name)
		if err != nil {
			log.Fatalf("%s requires executable %q: %v", purpose, name, err)
		}
		if info.IsDir() {
			log.Fatalf("%s requires executable %q, but it is a directory", purpose, name)
		}
		if info.Mode()&0111 == 0 {
			log.Fatalf("%s requires executable %q, but it is not executable", purpose, name)
		}
		return name
	}
	path, err := exec.LookPath(name)
	if err != nil {
		log.Fatalf("%s requires %q in PATH or CADDY=/path/to/caddy; install Caddy before running these tests: %v", purpose, name, err)
	}
	return path
}

func requireHTTPSURL(raw, purpose string) {
	u, err := url.Parse(raw)
	if err != nil {
		log.Fatalf("%s must be a valid HTTPS URL, got %q: %v", purpose, raw, err)
	}
	if u.Scheme != "https" {
		log.Fatalf("%s must use HTTPS when SOCKERLESS_TF_HTTPS_GATEWAY=1, got %q", purpose, raw)
	}
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

func waitForHTTPSHealth(raw, caCert string) error {
	client, err := trustedHTTPClient(caCert)
	if err != nil {
		return err
	}
	for i := 0; i < 50; i++ {
		resp, err := client.Get(raw)
		if err == nil && resp.StatusCode == 200 {
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
