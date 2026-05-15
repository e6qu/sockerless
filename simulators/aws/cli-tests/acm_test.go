package aws_cli_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestACM_RequestDescribeDelete(t *testing.T) {
	out := runCLI(t, awsCLI("acm", "request-certificate",
		"--domain-name", "cli.example.com",
		"--validation-method", "DNS",
		"--subject-alternative-names", "www.cli.example.com",
		"--output", "json",
	))
	var createResult struct {
		CertificateArn string `json:"CertificateArn"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &createResult))
	require.NotEmpty(t, createResult.CertificateArn)
	arn := createResult.CertificateArn

	descOut := runCLI(t, awsCLI("acm", "describe-certificate",
		"--certificate-arn", arn, "--output", "json"))
	var descResult struct {
		Certificate struct {
			DomainName              string `json:"DomainName"`
			Status                  string `json:"Status"`
			DomainValidationOptions []struct {
				DomainName     string `json:"DomainName"`
				ResourceRecord *struct {
					Name string `json:"Name"`
					Type string `json:"Type"`
				} `json:"ResourceRecord"`
			} `json:"DomainValidationOptions"`
		} `json:"Certificate"`
	}
	require.NoError(t, json.Unmarshal([]byte(descOut), &descResult))
	require.Equal(t, "cli.example.com", descResult.Certificate.DomainName)
	require.Equal(t, "PENDING_VALIDATION", descResult.Certificate.Status)
	require.Len(t, descResult.Certificate.DomainValidationOptions, 2)
	for _, dvo := range descResult.Certificate.DomainValidationOptions {
		require.NotNil(t, dvo.ResourceRecord, "DNS validation must include ResourceRecord")
		if dvo.ResourceRecord != nil {
			require.Equal(t, "CNAME", dvo.ResourceRecord.Type)
		}
	}

	runCLI(t, awsCLI("acm", "list-certificates", "--output", "json"))
	runCLI(t, awsCLI("acm", "delete-certificate", "--certificate-arn", arn))
}

func TestACM_Tags(t *testing.T) {
	out := runCLI(t, awsCLI("acm", "request-certificate",
		"--domain-name", "cli-tags.example.com",
		"--validation-method", "DNS",
		"--tags", "Key=env,Value=test", "Key=team,Value=infra",
		"--output", "json",
	))
	var createResult struct {
		CertificateArn string `json:"CertificateArn"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &createResult))
	arn := createResult.CertificateArn
	defer func() {
		_ = awsCLI("acm", "delete-certificate", "--certificate-arn", arn).Run()
	}()

	listOut := runCLI(t, awsCLI("acm", "list-tags-for-certificate",
		"--certificate-arn", arn, "--output", "json"))
	var tagsResult struct {
		Tags []struct {
			Key   string `json:"Key"`
			Value string `json:"Value"`
		} `json:"Tags"`
	}
	require.NoError(t, json.Unmarshal([]byte(listOut), &tagsResult))
	require.Len(t, tagsResult.Tags, 2)
}
