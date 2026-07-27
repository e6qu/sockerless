package azure_cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const storageAccountKey = "a2tra2tra2tra2tra2tra2tra2tra2tra2tra2tra2s="

func azStorageContainer(args ...string) *exec.Cmd {
	baseArgs := append([]string{"storage", "container"}, args...)
	baseArgs = append(baseArgs,
		"--auth-mode", "key",
		"--account-name", "listpropsacct",
		"--account-key", storageAccountKey,
		"--blob-endpoint", baseURL+"/listpropsacct/",
		"--only-show-errors",
		"--output", "json",
	)
	cmd := exec.Command("az", baseArgs...)
	cmd.Env = append(os.Environ(),
		"AZURE_CONFIG_DIR="+filepath.Join(tmpDir, "azure-config"),
		"AZURE_CORE_NO_COLOR=1",
	)
	return cmd
}

func TestBlobListContainersPropertiesCLI(t *testing.T) {
	container := "cli-list-props"

	runCLI(t, azStorageContainer("create", "--name", container))
	t.Cleanup(func() {
		_ = azStorageContainer("delete", "--name", container).Run()
	})

	out := runCLI(t, azStorageContainer("show", "--name", container))
	var shown struct {
		Properties struct {
			ETag         string `json:"etag"`
			LastModified string `json:"lastModified"`
		} `json:"properties"`
	}
	parseJSON(t, out, &shown)
	require.NotEmpty(t, shown.Properties.ETag)
	require.NotEmpty(t, shown.Properties.LastModified)

	out = runCLI(t, azStorageContainer("list"))
	var containers []struct {
		Name       string `json:"name"`
		Properties struct {
			ETag         string `json:"etag"`
			LastModified string `json:"lastModified"`
		} `json:"properties"`
	}
	parseJSON(t, out, &containers)
	for _, listed := range containers {
		if listed.Name != container {
			continue
		}
		assert.Equal(t, shown.Properties.ETag, listed.Properties.ETag)
		assert.Equal(t, shown.Properties.LastModified, listed.Properties.LastModified)
		return
	}
	t.Fatalf("container %q not found in az storage container list output: %s", container, out)
}

// azStorageBlob builds an `az storage blob` invocation against the simulator's
// path-style blob endpoint, with the same account + key coordinates the
// container helper uses.
func azStorageBlob(args ...string) *exec.Cmd {
	baseArgs := append([]string{"storage", "blob"}, args...)
	baseArgs = append(baseArgs,
		"--auth-mode", "key",
		"--account-name", "listpropsacct",
		"--account-key", storageAccountKey,
		"--blob-endpoint", baseURL+"/listpropsacct/",
		"--only-show-errors",
		"--output", "json",
	)
	cmd := exec.Command("az", baseArgs...)
	cmd.Env = append(os.Environ(),
		"AZURE_CONFIG_DIR="+filepath.Join(tmpDir, "azure-config"),
		"AZURE_CORE_NO_COLOR=1",
	)
	return cmd
}

// runStorageCLIExpectFailure runs an `az storage` command that must fail and
// returns its combined output. A command that succeeds is the failure: it means
// the simulator answered an operation it does not implement.
func runStorageCLIExpectFailure(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	cmd.Env = append(cmd.Env, "PYTHONWARNINGS=ignore")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("CLI command unexpectedly succeeded: %s\nOutput: %s", strings.Join(cmd.Args, " "), out)
	}
	return string(out)
}

// TestBlobUnservedOperationDeclaresGapCLI drives Set Blob Tier — which the
// simulator does not implement — through the az CLI. The blob data plane used
// to select its handler from `comp` and fall through for an unrecognized value,
// so this call ran Put Blob: az reported success and the blob's contents were
// replaced by the empty request body.
func TestBlobUnservedOperationDeclaresGapCLI(t *testing.T) {
	container := "cli-gap-container"
	blobName := "kept.txt"
	payload := "kept through a refused set-tier"

	runCLI(t, azStorageContainer("create", "--name", container))
	t.Cleanup(func() {
		_ = azStorageContainer("delete", "--name", container).Run()
	})

	src := filepath.Join(t.TempDir(), blobName)
	require.NoError(t, os.WriteFile(src, []byte(payload), 0o600))
	runCLI(t, azStorageBlob("upload", "--container-name", container, "--name", blobName, "--file", src))

	out := runStorageCLIExpectFailure(t, azStorageBlob("set-tier",
		"--container-name", container, "--name", blobName, "--tier", "Cool"))
	assert.Contains(t, out, "NotImplemented",
		"az must surface the simulator's declared gap, not a success")

	dst := filepath.Join(t.TempDir(), blobName)
	runCLI(t, azStorageBlob("download", "--container-name", container, "--name", blobName, "--file", dst))
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, payload, string(got), "the blob must survive the refused set-tier untouched")
}
