package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// These pin the ARN the gate requests for the services BUG-2907 named. The
// assertions are the ARN shapes the AWS Service Reference publishes for each
// resource type, not what the code happens to build: an ARN that is merely
// self-consistent still denies every policy written against the real one.

func iamJSONRequest(target, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-amz-json-1.1")
	r.Header.Set("X-Amz-Target", target)
	r.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=ASIAEXAMPLECREDENTIAL/20260801/us-east-1/aws/aws4_request, SignedHeaders=host, Signature=00")
	return r
}

func iamQueryRequest(action, version string, params map[string]string) *http.Request {
	form := "Action=" + action + "&Version=" + version
	for k, v := range params {
		form += "&" + k + "=" + v
	}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=ASIAEXAMPLECREDENTIAL/20260801/us-east-1/iam/aws4_request, SignedHeaders=host, Signature=00")
	return r
}

func assertDerivedARNs(t *testing.T, r *http.Request, wantAction string, want ...string) {
	t.Helper()
	action, ok := iamActionForRequest(r)
	if !ok {
		t.Fatalf("request was not classified as an IAM action")
	}
	if action != wantAction {
		t.Fatalf("action = %q, want %q", action, wantAction)
	}
	got := iamResourceARNsForRequest(r, action)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%s resources = %v, want %v", action, got, want)
	}
}

func TestIAMResourceARNs_ECS(t *testing.T) {
	const p = "arn:aws:ecs:us-east-1:123456789012:"
	t.Run("RunTask names its task definition, not its cluster", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("AmazonEC2ContainerServiceV20141113.RunTask",
				`{"cluster":"edd","taskDefinition":"edd-control-plane:7"}`),
			"ecs:RunTask", p+"task-definition/edd-control-plane:7")
	})
	t.Run("a service ARN embeds its cluster", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("AmazonEC2ContainerServiceV20141113.DescribeServices",
				`{"cluster":"edd","services":["control-plane","ssh-gateway"]}`),
			"ecs:DescribeServices", p+"service/edd/control-plane", p+"service/edd/ssh-gateway")
	})
	t.Run("an omitted cluster is the default cluster", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("AmazonEC2ContainerServiceV20141113.StopTask", `{"task":"abc123"}`),
			"ecs:StopTask", p+"task/default/abc123")
	})
	t.Run("ExecuteCommand names both the cluster and the task", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("AmazonEC2ContainerServiceV20141113.ExecuteCommand",
				`{"cluster":"edd","task":"abc123","command":"sh","interactive":true}`),
			"ecs:ExecuteCommand", p+"task/edd/abc123", p+"cluster/edd")
	})
	t.Run("DescribeContainerInstances does not also demand the cluster", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("AmazonEC2ContainerServiceV20141113.DescribeContainerInstances",
				`{"cluster":"edd","containerInstances":["i-1"]}`),
			"ecs:DescribeContainerInstances", p+"container-instance/edd/i-1")
	})
	t.Run("an operation AWS declares no resource type for stays *", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("AmazonEC2ContainerServiceV20141113.DescribeTaskDefinition",
				`{"taskDefinition":"edd-control-plane:7"}`),
			"ecs:DescribeTaskDefinition", "*")
	})
	t.Run("tagging names the resource by its own ARN", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("AmazonEC2ContainerServiceV20141113.TagResource",
				`{"resourceArn":"`+p+`cluster/edd","tags":[]}`),
			"ecs:TagResource", p+"cluster/edd")
	})
}

// RegisterTaskDefinition authorizes against the task definition it is about to
// create, so the requested ARN carries the revision the call will be assigned.
func TestIAMResourceARNs_ECSRegisterTaskDefinitionCarriesTheNextRevision(t *testing.T) {
	ecsRevisionMu.Lock()
	if ecsRevisions == nil {
		ecsRevisions = map[string]int{}
	}
	ecsRevisions["edd-control-plane"] = 6
	ecsRevisionMu.Unlock()
	t.Cleanup(func() {
		ecsRevisionMu.Lock()
		delete(ecsRevisions, "edd-control-plane")
		ecsRevisionMu.Unlock()
	})
	assertDerivedARNs(t,
		iamJSONRequest("AmazonEC2ContainerServiceV20141113.RegisterTaskDefinition",
			`{"family":"edd-control-plane","containerDefinitions":[]}`),
		"ecs:RegisterTaskDefinition",
		"arn:aws:ecs:us-east-1:123456789012:task-definition/edd-control-plane:7")
}

// The four stream-scoped actions authorize against the log stream; everything
// else that names a group authorizes against the group. The group ARN carries
// no trailing ":*" — that suffix appears in some API responses, never in the
// resource an authorization request names.
func TestIAMResourceARNs_CloudWatchLogs(t *testing.T) {
	const group = "arn:aws:logs:us-east-1:123456789012:log-group:/aws/ecs/edd"
	t.Run("PutLogEvents names the stream", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("Logs_20140328.PutLogEvents",
				`{"logGroupName":"/aws/ecs/edd","logStreamName":"control-plane/abc","logEvents":[]}`),
			"logs:PutLogEvents", group+":log-stream:control-plane/abc")
	})
	t.Run("FilterLogEvents names the group with no trailing wildcard", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("Logs_20140328.FilterLogEvents", `{"logGroupName":"/aws/ecs/edd"}`),
			"logs:FilterLogEvents", group)
	})
	t.Run("DescribeLogStreams is group-scoped even though it is about streams", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("Logs_20140328.DescribeLogStreams",
				`{"logGroupName":"/aws/ecs/edd","logStreamNamePrefix":"control-plane"}`),
			"logs:DescribeLogStreams", group)
	})
	t.Run("DescribeLogGroups supports no resource-level permission", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("Logs_20140328.DescribeLogGroups", `{}`),
			"logs:DescribeLogGroups", "*")
	})
}

