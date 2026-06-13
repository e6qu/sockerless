package gcp_sdk_test

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCloudBuild_FaithfulBuildPush exercises the Cloud Build slice exactly as
// the cloudrun / cloudrun-functions backends do for the reverse-agent overlay:
// a `docker build -t <ref>` step followed by a `docker push <ref>` step. Faithful
// to real Cloud Build, the sim pushes the built image to its registry and drops
// the local copy, so the workload pulls it from the registry over /v2/ — not a
// local-daemon shortcut. We point the ref at a throwaway registry the build host
// can reach (a stand-in for the AR /v2/ a workload would pull from) and assert
// the image landed there and is gone from the local daemon.
func TestCloudBuild_FaithfulBuildPush(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker CLI required for Cloud Build push test (no fallback): %v", err)
	}

	const regPort = "5098"
	startThrowawayRegistry(t, regPort)
	// Pre-pull the build's base image so the sim's `docker build` uses the
	// local cache instead of racing a throttle-prone public-mirror pull.
	pullImageWithRetry(t, "public.ecr.aws/docker/library/alpine:3.20")

	imageName := fmt.Sprintf("127.0.0.1:%s/sockerless-overlay/cloudrun:test-%d", regPort, time.Now().UnixNano())

	project := "cb-push-project"
	bucket := "cb-push-bucket"
	createBucket(t, project, bucket)
	objectName := fmt.Sprintf("cb-push-%d.tar.gz", time.Now().UnixNano())
	tarball := makeTarGz(t, map[string]string{
		"Dockerfile": "FROM public.ecr.aws/docker/library/alpine:3.20\nRUN echo cloudbuild-ok > /opt/payload\n",
	})
	uploadGCSObject(t, bucket, objectName, tarball)

	buildURL := fmt.Sprintf("%s/v1/projects/%s/builds", baseURL, project)
	body := fmt.Sprintf(`{
		"source":{"storageSource":{"bucket":%q,"object":%q}},
		"steps":[
			{"name":"gcr.io/cloud-builders/docker","args":["build","-t",%q,"."]},
			{"name":"gcr.io/cloud-builders/docker","args":["push",%q]}
		],
		"images":[%q]
	}`, bucket, objectName, imageName, imageName, imageName)
	resp := httpPOST(t, buildURL, body)
	require.Contains(t, resp, `"status":"SUCCESS"`, "build+push should succeed: %s", resp)

	// Faithful build→push: the image must live in the registry (pullable via
	// /v2/), NOT on the build host's local daemon.
	t.Cleanup(func() { _ = exec.Command("docker", "image", "rm", "-f", imageName).Run() })
	tagOnly := imageName[strings.LastIndex(imageName, ":")+1:]
	manifestURL := fmt.Sprintf("http://127.0.0.1:%s/v2/sockerless-overlay/cloudrun/manifests/%s", regPort, tagOnly)
	mreq, _ := http.NewRequest(http.MethodGet, manifestURL, nil)
	mreq.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
	mresp, err := http.DefaultClient.Do(mreq)
	require.NoError(t, err)
	defer mresp.Body.Close()
	require.Equal(t, http.StatusOK, mresp.StatusCode, "built image must be present in the registry (/v2/ manifest)")
	assert.Error(t, exec.Command("docker", "image", "inspect", imageName).Run(),
		"built overlay image %s must NOT remain on the local daemon after push", imageName)
}

// pullImageWithRetry pulls an image with bounded exponential backoff so a
// transient public-mirror throttle doesn't flake docker-dependent setup.
func pullImageWithRetry(t *testing.T, image string) {
	t.Helper()
	var lastErr error
	delay := time.Second
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)
			if delay < 8*time.Second {
				delay *= 2
			}
		}
		out, err := exec.Command("docker", "pull", image).CombinedOutput()
		if err == nil {
			return
		}
		lastErr = fmt.Errorf("%v: %s", err, out)
	}
	t.Fatalf("pull %s after retries: %v", image, lastErr)
}

// startThrowawayRegistry runs a real registry:2 on 127.0.0.1:<port> for the
// duration of the test — a reachable, auto-insecure (on Docker) stand-in for
// the AR `/v2/` endpoint the sim's Cloud Build pushes to. Docker auto-trusts a
// 127.0.0.1 registry; a local Podman host needs an insecure-registries drop-in.
func startThrowawayRegistry(t *testing.T, port string) {
	t.Helper()
	const regImage = "public.ecr.aws/docker/library/registry:2"
	pullImageWithRetry(t, regImage)
	name := "gcp-cb-sdktest-reg-" + port
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		_ = exec.Command("docker", "rm", "-f", name).Run()
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		out, err := exec.Command("docker", "run", "-d", "--rm", "--name", name,
			"-p", port+":5000", regImage).CombinedOutput()
		if err == nil {
			lastErr = nil
			break
		}
		lastErr = fmt.Errorf("%v: %s", err, out)
	}
	require.NoError(t, lastErr, "start throwaway registry")
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, derr := http.Get(fmt.Sprintf("http://127.0.0.1:%s/v2/", port))
		if derr == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("throwaway registry on :%s did not become ready", port)
}
