package lambda

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
)

// AWSClients holds all AWS SDK clients for the Lambda backend.
type AWSClients struct {
	Lambda     *lambda.Client
	CloudWatch *cloudwatchlogs.Client
	ECR        *ecr.Client
	CodeBuild  *codebuild.Client
	S3         *s3.Client
	// EFS client backs named-volume provisioning via
	// awscommon.EFSManager (shared with ECS) + Function.FileSystemConfigs[]
	// attach on CreateFunction.
	EFS *efs.Client
	// ServiceDiscovery (Cloud Map) client backs the service-mesh
	// NetworkDiscovery driver. Lambda invocations resolve peers via
	// per-VPC namespaces created at NetworkCreate time; the per-
	// invocation IP isn't peer-reachable so the driver skips the
	// register-IP step (mirrors the cloudrun-jobs pattern).
	ServiceDiscovery *servicediscovery.Client
	// EC2 client backs VPC ID resolution from configured subnet IDs
	// (Cloud Map private DNS namespace creation requires VpcId).
	EC2 *ec2.Client
}

// NewAWSClients initializes AWS SDK clients from config.
func NewAWSClients(ctx context.Context, region string, endpointURL string) (*AWSClients, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	if endpointURL != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	if endpointURL != "" {
		return newClientsWithEndpoint(cfg, endpointURL), nil
	}
	return newClientsFromConfig(cfg), nil
}

func newClientsFromConfig(cfg aws.Config) *AWSClients {
	return &AWSClients{
		Lambda:           lambda.NewFromConfig(cfg),
		CloudWatch:       cloudwatchlogs.NewFromConfig(cfg),
		ECR:              ecr.NewFromConfig(cfg),
		CodeBuild:        codebuild.NewFromConfig(cfg),
		S3:               s3.NewFromConfig(cfg),
		EFS:              efs.NewFromConfig(cfg),
		ServiceDiscovery: servicediscovery.NewFromConfig(cfg),
		EC2:              ec2.NewFromConfig(cfg),
	}
}

func newClientsWithEndpoint(cfg aws.Config, endpoint string) *AWSClients {
	return &AWSClients{
		Lambda:           lambda.NewFromConfig(cfg, func(o *lambda.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		CloudWatch:       cloudwatchlogs.NewFromConfig(cfg, func(o *cloudwatchlogs.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		ECR:              ecr.NewFromConfig(cfg, func(o *ecr.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		CodeBuild:        codebuild.NewFromConfig(cfg, func(o *codebuild.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		S3:               s3.NewFromConfig(cfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		EFS:              efs.NewFromConfig(cfg, func(o *efs.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		ServiceDiscovery: servicediscovery.NewFromConfig(cfg, func(o *servicediscovery.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		EC2:              ec2.NewFromConfig(cfg, func(o *ec2.Options) { o.BaseEndpoint = aws.String(endpoint) }),
	}
}
