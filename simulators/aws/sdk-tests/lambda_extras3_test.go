package aws_sdk_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lambdaFunctionArn builds the function ARN the sim mints for a given name.
func lambdaFunctionArn(name string) string {
	return "arn:aws:lambda:us-east-1:123456789012:function:" + name
}

// TestLambda_ConfigAndCode exercises GetFunctionConfiguration and
// UpdateFunctionCode against a real stored function.
func TestLambda_ConfigAndCode(t *testing.T) {
	lc := lambdaClient()
	fn := "extras3-config-fn"
	lambdaExtrasFunc(t, lc, fn)

	cfg, err := lc.GetFunctionConfiguration(ctx, &lambda.GetFunctionConfigurationInput{
		FunctionName: aws.String(fn),
	})
	require.NoError(t, err)
	assert.Equal(t, fn, aws.ToString(cfg.FunctionName))
	assert.Equal(t, lambdatypes.StateActive, cfg.State)

	upd, err := lc.UpdateFunctionCode(ctx, &lambda.UpdateFunctionCodeInput{
		FunctionName: aws.String(fn),
		ZipFile:      []byte("new-deployment-package-material"),
	})
	require.NoError(t, err)
	assert.Equal(t, fn, aws.ToString(upd.FunctionName))
	// A code swap re-stamps CodeSha256 and CodeSize.
	assert.NotEmpty(t, aws.ToString(upd.CodeSha256))

	// GetFunctionConfiguration for an unknown function is a 404.
	_, err = lc.GetFunctionConfiguration(ctx, &lambda.GetFunctionConfigurationInput{
		FunctionName: aws.String("extras3-no-such-fn"),
	})
	require.Error(t, err)
}

