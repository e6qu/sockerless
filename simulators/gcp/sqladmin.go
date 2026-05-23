package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Cloud SQL Admin v1 — REST surface. Real API path prefix is
// `/sql/v1beta4/...` in some clients and `/v1/...` in others;
// the Go `sqladmin/v1` client used by terraform-provider-google
// hits `/v1/projects/{p}/instances...`. Surface scoped to the
// 90th-percentile instance + database + user lifecycle. The
// database engine itself is not simulated; State=RUNNABLE
// immediately on insert.

type SQLInstance struct {
	Name            string           `json:"name"`
	Project         string           `json:"project,omitempty"`
	Region          string           `json:"region,omitempty"`
	DatabaseVersion string           `json:"databaseVersion,omitempty"`
	State           string           `json:"state,omitempty"`
	BackendType     string           `json:"backendType,omitempty"`
	InstanceType    string           `json:"instanceType,omitempty"`
	ConnectionName  string           `json:"connectionName,omitempty"`
	GceZone         string           `json:"gceZone,omitempty"`
	CreateTime      string           `json:"createTime,omitempty"`
	Settings        map[string]any   `json:"settings,omitempty"`
	IpAddresses     []map[string]any `json:"ipAddresses,omitempty"`
	SelfLink        string           `json:"selfLink,omitempty"`
}

type SQLDatabase struct {
	Name     string `json:"name"`
	Instance string `json:"instance,omitempty"`
	Project  string `json:"project,omitempty"`
	Charset  string `json:"charset,omitempty"`
	SelfLink string `json:"selfLink,omitempty"`
}

type SQLUser struct {
	Name     string `json:"name"`
	Instance string `json:"instance,omitempty"`
	Project  string `json:"project,omitempty"`
	Host     string `json:"host,omitempty"`
	Type     string `json:"type,omitempty"`
}

// SQLOperation mirrors the v1 sqladmin Operation envelope, which
// differs from the cloud.google.com/operations.v1 envelope used by
// other GCP services (Memorystore / APIGW use the latter). The sim
// emits a simplified done-immediately version.
type SQLOperation struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	OperationType string `json:"operationType,omitempty"`
	Status        string `json:"status"`
	TargetProject string `json:"targetProject,omitempty"`
	TargetID      string `json:"targetId,omitempty"`
	InsertTime    string `json:"insertTime,omitempty"`
	EndTime       string `json:"endTime,omitempty"`
	SelfLink      string `json:"selfLink,omitempty"`
}

var (
	sqlInstances sim.Store[SQLInstance]
	sqlDatabases sim.Store[SQLDatabase]
	sqlUsers     sim.Store[SQLUser]
)

func registerCloudSQL(srv *sim.Server) {
	sqlInstances = sim.MakeStore[SQLInstance](srv.DB(), "sql_instances")
	sqlDatabases = sim.MakeStore[SQLDatabase](srv.DB(), "sql_databases")
	sqlUsers = sim.MakeStore[SQLUser](srv.DB(), "sql_users")

	srv.HandleFunc("POST /v1/projects/{project}/instances", handleSQLInsertInstance)
	srv.HandleFunc("GET /v1/projects/{project}/instances/{instance}", handleSQLGetInstance)
	srv.HandleFunc("GET /v1/projects/{project}/instances", handleSQLListInstances)
	srv.HandleFunc("PATCH /v1/projects/{project}/instances/{instance}", handleSQLPatchInstance)
	srv.HandleFunc("DELETE /v1/projects/{project}/instances/{instance}", handleSQLDeleteInstance)

	srv.HandleFunc("POST /v1/projects/{project}/instances/{instance}/databases", handleSQLInsertDatabase)
	srv.HandleFunc("GET /v1/projects/{project}/instances/{instance}/databases", handleSQLListDatabases)
	srv.HandleFunc("DELETE /v1/projects/{project}/instances/{instance}/databases/{database}", handleSQLDeleteDatabase)

	srv.HandleFunc("POST /v1/projects/{project}/instances/{instance}/users", handleSQLInsertUser)
	srv.HandleFunc("GET /v1/projects/{project}/instances/{instance}/users", handleSQLListUsers)
}

func sqlInstanceKey(project, instance string) string {
	return fmt.Sprintf("%s/%s", project, instance)
}

