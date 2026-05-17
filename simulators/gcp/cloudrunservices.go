package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// enumString accepts both proto-JSON enum encodings: the canonical
// string ("INGRESS_TRAFFIC_INTERNAL_ONLY") and the numeric form (4)
// emitted by some Go REST clients (run/apiv2.NewServicesRESTClient
// serializes IngressTraffic as a number even though the wire spec
// allows both). Real Cloud Run accepts either; the sim must too.
type enumString string

func (e *enumString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*e = ""
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*e = enumString(s)
		return nil
	}
	// Numeric enum form — keep the digits as the string value.
	// The sim doesn't validate ingress against a known set; readers
	// only round-trip it on Get/List, so preserving the bytes works.
	*e = enumString(string(data))
	return nil
}

func (e enumString) MarshalJSON() ([]byte, error) {
	if e == "" {
		return []byte("null"), nil
	}
	return json.Marshal(string(e))
}

type vpcEgressString string

func (e *vpcEgressString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*e = ""
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*e = vpcEgressString(s)
		return nil
	}
	switch strings.TrimSpace(string(data)) {
	case "1":
		*e = "ALL_TRAFFIC"
	case "2":
		*e = "PRIVATE_RANGES_ONLY"
	case "0":
		*e = ""
	default:
		return fmt.Errorf("unknown VpcAccess egress enum %s", data)
	}
	return nil
}

func (e vpcEgressString) MarshalJSON() ([]byte, error) {
	if e == "" {
		return []byte("null"), nil
	}
	return json.Marshal(string(e))
}

// Cloud Run v2 services slice. The cloudrun backend uses
// cloud.google.com/go/run/apiv2 (REST client) which talks to the v2
// REST surface — `/v2/projects/{project}/locations/{location}/services`
// — not the v1 Knative paths handled in cloudrun.go. When
// Config.UseService=true the backend hits these endpoints; without
// them every Service call 404s.
//
// Real API: https://cloud.google.com/run/docs/reference/rest/v2/projects.locations.services

// ServiceV2 is the v2 Cloud Run Service (proto-JSON shape, not the v1
// Knative shape in CRService). Field set is the subset the cloudrun
// backend reads via runpb.Service: name, labels, annotations,
// createTime, template (with containers + env), terminalCondition,
// latestReadyRevision. Generation is encoded as a JSON string per
// proto-JSON int64 rules.
type ServiceV2 struct {
	Name                  string            `json:"name"`
	UID                   string            `json:"uid,omitempty"`
	Generation            int64             `json:"generation,string,omitempty"`
	Labels                map[string]string `json:"labels,omitempty"`
	Annotations           map[string]string `json:"annotations,omitempty"`
	CreateTime            string            `json:"createTime,omitempty"`
	UpdateTime            string            `json:"updateTime,omitempty"`
	LaunchStage           enumString        `json:"launchStage,omitempty"`
	Ingress               enumString        `json:"ingress,omitempty"`
	DefaultUriDisabled    bool              `json:"defaultUriDisabled,omitempty"`
	Template              *RevisionTemplate `json:"template,omitempty"`
	Traffic               []TrafficTarget   `json:"traffic,omitempty"`
	TerminalCondition     *Condition        `json:"terminalCondition,omitempty"`
	Conditions            []Condition       `json:"conditions,omitempty"`
	LatestReadyRevision   string            `json:"latestReadyRevision,omitempty"`
	LatestCreatedRevision string            `json:"latestCreatedRevision,omitempty"`
	URI                   string            `json:"uri,omitempty"`
	Reconciling           bool              `json:"reconciling,omitempty"`
}

// RevisionTemplate is the v2 Cloud Run revision template. Mirrors the
// runpb.RevisionTemplate fields the backend's buildServiceSpec sets.
type RevisionTemplate struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Containers  []Container       `json:"containers,omitempty"`
	Volumes     []Volume          `json:"volumes,omitempty"`
	Scaling     *RevisionScaling  `json:"scaling,omitempty"`
	VpcAccess   *VpcAccess        `json:"vpcAccess,omitempty"`
	Timeout     string            `json:"timeout,omitempty"`
}

// RevisionScaling caps min/max instance counts for a Cloud Run service
// revision. The backend pins both to 1 today (long-running, single-
// instance pattern) but the proto-JSON shape always carries them.
type RevisionScaling struct {
	MinInstanceCount int32 `json:"minInstanceCount,omitempty"`
	MaxInstanceCount int32 `json:"maxInstanceCount,omitempty"`
}