// TestLambda_FunctionScalingConfig exercises Put/Get of the per-function
// scaling config (2025-11-30).
func TestLambda_FunctionScalingConfig(t *testing.T) {
	lc := lambdaClient()
	fn := "extras3-scaling-fn"
	lambdaExtrasFunc(t, lc, fn)

	pub, err := lc.PublishVersion(ctx, &lambda.PublishVersionInput{FunctionName: aws.String(fn)})
	require.NoError(t, err)
	qual := aws.ToString(pub.Version)

	// Get before put: no scaling config set, but the call succeeds with the ARN.
	pre, err := lc.GetFunctionScalingConfig(ctx, &lambda.GetFunctionScalingConfigInput{
		FunctionName: aws.String(fn),
		Qualifier:    aws.String(qual),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, aws.ToString(pre.FunctionArn))

	_, err = lc.PutFunctionScalingConfig(ctx, &lambda.PutFunctionScalingConfigInput{
		FunctionName: aws.String(fn),
		Qualifier:    aws.String(qual),
		FunctionScalingConfig: &lambdatypes.FunctionScalingConfig{
			MinExecutionEnvironments: aws.Int32(1),
			MaxExecutionEnvironments: aws.Int32(5),
		},
	})
	require.NoError(t, err)

	got, err := lc.GetFunctionScalingConfig(ctx, &lambda.GetFunctionScalingConfigInput{
		FunctionName: aws.String(fn),
		Qualifier:    aws.String(qual),
	})
	require.NoError(t, err)
	require.NotNil(t, got.RequestedFunctionScalingConfig)
	assert.Equal(t, int32(1), aws.ToInt32(got.RequestedFunctionScalingConfig.MinExecutionEnvironments))
	assert.Equal(t, int32(5), aws.ToInt32(got.RequestedFunctionScalingConfig.MaxExecutionEnvironments))
	require.NotNil(t, got.AppliedFunctionScalingConfig)
	assert.Equal(t, int32(5), aws.ToInt32(got.AppliedFunctionScalingConfig.MaxExecutionEnvironments))
}

// TestLambda_ListFunctionsByCodeSigningConfig wires a function to a
// code-signing config and reads the membership back.
func TestLambda_ListFunctionsByCodeSigningConfig(t *testing.T) {
	lc := lambdaClient()
	fn := "extras3-csc-fn"
	lambdaExtrasFunc(t, lc, fn)

	csc, err := lc.CreateCodeSigningConfig(ctx, &lambda.CreateCodeSigningConfigInput{
		AllowedPublishers: &lambdatypes.AllowedPublishers{
			SigningProfileVersionArns: []string{
				"arn:aws:signer:us-east-1:123456789012:/signing-profiles/p/v",
			},
		},
	})
	require.NoError(t, err)
	cscArn := aws.ToString(csc.CodeSigningConfig.CodeSigningConfigArn)

	_, err = lc.PutFunctionCodeSigningConfig(ctx, &lambda.PutFunctionCodeSigningConfigInput{
		FunctionName:         aws.String(fn),
		CodeSigningConfigArn: aws.String(cscArn),
	})
	require.NoError(t, err)

	lst, err := lc.ListFunctionsByCodeSigningConfig(ctx, &lambda.ListFunctionsByCodeSigningConfigInput{
		CodeSigningConfigArn: aws.String(cscArn),
	})
	require.NoError(t, err)
	assert.Contains(t, lst.FunctionArns, lambdaFunctionArn(fn))

	_, _ = lc.DeleteFunctionCodeSigningConfig(ctx, &lambda.DeleteFunctionCodeSigningConfigInput{
		FunctionName: aws.String(fn),
	})
	_, _ = lc.DeleteCodeSigningConfig(ctx, &lambda.DeleteCodeSigningConfigInput{
		CodeSigningConfigArn: aws.String(cscArn),
	})
}

// TestLambda_CapacityProviders runs the full create/get/list/update/delete
// round-trip plus the function-versions read-back (2025-11-30).
func TestLambda_CapacityProviders(t *testing.T) {
	lc := lambdaClient()
	cpName := "extras3-cp"

	create, err := lc.CreateCapacityProvider(ctx, &lambda.CreateCapacityProviderInput{
		CapacityProviderName: aws.String(cpName),
		VpcConfig: &lambdatypes.CapacityProviderVpcConfig{
			SubnetIds:        []string{"subnet-0a1b2c3d"},
			SecurityGroupIds: []string{"sg-0a1b2c3d"},
		},
		PermissionsConfig: &lambdatypes.CapacityProviderPermissionsConfig{
			CapacityProviderOperatorRoleArn: aws.String("arn:aws:iam::123456789012:role/cp-operator"),
		},
		CapacityProviderScalingConfig: &lambdatypes.CapacityProviderScalingConfig{
			MaxVCpuCount: aws.Int32(8),
			ScalingMode:  lambdatypes.CapacityProviderScalingModeAuto,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, create.CapacityProvider)
	cpArn := aws.ToString(create.CapacityProvider.CapacityProviderArn)
	assert.NotEmpty(t, cpArn)
	assert.Equal(t, lambdatypes.CapacityProviderStateActive, create.CapacityProvider.State)

	t.Cleanup(func() {
		_, _ = lc.DeleteCapacityProvider(ctx, &lambda.DeleteCapacityProviderInput{
			CapacityProviderName: aws.String(cpName),
		})
	})

	get, err := lc.GetCapacityProvider(ctx, &lambda.GetCapacityProviderInput{
		CapacityProviderName: aws.String(cpName),
	})
	require.NoError(t, err)
	require.NotNil(t, get.CapacityProvider)
	assert.Equal(t, cpArn, aws.ToString(get.CapacityProvider.CapacityProviderArn))
	require.NotNil(t, get.CapacityProvider.VpcConfig)
	assert.Equal(t, []string{"subnet-0a1b2c3d"}, get.CapacityProvider.VpcConfig.SubnetIds)

	upd, err := lc.UpdateCapacityProvider(ctx, &lambda.UpdateCapacityProviderInput{
		CapacityProviderName: aws.String(cpName),
		CapacityProviderScalingConfig: &lambdatypes.CapacityProviderScalingConfig{
			MaxVCpuCount: aws.Int32(16),
			ScalingMode:  lambdatypes.CapacityProviderScalingModeManual,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, upd.CapacityProvider)
	require.NotNil(t, upd.CapacityProvider.CapacityProviderScalingConfig)
	assert.Equal(t, int32(16), aws.ToInt32(upd.CapacityProvider.CapacityProviderScalingConfig.MaxVCpuCount))

	lst, err := lc.ListCapacityProviders(ctx, &lambda.ListCapacityProvidersInput{})
	require.NoError(t, err)
	found := false
	for _, cp := range lst.CapacityProviders {
		if aws.ToString(cp.CapacityProviderArn) == cpArn {
			found = true
		}
	}
	assert.True(t, found, "created capacity provider should appear in the list")

	fv, err := lc.ListFunctionVersionsByCapacityProvider(ctx, &lambda.ListFunctionVersionsByCapacityProviderInput{
		CapacityProviderName: aws.String(cpName),
	})
	require.NoError(t, err)
	assert.Equal(t, cpArn, aws.ToString(fv.CapacityProviderArn))
	assert.Empty(t, fv.FunctionVersions)
}

// TestLambda_InvokeAsync exercises the deprecated InvokeAsync entry point (202).
func TestLambda_InvokeAsync(t *testing.T) {
	lc := lambdaClient()
	fn := "extras3-async-fn"
	lambdaExtrasFunc(t, lc, fn)

	out, err := lc.InvokeAsync(ctx, &lambda.InvokeAsyncInput{
		FunctionName: aws.String(fn),
		InvokeArgs:   strings.NewReader(`{"hello":"world"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, int32(202), out.Status)
}

// TestLambda_InvokeWithResponseStream exercises the streaming invoke entry
// point and reassembles the event stream the handler emits.
func TestLambda_InvokeWithResponseStream(t *testing.T) {
	lc := lambdaClient()
	fn := "extras3-stream-fn"
	lambdaExtrasFunc(t, lc, fn)

	out, err := lc.InvokeWithResponseStream(ctx, &lambda.InvokeWithResponseStreamInput{
		FunctionName: aws.String(fn),
		Payload:      []byte(`{"hello":"world"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, int32(200), out.StatusCode)

	stream := out.GetStream()
	defer stream.Close()
	sawComplete := false
	for ev := range stream.Events() {
		switch ev.(type) {
		case *lambdatypes.InvokeWithResponseStreamResponseEventMemberPayloadChunk:
			// A chunk of the Zip-package function's {} response is fine.
		case *lambdatypes.InvokeWithResponseStreamResponseEventMemberInvokeComplete:
			sawComplete = true
		}
	}
	require.NoError(t, stream.Err())
	assert.True(t, sawComplete, "stream must terminate with an InvokeComplete event")
}

// TestLambda_DurableExecutionLifecycle drives the durable-execution surface:
// a checkpoint materializes the execution (the runtime checkpointing itself),
// then Get/History/State/List/Stop and a callback advance it.
func TestLambda_DurableExecutionLifecycle(t *testing.T) {
	lc := lambdaClient()
	fn := "extras3-durable-fn"
	lambdaExtrasFunc(t, lc, fn)

	// Construct a valid DurableExecutionArn rooted at the function.
	deArn := fmt.Sprintf("%s:$LATEST/durable-execution/order-saga/exec-001", lambdaFunctionArn(fn))
	callbackID := "cb-await-payment"

	// Get before any checkpoint: 404.
	_, err := lc.GetDurableExecution(ctx, &lambda.GetDurableExecutionInput{
		DurableExecutionArn: aws.String(deArn),
	})
	require.Error(t, err)

	// First checkpoint materializes the execution and records a CALLBACK op.
	cp, err := lc.CheckpointDurableExecution(ctx, &lambda.CheckpointDurableExecutionInput{
		DurableExecutionArn: aws.String(deArn),
		CheckpointToken:     aws.String("ckpt-1"),
		Updates: []lambdatypes.OperationUpdate{
			{
				Id:     aws.String("step-validate"),
				Name:   aws.String("validate"),
				Type:   lambdatypes.OperationTypeStep,
				Action: lambdatypes.OperationActionSucceed,
			},
			{
				Id:     aws.String(callbackID),
				Name:   aws.String("await-payment"),
				Type:   lambdatypes.OperationTypeCallback,
				Action: lambdatypes.OperationActionStart,
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, cp.NewExecutionState)
	assert.GreaterOrEqual(t, len(cp.NewExecutionState.Operations), 2)

	got, err := lc.GetDurableExecution(ctx, &lambda.GetDurableExecutionInput{
		DurableExecutionArn: aws.String(deArn),
	})
	require.NoError(t, err)
	assert.Equal(t, deArn, aws.ToString(got.DurableExecutionArn))
	assert.Equal(t, lambdatypes.ExecutionStatusRunning, got.Status)

	hist, err := lc.GetDurableExecutionHistory(ctx, &lambda.GetDurableExecutionHistoryInput{
		DurableExecutionArn: aws.String(deArn),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, hist.Events, "history must carry the ExecutionStarted event")

	state, err := lc.GetDurableExecutionState(ctx, &lambda.GetDurableExecutionStateInput{
		DurableExecutionArn: aws.String(deArn),
		CheckpointToken:     aws.String("ckpt-1"),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, state.Operations)

	lst, err := lc.ListDurableExecutionsByFunction(ctx, &lambda.ListDurableExecutionsByFunctionInput{
		FunctionName: aws.String(fn),
	})
	require.NoError(t, err)
	require.NotEmpty(t, lst.DurableExecutions)

	// The callback ops advance the stored execution.
	_, err = lc.SendDurableExecutionCallbackHeartbeat(ctx, &lambda.SendDurableExecutionCallbackHeartbeatInput{
		CallbackId: aws.String(callbackID),
	})
	require.NoError(t, err)
	_, err = lc.SendDurableExecutionCallbackSuccess(ctx, &lambda.SendDurableExecutionCallbackSuccessInput{
		CallbackId: aws.String(callbackID),
		Result:     []byte(`{"paid":true}`),
	})
	require.NoError(t, err)

	// An unknown callback id is a 404.
	_, err = lc.SendDurableExecutionCallbackFailure(ctx, &lambda.SendDurableExecutionCallbackFailureInput{
		CallbackId: aws.String("cb-does-not-exist"),
		Error:      &lambdatypes.ErrorObject{ErrorMessage: aws.String("nope")},
	})
	require.Error(t, err)

	stop, err := lc.StopDurableExecution(ctx, &lambda.StopDurableExecutionInput{
		DurableExecutionArn: aws.String(deArn),
		Error:               &lambdatypes.ErrorObject{ErrorMessage: aws.String("operator stop")},
	})
	require.NoError(t, err)
	require.NotNil(t, stop.StopTimestamp)

	after, err := lc.GetDurableExecution(ctx, &lambda.GetDurableExecutionInput{
		DurableExecutionArn: aws.String(deArn),
	})
	require.NoError(t, err)
	assert.Equal(t, lambdatypes.ExecutionStatusStopped, after.Status)
}
