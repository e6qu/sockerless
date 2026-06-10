package aws_sdk_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apigwv2types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	batchtypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	ebtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	ktypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	wafv2types "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CreateAutoScalingGroup then DescribeAutoScalingGroups must surface the
// group ARN and the (defaulted) HealthCheckType, which terraform reads back.
func TestConformanceAutoScalingGroupARNAndHealthCheck(t *testing.T) {
	c := autoScalingClient()
	ec2c := ec2Client()
	lcName := "conf-asg-lc"
	asgName := "conf-asg-roundtrip"

	vpcOut, err := ec2c.CreateVpc(ctx, &ec2.CreateVpcInput{CidrBlock: aws.String("10.91.0.0/16")})
	require.NoError(t, err)
	subnetOut, err := ec2c.CreateSubnet(ctx, &ec2.CreateSubnetInput{
		VpcId:            vpcOut.Vpc.VpcId,
		CidrBlock:        aws.String("10.91.1.0/24"),
		AvailabilityZone: aws.String("us-east-1a"),
	})
	require.NoError(t, err)

	_, err = c.CreateLaunchConfiguration(ctx, &autoscaling.CreateLaunchConfigurationInput{
		LaunchConfigurationName: aws.String(lcName),
		ImageId:                 aws.String("ami-00000001"),
		InstanceType:            aws.String("t3.micro"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteLaunchConfiguration(ctx, &autoscaling.DeleteLaunchConfigurationInput{
			LaunchConfigurationName: aws.String(lcName),
		})
	})

	_, err = c.CreateAutoScalingGroup(ctx, &autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName:    aws.String(asgName),
		LaunchConfigurationName: aws.String(lcName),
		MinSize:                 aws.Int32(1),
		MaxSize:                 aws.Int32(3),
		DesiredCapacity:         aws.Int32(1),
		VPCZoneIdentifier:       subnetOut.Subnet.SubnetId,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteAutoScalingGroup(ctx, &autoscaling.DeleteAutoScalingGroupInput{
			AutoScalingGroupName: aws.String(asgName),
			ForceDelete:          aws.Bool(true),
		})
	})

	out, err := c.DescribeAutoScalingGroups(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []string{asgName},
	})
	require.NoError(t, err)
	require.Len(t, out.AutoScalingGroups, 1)
	g := out.AutoScalingGroups[0]
	arn := aws.ToString(g.AutoScalingGroupARN)
	require.NotEmpty(t, arn, "DescribeAutoScalingGroups must return AutoScalingGroupARN")
	assert.Contains(t, arn, ":autoScalingGroup:")
	assert.Equal(t, "EC2", aws.ToString(g.HealthCheckType))
}

