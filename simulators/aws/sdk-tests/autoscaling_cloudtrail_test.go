package aws_sdk_test

import (
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func autoScalingClient() *autoscaling.Client {
	return autoscaling.NewFromConfig(sdkConfig(), func(o *autoscaling.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func cloudTrailClient() *cloudtrail.Client {
	return cloudtrail.NewFromConfig(sdkConfig(), func(o *cloudtrail.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestAutoScalingGroupLifecycleSDK(t *testing.T) {
	asgClient := autoScalingClient()
	ec2Client := ec2Client()

	vpcOut, err := ec2Client.CreateVpc(ctx, &ec2.CreateVpcInput{CidrBlock: aws.String("10.90.0.0/16")})
	require.NoError(t, err)
	subnetOut, err := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
		VpcId:            vpcOut.Vpc.VpcId,
		CidrBlock:        aws.String("10.90.1.0/24"),
		AvailabilityZone: aws.String("us-east-1a"),
	})
	require.NoError(t, err)

	_, err = asgClient.CreateLaunchConfiguration(ctx, &autoscaling.CreateLaunchConfigurationInput{
		LaunchConfigurationName: aws.String("sdk-lc"),
		ImageId:                 aws.String("ami-asg1234"),
		InstanceType:            aws.String("t3.micro"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = asgClient.DeleteLaunchConfiguration(ctx, &autoscaling.DeleteLaunchConfigurationInput{
			LaunchConfigurationName: aws.String("sdk-lc"),
		})
	})

	lcOut, err := asgClient.DescribeLaunchConfigurations(ctx, &autoscaling.DescribeLaunchConfigurationsInput{
		LaunchConfigurationNames: []string{"sdk-lc"},
	})
	require.NoError(t, err)
	require.Len(t, lcOut.LaunchConfigurations, 1)
	assert.Equal(t, "ami-asg1234", aws.ToString(lcOut.LaunchConfigurations[0].ImageId))

	_, err = asgClient.CreateAutoScalingGroup(ctx, &autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName:    aws.String("sdk-asg"),
		LaunchConfigurationName: aws.String("sdk-lc"),
		MinSize:                 aws.Int32(1),
		MaxSize:                 aws.Int32(2),
		DesiredCapacity:         aws.Int32(1),
		VPCZoneIdentifier:       subnetOut.Subnet.SubnetId,
		Tags: []types.Tag{{
			Key:               aws.String("env"),
			Value:             aws.String("sdk"),
			PropagateAtLaunch: aws.Bool(true),
		}},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = asgClient.DeleteAutoScalingGroup(ctx, &autoscaling.DeleteAutoScalingGroupInput{
			AutoScalingGroupName: aws.String("sdk-asg"),
			ForceDelete:          aws.Bool(true),
		})
	})

	groupsOut, err := asgClient.DescribeAutoScalingGroups(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []string{"sdk-asg"},
	})
	require.NoError(t, err)
	require.Len(t, groupsOut.AutoScalingGroups, 1)
	require.Len(t, groupsOut.AutoScalingGroups[0].Instances, 1)
	instanceID := aws.ToString(groupsOut.AutoScalingGroups[0].Instances[0].InstanceId)
	require.NotEmpty(t, instanceID)

	instOut, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}})
	require.NoError(t, err)
	require.Len(t, instOut.Reservations, 1)
	assert.Equal(t, ec2types.InstanceStateNameRunning, instOut.Reservations[0].Instances[0].State.Name)

	_, err = asgClient.SetDesiredCapacity(ctx, &autoscaling.SetDesiredCapacityInput{
		AutoScalingGroupName: aws.String("sdk-asg"),
		DesiredCapacity:      aws.Int32(2),
	})
	require.NoError(t, err)

	groupsOut, err = asgClient.DescribeAutoScalingGroups(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []string{"sdk-asg"},
	})
	require.NoError(t, err)
	require.Len(t, groupsOut.AutoScalingGroups, 1)
	require.Len(t, groupsOut.AutoScalingGroups[0].Instances, 2)

	activitiesOut, err := asgClient.DescribeScalingActivities(ctx, &autoscaling.DescribeScalingActivitiesInput{
		AutoScalingGroupName: aws.String("sdk-asg"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, activitiesOut.Activities)
}

func TestCloudTrailRecordsAPICallsToS3SDK(t *testing.T) {
	ctClient := cloudTrailClient()
	s3Client := s3Client()
	ec2Client := ec2Client()
	bucket := "sdk-cloudtrail-bucket"

	_, err := s3Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)

	createOut, err := ctClient.CreateTrail(ctx, &cloudtrail.CreateTrailInput{
		Name:         aws.String("sdk-trail"),
		S3BucketName: aws.String(bucket),
		S3KeyPrefix:  aws.String("trail-logs"),
	})
	require.NoError(t, err)
	assert.Equal(t, "sdk-trail", aws.ToString(createOut.Name))
	assert.Contains(t, aws.ToString(createOut.TrailARN), ":trail/sdk-trail")

	statusOut, err := ctClient.GetTrailStatus(ctx, &cloudtrail.GetTrailStatusInput{Name: aws.String("sdk-trail")})
	require.NoError(t, err)
	assert.False(t, aws.ToBool(statusOut.IsLogging))

	_, err = ctClient.StartLogging(ctx, &cloudtrail.StartLoggingInput{Name: aws.String("sdk-trail")})
	require.NoError(t, err)

	statusOut, err = ctClient.GetTrailStatus(ctx, &cloudtrail.GetTrailStatusInput{Name: aws.String("sdk-trail")})
	require.NoError(t, err)
	assert.True(t, aws.ToBool(statusOut.IsLogging))

	_, err = ec2Client.CreateVpc(ctx, &ec2.CreateVpcInput{CidrBlock: aws.String("10.91.0.0/16")})
	require.NoError(t, err)

	eventsOut, err := ctClient.LookupEvents(ctx, &cloudtrail.LookupEventsInput{})
	require.NoError(t, err)
	foundCreateVpc := false
	for _, event := range eventsOut.Events {
		if aws.ToString(event.EventName) == "CreateVpc" {
			foundCreateVpc = true
			break
		}
	}
	assert.True(t, foundCreateVpc, "LookupEvents must include the recorded EC2 CreateVpc management event")

	objectsOut, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("trail-logs/AWSLogs/123456789012/CloudTrail/us-east-1/"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, objectsOut.Contents)

	getOut, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    objectsOut.Contents[0].Key,
	})
	require.NoError(t, err)
	defer getOut.Body.Close()
	body, err := io.ReadAll(getOut.Body)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(body), "\x1f\x8b"), "CloudTrail log objects are gzip streams")

	_, err = ctClient.StopLogging(ctx, &cloudtrail.StopLoggingInput{Name: aws.String("sdk-trail")})
	require.NoError(t, err)
	_, err = ctClient.DeleteTrail(ctx, &cloudtrail.DeleteTrailInput{Name: aws.String("sdk-trail")})
	require.NoError(t, err)
}
