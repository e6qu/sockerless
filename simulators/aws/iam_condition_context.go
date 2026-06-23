package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// Resource-scoped + service-specific IAM condition keys (#661). The evaluator
// (#660) already supports the operators; this feeds the request's target
// resource into the condition context so tag- and cluster-conditioned grants
// enforce faithfully:
//
//   - aws:ResourceTag/<k> (and the service-prefixed ec2:ResourceTag/<k>) — the
//     tags of the resource the request targets (e.g. the volume DeleteVolume
//     acts on), so a policy allowing the action only on resources carrying a
//     given tag matches when, and only when, the resource carries it.
//   - ecs:cluster — the cluster ARN an ECS task operation targets.
//   - aws:RequestTag/<k> + aws:TagKeys — the tags supplied on a tag-on-create /
//     CreateTags request.

// iamPopulateResourceConditionKeys augments ctx with the resource-scoped and
// service-specific condition keys implied by the request.
func iamPopulateResourceConditionKeys(r *http.Request, action string, ctx map[string][]string) {
	service := strings.SplitN(action, ":", 2)[0]
	switch service {
	case "ec2":
		iamPopulateEC2ResourceTags(r, ctx)
	case "ecs":
		iamPopulateECSCluster(r, ctx)
	}
	iamPopulateRequestTags(r, ctx)
}

// iamPopulateEC2ResourceTags resolves the tags of the EC2 resource the request
// targets and exposes them as aws:ResourceTag/<k> and ec2:ResourceTag/<k>.
func iamPopulateEC2ResourceTags(r *http.Request, ctx map[string][]string) {
	tags, ok := iamEC2RequestResourceTags(r)
	if !ok {
		return
	}
	for _, t := range tags {
		ctx["aws:ResourceTag/"+t.Key] = []string{t.Value}
		ctx["ec2:ResourceTag/"+t.Key] = []string{t.Value}
	}
}

// iamEC2RequestResourceTags returns the tags of the first EC2 resource the
// request references by id (volume / snapshot / instance / network interface).
func iamEC2RequestResourceTags(r *http.Request) ([]EC2Tag, bool) {
	for _, param := range []string{"VolumeId", "SnapshotId", "InstanceId", "InstanceId.1", "NetworkInterfaceId", "ResourceId", "ResourceId.1"} {
		id := r.FormValue(param)
		if id == "" {
			continue
		}
		switch {
		case strings.HasPrefix(id, "vol-"):
			if v, ok := ec2Volumes.Get(id); ok {
				return v.Tags, true
			}
		case strings.HasPrefix(id, "snap-"):
			if s, ok := ec2Snapshots.Get(id); ok {
				return s.Tags, true
			}
		case strings.HasPrefix(id, "i-"):
			if i, ok := ec2Instances.Get(id); ok {
				return i.Tags, true
			}
		case strings.HasPrefix(id, "eni-"):
			if e, ok := ec2NetworkInterfaces.Get(id); ok {
				return e.Tags, true
			}
		}
	}
	return nil, false
}

// iamPopulateECSCluster exposes ecs:cluster (the targeted cluster's ARN) for an
// ECS task operation. ECS is awsJson, so the cluster lives in the request body;
// the body is read and restored so the downstream handler still sees it.
func iamPopulateECSCluster(r *http.Request, ctx map[string][]string) {
	if r.Body == nil {
		return
	}
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || len(body) == 0 {
		return
	}
	var req struct {
		Cluster string `json:"cluster"`
	}
	if json.Unmarshal(body, &req) != nil {
		return
	}
	name := req.Cluster
	if name == "" {
		name = "default"
	}
	arn := name
	if !strings.HasPrefix(name, "arn:") {
		arn = ecsArn("cluster", name)
	}
	ctx["ecs:cluster"] = []string{arn}
}

// iamPopulateRequestTags exposes aws:RequestTag/<k> + aws:TagKeys from the tags
// supplied on a tag-on-create / CreateTags request (Tag.N.Key/Value form).
func iamPopulateRequestTags(r *http.Request, ctx map[string][]string) {
	tags := parseIndexedTags(r, "Tag")
	if len(tags) == 0 {
		return
	}
	var keys []string
	for _, t := range tags {
		ctx["aws:RequestTag/"+t.Key] = []string{t.Value}
		keys = append(keys, t.Key)
	}
	ctx["aws:TagKeys"] = keys
}
