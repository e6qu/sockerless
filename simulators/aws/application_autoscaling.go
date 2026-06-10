package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	sim "github.com/sockerless/simulator"
)

// Application Auto Scaling — terraform-provider-aws and the AWS SDK use this
// service (distinct from EC2 Auto Scaling in autoscaling.go) to scale ECS
// services, DynamoDB tables, Aurora replicas, and the like. Runner platform
// modules declare `aws_appautoscaling_target` + `aws_appautoscaling_policy`
// to autoscale the ECS service that backs sockerless. Wire format is the JSON
// protocol with `X-Amz-Target: AnyScaleFrontendService.<Action>`.

// AppScalableTarget is the registration that bounds a resource's capacity.
// The identity is the (ServiceNamespace, ResourceId, ScalableDimension)
// triple. Config blocks are held as raw JSON so they round-trip byte-exact.
type AppScalableTarget struct {
	ServiceNamespace  string            `json:"ServiceNamespace"`
	ResourceId        string            `json:"ResourceId"`
	ScalableDimension string            `json:"ScalableDimension"`
	MinCapacity       int               `json:"MinCapacity"`
	MaxCapacity       int               `json:"MaxCapacity"`
	RoleARN           string            `json:"RoleARN,omitempty"`
	CreationTime      float64           `json:"CreationTime"`
	ARN               string            `json:"ScalableTargetARN"`
	SuspendedState    json.RawMessage   `json:"SuspendedState,omitempty"`
	Tags              map[string]string `json:"Tags,omitempty"`
}

// AppScalingPolicy attaches a scaling rule to a scalable target. Identity is
// the target triple plus PolicyName.
type AppScalingPolicy struct {
	PolicyName        string          `json:"PolicyName"`
	PolicyARN         string          `json:"PolicyARN"`
	ServiceNamespace  string          `json:"ServiceNamespace"`
	ResourceId        string          `json:"ResourceId"`
	ScalableDimension string          `json:"ScalableDimension"`
	PolicyType        string          `json:"PolicyType"`
	TargetTracking    json.RawMessage `json:"TargetTrackingScalingPolicyConfiguration,omitempty"`
	StepScaling       json.RawMessage `json:"StepScalingPolicyConfiguration,omitempty"`
	CreationTime      float64         `json:"CreationTime"`
}

var (
	appScalableTargets sim.Store[AppScalableTarget]
	appScalingPolicies sim.Store[AppScalingPolicy]
)

func registerApplicationAutoScaling(r *sim.AWSRouter, srv *sim.Server) {
	appScalableTargets = sim.MakeStore[AppScalableTarget](srv.DB(), "app_scalable_targets")
	appScalingPolicies = sim.MakeStore[AppScalingPolicy](srv.DB(), "app_scaling_policies")

	r.Register("AnyScaleFrontendService.RegisterScalableTarget", handleAppASRegisterScalableTarget)
	r.Register("AnyScaleFrontendService.DeregisterScalableTarget", handleAppASDeregisterScalableTarget)
	r.Register("AnyScaleFrontendService.DescribeScalableTargets", handleAppASDescribeScalableTargets)
	r.Register("AnyScaleFrontendService.PutScalingPolicy", handleAppASPutScalingPolicy)
	r.Register("AnyScaleFrontendService.DeleteScalingPolicy", handleAppASDeleteScalingPolicy)
	r.Register("AnyScaleFrontendService.DescribeScalingPolicies", handleAppASDescribeScalingPolicies)
	r.Register("AnyScaleFrontendService.ListTagsForResource", handleAppASListTagsForResource)
	r.Register("AnyScaleFrontendService.TagResource", handleAppASTagResource)
	r.Register("AnyScaleFrontendService.UntagResource", handleAppASUntagResource)
}

// appScalableTargetKey is the storage key for the identity triple.
func appScalableTargetKey(ns, resourceID, dim string) string {
	return ns + "|" + resourceID + "|" + dim
}

func appScalingPolicyKey(ns, resourceID, dim, name string) string {
	return ns + "|" + resourceID + "|" + dim + "|" + name
}

func appScalableTargetARN(id string) string {
	return fmt.Sprintf("arn:aws:application-autoscaling:%s:%s:scalable-target/%s",
		awsRegion(), awsAccountID(), id)
}

