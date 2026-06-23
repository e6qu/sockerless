package main

import "net/http"

// Service-initiation IAM condition keys (aws:ViaAWSService / aws:CalledVia /
// aws:SourceArn / aws:SourceAccount) and Service-principal matching.
//
// For a direct client call — the sim's request model — aws:ViaAWSService is
// false and the source keys are absent, exactly as in real AWS: a direct caller
// is not a service, so a resource policy that trusts a Service principal under a
// SourceArn condition correctly does not apply.
//
// When an AWS service originates the call on the principal's behalf (a sim
// internal event delivery — e.g. SNS → Lambda), the delivery records the source
// context (the originating service + the source resource ARN/account); this
// surfaces it as aws:CalledVia / aws:SourceArn / aws:SourceAccount and sets
// aws:ViaAWSService=true, and the resource-policy evaluator matches the
// statement's Principal:{Service:…} against the calling service.

type iamServiceSource struct {
	Service       string // e.g. "sns.amazonaws.com"
	SourceArn     string // the originating resource ARN
	SourceAccount string // the originating resource's account
}

// iamDeliverySource reports the originating-service context for a request that
// arrived at the front door. Direct client calls (everything that hits POST /)
// are never service-initiated, so this is nil: a service-initiated call does not
// arrive as a client request — it is the sim's internal event delivery (see
// snsDeliverToLambda), which authorizes the target's resource policy directly
// with iamEvalServiceInitiated rather than re-entering the gate.
func iamDeliverySource(_ *http.Request) *iamServiceSource {
	return nil
}

// iamAuthorizeServiceDelivery is the gate the sim's internal event delivery
// calls before delivering from a source resource to a target: it authorizes the
// originating service against the TARGET's resource-based policy (an SQS queue
// policy, a Lambda function policy, etc.) with the service condition context. A
// target with no resource policy denies (real AWS requires the target to grant
// the source service permission). targetArn keys the resource policy; action is
// the delivery action (e.g. sqs:SendMessage, lambda:InvokeFunction).
func iamAuthorizeServiceDelivery(targetArn, action string, src iamServiceSource) bool {
	docs := iamResourcePolicyDocsForARN(targetArn)
	if len(docs) == 0 {
		return false
	}
	return iamEvalServiceInitiated(docs, action, targetArn, src)
}

// iamEvalServiceInitiated authorizes an AWS-service-initiated call against the
// target's resource-based policy, with the originating-service condition context
// populated (aws:CalledVia / aws:SourceArn / aws:SourceAccount / aws:Via-
// AWSService=true). It returns true when the resource policy admits the service.
func iamEvalServiceInitiated(docs []iamPolicyDoc, action, resource string, src iamServiceSource) bool {
	ctx := map[string][]string{"aws:ViaAWSService": {"true"}}
	if src.Service != "" {
		ctx["aws:CalledVia"] = []string{src.Service}
	}
	if src.SourceArn != "" {
		ctx["aws:SourceArn"] = []string{src.SourceArn}
	}
	if src.SourceAccount != "" {
		ctx["aws:SourceAccount"] = []string{src.SourceAccount}
	}
	decision, _ := iamEvalDecisionForPrincipal(docs, action, resource, "service:"+src.Service, ctx)
	return decision == "allowed"
}

// iamServiceInitiation returns the originating-service context when the request
// was made by an AWS service on the principal's behalf, or nil for a direct
// client call. The carrier is the sim's internal event-delivery path
// (iamDeliverySource), which a delivering service stamps onto the request it
// makes to the target.
func iamServiceInitiation(r *http.Request) *iamServiceSource {
	return iamDeliverySource(r)
}

// iamPopulateServiceContext sets the service-initiation condition keys on ctx.
func iamPopulateServiceContext(r *http.Request, ctx map[string][]string) {
	src := iamServiceInitiation(r)
	if src == nil {
		ctx["aws:ViaAWSService"] = []string{"false"}
		return
	}
	ctx["aws:ViaAWSService"] = []string{"true"}
	if src.Service != "" {
		ctx["aws:CalledVia"] = []string{src.Service}
	}
	if src.SourceArn != "" {
		ctx["aws:SourceArn"] = []string{src.SourceArn}
	}
	if src.SourceAccount != "" {
		ctx["aws:SourceAccount"] = []string{src.SourceAccount}
	}
}
