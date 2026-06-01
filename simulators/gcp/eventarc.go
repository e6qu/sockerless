package main

import (
	"fmt"
	"net/http"
	"strings"

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

type EventarcChannel struct {
	Name            string            `json:"name"`
	Uid             string            `json:"uid,omitempty"`
	CreateTime      string            `json:"createTime,omitempty"`
	UpdateTime      string            `json:"updateTime,omitempty"`
	Provider        string            `json:"provider,omitempty"`
	PubsubTopic     string            `json:"pubsubTopic,omitempty"`
	State           string            `json:"state,omitempty"`
	ActivationToken string            `json:"activationToken,omitempty"`
	CryptoKeyName   string            `json:"cryptoKeyName,omitempty"`
	SatisfiesPzs    bool              `json:"satisfiesPzs,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type EventarcChannelConnection struct {
	Name            string            `json:"name"`
	Uid             string            `json:"uid,omitempty"`
	Channel         string            `json:"channel,omitempty"`
	CreateTime      string            `json:"createTime,omitempty"`
	UpdateTime      string            `json:"updateTime,omitempty"`
	ActivationToken string            `json:"activationToken,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type EventarcProvider struct {
	Name        string                  `json:"name"`
	DisplayName string                  `json:"displayName,omitempty"`
	EventTypes  []EventarcProviderEvent `json:"eventTypes,omitempty"`
}

type EventarcProviderEvent struct {
	Type                string                       `json:"type"`
	Description         string                       `json:"description,omitempty"`
	FilteringAttributes []EventarcFilteringAttribute `json:"filteringAttributes,omitempty"`
	EventSchemaURI      string                       `json:"eventSchemaUri,omitempty"`
}

type EventarcFilteringAttribute struct {
	Attribute string `json:"attribute"`
	Required  bool   `json:"required,omitempty"`
}

var (
	eventarcTriggers           sim.Store[EventarcTrigger]
	eventarcChannels           sim.Store[EventarcChannel]
	eventarcChannelConnections sim.Store[EventarcChannelConnection]
)

func registerEventarc(srv *sim.Server) {
	eventarcTriggers = sim.MakeStore[EventarcTrigger](srv.DB(), "eventarc_triggers")
	eventarcChannels = sim.MakeStore[EventarcChannel](srv.DB(), "eventarc_channels")
	eventarcChannelConnections = sim.MakeStore[EventarcChannelConnection](srv.DB(), "eventarc_channel_connections")

	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/triggers", handleGCPRegionalTriggerCreate)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/triggers", handleGCPRegionalTriggerList)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/triggers/{trigger}", handleGCPRegionalTriggerGet)
	srv.HandleFunc("PATCH /v1/projects/{project}/locations/{location}/triggers/{trigger}", handleGCPRegionalTriggerPatch)
	srv.HandleFunc("DELETE /v1/projects/{project}/locations/{location}/triggers/{trigger}", handleGCPRegionalTriggerDelete)
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/channels", handleEventarcCreateChannel)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/channels", handleEventarcListChannels)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/channels/{channel}", handleEventarcGetChannel)
	srv.HandleFunc("PATCH /v1/projects/{project}/locations/{location}/channels/{channel}", handleEventarcPatchChannel)
	srv.HandleFunc("DELETE /v1/projects/{project}/locations/{location}/channels/{channel}", handleEventarcDeleteChannel)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/providers", handleEventarcListProviders)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/providers/{provider}", handleEventarcGetProvider)
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/channelConnections", handleEventarcCreateChannelConnection)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/channelConnections", handleEventarcListChannelConnections)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/channelConnections/{connection}", handleEventarcGetChannelConnection)
	srv.HandleFunc("DELETE /v1/projects/{project}/locations/{location}/channelConnections/{connection}", handleEventarcDeleteChannelConnection)
}

func isCloudBuildRequest(r *http.Request) bool {
	if strings.Contains(strings.ToLower(r.Host), "cloudbuild") {
		return true
	}
	if r.Method == http.MethodPost && r.URL.Query().Get("triggerId") == "" {
		return true
	}
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	if location == "global" {
		return true
	}
	trigger := sim.PathParam(r, "trigger")
	if project == "" || location == "" || trigger == "" {
		return false
	}
	_, ok := cbTriggers.Get(buildTriggerKey(project, location, trigger))
	return ok
}

