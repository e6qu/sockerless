package aws_sdk_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func lambdaNodeDeploymentZip(t *testing.T, responseExpression string) []byte {
	t.Helper()
	if responseExpression == "" {
		responseExpression = "event"
	}
	return lambdaNodeSourceZip(t, fmt.Sprintf(
		"exports.handler = async (event) => { console.log(JSON.stringify(event)); return %s; };",
		responseExpression,
	))
}

func lambdaNodeSourceZip(t *testing.T, source string) []byte {
	t.Helper()
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	entry, err := zw.Create("index.js")
	if err != nil {
		t.Fatalf("create AWS Lambda deployment entry: %v", err)
	}
	if _, err := entry.Write([]byte(source)); err != nil {
		t.Fatalf("write AWS Lambda deployment entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close AWS Lambda deployment archive: %v", err)
	}
	return out.Bytes()
}

func lambdaPythonSourceZip(t *testing.T, source string) []byte {
	t.Helper()
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	entry, err := zw.Create("index.py")
	if err != nil {
		t.Fatalf("create AWS Lambda Python deployment entry: %v", err)
	}
	if _, err := entry.Write([]byte(source)); err != nil {
		t.Fatalf("write AWS Lambda Python deployment entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close AWS Lambda Python deployment archive: %v", err)
	}
	return out.Bytes()
}

func lambdaDeploymentZip(t *testing.T) []byte {
	t.Helper()
	return lambdaNodeDeploymentZip(t, "event")
}

// emptySHA256 is the hex SHA-256 of an empty body — the payload hash for a
// signed GET request with no body (Lambda's REST list/get operations).
const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// signRawSigV4 signs a hand-built HTTP request with SigV4 using the seed
// credential every SDK/CLI/Terraform client already signs with (test/test,
// us-east-1). Raw requests that hit a SigV4-gated chokepoint (the awsJson /
// awsQuery control plane at POST /, the S3 REST data plane, and Lambda's REST
// control plane) must present a valid signature exactly as the real cloud
// front end requires; this is the same signature the aws-sdk-go-v2 client
// computes for those requests. Set every signed header on req before calling.
// payloadHash is the hex SHA-256 of the body (see signRawSigV4JSON) or a
// streaming sentinel such as "STREAMING-AWS4-HMAC-SHA256-PAYLOAD".
func signRawSigV4(t *testing.T, req *http.Request, service, payloadHash string) {
	t.Helper()
	signRawSigV4Creds(t, req, service, payloadHash, "test", "test")
}

// signRawSigV4Creds is signRawSigV4 with an explicit access key id and secret,
// for tests that need to sign with a real-but-wrong secret (proving the
// simulator rejects a well-formed signature that doesn't verify) rather than
// the seed admin credential.
func signRawSigV4Creds(t *testing.T, req *http.Request, service, payloadHash, akid, secret string) {
	t.Helper()
	creds := aws.Credentials{AccessKeyID: akid, SecretAccessKey: secret}
	if err := v4.NewSigner().SignHTTP(ctx, creds, req, payloadHash, service, "us-east-1", time.Now()); err != nil {
		t.Fatalf("SigV4 sign (%s): %v", service, err)
	}
}

// signRawSigV4JSON signs a control-plane request whose payload hash is the
// SHA-256 of body — the shape awsJson/awsQuery clients sign.
func signRawSigV4JSON(t *testing.T, req *http.Request, service string, body []byte) {
	t.Helper()
	sum := sha256.Sum256(body)
	signRawSigV4(t, req, service, hex.EncodeToString(sum[:]))
}

var (
	baseURL                string
	simPort                int
	dnsPort                int
	simCmd                 *exec.Cmd
	binaryPath             string
	evalImageName          string // Docker image containing eval-arithmetic binary
	lambdaHandlerImageName string // Docker image for Lambda Runtime API test handler
	containerCommandImage  string // Docker image containing container-command binary
	ctx                    = context.Background()
)

func sdkConfig() aws.Config {
	return aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
		HTTPClient:  simHTTPClient,
	}
}

