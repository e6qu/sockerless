package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// BigQuery v2 REST surface. The simulator implements dataset/table
// lifecycle, streaming inserts, query jobs, synchronous queries, and
// tabledata paging against persisted rows.

type BQDataset struct {
	Kind             string            `json:"kind"`
	Etag             string            `json:"etag,omitempty"`
	ID               string            `json:"id"`
	SelfLink         string            `json:"selfLink,omitempty"`
	DatasetReference BQDatasetRef      `json:"datasetReference"`
	FriendlyName     string            `json:"friendlyName,omitempty"`
	Description      string            `json:"description,omitempty"`
	Location         string            `json:"location,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	CreationTime     string            `json:"creationTime,omitempty"`
	LastModifiedTime string            `json:"lastModifiedTime,omitempty"`
}

type BQDatasetRef struct {
	ProjectID string `json:"projectId,omitempty"`
	DatasetID string `json:"datasetId"`
}

type BQTable struct {
	Kind             string            `json:"kind"`
	Etag             string            `json:"etag,omitempty"`
	ID               string            `json:"id"`
	SelfLink         string            `json:"selfLink,omitempty"`
	TableReference   BQTableRef        `json:"tableReference"`
	FriendlyName     string            `json:"friendlyName,omitempty"`
	Description      string            `json:"description,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Schema           *BQSchema         `json:"schema,omitempty"`
	Type             string            `json:"type,omitempty"`
	Location         string            `json:"location,omitempty"`
	CreationTime     string            `json:"creationTime,omitempty"`
	LastModifiedTime string            `json:"lastModifiedTime,omitempty"`
	NumRows          string            `json:"numRows"`
	NumBytes         string            `json:"numBytes"`
}

type BQTableRef struct {
	ProjectID string `json:"projectId,omitempty"`
	DatasetID string `json:"datasetId"`
	TableID   string `json:"tableId"`
}

type BQSchema struct {
	Fields []BQFieldSchema `json:"fields,omitempty"`
}

type BQFieldSchema struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Mode string `json:"mode,omitempty"`
}

type BQRowSet struct {
	Rows []map[string]any `json:"rows"`
}

type BQJob struct {
	Kind          string         `json:"kind"`
	Etag          string         `json:"etag,omitempty"`
	ID            string         `json:"id"`
	SelfLink      string         `json:"selfLink,omitempty"`
	JobReference  BQJobRef       `json:"jobReference"`
	Configuration map[string]any `json:"configuration,omitempty"`
	Status        map[string]any `json:"status"`
	Statistics    map[string]any `json:"statistics,omitempty"`
	UserEmail     string         `json:"user_email,omitempty"`
	Query         string         `json:"query,omitempty"`
	Result        BQQueryResult  `json:"result,omitempty"`
}

type BQJobRef struct {
	ProjectID string `json:"projectId"`
	JobID     string `json:"jobId"`
	Location  string `json:"location,omitempty"`
}

type BQQueryResult struct {
	Kind             string       `json:"kind,omitempty"`
	Schema           *BQSchema    `json:"schema,omitempty"`
	Rows             []BQTableRow `json:"rows,omitempty"`
	TotalRows        string       `json:"totalRows"`
	JobComplete      bool         `json:"jobComplete"`
	CacheHit         bool         `json:"cacheHit"`
	TotalBytesBilled string       `json:"totalBytesBilled,omitempty"`
}

type BQTableRow struct {
	F []map[string]any `json:"f"`
}

var (
	bqDatasets sim.Store[BQDataset]
	bqTables   sim.Store[BQTable]
	bqRows     sim.Store[BQRowSet]
	bqJobs     sim.Store[BQJob]

	bqFromRE  = regexp.MustCompile("(?i)\\bfrom\\s+`?([^`\\s]+)`?")
	bqWhereRE = regexp.MustCompile(`(?i)\bwhere\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*('([^']*)'|"([^"]*)"|[^\s;]+)`)
)

