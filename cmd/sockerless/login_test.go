package main

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func fullyConfiguredLoginEnvironment() *environment {
	return &environment{
		Backend: "docker",
		Login:   &loginConfig{Issuer: "http://localhost:8080", ClientID: "sockerless-cli"},
		AWS: &awsConfig{
			Region: "us-east-1",
			Login:  &awsLoginConfig{RoleARN: "arn:aws:iam::123456789012:role/cli-federation-role", EndpointURL: "http://localhost:29310"},
		},
		GCP: &gcpConfig{
			Project: "test-project",
			Login: &gcpLoginConfig{
				WorkforceAudience: "//iam.googleapis.com/locations/global/workforcePools/sockerless-console/providers/cli",
				STSEndpoint:       "http://localhost:29320",
				APIEndpoint:       "http://localhost:29320",
			},
		},
		Azure: &azureConfig{
			SubscriptionID: "00000000-0000-0000-0000-000000000001",
			Login: &azureLoginConfig{
				Tenant:                  "11111111-1111-1111-1111-111111111111",
				ClientID:                "33333333-3333-3333-3333-333333333333",
				AuthorityEndpoint:       "https://127.0.0.1:29331",
				ResourceManagerEndpoint: "https://127.0.0.1:29331",
			},
		},
	}
}

func TestLoginCloudSelectionsAllConfigured(t *testing.T) {
	selections := loginCloudSelections(fullyConfiguredLoginEnvironment(), "dev")
	if len(selections) != 3 {
		t.Fatalf("got %d selections, want 3", len(selections))
	}
	for _, selection := range selections {
		if selection.Skip != "" {
			t.Errorf("%s unexpectedly skipped: %s", selection.Name, selection.Skip)
		}
	}
}

func TestLoginCloudSelectionsLoudSkip(t *testing.T) {
	env := &environment{Backend: "docker", Login: &loginConfig{Issuer: "http://localhost:8080", ClientID: "sockerless-cli"}}
	selections := loginCloudSelections(env, "dev")
	wantSkips := map[string]string{
		"aws":   "environments.dev.aws.login.role_arn",
		"gcp":   "environments.dev.gcp.login.workforce_audience",
		"azure": "environments.dev.azure.login.tenant",
	}
	for _, selection := range selections {
		want := wantSkips[selection.Name]
		if selection.Skip == "" {
			t.Errorf("%s: expected a skip reason", selection.Name)
			continue
		}
		if !strings.Contains(selection.Skip, want) {
			t.Errorf("%s skip = %q, want mention of %q", selection.Name, selection.Skip, want)
		}
	}
}

func TestFilterCloudSelections(t *testing.T) {
	selections := loginCloudSelections(fullyConfiguredLoginEnvironment(), "dev")

	only, err := filterCloudSelections(selections, "aws")
	if err != nil {
		t.Fatalf("filter aws: %v", err)
	}
	if len(only) != 1 || only[0].Name != "aws" {
		t.Fatalf("filter aws = %+v", only)
	}

	all, err := filterCloudSelections(selections, "")
	if err != nil || len(all) != 3 {
		t.Fatalf("no filter = (%d, %v), want 3 clouds", len(all), err)
	}

	if _, err := filterCloudSelections(selections, "dockerhub"); err == nil {
		t.Fatal("unknown cloud accepted")
	}
}

