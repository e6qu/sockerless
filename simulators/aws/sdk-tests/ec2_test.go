package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ec2Client() *ec2.Client {
	return ec2.NewFromConfig(sdkConfig(), func(o *ec2.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestEC2_CreateVpc(t *testing.T) {
	client := ec2Client()
	out, err := client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock: aws.String("10.0.0.0/16"),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, *out.Vpc.VpcId)
	assert.Equal(t, "10.0.0.0/16", *out.Vpc.CidrBlock)
}

func TestEC2_CreateSubnet(t *testing.T) {
	client := ec2Client()

	vpcOut, err := client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock: aws.String("10.1.0.0/16"),
	})
	require.NoError(t, err)

	out, err := client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
		VpcId:     vpcOut.Vpc.VpcId,
		CidrBlock: aws.String("10.1.1.0/24"),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, *out.Subnet.SubnetId)
	assert.Equal(t, *vpcOut.Vpc.VpcId, *out.Subnet.VpcId)
}

func TestEC2_SecurityGroup(t *testing.T) {
	client := ec2Client()

	vpcOut, err := client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock: aws.String("10.2.0.0/16"),
	})
	require.NoError(t, err)

	sgOut, err := client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String("test-sg"),
		Description: aws.String("test security group"),
		VpcId:       vpcOut.Vpc.VpcId,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, *sgOut.GroupId)

	descOut, err := client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		GroupIds: []string{*sgOut.GroupId},
	})
	require.NoError(t, err)
	require.Len(t, descOut.SecurityGroups, 1)
	assert.Equal(t, "test-sg", *descOut.SecurityGroups[0].GroupName)
}

func TestEC2_InternetGateway(t *testing.T) {
	client := ec2Client()

	igwOut, err := client.CreateInternetGateway(ctx, &ec2.CreateInternetGatewayInput{})
	require.NoError(t, err)
	assert.NotEmpty(t, *igwOut.InternetGateway.InternetGatewayId)

	descOut, err := client.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{
		InternetGatewayIds: []string{*igwOut.InternetGateway.InternetGatewayId},
	})
	require.NoError(t, err)
	require.Len(t, descOut.InternetGateways, 1)
}
