package aws_cli_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRoute53_ZoneAndRecord(t *testing.T) {
	caller := "cli-r53-" + time.Now().Format("150405.000000")
	out := runCLI(t, awsCLI("route53", "create-hosted-zone",
		"--name", "cli-route53-test.local",
		"--caller-reference", caller,
		"--output", "json",
	))
	var createResult struct {
		HostedZone struct {
			Id   string `json:"Id"`
			Name string `json:"Name"`
		} `json:"HostedZone"`
		ChangeInfo struct {
			Status string `json:"Status"`
		} `json:"ChangeInfo"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &createResult))
	zoneID := strings.TrimPrefix(createResult.HostedZone.Id, "/hostedzone/")
	require.NotEmpty(t, zoneID)
	require.Equal(t, "INSYNC", createResult.ChangeInfo.Status)

	// Add an A record + alias record via change-batch JSON
	changeJSON := `{
		"Changes": [
			{
				"Action": "CREATE",
				"ResourceRecordSet": {
					"Name": "api.cli-route53-test.local.",
					"Type": "A",
					"TTL": 300,
					"ResourceRecords": [{"Value": "203.0.113.1"}]
				}
			},
			{
				"Action": "CREATE",
				"ResourceRecordSet": {
					"Name": "cdn.cli-route53-test.local.",
					"Type": "A",
					"AliasTarget": {
						"HostedZoneId": "Z2FDTNDATAQYW2",
						"DNSName": "d111111abcdef8.cloudfront.net.",
						"EvaluateTargetHealth": false
					}
				}
			}
		]
	}`

	runCLI(t, awsCLI("route53", "change-resource-record-sets",
		"--hosted-zone-id", zoneID,
		"--change-batch", changeJSON,
		"--output", "json",
	))

	listOut := runCLI(t, awsCLI("route53", "list-resource-record-sets",
		"--hosted-zone-id", zoneID, "--output", "json"))
	var listResult struct {
		ResourceRecordSets []struct {
			Name        string `json:"Name"`
			Type        string `json:"Type"`
			AliasTarget *struct {
				HostedZoneId string `json:"HostedZoneId"`
				DNSName      string `json:"DNSName"`
			} `json:"AliasTarget,omitempty"`
		} `json:"ResourceRecordSets"`
	}
	require.NoError(t, json.Unmarshal([]byte(listOut), &listResult))
	// Initial 2 (NS+SOA) + 2 new (A + alias)
	require.GreaterOrEqual(t, len(listResult.ResourceRecordSets), 4)
	aliasFound := false
	for _, r := range listResult.ResourceRecordSets {
		if r.Name == "cdn.cli-route53-test.local." {
			require.NotNil(t, r.AliasTarget)
			require.Equal(t, "Z2FDTNDATAQYW2", r.AliasTarget.HostedZoneId)
			aliasFound = true
		}
	}
	require.True(t, aliasFound)

	// Cleanup
	delJSON := `{
		"Changes": [
			{"Action": "DELETE", "ResourceRecordSet": {"Name": "api.cli-route53-test.local.", "Type": "A", "TTL": 300, "ResourceRecords": [{"Value": "203.0.113.1"}]}},
			{"Action": "DELETE", "ResourceRecordSet": {"Name": "cdn.cli-route53-test.local.", "Type": "A", "AliasTarget": {"HostedZoneId": "Z2FDTNDATAQYW2", "DNSName": "d111111abcdef8.cloudfront.net.", "EvaluateTargetHealth": false}}}
		]
	}`
	runCLI(t, awsCLI("route53", "change-resource-record-sets",
		"--hosted-zone-id", zoneID,
		"--change-batch", delJSON,
	))
	runCLI(t, awsCLI("route53", "delete-hosted-zone", "--id", zoneID))
}