func registerBigQuery(srv *sim.Server) {
	bqDatasets = sim.MakeStore[BQDataset](srv.DB(), "bigquery_datasets")
	bqTables = sim.MakeStore[BQTable](srv.DB(), "bigquery_tables")
	bqRows = sim.MakeStore[BQRowSet](srv.DB(), "bigquery_rows")
	bqJobs = sim.MakeStore[BQJob](srv.DB(), "bigquery_jobs")

	srv.HandleFunc("POST /bigquery/v2/projects/{project}/datasets", handleBQInsertDataset)
	srv.HandleFunc("GET /bigquery/v2/projects/{project}/datasets", handleBQListDatasets)
	srv.HandleFunc("GET /bigquery/v2/projects/{project}/datasets/{dataset}", handleBQGetDataset)
	srv.HandleFunc("PATCH /bigquery/v2/projects/{project}/datasets/{dataset}", handleBQPatchDataset)
	srv.HandleFunc("PUT /bigquery/v2/projects/{project}/datasets/{dataset}", handleBQPatchDataset)
	srv.HandleFunc("DELETE /bigquery/v2/projects/{project}/datasets/{dataset}", handleBQDeleteDataset)

	srv.HandleFunc("POST /bigquery/v2/projects/{project}/datasets/{dataset}/tables", handleBQInsertTable)
	srv.HandleFunc("GET /bigquery/v2/projects/{project}/datasets/{dataset}/tables", handleBQListTables)
	srv.HandleFunc("GET /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}", handleBQGetTable)
	srv.HandleFunc("PATCH /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}", handleBQPatchTable)
	srv.HandleFunc("PUT /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}", handleBQPatchTable)
	srv.HandleFunc("DELETE /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}", handleBQDeleteTable)

	srv.HandleFunc("POST /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}/insertAll", handleBQInsertAll)
	srv.HandleFunc("GET /bigquery/v2/projects/{project}/datasets/{dataset}/tables/{table}/data", handleBQTableDataList)

	srv.HandleFunc("POST /bigquery/v2/projects/{project}/queries", handleBQQuery)
	srv.HandleFunc("POST /bigquery/v2/projects/{project}/jobs", handleBQInsertJob)
	srv.HandleFunc("GET /bigquery/v2/projects/{project}/jobs", handleBQListJobs)
	srv.HandleFunc("GET /bigquery/v2/projects/{project}/jobs/{job}", handleBQGetJob)
	srv.HandleFunc("GET /bigquery/v2/projects/{project}/queries/{job}", handleBQGetQueryResults)
}

func bqDatasetKey(project, dataset string) string {
	return project + "/" + dataset
}

func bqTableKey(project, dataset, table string) string {
	return project + "/" + dataset + "/" + table
}

func bqJobKey(project, job string) string {
	return project + "/" + job
}

func bqMillisNow() string {
	return strconv.FormatInt(time.Now().UTC().UnixMilli(), 10)
}

func bqEtag(v any) string {
	b, _ := json.Marshal(v)
	return fmt.Sprintf(`"%x"`, len(b))
}

func bqDatasetSelfLink(r *http.Request, project, dataset string) string {
	return gcpSelfLink(r, "/bigquery/v2/projects/"+project+"/datasets/"+dataset)
}

func bqTableSelfLink(r *http.Request, project, dataset, table string) string {
	return gcpSelfLink(r, "/bigquery/v2/projects/"+project+"/datasets/"+dataset+"/tables/"+table)
}

func bqApplyDatasetDefaults(r *http.Request, d BQDataset, project, dataset string) BQDataset {
	now := bqMillisNow()
	d.Kind = "bigquery#dataset"
	d.ID = project + ":" + dataset
	d.DatasetReference.ProjectID = project
	d.DatasetReference.DatasetID = dataset
	d.SelfLink = bqDatasetSelfLink(r, project, dataset)
	if d.Location == "" {
		d.Location = "US"
	}
	if d.CreationTime == "" {
		d.CreationTime = now
	}
	d.LastModifiedTime = now
	d.Etag = bqEtag(d)
	return d
}