func TestGCPExternalAccountJSON(t *testing.T) {
	env := fullyConfiguredLoginEnvironment()
	raw, err := gcpExternalAccountJSON(env.GCP.Login, env.GCP.Project, "/home/u/.sockerless/contexts/dev/web-identity-token")
	if err != nil {
		t.Fatal(err)
	}
	var adc map[string]any
	if err := json.Unmarshal(raw, &adc); err != nil {
		t.Fatalf("credentials are not valid JSON: %v", err)
	}
	want := map[string]string{
		"type":                        "external_account",
		"audience":                    "//iam.googleapis.com/locations/global/workforcePools/sockerless-console/providers/cli",
		"subject_token_type":          "urn:ietf:params:oauth:token-type:id_token",
		"token_url":                   "http://localhost:29320/v1/token",
		"token_info_url":              "http://localhost:29320/v1/introspect",
		"workforce_pool_user_project": "test-project",
	}
	for key, value := range want {
		if got, _ := adc[key].(string); got != value {
			t.Errorf("%s = %q, want %q", key, adc[key], value)
		}
	}
	source, _ := adc["credential_source"].(map[string]any)
	if got, _ := source["file"].(string); got != "/home/u/.sockerless/contexts/dev/web-identity-token" {
		t.Errorf("credential_source.file = %q", source["file"])
	}
}

func TestGCPExternalAccountJSONDefaultsToRealGoogle(t *testing.T) {
	login := &gcpLoginConfig{
		WorkforceAudience:        "//iam.googleapis.com/locations/global/workforcePools/pool/providers/p",
		WorkforcePoolUserProject: "quota-project",
	}
	raw, err := gcpExternalAccountJSON(login, "other-project", "/tmp/token")
	if err != nil {
		t.Fatal(err)
	}
	var adc map[string]any
	if err := json.Unmarshal(raw, &adc); err != nil {
		t.Fatal(err)
	}
	if got, _ := adc["token_url"].(string); got != "https://sts.googleapis.com/v1/token" {
		t.Errorf("token_url = %q, want the real Security Token Service", got)
	}
	if got, _ := adc["token_info_url"].(string); got != "https://sts.googleapis.com/v1/introspect" {
		t.Errorf("token_info_url = %q", got)
	}
	if got, _ := adc["workforce_pool_user_project"].(string); got != "quota-project" {
		t.Errorf("workforce_pool_user_project = %q, want the explicit override", got)
	}
}

func TestGcloudEndpointOverrides(t *testing.T) {
	overrides := gcloudEndpointOverrides("http://localhost:29320/")
	byProperty := make(map[string]string, len(overrides))
	for _, kv := range overrides {
		byProperty[kv[0]] = kv[1]
	}
	if got := byProperty["api_endpoint_overrides/cloudresourcemanager"]; got != "http://localhost:29320/" {
		t.Errorf("cloudresourcemanager = %q", got)
	}
	if got := byProperty["api_endpoint_overrides/bigquery"]; got != "http://localhost:29320/bigquery/v2/" {
		t.Errorf("bigquery = %q", got)
	}
	if got := byProperty["api_endpoint_overrides/spanner"]; got != "http://localhost:29320/spanner/" {
		t.Errorf("spanner = %q", got)
	}
}

func TestLoginConfigYAMLRoundTrip(t *testing.T) {
	cfg := &unifiedConfig{Environments: map[string]*environment{"dev": fullyConfiguredLoginEnvironment()}}
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded unifiedConfig
	if err := yaml.Unmarshal(raw, &reloaded); err != nil {
		t.Fatal(err)
	}
	env := reloaded.Environments["dev"]
	if env.Login == nil || env.Login.Issuer != "http://localhost:8080" || env.Login.ClientID != "sockerless-cli" {
		t.Fatalf("login round-trip lost data: %+v", env.Login)
	}
	if env.AWS.Login.RoleARN != "arn:aws:iam::123456789012:role/cli-federation-role" {
		t.Fatalf("aws login round-trip lost data: %+v", env.AWS.Login)
	}
	if env.GCP.Login.WorkforceAudience == "" || env.GCP.Login.STSEndpoint != "http://localhost:29320" {
		t.Fatalf("gcp login round-trip lost data: %+v", env.GCP.Login)
	}
	if env.Azure.Login.Tenant != "11111111-1111-1111-1111-111111111111" || env.Azure.Login.AuthorityEndpoint != "https://127.0.0.1:29331" {
		t.Fatalf("azure login round-trip lost data: %+v", env.Azure.Login)
	}
}
