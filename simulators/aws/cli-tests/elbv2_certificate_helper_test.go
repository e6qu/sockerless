package aws_cli_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// importELBTestCertificate imports an ISSUED certificate with real private-key
// material through the vendor CLI. Transactional HTTPS/TLS listener creation
// must reject pending or fabricated certificates because they cannot terminate
// a real TLS data plane.
func importELBTestCertificate(t *testing.T, domain string) string {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate ELB test private key: %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certificateDER, err := x509.CreateCertificate(
		rand.Reader, template, template, &privateKey.PublicKey, privateKey,
	)
	if err != nil {
		t.Fatalf("create ELB test certificate: %v", err)
	}
	dir := t.TempDir()
	certificatePath := filepath.Join(dir, "certificate.pem")
	privateKeyPath := filepath.Join(dir, "private-key.pem")
	if err := os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: certificateDER,
	}), 0o600); err != nil {
		t.Fatalf("write ELB test certificate: %v", err)
	}
	if err := os.WriteFile(privateKeyPath, pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}), 0o600); err != nil {
		t.Fatalf("write ELB test private key: %v", err)
	}
	return strings.TrimSpace(runCLI(t, awsCLI(
		"acm", "import-certificate",
		"--certificate", "fileb://"+certificatePath,
		"--private-key", "fileb://"+privateKeyPath,
		"--query", "CertificateArn",
		"--output", "text",
	)))
}