func bqApplyTableDefaults(r *http.Request, t BQTable, project, dataset, table string) BQTable {
	now := bqMillisNow()
	t.Kind = "bigquery#table"
	t.ID = project + ":" + dataset + "." + table
	t.TableReference.ProjectID = project
	t.TableReference.DatasetID = dataset
	t.TableReference.TableID = table
	t.SelfLink = bqTableSelfLink(r, project, dataset, table)
	if t.Type == "" {
		t.Type = "TABLE"
	}
	if t.Location == "" {
		t.Location = "US"
	}
	if t.CreationTime == "" {
		t.CreationTime = now
	}
	t.LastModifiedTime = now
	if rows, ok := bqRows.Get(bqTableKey(project, dataset, table)); ok {
		t.NumRows = strconv.Itoa(len(rows.Rows))
	} else if t.NumRows == "" {
		t.NumRows = "0"
	}
	t.NumBytes = strconv.Itoa(len(t.NumRows))
	t.Etag = bqEtag(t)
	return t
}

func handleBQInsertDataset(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	var req BQDataset
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid dataset body: %v", err)
		return
	}
	dataset := req.DatasetReference.DatasetID
	if dataset == "" {
		sim.GCPError(w, http.StatusBadRequest, "datasetReference.datasetId is required", "INVALID_ARGUMENT")
		return
	}
	key := bqDatasetKey(project, dataset)
	if _, ok := bqDatasets.Get(key); ok {
		sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "Already Exists: Dataset %s:%s", project, dataset)
		return
	}
	req = bqApplyDatasetDefaults(r, req, project, dataset)
	bqDatasets.Put(key, req)
	sim.WriteJSON(w, http.StatusOK, req)
}

func handleBQGetDataset(w http.ResponseWriter, r *http.Request) {
	project, dataset := sim.PathParam(r, "project"), sim.PathParam(r, "dataset")
	d, ok := bqDatasets.Get(bqDatasetKey(project, dataset))
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Not found: Dataset %s:%s", project, dataset)
		return
	}
	sim.WriteJSON(w, http.StatusOK, d)
}

func handleBQListDatasets(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	all := bqDatasets.Filter(func(d BQDataset) bool {
		return d.DatasetReference.ProjectID == project
	})
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	items := make([]map[string]any, 0, len(all))
	for _, d := range all {
		items = append(items, map[string]any{
			"kind":             "bigquery#dataset",
			"id":               d.ID,
			"datasetReference": d.DatasetReference,
			"friendlyName":     d.FriendlyName,
			"labels":           d.Labels,
			"location":         d.Location,
		})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"kind": "bigquery#datasetList", "datasets": items})
}

func handleBQPatchDataset(w http.ResponseWriter, r *http.Request) {
	project, dataset := sim.PathParam(r, "project"), sim.PathParam(r, "dataset")
	key := bqDatasetKey(project, dataset)
	current, ok := bqDatasets.Get(key)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Not found: Dataset %s:%s", project, dataset)
		return
	}
	var req BQDataset
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid dataset body: %v", err)
		return
	}
	if req.FriendlyName != "" {
		current.FriendlyName = req.FriendlyName
	}
	if req.Description != "" {
		current.Description = req.Description
	}
	if req.Location != "" {
		current.Location = req.Location
	}
	if req.Labels != nil {
		current.Labels = req.Labels
	}
	current = bqApplyDatasetDefaults(r, current, project, dataset)
	bqDatasets.Put(key, current)
	sim.WriteJSON(w, http.StatusOK, current)
}

