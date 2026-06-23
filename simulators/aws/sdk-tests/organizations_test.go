package aws_sdk_test

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func orgClient() *organizations.Client {
	return organizations.NewFromConfig(sdkConfig(), func(o *organizations.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

// TestOrganizations_DescribeAndList covers the Organizations slice that backs the
// aws:PrincipalOrgID IAM condition key.
func TestOrganizations_DescribeAndList(t *testing.T) {
	c := orgClient()

	org, err := c.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	require.NoError(t, err)
	require.NotNil(t, org.Organization)
	assert.True(t, strings.HasPrefix(aws.ToString(org.Organization.Id), "o-"), "org id is o-…")
	assert.Equal(t, "ALL", string(org.Organization.FeatureSet))
	assert.NotEmpty(t, aws.ToString(org.Organization.MasterAccountId))

	accts, err := c.ListAccounts(ctx, &organizations.ListAccountsInput{})
	require.NoError(t, err)
	require.NotEmpty(t, accts.Accounts)
	assert.Equal(t, aws.ToString(org.Organization.MasterAccountId), aws.ToString(accts.Accounts[0].Id))
}
