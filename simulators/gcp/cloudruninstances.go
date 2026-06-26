package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Cloud Run v2 Instances slice. An Instance is a single long-lived Cloud
// Run instance (the v2 resource the SDK reaches via
// run.NewInstancesRESTClient and `/v2/projects/{project}/locations/{location}/instances`).
//
// Real API: https://cloud.google.com/run/docs/reference/rest/v2/projects.locations.instances

// InstanceV2 mirrors google.cloud.run.v2.Instance (proto-JSON shape).
type InstanceV2 struct {
	Name               string            `json:"name"`
	UID                string            `json:"uid,omitempty"`
	Generation         int64             `json:"generation,string,omitempty"`
	Labels             map[string]string `json:"labels,omitempty"`
	Annotations        map[string]string `json:"annotations,omitempty"`
	Description        string            `json:"description,omitempty"`
	CreateTime         string            `json:"createTime,omitempty"`
	UpdateTime         string            `json:"updateTime,omitempty"`
	LaunchStage        enumString        `json:"launchStage,omitempty"`
	Ingress            enumString        `json:"ingress,omitempty"`
	DefaultUriDisabled bool              `json:"defaultUriDisabled,omitempty"`
	Containers         []Container       `json:"containers,omitempty"`
	Volumes            []Volume          `json:"volumes,omitempty"`
	VpcAccess          *VpcAccess        `json:"vpcAccess,omitempty"`
	ServiceAccount     string            `json:"serviceAccount,omitempty"`
	NodeSelector       *NodeSelector     `json:"nodeSelector,omitempty"`
	RestartPolicy      enumString        `json:"restartPolicy,omitempty"`
	URLs               []string          `json:"urls,omitempty"`
	TerminalCondition  *Condition        `json:"terminalCondition,omitempty"`
	Conditions         []Condition       `json:"conditions,omitempty"`
	ObservedGeneration int64             `json:"observedGeneration,string,omitempty"`
	Etag               string            `json:"etag,omitempty"`
	Reconciling        bool              `json:"reconciling,omitempty"`
}

func registerCloudRunInstancesV2(srv *sim.Server) {
	instances := sim.MakeStore[InstanceV2](srv.DB(), "crv2_instances")
	if crOperations == nil {
		crOperations = sim.MakeStore[Operation](srv.DB(), "operations")
	}

	const instType = "type.googleapis.com/google.cloud.run.v2.Instance"

	// CreateInstance: POST .../instances?instanceId=<id>
	srv.HandleFunc("POST /v2/projects/{project}/locations/{location}/instances", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		instanceID := r.URL.Query().Get("instanceId")
		if instanceID == "" {
			sim.GCPError(w, http.StatusBadRequest, "instanceId query parameter is required", "INVALID_ARGUMENT")
			return
		}
		var inst InstanceV2
		if err := sim.ReadJSON(r, &inst); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		name := fmt.Sprintf("projects/%s/locations/%s/instances/%s", project, location, instanceID)
		if _, exists := instances.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "instance %q already exists", name)
			return
		}
		inst = seedInstanceV2Defaults(inst, r.Host, project, location, instanceID)
		instances.Put(name, inst)
		lro := newLRO(project, location, inst, instType)
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// GetInstance (the {instance} wildcard also carries the GET-side
	// `{instance}:getIamPolicy` verb).
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/instances/{instance}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		instParam := sim.PathParam(r, "instance")
		if id, action, found := strings.Cut(instParam, ":"); found {
			if action == "getIamPolicy" {
				handleResourceIAM(w, r, gcpResourceIAMStore(),
					fmt.Sprintf("projects/%s/locations/%s/instances/%s", project, location, id), action)
				return
			}
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown action %q on instance %q", action, id)
			return
		}
		name := fmt.Sprintf("projects/%s/locations/%s/instances/%s", project, location, instParam)
		inst, ok := instances.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, inst)
	})

	// ListInstances
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/instances", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		prefix := fmt.Sprintf("projects/%s/locations/%s/instances/", project, location)
		result := instances.Filter(func(i InstanceV2) bool { return strings.HasPrefix(i.Name, prefix) })
		if result == nil {
			result = []InstanceV2{}
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
		page, next, ok := paginateList(w, r, result)
		if !ok {
			return
		}
		resp := map[string]any{"instances": page}
		if next != "" {
			resp["nextPageToken"] = next
		}
		sim.WriteJSON(w, http.StatusOK, resp)
	})

	// DeleteInstance
	srv.HandleFunc("DELETE /v2/projects/{project}/locations/{location}/instances/{instance}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		instanceID := sim.PathParam(r, "instance")
		name := fmt.Sprintf("projects/%s/locations/%s/instances/%s", project, location, instanceID)
		inst, ok := instances.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance %q not found", name)
			return
		}
		instances.Delete(name)
		lro := newLRO(project, location, inst, instType)
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// StartInstance / StopInstance / setIamPolicy / testIamPermissions all
	// arrive as POST .../instances/{instance}:<verb>.
	srv.HandleFunc("POST /v2/projects/{project}/locations/{location}/instances/{instanceAction}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		instanceAction := sim.PathParam(r, "instanceAction")
		id, action, found := strings.Cut(instanceAction, ":")
		if !found {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown action on instance %q", instanceAction)
			return
		}
		switch action {
		case "setIamPolicy", "testIamPermissions":
			handleResourceIAM(w, r, gcpResourceIAMStore(),
				fmt.Sprintf("projects/%s/locations/%s/instances/%s", project, location, id), action)
			return
		case "start", "stop":
			name := fmt.Sprintf("projects/%s/locations/%s/instances/%s", project, location, id)
			if _, ok := instances.Get(name); !ok {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance %q not found", name)
				return
			}
			now := nowTimestamp()
			state := enumString("CONDITION_SUCCEEDED")
			reason := ""
			if action == "stop" {
				state = "CONDITION_PENDING"
				reason = "Stopped"
			}
			instances.Update(name, func(i *InstanceV2) {
				i.UpdateTime = now
				i.TerminalCondition = &Condition{Type: "Ready", State: state, LastTransitionTime: now, Reason: reason}
			})
			inst, _ := instances.Get(name)
			lro := newLRO(project, location, inst, instType)
			sim.WriteJSON(w, http.StatusOK, lro)
		default:
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown action %q on instance %q", action, id)
		}
	})
}

func seedInstanceV2Defaults(inst InstanceV2, host, project, location, instanceID string) InstanceV2 {
	now := nowTimestamp()
	inst.Name = fmt.Sprintf("projects/%s/locations/%s/instances/%s", project, location, instanceID)
	inst.UID = generateUUID()
	inst.Generation = 1
	inst.ObservedGeneration = 1
	inst.CreateTime = now
	inst.UpdateTime = now
	if inst.LaunchStage == "" {
		inst.LaunchStage = "GA"
	}
	inst.TerminalCondition = &Condition{
		Type:               "Ready",
		State:              "CONDITION_SUCCEEDED",
		LastTransitionTime: now,
	}
	inst.Conditions = []Condition{
		{Type: "Ready", State: "CONDITION_SUCCEEDED", LastTransitionTime: now},
	}
	if !inst.DefaultUriDisabled {
		inst.URLs = []string{fmt.Sprintf("http://%s/v2-services-invoke/%s/%s/%s", host, project, location, instanceID)}
	}
	return inst
}