// appScalingPolicyARN matches the real PolicyARN shape, which embeds the
// resource path and policy name.
func appScalingPolicyARN(ns, resourceID, name string) string {
	return fmt.Sprintf("arn:aws:autoscaling:%s:%s:scalingPolicy:%s:resource/%s/%s:policyName/%s",
		awsRegion(), awsAccountID(), generateUUID(), ns, resourceID, name)
}

func handleAppASRegisterScalableTarget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceNamespace  string            `json:"ServiceNamespace"`
		ResourceId        string            `json:"ResourceId"`
		ScalableDimension string            `json:"ScalableDimension"`
		MinCapacity       *int              `json:"MinCapacity"`
		MaxCapacity       *int              `json:"MaxCapacity"`
		RoleARN           string            `json:"RoleARN"`
		SuspendedState    json.RawMessage   `json:"SuspendedState"`
		Tags              map[string]string `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceNamespace == "" || req.ResourceId == "" || req.ScalableDimension == "" {
		sim.AWSError(w, "ValidationException",
			"ServiceNamespace, ResourceId, and ScalableDimension are required", http.StatusBadRequest)
		return
	}
	key := appScalableTargetKey(req.ServiceNamespace, req.ResourceId, req.ScalableDimension)

	// Register is upsert: an existing target keeps fields the caller omits.
	target, exists := appScalableTargets.Get(key)
	if !exists {
		target = AppScalableTarget{
			ServiceNamespace:  req.ServiceNamespace,
			ResourceId:        req.ResourceId,
			ScalableDimension: req.ScalableDimension,
			CreationTime:      float64(time.Now().Unix()),
			ARN:               appScalableTargetARN(generateUUID()),
		}
	}
	if req.MinCapacity != nil {
		target.MinCapacity = *req.MinCapacity
	}
	if req.MaxCapacity != nil {
		target.MaxCapacity = *req.MaxCapacity
	}
	if req.RoleARN != "" {
		target.RoleARN = req.RoleARN
	}
	if req.SuspendedState != nil {
		target.SuspendedState = req.SuspendedState
	}
	if req.Tags != nil {
		target.Tags = req.Tags
	}
	appScalableTargets.Put(key, target)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"ScalableTargetARN": target.ARN,
	})
}

func handleAppASDeregisterScalableTarget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceNamespace  string `json:"ServiceNamespace"`
		ResourceId        string `json:"ResourceId"`
		ScalableDimension string `json:"ScalableDimension"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	key := appScalableTargetKey(req.ServiceNamespace, req.ResourceId, req.ScalableDimension)
	if _, ok := appScalableTargets.Get(key); !ok {
		sim.AWSErrorf(w, "ObjectNotFoundException", http.StatusBadRequest,
			"No scalable target registered for %s/%s/%s",
			req.ServiceNamespace, req.ResourceId, req.ScalableDimension)
		return
	}
	appScalableTargets.Delete(key)
	// Deregistering a target removes its policies too.
	for _, p := range appScalingPolicies.List() {
		if p.ServiceNamespace == req.ServiceNamespace &&
			p.ResourceId == req.ResourceId &&
			p.ScalableDimension == req.ScalableDimension {
			appScalingPolicies.Delete(appScalingPolicyKey(p.ServiceNamespace, p.ResourceId, p.ScalableDimension, p.PolicyName))
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleAppASDescribeScalableTargets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceNamespace  string   `json:"ServiceNamespace"`
		ResourceIds       []string `json:"ResourceIds"`
		ScalableDimension string   `json:"ScalableDimension"`
		MaxResults        *int32   `json:"MaxResults"`
		NextToken         string   `json:"NextToken"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceNamespace == "" {
		sim.AWSError(w, "ValidationException", "ServiceNamespace is required", http.StatusBadRequest)
		return
	}
	wantIDs := map[string]bool{}
	for _, id := range req.ResourceIds {
		wantIDs[id] = true
	}
	matched := appScalableTargets.Filter(func(t AppScalableTarget) bool {
		if t.ServiceNamespace != req.ServiceNamespace {
			return false
		}
		if len(wantIDs) > 0 && !wantIDs[t.ResourceId] {
			return false
		}
		if req.ScalableDimension != "" && t.ScalableDimension != req.ScalableDimension {
			return false
		}
		return true
	})
	matched = sortBy(matched, func(t AppScalableTarget) string { return t.ResourceId })
	page, next := awsPageExplicit(matched, req.NextToken, awsMaxResults(req.MaxResults))

	out := make([]map[string]any, 0, len(page))
	for _, t := range page {
		out = append(out, scalableTargetToJSON(t))
	}
	resp := map[string]any{"ScalableTargets": out}
	if next != "" {
		resp["NextToken"] = next
	}
	sim.WriteJSON(w, http.StatusOK, resp)
}

func scalableTargetToJSON(t AppScalableTarget) map[string]any {
	m := map[string]any{
		"ServiceNamespace":  t.ServiceNamespace,
		"ResourceId":        t.ResourceId,
		"ScalableDimension": t.ScalableDimension,
		"MinCapacity":       t.MinCapacity,
		"MaxCapacity":       t.MaxCapacity,
		"CreationTime":      t.CreationTime,
		"ScalableTargetARN": t.ARN,
	}
	if t.RoleARN != "" {
		m["RoleARN"] = t.RoleARN
	}
	if t.SuspendedState != nil {
		m["SuspendedState"] = t.SuspendedState
	}
	return m
}

func handleAppASPutScalingPolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PolicyName        string          `json:"PolicyName"`
		ServiceNamespace  string          `json:"ServiceNamespace"`
		ResourceId        string          `json:"ResourceId"`
		ScalableDimension string          `json:"ScalableDimension"`
		PolicyType        string          `json:"PolicyType"`
		TargetTracking    json.RawMessage `json:"TargetTrackingScalingPolicyConfiguration"`
		StepScaling       json.RawMessage `json:"StepScalingPolicyConfiguration"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.PolicyName == "" || req.ServiceNamespace == "" || req.ResourceId == "" || req.ScalableDimension == "" {
		sim.AWSError(w, "ValidationException",
			"PolicyName, ServiceNamespace, ResourceId, and ScalableDimension are required", http.StatusBadRequest)
		return
	}
	// A policy requires a registered scalable target.
	targetKey := appScalableTargetKey(req.ServiceNamespace, req.ResourceId, req.ScalableDimension)
	if _, ok := appScalableTargets.Get(targetKey); !ok {
		sim.AWSErrorf(w, "ObjectNotFoundException", http.StatusBadRequest,
			"No scalable target registered for %s/%s/%s",
			req.ServiceNamespace, req.ResourceId, req.ScalableDimension)
		return
	}
	if req.PolicyType == "" {
		req.PolicyType = "StepScaling"
	}
	key := appScalingPolicyKey(req.ServiceNamespace, req.ResourceId, req.ScalableDimension, req.PolicyName)
	policy, exists := appScalingPolicies.Get(key)
	if !exists {
		policy = AppScalingPolicy{
			PolicyName:        req.PolicyName,
			PolicyARN:         appScalingPolicyARN(req.ServiceNamespace, req.ResourceId, req.PolicyName),
			ServiceNamespace:  req.ServiceNamespace,
			ResourceId:        req.ResourceId,
			ScalableDimension: req.ScalableDimension,
			CreationTime:      float64(time.Now().Unix()),
		}
	}
	policy.PolicyType = req.PolicyType
	policy.TargetTracking = req.TargetTracking
	policy.StepScaling = req.StepScaling
	appScalingPolicies.Put(key, policy)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"PolicyARN": policy.PolicyARN,
		"Alarms":    []any{},
	})
}

func handleAppASDeleteScalingPolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PolicyName        string `json:"PolicyName"`
		ServiceNamespace  string `json:"ServiceNamespace"`
		ResourceId        string `json:"ResourceId"`
		ScalableDimension string `json:"ScalableDimension"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	key := appScalingPolicyKey(req.ServiceNamespace, req.ResourceId, req.ScalableDimension, req.PolicyName)
	if _, ok := appScalingPolicies.Get(key); !ok {
		sim.AWSErrorf(w, "ObjectNotFoundException", http.StatusBadRequest,
			"No scaling policy named %q for %s/%s/%s",
			req.PolicyName, req.ServiceNamespace, req.ResourceId, req.ScalableDimension)
		return
	}
	appScalingPolicies.Delete(key)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleAppASDescribeScalingPolicies(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PolicyNames       []string `json:"PolicyNames"`
		ServiceNamespace  string   `json:"ServiceNamespace"`
		ResourceId        string   `json:"ResourceId"`
		ScalableDimension string   `json:"ScalableDimension"`
		MaxResults        *int32   `json:"MaxResults"`
		NextToken         string   `json:"NextToken"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceNamespace == "" {
		sim.AWSError(w, "ValidationException", "ServiceNamespace is required", http.StatusBadRequest)
		return
	}
	wantNames := map[string]bool{}
	for _, n := range req.PolicyNames {
		wantNames[n] = true
	}
	matched := appScalingPolicies.Filter(func(p AppScalingPolicy) bool {
		if p.ServiceNamespace != req.ServiceNamespace {
			return false
		}
		if len(wantNames) > 0 && !wantNames[p.PolicyName] {
			return false
		}
		if req.ResourceId != "" && p.ResourceId != req.ResourceId {
			return false
		}
		if req.ScalableDimension != "" && p.ScalableDimension != req.ScalableDimension {
			return false
		}
		return true
	})
	matched = sortBy(matched, func(p AppScalingPolicy) string { return p.PolicyName })
	page, next := awsPageExplicit(matched, req.NextToken, awsMaxResults(req.MaxResults))

	out := make([]map[string]any, 0, len(page))
	for _, p := range page {
		out = append(out, scalingPolicyToJSON(p))
	}
	resp := map[string]any{"ScalingPolicies": out}
	if next != "" {
		resp["NextToken"] = next
	}
	sim.WriteJSON(w, http.StatusOK, resp)
}

