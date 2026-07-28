package main

import (
	"encoding/json"
	"strings"
	"time"
)

func lambdaAsyncQualifier(identifier, queryQualifier string) string {
	if queryQualifier != "" {
		return queryQualifier
	}
	if marker := strings.Index(identifier, ":function:"); marker >= 0 {
		identifier = identifier[marker+len(":function:"):]
	}
	if separator := strings.IndexByte(identifier, ':'); separator >= 0 {
		return identifier[separator+1:]
	}
	return "$LATEST"
}

func lambdaEventInvokeConfig(functionName, qualifier string) (LambdaEventInvokeConfig, bool) {
	lambdaEICMu.Lock()
	defer lambdaEICMu.Unlock()
	config, ok := lambdaEICs[lambdaEICKey(functionName, qualifier)]
	return config, ok
}

func lambdaInvokeAsynchronously(function LambdaFunction, payload []byte, qualifier string) {
	requestID := generateUUID()
	started := time.Now().UTC()
	config, configured := lambdaEventInvokeConfig(function.FunctionName, qualifier)
	maxRetries := 2
	maxAge := 6 * 60 * 60
	if configured && config.MaximumRetryAttempts != nil {
		maxRetries = *config.MaximumRetryAttempts
	}
	if configured && config.MaximumEventAgeInSeconds != nil {
		maxAge = *config.MaximumEventAgeInSeconds
	}

	var response []byte
	var unhandled bool
	invokeCount := 0
	condition := "Success"
	for attempt := 0; ; attempt++ {
		if time.Since(started) > time.Duration(maxAge)*time.Second {
			unhandled = true
			condition = "EventAgeExceeded"
			break
		}
		invokeCount++
		response, unhandled, _ = invokeLambdaViaRuntimeAPI(function, payload)
		if !unhandled {
			break
		}
		if attempt >= maxRetries {
			condition = "RetriesExhausted"
			break
		}
		// AWS Lambda retries function errors twice, one minute and then two
		// minutes after the preceding attempt.
		delay := time.Minute
		if attempt > 0 {
			delay = 2 * time.Minute
		}
		time.Sleep(delay)
	}

	if !configured || config.DestinationConfig == nil {
		return
	}
	var destination *lambdaDestination
	if unhandled {
		destination = config.DestinationConfig.OnFailure
	} else {
		destination = config.DestinationConfig.OnSuccess
	}
	if destination == nil || destination.Destination == "" {
		return
	}

	var requestPayload any
	if json.Unmarshal(payload, &requestPayload) != nil {
		requestPayload = string(payload)
	}
	var responsePayload any
	if json.Unmarshal(response, &responsePayload) != nil {
		responsePayload = string(response)
	}
	responseContext := map[string]any{
		"statusCode":      200,
		"executedVersion": function.Version,
	}
	if unhandled {
		responseContext["functionError"] = "Unhandled"
	}
	record := map[string]any{
		"version":   "1.0",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"requestContext": map[string]any{
			"requestId":              requestID,
			"functionArn":            function.FunctionArn,
			"condition":              condition,
			"approximateInvokeCount": invokeCount,
		},
		"requestPayload":  requestPayload,
		"responseContext": responseContext,
		"responsePayload": responsePayload,
	}
	body, err := json.Marshal(record)
	if err != nil {
		return
	}
	lambdaDeliverAsyncDestination(destination.Destination, body)
}

func lambdaDeliverAsyncDestination(destinationARN string, body []byte) {
	switch {
	case strings.HasPrefix(destinationARN, "arn:aws:sqs:"):
		queueName := snsTopicNameFromARN(destinationARN)
		if _, ok := sqsQueues.Get(queueName); ok {
			sqsEnqueueBody(queueName, string(body))
		}
	case strings.HasPrefix(destinationARN, "arn:aws:sns:"):
		if _, ok := snsTopics.Get(snsTopicNameFromARN(destinationARN)); ok {
			snsFanout(destinationARN, generateUUID(), "", string(body), nil)
		}
	case strings.HasPrefix(destinationARN, "arn:aws:events:"):
		_, _ = sfnInvokeJSONService(handleEBPutEvents, map[string]any{"Entries": []map[string]any{{
			"Source":       "lambda",
			"DetailType":   "Lambda Function Invocation Result",
			"Detail":       string(body),
			"EventBusName": destinationARN,
		}}})
	case strings.HasPrefix(destinationARN, "arn:aws:lambda:"):
		if target, _, ok := lambdaResolveInvocationTarget(destinationARN, ""); ok {
			_, _, _ = invokeLambdaViaRuntimeAPI(target, body)
		}
	}
}
