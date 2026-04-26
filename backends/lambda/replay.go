package lambda

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	core "github.com/sockerless/backend-core"
)

// Tag keys used to persist the most recent invocation outcome on a
// Lambda function. Source of truth for `Store.InvocationResults`
// across backend process restarts — see ReplayInvocationsFromCloudWatch.
const (
	tagLastExitCode   = "sockerless-last-exit-code"
	tagLastFinishedAt = "sockerless-last-finished-at"
	tagLastError      = "sockerless-last-error"
)

// persistInvocationResultToTags writes the InvocationResult to the
// Lambda function's tags so a future backend process can reconstruct
// `Store.InvocationResults` on startup. Tag values are length-capped
// at AWS's 256-char limit; errors here are logged but non-fatal — the
// in-memory `Store.InvocationResults` is the live truth and persistence
// is recovery scaffolding.
func (s *Server) persistInvocationResultToTags(ctx context.Context, functionARN string, inv core.InvocationResult) {
	if functionARN == "" || s.aws.Lambda == nil {
		return
	}
	errMsg := inv.Error
	if len(errMsg) > 250 {
		errMsg = errMsg[:250]
	}
	tags := map[string]string{
		tagLastExitCode:   strconv.Itoa(inv.ExitCode),
		tagLastFinishedAt: strconv.FormatInt(inv.FinishedAt.UTC().Unix(), 10),
	}
	if errMsg != "" {
		tags[tagLastError] = errMsg
	}
	if _, err := s.aws.Lambda.TagResource(ctx, &awslambda.TagResourceInput{
		Resource: aws.String(functionARN),
		Tags:     tags,
	}); err != nil {
		s.Logger.Warn().Err(err).Str("function", functionARN).Msg("failed to persist invocation result to tags")
	}
}

// ReplayInvocationsFromCloudWatch reconstructs `Store.InvocationResults`
// for every sockerless-managed Lambda function whose tags carry the
// `sockerless-last-*` entries written by `persistInvocationResultToTags`.
//
// Why: `Store.InvocationResults` is in-memory only. Across a backend
// process restart that map is empty, so `cloud_state.go::queryFunctions`
// falls through to the default `Running=true` branch — and `docker ps`
// reports every previously-invoked function as still running, with
// zero StartedAt/FinishedAt. We persist the outcome of each completed
// invocation to function tags (the single source of truth alongside the
// in-memory map) and read it back on startup.
//
// Name kept for compatibility with the original CloudWatch-based
// design; implementation now reads tags instead of CloudWatch logs
// because tag reads are atomic, exit codes survive losslessly, and
// we don't have to interpret AWS's runtime log strings.
func (s *Server) ReplayInvocationsFromCloudWatch(ctx context.Context) error {
	if s.aws.Lambda == nil {
		return nil
	}
	paginator := awslambda.NewListFunctionsPaginator(s.aws.Lambda, &awslambda.ListFunctionsInput{})
	replayed := 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("ListFunctions: %w", err)
		}
		for _, fn := range page.Functions {
			tagsResult, err := s.aws.Lambda.ListTags(ctx, &awslambda.ListTagsInput{
				Resource: fn.FunctionArn,
			})
			if err != nil || tagsResult.Tags["sockerless-managed"] != "true" {
				continue
			}
			containerID := tagsResult.Tags["sockerless-container-id"]
			if containerID == "" {
				continue
			}
			if _, exists := s.Store.GetInvocationResult(containerID); exists {
				continue
			}
			inv, ok := invocationResultFromTags(tagsResult.Tags)
			if !ok {
				continue
			}
			s.Store.PutInvocationResult(containerID, inv)
			replayed++
		}
	}
	if replayed > 0 {
		s.Logger.Info().Int("replayed", replayed).Msg("replayed Lambda invocation results from function tags")
	}
	return nil
}

func invocationResultFromTags(tags map[string]string) (core.InvocationResult, bool) {
	exitStr, ok := tags[tagLastExitCode]
	if !ok {
		return core.InvocationResult{}, false
	}
	exitCode, err := strconv.Atoi(exitStr)
	if err != nil {
		return core.InvocationResult{}, false
	}
	inv := core.InvocationResult{ExitCode: exitCode, Error: tags[tagLastError]}
	if finStr, ok := tags[tagLastFinishedAt]; ok {
		if sec, err := strconv.ParseInt(finStr, 10, 64); err == nil {
			inv.FinishedAt = time.Unix(sec, 0)
		}
	}
	return inv, true
}