func handleGCPRegionalTriggerCreate(w http.ResponseWriter, r *http.Request) {
	if isCloudBuildRequest(r) {
		handleCreateBuildTrigger(w, r)
		return
	}
	handleEventarcCreateTrigger(w, r)
}

func handleGCPRegionalTriggerList(w http.ResponseWriter, r *http.Request) {
	if isCloudBuildRequest(r) {
		handleListBuildTriggers(w, r)
		return
	}
	handleEventarcListTriggers(w, r)
}

func handleGCPRegionalTriggerGet(w http.ResponseWriter, r *http.Request) {
	if isCloudBuildRequest(r) {
		handleGetBuildTrigger(w, r)
		return
	}
	handleEventarcGetTrigger(w, r)
}

func handleGCPRegionalTriggerPatch(w http.ResponseWriter, r *http.Request) {
	if isCloudBuildRequest(r) {
		handleUpdateBuildTrigger(w, r)
		return
	}
	handleEventarcPatchTrigger(w, r)
}

func handleGCPRegionalTriggerDelete(w http.ResponseWriter, r *http.Request) {
	if isCloudBuildRequest(r) {
		handleDeleteBuildTrigger(w, r)
		return
	}
	handleEventarcDeleteTrigger(w, r)
}

func eventarcTriggerName(project, location, trigger string) string {
	return fmt.Sprintf("projects/%s/locations/%s/triggers/%s", project, location, trigger)
}

func eventarcTriggerKey(project, location, trigger string) string {
	return project + "/" + location + "/" + trigger
}

func eventarcChannelName(project, location, channel string) string {
	return fmt.Sprintf("projects/%s/locations/%s/channels/%s", project, location, channel)
}

func eventarcChannelKey(project, location, channel string) string {
	return project + "/" + location + "/" + channel
}

func eventarcChannelConnectionName(project, location, connection string) string {
	return fmt.Sprintf("projects/%s/locations/%s/channelConnections/%s", project, location, connection)
}

func eventarcChannelConnectionKey(project, location, connection string) string {
	return project + "/" + location + "/" + connection
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
	now := nowTimestamp()
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
	existing.UpdateTime = nowTimestamp()
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

func handleEventarcCreateChannel(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	channelID := r.URL.Query().Get("channelId")
	if channelID == "" {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "channelId is required")
		return
	}
	var req EventarcChannel
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "bad request body: %v", err)
		return
	}
	now := nowTimestamp()
	req.Name = eventarcChannelName(project, location, channelID)
	req.Uid = generateUUID()
	req.CreateTime = now
	req.UpdateTime = now
	req.State = "ACTIVE"
	req.ActivationToken = generateUUID()
	eventarcChannels.Put(eventarcChannelKey(project, location, channelID), req)
	op := newLRO(project, location, req, "type.googleapis.com/google.cloud.eventarc.v1.Channel")
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleEventarcGetChannel(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	channel := sim.PathParam(r, "channel")
	c, ok := eventarcChannels.Get(eventarcChannelKey(project, location, channel))
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "channel %q not found", channel)
		return
	}
	sim.WriteJSON(w, http.StatusOK, c)
}

func handleEventarcListChannels(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	prefix := fmt.Sprintf("projects/%s/locations/%s/channels/", project, location)
	out := make([]EventarcChannel, 0)
	for _, c := range eventarcChannels.List() {
		if strings.HasPrefix(c.Name, prefix) {
			out = append(out, c)
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"channels": out})
}