func TestIAMResourceARNs_CodeBuild(t *testing.T) {
	const p = "arn:aws:codebuild:us-east-1:123456789012:"
	t.Run("StartBuild names its project", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("CodeBuild_20161006.StartBuild", `{"projectName":"edd-image-source"}`),
			"codebuild:StartBuild", p+"project/edd-image-source")
	})
	t.Run("a build id resolves to the project that owns it", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("CodeBuild_20161006.BatchGetBuilds",
				`{"ids":["edd-image-source:0e3a1f2c-1111-2222-3333-444455556666"]}`),
			"codebuild:BatchGetBuilds", p+"project/edd-image-source")
	})
	t.Run("ListProjects supports no resource-level permission", func(t *testing.T) {
		assertDerivedARNs(t,
			iamJSONRequest("CodeBuild_20161006.ListProjects", `{}`),
			"codebuild:ListProjects", "*")
	})
}

func TestIAMResourceARNs_WAFv2(t *testing.T) {
	t.Run("GetIPSet names the ip set, not the web ACL", func(t *testing.T) {
		r := iamJSONRequest("AWSWAF_20190729.GetIPSet",
			`{"Name":"edd-allow","Scope":"REGIONAL","Id":"aaaa-bbbb-cccc"}`)
		assertDerivedARNs(t, r, "wafv2:GetIPSet",
			"arn:aws:wafv2:us-east-1:123456789012:regional/ipset/edd-allow/aaaa-bbbb-cccc")
	})
	t.Run("a CLOUDFRONT-scoped web ACL is global", func(t *testing.T) {
		r := iamJSONRequest("AWSWAF_20190729.GetWebACL",
			`{"Name":"edd-edge","Scope":"CLOUDFRONT","Id":"dddd-eeee-ffff"}`)
		assertDerivedARNs(t, r, "wafv2:GetWebACL",
			"arn:aws:wafv2:us-east-1:123456789012:global/webacl/edd-edge/dddd-eeee-ffff")
	})
}

func TestIAMResourceARNs_IAMIsGlobalAndCarriesNoRegion(t *testing.T) {
	assertDerivedARNs(t,
		iamQueryRequest("GetRole", "2010-05-08", map[string]string{"RoleName": "edd-control-plane"}),
		"iam:GetRole", "arn:aws:iam::123456789012:role/edd-control-plane")
	assertDerivedARNs(t,
		iamQueryRequest("GetUser", "2010-05-08", map[string]string{"UserName": "deployer"}),
		"iam:GetUser", "arn:aws:iam::123456789012:user/deployer")
}

// A role handed to a service is authorized separately, against the role's own
// ARN. Without it a PassRole statement scoped to specific roles means nothing,
// because nothing ever evaluates it.
func TestIAMPassedRoleARNsAreFoundWhereverTheRequestCarriesThem(t *testing.T) {
	cases := []struct {
		name, target, body string
		want               []string
	}{
		{
			"Amazon ECS task and execution roles, both at the top level",
			"AmazonEC2ContainerServiceV20141113.RegisterTaskDefinition",
			`{"family":"edd","taskRoleArn":"arn:aws:iam::123456789012:role/edd-task",
			  "executionRoleArn":"arn:aws:iam::123456789012:role/edd-exec"}`,
			[]string{"arn:aws:iam::123456789012:role/edd-exec", "arn:aws:iam::123456789012:role/edd-task"},
		},
		{
			"a role nested in an overrides object",
			"AmazonEC2ContainerServiceV20141113.RunTask",
			`{"taskDefinition":"edd:1","overrides":{"taskRoleArn":"arn:aws:iam::123456789012:role/edd-task"}}`,
			[]string{"arn:aws:iam::123456789012:role/edd-task"},
		},
		{
			"AWS CodeBuild names it serviceRole",
			"CodeBuild_20161006.CreateProject",
			`{"name":"edd","serviceRole":"arn:aws:iam::123456789012:role/edd-build"}`,
			[]string{"arn:aws:iam::123456789012:role/edd-build"},
		},
		{
			"a request that passes no role carries none",
			"CodeBuild_20161006.StartBuild", `{"projectName":"edd"}`,
			nil,
		},
		{
			"an ARN that is not a role is not a passed role",
			"CodeBuild_20161006.CreateProject",
			`{"name":"edd","encryptionKey":"arn:aws:kms:us-east-1:123456789012:key/abc"}`,
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := iamPassedRoleARNs(iamJSONRequest(tc.target, tc.body))
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("passed roles = %v, want %v", got, tc.want)
			}
		})
	}
}

// The operations that require PassRole are AWS's list, and the ones that read
// rather than create pass nothing.
func TestIAMPassRoleOperationsCarryTheServicePrincipal(t *testing.T) {
	for _, tc := range []struct {
		action string
		want   string
	}{
		{"ecs:RegisterTaskDefinition", "ecs-tasks.amazonaws.com"},
		{"codebuild:CreateProject", "codebuild.amazonaws.com"},
	} {
		principals, ok := iamPassRoleOperations[tc.action]
		if !ok {
			t.Fatalf("%s should require iam:PassRole", tc.action)
		}
		found := false
		for _, p := range principals {
			if p == tc.want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s iam:PassedToService = %v, want it to include %q", tc.action, principals, tc.want)
		}
	}
	if _, ok := iamPassRoleOperations["ecs:DescribeServices"]; ok {
		t.Error("a read operation passes no role and must not require iam:PassRole")
	}
}
