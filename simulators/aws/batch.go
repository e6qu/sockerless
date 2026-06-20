package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sim "github.com/sockerless/simulator"
)

// AWS Batch — REST/JSON protocol with operation-specific POST paths (/v1/<opname>).
// All operations are POST. Container jobs execute through the shared workload runner.

type BatchComputeEnvironment struct {
	ComputeEnvironmentName string            `json:"computeEnvironmentName"`
	ComputeEnvironmentArn  string            `json:"computeEnvironmentArn"`
	EcsClusterArn          string            `json:"ecsClusterArn"`
	State                  string            `json:"state"`
	Status                 string            `json:"status"`
	StatusReason           string            `json:"statusReason,omitempty"`
	Type                   string            `json:"type"`
	ComputeResources       map[string]any    `json:"computeResources,omitempty"`
	ServiceRole            string            `json:"serviceRole,omitempty"`
	Tags                   map[string]string `json:"tags,omitempty"`
}

type BatchJobQueue struct {
	JobQueueName            string            `json:"jobQueueName"`
	JobQueueArn             string            `json:"jobQueueArn"`
	State                   string            `json:"state"`
	Status                  string            `json:"status"`
	StatusReason            string            `json:"statusReason,omitempty"`
	Priority                int               `json:"priority"`
	ComputeEnvironmentOrder []map[string]any  `json:"computeEnvironmentOrder"`
	Tags                    map[string]string `json:"tags,omitempty"`
}

type BatchJobDefinition struct {
	JobDefinitionName   string            `json:"jobDefinitionName"`
	JobDefinitionArn    string            `json:"jobDefinitionArn"`
	Revision            int               `json:"revision"`
	Status              string            `json:"status"`
	Type                string            `json:"type"`
	ContainerProperties map[string]any    `json:"containerProperties,omitempty"`
	RetryStrategy       map[string]any    `json:"retryStrategy,omitempty"`
	Timeout             map[string]any    `json:"timeout,omitempty"`
	Tags                map[string]string `json:"tags,omitempty"`
}

