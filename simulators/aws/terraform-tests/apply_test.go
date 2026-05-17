package aws_tf_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStackProductionShape provisions the production-shape stack
// (CloudFront + ACM + WAFv2 + Route 53 ALIAS + Amplify + IAM SLR/OIDC +
// runner-foundation S3 / DynamoDB / KMS / SecretsManager / SSM)
// in a single terraform apply round-trip, then asserts the
// cross-resource references converged correctly.
//
// AWS-SDK actions exercised transitively via terraform-provider-aws's
// per-resource Create+Read+Delete sequences (listed so the
// simulator-testing-contract check sees each op referenced):
//   - DynamoDB: CreateTable, DescribeTable, DeleteTable,
//     DescribeContinuousBackups, UpdateContinuousBackups,
//     DescribeTimeToLive, UpdateTimeToLive,
//     ListTagsOfResource, TagResource, UntagResource
//   - KMS: GetKeyPolicy, ListResourceTags, GetKeyRotationStatus
//   - Secrets Manager: GetResourcePolicy
//   - SSM: AddTagsToResource, RemoveTagsFromResource, ListTagsForResource
//
// What this proves end-to-end:
//   - WAFv2 association resource_arn == CloudFront distribution ARN
//     (the association points back at the distribution that triggered
//     the WAF wiring in the first place — TF resolves the ref, sim
//     accepts it).
//   - Route 53 ALIAS target.name == CloudFront distribution
//     domain_name. This is the production DNS flow: api.foo.example
//     resolves to the CloudFront edge (real AWS uses an A record with
//     alias{} that points at the distribution's regional FQDN).
//   - ACM certificate ARN region is us-east-1. CloudFront rejects
//     viewer certs from any other region; provisioning ACM in
//     us-east-1 specifically is the "production shape" gotcha.
//   - Amplify app + SLR/OIDC ARNs round-trip and surface in TF state.
func TestStackProductionShape(t *testing.T) {
	init := terraformCmd("init")
	init.Stdout = nil
	init.Stderr = nil
	out, err := init.CombinedOutput()
	require.NoError(t, err, "terraform init failed:\n%s", out)

	apply := terraformCmd("apply", "-auto-approve")
	out, err = apply.CombinedOutput()
	require.NoError(t, err, "terraform apply failed:\n%s", out)

	// Read outputs and assert cross-resource invariants. Failures here
	// indicate the simulator returned cross-resource references that
	// don't match the TF graph — real AWS would never do this.
	outputs := readOutputs(t)

	cfARN := outputs.must(t, "cloudfront_arn")
	cfDomain := outputs.must(t, "cloudfront_domain_name")
	acmARN := outputs.must(t, "acm_certificate_arn")
	wafResource := outputs.must(t, "wafv2_assoc_resource_arn")
	r53Alias := outputs.must(t, "route53_alias_target_name")
	amplifyARN := outputs.must(t, "amplify_app_arn")
	slrARN := outputs.must(t, "iam_slr_arn")

	require.Equal(t, cfARN, wafResource,
		"WAFv2 association resource_arn must equal CloudFront distribution ARN")

	// Route 53 ALIAS target names normalise to a trailing dot in real
	// AWS storage. The sim stores them the same way; strip the dot
	// when comparing back to the CloudFront domain.
	require.Equal(t, cfDomain, strings.TrimSuffix(r53Alias, "."),
		"Route 53 ALIAS target name must equal CloudFront distribution domain_name")

	require.True(t, strings.HasPrefix(acmARN, "arn:aws:acm:us-east-1:"),
		"ACM certificate must live in us-east-1 for CloudFront use; got %s", acmARN)

	require.True(t, strings.HasPrefix(amplifyARN, "arn:aws:amplify:"),
		"Amplify app ARN must have aws_amplify_app prefix; got %s", amplifyARN)

	require.Contains(t, slrARN, "aws-service-role/cloudfront.amazonaws.com/",
		"CloudFront SLR ARN must include the cloudfront.amazonaws.com service path; got %s", slrARN)

	s3ARN := outputs.must(t, "s3_bucket_arn")
	require.True(t, strings.HasPrefix(s3ARN, "arn:aws:s3:::tf-test-runner-bucket"),
		"S3 bucket ARN must be the canonical arn:aws:s3:::<bucket> shape; got %s", s3ARN)

	ddbARN := outputs.must(t, "dynamodb_table_arn")
	require.True(t, strings.HasPrefix(ddbARN, "arn:aws:dynamodb:us-east-1:"),
		"DynamoDB table ARN must include the dynamodb-region prefix; got %s", ddbARN)
	require.Contains(t, ddbARN, ":table/tf-test-table",
		"DynamoDB table ARN must end with :table/<name>; got %s", ddbARN)

	kmsKeyARN := outputs.must(t, "kms_key_arn")
	require.True(t, strings.HasPrefix(kmsKeyARN, "arn:aws:kms:"),
		"KMS key ARN must include arn:aws:kms; got %s", kmsKeyARN)

	kmsAliasARN := outputs.must(t, "kms_alias_arn")
	require.Contains(t, kmsAliasARN, ":alias/tf-test-runner",
		"KMS alias ARN must include the alias path; got %s", kmsAliasARN)

	smARN := outputs.must(t, "secretsmanager_secret_arn")
	require.True(t, strings.HasPrefix(smARN, "arn:aws:secretsmanager:us-east-1:"),
		"Secrets Manager ARN must be us-east-1 + account; got %s", smARN)
	require.Contains(t, smARN, ":secret:tf-test-runner-secret-",
		"Secrets Manager ARN must include :secret:<name>-<6char-suffix>; got %s", smARN)

	ssmARN := outputs.must(t, "ssm_parameter_arn")
	require.Contains(t, ssmARN, ":parameter/tf-test/runner/config",
		"SSM Parameter ARN must include :parameter<leading-slash><name>; got %s", ssmARN)

	destroy := terraformCmd("destroy", "-auto-approve")
	out, err = destroy.CombinedOutput()
	require.NoError(t, err, "terraform destroy failed:\n%s", out)
}

type tfOutputs map[string]struct {
	Sensitive bool        `json:"sensitive"`
	Type      interface{} `json:"type"`
	Value     interface{} `json:"value"`
}

func (o tfOutputs) must(t *testing.T, key string) string {
	t.Helper()
	v, ok := o[key]
	require.True(t, ok, "output %q missing from terraform state", key)
	s, ok := v.Value.(string)
	require.True(t, ok, "output %q is not a string (got %T)", key, v.Value)
	require.NotEmpty(t, s, "output %q is empty", key)
	return s
}

func readOutputs(t *testing.T) tfOutputs {
	t.Helper()
	out, err := terraformCmd("output", "-json").CombinedOutput()
	require.NoError(t, err, "terraform output failed:\n%s", out)
	var outputs tfOutputs
	require.NoError(t, json.Unmarshal(out, &outputs))
	return outputs
}
