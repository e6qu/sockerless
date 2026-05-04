package lambda

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
)

// — Lambda function reuse pool ———————————————————————————————————————
//
// Mirrors the gcf pool design (specs/CLOUD_RESOURCE_MAPPING.md
// § Stateless image cache + Function/Site reuse pool). State lives
// entirely in cloud-side Lambda function tags — no local cache:
//
//   sockerless-managed=true            — sockerless-owned function
//   sockerless-overlay-hash=<tag>      — overlay content hash
//   sockerless-allocation=<containerID>— claimed by this container; missing/empty = free
//
// Lambda doesn't expose etag / RevisionId-based CAS on tag mutations the
// way GCP and Azure do. Concurrent claims from multiple sockerless
// instances would technically race; in single-instance deployments
// (the documented norm) the race is impossible. Operators running
// multiple sockerless backends against the same AWS account should
// either (a) pin POOL_MAX=0 to disable pool reuse, or (b) tolerate
// the rare double-claim — the second container's invoke runs against
// the same function, which is idempotent for sockerless's
// invoke-then-rm cycle.

// claimFreeFunction lists sockerless-managed Lambda functions for the
// given overlay-content-tag and claims one whose `sockerless-allocation`
// tag is empty. Lambda's `ListFunctions` doesn't filter by tags directly,
// so we iterate sockerless-managed functions client-side via
// `resourcegroupstaggingapi.GetResources`. Returns the claimed function's
// name on success, "" + nil if no free function exists.
func (s *Server) claimFreeFunction(ctx context.Context, contentTag, containerID, containerName string) (string, error) {
	// Lambda doesn't have a server-side "list-by-tag-filter" on its own
	// ListFunctions. The Resource Groups Tagging API does, but adds a
	// dependency surface we don't otherwise need. Iterating sockerless's
	// functions and filtering client-side is fine — the pool is bounded
	// by SOCKERLESS_LAMBDA_POOL_MAX (default 10) per overlay-hash.
	var marker *string
	for {
		out, err := s.aws.Lambda.ListFunctions(ctx, &awslambda.ListFunctionsInput{
			Marker:   marker,
			MaxItems: aws.Int32(50),
		})
		if err != nil {
			return "", fmt.Errorf("list functions for pool query: %w", err)
		}
		for _, fn := range out.Functions {
			fnName := aws.ToString(fn.FunctionName)
			fnArn := aws.ToString(fn.FunctionArn)
			tags, terr := s.aws.Lambda.ListTags(ctx, &awslambda.ListTagsInput{Resource: aws.String(fnArn)})
			if terr != nil {
				continue
			}
			if tags.Tags["sockerless-managed"] != "true" {
				continue
			}
			if tags.Tags["sockerless-overlay-hash"] != contentTag {
				continue
			}
			if alloc := tags.Tags["sockerless-allocation"]; alloc != "" {
				continue
			}
			// Claim by tagging. Lambda's TagResource is idempotent —
			// last-write-wins. With multi-instance concurrent claims a
			// later sockerless might win the same function; documented
			// limitation (see file header).
			_, err := s.aws.Lambda.TagResource(ctx, &awslambda.TagResourceInput{
				Resource: aws.String(fnArn),
				Tags: map[string]string{
					"sockerless-allocation":   shortAllocLabelLambda(containerID),
					"sockerless-name":         strings.TrimPrefix(containerName, "/"),
					"sockerless-container-id": containerID,
				},
			})
			if err != nil {
				continue
			}
			return fnName, nil
		}
		if out.NextMarker == nil || aws.ToString(out.NextMarker) == "" {
			break
		}
		marker = out.NextMarker
	}
	return "", nil
}

// releaseOrDeleteFunction is the pool-side counterpart to
// claimFreeFunction. On `docker rm <containerID>`:
//   - if the free pool size for this overlay-hash is >= POOL_MAX,
//     `DeleteFunction` (return-to-steady-state)
//   - otherwise `UntagResource` to clear the allocation tag (release
//     back to the pool for the next `docker run` of the same image).
func (s *Server) releaseOrDeleteFunction(ctx context.Context, fnName, contentTag string) error {
	// Count free functions for this overlay-hash.
	freeCount := 0
	var marker *string
	for {
		out, err := s.aws.Lambda.ListFunctions(ctx, &awslambda.ListFunctionsInput{
			Marker:   marker,
			MaxItems: aws.Int32(50),
		})
		if err != nil {
			s.Logger.Warn().Err(err).Msg("count free pool entries failed; defaulting to delete")
			break
		}
		for _, fn := range out.Functions {
			tags, terr := s.aws.Lambda.ListTags(ctx, &awslambda.ListTagsInput{Resource: fn.FunctionArn})
			if terr != nil {
				continue
			}
			if tags.Tags["sockerless-managed"] != "true" {
				continue
			}
			if tags.Tags["sockerless-overlay-hash"] != contentTag {
				continue
			}
			if alloc := tags.Tags["sockerless-allocation"]; alloc == "" {
				freeCount++
			}
		}
		if out.NextMarker == nil || aws.ToString(out.NextMarker) == "" {
			break
		}
		marker = out.NextMarker
	}

	if freeCount >= s.config.PoolMax {
		_, err := s.aws.Lambda.DeleteFunction(ctx, &awslambda.DeleteFunctionInput{
			FunctionName: aws.String(fnName),
		})
		if err != nil {
			return fmt.Errorf("delete function %s: %w", fnName, err)
		}
		return nil
	}

	// Release: clear allocation + per-claim tags.
	_, err := s.aws.Lambda.UntagResource(ctx, &awslambda.UntagResourceInput{
		Resource: aws.String(s.functionARN(fnName)),
		TagKeys: []string{
			"sockerless-allocation",
			"sockerless-name",
			"sockerless-container-id",
		},
	})
	if err != nil {
		return fmt.Errorf("release function %s: %w", fnName, err)
	}
	return nil
}

// functionARN constructs an ARN from a function name. Lambda accepts
// either name or ARN for most operations; UntagResource specifically
// requires an ARN, hence this helper.
func (s *Server) functionARN(fnName string) string {
	return fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s",
		s.config.Region, s.accountID(), fnName)
}

// accountID returns the AWS account ID extracted from the operator-
// configured RoleARN (which has the account ID embedded). Avoids
// adding an STS dependency for a value that's already known. Returns
// "*" as a fallback so a malformed RoleARN doesn't make pool ops
// crash — the resulting ARN will fail at the API gate with a clear
// "invalid ARN" error.
func (s *Server) accountID() string {
	parts := strings.Split(s.config.RoleARN, ":")
	if len(parts) >= 5 && parts[4] != "" {
		return parts[4]
	}
	return "*"
}

// shortAllocLabelLambda returns a tag-safe short form of a 64-char
// container ID. AWS tag values cap at 256 chars (well above 64), but
// shorter values keep tag query results cleaner and align with the
// gcf pool's GCP-label-safe shortening.
func shortAllocLabelLambda(containerID string) string {
	if len(containerID) > 32 {
		return containerID[:32]
	}
	return containerID
}
