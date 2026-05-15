package aws_cli_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWAFv2_WebACL_Lifecycle(t *testing.T) {
	name := "cli-acl-" + time.Now().Format("150405.000000")
	out := runCLI(t, awsCLI("wafv2", "create-web-acl",
		"--name", name,
		"--scope", "CLOUDFRONT",
		"--default-action", `{"Allow":{}}`,
		"--visibility-config", fmt.Sprintf(`{"SampledRequestsEnabled":true,"CloudWatchMetricsEnabled":true,"MetricName":"%s-metric"}`, name),
		"--output", "json",
	))
	var createResult struct {
		Summary struct {
			Id        string `json:"Id"`
			ARN       string `json:"ARN"`
			LockToken string `json:"LockToken"`
		} `json:"Summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &createResult))
	require.NotEmpty(t, createResult.Summary.Id)
	require.NotEmpty(t, createResult.Summary.LockToken)

	id := createResult.Summary.Id
	lock := createResult.Summary.LockToken

	getOut := runCLI(t, awsCLI("wafv2", "get-web-acl",
		"--name", name, "--scope", "CLOUDFRONT", "--id", id, "--output", "json",
	))
	var getResult struct {
		WebACL struct {
			Name string `json:"Name"`
		} `json:"WebACL"`
		LockToken string `json:"LockToken"`
	}
	require.NoError(t, json.Unmarshal([]byte(getOut), &getResult))
	require.Equal(t, name, getResult.WebACL.Name)

	runCLI(t, awsCLI("wafv2", "list-web-acls", "--scope", "CLOUDFRONT", "--output", "json"))

	runCLI(t, awsCLI("wafv2", "delete-web-acl",
		"--name", name, "--scope", "CLOUDFRONT", "--id", id,
		"--lock-token", lock,
	))
}

func TestWAFv2_IPSet_Lifecycle(t *testing.T) {
	name := "cli-ipset-" + time.Now().Format("150405.000000")
	out := runCLI(t, awsCLI("wafv2", "create-ip-set",
		"--name", name, "--scope", "CLOUDFRONT",
		"--ip-address-version", "IPV4",
		"--addresses", "203.0.113.0/24", "198.51.100.10/32",
		"--output", "json",
	))
	var createResult struct {
		Summary struct {
			Id        string `json:"Id"`
			LockToken string `json:"LockToken"`
		} `json:"Summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &createResult))
	id := createResult.Summary.Id
	lock := createResult.Summary.LockToken

	runCLI(t, awsCLI("wafv2", "get-ip-set",
		"--name", name, "--scope", "CLOUDFRONT", "--id", id, "--output", "json"))

	runCLI(t, awsCLI("wafv2", "delete-ip-set",
		"--name", name, "--scope", "CLOUDFRONT", "--id", id,
		"--lock-token", lock,
	))
}