// VpcAccess wires a service revision to a Serverless VPC Access
// connector so peer containers can reach the service over its
// internal-ingress IP. The backend sets this when Config.VPCConnector
// is non-empty.
type VpcAccess struct {
	Connector string          `json:"connector,omitempty"`
	Egress    vpcEgressString `json:"egress,omitempty"`
}

// TrafficTarget is one entry in the Service's traffic-split list.
type TrafficTarget struct {
	Type     string `json:"type,omitempty"`
	Revision string `json:"revision,omitempty"`
	Percent  int32  `json:"percent,omitempty"`
	Tag      string `json:"tag,omitempty"`
}

func containerEnvMap(envVars []EnvVar) map[string]string {
	if len(envVars) == 0 {
		return nil
	}
	env := make(map[string]string, len(envVars))
	for _, ev := range envVars {
		env[ev.Name] = ev.Value
	}
	return env
}

type cloudRunServiceInstance struct {
	containerID  string
	hostPort     int
	image        string
	envSignature string
	cancelLogs   context.CancelFunc
}

var cloudRunServiceInstances = struct {
	sync.Mutex
	byName map[string]*cloudRunServiceInstance
}{byName: map[string]*cloudRunServiceInstance{}}

func ensureCloudRunServiceInstance(ctx context.Context, name, serviceID, image string, env map[string]string, sink sim.LogSink) (*cloudRunServiceInstance, error) {
	localImage := sim.ResolveLocalImage(image)
	envSig := envSignature(env)

	cloudRunServiceInstances.Lock()
	if inst := cloudRunServiceInstances.byName[name]; inst != nil && inst.image == localImage && inst.envSignature == envSig {
		cloudRunServiceInstances.Unlock()
		return inst, nil
	}
	old := cloudRunServiceInstances.byName[name]
	delete(cloudRunServiceInstances.byName, name)
	cloudRunServiceInstances.Unlock()
	stopCloudRunServiceInstance(old)

	platform, err := localImagePlatform(ctx, localImage)
	if err != nil {
		return nil, err
	}
	hostPort, err := pickFreeTCPPort()
	if err != nil {
		return nil, fmt.Errorf("pick free port: %w", err)
	}
	containerID, err := sim.StartHTTPContainer(ctx, sim.HTTPContainerConfig{
		Image:        localImage,
		Architecture: platform,
		HostPort:     hostPort,
		Env: mergeEnv(mergeEnv(map[string]string{
			"PORT": "8080",
		}, env), hostMetadataEnv()),
		Name:       fmt.Sprintf("sockerless-sim-cloudrun-svc-%s-%d", serviceID, hostPort),
		Labels:     map[string]string{"sockerless-sim-service": serviceID},
		ExtraHosts: hostMetadataExtraHosts(),
		Sandbox:    sim.SandboxCloudRun,
	})
	if err != nil {
		return nil, fmt.Errorf("start service container: %w", err)
	}

	logCtx, cancelLogs := context.WithCancel(context.Background())
	go sim.StreamContainerLogs(logCtx, containerID, sink)

	inst := &cloudRunServiceInstance{
		containerID:  containerID,
		hostPort:     hostPort,
		image:        localImage,
		envSignature: envSig,
		cancelLogs:   cancelLogs,
	}
	cloudRunServiceInstances.Lock()
	if old := cloudRunServiceInstances.byName[name]; old != nil {
		cloudRunServiceInstances.Unlock()
		stopCloudRunServiceInstance(inst)
		return nil, fmt.Errorf("service %q instance replaced while starting", name)
	}
	cloudRunServiceInstances.byName[name] = inst
	cloudRunServiceInstances.Unlock()
	return inst, nil
}

func deleteCloudRunServiceInstance(name string) {
	cloudRunServiceInstances.Lock()
	inst := cloudRunServiceInstances.byName[name]
	delete(cloudRunServiceInstances.byName, name)
	cloudRunServiceInstances.Unlock()
	stopCloudRunServiceInstance(inst)
}

func stopCloudRunServiceInstance(inst *cloudRunServiceInstance) {
	if inst == nil {
		return
	}
	if inst.cancelLogs != nil {
		inst.cancelLogs()
	}
	sim.StopAndRemoveContainer(inst.containerID)
}

