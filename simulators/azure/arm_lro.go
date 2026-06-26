package main

import (
	"fmt"
	"net/http"
	"time"

	sim "github.com/sockerless/simulator"
)

var azureAsyncOps sim.Store[AsyncOperationStatus]

func registerAzureAsyncOperations(srv *sim.Server) {
	azureAsyncOps = sim.MakeStore[AsyncOperationStatus](srv.DB(), "azure_async_ops")
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/providers/{provider}/locations/{location}/operationStatuses/{opId}", handleAzureAsyncOperationStatus)
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/providers/{provider}/locations/{location}/operationResults/{opId}", handleAzureAsyncOperationStatus)
}

func issueAzureAsyncOperation(complete func()) string {
	opID := generateUUID()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	azureAsyncOps.Put(opID, AsyncOperationStatus{
		Name:      opID,
		Status:    "InProgress",
		StartTime: now,
	})
	go func() {
		time.Sleep(50 * time.Millisecond)
		if complete != nil {
			complete()
		}
		azureAsyncOps.Update(opID, func(op *AsyncOperationStatus) {
			op.Status = "Succeeded"
			op.EndTime = time.Now().UTC().Format(time.RFC3339Nano)
		})
	}()
	return opID
}

func azureAsyncOperationHeader(r *http.Request, sub, provider, location, kind, opID, apiVersion string) string {
	scheme := azureRequestScheme(r)
	if apiVersion == "" {
		apiVersion = "2024-01-01"
	}
	if kind == "" {
		kind = "operationStatuses"
	}
	return fmt.Sprintf("%s://%s/subscriptions/%s/providers/%s/locations/%s/%s/%s?api-version=%s",
		scheme, r.Host, sub, provider, location, kind, opID, apiVersion)
}

func azureCurrentRequestURL(r *http.Request) string {
	return fmt.Sprintf("%s://%s%s", azureRequestScheme(r), r.Host, r.URL.RequestURI())
}

func writeAzureAsyncCreateHeaders(w http.ResponseWriter, opURL, locationURL string) {
	w.Header().Set("Azure-AsyncOperation", opURL)
	w.Header().Set("Location", locationURL)
	w.Header().Set("Retry-After", "0")
}

func handleAzureAsyncOperationStatus(w http.ResponseWriter, r *http.Request) {
	opID := sim.PathParam(r, "opId")
	op, ok := azureAsyncOps.Get(opID)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "Operation %q not found.", opID)
		return
	}
	// Real Azure returns a Retry-After on an in-progress operation poll; advertise
	// a short one so the SDK poller re-polls promptly. Without it azcore falls
	// back to its 30s default frequency (a Retry-After of 0 is ignored), which
	// would make each long-running operation in a test take 30s.
	if op.Status == "InProgress" {
		w.Header().Set("Retry-After", "1")
	}
	sim.WriteJSON(w, http.StatusOK, op)
}
