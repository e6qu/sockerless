package lambda

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// AWSClients holds all AWS SDK clients for the Lambda backend.
type AWSClients struct {
	Lambda     *lambda.Client
	CloudWatch *cloudwatchlogs.Client
	ECR        *ecr.Client
	CodeBuild  *codebuild.Client
	S3         *s3.Client
	// Phase 94b: EFS client backs named-volume provisioning via
	// awscommon.EFSManager (shared with ECS) + Function.FileSystemConfigs[]
	// attach on CreateFunction.
	EFS *efs.Client
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
		Lambda:     lambda.NewFromConfig(cfg),
		CloudWatch: cloudwatchlogs.NewFromConfig(cfg),
		ECR:        ecr.NewFromConfig(cfg),
		CodeBuild:  codebuild.NewFromConfig(cfg),
		S3:         s3.NewFromConfig(cfg),
		EFS:        efs.NewFromConfig(cfg),
	}
}

func newClientsWithEndpoint(cfg aws.Config, endpoint string) *AWSClients {
	return &AWSClients{
		Lambda:     lambda.NewFromConfig(cfg, func(o *lambda.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		CloudWatch: cloudwatchlogs.NewFromConfig(cfg, func(o *cloudwatchlogs.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		ECR:        ecr.NewFromConfig(cfg, func(o *ecr.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		CodeBuild:  codebuild.NewFromConfig(cfg, func(o *codebuild.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		S3:         s3.NewFromConfig(cfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		EFS:        efs.NewFromConfig(cfg, func(o *efs.Options) { o.BaseEndpoint = aws.String(endpoint) }),
	}
}
