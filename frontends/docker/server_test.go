package frontend

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// genSelfSignedCert writes a self-signed cert+key to tmpDir and returns the paths.
func genSelfSignedCert(t *testing.T, tmpDir string) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	certFile = filepath.Join(tmpDir, "cert.pem")
	keyFile = filepath.Join(tmpDir, "key.pem")

	cf, _ := os.Create(certFile)
	pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	cf.Close()

	keyDER, _ := x509.MarshalECPrivateKey(key)
	kf, _ := os.Create(keyFile)
	pem.Encode(kf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	kf.Close()

	return certFile, keyFile
}

func TestTLSListenerAcceptsHTTPS(t *testing.T) {
	tmpDir := t.TempDir()
	certFile, keyFile := genSelfSignedCert(t, tmpDir)

	logger := zerolog.Nop()
	s := NewServer(logger, "http://localhost:9100")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	go func() {
		_ = s.ListenAndServe(addr, certFile, keyFile)
	}()
	time.Sleep(100 * time.Millisecond)

	// Load cert for verification
	certPEM, _ := os.ReadFile(certFile)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}
	resp, err := client.Get("https://" + addr + "/_ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestNonTLSFallback(t *testing.T) {
	logger := zerolog.Nop()
	s := NewServer(logger, "http://localhost:9100")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	go func() {
		_ = s.ListenAndServe(addr, "", "")
	}()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/_ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestUnixSocketIgnoresTLS(t *testing.T) {
	tmpDir := t.TempDir()
	certFile, keyFile := genSelfSignedCert(t, tmpDir)
	sockPath := filepath.Join(tmpDir, "docker.sock")

	logger := zerolog.Nop()
	s := NewServer(logger, "http://localhost:9100")

	go func() {
		_ = s.ListenAndServe(sockPath, certFile, keyFile)
	}()
	time.Sleep(100 * time.Millisecond)

	// Unix socket connection (plain HTTP, TLS args ignored)
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
	}
	resp, err := client.Get("http://localhost/_ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMgmtTLS(t *testing.T) {
	tmpDir := t.TempDir()
	certFile, keyFile := genSelfSignedCert(t, tmpDir)

	logger := zerolog.Nop()
	mgmt := NewMgmtServer(logger, ":2375", "http://localhost:9100")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	go func() {
		_ = mgmt.ListenAndServe(addr, certFile, keyFile)
	}()
	time.Sleep(100 * time.Millisecond)

	certPEM, _ := os.ReadFile(certFile)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}
	resp, err := client.Get("https://" + addr + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
