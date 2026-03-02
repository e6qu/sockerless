package azure_tf_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
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
	"testing"
	"time"
)

var (
	baseURL    string
	simCmd     *exec.Cmd
	binaryPath string
	simPort    int
	caCertFile string
)

// generateTLSCerts creates a CA and server certificate in dir.
// Returns the paths to caCert, serverCert, and serverKey files.
func generateTLSCerts(dir string) (string, string, string) {
	// Generate CA key
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate CA key: %v", err)
	}

	// CA certificate template
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	// Self-sign CA cert
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		log.Fatalf("Failed to create CA certificate: %v", err)
	}

	// Generate server key
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate server key: %v", err)
	}

	// Server certificate template
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:     []string{"localhost"},
	}

	// Sign server cert with CA
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		log.Fatalf("Failed to parse CA certificate: %v", err)
	}
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		log.Fatalf("Failed to create server certificate: %v", err)
	}

	// Write CA cert
	caCertPath := filepath.Join(dir, "ca.pem")
	writePEM(caCertPath, "CERTIFICATE", caCertDER)

	// Write server cert
	serverCertPath := filepath.Join(dir, "server-cert.pem")
	writePEM(serverCertPath, "CERTIFICATE", serverCertDER)

	// Write server key
	serverKeyPath := filepath.Join(dir, "server-key.pem")
	keyBytes, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		log.Fatalf("Failed to marshal server key: %v", err)
	}
	writePEM(serverKeyPath, "EC PRIVATE KEY", keyBytes)

	return caCertPath, serverCertPath, serverKeyPath
}

func writePEM(path, blockType string, data []byte) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("Failed to create %s: %v", path, err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: data}); err != nil {
		log.Fatalf("Failed to write %s: %v", path, err)
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

	// Generate TLS certificates
	certDir, err := os.MkdirTemp("", "azure-tls-*")
	if err != nil {
		log.Fatalf("Failed to create cert dir: %v", err)
	}
	caCertPath, serverCertPath, serverKeyPath := generateTLSCerts(certDir)
	caCertFile = caCertPath

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free port: %v", err)
	}
	simPort = ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	simCmd = exec.Command(binaryPath)
	simCmd.Env = append(os.Environ(),
		fmt.Sprintf("SIM_LISTEN_ADDR=:%d", simPort),
		fmt.Sprintf("SIM_TLS_CERT=%s", serverCertPath),
		fmt.Sprintf("SIM_TLS_KEY=%s", serverKeyPath),
	)
	simCmd.Stdout = os.Stdout
	simCmd.Stderr = os.Stderr
	if err := simCmd.Start(); err != nil {
		log.Fatalf("Failed to start simulator: %v", err)
	}

	baseURL = fmt.Sprintf("https://127.0.0.1:%d", simPort)

	if err := waitForHealth(baseURL+"/health", caCertPath); err != nil {
		simCmd.Process.Kill()
		log.Fatalf("Simulator did not become healthy: %v", err)
	}

	code := m.Run()
	simCmd.Process.Kill()
	simCmd.Wait()
	os.RemoveAll(certDir)
	os.Exit(code)
}

func waitForHealth(url, caCert string) error {
	caPEM, err := os.ReadFile(caCert)
	if err != nil {
		return fmt.Errorf("read CA cert: %w", err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)

	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}
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

func terraformCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("terraform", args...)
	cmd.Dir = filepath.Dir(mustAbs("main.tf"))
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("TF_VAR_endpoint=%s", baseURL),
		fmt.Sprintf("SSL_CERT_FILE=%s", caCertFile),
		"ARM_CLIENT_ID=test-client-id",
		"ARM_CLIENT_SECRET=test-client-secret",
		"ARM_TENANT_ID=11111111-1111-1111-1111-111111111111",
		"ARM_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000001",
		fmt.Sprintf("ARM_ENDPOINT=%s", baseURL),
	)
	return cmd
}

func mustAbs(name string) string {
	p, err := filepath.Abs(name)
	if err != nil {
		log.Fatal(err)
	}
	return p
}