func postCloudRunServiceInstance(ctx context.Context, inst *cloudRunServiceInstance, body io.Reader, contentType string) ([]byte, int, error) {
	bootstrapURL := fmt.Sprintf("http://127.0.0.1:%d/", inst.hostPort)
	if err := waitForHTTP(ctx, bootstrapURL, 30*time.Second); err != nil {
		return nil, -1, fmt.Errorf("bootstrap not ready at %s: %w", bootstrapURL, err)
	}
	httpClient := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bootstrapURL, body)
	if err != nil {
		return nil, -1, fmt.Errorf("build request: %w", err)
	}
	if contentType == "" {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, -1, fmt.Errorf("invoke bootstrap: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	exitCode := 0
	if hdr := resp.Header.Get("X-Sockerless-Exit-Code"); hdr != "" {
		if n, parseErr := strconv.Atoi(hdr); parseErr == nil {
			exitCode = n
		}
	} else if resp.StatusCode >= 400 {
		exitCode = 1
	}
	return respBytes, exitCode, nil
}

func envSignature(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(env[k])
		b.WriteByte('\x00')
	}
	return b.String()
}

// crv2Services is the package-scope handle the cloudfunctions slice
// uses to auto-create the backing Cloud Run service when a Cloud
// Functions Gen2 function is created. Real GCP wires the two services
// together server-side; the sim mirrors that linkage so backends that
// expect `function.ServiceConfig.Service` to resolve to a real
// `runpb.Service` (e.g. the gcf overlay-and-swap path) work end-to-end.
var crv2Services sim.Store[ServiceV2]

// seedServiceV2Defaults stamps the immutable identity + initial-rollout
// fields onto a freshly-created service. Real Cloud Run does this
// server-side (UID, generation 1, Ready condition, default URI, first
// revision); the sim mirrors it for both REST CreateService and the
// cloudfunctions auto-wire path so a single source of truth controls
// the shape of "just-created" services.
//
// `host` is the simulator's HTTP host (Request.Host) so the URI we hand
// back routes invocations to the sim's own /v2-services-invoke handler
// rather than to the real *.run.app domain (which doesn't exist for
// fake project IDs and would 401 on TLS even if it did).
func seedServiceV2Defaults(svc ServiceV2, host, project, location, serviceID string) ServiceV2 {
	now := nowTimestamp()
	svc.Name = fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, serviceID)
	svc.UID = generateUUID()
	svc.Generation = 1
	svc.CreateTime = now
	svc.UpdateTime = now
	if svc.LaunchStage == "" {
		svc.LaunchStage = "GA"
	}
	svc.TerminalCondition = &Condition{
		Type:               "Ready",
		State:              "CONDITION_SUCCEEDED",
		LastTransitionTime: now,
	}
	svc.Conditions = []Condition{
		{Type: "Ready", State: "CONDITION_SUCCEEDED", LastTransitionTime: now},
	}
	svc.LatestReadyRevision = fmt.Sprintf("%s/revisions/%s-00001-abc", svc.Name, serviceID)
	svc.LatestCreatedRevision = svc.LatestReadyRevision
	if !svc.DefaultUriDisabled {
		svc.URI = fmt.Sprintf("http://%s/v2-services-invoke/%s/%s/%s", host, project, location, serviceID)
	}
	return svc
}

