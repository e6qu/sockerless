package main

import (
	"net"
	"net/http"
	"strings"
)

func azureRequestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func azureRequestHostParts(r *http.Request) (string, string) {
	host := r.Host
	if h, p, err := net.SplitHostPort(host); err == nil {
		return h, ":" + p
	}
	if strings.Count(host, ":") == 1 {
		if i := strings.LastIndex(host, ":"); i >= 0 {
			return host[:i], host[i:]
		}
	}
	return host, ""
}

func azureEndpointHost(r *http.Request, parts ...string) string {
	hostname, portSuffix := azureRequestHostParts(r)
	return strings.Join(append(parts, hostname), ".") + portSuffix
}

func azureEndpointHostname(r *http.Request, parts ...string) string {
	hostname, _ := azureRequestHostParts(r)
	return strings.Join(append(parts, hostname), ".")
}
