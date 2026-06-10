package aws_sdk_test

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apigwv2types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	batchtypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	ktypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
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