// simEndpoint returns the endpoint coordinate for one AWS service against the
// simulator: the service's own hostname in the `.localhost` family on the
// simulator's port. It has the same shape as the endpoint a real client
// resolves (`servicediscovery.us-east-1.amazonaws.com`), so an operation that
// carries a modeled endpoint host prefix — Cloud Map's `data-` on
// DiscoverInstances/DiscoverInstancesRevision, Step Functions' `sync-` on
// StartSyncExecution/TestState — builds and signs the real prefixed host
// (`data-servicediscovery.localhost`) instead of a prefix glued onto an IP
// literal. Only the endpoint coordinate differs from a real-cloud client; the
// request the SDK builds and signs is byte-for-byte the one AWS receives.
func simEndpoint(service string) string {
	return fmt.Sprintf("http://%s.localhost:%d", service, simPort)
}

// simHTTPClient is the SDK transport every client in this suite uses. It keeps
// the SDK's own transport defaults and replaces only name resolution: a host in
// the `.localhost` family resolves to the loopback address, which is what RFC
// 6761 mandates and what glibc does on Linux but macOS's resolver does not.
// Resolution is a coordinate, not a request property — the Host header, the
// URL and the SigV4 signature the SDK produces are untouched, so a request to
// `data-servicediscovery.localhost` carries and signs exactly that host.
var simHTTPClient = awshttp.NewBuildableClient().WithTransportOptions(func(tr *http.Transport) {
	tr.DialContext = simDialContext
	tr.Proxy = simProxy
})

var simDialer = &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}

func simDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if host, port, err := net.SplitHostPort(addr); err == nil && isLocalhostName(host) {
		addr = net.JoinHostPort("127.0.0.1", port)
	}
	return simDialer.DialContext(ctx, network, addr)
}

// simProxy keeps the SDK's environment-driven proxy behaviour for every host
// except the `.localhost` family, which resolves to loopback and must never be
// routed through a proxy Go would otherwise apply to a non-`localhost` name.
func simProxy(req *http.Request) (*url.URL, error) {
	if isLocalhostName(req.URL.Hostname()) {
		return nil, nil
	}
	return http.ProxyFromEnvironment(req)
}

func isLocalhostName(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	return h == "localhost" || strings.HasSuffix(h, ".localhost")
}

// capturedRequest holds the host and Authorization header of the request an
// SDK client actually put on the wire.
type capturedRequest struct {
	host          string
	authorization string
}

// signedHeaders returns the SignedHeaders list out of a SigV4 Authorization
// header, so a test can assert which headers the signature covers.
func (c capturedRequest) signedHeaders() []string {
	for _, part := range strings.Split(c.authorization, ",") {
		part = strings.TrimSpace(part)
		if rest, ok := strings.CutPrefix(part, "SignedHeaders="); ok {
			return strings.Split(rest, ";")
		}
	}
	return nil
}

// captureSignedRequest is an SDK API option that records the final request
// after every Finalize middleware has run — the modeled endpoint host-prefix
// mutation and the SigV4 signer included. A test uses it to assert the host the
// SDK addressed and signed, which is the only way to prove an operation with a
// modeled host prefix really exercised the prefixed host.
func captureSignedRequest(rec *capturedRequest) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		return stack.Finalize.Add(middleware.FinalizeMiddlewareFunc("captureSignedRequest",
			func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
				if req, ok := in.Request.(*smithyhttp.Request); ok {
					rec.host = req.URL.Host
					rec.authorization = req.Header.Get("Authorization")
				}
				return next.HandleFinalize(ctx, in)
			}), middleware.After)
	}
}

