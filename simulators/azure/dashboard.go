package main

import (
	"net/http"

	sim "github.com/sockerless/simulator"
)

func registerDashboard(srv *sim.Server) {
	srv.HandleFunc("GET /sim/v1/summary", handleDashboardSummary)
	srv.HandleFunc("GET /sim/v1/container-apps/jobs", handleDashboardCAJobs)
	srv.HandleFunc("GET /sim/v1/functions/sites", handleDashboardFunctionSites)
	srv.HandleFunc("GET /sim/v1/acr/registries", handleDashboardACRRegistries)
	srv.HandleFunc("GET /sim/v1/storage/accounts", handleDashboardStorageAccounts)
	srv.HandleFunc("GET /sim/v1/monitor/logs", handleDashboardMonitorLogs)
}

func handleDashboardSummary(w http.ResponseWriter, _ *http.Request) {
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"provider": "azure",
		"services": map[string]int{
			"container_app_jobs": acaJobs.Len(),
			"function_sites":    azfSites.Len(),
			"acr_registries":    acrRegistries.Len(),
			"storage_accounts":  azStorageAccounts.Len(),
			"monitor_logs":      monitorLogs.Len(),
		},
	})
}

func handleDashboardCAJobs(w http.ResponseWriter, _ *http.Request) {
	type jobSummary struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Location string `json:"location"`
		Type     string `json:"type"`
	}
	jobs := acaJobs.List()
	out := make([]jobSummary, len(jobs))
	for i, j := range jobs {
		out[i] = jobSummary{
			ID:       j.ID,
			Name:     j.Name,
			Location: j.Location,
			Type:     j.Type,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardFunctionSites(w http.ResponseWriter, _ *http.Request) {
	type siteSummary struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Location string `json:"location"`
		Kind     string `json:"kind"`
	}
	sites := azfSites.List()
	out := make([]siteSummary, len(sites))
	for i, s := range sites {
		out[i] = siteSummary{
			ID:       s.ID,
			Name:     s.Name,
			Location: s.Location,
			Kind:     s.Kind,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardACRRegistries(w http.ResponseWriter, _ *http.Request) {
	type registrySummary struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Location string `json:"location"`
	}
	regs := acrRegistries.List()
	out := make([]registrySummary, len(regs))
	for i, r := range regs {
		out[i] = registrySummary{
			ID:       r.ID,
			Name:     r.Name,
			Location: r.Location,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardStorageAccounts(w http.ResponseWriter, _ *http.Request) {
	type storageSummary struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Location string `json:"location"`
		Kind     string `json:"kind"`
	}
	accts := azStorageAccounts.List()
	out := make([]storageSummary, len(accts))
	for i, a := range accts {
		out[i] = storageSummary{
			ID:       a.ID,
			Name:     a.Name,
			Location: a.Location,
			Kind:     a.Kind,
		}
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleDashboardMonitorLogs(w http.ResponseWriter, _ *http.Request) {
	// Flatten all log rows (keyed by "workspaceID:tableName") into a single list, limited to 100.
	var out []monitorLogRow
	for _, rows := range monitorLogs.List() {
		for _, row := range rows {
			out = append(out, row)
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
