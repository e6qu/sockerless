package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Eventarc v1 REST surface. Triggers are regional resources and
// mutating operations return AIP-151 long-running operations.

type EventarcTrigger struct {
	Name                 string                 `json:"name"`
	Uid                  string                 `json:"uid,omitempty"`
	CreateTime           string                 `json:"createTime,omitempty"`
	UpdateTime           string                 `json:"updateTime,omitempty"`
	Labels               map[string]string      `json:"labels,omitempty"`
	EventFilters         []EventarcEventFilter  `json:"eventFilters,omitempty"`
	Destination          map[string]any         `json:"destination,omitempty"`
	Transport            map[string]any         `json:"transport,omitempty"`
	ServiceAccount       string                 `json:"serviceAccount,omitempty"`
	EventDataContentType string                 `json:"eventDataContentType,omitempty"`
	Conditions           map[string]any         `json:"conditions,omitempty"`
	Extra                map[string]interface{} `json:"-"`
}

type EventarcEventFilter struct {
	Attribute string `json:"attribute"`
	Value     string `json:"value"`
	Operator  string `json:"operator,omitempty"`
}

var eventarcTriggers sim.Store[EventarcTrigger]

func registerEventarc(srv *sim.Server) {
	eventarcTriggers = sim.MakeStore[EventarcTrigger](srv.DB(), "eventarc_triggers")

	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/triggers", handleEventarcCreateTrigger)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/triggers", handleEventarcListTriggers)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/triggers/{trigger}", handleEventarcGetTrigger)
	srv.HandleFunc("PATCH /v1/projects/{project}/locations/{location}/triggers/{trigger}", handleEventarcPatchTrigger)
	srv.HandleFunc("DELETE /v1/projects/{project}/locations/{location}/triggers/{trigger}", handleEventarcDeleteTrigger)
}

func eventarcTriggerName(project, location, trigger string) string {
	return fmt.Sprintf("projects/%s/locations/%s/triggers/%s", project, location, trigger)
}

func eventarcTriggerKey(project, location, trigger string) string {
	return project + "/" + location + "/" + trigger
}

func handleEventarcCreateTrigger(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	triggerID := r.URL.Query().Get("triggerId")
	if triggerID == "" {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "triggerId is required")
		return
	}
	var req EventarcTrigger
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "bad request body: %v", err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	req.Name = eventarcTriggerName(project, location, triggerID)
	req.Uid = generateUUID()
	req.CreateTime = now
	req.UpdateTime = now
	eventarcTriggers.Put(eventarcTriggerKey(project, location, triggerID), req)
	op := newLRO(project, location, req, "type.googleapis.com/google.cloud.eventarc.v1.Trigger")
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleEventarcGetTrigger(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	trigger := sim.PathParam(r, "trigger")
	t, ok := eventarcTriggers.Get(eventarcTriggerKey(project, location, trigger))
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "trigger %q not found", trigger)
		return
	}
	sim.WriteJSON(w, http.StatusOK, t)
}

func handleEventarcListTriggers(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	prefix := fmt.Sprintf("projects/%s/locations/%s/triggers/", project, location)
	out := make([]EventarcTrigger, 0)
	for _, t := range eventarcTriggers.List() {
		if strings.HasPrefix(t.Name, prefix) {
			out = append(out, t)
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"triggers": out})
}

func handleEventarcPatchTrigger(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	trigger := sim.PathParam(r, "trigger")
	key := eventarcTriggerKey(project, location, trigger)
	existing, ok := eventarcTriggers.Get(key)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "trigger %q not found", trigger)
		return
	}
	var req EventarcTrigger
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "bad request body: %v", err)
		return
	}
	if req.Labels != nil {
		existing.Labels = req.Labels
	}
	if req.EventFilters != nil {
		existing.EventFilters = req.EventFilters
	}
	if req.Destination != nil {
		existing.Destination = req.Destination
	}
	if req.Transport != nil {
		existing.Transport = req.Transport
	}
	if req.ServiceAccount != "" {
		existing.ServiceAccount = req.ServiceAccount
	}
	if req.EventDataContentType != "" {
		existing.EventDataContentType = req.EventDataContentType
	}
	existing.UpdateTime = time.Now().UTC().Format(time.RFC3339Nano)
	eventarcTriggers.Put(key, existing)
	op := newLRO(project, location, existing, "type.googleapis.com/google.cloud.eventarc.v1.Trigger")
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleEventarcDeleteTrigger(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	trigger := sim.PathParam(r, "trigger")
	key := eventarcTriggerKey(project, location, trigger)
	t, ok := eventarcTriggers.Get(key)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "trigger %q not found", trigger)
		return
	}
	eventarcTriggers.Delete(key)
	op := newLRO(project, location, t, "type.googleapis.com/google.cloud.eventarc.v1.Trigger")
	sim.WriteJSON(w, http.StatusOK, op)
}
