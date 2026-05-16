package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// AWS host-metadata services.
// Two distinct metadata layers exist in real AWS:
//
//  1. **EC2 IMDSv2** — `169.254.169.254/latest/meta-data/...` for any
//     EC2-style host (EC2 instance, Fargate task on EC2 launch type).
//     IMDSv2 requires a session token from PUT /latest/api/token.
//
//  2. **ECS task metadata v4** — `${ECS_CONTAINER_METADATA_URI_V4}/task`
//     served per-container on a task-local endpoint. Carries cluster,
//     task ARN, family, container statuses, network interfaces, limits.
//
// Lambda Runtime API (lambda_runtime.go) is its own thing and stays as-is.
//
// Workloads in the sim's Docker hosts reach these endpoints via the
// sim's main listener: cloud-product translators inject
// AWS_EC2_METADATA_SERVICE_ENDPOINT and ECS_CONTAINER_METADATA_URI_V4
// env vars on the workload host so the AWS Go/Python/JS SDKs route
// metadata reads to the sim's port.

// imdsTokens holds active IMDSv2 session tokens. Real AWS scopes them
// per-instance + per-TTL; the sim accepts any presented token that was
// previously issued, mirroring the API contract without enforcing the
// per-instance binding.
var imdsTokens sync.Map // map[string]time.Time (issued-at)