func scalingPolicyToJSON(p AppScalingPolicy) map[string]any {
	m := map[string]any{
		"PolicyARN":         p.PolicyARN,
		"PolicyName":        p.PolicyName,
		"ServiceNamespace":  p.ServiceNamespace,
		"ResourceId":        p.ResourceId,
		"ScalableDimension": p.ScalableDimension,
		"PolicyType":        p.PolicyType,
		"CreationTime":      p.CreationTime,
		"Alarms":            []any{},
	}
	if p.TargetTracking != nil {
		m["TargetTrackingScalingPolicyConfiguration"] = p.TargetTracking
	}
	if p.StepScaling != nil {
		m["StepScalingPolicyConfiguration"] = p.StepScaling
	}
	return m
}

// appScalableTargetByARN finds the target whose ARN matches, for the tag ops
// (which address resources by ScalableTargetARN).
func appScalableTargetByARN(arn string) (string, AppScalableTarget, bool) {
	for _, t := range appScalableTargets.List() {
		if t.ARN == arn {
			return appScalableTargetKey(t.ServiceNamespace, t.ResourceId, t.ScalableDimension), t, true
		}
	}
	return "", AppScalableTarget{}, false
}

func handleAppASListTagsForResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceARN string `json:"ResourceARN"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	_, target, ok := appScalableTargetByARN(req.ResourceARN)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"No resource found for ARN %q", req.ResourceARN)
		return
	}
	tags := target.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Tags": tags})
}

func handleAppASTagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceARN string            `json:"ResourceARN"`
		Tags        map[string]string `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	key, target, ok := appScalableTargetByARN(req.ResourceARN)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"No resource found for ARN %q", req.ResourceARN)
		return
	}
	if target.Tags == nil {
		target.Tags = map[string]string{}
	}
	for k, v := range req.Tags {
		target.Tags[k] = v
	}
	appScalableTargets.Put(key, target)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleAppASUntagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceARN string   `json:"ResourceARN"`
		TagKeys     []string `json:"TagKeys"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	key, target, ok := appScalableTargetByARN(req.ResourceARN)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"No resource found for ARN %q", req.ResourceARN)
		return
	}
	for _, k := range req.TagKeys {
		delete(target.Tags, k)
	}
	appScalableTargets.Put(key, target)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}
