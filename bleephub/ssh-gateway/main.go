package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	maxConcurrentConnections = 32
	maxConnectionsPerMinute  = 10
	startupTimeout           = 90 * time.Second
)

type sourceRateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
}

func (l *sourceRateLimiter) allow(address string, now time.Time) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	cutoff := now.Add(-time.Minute)
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.requests[host]
	start := 0
	for start < len(recent) && recent[start].Before(cutoff) {
		start++
	}
	recent = recent[start:]
	if len(recent) >= maxConnectionsPerMinute {
		l.requests[host] = recent
		return false
	}
	l.requests[host] = append(recent, now)
	return true
}

func main() {
	listenAddress := valueOr("BLEEPHUB_SSH_GATEWAY_ADDR", ":2222")
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("listen SSH gateway: %v", err)
	}
	defer func() { _ = listener.Close() }()

	limiter := &sourceRateLimiter{requests: make(map[string][]time.Time)}
	connections := make(chan struct{}, maxConcurrentConnections)
	for {
		connection, err := listener.Accept()
		if err != nil {
			log.Printf("accept SSH connection: %v", err)
			continue
		}
		if !limiter.allow(connection.RemoteAddr().String(), time.Now()) {
			_ = connection.Close()
			continue
		}
		select {
		case connections <- struct{}{}:
			go func() {
				defer func() { <-connections }()
				handle(connection)
			}()
		default:
			_ = connection.Close()
		}
	}
}

func handle(client net.Conn) {
	defer func() { _ = client.Close() }()
	if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return
	}
	reader := bufio.NewReaderSize(client, 1024)
	banner, err := reader.ReadString('\n')
	if err != nil || len(banner) > 255 || !strings.HasPrefix(banner, "SSH-2.0-") {
		return
	}
	if err := client.SetReadDeadline(time.Time{}); err != nil {
		return
	}

	upstream, err := wakeAndConnect(context.Background())
	if err != nil {
		log.Printf("connect Bleephub SSH service: %v", err)
		return
	}
	defer func() { _ = upstream.Close() }()
	if _, err := io.WriteString(upstream, banner); err != nil {
		return
	}
	go func() { _, _ = io.Copy(upstream, reader) }()
	_, _ = io.Copy(client, upstream)
}

func wakeAndConnect(ctx context.Context) (net.Conn, error) {
	wakeURL := os.Getenv("BLEEPHUB_WAKE_URL")
	target := os.Getenv("BLEEPHUB_INTERNAL_SSH_TARGET")
	if wakeURL == "" || target == "" {
		return nil, fmt.Errorf("BLEEPHUB_WAKE_URL and BLEEPHUB_INTERNAL_SSH_TARGET are required")
	}

	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, wakeURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create bleephub wake request: %w", err)
		}
		response, err := (&http.Client{Timeout: 5 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}).Do(request)
		if err == nil {
			_ = response.Body.Close()
		}
		connection, err := net.DialTimeout("tcp", target, 5*time.Second)
		if err == nil {
			return connection, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("bleephub SSH service did not become reachable within %s", startupTimeout)
}

func valueOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