// SubmitJob then DescribeJobs must return the same jobArn DescribeJobs is
// expected to echo (terraform/SDK read it back).
func TestConformanceBatchDescribeJobsArn(t *testing.T) {
	c := batchClient()

	_, err := c.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String("conf-batch-ce"),
		Type:                   batchtypes.CETypeManaged,
		State:                  batchtypes.CEStateEnabled,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteComputeEnvironment(ctx, &batch.DeleteComputeEnvironmentInput{
			ComputeEnvironment: aws.String("conf-batch-ce"),
		})
	})

	_, err = c.CreateJobQueue(ctx, &batch.CreateJobQueueInput{
		JobQueueName: aws.String("conf-batch-q"),
		Priority:     aws.Int32(1),
		ComputeEnvironmentOrder: []batchtypes.ComputeEnvironmentOrder{
			{Order: aws.Int32(1), ComputeEnvironment: aws.String("conf-batch-ce")},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteJobQueue(ctx, &batch.DeleteJobQueueInput{JobQueue: aws.String("conf-batch-q")})
	})

	reg, err := c.RegisterJobDefinition(ctx, &batch.RegisterJobDefinitionInput{
		JobDefinitionName: aws.String("conf-batch-jd"),
		Type:              batchtypes.JobDefinitionTypeContainer,
		ContainerProperties: &batchtypes.ContainerProperties{
			Image:   aws.String("public.ecr.aws/docker/library/busybox:latest"),
			Command: []string{"true"},
			ResourceRequirements: []batchtypes.ResourceRequirement{
				{Type: batchtypes.ResourceTypeVcpu, Value: aws.String("1")},
				{Type: batchtypes.ResourceTypeMemory, Value: aws.String("512")},
			},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeregisterJobDefinition(ctx, &batch.DeregisterJobDefinitionInput{
			JobDefinition: reg.JobDefinitionArn,
		})
	})

	submit, err := c.SubmitJob(ctx, &batch.SubmitJobInput{
		JobName:       aws.String("conf-batch-job"),
		JobQueue:      aws.String("conf-batch-q"),
		JobDefinition: reg.JobDefinitionArn,
	})
	require.NoError(t, err)
	jobID := aws.ToString(submit.JobId)
	require.NotEmpty(t, jobID)

	desc, err := c.DescribeJobs(ctx, &batch.DescribeJobsInput{Jobs: []string{jobID}})
	require.NoError(t, err)
	require.Len(t, desc.Jobs, 1)
	require.NotEmpty(t, aws.ToString(desc.Jobs[0].JobArn), "DescribeJobs must return jobArn")
}

// CreateRestApi must surface the auto-created root resource id, and that id
// must be usable as a CreateResource ParentId.
func TestConformanceAPIGatewayRootResourceId(t *testing.T) {
	c := apigwClient()
	create, err := c.CreateRestApi(ctx, &apigateway.CreateRestApiInput{
		Name: aws.String("conf-rest-root"),
	})
	require.NoError(t, err)
	apiId := aws.ToString(create.Id)
	require.NotEmpty(t, apiId)
	t.Cleanup(func() {
		_, _ = c.DeleteRestApi(ctx, &apigateway.DeleteRestApiInput{RestApiId: aws.String(apiId)})
	})

	rootId := aws.ToString(create.RootResourceId)
	require.NotEmpty(t, rootId, "CreateRestApi must return RootResourceId")

	get, err := c.GetRestApi(ctx, &apigateway.GetRestApiInput{RestApiId: aws.String(apiId)})
	require.NoError(t, err)
	assert.Equal(t, rootId, aws.ToString(get.RootResourceId))

	child, err := c.CreateResource(ctx, &apigateway.CreateResourceInput{
		RestApiId: aws.String(apiId),
		ParentId:  aws.String(rootId),
		PathPart:  aws.String("hello"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(child.Id))
}

// CreateApi (HTTP) must return ApiEndpoint.
func TestConformanceAPIGatewayV2ApiEndpoint(t *testing.T) {
	c := apigwv2Client()
	create, err := c.CreateApi(ctx, &apigatewayv2.CreateApiInput{
		Name:         aws.String("conf-http-api"),
		ProtocolType: apigwv2types.ProtocolTypeHttp,
	})
	require.NoError(t, err)
	apiId := aws.ToString(create.ApiId)
	require.NotEmpty(t, apiId)
	t.Cleanup(func() {
		_, _ = c.DeleteApi(ctx, &apigatewayv2.DeleteApiInput{ApiId: aws.String(apiId)})
	})
	require.NotEmpty(t, aws.ToString(create.ApiEndpoint), "CreateApi must return ApiEndpoint")
	assert.Contains(t, aws.ToString(create.ApiEndpoint), apiId)
}

// RequestCertificate without Options must default CT logging to ENABLED, and
// DescribeCertificate must return it.
func TestConformanceACMDefaultCertificateOptions(t *testing.T) {
	c := acmClient()
	req, err := c.RequestCertificate(ctx, &acm.RequestCertificateInput{
		DomainName:       aws.String("conf-acm.example.com"),
		ValidationMethod: acmtypes.ValidationMethodDns,
	})
	require.NoError(t, err)
	arn := aws.ToString(req.CertificateArn)
	require.NotEmpty(t, arn)
	t.Cleanup(func() {
		_, _ = c.DeleteCertificate(ctx, &acm.DeleteCertificateInput{CertificateArn: aws.String(arn)})
	})

	desc, err := c.DescribeCertificate(ctx, &acm.DescribeCertificateInput{CertificateArn: aws.String(arn)})
	require.NoError(t, err)
	require.NotNil(t, desc.Certificate)
	require.NotNil(t, desc.Certificate.Options, "DescribeCertificate must return Options")
	assert.Equal(t,
		acmtypes.CertificateTransparencyLoggingPreferenceEnabled,
		desc.Certificate.Options.CertificateTransparencyLoggingPreference)
}

// CreateStream (unencrypted) then DescribeStreamSummary must report
// EncryptionType=NONE, not the empty stored value.
func TestConformanceKinesisEncryptionTypeNone(t *testing.T) {
	c := kinesisClient()
	streamName := "conf-kinesis-enc"
	_, err := c.CreateStream(ctx, &kinesis.CreateStreamInput{
		StreamName: aws.String(streamName),
		ShardCount: aws.Int32(1),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteStream(ctx, &kinesis.DeleteStreamInput{StreamName: aws.String(streamName)})
	})

	summary, err := c.DescribeStreamSummary(ctx, &kinesis.DescribeStreamSummaryInput{
		StreamName: aws.String(streamName),
	})
	require.NoError(t, err)
	require.NotNil(t, summary.StreamDescriptionSummary)
	assert.Equal(t, ktypes.EncryptionTypeNone, summary.StreamDescriptionSummary.EncryptionType)
	assert.Empty(t, strings.TrimSpace(aws.ToString(summary.StreamDescriptionSummary.KeyId)))
}

// CreateFunction (Image package) with ImageConfig then GetFunction must
// surface the image config under ImageConfigResponse, the field the SDK
// FunctionConfiguration shape reads.
func TestConformanceLambdaImageConfigResponse(t *testing.T) {
	c := lambdaClient()
	fnName := "conf-lambda-imageconfig"
	_, err := c.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: aws.String(fnName),
		Role:         aws.String("arn:aws:iam::123456789012:role/test-role"),
		PackageType:  lambdatypes.PackageTypeImage,
		Code: &lambdatypes.FunctionCode{
			ImageUri: aws.String("123456789012.dkr.ecr.us-east-1.amazonaws.com/x:latest"),
		},
		ImageConfig: &lambdatypes.ImageConfig{
			Command:    []string{"/bin/sh"},
			EntryPoint: []string{"x"},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(fnName)})
	})

	out, err := c.GetFunction(ctx, &lambda.GetFunctionInput{FunctionName: aws.String(fnName)})
	require.NoError(t, err)
	require.NotNil(t, out.Configuration)
	require.NotNil(t, out.Configuration.ImageConfigResponse,
		"GetFunction must return ImageConfigResponse")
	require.NotNil(t, out.Configuration.ImageConfigResponse.ImageConfig,
		"ImageConfigResponse must carry ImageConfig")
	assert.Equal(t, []string{"/bin/sh"}, out.Configuration.ImageConfigResponse.ImageConfig.Command)
	assert.Equal(t, []string{"x"}, out.Configuration.ImageConfigResponse.ImageConfig.EntryPoint)
}

