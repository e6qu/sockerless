//go:build dqlite

package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/canonical/go-dqlite/v3/app"
	"github.com/canonical/go-dqlite/v3/client"
)

func main() {
	dataDir := requiredEnv("BLEEPHUB_DQLITE_DATA_DIR")
	address := requiredEnv("BLEEPHUB_DQLITE_ADVERTISE_ADDR")
	join := csvEnv("BLEEPHUB_DQLITE_JOIN")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Fatalf("create dqlite data directory %s: %v", dataDir, err)
	}

	listener, err := net.Listen("tcp", ":9000")
	if err != nil {
		log.Fatalf("listen dqlite transport: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn)
	var ready atomic.Bool
	server := &http.Server{Handler: dqliteHandler(accepted, &ready)}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("dqlite transport server: %v", err)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	node, err := app.New(
		dataDir,
		app.WithAddress(address),
		app.WithCluster(join),
		app.WithExternalConn(dqliteHTTPDial, accepted),
		app.WithVoters(3),
	)
	if err != nil {
		log.Fatalf("start dqlite node: %v", err)
	}
	var closeOnce sync.Once
	closeNode := func() {
		closeOnce.Do(func() {
			if err := node.Close(); err != nil {
				log.Printf("close dqlite node: %v", err)
			}
		})
	}
	go func() {
		<-ctx.Done()
		closeNode()
	}()
	if err := node.Ready(ctx); err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Fatalf("wait for dqlite quorum: %v", err)
	}
	ready.Store(true)
	log.Printf("dqlite quorum ready address=%s", address)
	<-ctx.Done()
	ready.Store(false)
	// The wake coordinator intentionally stops every voter together during an
	// idle transition. A handover needs another live voter, so direct closure
	// is the correct dqlite shutdown operation for that complete-quorum case.
	closeNode()
}

func requiredEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func csvEnv(name string) []string {
	values := []string{}
	for _, value := range strings.Split(os.Getenv(name), ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func dqliteHandler(accepted chan<- net.Conn, ready *atomic.Bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			if ready.Load() {
				w.WriteHeader(http.StatusOK)
				return
			}
			http.Error(w, "dqlite quorum is not ready", http.StatusServiceUnavailable)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/dqlite" || !strings.EqualFold(r.Header.Get("Upgrade"), "dqlite") {
			http.Error(w, "dqlite upgrade required", http.StatusUpgradeRequired)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "connection hijacking unavailable", http.StatusInternalServerError)
			return
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		accepted <- connection
	})
}

func dqliteHTTPDial(ctx context.Context, address string) (net.Conn, error) {
	dialer := &net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	requestURL, err := url.Parse("http://" + address + "/dqlite")
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	request := &http.Request{Method: http.MethodGet, URL: requestURL, Host: requestURL.Host, Header: make(http.Header)}
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "dqlite")
	if err := request.Write(connection); err != nil {
		_ = connection.Close()
		return nil, err
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), request)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	if response.StatusCode != http.StatusSwitchingProtocols || !strings.EqualFold(response.Header.Get("Upgrade"), "dqlite") {
		_ = response.Body.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("dqlite endpoint %s refused upgrade: %s", address, response.Status)
	}
	return connection, nil
}

var _ client.DialFunc = dqliteHTTPDial
