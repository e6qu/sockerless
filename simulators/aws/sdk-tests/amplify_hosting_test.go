package aws_sdk_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/amplify"
	amplifytypes "github.com/aws/aws-sdk-go-v2/service/amplify/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Amplify hosting data plane + real build + Route 53 verification e2e.
// Hosting requests hit the sim's loopback address with a Host: header (the
// cross-platform pattern — *.amplifyapp.com does not resolve locally).

func amplifyHostGet(t *testing.T, host, path string, mutate func(*http.Request)) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	require.NoError(t, err)
	req.Host = host
	if mutate != nil {
		mutate(req)
	}
	client := &http.Client{
		// Redirect targets point at the virtual host; assert on the 3xx
		// itself instead of following it to the loopback base URL.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       2 * time.Minute,
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, body
}

func amplifyZipBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		f, err := zw.Create(name)
		require.NoError(t, err)
		_, err = f.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// amplifyDeployZip drives the manual-deploy flow: CreateDeployment →
// presigned PUT of the zip → StartDeployment → SUCCEED.
func amplifyDeployZip(t *testing.T, ctx context.Context, c *amplify.Client, appID, branch string, zipBytes []byte) string {
	t.Helper()
	createDep, err := c.CreateDeployment(ctx, &amplify.CreateDeploymentInput{
		AppId:      aws.String(appID),
		BranchName: aws.String(branch),
	})
	require.NoError(t, err)
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, aws.ToString(createDep.ZipUploadUrl), bytes.NewReader(zipBytes))
	require.NoError(t, err)
	putReq.Header.Set("Content-Type", "application/zip")
	putResp, err := http.DefaultClient.Do(putReq)
	require.NoError(t, err)
	putResp.Body.Close()
	require.Equal(t, http.StatusOK, putResp.StatusCode)
	jobID := aws.ToString(createDep.JobId)
	_, err = c.StartDeployment(ctx, &amplify.StartDeploymentInput{
		AppId:      aws.String(appID),
		BranchName: aws.String(branch),
		JobId:      aws.String(jobID),
	})
	require.NoError(t, err)
	amplifyWaitJobStatus(t, ctx, c, appID, branch, jobID, amplifytypes.JobStatusSucceed)
	return jobID
}