// gcpSelfLink builds a fully-qualified selfLink rooted at the
// scheme+host the request arrived on. Real GCP emits absolute URLs
// (`https://<service>.googleapis.com/v1/...`); clients pass selfLink
// around as an opaque pointer and follow it later, which only works
// when the URL is absolute. The sim previously emitted relative
// URLs (`/v1/projects/...`) which broke audit / sync / cross-resource
// reference patterns. Use this helper for every selfLink emission.
func gcpSelfLink(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if r.URL.Scheme != "" {
		scheme = r.URL.Scheme
	}
	host := r.Host
	if host == "" {
		host = "sqladmin.googleapis.com"
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}

func newSQLOperation(project, opType, targetID string) SQLOperation {
	now := nowTimestamp()
	return SQLOperation{
		Kind:          "sql#operation",
		Name:          generateUUID(),
		OperationType: opType,
		Status:        "DONE",
		TargetProject: project,
		TargetID:      targetID,
		InsertTime:    now,
		EndTime:       now,
	}
}

func handleSQLInsertInstance(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	var req SQLInstance
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%s", err.Error())
		return
	}
	if req.Name == "" {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "name is required")
		return
	}
	inst := SQLInstance{
		Name:            req.Name,
		Project:         project,
		Region:          defaultStr(req.Region, "us-central1"),
		DatabaseVersion: defaultStr(req.DatabaseVersion, "POSTGRES_15"),
		State:           "RUNNABLE",
		BackendType:     "SECOND_GEN",
		InstanceType:    "CLOUD_SQL_INSTANCE",
		ConnectionName:  fmt.Sprintf("%s:%s:%s", project, defaultStr(req.Region, "us-central1"), req.Name),
		CreateTime:      nowTimestamp(),
		Settings:        req.Settings,
		IpAddresses: []map[string]any{
			{"type": "PRIMARY", "ipAddress": "10.0.0.1"},
		},
		SelfLink: gcpSelfLink(r, fmt.Sprintf("/v1/projects/%s/instances/%s", project, req.Name)),
	}
	sqlInstances.Put(sqlInstanceKey(project, req.Name), inst)
	op := newSQLOperation(project, "CREATE", req.Name)
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleSQLGetInstance(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	name := sim.PathParam(r, "instance")
	inst, ok := sqlInstances.Get(sqlInstanceKey(project, name))
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance not found: %s", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, inst)
}

func handleSQLListInstances(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	var out []SQLInstance
	for _, i := range sqlInstances.List() {
		if i.Project == project {
			out = append(out, i)
		}
	}
	if out == nil {
		out = []SQLInstance{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"kind": "sql#instancesList", "items": out})
}

func handleSQLPatchInstance(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	name := sim.PathParam(r, "instance")
	key := sqlInstanceKey(project, name)
	if _, ok := sqlInstances.Get(key); !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance not found: %s", name)
		return
	}
	var req SQLInstance
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%s", err.Error())
		return
	}
	sqlInstances.Update(key, func(i *SQLInstance) {
		if req.DatabaseVersion != "" {
			i.DatabaseVersion = req.DatabaseVersion
		}
		if req.Settings != nil {
			i.Settings = req.Settings
		}
	})
	op := newSQLOperation(project, "UPDATE", name)
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleSQLDeleteInstance(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	name := sim.PathParam(r, "instance")
	if !sqlInstances.Delete(sqlInstanceKey(project, name)) {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance not found: %s", name)
		return
	}
	// Cascade-clear databases + users.
	prefix := fmt.Sprintf("%s/%s/", project, name)
	for _, d := range sqlDatabases.List() {
		if strings.HasPrefix(d.Instance, name) && d.Project == project {
			sqlDatabases.Delete(prefix + d.Name)
		}
	}
	for _, u := range sqlUsers.List() {
		if u.Instance == name && u.Project == project {
			sqlUsers.Delete(prefix + u.Name)
		}
	}
	op := newSQLOperation(project, "DELETE", name)
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleSQLInsertDatabase(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	instance := sim.PathParam(r, "instance")
	if _, ok := sqlInstances.Get(sqlInstanceKey(project, instance)); !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance not found: %s", instance)
		return
	}
	var req SQLDatabase
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%s", err.Error())
		return
	}
	if req.Name == "" {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "name is required")
		return
	}
	db := SQLDatabase{
		Name:     req.Name,
		Instance: instance,
		Project:  project,
		Charset:  defaultStr(req.Charset, "UTF8"),
		SelfLink: gcpSelfLink(r, fmt.Sprintf("/v1/projects/%s/instances/%s/databases/%s", project, instance, req.Name)),
	}
	sqlDatabases.Put(fmt.Sprintf("%s/%s/%s", project, instance, req.Name), db)
	op := newSQLOperation(project, "CREATE_DATABASE", req.Name)
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleSQLListDatabases(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	instance := sim.PathParam(r, "instance")
	var out []SQLDatabase
	for _, d := range sqlDatabases.List() {
		if d.Project == project && d.Instance == instance {
			out = append(out, d)
		}
	}
	if out == nil {
		out = []SQLDatabase{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"kind": "sql#databasesList", "items": out})
}

func handleSQLDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	instance := sim.PathParam(r, "instance")
	name := sim.PathParam(r, "database")
	key := fmt.Sprintf("%s/%s/%s", project, instance, name)
	if !sqlDatabases.Delete(key) {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "database not found: %s", name)
		return
	}
	op := newSQLOperation(project, "DELETE_DATABASE", name)
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleSQLInsertUser(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	instance := sim.PathParam(r, "instance")
	if _, ok := sqlInstances.Get(sqlInstanceKey(project, instance)); !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "instance not found: %s", instance)
		return
	}
	var req SQLUser
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%s", err.Error())
		return
	}
	if req.Name == "" {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "name is required")
		return
	}
	u := SQLUser{
		Name:     req.Name,
		Instance: instance,
		Project:  project,
		Host:     req.Host,
		Type:     defaultStr(req.Type, "BUILT_IN"),
	}
	sqlUsers.Put(fmt.Sprintf("%s/%s/%s", project, instance, req.Name), u)
	op := newSQLOperation(project, "CREATE_USER", req.Name)
	sim.WriteJSON(w, http.StatusOK, op)
}

func handleSQLListUsers(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	instance := sim.PathParam(r, "instance")
	var out []SQLUser
	for _, u := range sqlUsers.List() {
		if u.Project == project && u.Instance == instance {
			out = append(out, u)
		}
	}
	if out == nil {
		out = []SQLUser{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"kind": "sql#usersList", "items": out})
}
