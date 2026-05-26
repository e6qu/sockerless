package azure_sdk_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

var (
	baseURL        string
	simCmd         *exec.Cmd
	binaryPath     string
	evalImageName  string // Docker image containing eval-arithmetic binary
	sbAMQPEndpoint string
	ctx            = context.Background()
	subscriptionID = "00000000-0000-0000-0000-000000000001"
)

type fakeCredential struct{}

func (f *fakeCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "fake-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func clientOpts() *arm.ClientOptions {
	return &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: cloud.Configuration{
				Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
					cloud.ResourceManager: {
						Endpoint: baseURL,
						Audience: "https://management.azure.com/",
					},
				},
			},
			InsecureAllowCredentialWithHTTP: true,
		},
	}
}

func TestMain(m *testing.M) {
	binaryPath, _ = filepath.Abs("../simulator-azure")

	simDir, _ := filepath.Abs("..")
	build := exec.Command("go", "build", "-tags", "noui", "-o", binaryPath, ".")
	build.Dir = simDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build simulator: %v\n%s", err, out)
	}

	// Build the Docker image hosting eval-arithmetic. Multi-stage Docker
	// build forced to linux/arm64 — sim's primary capacity contract.
	evalDir, _ := filepath.Abs("../../testdata/eval-arithmetic")
	evalImageName = "sockerless-eval-arithmetic:test"
	dockerfile := `FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /eval-arithmetic .
FROM alpine:latest
COPY --from=build /eval-arithmetic /usr/local/bin/eval-arithmetic
ENTRYPOINT ["/usr/local/bin/eval-arithmetic"]
`
	dockerBuild := exec.Command("docker", "build",
		"--platform", "linux/arm64",
		"-t", evalImageName, "-f", "-", evalDir)
	dockerBuild.Stdin = strings.NewReader(dockerfile)
	if out, err := dockerBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build eval-arithmetic Docker image: %v\n%s", err, out)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	amqpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free Service Bus AMQP port: %v", err)
	}
	amqpPort := amqpLn.Addr().(*net.TCPAddr).Port
	amqpLn.Close()
	sbAMQPEndpoint = fmt.Sprintf("127.0.0.1:%d", amqpPort)

	certDir, err := os.MkdirTemp("", "sockerless-servicebus-amqp-tls-*")
	if err != nil {
		log.Fatalf("Failed to create Service Bus AMQP cert dir: %v", err)
	}
	defer os.RemoveAll(certDir)
	certPath, keyPath := writeServiceBusAMQPCert(certDir)

	simCmd = exec.Command(binaryPath)
	simCmd.Env = append(os.Environ(),
		fmt.Sprintf("SIM_LISTEN_ADDR=:%d", port),
		fmt.Sprintf("SIM_SERVICEBUS_AMQP_LISTEN_ADDR=:%d", amqpPort),
		fmt.Sprintf("SIM_SERVICEBUS_AMQP_TLS_CERT=%s", certPath),
		fmt.Sprintf("SIM_SERVICEBUS_AMQP_TLS_KEY=%s", keyPath),
	)
	simCmd.Stdout = os.Stdout
	simCmd.Stderr = os.Stderr
	if err := simCmd.Start(); err != nil {
		log.Fatalf("Failed to start simulator: %v", err)
	}

	baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	if err := waitForHealth(baseURL + "/health"); err != nil {
		simCmd.Process.Kill()
		log.Fatalf("Simulator did not become healthy: %v", err)
	}

	code := m.Run()
	simCmd.Process.Kill()
	simCmd.Wait()
	os.Exit(code)
}

func writeServiceBusAMQPCert(dir string) (string, string) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("generate Service Bus AMQP TLS key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		log.Fatalf("generate Service Bus AMQP TLS serial: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "servicebus.localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"servicebus.localhost", "*.servicebus.localhost", "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		log.Fatalf("create Service Bus AMQP TLS cert: %v", err)
	}
	certPath := filepath.Join(dir, "servicebus-amqp-cert.pem")
	keyPath := filepath.Join(dir, "servicebus-amqp-key.pem")
	certFile, err := os.Create(certPath)
	if err != nil {
		log.Fatalf("create Service Bus AMQP TLS cert file: %v", err)
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		certFile.Close()
		log.Fatalf("write Service Bus AMQP TLS cert: %v", err)
	}
	if err := certFile.Close(); err != nil {
		log.Fatalf("close Service Bus AMQP TLS cert: %v", err)
	}
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("create Service Bus AMQP TLS key file: %v", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		keyFile.Close()
		log.Fatalf("write Service Bus AMQP TLS key: %v", err)
	}
	if err := keyFile.Close(); err != nil {
		log.Fatalf("close Service Bus AMQP TLS key: %v", err)
	}
	return certPath, keyPath
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
