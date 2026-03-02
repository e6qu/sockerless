package main

import (
	"net/http"

	sim "github.com/sockerless/simulator"
)

func registerDashboard(srv *sim.Server) {
	srv.HandleFunc("GET /sim/v1/summary", handleDashboardSummary)
	srv.HandleFunc("GET /sim/v1/cloudrun/jobs", handleDashboardCloudRunJobs)
	srv.HandleFunc("GET /sim/v1/functions", handleDashboardFunctions)
	srv.HandleFunc("GET /sim/v1/ar/repositories", handleDashboardARRepos)
	srv.HandleFunc("GET /sim/v1/gcs/buckets", handleDashboardGCSBuckets)
	srv.HandleFunc("GET /sim/v1/logging/entries", handleDashboardLogEntries)
}

func handleDashboardSummary(w http.ResponseWriter, _ *http.Request) {
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"provider": "gcp",
		"services": map[string]int{
			"cloudrun_jobs": crjJobs.Len(),
			"functions":     gcfFunctions.Len(),
			"ar_repos":      arRepos.Len(),
			"gcs_buckets":   gcsBuckets.Len(),
			"log_entries":   logEntries.Len(),
		},
	})
}

func handleDashboardCloudRunJobs(w http.ResponseWriter, _ *http.Request) {
	type jobSummary struct {
		Name           string `json:"name"`
		CreateTime     string `json:"createTime"`
		ExecutionCount int32  `json:"executionCount"`
		LaunchStage    string `json:"launchStage"`
	}
	jobs := crjJobs.List()
	out := make([]jobSummary, len(jobs))
	for i, j := range jobs {
		out[i] = jobSummary{
			Name:           j.Name,
			CreateTime:     j.CreateTime,
			ExecutionCount: j.ExecutionCount,
			LaunchStage:    j.LaunchStage,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardFunctions(w http.ResponseWriter, _ *http.Request) {
	type fnSummary struct {
		Name        string `json:"name"`
		State       string `json:"state"`
		Environment string `json:"environment"`
		CreateTime  string `json:"createTime"`
	}
	fns := gcfFunctions.List()
	out := make([]fnSummary, len(fns))
	for i, f := range fns {
		out[i] = fnSummary{
			Name:        f.Name,
			State:       f.State,
			Environment: f.Environment,
			CreateTime:  f.CreateTime,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardARRepos(w http.ResponseWriter, _ *http.Request) {
	type repoSummary struct {
		Name       string `json:"name"`
		Format     string `json:"format"`
		CreateTime string `json:"createTime"`
	}
	repos := arRepos.List()
	out := make([]repoSummary, len(repos))
	for i, r := range repos {
		out[i] = repoSummary{
			Name:       r.Name,
			Format:     r.Format,
			CreateTime: r.CreateTime,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardGCSBuckets(w http.ResponseWriter, _ *http.Request) {
	type bucketSummary struct {
		Name string         `json:"name"`
		Data map[string]any `json:"data"`
	}
	bkts := gcsBuckets.List()
	out := make([]bucketSummary, len(bkts))
	for i, b := range bkts {
		out[i] = bucketSummary{
			Name: b.Data["name"].(string),
			Data: b.Data,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardLogEntries(w http.ResponseWriter, _ *http.Request) {
	type entrySummary struct {
		LogName     string `json:"logName"`
		Timestamp   string `json:"timestamp"`
		Severity    string `json:"severity"`
		TextPayload string `json:"textPayload,omitempty"`
	}

	// Flatten all log entry slices (keyed by log name) into a single list, limited to 100.
	var out []entrySummary
	for _, entries := range logEntries.List() {
		for _, e := range entries {
			out = append(out, entrySummary{
				LogName:     e.LogName,
				Timestamp:   e.Timestamp,
				Severity:    e.Severity,
				TextPayload: e.TextPayload,
			})
			if len(out) >= 100 {
				break
			}
		}
		if len(out) >= 100 {
			break
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}
