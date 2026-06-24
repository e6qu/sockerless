package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sim "github.com/sockerless/simulator"
)

// AWS Glue — AWS JSON 1.1 protocol (X-Amz-Target: AWSGlue.<Op>).
// Python shell job runs execute the script stored at the job's S3 script location.

type GlueDatabase struct {
	Name        string            `json:"Name"`
	Parameters  map[string]string `json:"Parameters,omitempty"`
	CreateTime  float64           `json:"CreateTime"`
	LocationUri string            `json:"LocationUri,omitempty"`
	Description string            `json:"Description,omitempty"`
	Tags        map[string]string `json:"Tags,omitempty"`
}

type GlueTable struct {
	Name              string            `json:"Name"`
	DatabaseName      string            `json:"DatabaseName"`
	StorageDescriptor map[string]any    `json:"StorageDescriptor,omitempty"`
	PartitionKeys     []map[string]any  `json:"PartitionKeys,omitempty"`
	Parameters        map[string]string `json:"Parameters,omitempty"`
	CreateTime        float64           `json:"CreateTime"`
	UpdateTime        float64           `json:"UpdateTime"`
	TableType         string            `json:"TableType,omitempty"`
}

type GlueJob struct {
	Name             string            `json:"Name"`
	Description      string            `json:"Description,omitempty"`
	Role             string            `json:"Role"`
	Command          map[string]any    `json:"Command"`
	DefaultArguments map[string]string `json:"DefaultArguments,omitempty"`
	GlueVersion      string            `json:"GlueVersion,omitempty"`
	MaxCapacity      *float64          `json:"MaxCapacity,omitempty"`
	WorkerType       string            `json:"WorkerType,omitempty"`
	NumberOfWorkers  *int              `json:"NumberOfWorkers,omitempty"`
	CreatedOn        float64           `json:"CreatedOn"`
	LastModifiedOn   float64           `json:"LastModifiedOn"`
	Tags             map[string]string `json:"Tags,omitempty"`
}

type GlueJobRun struct {
	ID            string            `json:"Id"`
	JobName       string            `json:"JobName"`
	JobRunState   string            `json:"JobRunState"`
	StartedOn     float64           `json:"StartedOn"`
	CompletedOn   float64           `json:"CompletedOn"`
	ExecutionTime int               `json:"ExecutionTime"`
	Arguments     map[string]string `json:"Arguments,omitempty"`
	ErrorMessage  string            `json:"ErrorMessage,omitempty"`
}

// GluePartition models a Data Catalog partition belonging to a table.
type GluePartition struct {
	Values            []string          `json:"Values,omitempty"`
	DatabaseName      string            `json:"DatabaseName"`
	TableName         string            `json:"TableName"`
	StorageDescriptor map[string]any    `json:"StorageDescriptor,omitempty"`
	Parameters        map[string]string `json:"Parameters,omitempty"`
	CreationTime      float64           `json:"CreationTime"`
	LastAccessTime    *float64          `json:"LastAccessTime,omitempty"`
	LastAnalyzedTime  *float64          `json:"LastAnalyzedTime,omitempty"`
}

var (
	glueDatabases  sim.Store[GlueDatabase]
	glueTables     sim.Store[GlueTable]
	gluePartitions sim.Store[GluePartition]
	glueJobs       sim.Store[GlueJob]
	glueJobRuns    sim.Store[GlueJobRun]
	glueMu         sync.Mutex
)

