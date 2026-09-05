package awscommon

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// LoadConfig loads the AWS SDK configuration a backend builds its service
// clients from. region, when set, overrides the environment's region. A
// custom endpointURL is a coordinate for a cloud reached at another
// address; the AWS Signature Version 4 signer still needs a credential to
// sign with, so a static one is supplied when none of the standard
// credential sources apply. The caller passes endpointURL to each client's
// BaseEndpoint.
func LoadConfig(ctx context.Context, region, endpointURL string) (aws.Config, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	if endpointURL != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		))
	}
	return awsconfig.LoadDefaultConfig(ctx, opts...)
}
