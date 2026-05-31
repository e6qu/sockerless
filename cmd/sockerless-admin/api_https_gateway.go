package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const httpsGatewayName = "https-gateway"

type HTTPSGatewayInfo struct {
	Running   bool              `json:"running"`
	PID       int               `json:"pid"`
	Port      int               `json:"port"`
	AdminPort int               `json:"admin_port"`
	CAPath    string            `json:"ca_path"`
	CAPresent bool              `json:"ca_present"`
	Endpoints map[string]string `json:"endpoints"`
	Commands  []string          `json:"commands"`
}

func registerHTTPSGatewayAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/https-gateway", handleHTTPSGateway())
}

func handleHTTPSGateway() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, readHTTPSGatewayInfo())
	}
}

func readHTTPSGatewayInfo() HTTPSGatewayInfo {
	root := discoverRepoRoot()
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	pid, running := readPidFile(filepath.Join(root, ".stack-pids", httpsGatewayName+".pid"))
	env := readKeyValueFile(filepath.Join(root, ".stack-pids", httpsGatewayName+".env"))

	port := intFromEnv(env, "SOCKERLESS_HTTPS_GATEWAY_PORT", 8443)
	adminPort := intFromEnv(env, "SOCKERLESS_HTTPS_GATEWAY_ADMIN_PORT", 28443)
	caPath := env["SOCKERLESS_HTTPS_GATEWAY_CA_CERT"]
	if caPath == "" {
		caPath = filepath.Join(root, ".sockerless-state", "https-gateway", "data", "caddy", "pki", "authorities", "local", "root.crt")
	}
	_, caErr := os.Stat(caPath)

	return HTTPSGatewayInfo{
		Running:   running,
		PID:       pid,
		Port:      port,
		AdminPort: adminPort,
		CAPath:    caPath,
		CAPresent: caErr == nil,
		Endpoints: map[string]string{
			"aws":              "https://aws.sockerless.localhost:" + strconv.Itoa(port),
			"gcp":              "https://gcp.sockerless.localhost:" + strconv.Itoa(port),
			"azure":            "https://azure.sockerless.localhost:" + strconv.Itoa(port),
			"azure_blob":       "https://{account}.blob.azure.sockerless.localhost:" + strconv.Itoa(port),
			"azure_key_vault":  "https://{vault}.vault.azure.sockerless.localhost:" + strconv.Itoa(port),
			"azure_servicebus": "https://{namespace}.servicebus.azure.sockerless.localhost:" + strconv.Itoa(port),
		},
		Commands: []string{
			"make stack-https-up",
			"make stack-https-status",
			"make stack-https-ca",
			"make stack-https-down",
		},
	}
}

func readPidFile(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	return pid, proc.Signal(syscall0()) == nil
}

func readKeyValueFile(path string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

func intFromEnv(env map[string]string, key string, def int) int {
	raw := env[key]
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return def
	}
	return v
}
