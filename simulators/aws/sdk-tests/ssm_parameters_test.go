package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ssmClient() *ssm.Client {
	return ssm.NewFromConfig(sdkConfig(), func(o *ssm.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

// TestSSMParameter_PutGetDelete pins the canonical Parameter Store
// flow runner setups use: write a config value, fetch it from a
// workflow step, delete it on tear-down.
func TestSSMParameter_PutGetDelete(t *testing.T) {
	c := ssmClient()

	// PUT a String parameter.
	put, err := c.PutParameter(ctx, &ssm.PutParameterInput{
		Name:  aws.String("/runner/test-config"),
		Type:  ssmtypes.ParameterTypeString,
		Value: aws.String("hello"),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), put.Version)

	// GET it back.
	got, err := c.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String("/runner/test-config"),
	})
	require.NoError(t, err)
	require.NotNil(t, got.Parameter)
	assert.Equal(t, "hello", *got.Parameter.Value)
	assert.Equal(t, ssmtypes.ParameterTypeString, got.Parameter.Type)
	assert.Contains(t, *got.Parameter.ARN, "arn:aws:ssm:")

	// PUT again without Overwrite=true must fail.
	_, err = c.PutParameter(ctx, &ssm.PutParameterInput{
		Name:  aws.String("/runner/test-config"),
		Type:  ssmtypes.ParameterTypeString,
		Value: aws.String("collision"),
	})
	require.Error(t, err, "second PUT without Overwrite must fail like real Parameter Store")

	// PUT with Overwrite=true should succeed and bump Version.
	rotated, err := c.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String("/runner/test-config"),
		Type:      ssmtypes.ParameterTypeString,
		Value:     aws.String("rotated"),
		Overwrite: aws.Bool(true),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), rotated.Version)

	// Cleanup.
	_, err = c.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: aws.String("/runner/test-config"),
	})
	require.NoError(t, err)
}

// TestSSMParameter_GetParametersByPath pins the hierarchy lookup that
// terraform's `aws_ssm_parameters_by_path` data source and runner
// bulk-fetch flows use.
func TestSSMParameter_GetParametersByPath(t *testing.T) {
	c := ssmClient()

	// Seed: /env/prod/db, /env/prod/cache, /env/staging/db
	for _, p := range []struct{ name, val string }{
		{"/env/prod/db", "prod-db"},
		{"/env/prod/cache", "prod-cache"},
		{"/env/staging/db", "staging-db"},
	} {
		_, err := c.PutParameter(ctx, &ssm.PutParameterInput{
			Name:  aws.String(p.name),
			Type:  ssmtypes.ParameterTypeString,
			Value: aws.String(p.val),
		})
		require.NoError(t, err)
		defer c.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String(p.name)})
	}

	// Non-recursive: returns direct children of /env/prod only.
	flat, err := c.GetParametersByPath(ctx, &ssm.GetParametersByPathInput{
		Path:      aws.String("/env/prod"),
		Recursive: aws.Bool(false),
	})
	require.NoError(t, err)
	names := make(map[string]string)
	for _, p := range flat.Parameters {
		names[*p.Name] = *p.Value
	}
	assert.Equal(t, "prod-db", names["/env/prod/db"])
	assert.Equal(t, "prod-cache", names["/env/prod/cache"])
	assert.NotContains(t, names, "/env/staging/db", "Non-recursive must skip staging branch")

	// Recursive: returns all under /env.
	deep, err := c.GetParametersByPath(ctx, &ssm.GetParametersByPathInput{
		Path:      aws.String("/env"),
		Recursive: aws.Bool(true),
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(deep.Parameters), 3, "Recursive must return all 3 seeded parameters")
}

func TestSSMParameter_Describe(t *testing.T) {
	c := ssmClient()
	defer c.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String("/desc/p1")})

	_, err := c.PutParameter(ctx, &ssm.PutParameterInput{
		Name:  aws.String("/desc/p1"),
		Type:  ssmtypes.ParameterTypeString,
		Value: aws.String("v"),
	})
	require.NoError(t, err)

	out, err := c.DescribeParameters(ctx, &ssm.DescribeParametersInput{})
	require.NoError(t, err)
	found := false
	for _, p := range out.Parameters {
		if p.Name != nil && *p.Name == "/desc/p1" {
			found = true
		}
	}
	assert.True(t, found, "DescribeParameters must include the seeded parameter")
}

func TestSSMParameter_DeleteParameters(t *testing.T) {
	c := ssmClient()
	for _, n := range []string{"/del/a", "/del/b"} {
		_, err := c.PutParameter(ctx, &ssm.PutParameterInput{
			Name: aws.String(n), Type: ssmtypes.ParameterTypeString, Value: aws.String("v"),
		})
		require.NoError(t, err)
	}
	out, err := c.DeleteParameters(ctx, &ssm.DeleteParametersInput{
		Names: []string{"/del/a", "/del/b", "/del/missing"},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"/del/a", "/del/b"}, out.DeletedParameters)
	assert.Equal(t, []string{"/del/missing"}, out.InvalidParameters)
}
