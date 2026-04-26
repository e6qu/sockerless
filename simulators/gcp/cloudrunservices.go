package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Cloud Run v2 services slice. The cloudrun backend uses
// cloud.google.com/go/run/apiv2 (REST client) which talks to the v2
// REST surface — `/v2/projects/{project}/locations/{location}/services`
// — not the v1 Knative paths handled in cloudrun.go. When
// Config.UseService=true the backend hits these endpoints; without
// them every Service call 404s, which is why Phase 108's GCP audit
// flagged BUG-833.
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
	LaunchStage           string            `json:"launchStage,omitempty"`
	Ingress               string            `json:"ingress,omitempty"`
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
	Connector string `json:"connector,omitempty"`
	Egress    string `json:"egress,omitempty"`
}

// TrafficTarget is one entry in the Service's traffic-split list.
type TrafficTarget struct {
	Type     string `json:"type,omitempty"`
	Revision string `json:"revision,omitempty"`
	Percent  int32  `json:"percent,omitempty"`
	Tag      string `json:"tag,omitempty"`
}

func registerCloudRunServicesV2(srv *sim.Server) {
	services := sim.MakeStore[ServiceV2](srv.DB(), "crv2_services")

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

		now := nowTimestamp()
		svc.Name = name
		svc.UID = generateUUID()
		svc.Generation = 1
		svc.CreateTime = now
		svc.UpdateTime = now
		if svc.LaunchStage == "" {
			svc.LaunchStage = "GA"
		}
		// Services come up Ready immediately in the sim — there's no
		// rollout window, so backend code that calls op.Wait() reads
		// back a fully-Ready Service the first time.
		revName := fmt.Sprintf("%s-00001-abc", serviceID)
		svc.TerminalCondition = &Condition{
			Type:               "Ready",
			State:              "CONDITION_SUCCEEDED",
			LastTransitionTime: now,
		}
		svc.Conditions = []Condition{
			{Type: "Ready", State: "CONDITION_SUCCEEDED", LastTransitionTime: now},
		}
		svc.LatestReadyRevision = fmt.Sprintf("%s/revisions/%s", name, revName)
		svc.LatestCreatedRevision = svc.LatestReadyRevision
		if !svc.DefaultUriDisabled {
			svc.URI = fmt.Sprintf("https://%s-%s.run.app", serviceID, project)
		}

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
		lro := newLRO(project, location, svc, "type.googleapis.com/google.cloud.run.v2.Service")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// UpdateService is not invoked by sockerless today (the backend
	// recreates services rather than patching them). Implement it
	// anyway so terraform's `google_cloud_run_v2_service` resource
	// round-trips against the sim — Phase 108's no-defer rule says
	// every cloud-API call sockerless or its declarative-driver
	// counterparts touch must be implemented at fidelity.
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
}
