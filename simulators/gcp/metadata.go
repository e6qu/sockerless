package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// registerComputeMetadata serves the GCE metadata server endpoints used
// by every GCP compute primitive that runs a workload (GCE, Cloud Run,
// Cloud Functions, App Engine). Real GCP exposes the service at
//
//	metadata.google.internal     (resolves to 169.254.169.254)
//	metadata                     (short alias)
//
// on port 80, requiring the `Metadata-Flavor: Google` request header on
// every read. Workloads in the sim reach it via the sim's main HTTP
// listener; cloud-product translators inject `GCE_METADATA_HOST` and
// `GCE_METADATA_IP` env vars on the workload host so the GCP Go/Python
// SDKs pick them up automatically.
//
// Coverage today: project ID, default-zone, instance ID + zone + name,
// service-account default access tokens (delegates to the existing IAM
// `iamcredentials.generateAccessToken` shape) and ID tokens (delegates
// to `iamcredentials.generateIdToken`). Not yet covered: instance
// attributes, disks, network interfaces, startup-script — add as
// workloads need them per the sim-parity-per-commit rule.
func registerComputeMetadata(srv *sim.Server) {
	mustFlavor := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Metadata-Flavor") != "Google" {
			http.Error(w, "Missing Metadata-Flavor:Google header.", http.StatusForbidden)
			return false
		}
		w.Header().Set("Metadata-Flavor", "Google")
		w.Header().Set("Server", "Metadata Server for VM")
		return true
	}
	writeText := func(w http.ResponseWriter, s string) {
		w.Header().Set("Content-Type", "application/text")
		_, _ = w.Write([]byte(s))
	}

	// /computeMetadata/v1/project/project-id
	srv.HandleFunc("GET /computeMetadata/v1/project/project-id", func(w http.ResponseWriter, r *http.Request) {
		if !mustFlavor(w, r) {
			return
		}
		writeText(w, defaultMetadataProject(r))
	})

	srv.HandleFunc("GET /computeMetadata/v1/project/numeric-project-id", func(w http.ResponseWriter, r *http.Request) {
		if !mustFlavor(w, r) {
			return
		}
		writeText(w, "1000000000001")
	})

	// /computeMetadata/v1/instance/{id|zone|name|hostname}
	srv.HandleFunc("GET /computeMetadata/v1/instance/id", func(w http.ResponseWriter, r *http.Request) {
		if !mustFlavor(w, r) {
			return
		}
		writeText(w, "1000000000001")
	})
	srv.HandleFunc("GET /computeMetadata/v1/instance/zone", func(w http.ResponseWriter, r *http.Request) {
		if !mustFlavor(w, r) {
			return
		}
		writeText(w, fmt.Sprintf("projects/%s/zones/%s", defaultMetadataProject(r), defaultMetadataZone(r)))
	})
	srv.HandleFunc("GET /computeMetadata/v1/instance/name", func(w http.ResponseWriter, r *http.Request) {
		if !mustFlavor(w, r) {
			return
		}
		writeText(w, "sim-instance-1")
	})
	srv.HandleFunc("GET /computeMetadata/v1/instance/hostname", func(w http.ResponseWriter, r *http.Request) {
		if !mustFlavor(w, r) {
			return
		}
		writeText(w, fmt.Sprintf("sim-instance-1.%s.c.%s.internal", defaultMetadataZone(r), defaultMetadataProject(r)))
	})

	// /computeMetadata/v1/instance/service-accounts/{sa}/{leaf}
	//
	// `default` is an alias for the project's default compute SA. The
	// sim accepts any SA name and stamps the requested audience into
	// the response (matches real GCE behaviour).
	srv.HandleFunc("GET /computeMetadata/v1/instance/service-accounts/{sa}/email", func(w http.ResponseWriter, r *http.Request) {
		if !mustFlavor(w, r) {
			return
		}
		sa := sim.PathParam(r, "sa")
		if sa == "default" {
			sa = fmt.Sprintf("default@%s.iam.gserviceaccount.com", defaultMetadataProject(r))
		}
		writeText(w, sa)
	})
	srv.HandleFunc("GET /computeMetadata/v1/instance/service-accounts/{sa}/scopes", func(w http.ResponseWriter, r *http.Request) {
		if !mustFlavor(w, r) {
			return
		}
		// Cloud-platform broad scope is what real GCE returns by default.
		writeText(w, "https://www.googleapis.com/auth/cloud-platform")
	})
	srv.HandleFunc("GET /computeMetadata/v1/instance/service-accounts/{sa}/aliases", func(w http.ResponseWriter, r *http.Request) {
		if !mustFlavor(w, r) {
			return
		}
		writeText(w, "default")
	})

	// /computeMetadata/v1/instance/service-accounts/{sa}/token
	// Bearer access token. Mints a sim token; the sim's gating endpoints
	// do not verify the token signature today (matches the sim's
	// existing fakeCredential pattern for ARM).
	srv.HandleFunc("GET /computeMetadata/v1/instance/service-accounts/{sa}/token", func(w http.ResponseWriter, r *http.Request) {
		if !mustFlavor(w, r) {
			return
		}
		// Real GCE returns JSON: {access_token,expires_in,token_type}.
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"sim-access-token","expires_in":%d,"token_type":"Bearer"}`, 3599)
	})

	// /computeMetadata/v1/instance/service-accounts/{sa}/identity?audience=...
	// ID token. Delegates to the existing mintSimIdToken helper used by
	// iamcredentials.generateIdToken (so the JWT shape round-trips).
	srv.HandleFunc("GET /computeMetadata/v1/instance/service-accounts/{sa}/identity", func(w http.ResponseWriter, r *http.Request) {
		if !mustFlavor(w, r) {
			return
		}
		audience := r.URL.Query().Get("audience")
		if audience == "" {
			http.Error(w, "non-empty audience parameter required", http.StatusBadRequest)
			return
		}
		sa := sim.PathParam(r, "sa")
		if sa == "default" {
			sa = fmt.Sprintf("default@%s.iam.gserviceaccount.com", defaultMetadataProject(r))
		}
		now := time.Now()
		expires := now.Add(time.Hour)
		token := mintSimIdToken(idTokenSignKey(), sa, audience, true, now, expires)
		w.Header().Set("Content-Type", "application/text")
		_, _ = w.Write([]byte(token))
	})
}

func defaultMetadataProject(r *http.Request) string {
	if v := r.URL.Query().Get("project"); v != "" {
		return v
	}
	return "sim-project"
}

func defaultMetadataZone(r *http.Request) string {
	if v := r.URL.Query().Get("zone"); v != "" {
		return v
	}
	return "us-central1-a"
}

// simListenAddr is captured by main() so host translators can wire it
// into workload-host env. Workloads in Docker reach the sim host via
// host.docker.internal.
var simListenAddr string

// hostMetadataAddr returns the address workloads use to reach the sim's
// metadata service. Cloud-product translators inject this as
// GCE_METADATA_HOST on the workload host so the GCP SDKs route metadata
// reads here instead of attempting metadata.google.internal:80.
func hostMetadataAddr() string {
	port := simListenAddr
	if idx := strings.LastIndex(simListenAddr, ":"); idx >= 0 {
		port = simListenAddr[idx+1:]
	}
	return "host.docker.internal:" + port
}

// hostMetadataExtraHosts returns ExtraHosts entries needed for the
// workload to resolve host.docker.internal AND metadata.google.internal
// to the sim's host gateway. Workloads that read GCE_METADATA_HOST will
// use the explicit address; workloads that hard-code metadata.google.internal
// will resolve it to host.docker.internal via /etc/hosts.
func hostMetadataExtraHosts() []string {
	info := strings.ToLower(sim.RuntimeInfo())
	if strings.Contains(info, "podman") {
		// Podman exposes host.docker.internal natively.
		return []string{"metadata.google.internal:host.docker.internal"}
	}
	return []string{
		"host.docker.internal:host-gateway",
		"metadata.google.internal:host-gateway",
		"metadata:host-gateway",
	}
}

// hostMetadataEnv returns env vars to inject on every GCP workload host
// so the GCP SDKs route metadata-server reads to the sim. Apply on every
// Cloud Run / Cloud Run Jobs / Cloud Functions / GCE-style workload host.
func hostMetadataEnv() map[string]string {
	addr := hostMetadataAddr()
	return map[string]string{
		"GCE_METADATA_HOST": addr,
		"GCE_METADATA_IP":   addr,
		"GCE_METADATA_ROOT": addr,
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
