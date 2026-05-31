package azure_cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedisCLI_ARMResources(t *testing.T) {
	name := "cliredis"

	cacheURL := armURL("Microsoft.Cache", "Redis/"+name, "2024-11-01")
	out := runCLI(t, azRest("PUT", cacheURL, `{
		"location":"eastus",
		"sku":{"name":"Basic","family":"C","capacity":1},
		"properties":{"minimumTlsVersion":"1.2","redisVersion":"6"},
		"tags":{"env":"cli"}
	}`))
	t.Cleanup(func() {
		_ = azRest("DELETE", cacheURL, "").Run()
	})

	var cache struct {
		Name       string         `json:"name"`
		SKU        map[string]any `json:"sku"`
		Properties struct {
			ProvisioningState string `json:"provisioningState"`
			HostName          string `json:"hostName"`
			SSLPort           int    `json:"sslPort"`
		} `json:"properties"`
		Tags map[string]string `json:"tags"`
	}
	parseJSON(t, out, &cache)
	require.Equal(t, name, cache.Name)
	require.Equal(t, "Basic", cache.SKU["name"])
	require.Equal(t, "Succeeded", cache.Properties.ProvisioningState)
	require.Contains(t, cache.Properties.HostName, name+".redis.cache.")
	require.Equal(t, 6380, cache.Properties.SSLPort)
	require.Equal(t, "cli", cache.Tags["env"])

	keysOut := runCLI(t, azRest("POST", armURL("Microsoft.Cache", "Redis/"+name+"/listKeys", "2024-11-01"), ""))
	var keys struct {
		PrimaryKey   string `json:"primaryKey"`
		SecondaryKey string `json:"secondaryKey"`
	}
	parseJSON(t, keysOut, &keys)
	require.NotEmpty(t, keys.PrimaryKey)
	require.NotEqual(t, keys.PrimaryKey, keys.SecondaryKey)

	ruleURL := armURL("Microsoft.Cache", "Redis/"+name+"/firewallRules/allow-ci", "2024-11-01")
	ruleOut := runCLI(t, azRest("PUT", ruleURL, `{
		"properties":{"startIP":"203.0.113.10","endIP":"203.0.113.10"}
	}`))
	var rule struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Properties struct {
			StartIP string `json:"startIP"`
			EndIP   string `json:"endIP"`
		} `json:"properties"`
	}
	parseJSON(t, ruleOut, &rule)
	require.Equal(t, "allow-ci", rule.Name)
	require.Contains(t, rule.ID, "/Redis/"+name+"/firewallRules/allow-ci")
	require.Equal(t, "203.0.113.10", rule.Properties.StartIP)

	listOut := runCLI(t, azRest("GET", armURL("Microsoft.Cache", "Redis/"+name+"/firewallRules", "2024-11-01"), ""))
	require.Contains(t, listOut, `"name": "allow-ci"`)

	runCLI(t, azRest("DELETE", ruleURL, ""))
}