func registerGlue(r *sim.AWSRouter, srv *sim.Server) {
	glueDatabases = sim.MakeStore[GlueDatabase](srv.DB(), "glue_databases")
	glueTables = sim.MakeStore[GlueTable](srv.DB(), "glue_tables")
	gluePartitions = sim.MakeStore[GluePartition](srv.DB(), "glue_partitions")
	glueJobs = sim.MakeStore[GlueJob](srv.DB(), "glue_jobs")
	glueJobRuns = sim.MakeStore[GlueJobRun](srv.DB(), "glue_job_runs")

	r.Register("AWSGlue.CreateDatabase", handleGlueCreateDatabase)
	r.Register("AWSGlue.GetDatabase", handleGlueGetDatabase)
	r.Register("AWSGlue.GetDatabases", handleGlueGetDatabases)
	r.Register("AWSGlue.UpdateDatabase", handleGlueUpdateDatabase)
	r.Register("AWSGlue.DeleteDatabase", handleGlueDeleteDatabase)
	r.Register("AWSGlue.CreateTable", handleGlueCreateTable)
	r.Register("AWSGlue.GetTable", handleGlueGetTable)
	r.Register("AWSGlue.GetTables", handleGlueGetTables)
	r.Register("AWSGlue.UpdateTable", handleGlueUpdateTable)
	r.Register("AWSGlue.DeleteTable", handleGlueDeleteTable)
	r.Register("AWSGlue.BatchDeleteTable", handleGlueBatchDeleteTable)
	r.Register("AWSGlue.CreatePartition", handleGlueCreatePartition)
	r.Register("AWSGlue.BatchCreatePartition", handleGlueBatchCreatePartition)
	r.Register("AWSGlue.GetPartition", handleGlueGetPartition)
	r.Register("AWSGlue.GetPartitions", handleGlueGetPartitions)
	r.Register("AWSGlue.BatchGetPartition", handleGlueBatchGetPartition)
	r.Register("AWSGlue.UpdatePartition", handleGlueUpdatePartition)
	r.Register("AWSGlue.DeletePartition", handleGlueDeletePartition)
	r.Register("AWSGlue.BatchDeletePartition", handleGlueBatchDeletePartition)
	r.Register("AWSGlue.CreateJob", handleGlueCreateJob)
	r.Register("AWSGlue.GetJob", handleGlueGetJob)
	r.Register("AWSGlue.GetJobs", handleGlueGetJobs)
	r.Register("AWSGlue.DeleteJob", handleGlueDeleteJob)
	r.Register("AWSGlue.StartJobRun", handleGlueStartJobRun)
	r.Register("AWSGlue.GetJobRun", handleGlueGetJobRun)
	r.Register("AWSGlue.GetJobRuns", handleGlueGetJobRuns)
	r.Register("AWSGlue.GetPartitionIndexes", handleGlueGetPartitionIndexes)
	r.Register("AWSGlue.TagResource", handleGlueTagResource)
	r.Register("AWSGlue.UntagResource", handleGlueUntagResource)
	r.Register("AWSGlue.GetTags", handleGlueGetTags)
}

func glueEpochNow() float64 {
	return float64(time.Now().UTC().Unix())
}

func glueWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func glueWriteError(w http.ResponseWriter, code string, msg string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.Header().Set("X-Amzn-Errortype", code)
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"__type":  code,
		"message": msg,
	})
}

// ---------- Database ----------

func handleGlueCreateDatabase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseInput struct {
			Name        string            `json:"Name"`
			Parameters  map[string]string `json:"Parameters"`
			LocationUri string            `json:"LocationUri"`
			Description string            `json:"Description"`
		} `json:"DatabaseInput"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.DatabaseInput.Name == "" {
		glueWriteError(w, "InvalidInputException", "Name is required")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	if _, ok := glueDatabases.Get(req.DatabaseInput.Name); ok {
		glueWriteError(w, "AlreadyExistsException", "Database already exists: "+req.DatabaseInput.Name)
		return
	}
	db := GlueDatabase{
		Name:        req.DatabaseInput.Name,
		Parameters:  req.DatabaseInput.Parameters,
		LocationUri: req.DatabaseInput.LocationUri,
		Description: req.DatabaseInput.Description,
		CreateTime:  glueEpochNow(),
	}
	glueDatabases.Put(req.DatabaseInput.Name, db)
	glueWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleGlueGetDatabase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"Name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	db, ok := glueDatabases.Get(req.Name)
	if !ok {
		glueWriteError(w, "EntityNotFoundException", "Database not found: "+req.Name)
		return
	}
	glueWriteJSON(w, http.StatusOK, map[string]any{"Database": db})
}

func handleGlueGetDatabases(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NextToken  string `json:"NextToken"`
		MaxResults *int   `json:"MaxResults"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	all := glueDatabases.List()
	maxR := 0
	if req.MaxResults != nil {
		maxR = *req.MaxResults
	}
	page, nextTok := awsPage(all, req.NextToken, maxR, 100)
	resp := map[string]any{"DatabaseList": page}
	if nextTok != "" {
		resp["NextToken"] = nextTok
	}
	glueWriteJSON(w, http.StatusOK, resp)
}

func handleGlueDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"Name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	if _, ok := glueDatabases.Get(req.Name); !ok {
		glueWriteError(w, "EntityNotFoundException", "Database not found: "+req.Name)
		return
	}
	glueDatabases.Delete(req.Name)
	glueWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleGlueUpdateDatabase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string `json:"Name"`
		DatabaseInput struct {
			Name        string            `json:"Name"`
			Parameters  map[string]string `json:"Parameters"`
			LocationUri string            `json:"LocationUri"`
			Description string            `json:"Description"`
		} `json:"DatabaseInput"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.Name == "" || req.DatabaseInput.Name == "" {
		glueWriteError(w, "InvalidInputException", "Name and DatabaseInput.Name are required")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	existing, ok := glueDatabases.Get(req.Name)
	if !ok {
		glueWriteError(w, "EntityNotFoundException", "Database not found: "+req.Name)
		return
	}
	updated := GlueDatabase{
		Name:        req.DatabaseInput.Name,
		Parameters:  req.DatabaseInput.Parameters,
		LocationUri: req.DatabaseInput.LocationUri,
		Description: req.DatabaseInput.Description,
		CreateTime:  existing.CreateTime,
	}
	// A rename moves the row; otherwise overwrite in place.
	if req.DatabaseInput.Name != req.Name {
		if _, clash := glueDatabases.Get(req.DatabaseInput.Name); clash {
			glueWriteError(w, "AlreadyExistsException", "Database already exists: "+req.DatabaseInput.Name)
			return
		}
		glueDatabases.Delete(req.Name)
	}
	glueDatabases.Put(req.DatabaseInput.Name, updated)
	glueWriteJSON(w, http.StatusOK, map[string]any{})
}

// ---------- Table ----------

func glueTableKey(database, table string) string {
	return database + "/" + table
}

func handleGlueCreateTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName string `json:"DatabaseName"`
		TableInput   struct {
			Name              string            `json:"Name"`
			StorageDescriptor map[string]any    `json:"StorageDescriptor"`
			PartitionKeys     []map[string]any  `json:"PartitionKeys"`
			Parameters        map[string]string `json:"Parameters"`
			TableType         string            `json:"TableType"`
		} `json:"TableInput"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.DatabaseName == "" || req.TableInput.Name == "" {
		glueWriteError(w, "InvalidInputException", "DatabaseName and Name are required")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	if _, ok := glueDatabases.Get(req.DatabaseName); !ok {
		glueWriteError(w, "EntityNotFoundException", "Database not found: "+req.DatabaseName)
		return
	}
	key := glueTableKey(req.DatabaseName, req.TableInput.Name)
	if _, ok := glueTables.Get(key); ok {
		glueWriteError(w, "AlreadyExistsException", "Table already exists: "+req.TableInput.Name)
		return
	}
	now := glueEpochNow()
	t := GlueTable{
		Name:              req.TableInput.Name,
		DatabaseName:      req.DatabaseName,
		StorageDescriptor: req.TableInput.StorageDescriptor,
		PartitionKeys:     req.TableInput.PartitionKeys,
		Parameters:        req.TableInput.Parameters,
		TableType:         req.TableInput.TableType,
		CreateTime:        now,
		UpdateTime:        now,
	}
	glueTables.Put(key, t)
	glueWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleGlueGetTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName string `json:"DatabaseName"`
		Name         string `json:"Name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	t, ok := glueTables.Get(glueTableKey(req.DatabaseName, req.Name))
	if !ok {
		glueWriteError(w, "EntityNotFoundException", "Table not found: "+req.Name)
		return
	}
	glueWriteJSON(w, http.StatusOK, map[string]any{"Table": t})
}

func handleGlueGetTables(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName string `json:"DatabaseName"`
		NextToken    string `json:"NextToken"`
		MaxResults   *int   `json:"MaxResults"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	all := glueTables.List()
	var filtered []GlueTable
	for _, t := range all {
		if t.DatabaseName == req.DatabaseName {
			filtered = append(filtered, t)
		}
	}
	maxR := 0
	if req.MaxResults != nil {
		maxR = *req.MaxResults
	}
	page, nextTok := awsPage(filtered, req.NextToken, maxR, 100)
	resp := map[string]any{"TableList": page}
	if nextTok != "" {
		resp["NextToken"] = nextTok
	}
	glueWriteJSON(w, http.StatusOK, resp)
}

func handleGlueDeleteTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName string `json:"DatabaseName"`
		Name         string `json:"Name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	key := glueTableKey(req.DatabaseName, req.Name)
	if _, ok := glueTables.Get(key); !ok {
		glueWriteError(w, "EntityNotFoundException", "Table not found: "+req.Name)
		return
	}
	glueTables.Delete(key)
	glueWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleGlueUpdateTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName string `json:"DatabaseName"`
		TableInput   struct {
			Name              string            `json:"Name"`
			StorageDescriptor map[string]any    `json:"StorageDescriptor"`
			PartitionKeys     []map[string]any  `json:"PartitionKeys"`
			Parameters        map[string]string `json:"Parameters"`
			TableType         string            `json:"TableType"`
		} `json:"TableInput"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.DatabaseName == "" || req.TableInput.Name == "" {
		glueWriteError(w, "InvalidInputException", "DatabaseName and TableInput.Name are required")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	key := glueTableKey(req.DatabaseName, req.TableInput.Name)
	existing, ok := glueTables.Get(key)
	if !ok {
		glueWriteError(w, "EntityNotFoundException", "Table not found: "+req.TableInput.Name)
		return
	}
	updated := GlueTable{
		Name:              req.TableInput.Name,
		DatabaseName:      req.DatabaseName,
		StorageDescriptor: req.TableInput.StorageDescriptor,
		PartitionKeys:     req.TableInput.PartitionKeys,
		Parameters:        req.TableInput.Parameters,
		TableType:         req.TableInput.TableType,
		CreateTime:        existing.CreateTime,
		UpdateTime:        glueEpochNow(),
	}
	glueTables.Put(key, updated)
	glueWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleGlueBatchDeleteTable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName   string   `json:"DatabaseName"`
		TablesToDelete []string `json:"TablesToDelete"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.DatabaseName == "" {
		glueWriteError(w, "InvalidInputException", "DatabaseName is required")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	var errs []map[string]any
	for _, name := range req.TablesToDelete {
		key := glueTableKey(req.DatabaseName, name)
		if _, ok := glueTables.Get(key); !ok {
			errs = append(errs, map[string]any{
				"TableName": name,
				"ErrorDetail": map[string]any{
					"ErrorCode":    "EntityNotFoundException",
					"ErrorMessage": "Table not found: " + name,
				},
			})
			continue
		}
		glueTables.Delete(key)
		// Cascade-delete the table's partitions.
		for _, p := range gluePartitions.List() {
			if p.DatabaseName == req.DatabaseName && p.TableName == name {
				gluePartitions.Delete(gluePartitionKey(req.DatabaseName, name, p.Values))
			}
		}
	}
	resp := map[string]any{}
	if len(errs) > 0 {
		resp["Errors"] = errs
	}
	glueWriteJSON(w, http.StatusOK, resp)
}

// ---------- Partition ----------

// gluePartitionKey is the store key for a partition; partition values are
// ordered and joined under a key that also scopes them to (database, table).
func gluePartitionKey(database, table string, values []string) string {
	return database + "/" + table + "/" + strings.Join(values, "\x1f")
}

func glueValuesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// glueRequirePartitionTable validates the (database, table) parent exists.
// Caller holds glueMu. Returns false and writes the error if missing.
func glueRequirePartitionTable(w http.ResponseWriter, database, table string) bool {
	if _, ok := glueDatabases.Get(database); !ok {
		glueWriteError(w, "EntityNotFoundException", "Database not found: "+database)
		return false
	}
	if _, ok := glueTables.Get(glueTableKey(database, table)); !ok {
		glueWriteError(w, "EntityNotFoundException", "Table not found: "+table)
		return false
	}
	return true
}

type gluePartitionInput struct {
	Values            []string          `json:"Values"`
	StorageDescriptor map[string]any    `json:"StorageDescriptor"`
	Parameters        map[string]string `json:"Parameters"`
	LastAccessTime    *float64          `json:"LastAccessTime"`
	LastAnalyzedTime  *float64          `json:"LastAnalyzedTime"`
}

func gluePartitionFromInput(database, table string, in gluePartitionInput, creationTime float64) GluePartition {
	return GluePartition{
		Values:            in.Values,
		DatabaseName:      database,
		TableName:         table,
		StorageDescriptor: in.StorageDescriptor,
		Parameters:        in.Parameters,
		CreationTime:      creationTime,
		LastAccessTime:    in.LastAccessTime,
		LastAnalyzedTime:  in.LastAnalyzedTime,
	}
}

func handleGlueCreatePartition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName   string             `json:"DatabaseName"`
		TableName      string             `json:"TableName"`
		PartitionInput gluePartitionInput `json:"PartitionInput"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.DatabaseName == "" || req.TableName == "" {
		glueWriteError(w, "InvalidInputException", "DatabaseName and TableName are required")
		return
	}
	if len(req.PartitionInput.Values) == 0 {
		glueWriteError(w, "InvalidInputException", "PartitionInput.Values is required")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	if !glueRequirePartitionTable(w, req.DatabaseName, req.TableName) {
		return
	}
	key := gluePartitionKey(req.DatabaseName, req.TableName, req.PartitionInput.Values)
	if _, ok := gluePartitions.Get(key); ok {
		glueWriteError(w, "AlreadyExistsException", "Partition already exists")
		return
	}
	gluePartitions.Put(key, gluePartitionFromInput(req.DatabaseName, req.TableName, req.PartitionInput, glueEpochNow()))
	glueWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleGlueBatchCreatePartition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName       string               `json:"DatabaseName"`
		TableName          string               `json:"TableName"`
		PartitionInputList []gluePartitionInput `json:"PartitionInputList"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.DatabaseName == "" || req.TableName == "" {
		glueWriteError(w, "InvalidInputException", "DatabaseName and TableName are required")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	if !glueRequirePartitionTable(w, req.DatabaseName, req.TableName) {
		return
	}
	now := glueEpochNow()
	var errs []map[string]any
	for _, in := range req.PartitionInputList {
		key := gluePartitionKey(req.DatabaseName, req.TableName, in.Values)
		if _, ok := gluePartitions.Get(key); ok {
			errs = append(errs, map[string]any{
				"PartitionValues": in.Values,
				"ErrorDetail": map[string]any{
					"ErrorCode":    "AlreadyExistsException",
					"ErrorMessage": "Partition already exists",
				},
			})
			continue
		}
		gluePartitions.Put(key, gluePartitionFromInput(req.DatabaseName, req.TableName, in, now))
	}
	resp := map[string]any{}
	if len(errs) > 0 {
		resp["Errors"] = errs
	}
	glueWriteJSON(w, http.StatusOK, resp)
}

func handleGlueGetPartition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName    string   `json:"DatabaseName"`
		TableName       string   `json:"TableName"`
		PartitionValues []string `json:"PartitionValues"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	p, ok := gluePartitions.Get(gluePartitionKey(req.DatabaseName, req.TableName, req.PartitionValues))
	if !ok {
		glueWriteError(w, "EntityNotFoundException", "Partition not found")
		return
	}
	glueWriteJSON(w, http.StatusOK, map[string]any{"Partition": p})
}

func handleGlueGetPartitions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName string `json:"DatabaseName"`
		TableName    string `json:"TableName"`
		NextToken    string `json:"NextToken"`
		MaxResults   *int   `json:"MaxResults"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	var filtered []GluePartition
	for _, p := range gluePartitions.List() {
		if p.DatabaseName == req.DatabaseName && p.TableName == req.TableName {
			filtered = append(filtered, p)
		}
	}
	maxR := 0
	if req.MaxResults != nil {
		maxR = *req.MaxResults
	}
	page, nextTok := awsPage(filtered, req.NextToken, maxR, 100)
	resp := map[string]any{"Partitions": page}
	if nextTok != "" {
		resp["NextToken"] = nextTok
	}
	glueWriteJSON(w, http.StatusOK, resp)
}

func handleGlueBatchGetPartition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName    string `json:"DatabaseName"`
		TableName       string `json:"TableName"`
		PartitionsToGet []struct {
			Values []string `json:"Values"`
		} `json:"PartitionsToGet"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	var found []GluePartition
	var unprocessed []map[string]any
	for _, pk := range req.PartitionsToGet {
		p, ok := gluePartitions.Get(gluePartitionKey(req.DatabaseName, req.TableName, pk.Values))
		if ok {
			found = append(found, p)
		} else {
			unprocessed = append(unprocessed, map[string]any{"Values": pk.Values})
		}
	}
	resp := map[string]any{}
	if len(found) > 0 {
		resp["Partitions"] = found
	}
	if len(unprocessed) > 0 {
		resp["UnprocessedKeys"] = unprocessed
	}
	glueWriteJSON(w, http.StatusOK, resp)
}

