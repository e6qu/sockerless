package aws_cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEFS_CreateAndDescribeFileSystem(t *testing.T) {
	out := runCLI(t, awsCLI("efs", "create-file-system",
		"--creation-token", "cli-test-fs",
		"--tags", "Key=Name,Value=cli-test-fs",
		"--output", "json",
	))

	var createResult struct {
		FileSystemId   string `json:"FileSystemId"`
		LifeCycleState string `json:"LifeCycleState"`
		Name           string `json:"Name"`
	}
	parseJSON(t, out, &createResult)
	require.NotEmpty(t, createResult.FileSystemId)
	assert.Equal(t, "available", createResult.LifeCycleState)

	// Describe
	out = runCLI(t, awsCLI("efs", "describe-file-systems",
		"--file-system-id", createResult.FileSystemId,
		"--output", "json",
	))

	var descResult struct {
		FileSystems []struct {
			FileSystemId string `json:"FileSystemId"`
			Name         string `json:"Name"`
		} `json:"FileSystems"`
	}
	parseJSON(t, out, &descResult)
	require.Len(t, descResult.FileSystems, 1)
	assert.Equal(t, createResult.FileSystemId, descResult.FileSystems[0].FileSystemId)

	// Cleanup
	runCLI(t, awsCLI("efs", "delete-file-system",
		"--file-system-id", createResult.FileSystemId,
	))
}

func TestEFS_CreateMountTarget(t *testing.T) {
	out := runCLI(t, awsCLI("efs", "create-file-system",
		"--creation-token", "cli-mount-test",
		"--output", "json",
	))

	var fs struct {
		FileSystemId string `json:"FileSystemId"`
	}
	parseJSON(t, out, &fs)

	out = runCLI(t, awsCLI("efs", "create-mount-target",
		"--file-system-id", fs.FileSystemId,
		"--subnet-id", "subnet-12345678",
		"--output", "json",
	))

	var mt struct {
		MountTargetId  string `json:"MountTargetId"`
		FileSystemId   string `json:"FileSystemId"`
		LifeCycleState string `json:"LifeCycleState"`
	}
	parseJSON(t, out, &mt)
	require.NotEmpty(t, mt.MountTargetId)
	assert.Equal(t, fs.FileSystemId, mt.FileSystemId)
	assert.Equal(t, "available", mt.LifeCycleState)

	// Describe mount targets
	out = runCLI(t, awsCLI("efs", "describe-mount-targets",
		"--file-system-id", fs.FileSystemId,
		"--output", "json",
	))

	var descResult struct {
		MountTargets []struct {
			MountTargetId string `json:"MountTargetId"`
		} `json:"MountTargets"`
	}
	parseJSON(t, out, &descResult)
	require.Len(t, descResult.MountTargets, 1)

	// Cleanup
	runCLI(t, awsCLI("efs", "delete-file-system",
		"--file-system-id", fs.FileSystemId,
	))
}

func TestEFS_CreateAccessPoint(t *testing.T) {
	out := runCLI(t, awsCLI("efs", "create-file-system",
		"--creation-token", "cli-ap-test",
		"--output", "json",
	))

	var fs struct {
		FileSystemId string `json:"FileSystemId"`
	}
	parseJSON(t, out, &fs)

	out = runCLI(t, awsCLI("efs", "create-access-point",
		"--file-system-id", fs.FileSystemId,
		"--posix-user", "Uid=1000,Gid=1000",
		"--root-directory", `Path=/data,CreationInfo={OwnerUid=1000,OwnerGid=1000,Permissions=755}`,
		"--tags", "Key=Name,Value=cli-ap",
		"--output", "json",
	))

	var ap struct {
		AccessPointId  string `json:"AccessPointId"`
		FileSystemId   string `json:"FileSystemId"`
		LifeCycleState string `json:"LifeCycleState"`
	}
	parseJSON(t, out, &ap)
	require.NotEmpty(t, ap.AccessPointId)
	assert.Equal(t, fs.FileSystemId, ap.FileSystemId)

	// Describe access points
	out = runCLI(t, awsCLI("efs", "describe-access-points",
		"--file-system-id", fs.FileSystemId,
		"--output", "json",
	))

	var descResult struct {
		AccessPoints []struct {
			AccessPointId string `json:"AccessPointId"`
		} `json:"AccessPoints"`
	}
	parseJSON(t, out, &descResult)
	require.Len(t, descResult.AccessPoints, 1)

	// Cleanup
	runCLI(t, awsCLI("efs", "delete-access-point",
		"--access-point-id", ap.AccessPointId,
	))
	runCLI(t, awsCLI("efs", "delete-file-system",
		"--file-system-id", fs.FileSystemId,
	))
}

func TestEFS_DeleteFileSystem(t *testing.T) {
	out := runCLI(t, awsCLI("efs", "create-file-system",
		"--creation-token", "cli-delete-test",
		"--output", "json",
	))

	var fs struct {
		FileSystemId string `json:"FileSystemId"`
	}
	parseJSON(t, out, &fs)

	runCLI(t, awsCLI("efs", "delete-file-system",
		"--file-system-id", fs.FileSystemId,
	))

	// Verify it's gone
	out = runCLI(t, awsCLI("efs", "describe-file-systems", "--output", "json"))
	var result struct {
		FileSystems []struct {
			FileSystemId string `json:"FileSystemId"`
		} `json:"FileSystems"`
	}
	parseJSON(t, out, &result)
	for _, f := range result.FileSystems {
		assert.NotEqual(t, fs.FileSystemId, f.FileSystemId)
	}
}