func registerHostMetadata(srv *sim.Server) {
	// PUT /latest/api/token — IMDSv2 token request.
	srv.HandleFunc("PUT /latest/api/token", func(w http.ResponseWriter, r *http.Request) {
		ttl := r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds")
		if ttl == "" {
			ttl = "21600"
		}
		buf := make([]byte, 28)
		_, _ = rand.Read(buf)
		token := "AQ" + hex.EncodeToString(buf)
		imdsTokens.Store(token, time.Now())
		w.Header().Set("X-Aws-Ec2-Metadata-Token-Ttl-Seconds", ttl)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(token))
	})

	mustToken := func(w http.ResponseWriter, r *http.Request) bool {
		token := r.Header.Get("X-aws-ec2-metadata-token")
		if token == "" {
			http.Error(w, "IMDSv2 requires X-aws-ec2-metadata-token", http.StatusUnauthorized)
			return false
		}
		if _, ok := imdsTokens.Load(token); !ok {
			http.Error(w, "unknown IMDSv2 token", http.StatusUnauthorized)
			return false
		}
		return true
	}
	writeText := func(w http.ResponseWriter, s string) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(s))
	}

	// /latest/meta-data/ — top-level index. Real IMDS returns a
	// newline-separated leaf list; minimal coverage here.
	srv.HandleFunc("GET /latest/meta-data/", func(w http.ResponseWriter, r *http.Request) {
		if !mustToken(w, r) {
			return
		}
		writeText(w, "ami-id\ninstance-id\ninstance-type\nplacement/\niam/\nidentity-credentials/\n")
	})

	srv.HandleFunc("GET /latest/meta-data/instance-id", func(w http.ResponseWriter, r *http.Request) {
		if !mustToken(w, r) {
			return
		}
		writeText(w, "i-0abcdef1234567890")
	})
	srv.HandleFunc("GET /latest/meta-data/instance-type", func(w http.ResponseWriter, r *http.Request) {
		if !mustToken(w, r) {
			return
		}
		writeText(w, "t3.micro")
	})
	srv.HandleFunc("GET /latest/meta-data/ami-id", func(w http.ResponseWriter, r *http.Request) {
		if !mustToken(w, r) {
			return
		}
		writeText(w, "ami-0123456789abcdef0")
	})
	srv.HandleFunc("GET /latest/meta-data/placement/region", func(w http.ResponseWriter, r *http.Request) {
		if !mustToken(w, r) {
			return
		}
		writeText(w, defaultIMDSRegion(r))
	})
	srv.HandleFunc("GET /latest/meta-data/placement/availability-zone", func(w http.ResponseWriter, r *http.Request) {
		if !mustToken(w, r) {
			return
		}
		writeText(w, defaultIMDSRegion(r)+"a")
	})

	// IAM credentials at /latest/meta-data/iam/security-credentials/{role}.
	// Real EC2 returns a JSON document with AccessKeyId, SecretAccessKey,
	// Token, Expiration. Sim returns a stable sim-AK/SK pair.
	srv.HandleFunc("GET /latest/meta-data/iam/security-credentials/", func(w http.ResponseWriter, r *http.Request) {
		if !mustToken(w, r) {
			return
		}
		writeText(w, "sim-instance-role")
	})
	srv.HandleFunc("GET /latest/meta-data/iam/security-credentials/{role}", func(w http.ResponseWriter, r *http.Request) {
		if !mustToken(w, r) {
			return
		}
		exp := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"Code":"Success","LastUpdated":%q,"Type":"AWS-HMAC","AccessKeyId":"ASIASIMACCESSKEY","SecretAccessKey":"sim-secret-access-key","Token":"sim-session-token","Expiration":%q}`,
			time.Now().UTC().Format(time.RFC3339), exp)
	})

	// /latest/dynamic/instance-identity/document — signed JSON document
	// describing the instance. The AWS Go SDK's `imds.GetRegion()` reads
	// this rather than /latest/meta-data/placement/region. Real EC2
	// includes a base64 signature; sim omits the signature (workloads
	// running in the sim don't validate it).
	srv.HandleFunc("GET /latest/dynamic/instance-identity/document", func(w http.ResponseWriter, r *http.Request) {
		if !mustToken(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		region := defaultIMDSRegion(r)
		_, _ = fmt.Fprintf(w, `{
			"accountId": "000000000000",
			"architecture": "arm64",
			"availabilityZone": %q,
			"imageId": "ami-0123456789abcdef0",
			"instanceId": "i-0abcdef1234567890",
			"instanceType": "t3.micro",
			"region": %q,
			"version": "2017-09-30"
		}`, region+"a", region)
	})

	// ECS task metadata v4. Real ECS sets ECS_CONTAINER_METADATA_URI_V4
	// to a per-task local URL like http://169.254.170.2/v4/<id>. Sim
	// serves /v4/<id>/task on its main listener; cloud-product translator
	// passes the full URL via ECS_CONTAINER_METADATA_URI_V4.
	srv.HandleFunc("GET /v4/{id}/task", func(w http.ResponseWriter, r *http.Request) {
		id := sim.PathParam(r, "id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"Cluster": "sockerless-sim",
			"TaskARN": "arn:aws:ecs:%s:000000000000:task/sockerless-sim/%s",
			"Family": "sockerless-sim-task",
			"Revision": "1",
			"DesiredStatus": "RUNNING",
			"KnownStatus": "RUNNING",
			"Containers": [{"Name": "main", "DockerId": %q, "DockerName": %q, "Image": "alpine:latest"}],
			"LaunchType": "FARGATE"
		}`, defaultIMDSRegion(r), id, id, id)
	})
	srv.HandleFunc("GET /v4/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := sim.PathParam(r, "id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"DockerId": %q, "Name": "main", "Image": "alpine:latest"}`, id)
	})
}

func defaultIMDSRegion(r *http.Request) string {
	if v := r.URL.Query().Get("region"); v != "" {
		return v
	}
	return "eu-west-1"
}

// simHostMetadataAddr returns "host.docker.internal:<sim-port>" — the
// address sim workload hosts use to reach the sim's metadata services.
var simListenAddr string

func simHostMetadataAddr() string {
	port := simListenAddr
	if idx := strings.LastIndex(simListenAddr, ":"); idx >= 0 {
		port = simListenAddr[idx+1:]
	}
	return "host.docker.internal:" + port
}

// hostMetadataExtraHosts returns ExtraHosts entries needed for the
// workload to resolve host.docker.internal AND the IMDS link-local
// 169.254.169.254 to the sim's host gateway. The AWS SDK respects
// AWS_EC2_METADATA_SERVICE_ENDPOINT, so workloads that go through the
// SDK don't need the link-local hostname; ExtraHosts is best-effort
// for raw HTTP clients.
func hostMetadataExtraHosts() []string {
	info := strings.ToLower(sim.RuntimeInfo())
	if strings.Contains(info, "podman") {
		return nil
	}
	return []string{"host.docker.internal:host-gateway"}
}

// hostMetadataEnv returns env vars for every AWS workload host so SDKs
// route metadata reads to the sim. Cloud-product translators merge
// these onto the workload's ContainerConfig.Env.
//
// The taskID parameter is the sim-side container ID used as the
// ECS_CONTAINER_METADATA_URI_V4 token. For non-ECS hosts (Lambda)
// pass empty and the env var is omitted.
func hostMetadataEnv(taskID string) map[string]string {
	addr := simHostMetadataAddr()
	env := map[string]string{
		"AWS_EC2_METADATA_SERVICE_ENDPOINT":      "http://" + addr,
		"AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE": "IPv4",
	}
	if taskID != "" {
		env["ECS_CONTAINER_METADATA_URI_V4"] = "http://" + addr + "/v4/" + taskID
		env["ECS_CONTAINER_METADATA_URI"] = "http://" + addr + "/v4/" + taskID
	}
	return env
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
