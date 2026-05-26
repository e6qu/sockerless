package azure_cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
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
