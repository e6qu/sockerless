package lambda

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	core "github.com/sockerless/backend-core"
)

// ScanOrphanedResources discovers Sockerless-managed Lambda functions.
func (s *Server) ScanOrphanedResources(ctx context.Context, instanceID string) ([]core.ResourceEntry, error) {
	var orphans []core.ResourceEntry

	paginator := awslambda.NewListFunctionsPaginator(s.aws.Lambda, &awslambda.ListFunctionsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, fn := range page.Functions {
			arn := aws.ToString(fn.FunctionArn)
			tagsResult, err := s.aws.Lambda.ListTags(ctx, &awslambda.ListTagsInput{
				Resource: aws.String(arn),
			})
			if err != nil {
				continue
			}

			managed := tagsResult.Tags["sockerless-managed"] == "true"
			matchesInstance := tagsResult.Tags["sockerless-instance"] == instanceID

			if managed && matchesInstance {
				orphans = append(orphans, core.ResourceEntry{
					Backend:      "lambda",
					ResourceType: "function",
					ResourceID:   arn,
					InstanceID:   instanceID,
					CreatedAt:    time.Now(),
				})
			}
		}
	}

	return orphans, nil
}

// CleanupResource deletes a Lambda function.
func (s *Server) CleanupResource(ctx context.Context, entry core.ResourceEntry) error {
	_, err := s.aws.Lambda.DeleteFunction(ctx, &awslambda.DeleteFunctionInput{
		FunctionName: aws.String(entry.ResourceID),
	})
	return err
}