func registerCloudRunServicesV2(srv *sim.Server) {
	services := sim.MakeStore[ServiceV2](srv.DB(), "crv2_services")
	crv2Services = services

	// CreateService: POST /v2/projects/{project}/locations/{location}/services?serviceId=<id>
	srv.HandleFunc("POST /v2/projects/{project}/locations/{location}/services", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		serviceID := r.URL.Query().Get("serviceId")
		if serviceID == "" {
			sim.GCPError(w, http.StatusBadRequest, "serviceId query parameter is required", "INVALID_ARGUMENT")
			return
		}

		var svc ServiceV2
		if err := sim.ReadJSON(r, &svc); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		name := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, serviceID)
		if _, exists := services.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "service %q already exists", name)
			return
		}

		// Cloud Run regional CPU quota check. When SIM_GCP_CPU_QUOTA_PER_REGION
		// is set, each fresh revision deploy debits its CPU load against the
		// per-(project, region) sliding-window budget. Reproduces /
		// deterministically — the live cloud rejects with this same
		// error when the regional cpu_allocation quota is exhausted.
		if !regionalCPUQuotaInstance.tryDebit(project, location, serviceCPULoad(svc)) {
			regionalCPUQuotaErrorJSON(w, project, location, name)
			return
		}

		svc = seedServiceV2Defaults(svc, r.Host, project, location, serviceID)

		services.Put(name, svc)

		lro := newLRO(project, location, svc, "type.googleapis.com/google.cloud.run.v2.Service")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// GetService: GET /v2/projects/{project}/locations/{location}/services/{service}
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/services/{service}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		serviceID := sim.PathParam(r, "service")
		name := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, serviceID)
		svc, ok := services.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "service %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, svc)
	})

	// ListServices: GET /v2/projects/{project}/locations/{location}/services
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/services", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		prefix := fmt.Sprintf("projects/%s/locations/%s/services/", project, location)
		result := services.Filter(func(s ServiceV2) bool {
			return strings.HasPrefix(s.Name, prefix)
		})
		sim.WriteJSON(w, http.StatusOK, map[string]any{"services": result})
	})

	// DeleteService: DELETE /v2/projects/{project}/locations/{location}/services/{service}
	srv.HandleFunc("DELETE /v2/projects/{project}/locations/{location}/services/{service}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		serviceID := sim.PathParam(r, "service")
		name := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, serviceID)
		svc, ok := services.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "service %q not found", name)
			return
		}
		services.Delete(name)
		deleteCloudRunServiceInstance(name)
		lro := newLRO(project, location, svc, "type.googleapis.com/google.cloud.run.v2.Service")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// UpdateService is not invoked by sockerless today (the backend
	// recreates services rather than patching them). Implement it
	// anyway so terraform's `google_cloud_run_v2_service` resource
	// round-trips against the sim — every cloud-API call sockerless
	// or its declarative-driver counterparts touch must be implemented
	// at fidelity.
	srv.HandleFunc("PATCH /v2/projects/{project}/locations/{location}/services/{service}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		serviceID := sim.PathParam(r, "service")
		name := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, serviceID)

		existing, ok := services.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "service %q not found", name)
			return
		}

		var update ServiceV2
		if err := sim.ReadJSON(r, &update); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		// Cloud Run revisions are immutable, so each PATCH spawns a new
		// revision. Charge its CPU load against the regional sliding-window
		// quota — a quota-exhausted UpdateService is the failure mode
		// behind (the gcf overlay-and-swap path issues an Update
		// to flip the stub Buildpacks image to the real overlay).
		if !regionalCPUQuotaInstance.tryDebit(project, location, serviceCPULoad(update)) {
			regionalCPUQuotaErrorJSON(w, project, location, name)
			return
		}

		// Preserve identity fields; allow template / labels / annotations / ingress to change.
		update.Name = existing.Name
		update.UID = existing.UID
		update.CreateTime = existing.CreateTime
		update.Generation = existing.Generation + 1
		update.UpdateTime = nowTimestamp()
		if update.LaunchStage == "" {
			update.LaunchStage = existing.LaunchStage
		}
		update.TerminalCondition = &Condition{
			Type:               "Ready",
			State:              "CONDITION_SUCCEEDED",
			LastTransitionTime: update.UpdateTime,
		}
		revName := fmt.Sprintf("%s-%05d-abc", serviceID, update.Generation)
		update.LatestCreatedRevision = fmt.Sprintf("%s/revisions/%s", name, revName)
		update.LatestReadyRevision = update.LatestCreatedRevision
		update.URI = existing.URI

		services.Put(name, update)
		lro := newLRO(project, location, update, "type.googleapis.com/google.cloud.run.v2.Service")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// Invoke handler. Real Cloud Run hosts the service URI as
	// `https://<service>-<project>.run.app`; the sim's seedServiceV2Defaults
	// hands back `http://<sim>/v2-services-invoke/<project>/<location>/<service>`
	// instead so backends invoke the sim directly. The handler runs the
	// overlay container on demand and forwards the request envelope to
	// the bootstrap's HTTP listener — same flow as Cloud Functions Gen2
	// (`/v2-functions-invoke/`).
	srv.HandleFunc("POST /v2-services-invoke/{project}/{location}/{service}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		serviceID := sim.PathParam(r, "service")
		name := fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, serviceID)
		svc, ok := services.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "service %q not found", name)
			return
		}
		if svc.Template == nil || len(svc.Template.Containers) == 0 || svc.Template.Containers[0].Image == "" {
			sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "service %q has no container image", name)
			return
		}
		container := svc.Template.Containers[0]
		image := container.Image
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		ct := r.Header.Get("Content-Type")
		var body io.Reader
		if len(bodyBytes) > 0 {
			body = bytes.NewReader(bodyBytes)
		}
		env := containerEnvMap(container.Env)
		sink := &cfLogSink{project: project, functionName: serviceID}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()
		inst, err := ensureCloudRunServiceInstance(ctx, name, serviceID, image, env, sink)
		if err != nil {
			sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "invoke service %q: %v", name, err)
			return
		}
		respBody, exitCode, err := postCloudRunServiceInstance(ctx, inst, body, ct)
		if err != nil {
			sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "invoke service %q: %v", name, err)
			return
		}
		w.Header().Set("X-Sockerless-Exit-Code", strconv.Itoa(exitCode))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
	})
}