// UpdateItem must honor ReturnValues: default NONE returns no Attributes,
// ALL_NEW the full new item, UPDATED_NEW only the changed attrs, ALL_OLD
// the pre-update item.
func TestConformanceDynamoDBUpdateItemReturnValues(t *testing.T) {
	c := ddbClient()
	tableName := "conf-ddb-returnvalues"
	_, err := c.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []ddbtypes.AttributeDefinition{
			{AttributeName: aws.String("id"), AttributeType: ddbtypes.ScalarAttributeTypeS},
		},
		KeySchema: []ddbtypes.KeySchemaElement{
			{AttributeName: aws.String("id"), KeyType: ddbtypes.KeyTypeHash},
		},
		BillingMode: ddbtypes.BillingModePayPerRequest,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
	})

	_, err = c.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]ddbtypes.AttributeValue{
			"id":   &ddbtypes.AttributeValueMemberS{Value: "k1"},
			"val":  &ddbtypes.AttributeValueMemberS{Value: "before"},
			"keep": &ddbtypes.AttributeValueMemberS{Value: "same"},
		},
	})
	require.NoError(t, err)

	key := map[string]ddbtypes.AttributeValue{
		"id": &ddbtypes.AttributeValueMemberS{Value: "k1"},
	}

	// Default ReturnValues (NONE) -> no Attributes.
	noneOut, err := c.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(tableName),
		Key:                       key,
		UpdateExpression:          aws.String("SET #a = :v"),
		ExpressionAttributeNames:  map[string]string{"#a": "val"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":v": &ddbtypes.AttributeValueMemberS{Value: "n1"}},
	})
	require.NoError(t, err)
	assert.Empty(t, noneOut.Attributes, "default ReturnValues NONE must return no Attributes")

	// ALL_NEW -> full new item.
	allNewOut, err := c.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(tableName),
		Key:                       key,
		UpdateExpression:          aws.String("SET #a = :v"),
		ExpressionAttributeNames:  map[string]string{"#a": "val"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":v": &ddbtypes.AttributeValueMemberS{Value: "n2"}},
		ReturnValues:              ddbtypes.ReturnValueAllNew,
	})
	require.NoError(t, err)
	require.Contains(t, allNewOut.Attributes, "id")
	require.Contains(t, allNewOut.Attributes, "keep")
	valNew, ok := allNewOut.Attributes["val"].(*ddbtypes.AttributeValueMemberS)
	require.True(t, ok)
	assert.Equal(t, "n2", valNew.Value)

	// UPDATED_NEW -> only the changed attr (val), with the new value.
	updNewOut, err := c.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(tableName),
		Key:                       key,
		UpdateExpression:          aws.String("SET #a = :v"),
		ExpressionAttributeNames:  map[string]string{"#a": "val"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":v": &ddbtypes.AttributeValueMemberS{Value: "n3"}},
		ReturnValues:              ddbtypes.ReturnValueUpdatedNew,
	})
	require.NoError(t, err)
	require.Contains(t, updNewOut.Attributes, "val")
	assert.NotContains(t, updNewOut.Attributes, "keep", "UPDATED_NEW must omit unchanged attrs")
	assert.NotContains(t, updNewOut.Attributes, "id", "UPDATED_NEW must omit unchanged key attrs")
	valUpd, ok := updNewOut.Attributes["val"].(*ddbtypes.AttributeValueMemberS)
	require.True(t, ok)
	assert.Equal(t, "n3", valUpd.Value)

	// ALL_OLD -> pre-update item (val == n3 before this update).
	allOldOut, err := c.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(tableName),
		Key:                       key,
		UpdateExpression:          aws.String("SET #a = :v"),
		ExpressionAttributeNames:  map[string]string{"#a": "val"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":v": &ddbtypes.AttributeValueMemberS{Value: "n4"}},
		ReturnValues:              ddbtypes.ReturnValueAllOld,
	})
	require.NoError(t, err)
	valOld, ok := allOldOut.Attributes["val"].(*ddbtypes.AttributeValueMemberS)
	require.True(t, ok)
	assert.Equal(t, "n3", valOld.Value, "ALL_OLD must return the pre-update value")
}