func handleEventarcPatchChannel(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	channel := sim.PathParam(r, "channel")
	key := eventarcChannelKey(project, location, channel)
	existing, ok := eventarcChannels.Get(key)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "channel %q not found", channel)
		return
	}
	var req EventarcChannel
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "bad request body: %v", err)
		return
	}
	if req.Provider != "" {
		existing.Provider = req.Provider
	}
	if req.PubsubTopic != "" {
		existing.PubsubTopic = req.PubsubTopic
	}
	if req.CryptoKeyName != "" {
		existing.CryptoKeyName = req.CryptoKeyName
	}
	if req.Labels != nil {
		existing.Labels = req.Labels
	}
	existing.UpdateTime = nowTimestamp()
	eventarcChannels.Put(key, existing)
	op := newLRO(project, location, existing, "type.googleapis.com/google.cloud.eventarc.v1.Channel")
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleEventarcDeleteChannel(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	channel := sim.PathParam(r, "channel")
	key := eventarcChannelKey(project, location, channel)
	c, ok := eventarcChannels.Get(key)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "channel %q not found", channel)
		return
	}
	eventarcChannels.Delete(key)
	op := newLRO(project, location, c, "type.googleapis.com/google.cloud.eventarc.v1.Channel")
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleEventarcListProviders(w http.ResponseWriter, r *http.Request) {
	parent := fmt.Sprintf("projects/%s/locations/%s", sim.PathParam(r, "project"), sim.PathParam(r, "location"))
	sim.WriteJSON(w, http.StatusOK, map[string]any{"providers": eventarcProviders(parent)})
}

func handleEventarcGetProvider(w http.ResponseWriter, r *http.Request) {
	parent := fmt.Sprintf("projects/%s/locations/%s", sim.PathParam(r, "project"), sim.PathParam(r, "location"))
	name := parent + "/providers/" + sim.PathParam(r, "provider")
	for _, provider := range eventarcProviders(parent) {
		if provider.Name == name {
			sim.WriteJSON(w, http.StatusOK, provider)
			return
		}
	}
	sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "provider %q not found", name)
}

func eventarcProviders(parent string) []EventarcProvider {
	return []EventarcProvider{
		{
			Name:        parent + "/providers/cloud.pubsub",
			DisplayName: "Cloud Pub/Sub",
			EventTypes: []EventarcProviderEvent{{
				Type:        "google.cloud.pubsub.topic.v1.messagePublished",
				Description: "A Pub/Sub message was published.",
				FilteringAttributes: []EventarcFilteringAttribute{{
					Attribute: "type",
					Required:  true,
				}},
			}},
		},
		{
			Name:        parent + "/providers/cloud.storage",
			DisplayName: "Cloud Storage",
			EventTypes: []EventarcProviderEvent{{
				Type:        "google.cloud.storage.object.v1.finalized",
				Description: "A Cloud Storage object was finalized.",
				FilteringAttributes: []EventarcFilteringAttribute{{
					Attribute: "type",
					Required:  true,
				}},
			}},
		},
	}
}

func handleEventarcCreateChannelConnection(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	connectionID := r.URL.Query().Get("channelConnectionId")
	if connectionID == "" {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "channelConnectionId is required")
		return
	}
	var req EventarcChannelConnection
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "bad request body: %v", err)
		return
	}
	now := nowTimestamp()
	req.Name = eventarcChannelConnectionName(project, location, connectionID)
	req.Uid = generateUUID()
	req.CreateTime = now
	req.UpdateTime = now
	eventarcChannelConnections.Put(eventarcChannelConnectionKey(project, location, connectionID), req)
	op := newLRO(project, location, req, "type.googleapis.com/google.cloud.eventarc.v1.ChannelConnection")
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleEventarcGetChannelConnection(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	connection := sim.PathParam(r, "connection")
	cc, ok := eventarcChannelConnections.Get(eventarcChannelConnectionKey(project, location, connection))
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "channel connection %q not found", connection)
		return
	}
	sim.WriteJSON(w, http.StatusOK, cc)
}

func handleEventarcListChannelConnections(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	prefix := fmt.Sprintf("projects/%s/locations/%s/channelConnections/", project, location)
	out := make([]EventarcChannelConnection, 0)
	for _, cc := range eventarcChannelConnections.List() {
		if strings.HasPrefix(cc.Name, prefix) {
			out = append(out, cc)
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"channelConnections": out})
}

func handleEventarcDeleteChannelConnection(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	connection := sim.PathParam(r, "connection")
	key := eventarcChannelConnectionKey(project, location, connection)
	cc, ok := eventarcChannelConnections.Get(key)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "channel connection %q not found", connection)
		return
	}
	eventarcChannelConnections.Delete(key)
	op := newLRO(project, location, cc, "type.googleapis.com/google.cloud.eventarc.v1.ChannelConnection")
	sim.WriteJSON(w, http.StatusOK, op)
}
