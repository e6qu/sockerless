package main

import (
	"net/http"

	sim "github.com/sockerless/simulator"
)

func registerDashboard(srv *sim.Server) {
	srv.HandleFunc("GET /sim/v1/summary", handleDashboardSummary)
	srv.HandleFunc("GET /sim/v1/ecs/tasks", handleDashboardECSTasks)
	srv.HandleFunc("GET /sim/v1/lambda/functions", handleDashboardLambdaFunctions)
	srv.HandleFunc("GET /sim/v1/ecr/repositories", handleDashboardECRRepos)
	srv.HandleFunc("GET /sim/v1/s3/buckets", handleDashboardS3Buckets)
	srv.HandleFunc("GET /sim/v1/cloudwatch/log-groups", handleDashboardCWLogGroups)
}

func handleDashboardSummary(w http.ResponseWriter, _ *http.Request) {
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"provider": "aws",
		"services": map[string]int{
			"ecs_tasks":        ecsTasks.Len(),
			"lambda_functions": lambdaFunctions.Len(),
			"ecr_repositories": ecrRepositories.Len(),
			"s3_buckets":       s3Buckets_.Len(),
			"cw_log_groups":    cwLogGroups.Len(),
		},
	})
}

func handleDashboardECSTasks(w http.ResponseWriter, _ *http.Request) {
	type taskSummary struct {
		TaskArn    string `json:"taskArn"`
		Status     string `json:"status"`
		Cluster    string `json:"clusterArn"`
		LaunchType string `json:"launchType"`
		Cpu        string `json:"cpu"`
		Memory     string `json:"memory"`
	}
	tasks := ecsTasks.List()
	out := make([]taskSummary, len(tasks))
	for i, t := range tasks {
		out[i] = taskSummary{
			TaskArn:    t.TaskArn,
			Status:     t.LastStatus,
			Cluster:    t.ClusterArn,
			LaunchType: t.LaunchType,
			Cpu:        t.Cpu,
			Memory:     t.Memory,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardLambdaFunctions(w http.ResponseWriter, _ *http.Request) {
	type fnSummary struct {
		Name         string `json:"name"`
		Runtime      string `json:"runtime"`
		State        string `json:"state"`
		MemorySize   int    `json:"memorySize"`
		Timeout      int    `json:"timeout"`
		LastModified string `json:"lastModified"`
	}
	fns := lambdaFunctions.List()
	out := make([]fnSummary, len(fns))
	for i, f := range fns {
		out[i] = fnSummary{
			Name:         f.FunctionName,
			Runtime:      f.Runtime,
			State:        f.State,
			MemorySize:   f.MemorySize,
			Timeout:      f.Timeout,
			LastModified: f.LastModified,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardECRRepos(w http.ResponseWriter, _ *http.Request) {
	type repoSummary struct {
		Name      string `json:"name"`
		URI       string `json:"uri"`
		CreatedAt int64  `json:"createdAt"`
	}
	repos := ecrRepositories.List()
	out := make([]repoSummary, len(repos))
	for i, r := range repos {
		out[i] = repoSummary{
			Name:      r.RepositoryName,
			URI:       r.RepositoryUri,
			CreatedAt: r.CreatedAt,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardS3Buckets(w http.ResponseWriter, _ *http.Request) {
	type bucketSummary struct {
		Name         string `json:"name"`
		CreationDate string `json:"creationDate"`
	}
	buckets := s3Buckets_.List()
	out := make([]bucketSummary, len(buckets))
	for i, b := range buckets {
		out[i] = bucketSummary(b)
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardCWLogGroups(w http.ResponseWriter, _ *http.Request) {
	type logGroupSummary struct {
		Name         string `json:"name"`
		CreationTime int64  `json:"creationTime"`
		Retention    int    `json:"retentionInDays"`
		StoredBytes  int64  `json:"storedBytes"`
	}
	groups := cwLogGroups.List()
	out := make([]logGroupSummary, len(groups))
	for i, g := range groups {
		out[i] = logGroupSummary{
			Name:         g.LogGroupName,
			CreationTime: g.CreationTime,
			Retention:    g.RetentionInDays,
			StoredBytes:  g.StoredBytes,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}
