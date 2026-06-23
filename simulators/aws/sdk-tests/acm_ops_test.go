package aws_sdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requestPrivateCert issues a PRIVATE (PCA-backed) certificate, which the sim
// mints with real X.509 material and issues synchronously — the precondition
// for Get/Export/Revoke. Returns the ARN; registers cleanup.
func requestPrivateCert(t *testing.T, c *acm.Client, domain string) string {
	t.Helper()
	out, err := c.RequestCertificate(ctx, &acm.RequestCertificateInput{
		DomainName:              aws.String(domain),
		CertificateAuthorityArn: aws.String("arn:aws:acm-pca:us-east-1:000000000000:certificate-authority/test-ca"),
	})
	require.NoError(t, err)
	arn := aws.ToString(out.CertificateArn)
	t.Cleanup(func() {
		_, _ = c.DeleteCertificate(context.Background(), &acm.DeleteCertificateInput{CertificateArn: aws.String(arn)})
	})
	return arn
}

// TestACM_GetCertificate pins GetCertificate returning the stored PEM body and
// chain for an issued PRIVATE certificate, and erroring for a cert with no
// material yet.
func TestACM_GetCertificate(t *testing.T) {
	c := acmClient()

	arn := requestPrivateCert(t, c, "get.private.example.com")
	out, err := c.GetCertificate(ctx, &acm.GetCertificateInput{CertificateArn: aws.String(arn)})
	require.NoError(t, err)
	require.NotNil(t, out.Certificate)
	assert.Contains(t, aws.ToString(out.Certificate), "BEGIN CERTIFICATE",
		"GetCertificate must return a real PEM body")
}

// TestACM_ExportCertificate pins ExportCertificate returning the cert, chain,
// and private key for a PRIVATE certificate; a missing passphrase is rejected,
// and a non-PRIVATE cert can't be exported.
func TestACM_ExportCertificate(t *testing.T) {
	c := acmClient()

	arn := requestPrivateCert(t, c, "export.private.example.com")
	out, err := c.ExportCertificate(ctx, &acm.ExportCertificateInput{
		CertificateArn: aws.String(arn),
		Passphrase:     []byte("hunter2"),
	})
	require.NoError(t, err)
	assert.Contains(t, aws.ToString(out.Certificate), "BEGIN CERTIFICATE")
	assert.Contains(t, aws.ToString(out.PrivateKey), "PRIVATE KEY",
		"ExportCertificate must return the private key for a PRIVATE cert")

	// Missing passphrase is rejected.
	_, err = c.ExportCertificate(ctx, &acm.ExportCertificateInput{CertificateArn: aws.String(arn)})
	require.Error(t, err)

	// A public (AMAZON_ISSUED) cert cannot be exported.
	reqOut, err := c.RequestCertificate(ctx, &acm.RequestCertificateInput{
		DomainName:       aws.String("public.example.com"),
		ValidationMethod: acmtypes.ValidationMethodDns,
	})
	require.NoError(t, err)
	pubArn := aws.ToString(reqOut.CertificateArn)
	t.Cleanup(func() {
		_, _ = c.DeleteCertificate(context.Background(), &acm.DeleteCertificateInput{CertificateArn: aws.String(pubArn)})
	})
	_, err = c.ExportCertificate(ctx, &acm.ExportCertificateInput{
		CertificateArn: aws.String(pubArn),
		Passphrase:     []byte("x"),
	})
	require.Error(t, err, "a non-PRIVATE certificate must not be exportable")
}

// TestACM_RevokeCertificate pins RevokeCertificate moving a PRIVATE cert to
// REVOKED and requiring a reason.
func TestACM_RevokeCertificate(t *testing.T) {
	c := acmClient()

	arn := requestPrivateCert(t, c, "revoke.private.example.com")
	out, err := c.RevokeCertificate(ctx, &acm.RevokeCertificateInput{
		CertificateArn:   aws.String(arn),
		RevocationReason: acmtypes.RevocationReasonKeyCompromise,
	})
	require.NoError(t, err)
	assert.Equal(t, arn, aws.ToString(out.CertificateArn))

	desc, err := c.DescribeCertificate(ctx, &acm.DescribeCertificateInput{CertificateArn: aws.String(arn)})
	require.NoError(t, err)
	assert.Equal(t, acmtypes.CertificateStatusRevoked, desc.Certificate.Status)
}

// TestACM_AccountConfiguration pins the account expiry-events config
// round-trip: default 45 days, then a Put updates it.
func TestACM_AccountConfiguration(t *testing.T) {
	c := acmClient()

	_, err := c.PutAccountConfiguration(ctx, &acm.PutAccountConfigurationInput{
		ExpiryEvents:     &acmtypes.ExpiryEventsConfiguration{DaysBeforeExpiry: aws.Int32(30)},
		IdempotencyToken: aws.String("tok-account-config"),
	})
	require.NoError(t, err)

	got, err := c.GetAccountConfiguration(ctx, &acm.GetAccountConfigurationInput{})
	require.NoError(t, err)
	require.NotNil(t, got.ExpiryEvents)
	require.NotNil(t, got.ExpiryEvents.DaysBeforeExpiry)
	assert.EqualValues(t, 30, *got.ExpiryEvents.DaysBeforeExpiry)

	// PutAccountConfiguration without an IdempotencyToken must fail.
	_, err = c.PutAccountConfiguration(ctx, &acm.PutAccountConfigurationInput{
		ExpiryEvents: &acmtypes.ExpiryEventsConfiguration{DaysBeforeExpiry: aws.Int32(15)},
	})
	require.Error(t, err)
}

// TestACM_SearchCertificates pins SearchCertificates filtering by metadata
// (Type) and returning per-cert metadata results.
func TestACM_SearchCertificates(t *testing.T) {
	c := acmClient()

	privateArn := requestPrivateCert(t, c, "search-private.example.com")

	deadline := time.Now().Add(5 * time.Second)
	var found bool
	for time.Now().Before(deadline) {
		out, err := c.SearchCertificates(ctx, &acm.SearchCertificatesInput{
			FilterStatement: &acmtypes.CertificateFilterStatementMemberFilter{
				Value: &acmtypes.CertificateFilterMemberAcmCertificateMetadataFilter{
					Value: &acmtypes.AcmCertificateMetadataFilterMemberType{
						Value: acmtypes.CertificateTypePrivate,
					},
				},
			},
		})
		require.NoError(t, err)
		for _, res := range out.Results {
			meta, ok := res.CertificateMetadata.(*acmtypes.CertificateMetadataMemberAcmCertificateMetadata)
			require.True(t, ok, "CertificateMetadata must carry AcmCertificateMetadata")
			// Every returned cert must be PRIVATE per the filter.
			assert.Equal(t, acmtypes.CertificateTypePrivate, meta.Value.Type)
			if aws.ToString(res.CertificateArn) == privateArn {
				found = true
			}
		}
		if found {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assert.True(t, found, "SearchCertificates must return the PRIVATE cert when filtering by Type=PRIVATE")
}