func TestAmplifyHostingStaticE2E(t *testing.T) {
	c := amplifyClient()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	app, err := c.CreateApp(ctx, &amplify.CreateAppInput{
		Name: aws.String("host-app-" + time.Now().Format("150405.000000")),
		CustomRules: []amplifytypes.CustomRule{
			{Source: aws.String("/old"), Target: aws.String("/about.html"), Status: aws.String("301")},
			{Source: aws.String("/<*>"), Target: aws.String("/index.html"), Status: aws.String("404-200")},
		},
	})
	require.NoError(t, err)
	appID := aws.ToString(app.App.AppId)
	defer func() { _, _ = c.DeleteApp(ctx, &amplify.DeleteAppInput{AppId: aws.String(appID)}) }()
	_, err = c.CreateBranch(ctx, &amplify.CreateBranchInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
	})
	require.NoError(t, err)
	host := "main." + appID + ".amplifyapp.com"

	// No deployment yet → the real no-content experience.
	resp, _ := amplifyHostGet(t, host, "/", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	amplifyDeployZip(t, ctx, c, appID, "main", amplifyZipBytes(t, map[string][]byte{
		"index.html": []byte("<html>hosted home</html>"),
		"about.html": []byte("<html>about page</html>"),
	}))

	// Content via the defaultDomain child host.
	resp, body := amplifyHostGet(t, host, "/", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "<html>hosted home</html>", string(body))
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	resp, body = amplifyHostGet(t, host, "/about.html", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "<html>about page</html>", string(body))

	// Custom rule: redirect.
	resp, _ = amplifyHostGet(t, host, "/old", nil)
	require.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
	assert.Equal(t, "/about.html", resp.Header.Get("Location"))

	// Custom rule: SPA fallback.
	resp, body = amplifyHostGet(t, host, "/client/side/route", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "<html>hosted home</html>", string(body))

	// Basic auth: enable on the branch, then 401 without and 200 with
	// credentials (wire format: base64 "user:password").
	creds := base64.StdEncoding.EncodeToString([]byte("user:hunter2"))
	_, err = c.UpdateBranch(ctx, &amplify.UpdateBranchInput{
		AppId:                aws.String(appID),
		BranchName:           aws.String("main"),
		EnableBasicAuth:      aws.Bool(true),
		BasicAuthCredentials: aws.String(creds),
	})
	require.NoError(t, err)
	resp, _ = amplifyHostGet(t, host, "/", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("WWW-Authenticate"), "Basic")
	resp, body = amplifyHostGet(t, host, "/", func(r *http.Request) {
		r.SetBasicAuth("user", "hunter2")
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "<html>hosted home</html>", string(body))
}

func TestAmplifyHostingSSRComputeE2E(t *testing.T) {
	c := amplifyClient()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	app, err := c.CreateApp(ctx, &amplify.CreateAppInput{
		Name:     aws.String("ssr-app-" + time.Now().Format("150405.000000")),
		Platform: amplifytypes.PlatformWebCompute,
	})
	require.NoError(t, err)
	appID := aws.ToString(app.App.AppId)
	defer func() { _, _ = c.DeleteApp(ctx, &amplify.DeleteAppInput{AppId: aws.String(appID)}) }()
	_, err = c.CreateBranch(ctx, &amplify.CreateBranchInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
	})
	require.NoError(t, err)

	manifest, err := json.Marshal(map[string]any{
		"version":   1,
		"framework": map[string]string{"name": "custom", "version": "1.0.0"},
		"routes": []map[string]any{
			{"path": "/*.*", "target": map[string]string{"kind": "Static"}},
			{"path": "/*", "target": map[string]string{"kind": "Compute", "src": "default"}},
		},
		"computeResources": []map[string]string{
			{"name": "default", "runtime": "nodejs20.x", "entrypoint": "index.js"},
		},
	})
	require.NoError(t, err)
	// Tiny node http server answering with dynamic content; the deployment
	// spec's compute entrypoints listen on PORT (3000).
	entrypoint := `const http = require('http');
http.createServer((req, res) => {
  res.setHeader('content-type', 'text/plain');
  res.end('ssr-dynamic:' + req.url);
}).listen(process.env.PORT);
`
	amplifyDeployZip(t, ctx, c, appID, "main", amplifyZipBytes(t, map[string][]byte{
		"deploy-manifest.json":     manifest,
		"compute/default/index.js": []byte(entrypoint),
		"static/assets/hello.txt":  []byte("static asset from compute bundle"),
	}))
	host := "main." + appID + ".amplifyapp.com"

	// Compute route → proxied to the bundle's node server (lazy cold start
	// on this first request).
	resp, body := amplifyHostGet(t, host, "/render/me", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "compute proxy: %s", body)
	assert.Equal(t, "ssr-dynamic:/render/me", string(body))

	// Static route from the same bundle (spec: static assets under static/).
	resp, body = amplifyHostGet(t, host, "/assets/hello.txt", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "static asset from compute bundle", string(body))

	// Query strings ride through the proxy.
	resp, body = amplifyHostGet(t, host, "/search?q=x", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ssr-dynamic:/search?q=x", string(body))
}

// amplifyServeGitRepo creates a single-commit git repository with the given
// files and serves it over smart HTTP (git http-backend via CGI) from the
// test process. Returns the clone URL.
func amplifyServeGitRepo(t *testing.T, branch string, files map[string]string) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skipf("git not found in PATH: %v", err)
	}
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	run := func(args ...string) {
		cmd := exec.Command(gitPath, args...)
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init", "-q", "-b", branch)
	for name, content := range files {
		dest := filepath.Join(workDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o755))
		require.NoError(t, os.WriteFile(dest, []byte(content), 0o644))
	}
	run("add", "-A")
	run("-c", "user.email=test@sockerless.local", "-c", "user.name=sockerless", "commit", "-q", "-m", "initial")

	bare := exec.Command(gitPath, "clone", "-q", "--bare", workDir, filepath.Join(root, "repo.git"))
	out, err := bare.CombinedOutput()
	require.NoError(t, err, "git clone --bare: %s", out)

	handler := &cgi.Handler{
		Path: gitPath,
		Args: []string{"http-backend"},
		Env: []string{
			"GIT_PROJECT_ROOT=" + root,
			"GIT_HTTP_EXPORT_ALL=1",
		},
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL + "/repo.git"
}

