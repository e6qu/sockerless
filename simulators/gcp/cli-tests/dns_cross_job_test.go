package gcp_cli_test

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDNS_CrossJobResolution_CLI mirrors the SDK cross-job DNS test
// through the gcloud CLI. Private zone + two Cloud Run Jobs + A records
// pointing at the jobs' Docker IPs — one job resolves the other by
// short hostname via Docker's embedded DNS on the zone's backing
// network.
func TestDNS_CrossJobResolution_CLI(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI required for cross-job DNS test")
	}

	// 1. Create the private zone via gcloud.
	runCLI(t, gcloudCLI("dns", "managed-zones", "create", "cli-xjob-zone",
		"--dns-name=cli-xjob.local.",
		"--description=CLI cross-job DNS test",
		"--visibility=private",
		"--networks=",
	))
	defer runCLI(t, gcloudCLI("dns", "managed-zones", "delete", "cli-xjob-zone"))

	// Get zone ID so we can verify the Docker network exists.
	out := runCLI(t, gcloudCLI("dns", "managed-zones", "describe", "cli-xjob-zone", "--format=json"))
	var zone struct {
		Id         string `json:"id"`
		Visibility string `json:"visibility"`
	}
	parseJSON(t, out, &zone)
	require.Equal(t, "private", zone.Visibility)

	// 2. Create + run two Cloud Run Jobs via direct HTTP (gcloud run
	// jobs create against the simulator's v2 endpoint is not reliably
	// supported; the SDK/REST path is the gcloud back-door).
	createJob := func(name string) string {
		body := `{
			"template":{"template":{
				"containers":[{"image":"alpine:latest","command":["sh","-c"],"args":["sleep 30"]}],
				"timeout":"60s"
			}}
		}`
		createURL := fmt.Sprintf("%s/v2/projects/%s/locations/%s/jobs?jobId=%s",
			baseURL, project, location, name)
		_ = httpDoJSON(t, "POST", createURL, body)

		runURL := fmt.Sprintf("%s/v2/projects/%s/locations/%s/jobs/%s:run",
			baseURL, project, location, name)
		runOut := httpDoJSON(t, "POST", runURL, "{}")
		// The response is an LRO with an embedded Execution whose
		// `name` is projects/.../executions/<execID>.
		var op struct {
			Response struct {
				Name string `json:"name"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(runOut), &op); err == nil && op.Response.Name != "" {
			return op.Response.Name
		}
		// Fallback: RunJob returns the Execution directly on success.
		var execResp struct {
			Name string `json:"name"`
		}
		parseJSON(t, runOut, &execResp)
		return execResp.Name
	}

	alphaExec := createJob("cli-alpha")
	betaExec := createJob("cli-beta")

	alphaContainer := jobContainerName(alphaExec)
	betaContainer := jobContainerName(betaExec)

	require.Eventually(t, func() bool {
		for _, n := range []string{alphaContainer, betaContainer} {
			if err := exec.Command("docker", "inspect", n).Run(); err != nil {
				return false
			}
		}
		return true
	}, 30*time.Second, 300*time.Millisecond, "Docker containers should be running (alpha=%s, beta=%s)", alphaContainer, betaContainer)

	alphaIP := containerIP(t, alphaContainer)
	betaIP := containerIP(t, betaContainer)
	require.NotEmpty(t, alphaIP)
	require.NotEmpty(t, betaIP)

	// 3. Create A records (direct REST — gcloud record-sets create has
	// inconsistent endpoint-override handling).
	rrURL := fmt.Sprintf("%s/dns/v1/projects/%s/managedZones/cli-xjob-zone/rrsets", baseURL, project)
	httpDoJSON(t, "POST", rrURL, fmt.Sprintf(
		`{"name":"alpha.cli-xjob.local.","type":"A","ttl":60,"rrdatas":[%q]}`, alphaIP))
	httpDoJSON(t, "POST", rrURL, fmt.Sprintf(
		`{"name":"beta.cli-xjob.local.","type":"A","ttl":60,"rrdatas":[%q]}`, betaIP))

	// 4. Cross-job DNS: alpha resolves "beta".
	var getent []byte
	require.Eventually(t, func() bool {
		var err error
		getent, err = exec.Command("docker", "exec", alphaContainer, "getent", "hosts", "beta").CombinedOutput()
		return err == nil && len(getent) > 0
	}, 10*time.Second, 500*time.Millisecond, "alpha should resolve 'beta' via private zone: %s", getent)
	assert.Contains(t, string(getent), "beta", "getent output should mention beta: %s", getent)
}

// jobContainerName + containerIP mirror the SDK-test helpers.
func jobContainerName(executionName string) string {
	last := executionName
	if idx := strings.LastIndex(executionName, "/"); idx >= 0 {
		last = executionName[idx+1:]
	}
	if len(last) > 12 {
		last = last[:12]
	}
	return "sockerless-sim-gcp-job-" + last
}

func containerIP(t *testing.T, name string) string {
	t.Helper()
	out, err := exec.Command("docker", "inspect", name).Output()
	require.NoError(t, err)
	var inspected []struct {
		NetworkSettings struct {
			Networks map[string]struct {
				IPAddress string `json:"IPAddress"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	require.NoError(t, json.Unmarshal(out, &inspected))
	require.NotEmpty(t, inspected)
	for _, net := range inspected[0].NetworkSettings.Networks {
		if net.IPAddress != "" {
			return net.IPAddress
		}
	}
	return ""
}
