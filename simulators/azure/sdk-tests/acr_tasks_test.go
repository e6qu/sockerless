package azure_sdk_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestACRTasks_ScheduleRunDockerBuild exercises the ACR Tasks quick-build
// slice exactly as backends/aca does for the reverse-agent bootstrap
// overlay: upload a build context to blob storage, then
// RegistriesClient.BeginScheduleRun with a DockerBuildRequest pointing at
// that blob. The simulator fetches the context, runs `docker build` on the
// host engine, tags the image into the local daemon, and reports the Run
// as Succeeded. We then assert the image is really present locally — the
// proof that StartContainerSync can run the overlay without a registry
// pull.
func TestACRTasks_ScheduleRunDockerBuild(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker CLI required for ACR Tasks build test (no fallback): %v", err)
	}

	const (
		rg       = "acr-tasks-rg"
		account  = "acrbuildacct"
		registry = "acrbuildreg"
		ctr      = "build-context"
	)
	imageName := fmt.Sprintf("%s.azurecr.io/sockerless-overlay/aca:test-%d", registry, time.Now().UnixNano())

	// 1. Upload a build context (Dockerfile + a file COPY'd in, mirroring
	// the bootstrap overlay shape) to the sim's blob storage.
	blobClient, err := azblob.NewClientWithNoCredential(storageSDKURL(t, account, "blob"),
		&azblob.ClientOptions{ClientOptions: storageSDKOptions()})
	require.NoError(t, err)
	_, _ = blobClient.CreateContainer(ctx, ctr, nil)

	context := makeACRBuildContext(t, map[string]string{
		"Dockerfile": "FROM public.ecr.aws/docker/library/alpine:3.20\n" +
			"COPY payload /opt/sockerless/payload\n" +
			"RUN chmod +x /opt/sockerless/payload\n" +
			"ENTRYPOINT [\"/opt/sockerless/payload\"]\n",
		"payload": "#!/bin/sh\necho overlay-ok\n",
	})
	blobName := fmt.Sprintf("build-context/%d.tar.gz", time.Now().UnixNano())
	_, err = blobClient.UploadBuffer(ctx, ctr, blobName, context, nil)
	require.NoError(t, err)
	sourceLocation := fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", account, ctr, blobName)

	// 2. BeginScheduleRun with the DockerBuildRequest the backend builds.
	regClient, err := armcontainerregistry.NewRegistriesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	poller, err := regClient.BeginScheduleRun(ctx, rg, registry, &armcontainerregistry.DockerBuildRequest{
		Type:           to.Ptr("DockerBuildRequest"),
		DockerFilePath: to.Ptr("Dockerfile"),
		ImageNames:     []*string{to.Ptr(imageName)},
		SourceLocation: to.Ptr(sourceLocation),
		IsPushEnabled:  to.Ptr(true),
		Platform: &armcontainerregistry.PlatformProperties{
			OS: to.Ptr(armcontainerregistry.OSLinux),
		},
	}, nil)
	require.NoError(t, err)

	result, err := poller.PollUntilDone(ctx, nil)
	require.NoError(t, err, "ACR Task run should succeed")
	require.NotNil(t, result.Properties)
	require.NotNil(t, result.Properties.Status)
	assert.Equal(t, armcontainerregistry.RunStatusSucceeded, *result.Properties.Status)
	require.NotNil(t, result.Properties.RunID)
	assert.NotEmpty(t, *result.Properties.RunID)

	// 3. The built image must really exist in the local daemon — the
	// whole point of the slice (StartContainerSync runs it by tag).
	t.Cleanup(func() { _ = exec.Command("docker", "image", "rm", "-f", imageName).Run() })
	require.NoError(t, exec.Command("docker", "image", "inspect", imageName).Run(),
		"built overlay image %s must be present in the local daemon", imageName)

	// 4. GetRun round-trips the run record.
	runsClient, err := armcontainerregistry.NewRunsClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)
	got, err := runsClient.Get(ctx, rg, registry, *result.Properties.RunID, nil)
	require.NoError(t, err)
	require.NotNil(t, got.Properties)
	assert.Equal(t, armcontainerregistry.RunStatusSucceeded, *got.Properties.Status)
}

// TestACRTasks_ScheduleRunMissingContextFails asserts the build fails
// loudly when the source context blob doesn't exist, rather than silently
// producing no image. ACR reports a build outcome through the Run's
// `status` (the run resource is returned successfully; it's the *run* that
// failed), which is exactly what backends/aca's ACRBuildService checks to
// surface the error.
func TestACRTasks_ScheduleRunMissingContextFails(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker CLI required for ACR Tasks build test (no fallback): %v", err)
	}
	regClient, err := armcontainerregistry.NewRegistriesClient(subscriptionID, &fakeCredential{}, clientOpts())
	require.NoError(t, err)

	poller, err := regClient.BeginScheduleRun(ctx, "acr-tasks-rg", "acrbuildreg", &armcontainerregistry.DockerBuildRequest{
		Type:           to.Ptr("DockerBuildRequest"),
		DockerFilePath: to.Ptr("Dockerfile"),
		ImageNames:     []*string{to.Ptr("acrbuildreg.azurecr.io/sockerless-overlay/aca:missing")},
		SourceLocation: to.Ptr("https://acrbuildacct.blob.core.windows.net/build-context/does-not-exist.tar.gz"),
		IsPushEnabled:  to.Ptr(true),
	}, nil)
	require.NoError(t, err)

	result, err := poller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, result.Properties)
	require.NotNil(t, result.Properties.Status)
	assert.Equal(t, armcontainerregistry.RunStatusFailed, *result.Properties.Status,
		"missing build context must report the run as Failed")
}

func makeACRBuildContext(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}