func handleGlueUpdatePartition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName       string             `json:"DatabaseName"`
		TableName          string             `json:"TableName"`
		PartitionValueList []string           `json:"PartitionValueList"`
		PartitionInput     gluePartitionInput `json:"PartitionInput"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.DatabaseName == "" || req.TableName == "" {
		glueWriteError(w, "InvalidInputException", "DatabaseName and TableName are required")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	oldKey := gluePartitionKey(req.DatabaseName, req.TableName, req.PartitionValueList)
	existing, ok := gluePartitions.Get(oldKey)
	if !ok {
		glueWriteError(w, "EntityNotFoundException", "Partition not found")
		return
	}
	updated := gluePartitionFromInput(req.DatabaseName, req.TableName, req.PartitionInput, existing.CreationTime)
	// UpdatePartition may move the partition to new values.
	if !glueValuesEqual(req.PartitionInput.Values, req.PartitionValueList) {
		newKey := gluePartitionKey(req.DatabaseName, req.TableName, req.PartitionInput.Values)
		if _, clash := gluePartitions.Get(newKey); clash {
			glueWriteError(w, "AlreadyExistsException", "Partition already exists")
			return
		}
		gluePartitions.Delete(oldKey)
		gluePartitions.Put(newKey, updated)
	} else {
		gluePartitions.Put(oldKey, updated)
	}
	glueWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleGlueDeletePartition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName    string   `json:"DatabaseName"`
		TableName       string   `json:"TableName"`
		PartitionValues []string `json:"PartitionValues"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	key := gluePartitionKey(req.DatabaseName, req.TableName, req.PartitionValues)
	if _, ok := gluePartitions.Get(key); !ok {
		glueWriteError(w, "EntityNotFoundException", "Partition not found")
		return
	}
	gluePartitions.Delete(key)
	glueWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleGlueBatchDeletePartition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseName       string `json:"DatabaseName"`
		TableName          string `json:"TableName"`
		PartitionsToDelete []struct {
			Values []string `json:"Values"`
		} `json:"PartitionsToDelete"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	var errs []map[string]any
	for _, pk := range req.PartitionsToDelete {
		key := gluePartitionKey(req.DatabaseName, req.TableName, pk.Values)
		if _, ok := gluePartitions.Get(key); !ok {
			errs = append(errs, map[string]any{
				"PartitionValues": pk.Values,
				"ErrorDetail": map[string]any{
					"ErrorCode":    "EntityNotFoundException",
					"ErrorMessage": "Partition not found",
				},
			})
			continue
		}
		gluePartitions.Delete(key)
	}
	resp := map[string]any{}
	if len(errs) > 0 {
		resp["Errors"] = errs
	}
	glueWriteJSON(w, http.StatusOK, resp)
}

// ---------- Job ----------

func handleGlueCreateJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string            `json:"Name"`
		Description      string            `json:"Description"`
		Role             string            `json:"Role"`
		Command          map[string]any    `json:"Command"`
		DefaultArguments map[string]string `json:"DefaultArguments"`
		GlueVersion      string            `json:"GlueVersion"`
		MaxCapacity      *float64          `json:"MaxCapacity"`
		WorkerType       string            `json:"WorkerType"`
		NumberOfWorkers  *int              `json:"NumberOfWorkers"`
		Tags             map[string]string `json:"Tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.Name == "" {
		glueWriteError(w, "InvalidInputException", "Name is required")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	if _, ok := glueJobs.Get(req.Name); ok {
		glueWriteError(w, "AlreadyExistsException", "Job already exists: "+req.Name)
		return
	}
	now := glueEpochNow()
	job := GlueJob{
		Name:             req.Name,
		Description:      req.Description,
		Role:             req.Role,
		Command:          req.Command,
		DefaultArguments: req.DefaultArguments,
		GlueVersion:      req.GlueVersion,
		MaxCapacity:      req.MaxCapacity,
		WorkerType:       req.WorkerType,
		NumberOfWorkers:  req.NumberOfWorkers,
		CreatedOn:        now,
		LastModifiedOn:   now,
		Tags:             req.Tags,
	}
	glueJobs.Put(req.Name, job)
	glueWriteJSON(w, http.StatusOK, map[string]any{"Name": req.Name})
}

func handleGlueGetJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobName string `json:"JobName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	job, ok := glueJobs.Get(req.JobName)
	if !ok {
		glueWriteError(w, "EntityNotFoundException", "Job not found: "+req.JobName)
		return
	}
	glueWriteJSON(w, http.StatusOK, map[string]any{"Job": glueJobWire{job}})
}

// glueJobWire wraps GlueJob for response emission. The real Job shape
// has no Tags member — tags ride GetTags. The struct keeps its Tags
// JSON tag so tag state survives Store persistence; the wrapper strips
// it from GetJob/GetJobs responses.
type glueJobWire struct {
	GlueJob
}

func (j glueJobWire) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(j.GlueJob)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	delete(m, "Tags")
	return json.Marshal(m)
}

func handleGlueGetJobs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NextToken  string `json:"NextToken"`
		MaxResults *int   `json:"MaxResults"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	all := glueJobs.List()
	maxR := 0
	if req.MaxResults != nil {
		maxR = *req.MaxResults
	}
	page, nextTok := awsPage(all, req.NextToken, maxR, 25)
	jobs := make([]glueJobWire, 0, len(page))
	for _, job := range page {
		jobs = append(jobs, glueJobWire{job})
	}
	resp := map[string]any{"Jobs": jobs}
	if nextTok != "" {
		resp["NextToken"] = nextTok
	}
	glueWriteJSON(w, http.StatusOK, resp)
}

func handleGlueDeleteJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobName string `json:"JobName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	glueJobs.Delete(req.JobName)
	glueWriteJSON(w, http.StatusOK, map[string]any{"JobName": req.JobName})
}

func handleGlueStartJobRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobName   string            `json:"JobName"`
		Arguments map[string]string `json:"Arguments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	job, ok := glueJobs.Get(req.JobName)
	if !ok {
		glueWriteError(w, "EntityNotFoundException", "Job not found: "+req.JobName)
		return
	}
	script, err := gluePythonScript(job)
	if err != nil {
		glueWriteError(w, "InvalidInputException", err.Error())
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	runID := strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
	now := glueEpochNow()
	run := GlueJobRun{
		ID:          runID,
		JobName:     req.JobName,
		JobRunState: "RUNNING",
		StartedOn:   now,
		Arguments:   req.Arguments,
	}
	glueJobRuns.Put(req.JobName+"/"+runID, run)
	go glueRunPythonJob(req.JobName, runID, script, req.Arguments)
	glueWriteJSON(w, http.StatusOK, map[string]any{"JobRunId": runID})
}

func gluePythonScript(job GlueJob) ([]byte, error) {
	commandName := glueString(job.Command["Name"])
	if commandName == "" {
		commandName = glueString(job.Command["name"])
	}
	if commandName != "pythonshell" {
		return nil, fmt.Errorf("only pythonshell jobs execute in the simulator")
	}
	location := glueString(job.Command["ScriptLocation"])
	if location == "" {
		location = glueString(job.Command["scriptLocation"])
	}
	if location == "" {
		return nil, fmt.Errorf("Command.ScriptLocation is required")
	}
	bucket, key, ok := glueS3Location(location)
	if !ok {
		return nil, fmt.Errorf("Command.ScriptLocation must be an s3:// URL")
	}
	obj, ok := s3Objects.Get(s3ObjectKey(bucket, key))
	if !ok {
		return nil, fmt.Errorf("script object not found: %s", location)
	}
	return obj.Data, nil
}

func glueRunPythonJob(jobName, runID string, script []byte, args map[string]string) {
	workDir, err := os.MkdirTemp("", "sockerless-glue-*")
	if err != nil {
		glueCompleteRun(jobName, runID, "FAILED", 0, err.Error())
		return
	}
	defer os.RemoveAll(workDir)

	scriptPath := filepath.Join(workDir, "script.py")
	if err := os.WriteFile(scriptPath, script, 0700); err != nil {
		glueCompleteRun(jobName, runID, "FAILED", 0, err.Error())
		return
	}

	command := append([]string{"python3", scriptPath}, glueArgs(args)...)
	handle := sim.StartProcess(sim.ProcessConfig{
		Command: command,
		Dir:     filepath.Clean(workDir),
		Env: map[string]string{
			"PATH": os.Getenv("PATH"),
			"HOME": os.Getenv("HOME"),
		},
	}, sim.NoopSink{})
	result := handle.Wait()
	status := "SUCCEEDED"
	reason := ""
	if result.Error != nil {
		status = "FAILED"
		reason = result.Error.Error()
	} else if result.ExitCode != 0 {
		status = "FAILED"
		reason = fmt.Sprintf("Python shell script exited with status %d", result.ExitCode)
	}
	executionTime := int(result.StoppedAt.Sub(result.StartedAt).Seconds())
	if executionTime < 1 {
		executionTime = 1
	}
	glueCompleteRun(jobName, runID, status, executionTime, reason)
}

func glueCompleteRun(jobName, runID, status string, executionTime int, reason string) {
	glueMu.Lock()
	defer glueMu.Unlock()

	key := jobName + "/" + runID
	run, ok := glueJobRuns.Get(key)
	if !ok {
		return
	}
	run.JobRunState = status
	run.CompletedOn = glueEpochNow()
	run.ExecutionTime = executionTime
	run.ErrorMessage = reason
	glueJobRuns.Put(key, run)
}

func handleGlueGetJobRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobName string `json:"JobName"`
		RunId   string `json:"RunId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	run, ok := glueJobRuns.Get(req.JobName + "/" + req.RunId)
	if !ok {
		glueWriteError(w, "EntityNotFoundException", "Job run not found: "+req.RunId)
		return
	}
	glueWriteJSON(w, http.StatusOK, map[string]any{"JobRun": run})
}

func handleGlueGetJobRuns(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobName    string `json:"JobName"`
		NextToken  string `json:"NextToken"`
		MaxResults *int   `json:"MaxResults"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	if _, ok := glueJobs.Get(req.JobName); !ok {
		glueWriteError(w, "EntityNotFoundException", "Job with name: "+req.JobName+" not found")
		return
	}

	all := glueJobRuns.List()
	var runs []GlueJobRun
	for _, run := range all {
		if run.JobName == req.JobName {
			runs = append(runs, run)
		}
	}
	maxR := 0
	if req.MaxResults != nil {
		maxR = *req.MaxResults
	}
	page, nextTok := awsPage(runs, req.NextToken, maxR, 25)
	resp := map[string]any{"JobRuns": page}
	if nextTok != "" {
		resp["NextToken"] = nextTok
	}
	glueWriteJSON(w, http.StatusOK, resp)
}

