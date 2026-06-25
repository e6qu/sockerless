package aws_sdk_test

import (
	"bufio"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestELBv2_NLBRawTCPRoundTrip drives the Network Load Balancer raw-TCP data
// plane end-to-end through the real sim: create an NLB + TCP target group + TCP
// listener, register a live raw-TCP target, then discover the reachable endpoint
// the way a real client does (DescribeLoadBalancers -> DNSName) and prove a raw
// byte stream round-trips through to the target (the SSH-through-NLB shape). No
// HTTP is involved on the data path.
func TestELBv2_NLBRawTCPRoundTrip(t *testing.T) {
	elb := elbv2Client()
	ec2c := ec2Client()

	vpc, err := ec2c.CreateVpc(ctx, &ec2.CreateVpcInput{CidrBlock: aws.String("10.160.0.0/16")})
	require.NoError(t, err)
	vpcID := aws.ToString(vpc.Vpc.VpcId)
	sn, err := ec2c.CreateSubnet(ctx, &ec2.CreateSubnetInput{
		VpcId: vpc.Vpc.VpcId, CidrBlock: aws.String("10.160.1.0/24"), AvailabilityZone: aws.String("us-east-1a"),
	})
	require.NoError(t, err)
	subnetID := aws.ToString(sn.Subnet.SubnetId)

	// A raw-TCP echo backend (not HTTP): reads a line, writes "echo:<line>".
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer backend.Close()
	go func() {
		for {
			conn, err := backend.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				reader := bufio.NewReader(c)
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if _, err := c.Write([]byte("echo:" + line)); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	backendHost, backendPortText, err := net.SplitHostPort(backend.Addr().String())
	require.NoError(t, err)
	backendPort, err := strconv.Atoi(backendPortText)
	require.NoError(t, err)

	nlb, err := elb.CreateLoadBalancer(ctx, &elbv2.CreateLoadBalancerInput{
		Name: aws.String("nlb-rawtcp"), Type: elbtypes.LoadBalancerTypeEnumNetwork, Subnets: []string{subnetID},
	})
	require.NoError(t, err)
	lbArn := aws.ToString(nlb.LoadBalancers[0].LoadBalancerArn)

	tg, err := elb.CreateTargetGroup(ctx, &elbv2.CreateTargetGroupInput{
		Name: aws.String("nlb-rawtcp-tg"), Protocol: elbtypes.ProtocolEnumTcp, Port: aws.Int32(int32(backendPort)),
		VpcId: aws.String(vpcID), TargetType: elbtypes.TargetTypeEnumIp,
	})
	require.NoError(t, err)
	tgArn := aws.ToString(tg.TargetGroups[0].TargetGroupArn)
	// A TCP target group carries no Matcher (issue #685).
	assert.Nil(t, tg.TargetGroups[0].Matcher, "TCP target group carries no Matcher")

	_, err = elb.RegisterTargets(ctx, &elbv2.RegisterTargetsInput{
		TargetGroupArn: aws.String(tgArn),
		Targets:        []elbtypes.TargetDescription{{Id: aws.String(backendHost), Port: aws.Int32(int32(backendPort))}},
	})
	require.NoError(t, err)

	_, err = elb.CreateListener(ctx, &elbv2.CreateListenerInput{
		LoadBalancerArn: aws.String(lbArn), Protocol: elbtypes.ProtocolEnumTcp, Port: aws.Int32(2222),
		DefaultActions: []elbtypes.Action{{Type: elbtypes.ActionTypeEnumForward, TargetGroupArn: aws.String(tgArn)}},
	})
	require.NoError(t, err)

	// Discover the reachable endpoint the same way a real client does.
	desc, err := elb.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{LoadBalancerArns: []string{lbArn}})
	require.NoError(t, err)
	endpoint := aws.ToString(desc.LoadBalancers[0].DNSName)
	require.NotEmpty(t, endpoint, "NLB DNSName must surface the reachable stream endpoint")

	conn, err := net.DialTimeout("tcp", endpoint, 5*time.Second)
	require.NoError(t, err, "dial NLB endpoint %s", endpoint)
	defer conn.Close()
	_, err = conn.Write([]byte("hello-nlb\n"))
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	got, err := bufio.NewReader(conn).ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "echo:hello-nlb\n", got, "raw TCP byte stream must round-trip through the NLB to the target")
}
