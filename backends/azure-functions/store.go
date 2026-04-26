package azf

import "strings"

// invokeURLForHost returns the URL the backend POSTs to invoke a function
// app. In sim mode the simulator hosts every site on a single endpoint, so
// the URL points at that endpoint and the caller is expected to set the
// HTTP Host header to the function app's DefaultHostName so the simulator's
// virtual-host routing matches the right site (mirrors how real Azure routes
// per-app traffic). In real Azure mode every app has its own DNS-resolvable
// hostname, so the URL host equals the Host header — both forms collapse to
// the same `https://<host>/api/function`.
func invokeURLForHost(endpointURL, host string) string {
	scheme := "https"
	if strings.HasPrefix(endpointURL, "http://") {
		scheme = "http"
	}
	tcpHost := host
	if endpointURL != "" {
		// Strip scheme prefix; what's left is the TCP host[:port].
		tcpHost = strings.TrimPrefix(strings.TrimPrefix(endpointURL, "http://"), "https://")
		tcpHost = strings.TrimSuffix(tcpHost, "/")
	}
	return scheme + "://" + tcpHost + "/api/function"
}

// AZFState maps sockerless container IDs to Azure Functions resources.
type AZFState struct {
	FunctionAppName string
	ResourceID      string
	// FunctionURL is the URL the backend POSTs to invoke the function.
	// In sim mode this points at the simulator's endpoint with the function
	// app's hostname carried in the Host header (FunctionHost). In real
	// Azure mode they're the same hostname; the sim split mirrors how
	// virtual-hosted services route by Host header even when many sites
	// share an IP.
	FunctionURL  string
	FunctionHost string
}
