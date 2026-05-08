package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// registerMetadata serves the Azure cloud metadata endpoint used by both:
//   - azurestack provider (via ARM_METADATA_HOST): expects JSON array, api-version=2020-06-01
//   - azurerm v3 provider (via ARM_METADATA_HOSTNAME): expects single JSON object, api-version=2022-09-01
//
// The response redirects all Azure service URLs back to the simulator.
func registerMetadata(srv *sim.Server) {
	srv.HandleFunc("GET /metadata/endpoints", func(w http.ResponseWriter, r *http.Request) {
		host := r.Host

		// Detect scheme from the incoming request. If X-Forwarded-Proto is
		// set, honour it; otherwise fall back to whether TLS is active.
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if fp := r.Header.Get("X-Forwarded-Proto"); fp != "" {
			scheme = strings.ToLower(fp)
		}
		baseURL := fmt.Sprintf("%s://%s", scheme, host)

		env := map[string]any{
			"name": "AzureCloud",
			"authentication": map[string]any{
				"loginEndpoint": baseURL,
				"audiences": []string{
					baseURL + "/",
					"https://management.core.windows.net/",
					"https://management.azure.com/",
				},
				"tenant":           "common",
				"identityProvider": "AAD",
			},
			// No trailing slashes — go-azure-sdk prepends this to paths like
			// "/subscriptions/..." and a trailing slash would create "//subscriptions/..."
			// which triggers 301 redirects that change PUT→GET.
			"resourceManager":          baseURL,
			"microsoftGraphResourceId": baseURL + "/",
			"graph":                    baseURL,
			"portal":                   baseURL,
			"gallery":                  baseURL,
			"batch":                    baseURL,
			"suffixes": map[string]any{
				"keyVaultDns":       "localhost",
				"storage":           host,
				"acrLoginServer":    "localhost",
				"sqlServerHostname": "localhost",
			},
		}

		apiVersion := r.URL.Query().Get("api-version")
		if apiVersion == "2022-09-01" {
			// azurerm v3 (go-azure-sdk): expects a single object
			sim.WriteJSON(w, http.StatusOK, env)
		} else {
			// azurestack / older (go-azure-helpers): expects an array
			sim.WriteJSON(w, http.StatusOK, []any{env})
		}
	})

	// Phase 135c — Azure IMDS instance metadata. Real Azure exposes
	//
	//   GET http://169.254.169.254/metadata/instance?api-version=2021-02-01
	//
	// returning a {compute, network} document used by:
	//   - DefaultAzureCredential's IMDS probe.
	//   - Workloads that read `compute.subscriptionId`, `compute.location`,
	//     `compute.azEnvironment` for self-discovery.
	// All reads require `Metadata: true` request header.
	mustMetadataHeader := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Metadata") != "true" {
			http.Error(w, "Required Metadata: true header missing", http.StatusBadRequest)
			return false
		}
		return true
	}
	srv.HandleFunc("GET /metadata/instance", func(w http.ResponseWriter, r *http.Request) {
		if !mustMetadataHeader(w, r) {
			return
		}
		sub := r.URL.Query().Get("subscriptionId")
		if sub == "" {
			sub = "00000000-0000-0000-0000-000000000001"
		}
		loc := r.URL.Query().Get("location")
		if loc == "" {
			loc = "westeurope"
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"compute": map[string]any{
				"azEnvironment":        "AzurePublicCloud",
				"location":             loc,
				"name":                 "sim-vm-1",
				"offer":                "UbuntuServer",
				"osType":               "Linux",
				"placementGroupId":     "",
				"platformFaultDomain":  "0",
				"platformUpdateDomain": "0",
				"provider":             "Microsoft.Compute",
				"publisher":            "Canonical",
				"resourceGroupName":    "sim-rg",
				"resourceId":           fmt.Sprintf("/subscriptions/%s/resourceGroups/sim-rg/providers/Microsoft.Compute/virtualMachines/sim-vm-1", sub),
				"sku":                  "22_04-lts",
				"subscriptionId":       sub,
				"tags":                 "",
				"version":              "22.04.202401010",
				"vmId":                 "sim-vm-id-0001",
				"vmScaleSetName":       "",
				"vmSize":               "Standard_DS1_v2",
				"zone":                 "1",
			},
			"network": map[string]any{
				"interface": []map[string]any{{
					"ipv4": map[string]any{
						"ipAddress": []map[string]any{{
							"privateIpAddress": "10.0.0.4",
							"publicIpAddress":  "",
						}},
						"subnet": []map[string]any{{
							"address": "10.0.0.0",
							"prefix":  "24",
						}},
					},
					"macAddress": "00155DEADBEE",
				}},
			},
		})
	})
	srv.HandleFunc("GET /metadata/instance/compute", func(w http.ResponseWriter, r *http.Request) {
		if !mustMetadataHeader(w, r) {
			return
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"location":          "westeurope",
			"subscriptionId":    "00000000-0000-0000-0000-000000000001",
			"resourceGroupName": "sim-rg",
			"name":              "sim-vm-1",
			"vmId":              "sim-vm-id-0001",
			"azEnvironment":     "AzurePublicCloud",
		})
	})
}

// simListenAddr is captured by main() so host translators can wire it
// into workload-host env. Workloads in Docker reach the sim host via
// host.docker.internal.
var simListenAddr string

func simHostMetadataAddr() string {
	port := simListenAddr
	if idx := strings.LastIndex(simListenAddr, ":"); idx >= 0 {
		port = simListenAddr[idx+1:]
	}
	return "host.docker.internal:" + port
}

// hostMetadataExtraHosts returns ExtraHosts entries needed for the
// workload to resolve host.docker.internal to the sim's host gateway.
// Real Azure IMDS uses 169.254.169.254 (a link-local IP); workloads
// that hard-code that address need a routing override which Linux
// Docker can't easily express. The Azure SDK respects IDENTITY_ENDPOINT
// + IDENTITY_HEADER + AZURE_INSTANCE_METADATA_ENDPOINT for redirection,
// so SDK-based workloads route via env without needing the link-local.
func hostMetadataExtraHosts() []string {
	info := strings.ToLower(sim.RuntimeInfo())
	if strings.Contains(info, "podman") {
		return nil
	}
	return []string{"host.docker.internal:host-gateway"}
}

// hostMetadataEnv returns env vars to inject on every Azure workload
// host so the Azure SDKs route metadata + identity reads to the sim.
// Apply on every ACA / AZF / App Service workload host.
func hostMetadataEnv() map[string]string {
	addr := simHostMetadataAddr()
	return map[string]string{
		// DefaultAzureCredential picks up these two for managed-identity
		// token acquisition (App Service / Container Apps style).
		"IDENTITY_ENDPOINT": "http://" + addr + "/msi/token",
		"IDENTITY_HEADER":   "sim-identity-header",
		// Azure SDK respects this for IMDS instance metadata routing.
		"AZURE_INSTANCE_METADATA_ENDPOINT": "http://" + addr + "/metadata/instance",
	}
}

// mergeEnv returns a new map with all keys from `base` and `extra`,
// where `extra` wins on conflict. Both inputs may be nil.
func mergeEnv(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