func TestMain(m *testing.M) {
	if configuredBinary := os.Getenv("SOCKERLESS_AWS_SIMULATOR_BINARY"); configuredBinary != "" {
		var err error
		binaryPath, err = filepath.Abs(configuredBinary)
		if err != nil {
			log.Fatalf("Failed to resolve SOCKERLESS_AWS_SIMULATOR_BINARY: %v", err)
		}
		if info, err := os.Stat(binaryPath); err != nil {
			log.Fatalf("SOCKERLESS_AWS_SIMULATOR_BINARY is not readable: %v", err)
		} else if info.IsDir() || info.Mode()&0111 == 0 {
			log.Fatalf("SOCKERLESS_AWS_SIMULATOR_BINARY is not an executable file: %s", binaryPath)
		}
	} else {
		binaryPath, _ = filepath.Abs("../simulator-aws")
		simDir, _ := filepath.Abs("..")
		build := exec.Command("go", "build", "-tags", "noui", "-o", binaryPath, ".")
		build.Dir = simDir
		build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off")
		if out, err := build.CombinedOutput(); err != nil {
			log.Fatalf("Failed to build simulator: %v\n%s", err, out)
		}
	}

	workloadPlatform := nativeDockerPlatform()

	evalDir, _ := filepath.Abs("../../testdata/eval-arithmetic")
	evalImageName = "sockerless-eval-arithmetic:test"
	buildGoScratchImage(evalImageName, evalDir, "eval-arithmetic", workloadPlatform)

	lambdaHandlerDir, _ := filepath.Abs("../../testdata/lambda-runtime-handler")
	lambdaHandlerImageName = "sockerless-lambda-runtime-handler:test"
	buildGoScratchImage(lambdaHandlerImageName, lambdaHandlerDir, "lambda-runtime-handler", workloadPlatform)

	containerCommandDir, _ := filepath.Abs("../../testdata/container-command")
	containerCommandImage = "sockerless-container-command:test"
	buildGoScratchImage(containerCommandImage, containerCommandDir, "container-command", workloadPlatform)

	// Pre-pull busybox up front (with retry) — it backs the awsvpc netns
	// pause container AND is the workload image for many ECS tests. Pulling
	// it lazily at RunTask time made the ECR-gallery fetch a flaky dependency
	// of the task lifecycle: a transient throttle there surfaced as the task
	// container "failing to start" (ExitCode -1) rather than a clear pull
	// error. Fetching it once here removes that race from every test.
	pullImageWithRetry("public.ecr.aws/docker/library/busybox:latest")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	simPort = port
	ln.Close()

	// Free UDP port for the Route 53 DNS server. The simulator and the
	// test share SIM_DNS_PORT so both know where to send queries.
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free DNS port: %v", err)
	}
	dnsPort = udpConn.(*net.UDPConn).LocalAddr().(*net.UDPAddr).Port
	udpConn.Close()

	simCmd = exec.Command(binaryPath)
	simCmd.Env = append(os.Environ(),
		fmt.Sprintf("SIM_LISTEN_ADDR=:%d", port),
		fmt.Sprintf("SIM_DNS_PORT=%d", dnsPort),
		"SIM_LOG_LEVEL=warn",
	)
	simCmd.Stdout = os.Stdout
	simCmd.Stderr = os.Stderr
	if err := simCmd.Start(); err != nil {
		log.Fatalf("Failed to start simulator: %v", err)
	}

	baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	if err := waitForHealth(baseURL + "/health"); err != nil {
		shutdownSimulator(simCmd)
		log.Fatalf("Simulator did not become healthy: %v", err)
	}

	code := m.Run()
	shutdownSimulator(simCmd)
	os.Exit(code)
}

func shutdownSimulator(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	_ = cmd.Process.Signal(os.Interrupt)
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

func nativeDockerPlatform() string {
	return "linux/" + runtime.GOARCH
}

// pullImageWithRetry pulls a public image up front with bounded exponential
// backoff so a transient registry throttle doesn't flake a test that runs the
// image. Mirrors the azure sdk-tests pattern. Fails the suite only after
// exhausting retries — a genuinely unreachable image must fail loud.
func pullImageWithRetry(image string) {
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		cmd := exec.Command("docker", "pull", image)
		if out, err := cmd.CombinedOutput(); err == nil {
			return
		} else {
			lastErr = fmt.Errorf("%w\n%s", err, out)
		}
		time.Sleep(time.Duration(attempt*attempt) * time.Second)
	}
	log.Fatalf("Failed to pull %s after retries: %v", image, lastErr)
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
	// On a docker-container buildx driver (the default on many dev machines),
	// `docker build -t` leaves the image in the build cache only — never the
	// daemon store — so the sim's container start can't find it. `docker buildx
	// build --load` materializes it into the daemon store. The legacy builder
	// builds into the store natively and rejects the buildx-only `--load`, so
	// omit it there.
	var args []string
	if exec.Command("docker", "buildx", "version").Run() == nil {
		args = []string{"buildx", "build", "--load"}
	} else {
		args = []string{"build"}
	}
	args = append(args, "--platform", platform, "-t", imageName, "-f", "-", buildDir)
	dockerBuild := exec.Command("docker", args...)
	dockerBuild.Stdin = strings.NewReader(dockerfile)
	if out, err := dockerBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build %s Docker image: %v\n%s", binaryName, err, out)
	}
}

func waitForHealth(url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == 200 {
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