func TestAmplifyRealBuildE2E(t *testing.T) {
	c := amplifyClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	marker := fmt.Sprintf("build-%d", time.Now().UnixNano())
	repoURL := amplifyServeGitRepo(t, "main", map[string]string{
		"index.html": "<html>built site " + marker + "</html>",
		"data.txt":   "source file",
	})
	// Plain shell commands writing files — no npm needed.
	buildSpec := `version: 1
frontend:
  phases:
    preBuild:
      commands:
        - echo "prebuild ` + marker + `"
    build:
      commands:
        - mkdir -p dist
        - cp index.html dist/index.html
        - echo "built by $AWS_BRANCH on app $AWS_APP_ID" > dist/build-info.txt
  artifacts:
    baseDirectory: dist
    files:
      - '**/*'
`
	app, err := c.CreateApp(ctx, &amplify.CreateAppInput{
		Name:       aws.String("build-app-" + time.Now().Format("150405.000000")),
		Repository: aws.String(repoURL),
		BuildSpec:  aws.String(buildSpec),
	})
	require.NoError(t, err)
	appID := aws.ToString(app.App.AppId)
	defer func() { _, _ = c.DeleteApp(ctx, &amplify.DeleteAppInput{AppId: aws.String(appID)}) }()
	_, err = c.CreateBranch(ctx, &amplify.CreateBranchInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
	})
	require.NoError(t, err)

	startOut, err := c.StartJob(ctx, &amplify.StartJobInput{
		AppId:      aws.String(appID),
		BranchName: aws.String("main"),
		JobType:    amplifytypes.JobTypeRelease,
		CommitId:   aws.String("HEAD"),
	})
	require.NoError(t, err)
	jobID := aws.ToString(startOut.JobSummary.JobId)

	// The job must pass through RUNNING (a real build takes real time)
	// before landing SUCCEED.
	sawRunning := false
	var job *amplifytypes.Job
	require.Eventually(t, func() bool {
		out, err := c.GetJob(ctx, &amplify.GetJobInput{
			AppId: aws.String(appID), BranchName: aws.String("main"), JobId: aws.String(jobID),
		})
		if err != nil || out.Job.Summary == nil {
			return false
		}
		job = out.Job
		switch out.Job.Summary.Status {
		case amplifytypes.JobStatusRunning:
			sawRunning = true
			return false
		case amplifytypes.JobStatusSucceed:
			return true
		case amplifytypes.JobStatusFailed, amplifytypes.JobStatusCancelled:
			stepDump, _ := json.Marshal(out.Job.Steps)
			t.Fatalf("build job landed %s: %s", out.Job.Summary.Status, stepDump)
		}
		return false
	}, 4*time.Minute, 100*time.Millisecond, "build never succeeded")
	require.True(t, sawRunning, "job must be observable in RUNNING while the build executes")

	// Per-step logs are retrievable via each step's logUrl.
	require.Len(t, job.Steps, 3)
	stepLogs := map[string]string{}
	for _, step := range job.Steps {
		assert.Equal(t, amplifytypes.JobStatusSucceed, step.Status, "step %s", aws.ToString(step.StepName))
		logURL := aws.ToString(step.LogUrl)
		require.NotEmpty(t, logURL, "step %s must carry a logUrl", aws.ToString(step.StepName))
		logResp, err := http.Get(logURL)
		require.NoError(t, err)
		logBody, err := io.ReadAll(logResp.Body)
		logResp.Body.Close()
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, logResp.StatusCode)
		stepLogs[aws.ToString(step.StepName)] = string(logBody)
	}
	assert.Contains(t, stepLogs["PROVISION"], "Cloning repository")
	assert.Contains(t, stepLogs["BUILD"], "prebuild "+marker, "BUILD log must carry the command output")
	assert.Contains(t, stepLogs["DEPLOY"], "index.html")

	// The build artifact zip is the job's artifact…
	artifacts, err := c.ListArtifacts(ctx, &amplify.ListArtifactsInput{
		AppId: aws.String(appID), BranchName: aws.String("main"), JobId: aws.String(jobID),
	})
	require.NoError(t, err)
	require.Len(t, artifacts.Artifacts, 1)
	assert.Equal(t, "artifacts.zip", aws.ToString(artifacts.Artifacts[0].ArtifactFileName))

	// …and the hosting data plane serves it.
	host := "main." + appID + ".amplifyapp.com"
	resp, body := amplifyHostGet(t, host, "/", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "<html>built site "+marker+"</html>", string(body))
	resp, body = amplifyHostGet(t, host, "/build-info.txt", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "built by main on app "+appID)
	// Files outside the artifact baseDirectory are not deployed.
	resp, _ = amplifyHostGet(t, host, "/data.txt", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAmplifyDomainVerificationRoute53Flow(t *testing.T) {
	c := amplifyClient()
	r53 := r53Client()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	app, err := c.CreateApp(ctx, &amplify.CreateAppInput{
		Name: aws.String("verify-app-" + time.Now().Format("150405.000000")),
	})
	require.NoError(t, err)
	appID := aws.ToString(app.App.AppId)
	defer func() { _, _ = c.DeleteApp(ctx, &amplify.DeleteAppInput{AppId: aws.String(appID)}) }()
	_, err = c.CreateBranch(ctx, &amplify.CreateBranchInput{
		AppId: aws.String(appID), BranchName: aws.String("main"),
	})
	require.NoError(t, err)

	domain := fmt.Sprintf("verify-%d.example.dev", time.Now().UnixNano())
	dom, err := c.CreateDomainAssociation(ctx, &amplify.CreateDomainAssociationInput{
		AppId:      aws.String(appID),
		DomainName: aws.String(domain),
		SubDomainSettings: []amplifytypes.SubDomainSetting{
			{Prefix: aws.String("www"), BranchName: aws.String("main")},
		},
	})
	require.NoError(t, err)
	// AMPLIFY_MANAGED certificates start PENDING_VERIFICATION with the ACM
	// verification CNAME advertised.
	require.Equal(t, amplifytypes.DomainStatusPendingVerification, dom.DomainAssociation.DomainStatus)
	record := aws.ToString(dom.DomainAssociation.CertificateVerificationDNSRecord)
	parts := strings.Fields(record)
	require.Len(t, parts, 3, "verification record %q must be 'name CNAME value'", record)
	require.Equal(t, "CNAME", parts[1])
	require.True(t, strings.HasSuffix(parts[2], ".acm-validations.aws."), "value %q", parts[2])
	require.False(t, aws.ToBool(dom.DomainAssociation.SubDomains[0].Verified))
	// The subdomain CNAME targets the app's (deterministic) cloudfront host.
	assert.Contains(t, aws.ToString(dom.DomainAssociation.SubDomains[0].DnsRecord), ".cloudfront.net")

	// Without DNS records the association honestly stays pending.
	getOut, err := c.GetDomainAssociation(ctx, &amplify.GetDomainAssociationInput{
		AppId: aws.String(appID), DomainName: aws.String(domain),
	})
	require.NoError(t, err)
	require.Equal(t, amplifytypes.DomainStatusPendingVerification, getOut.DomainAssociation.DomainStatus)

	// Create the hosted zone + verification CNAME in the sim's Route 53 —
	// exactly what a real-world terraform config does.
	zoneOut, err := r53.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
		Name:            aws.String(domain),
		CallerReference: aws.String("amplify-verify-" + time.Now().Format("150405.000000")),
	})
	require.NoError(t, err)
	zoneID := strings.TrimPrefix(aws.ToString(zoneOut.HostedZone.Id), "/hostedzone/")
	_, err = r53.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{{
				Action: r53types.ChangeActionCreate,
				ResourceRecordSet: &r53types.ResourceRecordSet{
					Name: aws.String(parts[0]),
					Type: r53types.RRTypeCname,
					TTL:  aws.Int64(300),
					ResourceRecords: []r53types.ResourceRecord{
						{Value: aws.String(parts[2])},
					},
				},
			}},
		},
	})
	require.NoError(t, err)

	// Read-triggered evaluation: GetDomainAssociation (what terraform's
	// wait_for_verification polls) now reports AVAILABLE.
	getOut, err = c.GetDomainAssociation(ctx, &amplify.GetDomainAssociationInput{
		AppId: aws.String(appID), DomainName: aws.String(domain),
	})
	require.NoError(t, err)
	require.Equal(t, amplifytypes.DomainStatusAvailable, getOut.DomainAssociation.DomainStatus)
	require.True(t, aws.ToBool(getOut.DomainAssociation.SubDomains[0].Verified))

	// The verified custom domain serves the branch's deployment.
	amplifyDeployZip(t, ctx, c, appID, "main", amplifyZipBytes(t, map[string][]byte{
		"index.html": []byte("<html>custom domain content</html>"),
	}))
	resp, body := amplifyHostGet(t, "www."+domain, "/", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "<html>custom domain content</html>", string(body))

	_, err = c.DeleteDomainAssociation(ctx, &amplify.DeleteDomainAssociationInput{
		AppId: aws.String(appID), DomainName: aws.String(domain),
	})
	require.NoError(t, err)
	// The zone must be emptied of user records before deletion.
	_, err = r53.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
		ChangeBatch: &r53types.ChangeBatch{
			Changes: []r53types.Change{{
				Action: r53types.ChangeActionDelete,
				ResourceRecordSet: &r53types.ResourceRecordSet{
					Name: aws.String(parts[0]),
					Type: r53types.RRTypeCname,
					TTL:  aws.Int64(300),
					ResourceRecords: []r53types.ResourceRecord{
						{Value: aws.String(parts[2])},
					},
				},
			}},
		},
	})
	require.NoError(t, err)
	_, err = r53.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{Id: aws.String(zoneID)})
	require.NoError(t, err)
}
