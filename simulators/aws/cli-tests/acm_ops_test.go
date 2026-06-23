package aws_cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acmRequestPrivateCertCLI issues a PRIVATE (PCA-backed) certificate, which
// the sim mints with real X.509 material and issues synchronously — the
// precondition for get/export/revoke. Returns the ARN; registers cleanup.
func acmRequestPrivateCertCLI(t *testing.T, domain string) string {
	t.Helper()
	out := runCLI(t, awsCLI("acm", "request-certificate",
		"--domain-name", domain,
		"--certificate-authority-arn", "arn:aws:acm-pca:us-east-1:000000000000:certificate-authority/test-ca",
		"--output", "json"))
	var res struct {
		CertificateArn string `json:"CertificateArn"`
	}
	parseJSON(t, out, &res)
	require.NotEmpty(t, res.CertificateArn)
	t.Cleanup(func() {
		_ = awsCLI("acm", "delete-certificate", "--certificate-arn", res.CertificateArn).Run()
	})
	return res.CertificateArn
}

func TestACMCLI_GetCertificate(t *testing.T) {
	arn := acmRequestPrivateCertCLI(t, "get.cli.private.example.com")
	out := runCLI(t, awsCLI("acm", "get-certificate", "--certificate-arn", arn, "--output", "json"))
	var res struct {
		Certificate string `json:"Certificate"`
	}
	parseJSON(t, out, &res)
	assert.Contains(t, res.Certificate, "BEGIN CERTIFICATE")
}

func TestACMCLI_ExportCertificate(t *testing.T) {
	arn := acmRequestPrivateCertCLI(t, "export.cli.private.example.com")
	out := runCLI(t, awsCLI("acm", "export-certificate",
		"--certificate-arn", arn,
		"--passphrase", "hunter2",
		"--cli-binary-format", "raw-in-base64-out",
		"--output", "json"))
	var res struct {
		Certificate string `json:"Certificate"`
		PrivateKey  string `json:"PrivateKey"`
	}
	parseJSON(t, out, &res)
	assert.Contains(t, res.Certificate, "BEGIN CERTIFICATE")
	assert.Contains(t, res.PrivateKey, "PRIVATE KEY")
}

func TestACMCLI_RevokeCertificate(t *testing.T) {
	arn := acmRequestPrivateCertCLI(t, "revoke.cli.private.example.com")
	out := runCLI(t, awsCLI("acm", "revoke-certificate",
		"--certificate-arn", arn,
		"--revocation-reason", "KEY_COMPROMISE",
		"--output", "json"))
	var res struct {
		CertificateArn string `json:"CertificateArn"`
	}
	parseJSON(t, out, &res)
	assert.Equal(t, arn, res.CertificateArn)

	descOut := runCLI(t, awsCLI("acm", "describe-certificate", "--certificate-arn", arn, "--output", "json"))
	var desc struct {
		Certificate struct {
			Status string `json:"Status"`
		} `json:"Certificate"`
	}
	parseJSON(t, descOut, &desc)
	assert.Equal(t, "REVOKED", desc.Certificate.Status)
}

func TestACMCLI_AccountConfiguration(t *testing.T) {
	runCLI(t, awsCLI("acm", "put-account-configuration",
		"--expiry-events", "DaysBeforeExpiry=21",
		"--idempotency-token", "cli-acct-config-tok"))

	out := runCLI(t, awsCLI("acm", "get-account-configuration", "--output", "json"))
	var res struct {
		ExpiryEvents struct {
			DaysBeforeExpiry int `json:"DaysBeforeExpiry"`
		} `json:"ExpiryEvents"`
	}
	parseJSON(t, out, &res)
	assert.Equal(t, 21, res.ExpiryEvents.DaysBeforeExpiry)
}

func TestACMCLI_SearchCertificates(t *testing.T) {
	arn := acmRequestPrivateCertCLI(t, "search.cli.private.example.com")

	out := runCLI(t, awsCLI("acm", "search-certificates",
		"--filter-statement", `{"Filter":{"AcmCertificateMetadataFilter":{"Type":"PRIVATE"}}}`,
		"--output", "json"))
	var res struct {
		Results []struct {
			CertificateArn      string `json:"CertificateArn"`
			CertificateMetadata struct {
				AcmCertificateMetadata struct {
					Type string `json:"Type"`
				} `json:"AcmCertificateMetadata"`
			} `json:"CertificateMetadata"`
		} `json:"Results"`
	}
	parseJSON(t, out, &res)
	var found bool
	for _, r := range res.Results {
		assert.Equal(t, "PRIVATE", r.CertificateMetadata.AcmCertificateMetadata.Type,
			"search filtered by Type=PRIVATE must only return PRIVATE certs")
		if r.CertificateArn == arn {
			found = true
		}
	}
	assert.True(t, found, "search must return the just-issued PRIVATE certificate")
}