// ---------- Partition indexes ----------

func handleGlueGetPartitionIndexes(w http.ResponseWriter, r *http.Request) {
	// TF provider reads partition indexes on table refresh; sim has none.
	_ = r.Body
	glueWriteJSON(w, http.StatusOK, map[string]any{"PartitionIndexDescriptorList": []any{}})
}

// ---------- Tags ----------

// glueResourceFromARN splits the last colon-segment of a Glue ARN into resource type and name.
// e.g. arn:aws:glue:us-east-1::database/my-db → ("database", "my-db")
//
//	arn:aws:glue:us-east-1:123:job/my-job  → ("job", "my-job")
func glueResourceFromARN(arn string) (resType, name string) {
	idx := strings.LastIndex(arn, ":")
	resource := arn[idx+1:]
	slash := strings.Index(resource, "/")
	if slash < 0 {
		return "job", resource
	}
	return resource[:slash], resource[slash+1:]
}

func glueString(v any) string {
	s, _ := v.(string)
	return s
}

func glueS3Location(location string) (bucket, key string, ok bool) {
	if !strings.HasPrefix(location, "s3://") {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(location, "s3://"), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func glueArgs(args map[string]string) []string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(args)*2)
	for _, k := range keys {
		out = append(out, k, args[k])
	}
	return out
}

func handleGlueTagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string            `json:"ResourceArn"`
		TagsToAdd   map[string]string `json:"TagsToAdd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	resType, name := glueResourceFromARN(req.ResourceArn)
	switch resType {
	case "database":
		db, ok := glueDatabases.Get(name)
		if !ok {
			glueWriteError(w, "EntityNotFoundException", "Resource not found: "+req.ResourceArn)
			return
		}
		if db.Tags == nil {
			db.Tags = make(map[string]string)
		}
		for k, v := range req.TagsToAdd {
			db.Tags[k] = v
		}
		glueDatabases.Put(name, db)
	default:
		job, ok := glueJobs.Get(name)
		if !ok {
			glueWriteError(w, "EntityNotFoundException", "Resource not found: "+req.ResourceArn)
			return
		}
		if job.Tags == nil {
			job.Tags = make(map[string]string)
		}
		for k, v := range req.TagsToAdd {
			job.Tags[k] = v
		}
		glueJobs.Put(name, job)
	}
	glueWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleGlueUntagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn  string   `json:"ResourceArn"`
		TagsToRemove []string `json:"TagsToRemove"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	glueMu.Lock()
	defer glueMu.Unlock()

	resType, name := glueResourceFromARN(req.ResourceArn)
	switch resType {
	case "database":
		db, ok := glueDatabases.Get(name)
		if !ok {
			glueWriteError(w, "EntityNotFoundException", "Resource not found: "+req.ResourceArn)
			return
		}
		for _, k := range req.TagsToRemove {
			delete(db.Tags, k)
		}
		glueDatabases.Put(name, db)
	default:
		job, ok := glueJobs.Get(name)
		if !ok {
			glueWriteError(w, "EntityNotFoundException", "Resource not found: "+req.ResourceArn)
			return
		}
		for _, k := range req.TagsToRemove {
			delete(job.Tags, k)
		}
		glueJobs.Put(name, job)
	}
	glueWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleGlueGetTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string `json:"ResourceArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glueWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	resType, name := glueResourceFromARN(req.ResourceArn)
	var tags map[string]string
	switch resType {
	case "database":
		db, ok := glueDatabases.Get(name)
		if !ok {
			glueWriteError(w, "EntityNotFoundException", "Resource not found: "+req.ResourceArn)
			return
		}
		tags = db.Tags
	default:
		job, ok := glueJobs.Get(name)
		if !ok {
			glueWriteError(w, "EntityNotFoundException", "Resource not found: "+req.ResourceArn)
			return
		}
		tags = job.Tags
	}
	if tags == nil {
		tags = map[string]string{}
	}
	glueWriteJSON(w, http.StatusOK, map[string]any{"Tags": tags})
}