func handleBQDeleteDataset(w http.ResponseWriter, r *http.Request) {
	project, dataset := sim.PathParam(r, "project"), sim.PathParam(r, "dataset")
	if !bqDatasets.Delete(bqDatasetKey(project, dataset)) {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Not found: Dataset %s:%s", project, dataset)
		return
	}
	prefix := project + "/" + dataset + "/"
	for _, t := range bqTables.List() {
		if strings.HasPrefix(bqTableKey(t.TableReference.ProjectID, t.TableReference.DatasetID, t.TableReference.TableID), prefix) {
			key := bqTableKey(t.TableReference.ProjectID, t.TableReference.DatasetID, t.TableReference.TableID)
			bqTables.Delete(key)
			bqRows.Delete(key)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleBQInsertTable(w http.ResponseWriter, r *http.Request) {
	project, dataset := sim.PathParam(r, "project"), sim.PathParam(r, "dataset")
	if _, ok := bqDatasets.Get(bqDatasetKey(project, dataset)); !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Not found: Dataset %s:%s", project, dataset)
		return
	}
	var req BQTable
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid table body: %v", err)
		return
	}
	table := req.TableReference.TableID
	if table == "" {
		sim.GCPError(w, http.StatusBadRequest, "tableReference.tableId is required", "INVALID_ARGUMENT")
		return
	}
	key := bqTableKey(project, dataset, table)
	if _, ok := bqTables.Get(key); ok {
		sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "Already Exists: Table %s:%s.%s", project, dataset, table)
		return
	}
	req = bqApplyTableDefaults(r, req, project, dataset, table)
	bqTables.Put(key, req)
	bqRows.Put(key, BQRowSet{Rows: []map[string]any{}})
	sim.WriteJSON(w, http.StatusOK, req)
}

func handleBQGetTable(w http.ResponseWriter, r *http.Request) {
	project, dataset, table := sim.PathParam(r, "project"), sim.PathParam(r, "dataset"), sim.PathParam(r, "table")
	t, ok := bqTables.Get(bqTableKey(project, dataset, table))
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Not found: Table %s:%s.%s", project, dataset, table)
		return
	}
	sim.WriteJSON(w, http.StatusOK, bqApplyTableDefaults(r, t, project, dataset, table))
}

func handleBQListTables(w http.ResponseWriter, r *http.Request) {
	project, dataset := sim.PathParam(r, "project"), sim.PathParam(r, "dataset")
	all := bqTables.Filter(func(t BQTable) bool {
		return t.TableReference.ProjectID == project && t.TableReference.DatasetID == dataset
	})
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	items := make([]map[string]any, 0, len(all))
	for _, t := range all {
		items = append(items, map[string]any{
			"kind":           "bigquery#table",
			"id":             t.ID,
			"tableReference": t.TableReference,
			"type":           t.Type,
			"friendlyName":   t.FriendlyName,
			"labels":         t.Labels,
		})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"kind": "bigquery#tableList", "tables": items})
}

func handleBQPatchTable(w http.ResponseWriter, r *http.Request) {
	project, dataset, table := sim.PathParam(r, "project"), sim.PathParam(r, "dataset"), sim.PathParam(r, "table")
	key := bqTableKey(project, dataset, table)
	current, ok := bqTables.Get(key)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Not found: Table %s:%s.%s", project, dataset, table)
		return
	}
	var req BQTable
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid table body: %v", err)
		return
	}
	if req.FriendlyName != "" {
		current.FriendlyName = req.FriendlyName
	}
	if req.Description != "" {
		current.Description = req.Description
	}
	if req.Labels != nil {
		current.Labels = req.Labels
	}
	if req.Schema != nil {
		current.Schema = req.Schema
	}
	current = bqApplyTableDefaults(r, current, project, dataset, table)
	bqTables.Put(key, current)
	sim.WriteJSON(w, http.StatusOK, current)
}

func handleBQDeleteTable(w http.ResponseWriter, r *http.Request) {
	project, dataset, table := sim.PathParam(r, "project"), sim.PathParam(r, "dataset"), sim.PathParam(r, "table")
	key := bqTableKey(project, dataset, table)
	if !bqTables.Delete(key) {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Not found: Table %s:%s.%s", project, dataset, table)
		return
	}
	bqRows.Delete(key)
	w.WriteHeader(http.StatusNoContent)
}

