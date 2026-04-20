package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Cloud Run v1 services slice. Shipped for parity completeness — the
// GCP simulator should cover every API surface sockerless's cloud
// offers on the runner path. Sockerless uses v2 Cloud Run Jobs today;
// services v1 (Knative-style) is here so `gcloud run services *` and
// `google_cloud_run_v2_service` terraform resources round-trip against
// the simulator. Real API:
//   https://cloud.google.com/run/docs/reference/rest/v1/namespaces.services

// CRService represents a Cloud Run v1 service (Knative shape).
type CRService struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   CRServiceMetadata `json:"metadata"`
	Spec       CRServiceSpec     `json:"spec"`
	Status     *CRServiceStatus  `json:"status,omitempty"`
}

type CRServiceMetadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	UID               string            `json:"uid,omitempty"`
	Generation        int64             `json:"generation,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
}

type CRServiceSpec struct {
	Template *CRServiceTemplate `json:"template,omitempty"`
	Traffic  []CRTraffic        `json:"traffic,omitempty"`
}

type CRServiceTemplate struct {
	Metadata *CRServiceMetadata `json:"metadata,omitempty"`
	Spec     *CRTemplateSpec    `json:"spec,omitempty"`
}

type CRTemplateSpec struct {
	Containers         []CRContainer `json:"containers,omitempty"`
	TimeoutSeconds     int64         `json:"timeoutSeconds,omitempty"`
	ServiceAccountName string        `json:"serviceAccountName,omitempty"`
}

type CRContainer struct {
	Name    string     `json:"name,omitempty"`
	Image   string     `json:"image"`
	Command []string   `json:"command,omitempty"`
	Args    []string   `json:"args,omitempty"`
	Env     []CREnvVar `json:"env,omitempty"`
	Ports   []CRPort   `json:"ports,omitempty"`
}

type CREnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

type CRPort struct {
	Name          string `json:"name,omitempty"`
	ContainerPort int32  `json:"containerPort"`
}

type CRTraffic struct {
	RevisionName   string `json:"revisionName,omitempty"`
	Percent        int32  `json:"percent,omitempty"`
	Tag            string `json:"tag,omitempty"`
	LatestRevision bool   `json:"latestRevision,omitempty"`
}

type CRServiceStatus struct {
	ObservedGeneration        int64         `json:"observedGeneration,omitempty"`
	LatestReadyRevisionName   string        `json:"latestReadyRevisionName,omitempty"`
	LatestCreatedRevisionName string        `json:"latestCreatedRevisionName,omitempty"`
	URL                       string        `json:"url,omitempty"`
	Address                   *CRAddress    `json:"address,omitempty"`
	Conditions                []CRCondition `json:"conditions,omitempty"`
	Traffic                   []CRTraffic   `json:"traffic,omitempty"`
}

type CRAddress struct {
	URL string `json:"url"`
}

type CRCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

// CRServiceList is the Knative list-response shape.
type CRServiceList struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Items      []CRService    `json:"items"`
}

func registerCloudRun(srv *sim.Server) {
	services := sim.MakeStore[CRService](srv.DB(), "cloudrun_services")

	svcKey := func(namespace, name string) string {
		return namespace + "/" + name
	}

	// CreateService: POST /v1/namespaces/{namespace}/services
	srv.HandleFunc("POST /v1/namespaces/{namespace}/services", func(w http.ResponseWriter, r *http.Request) {
		namespace := sim.PathParam(r, "namespace")

		var svc CRService
		if err := sim.ReadJSON(r, &svc); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid service body: %v", err)
			return
		}
		if svc.Metadata.Name == "" {
			sim.GCPError(w, http.StatusBadRequest, "metadata.name is required", "INVALID_ARGUMENT")
			return
		}
		// Namespace in URL wins over body.
		svc.Metadata.Namespace = namespace
		key := svcKey(namespace, svc.Metadata.Name)

		if _, exists := services.Get(key); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS",
				"service %q already exists in namespace %q", svc.Metadata.Name, namespace)
			return
		}

		svc.APIVersion = "serving.knative.dev/v1"
		svc.Kind = "Service"
		svc.Metadata.UID = generateUUID()
		svc.Metadata.Generation = 1
		svc.Metadata.ResourceVersion = "1"
		svc.Metadata.CreationTimestamp = time.Now().UTC().Format(time.RFC3339)

		svc.Status = &CRServiceStatus{
			ObservedGeneration:        1,
			LatestReadyRevisionName:   svc.Metadata.Name + "-00001",
			LatestCreatedRevisionName: svc.Metadata.Name + "-00001",
			URL:                       fmt.Sprintf("https://%s-%s.run.app", svc.Metadata.Name, namespace),
			Address:                   &CRAddress{URL: fmt.Sprintf("http://%s.%s.svc.cluster.local", svc.Metadata.Name, namespace)},
			Conditions: []CRCondition{{
				Type:               "Ready",
				Status:             "True",
				LastTransitionTime: time.Now().UTC().Format(time.RFC3339),
			}},
			Traffic: []CRTraffic{{
				RevisionName:   svc.Metadata.Name + "-00001",
				Percent:        100,
				LatestRevision: true,
			}},
		}

		services.Put(key, svc)
		sim.WriteJSON(w, http.StatusOK, svc)
	})

	// GetService: GET /v1/namespaces/{namespace}/services/{name}
	srv.HandleFunc("GET /v1/namespaces/{namespace}/services/{name}", func(w http.ResponseWriter, r *http.Request) {
		namespace := sim.PathParam(r, "namespace")
		name := sim.PathParam(r, "name")
		svc, ok := services.Get(svcKey(namespace, name))
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
				"service %q not found in namespace %q", name, namespace)
			return
		}
		sim.WriteJSON(w, http.StatusOK, svc)
	})

	// ListServices: GET /v1/namespaces/{namespace}/services
	srv.HandleFunc("GET /v1/namespaces/{namespace}/services", func(w http.ResponseWriter, r *http.Request) {
		namespace := sim.PathParam(r, "namespace")
		prefix := namespace + "/"
		all := services.List()
		items := make([]CRService, 0, len(all))
		for _, s := range all {
			if strings.HasPrefix(svcKey(s.Metadata.Namespace, s.Metadata.Name), prefix) {
				items = append(items, s)
			}
		}
		sim.WriteJSON(w, http.StatusOK, CRServiceList{
			APIVersion: "serving.knative.dev/v1",
			Kind:       "ServiceList",
			Items:      items,
		})
	})

	// ReplaceService: PUT /v1/namespaces/{namespace}/services/{name}
	srv.HandleFunc("PUT /v1/namespaces/{namespace}/services/{name}", func(w http.ResponseWriter, r *http.Request) {
		namespace := sim.PathParam(r, "namespace")
		name := sim.PathParam(r, "name")
		key := svcKey(namespace, name)

		existing, ok := services.Get(key)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
				"service %q not found in namespace %q", name, namespace)
			return
		}

		var update CRService
		if err := sim.ReadJSON(r, &update); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid service body: %v", err)
			return
		}
		update.APIVersion = "serving.knative.dev/v1"
		update.Kind = "Service"
		update.Metadata.Name = name
		update.Metadata.Namespace = namespace
		update.Metadata.UID = existing.Metadata.UID
		update.Metadata.Generation = existing.Metadata.Generation + 1
		update.Metadata.ResourceVersion = fmt.Sprintf("%d", update.Metadata.Generation)
		update.Metadata.CreationTimestamp = existing.Metadata.CreationTimestamp
		revName := fmt.Sprintf("%s-%05d", name, update.Metadata.Generation)
		update.Status = &CRServiceStatus{
			ObservedGeneration:        update.Metadata.Generation,
			LatestReadyRevisionName:   revName,
			LatestCreatedRevisionName: revName,
			URL:                       fmt.Sprintf("https://%s-%s.run.app", name, namespace),
			Address:                   &CRAddress{URL: fmt.Sprintf("http://%s.%s.svc.cluster.local", name, namespace)},
			Conditions: []CRCondition{{
				Type:               "Ready",
				Status:             "True",
				LastTransitionTime: time.Now().UTC().Format(time.RFC3339),
			}},
			Traffic: []CRTraffic{{
				RevisionName:   revName,
				Percent:        100,
				LatestRevision: true,
			}},
		}

		services.Put(key, update)
		sim.WriteJSON(w, http.StatusOK, update)
	})

	// DeleteService: DELETE /v1/namespaces/{namespace}/services/{name}
	srv.HandleFunc("DELETE /v1/namespaces/{namespace}/services/{name}", func(w http.ResponseWriter, r *http.Request) {
		namespace := sim.PathParam(r, "namespace")
		name := sim.PathParam(r, "name")
		if !services.Delete(svcKey(namespace, name)) {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
				"service %q not found in namespace %q", name, namespace)
			return
		}
		// Knative DELETE returns a Status object; the Go client
		// tolerates an empty 200.
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"apiVersion": "v1",
			"kind":       "Status",
			"status":     "Success",
		})
	})
}
