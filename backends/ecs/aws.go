package ecs

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
)

// AWSClients holds all AWS SDK clients.
type AWSClients struct {
	ECS              *ecs.Client
	CloudWatch       *cloudwatchlogs.Client
	EFS              *efs.Client
	ServiceDiscovery *servicediscovery.Client
	ECR              *ecr.Client
	EC2              *ec2.Client
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
		ECS:              ecs.NewFromConfig(cfg),
		CloudWatch:       cloudwatchlogs.NewFromConfig(cfg),
		EFS:              efs.NewFromConfig(cfg),
		ServiceDiscovery: servicediscovery.NewFromConfig(cfg),
		ECR:              ecr.NewFromConfig(cfg),
		EC2:              ec2.NewFromConfig(cfg),
	}
}

func newClientsWithEndpoint(cfg aws.Config, endpoint string) *AWSClients {
	return &AWSClients{
		ECS:              ecs.NewFromConfig(cfg, func(o *ecs.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		CloudWatch:       cloudwatchlogs.NewFromConfig(cfg, func(o *cloudwatchlogs.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		EFS:              efs.NewFromConfig(cfg, func(o *efs.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		ServiceDiscovery: servicediscovery.NewFromConfig(cfg, func(o *servicediscovery.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		ECR:              ecr.NewFromConfig(cfg, func(o *ecr.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		EC2:              ec2.NewFromConfig(cfg, func(o *ec2.Options) { o.BaseEndpoint = aws.String(endpoint) }),
	}
}