func handleBQInsertAll(w http.ResponseWriter, r *http.Request) {
	project, dataset, table := sim.PathParam(r, "project"), sim.PathParam(r, "dataset"), sim.PathParam(r, "table")
	key := bqTableKey(project, dataset, table)
	if _, ok := bqTables.Get(key); !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Not found: Table %s:%s.%s", project, dataset, table)
		return
	}
	var req struct {
		Rows []struct {
			InsertID string         `json:"insertId,omitempty"`
			JSON     map[string]any `json:"json"`
		} `json:"rows"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid insertAll body: %v", err)
		return
	}
	rowSet, _ := bqRows.Get(key)
	for _, row := range req.Rows {
		copied := make(map[string]any, len(row.JSON))
		for k, v := range row.JSON {
			copied[k] = v
		}
		rowSet.Rows = append(rowSet.Rows, copied)
	}
	bqRows.Put(key, rowSet)
	if t, ok := bqTables.Get(key); ok {
		t = bqApplyTableDefaults(r, t, project, dataset, table)
		bqTables.Put(key, t)
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"kind": "bigquery#tableDataInsertAllResponse"})
}

func handleBQTableDataList(w http.ResponseWriter, r *http.Request) {
	project, dataset, table := sim.PathParam(r, "project"), sim.PathParam(r, "dataset"), sim.PathParam(r, "table")
	t, ok := bqTables.Get(bqTableKey(project, dataset, table))
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Not found: Table %s:%s.%s", project, dataset, table)
		return
	}
	rowSet, _ := bqRows.Get(bqTableKey(project, dataset, table))
	start, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
	max, _ := strconv.Atoi(r.URL.Query().Get("maxResults"))
	if max <= 0 {
		max = len(rowSet.Rows)
	}
	end := start + max
	if start > len(rowSet.Rows) {
		start = len(rowSet.Rows)
	}
	if end > len(rowSet.Rows) {
		end = len(rowSet.Rows)
	}
	rows := bqEncodeRows(t.Schema, rowSet.Rows[start:end])
	resp := map[string]any{
		"kind":      "bigquery#tableDataList",
		"totalRows": strconv.Itoa(len(rowSet.Rows)),
		"rows":      rows,
	}
	if end < len(rowSet.Rows) {
		resp["pageToken"] = strconv.Itoa(end)
	}
	sim.WriteJSON(w, http.StatusOK, resp)
}

func handleBQQuery(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	var req struct {
		Query    string `json:"query"`
		Location string `json:"location,omitempty"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid query body: %v", err)
		return
	}
	result, err := bqEvaluateQuery(project, req.Query)
	if err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%v", err)
		return
	}
	jobID := "job_" + generateUUID()
	job := bqDoneQueryJob(r, project, jobID, req.Location, req.Query, result)
	bqJobs.Put(bqJobKey(project, jobID), job)
	result.Kind = "bigquery#queryResponse"
	sim.WriteJSON(w, http.StatusOK, result)
}

func handleBQInsertJob(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	var req BQJob
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid job body: %v", err)
		return
	}
	jobID := req.JobReference.JobID
	if jobID == "" {
		jobID = "job_" + generateUUID()
	}
	location := req.JobReference.Location
	query := ""
	if q, ok := req.Configuration["query"].(map[string]any); ok {
		query, _ = q["query"].(string)
	}
	result := BQQueryResult{JobComplete: true, TotalRows: "0", CacheHit: false}
	if query != "" {
		var err error
		result, err = bqEvaluateQuery(project, query)
		if err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%v", err)
			return
		}
	}
	job := bqDoneQueryJob(r, project, jobID, location, query, result)
	job.Configuration = req.Configuration
	bqJobs.Put(bqJobKey(project, jobID), job)
	sim.WriteJSON(w, http.StatusOK, job)
}

func handleBQGetJob(w http.ResponseWriter, r *http.Request) {
	project, jobID := sim.PathParam(r, "project"), sim.PathParam(r, "job")
	job, ok := bqJobs.Get(bqJobKey(project, jobID))
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Not found: Job %s:%s", project, jobID)
		return
	}
	sim.WriteJSON(w, http.StatusOK, job)
}

func handleBQListJobs(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	all := bqJobs.Filter(func(j BQJob) bool { return j.JobReference.ProjectID == project })
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	// jobs.list items follow the lighter JobListJobs shape: the running state
	// is a top-level `state` field (mirrored from status.state), not nested.
	items := make([]map[string]any, 0, len(all))
	for _, j := range all {
		state, _ := j.Status["state"].(string)
		items = append(items, map[string]any{
			"kind":          "bigquery#job",
			"id":            j.ID,
			"jobReference":  j.JobReference,
			"state":         state,
			"status":        j.Status,
			"statistics":    j.Statistics,
			"configuration": j.Configuration,
			"user_email":    j.UserEmail,
		})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"kind": "bigquery#jobList", "jobs": items})
}

