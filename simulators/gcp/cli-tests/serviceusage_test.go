package gcp_cli_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func serviceURL(service string) string {
	return fmt.Sprintf("%s/v1/projects/%s/services/%s", baseURL, project, service)
}

func TestServiceUsage_EnableAndList(t *testing.T) {
	// Enable a service
	enableURL := serviceURL("compute.googleapis.com:enable")
	httpDoJSON(t, "POST", enableURL, `{}`)

	// Get service to verify it's enabled
	getURL := serviceURL("compute.googleapis.com")
	out := httpDoJSON(t, "GET", getURL, "")

	var svc struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	parseJSON(t, out, &svc)
	assert.Equal(t, "ENABLED", svc.State)

	// List services
	listURL := fmt.Sprintf("%s/v1/projects/%s/services", baseURL, project)
	out = httpDoJSON(t, "GET", listURL, "")

	var result struct {
		Services []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"services"`
	}
	parseJSON(t, out, &result)
	assert.NotEmpty(t, result.Services)
}

func TestServiceUsage_Disable(t *testing.T) {
	// Enable first
	enableURL := serviceURL("storage.googleapis.com:enable")
	httpDoJSON(t, "POST", enableURL, `{}`)

	// Disable
	disableURL := serviceURL("storage.googleapis.com:disable")
	httpDoJSON(t, "POST", disableURL, `{}`)

	// Verify it's disabled
	getURL := serviceURL("storage.googleapis.com")
	out := httpDoJSON(t, "GET", getURL, "")

	var svc struct {
		State string `json:"state"`
	}
	parseJSON(t, out, &svc)
	assert.Equal(t, "DISABLED", svc.State)
}