// PutTargets with structured target parameters then ListTargetsByRule must
// round-trip EcsParameters and RetryPolicy, which terraform reads back.
func TestConformanceEventBridgeTargetParameters(t *testing.T) {
	eb := eventbridgeClient()
	busName := "conf-eb-target-bus"
	ruleName := "conf-eb-target-rule"

	_, err := eb.CreateEventBus(ctx, &eventbridge.CreateEventBusInput{Name: aws.String(busName)})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = eb.DeleteEventBus(ctx, &eventbridge.DeleteEventBusInput{Name: aws.String(busName)})
	})

	_, err = eb.PutRule(ctx, &eventbridge.PutRuleInput{
		Name:         aws.String(ruleName),
		EventBusName: aws.String(busName),
		EventPattern: aws.String(`{"source":["conf.test"]}`),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = eb.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
			Rule: aws.String(ruleName), EventBusName: aws.String(busName), Ids: []string{"t1"},
		})
		_, _ = eb.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
			Name: aws.String(ruleName), EventBusName: aws.String(busName),
		})
	})

	_, err = eb.PutTargets(ctx, &eventbridge.PutTargetsInput{
		Rule:         aws.String(ruleName),
		EventBusName: aws.String(busName),
		Targets: []ebtypes.Target{
			{
				Id:      aws.String("t1"),
				Arn:     aws.String("arn:aws:ecs:us-east-1:123456789012:cluster/conf"),
				RoleArn: aws.String("arn:aws:iam::123456789012:role/eb-role"),
				EcsParameters: &ebtypes.EcsParameters{
					TaskDefinitionArn: aws.String("arn:aws:ecs:us-east-1:123456789012:task-definition/x:1"),
					TaskCount:         aws.Int32(1),
					LaunchType:        ebtypes.LaunchTypeFargate,
				},
				RetryPolicy: &ebtypes.RetryPolicy{
					MaximumRetryAttempts: aws.Int32(2),
				},
			},
		},
	})
	require.NoError(t, err)

	out, err := eb.ListTargetsByRule(ctx, &eventbridge.ListTargetsByRuleInput{
		Rule:         aws.String(ruleName),
		EventBusName: aws.String(busName),
	})
	require.NoError(t, err)
	require.Len(t, out.Targets, 1)
	tgt := out.Targets[0]
	require.NotNil(t, tgt.EcsParameters, "ListTargetsByRule must round-trip EcsParameters")
	assert.Equal(t, "arn:aws:ecs:us-east-1:123456789012:task-definition/x:1",
		aws.ToString(tgt.EcsParameters.TaskDefinitionArn))
	require.NotNil(t, tgt.RetryPolicy, "ListTargetsByRule must round-trip RetryPolicy")
	assert.Equal(t, int32(2), aws.ToInt32(tgt.RetryPolicy.MaximumRetryAttempts))
}

