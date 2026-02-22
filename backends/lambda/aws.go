package lambda

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
)

// AWSClients holds all AWS SDK clients for the Lambda backend.
type AWSClients struct {
	Lambda     *lambda.Client
	CloudWatch *cloudwatchlogs.Client
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
	}
}

func newClientsWithEndpoint(cfg aws.Config, endpoint string) *AWSClients {
	return &AWSClients{
		Lambda:     lambda.NewFromConfig(cfg, func(o *lambda.Options) { o.BaseEndpoint = aws.String(endpoint) }),
		CloudWatch: cloudwatchlogs.NewFromConfig(cfg, func(o *cloudwatchlogs.Options) { o.BaseEndpoint = aws.String(endpoint) }),
	}
}
