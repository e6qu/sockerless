package aws_cli_test

import (
	"strings"
	"testing"
)

// TestIAM_UserLifecycleAndEnforcementCLI covers the IAM user ops over the aws
// CLI (create-user / get-user / create-access-key / list-access-keys /
// put-user-policy / get-user-policy / attach-user-policy /
// list-attached-user-policies / detach-user-policy / delete-user-policy /
// delete-access-key / delete-user) and proves call-time enforcement (#657): a
// CLI call made with the restricted user's minted key is denied
// UnauthorizedOperation for an action its policy doesn't grant.
func TestIAM_UserLifecycleAndEnforcementCLI(t *testing.T) {
	user := "cli-restricted"
	runCLI(t, awsCLI("iam", "create-user", "--user-name", user))
	t.Cleanup(func() { _ = awsCLI("iam", "delete-user", "--user-name", user).Run() })

	out := runCLI(t, awsCLI("iam", "get-user", "--user-name", user, "--output", "json"))
	var gu struct {
		User struct {
			UserName string `json:"UserName"`
			Arn      string `json:"Arn"`
		} `json:"User"`
	}
	parseJSON(t, out, &gu)
	if gu.User.UserName != user {
		t.Fatalf("get-user returned %q", gu.User.UserName)
	}

	runCLI(t, awsCLI("iam", "put-user-policy", "--user-name", user,
		"--policy-name", "least-priv",
		"--policy-document", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ec2:DescribeVolumes","Resource":"*"}]}`))
	runCLI(t, awsCLI("iam", "get-user-policy", "--user-name", user, "--policy-name", "least-priv"))
	if lp := runCLI(t, awsCLI("iam", "list-user-policies", "--user-name", user, "--output", "json")); !strings.Contains(lp, "least-priv") {
		t.Fatalf("list-user-policies missing the inline policy: %s", lp)
	}

	runCLI(t, awsCLI("iam", "create-policy", "--policy-name", "cli-managed",
		"--policy-document", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sqs:SendMessage","Resource":"*"}]}`))
	arn := "arn:aws:iam::000000000000:policy/cli-managed"
	runCLI(t, awsCLI("iam", "attach-user-policy", "--user-name", user, "--policy-arn", arn))
	if la := runCLI(t, awsCLI("iam", "list-attached-user-policies", "--user-name", user, "--output", "json")); !strings.Contains(la, "cli-managed") {
		t.Fatalf("list-attached-user-policies missing the managed policy: %s", la)
	}
	runCLI(t, awsCLI("iam", "detach-user-policy", "--user-name", user, "--policy-arn", arn))
	runCLI(t, awsCLI("iam", "delete-user-policy", "--user-name", user, "--policy-name", "least-priv"))

	// Re-grant just DescribeVolumes, mint a key, and prove enforcement.
	runCLI(t, awsCLI("iam", "put-user-policy", "--user-name", user,
		"--policy-name", "least-priv",
		"--policy-document", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ec2:DescribeVolumes","Resource":"*"}]}`))
	out = runCLI(t, awsCLI("iam", "create-access-key", "--user-name", user, "--output", "json"))
	var ck struct {
		AccessKey struct {
			AccessKeyId     string `json:"AccessKeyId"`
			SecretAccessKey string `json:"SecretAccessKey"`
		} `json:"AccessKey"`
	}
	parseJSON(t, out, &ck)
	if ck.AccessKey.AccessKeyId == "" {
		t.Fatal("create-access-key returned no key id")
	}
	if lk := runCLI(t, awsCLI("iam", "list-access-keys", "--user-name", user, "--output", "json")); !strings.Contains(lk, ck.AccessKey.AccessKeyId) {
		t.Fatalf("list-access-keys missing the minted key: %s", lk)
	}

	// Denied action with the restricted key → UnauthorizedOperation.
	denyCmd := awsCLI("ec2", "create-volume", "--availability-zone", "us-east-1a", "--size", "1")
	denyCmd.Env = withCreds(denyCmd.Env, ck.AccessKey.AccessKeyId, ck.AccessKey.SecretAccessKey)
	deny := runCLIExpectError(t, denyCmd)
	if !strings.Contains(deny, "UnauthorizedOperation") {
		t.Fatalf("create-volume with restricted key expected UnauthorizedOperation; got: %s", deny)
	}

	// Allowed action with the restricted key → succeeds.
	allowCmd := awsCLI("ec2", "describe-volumes")
	allowCmd.Env = withCreds(allowCmd.Env, ck.AccessKey.AccessKeyId, ck.AccessKey.SecretAccessKey)
	runCLI(t, allowCmd)

	runCLI(t, awsCLI("iam", "delete-access-key", "--user-name", user, "--access-key-id", ck.AccessKey.AccessKeyId))
}

// withCreds replaces the AWS credential env entries so the CLI call is signed
// with the given (restricted) access key.
func withCreds(env []string, akid, secret string) []string {
	out := make([]string, 0, len(env)+2)
	for _, e := range env {
		if strings.HasPrefix(e, "AWS_ACCESS_KEY_ID=") || strings.HasPrefix(e, "AWS_SECRET_ACCESS_KEY=") {
			continue
		}
		out = append(out, e)
	}
	return append(out, "AWS_ACCESS_KEY_ID="+akid, "AWS_SECRET_ACCESS_KEY="+secret)
}