// DescribeLoadBalancers/TargetGroups/Listeners with an explicit identifier
// that does not exist must raise the typed NotFound exception, not return an
// empty 200. A no-filter Describe (list all) must still succeed with whatever
// is present.
func TestConformanceELBv2DescribeMissingExplicitIdentifier(t *testing.T) {
	c := elbv2Client()

	_, err := c.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []string{
			"arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/nope/0",
		},
	})
	var lbNotFound *elbtypes.LoadBalancerNotFoundException
	require.Error(t, err)
	require.True(t, errors.As(err, &lbNotFound),
		"DescribeLoadBalancers with a missing ARN must raise LoadBalancerNotFoundException, got %v", err)

	_, err = c.DescribeTargetGroups(ctx, &elbv2.DescribeTargetGroupsInput{
		TargetGroupArns: []string{
			"arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/nope/0",
		},
	})
	var tgNotFound *elbtypes.TargetGroupNotFoundException
	require.Error(t, err)
	require.True(t, errors.As(err, &tgNotFound),
		"DescribeTargetGroups with a missing ARN must raise TargetGroupNotFoundException, got %v", err)

	_, err = c.DescribeListeners(ctx, &elbv2.DescribeListenersInput{
		ListenerArns: []string{
			"arn:aws:elasticloadbalancing:us-east-1:123456789012:listener/app/nope/0/0",
		},
	})
	var lsnNotFound *elbtypes.ListenerNotFoundException
	require.Error(t, err)
	require.True(t, errors.As(err, &lsnNotFound),
		"DescribeListeners with a missing ARN must raise ListenerNotFoundException, got %v", err)

	// A no-filter Describe (list all) must NOT error even when nothing matches.
	_, err = c.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{})
	require.NoError(t, err, "DescribeLoadBalancers with no filter must not error")
	_, err = c.DescribeTargetGroups(ctx, &elbv2.DescribeTargetGroupsInput{})
	require.NoError(t, err, "DescribeTargetGroups with no filter must not error")
	_, err = c.DescribeListeners(ctx, &elbv2.DescribeListenersInput{
		LoadBalancerArn: aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/listall/0"),
	})
	require.NoError(t, err, "DescribeListeners filtered by LoadBalancerArn must not error")
}