type BatchJob struct {
	JobID         string            `json:"jobId"`
	JobArn        string            `json:"jobArn,omitempty"`
	JobName       string            `json:"jobName"`
	JobQueue      string            `json:"jobQueue"`
	Status        string            `json:"status"`
	StatusReason  string            `json:"statusReason,omitempty"`
	JobDefinition string            `json:"jobDefinition"`
	CreatedAt     int64             `json:"createdAt"`
	StartedAt     int64             `json:"startedAt"`
	StoppedAt     int64             `json:"stoppedAt"`
	Container     map[string]any    `json:"container,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
}

var (
	batchComputeEnvs  sim.Store[BatchComputeEnvironment]
	batchJobQueues    sim.Store[BatchJobQueue]
	batchJobDefs      sim.Store[BatchJobDefinition]
	batchJobs         sim.Store[BatchJob]
	batchJobRevisions sim.Store[int]
	batchJobHandles   sync.Map
	batchMu           sync.Mutex
)

func registerBatch(srv *sim.Server) {
	batchComputeEnvs = sim.MakeStore[BatchComputeEnvironment](srv.DB(), "batch_compute_envs")
	batchJobQueues = sim.MakeStore[BatchJobQueue](srv.DB(), "batch_job_queues")
	batchJobDefs = sim.MakeStore[BatchJobDefinition](srv.DB(), "batch_job_definitions")
	batchJobs = sim.MakeStore[BatchJob](srv.DB(), "batch_jobs")
	batchJobRevisions = sim.MakeStore[int](srv.DB(), "batch_job_revisions")

	batchResource := cloudTrailRESTResource("AWS::Batch::Resource", "resourceArn")
	// All Batch ops are POST to /v1/<lowercaseopname>
	srv.HandleFunc("POST /v1/createcomputeenvironment", cloudTrailRecordedREST("CreateComputeEnvironment", "batch.amazonaws.com", nil, handleBatchCreateComputeEnvironment))
	srv.HandleFunc("POST /v1/describecomputeenvironments", cloudTrailRecordedREST("DescribeComputeEnvironments", "batch.amazonaws.com", nil, handleBatchDescribeComputeEnvironments))
	srv.HandleFunc("POST /v1/updatecomputeenvironment", cloudTrailRecordedREST("UpdateComputeEnvironment", "batch.amazonaws.com", nil, handleBatchUpdateComputeEnvironment))
	srv.HandleFunc("POST /v1/deletecomputeenvironment", cloudTrailRecordedREST("DeleteComputeEnvironment", "batch.amazonaws.com", nil, handleBatchDeleteComputeEnvironment))

	srv.HandleFunc("POST /v1/createjobqueue", cloudTrailRecordedREST("CreateJobQueue", "batch.amazonaws.com", nil, handleBatchCreateJobQueue))
	srv.HandleFunc("POST /v1/describejobqueues", cloudTrailRecordedREST("DescribeJobQueues", "batch.amazonaws.com", nil, handleBatchDescribeJobQueues))
	srv.HandleFunc("POST /v1/updatejobqueue", cloudTrailRecordedREST("UpdateJobQueue", "batch.amazonaws.com", nil, handleBatchUpdateJobQueue))
	srv.HandleFunc("POST /v1/deletejobqueue", cloudTrailRecordedREST("DeleteJobQueue", "batch.amazonaws.com", nil, handleBatchDeleteJobQueue))

	srv.HandleFunc("POST /v1/registerjobdefinition", cloudTrailRecordedREST("RegisterJobDefinition", "batch.amazonaws.com", nil, handleBatchRegisterJobDefinition))
	srv.HandleFunc("POST /v1/describejobdefinitions", cloudTrailRecordedREST("DescribeJobDefinitions", "batch.amazonaws.com", nil, handleBatchDescribeJobDefinitions))
	srv.HandleFunc("POST /v1/deregisterjobdefinition", cloudTrailRecordedREST("DeregisterJobDefinition", "batch.amazonaws.com", nil, handleBatchDeregisterJobDefinition))

	srv.HandleFunc("POST /v1/submitjob", cloudTrailRecordedREST("SubmitJob", "batch.amazonaws.com", nil, handleBatchSubmitJob))
	srv.HandleFunc("POST /v1/describejobs", cloudTrailRecordedREST("DescribeJobs", "batch.amazonaws.com", nil, handleBatchDescribeJobs))
	srv.HandleFunc("POST /v1/listjobs", cloudTrailRecordedREST("ListJobs", "batch.amazonaws.com", nil, handleBatchListJobs))
	srv.HandleFunc("POST /v1/canceljob", cloudTrailRecordedREST("CancelJob", "batch.amazonaws.com", nil, handleBatchCancelJob))
	srv.HandleFunc("POST /v1/terminatejob", cloudTrailRecordedREST("TerminateJob", "batch.amazonaws.com", nil, handleBatchTerminateJob))

	// Resource-level tags
	srv.HandleFunc("GET /v1/tags/{resourceArn}", cloudTrailRecordedREST("ListTagsForResource", "batch.amazonaws.com", batchResource, handleBatchListTagsForResource))
	srv.HandleFunc("POST /v1/tags/{resourceArn}", cloudTrailRecordedREST("TagResource", "batch.amazonaws.com", batchResource, handleBatchTagResource))
	srv.HandleFunc("DELETE /v1/tags/{resourceArn}", cloudTrailRecordedREST("UntagResource", "batch.amazonaws.com", batchResource, handleBatchUntagResource))
}

func batchARN(resource string) string {
	return fmt.Sprintf("arn:aws:batch:us-east-1:123456789012:%s", resource)
}

func batchEpochMs() int64 {
	return time.Now().UnixMilli()
}

func batchWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func batchWriteError(w http.ResponseWriter, status int, msg string) {
	code := "ClientException"
	if status >= 500 {
		code = "ServerException"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Amzn-Errortype", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"__type":  code,
		"message": msg,
	})
}

// ---------- Compute Environments ----------

func handleBatchCreateComputeEnvironment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ComputeEnvironmentName string            `json:"computeEnvironmentName"`
		Type                   string            `json:"type"`
		State                  string            `json:"state"`
		ComputeResources       map[string]any    `json:"computeResources"`
		ServiceRole            string            `json:"serviceRole"`
		Tags                   map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		batchWriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.ComputeEnvironmentName == "" {
		batchWriteError(w, http.StatusBadRequest, "computeEnvironmentName is required")
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	if _, ok := batchComputeEnvs.Get(req.ComputeEnvironmentName); ok {
		batchWriteError(w, http.StatusBadRequest, "Compute environment already exists: "+req.ComputeEnvironmentName)
		return
	}
	state := req.State
	if state == "" {
		state = "ENABLED"
	}
	ceType := req.Type
	if ceType == "" {
		ceType = "MANAGED"
	}
	ce := BatchComputeEnvironment{
		ComputeEnvironmentName: req.ComputeEnvironmentName,
		ComputeEnvironmentArn:  batchARN("compute-environment/" + req.ComputeEnvironmentName),
		EcsClusterArn:          batchARN("cluster/" + req.ComputeEnvironmentName),
		State:                  state,
		Status:                 "VALID",
		Type:                   ceType,
		ComputeResources:       req.ComputeResources,
		ServiceRole:            req.ServiceRole,
		Tags:                   req.Tags,
	}
	batchComputeEnvs.Put(req.ComputeEnvironmentName, ce)
	batchWriteJSON(w, http.StatusOK, map[string]any{
		"computeEnvironmentArn":  ce.ComputeEnvironmentArn,
		"computeEnvironmentName": ce.ComputeEnvironmentName,
	})
}

func handleBatchDescribeComputeEnvironments(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ComputeEnvironments []string `json:"computeEnvironments"`
		MaxResults          *int32   `json:"maxResults"`
		NextToken           string   `json:"nextToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var result []BatchComputeEnvironment
	if len(req.ComputeEnvironments) > 0 {
		for _, nameOrARN := range req.ComputeEnvironments {
			name := batchNameFromARN(nameOrARN)
			if ce, ok := batchComputeEnvs.Get(name); ok {
				result = append(result, ce)
			}
		}
	} else {
		result = batchComputeEnvs.List()
		sort.Slice(result, func(i, j int) bool { return result[i].ComputeEnvironmentName < result[j].ComputeEnvironmentName })
	}
	if result == nil {
		result = []BatchComputeEnvironment{}
	}
	page, next := awsPageExplicit(result, req.NextToken, awsMaxResults(req.MaxResults))
	out := map[string]any{"computeEnvironments": page}
	if next != "" {
		out["nextToken"] = next
	}
	batchWriteJSON(w, http.StatusOK, out)
}

