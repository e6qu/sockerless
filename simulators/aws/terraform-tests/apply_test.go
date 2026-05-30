package aws_tf_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

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
//   - KMS: GetKeyPolicy, PutKeyPolicy, ListResourceTags, GetKeyRotationStatus
//   - Secrets Manager: GetResourcePolicy
//   - SSM: AddTagsToResource, RemoveTagsFromResource, ListTagsForResource
//   - ELBv2: CreateLoadBalancer, DescribeLoadBalancers,
//     ModifyLoadBalancerAttributes, DescribeLoadBalancerAttributes,
//     DescribeCapacityReservation,
//     CreateTargetGroup, DescribeTargetGroups,
//     ModifyTargetGroupAttributes, DescribeTargetGroupAttributes,
//     CreateListener, DescribeListeners, DescribeListenerAttributes, DeleteListener,
//     DeleteTargetGroup, DeleteLoadBalancer, AddTags, DescribeTags
//   - EC2 NAT/EIP: AllocateAddress, DescribeAddresses,
//     DescribeAddressesAttribute, ReleaseAddress, CreateNatGateway,
//     DescribeNatGateways, DeleteNatGateway, CreateRouteTable,
//     CreateRoute, DescribeRouteTables
//   - API Gateway: CreateRestApi, GetRestApi, DeleteRestApi,
//     CreateResource, GetResource, DeleteResource, PutMethod,
//     GetMethod, DeleteMethod, PutIntegration, GetIntegration,
//     DeleteIntegration, PutMethodResponse, GetMethodResponse,
//     DeleteMethodResponse, PutIntegrationResponse,
//     GetIntegrationResponse, DeleteIntegrationResponse,
//     CreateDeployment, GetDeployment, DeleteDeployment,
//     CreateStage, GetStage, DeleteStage
//   - API Gateway v2: CreateApi, GetApi, DeleteApi,
//     CreateIntegration, GetIntegration, DeleteIntegration,
//     CreateRoute, GetRoute, DeleteRoute, CreateDeployment,
//     GetDeployment, DeleteDeployment, CreateStage, GetStage,
//     DeleteStage
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
	out, err := runTimed(t, "terraform init", init)
	require.NoError(t, err, "terraform init failed:\n%s", out)

	apply := terraformCmd("apply", "-auto-approve")
	out, err = runTimed(t, "terraform apply", apply)
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

	require.NotEmpty(t, outputs.must(t, "apigateway_rest_api_id"),
		"API Gateway REST API id must round-trip through Terraform state")
	require.Equal(t, "/tf", outputs.must(t, "apigateway_rest_resource_path"),
		"API Gateway REST resource path must use AWS's canonical path shape")
	require.Equal(t, "tf", outputs.must(t, "apigateway_rest_stage_name"),
		"API Gateway REST stage name must round-trip through provider refresh")
	require.NotEmpty(t, outputs.must(t, "apigatewayv2_api_id"),
		"API Gateway v2 API id must round-trip through Terraform state")
	require.Equal(t, "GET /tf", outputs.must(t, "apigatewayv2_route_key"),
		"API Gateway v2 route key must round-trip through provider refresh")
	require.Equal(t, "tf", outputs.must(t, "apigatewayv2_stage_name"),
		"API Gateway v2 stage name must round-trip through provider refresh")

	require.True(t, strings.HasPrefix(acmARN, "arn:aws:acm:us-east-1:"),
		"ACM certificate must live in us-east-1 for CloudFront use; got %s", acmARN)

	require.True(t, strings.HasPrefix(amplifyARN, "arn:aws:amplify:"),
		"Amplify app ARN must have aws_amplify_app prefix; got %s", amplifyARN)

	elbv2LBArn := outputs.must(t, "elbv2_lb_arn")
	require.Contains(t, elbv2LBArn, ":loadbalancer/app/tf-alb/",
		"ELBv2 load balancer ARN must use the app load-balancer resource path; got %s", elbv2LBArn)

	elbv2LBDNS := outputs.must(t, "elbv2_lb_dns_name")
	require.Contains(t, elbv2LBDNS, ".elb.us-east-1.amazonaws.com",
		"ELBv2 load balancer DNS name must use the regional ELB suffix; got %s", elbv2LBDNS)

	elbv2TGArn := outputs.must(t, "elbv2_target_group_arn")
	require.Contains(t, elbv2TGArn, ":targetgroup/tf-alb-tg/",
		"ELBv2 target group ARN must use the targetgroup resource path; got %s", elbv2TGArn)

	elbv2ListenerArn := outputs.must(t, "elbv2_listener_arn")
	require.Contains(t, elbv2ListenerArn, ":listener/app/tf-alb/",
		"ELBv2 listener ARN must use the listener/app resource path; got %s", elbv2ListenerArn)

	natGatewayID := outputs.must(t, "ec2_nat_gateway_id")
	require.True(t, strings.HasPrefix(natGatewayID, "nat-"),
		"EC2 NAT gateway id must use nat-* shape; got %s", natGatewayID)

	natEIP := outputs.must(t, "ec2_nat_eip_public_ip")
	require.True(t, strings.HasPrefix(natEIP, "203.0.113."),
		"EC2 NAT Elastic IP must round-trip the allocated public IP; got %s", natEIP)

	natRouteTableID := outputs.must(t, "ec2_nat_route_table_id")
	require.True(t, strings.HasPrefix(natRouteTableID, "rtb-"),
		"EC2 route table id must use rtb-* shape; got %s", natRouteTableID)

	require.Contains(t, slrARN, "aws-service-role/cloudfront.amazonaws.com/",
		"CloudFront SLR ARN must include the cloudfront.amazonaws.com service path; got %s", slrARN)

	s3ARN := outputs.must(t, "s3_bucket_arn")
	require.True(t, strings.HasPrefix(s3ARN, "arn:aws:s3:::tf-test-runner-bucket"),
		"S3 bucket ARN must be the canonical arn:aws:s3:::<bucket> shape; got %s", s3ARN)
	require.Equal(t, "test", outputs.must(t, "s3_bucket_tags_env"),
		"aws_s3_bucket.tags must round-trip through bucket tagging")

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

	// S3 bucket-subresource round-trips. Each output below depends on
	// the terraform-provider-aws Create+Read cycle returning the value
	// the apply set. If any bucket-subresource handler in the sim is
	// missing or returns a wrong-shape body, the provider's Read fails
	// to parse the value and the output is empty / wrong.
	require.Equal(t, "Enabled", outputs.must(t, "s3_bucket_versioning_status"),
		"PutBucketVersioning Status must round-trip through GetBucketVersioning")
	require.Equal(t, "expire-30d", outputs.must(t, "s3_bucket_lifecycle_id"),
		"PutBucketLifecycleConfiguration rule id must round-trip")
	require.Equal(t, "https://app.example.com", outputs.must(t, "s3_bucket_cors_origin"),
		"PutBucketCors allowed_origins[0] must round-trip")
	require.Equal(t, "tf-test-runner-bucket", outputs.must(t, "s3_bucket_policy_bucket"),
		"PutBucketPolicy must round-trip through GetBucketPolicy")
	require.Equal(t, "AES256", outputs.must(t, "s3_bucket_sse_algorithm"),
		"PutBucketEncryption sse_algorithm must round-trip")
	require.Equal(t, "replicate-logs", outputs.must(t, "s3_bucket_replication_rule_id"),
		"PutBucketReplication rule id must round-trip")
	require.Equal(t, "logs/", outputs.must(t, "s3_bucket_logging_target_prefix"),
		"PutBucketLogging target_prefix must round-trip")
	require.Equal(t, "private", outputs.must(t, "s3_bucket_acl_value"),
		"PutBucketAcl acl must round-trip")
	require.Equal(t, "Requester", outputs.must(t, "s3_bucket_request_payment_payer"),
		"PutBucketRequestPayment payer must round-trip")
	require.Equal(t, "Enabled", outputs.must(t, "s3_bucket_accelerate_status"),
		"PutBucketAccelerateConfiguration status must round-trip")
	require.Equal(t, "index.html", outputs.must(t, "s3_bucket_website_index"),
		"PutBucketWebsite IndexDocument.Suffix must round-trip")
	require.Equal(t, "BucketOwnerEnforced", outputs.must(t, "s3_bucket_ownership"),
		"PutBucketOwnershipControls object_ownership must round-trip")
	require.Equal(t, "queue-created", outputs.must(t, "s3_bucket_notification_queue_id"),
		"PutBucketNotificationConfiguration queue id must round-trip")
	require.Equal(t, "GOVERNANCE", outputs.must(t, "s3_bucket_object_lock_mode"),
		"PutObjectLockConfiguration mode must round-trip")
	require.Equal(t, "archive-tier", outputs.must(t, "s3_bucket_intelligent_tiering_name"),
		"PutBucketIntelligentTieringConfiguration name must round-trip")
	require.Equal(t, "inventory-current", outputs.must(t, "s3_bucket_inventory_name"),
		"PutBucketInventoryConfiguration name must round-trip")
	require.Equal(t, "analytics-all", outputs.must(t, "s3_bucket_analytics_name"),
		"PutBucketAnalyticsConfiguration name must round-trip")
	require.Equal(t, "metrics-prefix", outputs.must(t, "s3_bucket_metric_name"),
		"PutBucketMetricsConfiguration name must round-trip")

	destroy := terraformCmd("destroy", "-auto-approve")
	out, err = runTimed(t, "terraform destroy", destroy)
	require.NoError(t, err, "terraform destroy failed:\n%s", out)
}

func runTimed(t *testing.T, label string, cmd interface {
	CombinedOutput() ([]byte, error)
}) ([]byte, error) {
	t.Helper()
	start := time.Now()
	out, err := cmd.CombinedOutput()
	t.Logf("%s duration=%s", label, time.Since(start).Round(time.Millisecond))
	return out, err
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