// DescribeServices against a cluster that does not exist must short-circuit
// with ClusterNotFoundException, not return per-service MISSING failures.
func TestConformanceECSDescribeServicesGhostCluster(t *testing.T) {
	c := ecsClient()

	_, err := c.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String("ghost-cluster"),
		Services: []string{"x"},
	})
	var clusterNotFound *ecstypes.ClusterNotFoundException
	require.Error(t, err)
	require.True(t, errors.As(err, &clusterNotFound),
		"DescribeServices against a missing cluster must raise ClusterNotFoundException, got %v", err)
}

// CreateRole with a name that already exists must raise
// EntityAlreadyExistsException on the second call.
func TestConformanceIAMCreateRoleDuplicate(t *testing.T) {
	c := iamClient()
	name := "conf-dup-role"
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

	_, err := c.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(name),
		AssumeRolePolicyDocument: aws.String(policy),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(name)})
	})

	_, err = c.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(name),
		AssumeRolePolicyDocument: aws.String(policy),
	})
	var dup *iamtypes.EntityAlreadyExistsException
	require.Error(t, err)
	require.True(t, errors.As(err, &dup),
		"CreateRole with a duplicate name must raise EntityAlreadyExistsException, got %v", err)
}

// Batch client errors must be typed as ClientException so the restJson1 SDK
// classifies them rather than falling back to a generic error.
func TestConformanceBatchTypedClientException(t *testing.T) {
	c := batchClient()
	name := "conf-dup-ce"

	_, err := c.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String(name),
		Type:                   batchtypes.CETypeUnmanaged,
		State:                  batchtypes.CEStateEnabled,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteComputeEnvironment(ctx, &batch.DeleteComputeEnvironmentInput{
			ComputeEnvironment: aws.String(name),
		})
	})

	_, err = c.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String(name),
		Type:                   batchtypes.CETypeUnmanaged,
		State:                  batchtypes.CEStateEnabled,
	})
	var clientErr *batchtypes.ClientException
	require.Error(t, err)
	require.True(t, errors.As(err, &clientErr),
		"Batch client error must be typed as ClientException, got %v", err)
}

// servicediscovery GetService for a missing id must return HTTP 400 (modeled
// awsJson1.1 exception) and deserialize to the typed ServiceNotFound.
func TestConformanceCloudMapServiceNotFoundStatus(t *testing.T) {
	c := cmClient()

	_, err := c.GetService(ctx, &servicediscovery.GetServiceInput{
		Id: aws.String("srv-missing000000000"),
	})
	require.Error(t, err)

	var svcNotFound *sdtypes.ServiceNotFound
	require.True(t, errors.As(err, &svcNotFound),
		"GetService for a missing id must raise ServiceNotFound, got %v", err)

	var respErr *awshttp.ResponseError
	require.True(t, errors.As(err, &respErr), "expected an HTTP ResponseError, got %v", err)
	assert.Equal(t, http.StatusBadRequest, respErr.HTTPStatusCode(),
		"servicediscovery modeled errors must be HTTP 400, not 404")
}

// CreateWebACL with a name+scope that already exists must raise
// WAFDuplicateItemException on the second call.
func TestConformanceWAFv2CreateWebACLDuplicate(t *testing.T) {
	c := wafClient()
	name := "conf-dup-acl"
	visibility := &wafv2types.VisibilityConfig{
		CloudWatchMetricsEnabled: true,
		MetricName:               aws.String(name),
		SampledRequestsEnabled:   true,
	}

	_, err := c.CreateWebACL(ctx, &wafv2.CreateWebACLInput{
		Name:             aws.String(name),
		Scope:            wafv2types.ScopeRegional,
		DefaultAction:    &wafv2types.DefaultAction{Allow: &wafv2types.AllowAction{}},
		VisibilityConfig: visibility,
	})
	require.NoError(t, err)

	_, err = c.CreateWebACL(ctx, &wafv2.CreateWebACLInput{
		Name:             aws.String(name),
		Scope:            wafv2types.ScopeRegional,
		DefaultAction:    &wafv2types.DefaultAction{Allow: &wafv2types.AllowAction{}},
		VisibilityConfig: visibility,
	})
	var dup *wafv2types.WAFDuplicateItemException
	require.Error(t, err)
	require.True(t, errors.As(err, &dup),
		"CreateWebACL with a duplicate name+scope must raise WAFDuplicateItemException, got %v", err)
}
