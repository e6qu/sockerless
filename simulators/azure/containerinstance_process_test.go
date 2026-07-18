package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	sim "github.com/sockerless/simulator"
)

func TestACIProcessRuntimeRejectsWorkloadExecution(t *testing.T) {
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := buildSimulator(sim.Config{Provider: "azure", ListenAddr: ":0", LogLevel: "error"})
	if err != nil {
		t.Fatalf("build simulator: %v", err)
	}

	request := httptest.NewRequest(http.MethodPut,
		"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.ContainerInstance/containerGroups/workload?api-version=2021-10-01",
		bytes.NewBufferString(`{"location":"westeurope","properties":{"containers":[{"name":"workload","properties":{"image":"alpine:3.20"}}],"osType":"Linux"}}`))
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}