func handleBatchUpdateComputeEnvironment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ComputeEnvironment string         `json:"computeEnvironment"`
		State              string         `json:"state"`
		ComputeResources   map[string]any `json:"computeResources"`
		ServiceRole        string         `json:"serviceRole"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		batchWriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	name := batchNameFromARN(req.ComputeEnvironment)
	ce, ok := batchComputeEnvs.Get(name)
	if !ok {
		batchWriteError(w, http.StatusBadRequest, "Compute environment not found: "+name)
		return
	}
	if req.State != "" {
		ce.State = req.State
	}
	if req.ComputeResources != nil {
		ce.ComputeResources = req.ComputeResources
	}
	if req.ServiceRole != "" {
		ce.ServiceRole = req.ServiceRole
	}
	batchComputeEnvs.Put(name, ce)
	batchWriteJSON(w, http.StatusOK, map[string]any{
		"computeEnvironmentArn":  ce.ComputeEnvironmentArn,
		"computeEnvironmentName": ce.ComputeEnvironmentName,
	})
}

func handleBatchDeleteComputeEnvironment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ComputeEnvironment string `json:"computeEnvironment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		batchWriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	batchComputeEnvs.Delete(batchNameFromARN(req.ComputeEnvironment))
	batchWriteJSON(w, http.StatusOK, map[string]any{})
}

// ---------- Job Queues ----------

func handleBatchCreateJobQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobQueueName            string            `json:"jobQueueName"`
		State                   string            `json:"state"`
		Priority                int               `json:"priority"`
		ComputeEnvironmentOrder []map[string]any  `json:"computeEnvironmentOrder"`
		Tags                    map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		batchWriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.JobQueueName == "" {
		batchWriteError(w, http.StatusBadRequest, "jobQueueName is required")
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	if _, ok := batchJobQueues.Get(req.JobQueueName); ok {
		batchWriteError(w, http.StatusBadRequest, "Job queue already exists: "+req.JobQueueName)
		return
	}
	state := req.State
	if state == "" {
		state = "ENABLED"
	}
	ceOrder := req.ComputeEnvironmentOrder
	if ceOrder == nil {
		ceOrder = []map[string]any{}
	}
	q := BatchJobQueue{
		JobQueueName:            req.JobQueueName,
		JobQueueArn:             batchARN("job-queue/" + req.JobQueueName),
		State:                   state,
		Status:                  "VALID",
		Priority:                req.Priority,
		ComputeEnvironmentOrder: ceOrder,
		Tags:                    req.Tags,
	}
	batchJobQueues.Put(req.JobQueueName, q)
	batchWriteJSON(w, http.StatusOK, map[string]any{
		"jobQueueArn":  q.JobQueueArn,
		"jobQueueName": q.JobQueueName,
	})
}

func handleBatchDescribeJobQueues(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobQueues  []string `json:"jobQueues"`
		MaxResults *int32   `json:"maxResults"`
		NextToken  string   `json:"nextToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var result []BatchJobQueue
	if len(req.JobQueues) > 0 {
		for _, nameOrARN := range req.JobQueues {
			name := batchNameFromARN(nameOrARN)
			if q, ok := batchJobQueues.Get(name); ok {
				result = append(result, q)
			}
		}
	} else {
		result = batchJobQueues.List()
		sort.Slice(result, func(i, j int) bool { return result[i].JobQueueName < result[j].JobQueueName })
	}
	if result == nil {
		result = []BatchJobQueue{}
	}
	page, next := awsPageExplicit(result, req.NextToken, awsMaxResults(req.MaxResults))
	out := map[string]any{"jobQueues": page}
	if next != "" {
		out["nextToken"] = next
	}
	batchWriteJSON(w, http.StatusOK, out)
}

func handleBatchUpdateJobQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobQueue                string           `json:"jobQueue"`
		State                   string           `json:"state"`
		Priority                *int             `json:"priority"`
		ComputeEnvironmentOrder []map[string]any `json:"computeEnvironmentOrder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		batchWriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	name := batchNameFromARN(req.JobQueue)
	q, ok := batchJobQueues.Get(name)
	if !ok {
		batchWriteError(w, http.StatusBadRequest, "Job queue not found: "+req.JobQueue)
		return
	}
	if req.State != "" {
		q.State = req.State
	}
	if req.Priority != nil {
		q.Priority = *req.Priority
	}
	if req.ComputeEnvironmentOrder != nil {
		q.ComputeEnvironmentOrder = req.ComputeEnvironmentOrder
	}
	batchJobQueues.Put(name, q)
	batchWriteJSON(w, http.StatusOK, map[string]any{
		"jobQueueArn":  q.JobQueueArn,
		"jobQueueName": q.JobQueueName,
	})
}

func handleBatchDeleteJobQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobQueue string `json:"jobQueue"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		batchWriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	batchJobQueues.Delete(batchNameFromARN(req.JobQueue))
	batchWriteJSON(w, http.StatusOK, map[string]any{})
}

// ---------- Job Definitions ----------

func handleBatchRegisterJobDefinition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobDefinitionName   string            `json:"jobDefinitionName"`
		Type                string            `json:"type"`
		ContainerProperties map[string]any    `json:"containerProperties"`
		RetryStrategy       map[string]any    `json:"retryStrategy"`
		Timeout             map[string]any    `json:"timeout"`
		Tags                map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		batchWriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.JobDefinitionName == "" {
		batchWriteError(w, http.StatusBadRequest, "jobDefinitionName is required")
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	rev, _ := batchJobRevisions.Get(req.JobDefinitionName)
	rev++
	batchJobRevisions.Put(req.JobDefinitionName, rev)

	jobType := req.Type
	if jobType == "" {
		jobType = "container"
	}
	key := fmt.Sprintf("%s:%d", req.JobDefinitionName, rev)
	jd := BatchJobDefinition{
		JobDefinitionName:   req.JobDefinitionName,
		JobDefinitionArn:    batchARN(fmt.Sprintf("job-definition/%s:%d", req.JobDefinitionName, rev)),
		Revision:            rev,
		Status:              "ACTIVE",
		Type:                jobType,
		ContainerProperties: req.ContainerProperties,
		RetryStrategy:       req.RetryStrategy,
		Timeout:             req.Timeout,
		Tags:                req.Tags,
	}
	batchJobDefs.Put(key, jd)
	batchWriteJSON(w, http.StatusOK, map[string]any{
		"jobDefinitionArn":  jd.JobDefinitionArn,
		"jobDefinitionName": jd.JobDefinitionName,
		"revision":          jd.Revision,
	})
}

func handleBatchDescribeJobDefinitions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobDefinitionName string   `json:"jobDefinitionName"`
		JobDefinitions    []string `json:"jobDefinitions"`
		Status            string   `json:"status"`
		MaxResults        *int32   `json:"maxResults"`
		NextToken         string   `json:"nextToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	all := batchJobDefs.List()
	var result []BatchJobDefinition
	for _, jd := range all {
		if req.JobDefinitionName != "" && jd.JobDefinitionName != req.JobDefinitionName {
			continue
		}
		if req.Status != "" && jd.Status != req.Status {
			continue
		}
		if len(req.JobDefinitions) > 0 {
			nameRev := fmt.Sprintf("%s:%d", jd.JobDefinitionName, jd.Revision)
			matched := false
			for _, want := range req.JobDefinitions {
				if want == jd.JobDefinitionArn || want == nameRev || want == jd.JobDefinitionName {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		result = append(result, jd)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].JobDefinitionArn < result[j].JobDefinitionArn })
	if result == nil {
		result = []BatchJobDefinition{}
	}
	page, next := awsPageExplicit(result, req.NextToken, awsMaxResults(req.MaxResults))
	out := map[string]any{"jobDefinitions": page}
	if next != "" {
		out["nextToken"] = next
	}
	batchWriteJSON(w, http.StatusOK, out)
}