func handleBQGetQueryResults(w http.ResponseWriter, r *http.Request) {
	project, jobID := sim.PathParam(r, "project"), sim.PathParam(r, "job")
	job, ok := bqJobs.Get(bqJobKey(project, jobID))
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Not found: Job %s:%s", project, jobID)
		return
	}
	result := job.Result
	result.Kind = "bigquery#getQueryResultsResponse"
	sim.WriteJSON(w, http.StatusOK, result)
}

func bqDoneQueryJob(r *http.Request, project, jobID, location, query string, result BQQueryResult) BQJob {
	if location == "" {
		location = "US"
	}
	return BQJob{
		Kind:     "bigquery#job",
		Etag:     bqEtag(jobID),
		ID:       project + ":" + jobID,
		SelfLink: gcpSelfLink(r, "/bigquery/v2/projects/"+project+"/jobs/"+jobID),
		JobReference: BQJobRef{
			ProjectID: project,
			JobID:     jobID,
			Location:  location,
		},
		Configuration: map[string]any{"query": map[string]any{"query": query}},
		Status:        map[string]any{"state": "DONE"},
		Statistics: map[string]any{
			"creationTime": bqMillisNow(),
			"startTime":    bqMillisNow(),
			"endTime":      bqMillisNow(),
			"query":        map[string]any{"totalBytesProcessed": "0", "cacheHit": false},
		},
		Query:  query,
		Result: result,
	}
}

func bqEvaluateQuery(defaultProject, query string) (BQQueryResult, error) {
	query = strings.TrimSpace(strings.TrimSuffix(query, ";"))
	m := bqFromRE.FindStringSubmatch(query)
	if len(m) < 2 {
		return BQQueryResult{}, fmt.Errorf("only SELECT queries with FROM are supported")
	}
	project, dataset, table, err := bqParseTableRef(defaultProject, m[1])
	if err != nil {
		return BQQueryResult{}, err
	}
	t, ok := bqTables.Get(bqTableKey(project, dataset, table))
	if !ok {
		return BQQueryResult{}, fmt.Errorf("not found: Table %s:%s.%s", project, dataset, table)
	}
	rowSet, _ := bqRows.Get(bqTableKey(project, dataset, table))
	rows := rowSet.Rows
	if wm := bqWhereRE.FindStringSubmatch(query); len(wm) >= 3 {
		field := wm[1]
		want := strings.Trim(wm[2], `'"`)
		filtered := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			if fmt.Sprint(row[field]) == want {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	return BQQueryResult{
		Schema:           t.Schema,
		Rows:             bqEncodeRows(t.Schema, rows),
		TotalRows:        strconv.Itoa(len(rows)),
		JobComplete:      true,
		CacheHit:         false,
		TotalBytesBilled: "0",
	}, nil
}

func bqParseTableRef(defaultProject, ref string) (string, string, string, error) {
	ref = strings.Trim(ref, "`")
	parts := strings.Split(ref, ".")
	switch len(parts) {
	case 2:
		return defaultProject, parts[0], parts[1], nil
	case 3:
		return parts[0], parts[1], parts[2], nil
	default:
		return "", "", "", fmt.Errorf("invalid table reference %q", ref)
	}
}

func bqEncodeRows(schema *BQSchema, rows []map[string]any) []BQTableRow {
	fields := []string{}
	if schema != nil {
		for _, f := range schema.Fields {
			fields = append(fields, f.Name)
		}
	}
	if len(fields) == 0 && len(rows) > 0 {
		for k := range rows[0] {
			fields = append(fields, k)
		}
		sort.Strings(fields)
	}
	out := make([]BQTableRow, 0, len(rows))
	for _, row := range rows {
		tr := BQTableRow{F: make([]map[string]any, 0, len(fields))}
		for _, f := range fields {
			tr.F = append(tr.F, map[string]any{"v": row[f]})
		}
		out = append(out, tr)
	}
	return out
}