func handleBatchDeregisterJobDefinition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobDefinition string `json:"jobDefinition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		batchWriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	// req.JobDefinition may be "name:rev" or an ARN — look up by key suffix
	key := batchJobDefKey(req.JobDefinition)
	if jd, ok := batchJobDefs.Get(key); ok {
		jd.Status = "INACTIVE"
		batchJobDefs.Put(key, jd)
	}
	batchWriteJSON(w, http.StatusOK, map[string]any{})
}

// ---------- Jobs ----------

func handleBatchSubmitJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobName            string            `json:"jobName"`
		JobQueue           string            `json:"jobQueue"`
		JobDefinition      string            `json:"jobDefinition"`
		ContainerOverrides map[string]any    `json:"containerOverrides"`
		Tags               map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		batchWriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.JobName == "" || req.JobQueue == "" || req.JobDefinition == "" {
		batchWriteError(w, http.StatusBadRequest, "jobName, jobQueue, and jobDefinition are required")
		return
	}

	queueName := batchNameFromARN(req.JobQueue)
	if _, ok := batchJobQueues.Get(queueName); !ok {
		batchWriteError(w, http.StatusBadRequest, "Job queue not found: "+req.JobQueue)
		return
	}
	jd, ok := batchLookupJobDefinition(req.JobDefinition)
	if !ok {
		batchWriteError(w, http.StatusBadRequest, "Job definition not found: "+req.JobDefinition)
		return
	}
	if jd.Status != "ACTIVE" {
		batchWriteError(w, http.StatusBadRequest, "Job definition is not active: "+req.JobDefinition)
		return
	}
	cfg, containerMeta, err := batchContainerConfig(jd, req.ContainerOverrides)
	if err != nil {
		batchWriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	batchMu.Lock()
	defer batchMu.Unlock()

	jobID := uuid.New().String()
	now := batchEpochMs()
	cfg.Name = "sockerless-batch-" + jobID
	cfg.Labels = map[string]string{"aws-batch-job-id": jobID}
	job := BatchJob{
		JobID:         jobID,
		JobArn:        batchARN("job/" + jobID),
		JobName:       req.JobName,
		JobQueue:      req.JobQueue,
		Status:        "SUBMITTED",
		JobDefinition: jd.JobDefinitionArn,
		CreatedAt:     now,
		StartedAt:     now,
		Container:     containerMeta,
		Tags:          req.Tags,
	}
	batchJobs.Put(jobID, job)

	handle, err := sim.StartContainerSync(cfg, sim.NoopSink{})
	if err != nil {
		job.Status = "FAILED"
		job.StatusReason = err.Error()
		job.StoppedAt = batchEpochMs()
		job.Container["reason"] = err.Error()
		batchJobs.Put(jobID, job)
	} else {
		job.Container["containerInstanceArn"] = batchARN("container/" + handle.ContainerID)
		batchJobs.Put(jobID, job)
		batchJobHandles.Store(jobID, handle)
		go batchRunJobLifecycle(jobID, handle)
	}
	batchWriteJSON(w, http.StatusOK, map[string]any{
		"jobId":   jobID,
		"jobName": req.JobName,
		"jobArn":  batchARN("job/" + jobID),
	})
}

func batchTerminal(status string) bool {
	return status == "SUCCEEDED" || status == "FAILED"
}

// batchRunJobLifecycle drives the real Batch job state machine
// SUBMITTED→PENDING→RUNNABLE→STARTING→RUNNING (sub-second dwells, real states,
// no synthetic timer) before delegating to batchWaitForJob for the terminal
// transition driven by the real container exit. A job whose container has
// already finished is not regressed back into a running state.
func batchRunJobLifecycle(jobID string, handle *sim.ContainerHandle) {
	for _, st := range []string{"PENDING", "RUNNABLE", "STARTING", "RUNNING"} {
		time.Sleep(40 * time.Millisecond)
		batchMu.Lock()
		job, ok := batchJobs.Get(jobID)
		if !ok || batchTerminal(job.Status) {
			batchMu.Unlock()
			break
		}
		job.Status = st
		if st == "RUNNING" {
			job.StartedAt = batchEpochMs()
		}
		batchJobs.Put(jobID, job)
		batchMu.Unlock()
	}
	batchWaitForJob(jobID, handle)
}

func batchWaitForJob(jobID string, handle *sim.ContainerHandle) {
	result := handle.Wait()
	batchJobHandles.Delete(jobID)

	batchMu.Lock()
	defer batchMu.Unlock()

	job, ok := batchJobs.Get(jobID)
	if !ok {
		return
	}
	if job.Status == "FAILED" && job.StoppedAt > 0 {
		return
	}
	if result.ExitCode == 0 && result.Error == nil {
		job.Status = "SUCCEEDED"
	} else {
		job.Status = "FAILED"
		if result.Error != nil {
			job.StatusReason = result.Error.Error()
		} else {
			job.StatusReason = fmt.Sprintf("Container exited with status %d", result.ExitCode)
		}
	}
	job.StoppedAt = result.StoppedAt.UnixMilli()
	if job.Container == nil {
		job.Container = map[string]any{}
	}
	job.Container["exitCode"] = result.ExitCode
	if result.Error != nil {
		job.Container["reason"] = result.Error.Error()
	}
	batchJobs.Put(jobID, job)
}

func handleBatchDescribeJobs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Jobs []string `json:"jobs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		batchWriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	var result []BatchJob
	for _, id := range req.Jobs {
		if job, ok := batchJobs.Get(id); ok {
			result = append(result, job)
		}
	}
	if result == nil {
		result = []BatchJob{}
	}
	batchWriteJSON(w, http.StatusOK, map[string]any{"jobs": result})
}

func handleBatchListJobs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobQueue   string `json:"jobQueue"`
		JobStatus  string `json:"jobStatus"`
		MaxResults *int32 `json:"maxResults"`
		NextToken  string `json:"nextToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	all := batchJobs.List()
	var result []map[string]any
	for _, job := range all {
		if req.JobQueue != "" && job.JobQueue != req.JobQueue {
			continue
		}
		if req.JobStatus != "" && job.Status != req.JobStatus {
			continue
		}
		result = append(result, map[string]any{
			"jobId":   job.JobID,
			"jobName": job.JobName,
			"jobArn":  batchARN("job/" + job.JobID),
			"status":  job.Status,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i]["jobId"].(string) < result[j]["jobId"].(string)
	})
	if result == nil {
		result = []map[string]any{}
	}
	page, next := awsPageExplicit(result, req.NextToken, awsMaxResults(req.MaxResults))
	out := map[string]any{"jobSummaryList": page}
	if next != "" {
		out["nextToken"] = next
	}
	batchWriteJSON(w, http.StatusOK, out)
}

func handleBatchCancelJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		JobID  string `json:"jobId"`
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	batchMu.Lock()
	defer batchMu.Unlock()

	job, ok := batchJobs.Get(req.JobID)
	if !ok {
		batchWriteJSON(w, http.StatusOK, map[string]any{})
		return
	}
	if job.Status != "SUCCEEDED" && job.Status != "FAILED" {
		if handleAny, ok := batchJobHandles.Load(req.JobID); ok {
			handleAny.(*sim.ContainerHandle).Cancel()
			batchJobHandles.Delete(req.JobID)
		}
		job.Status = "FAILED"
		job.StatusReason = req.Reason
		job.StoppedAt = batchEpochMs()
		batchJobs.Put(req.JobID, job)
	}
	batchWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleBatchTerminateJob(w http.ResponseWriter, r *http.Request) {
	// Same as cancel for simulator purposes
	handleBatchCancelJob(w, r)
}

// ---------- Tags (resource-level) ----------

func handleBatchListTagsForResource(w http.ResponseWriter, r *http.Request) {
	arn := r.PathValue("resourceArn")
	tags := batchTagsForARN(arn)
	batchWriteJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func handleBatchTagResource(w http.ResponseWriter, r *http.Request) {
	arn := r.PathValue("resourceArn")
	var req struct {
		Tags map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		batchWriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	batchMu.Lock()
	defer batchMu.Unlock()
	batchApplyTags(arn, req.Tags)
	batchWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleBatchUntagResource(w http.ResponseWriter, r *http.Request) {
	arn := r.PathValue("resourceArn")
	keys := r.URL.Query()["tagKeys"]
	batchMu.Lock()
	defer batchMu.Unlock()
	batchRemoveTags(arn, keys)
	batchWriteJSON(w, http.StatusOK, map[string]any{})
}

func batchTagsForARN(arn string) map[string]string {
	if strings.Contains(arn, ":compute-environment/") {
		name := batchNameFromARN(arn)
		if ce, ok := batchComputeEnvs.Get(name); ok && ce.Tags != nil {
			return ce.Tags
		}
	} else if strings.Contains(arn, ":job-queue/") {
		name := batchNameFromARN(arn)
		if q, ok := batchJobQueues.Get(name); ok && q.Tags != nil {
			return q.Tags
		}
	}
	return map[string]string{}
}

func batchApplyTags(arn string, tags map[string]string) {
	if strings.Contains(arn, ":compute-environment/") {
		name := batchNameFromARN(arn)
		if ce, ok := batchComputeEnvs.Get(name); ok {
			if ce.Tags == nil {
				ce.Tags = make(map[string]string)
			}
			for k, v := range tags {
				ce.Tags[k] = v
			}
			batchComputeEnvs.Put(name, ce)
		}
	} else if strings.Contains(arn, ":job-queue/") {
		name := batchNameFromARN(arn)
		if q, ok := batchJobQueues.Get(name); ok {
			if q.Tags == nil {
				q.Tags = make(map[string]string)
			}
			for k, v := range tags {
				q.Tags[k] = v
			}
			batchJobQueues.Put(name, q)
		}
	}
}

func batchRemoveTags(arn string, keys []string) {
	if strings.Contains(arn, ":compute-environment/") {
		name := batchNameFromARN(arn)
		if ce, ok := batchComputeEnvs.Get(name); ok {
			for _, k := range keys {
				delete(ce.Tags, k)
			}
			batchComputeEnvs.Put(name, ce)
		}
	} else if strings.Contains(arn, ":job-queue/") {
		name := batchNameFromARN(arn)
		if q, ok := batchJobQueues.Get(name); ok {
			for _, k := range keys {
				delete(q.Tags, k)
			}
			batchJobQueues.Put(name, q)
		}
	}
}

func batchNameFromARN(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return arn
}

func batchJobDefKey(nameOrARN string) string {
	// Accept "name:rev" or ARN ending with "name:rev"
	parts := strings.Split(nameOrARN, "/")
	return parts[len(parts)-1]
}

func batchLookupJobDefinition(nameOrARN string) (BatchJobDefinition, bool) {
	key := batchJobDefKey(nameOrARN)
	if strings.Contains(key, ":") {
		return batchJobDefs.Get(key)
	}
	rev, ok := batchJobRevisions.Get(key)
	if !ok {
		return BatchJobDefinition{}, false
	}
	for rev > 0 {
		if jd, ok := batchJobDefs.Get(fmt.Sprintf("%s:%d", key, rev)); ok && jd.Status == "ACTIVE" {
			return jd, true
		}
		rev--
	}
	return BatchJobDefinition{}, false
}

func batchContainerConfig(jd BatchJobDefinition, overrides map[string]any) (sim.ContainerConfig, map[string]any, error) {
	image := batchString(jd.ContainerProperties["image"])
	if image == "" {
		return sim.ContainerConfig{}, nil, fmt.Errorf("containerProperties.image is required")
	}
	command := batchStringSlice(jd.ContainerProperties["command"])
	env := batchEnvironment(jd.ContainerProperties["environment"])
	if overrides != nil {
		if overrideCommand := batchStringSlice(overrides["command"]); len(overrideCommand) > 0 {
			command = overrideCommand
		}
		for k, v := range batchEnvironment(overrides["environment"]) {
			env[k] = v
		}
	}
	timeout := batchTimeout(jd.Timeout)
	meta := map[string]any{
		"image": image,
	}
	if len(command) > 0 {
		meta["command"] = command
	}
	if len(env) > 0 {
		meta["environment"] = batchEnvironmentList(env)
	}
	return sim.ContainerConfig{
		Image:        sim.ResolveLocalImage(image),
		Architecture: "linux/" + runtime.GOARCH,
		Args:         command,
		Env:          env,
		Timeout:      timeout,
		Sandbox:      sim.SandboxFargate,
	}, meta, nil
}

func batchString(v any) string {
	s, _ := v.(string)
	return s
}

func batchStringSlice(v any) []string {
	switch values := v.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if s, ok := value.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func batchEnvironment(v any) map[string]string {
	env := map[string]string{}
	values, ok := v.([]any)
	if !ok {
		return env
	}
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		name := batchString(item["name"])
		if name == "" {
			name = batchString(item["Name"])
		}
		if name == "" {
			continue
		}
		env[name] = batchString(item["value"])
		if env[name] == "" {
			env[name] = batchString(item["Value"])
		}
	}
	return env
}

func batchEnvironmentList(env map[string]string) []map[string]string {
	out := make([]map[string]string, 0, len(env))
	for k, v := range env {
		out = append(out, map[string]string{"name": k, "value": v})
	}
	return out
}

func batchTimeout(timeout map[string]any) time.Duration {
	seconds, ok := timeout["attemptDurationSeconds"].(float64)
	if !ok || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
