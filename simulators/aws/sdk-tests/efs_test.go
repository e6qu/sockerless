package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func efsClient() *efs.Client {
	return efs.NewFromConfig(sdkConfig(), func(o *efs.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestEFS_CreateAndDescribeFileSystem(t *testing.T) {
	client := efsClient()

	createOut, err := client.CreateFileSystem(ctx, &efs.CreateFileSystemInput{
		CreationToken:   aws.String("test-fs-token"),
		PerformanceMode: efstypes.PerformanceModeGeneralPurpose,
		Tags: []efstypes.Tag{
			{Key: aws.String("Name"), Value: aws.String("test-fs")},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, *createOut.FileSystemId)
	assert.Equal(t, efstypes.LifeCycleStateAvailable, createOut.LifeCycleState)
	fsID := *createOut.FileSystemId

	// Describe
	descOut, err := client.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{
		FileSystemId: aws.String(fsID),
	})
	require.NoError(t, err)
	require.Len(t, descOut.FileSystems, 1)
	assert.Equal(t, fsID, *descOut.FileSystems[0].FileSystemId)
	assert.Equal(t, "test-fs", *descOut.FileSystems[0].Name)

	// Cleanup
	_, err = client.DeleteFileSystem(ctx, &efs.DeleteFileSystemInput{
		FileSystemId: aws.String(fsID),
	})
	require.NoError(t, err)

	// Verify deleted
	descOut2, err := client.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{
		FileSystemId: aws.String(fsID),
	})
	require.NoError(t, err)
	assert.Empty(t, descOut2.FileSystems)
}

func TestEFS_CreateAndDescribeMountTargets(t *testing.T) {
	client := efsClient()

	// Create file system first
	createOut, err := client.CreateFileSystem(ctx, &efs.CreateFileSystemInput{
		CreationToken: aws.String("mt-test-fs"),
	})
	require.NoError(t, err)
	fsID := *createOut.FileSystemId

	// Create mount target
	mtOut, err := client.CreateMountTarget(ctx, &efs.CreateMountTargetInput{
		FileSystemId:   aws.String(fsID),
		SubnetId:       aws.String("subnet-sim"),
		SecurityGroups: []string{"sg-12345"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, *mtOut.MountTargetId)
	assert.Equal(t, fsID, *mtOut.FileSystemId)
	assert.Equal(t, efstypes.LifeCycleStateAvailable, mtOut.LifeCycleState)
	mtID := *mtOut.MountTargetId

	// Describe mount targets by filesystem
	descOut, err := client.DescribeMountTargets(ctx, &efs.DescribeMountTargetsInput{
		FileSystemId: aws.String(fsID),
	})
	require.NoError(t, err)
	require.Len(t, descOut.MountTargets, 1)
	assert.Equal(t, mtID, *descOut.MountTargets[0].MountTargetId)

	// Verify file system now shows mount target count
	fsDesc, err := client.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{
		FileSystemId: aws.String(fsID),
	})
	require.NoError(t, err)
	require.Len(t, fsDesc.FileSystems, 1)
	assert.Equal(t, int32(1), fsDesc.FileSystems[0].NumberOfMountTargets)

	// Cleanup
	_, err = client.DeleteMountTarget(ctx, &efs.DeleteMountTargetInput{
		MountTargetId: aws.String(mtID),
	})
	require.NoError(t, err)

	_, err = client.DeleteFileSystem(ctx, &efs.DeleteFileSystemInput{
		FileSystemId: aws.String(fsID),
	})
	require.NoError(t, err)
}

func TestEFS_CreateAndDescribeAccessPoints(t *testing.T) {
	client := efsClient()

	// Create file system
	createOut, err := client.CreateFileSystem(ctx, &efs.CreateFileSystemInput{
		CreationToken: aws.String("ap-test-fs"),
	})
	require.NoError(t, err)
	fsID := *createOut.FileSystemId

	// Create access point
	apOut, err := client.CreateAccessPoint(ctx, &efs.CreateAccessPointInput{
		FileSystemId: aws.String(fsID),
		PosixUser: &efstypes.PosixUser{
			Uid: aws.Int64(1000),
			Gid: aws.Int64(1000),
		},
		RootDirectory: &efstypes.RootDirectory{
			Path: aws.String("/data"),
			CreationInfo: &efstypes.CreationInfo{
				OwnerUid:    aws.Int64(1000),
				OwnerGid:    aws.Int64(1000),
				Permissions: aws.String("755"),
			},
		},
		Tags: []efstypes.Tag{
			{Key: aws.String("Name"), Value: aws.String("test-ap")},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, *apOut.AccessPointId)
	assert.Equal(t, fsID, *apOut.FileSystemId)
	assert.Equal(t, efstypes.LifeCycleStateAvailable, apOut.LifeCycleState)
	apID := *apOut.AccessPointId

	// Describe access points by filesystem
	descOut, err := client.DescribeAccessPoints(ctx, &efs.DescribeAccessPointsInput{
		FileSystemId: aws.String(fsID),
	})
	require.NoError(t, err)
	require.Len(t, descOut.AccessPoints, 1)
	assert.Equal(t, apID, *descOut.AccessPoints[0].AccessPointId)
	assert.Equal(t, int64(1000), *descOut.AccessPoints[0].PosixUser.Uid)
	assert.Equal(t, "/data", *descOut.AccessPoints[0].RootDirectory.Path)

	// Cleanup: delete in reverse order
	_, err = client.DeleteAccessPoint(ctx, &efs.DeleteAccessPointInput{
		AccessPointId: aws.String(apID),
	})
	require.NoError(t, err)

	// Verify deleted
	descOut2, err := client.DescribeAccessPoints(ctx, &efs.DescribeAccessPointsInput{
		FileSystemId: aws.String(fsID),
	})
	require.NoError(t, err)
	assert.Empty(t, descOut2.AccessPoints)

	_, err = client.DeleteFileSystem(ctx, &efs.DeleteFileSystemInput{
		FileSystemId: aws.String(fsID),
	})
	require.NoError(t, err)
}

func TestEFS_FullLifecycle(t *testing.T) {
	client := efsClient()

	// Create file system
	fsOut, err := client.CreateFileSystem(ctx, &efs.CreateFileSystemInput{
		CreationToken: aws.String("lifecycle-test-fs"),
		Tags: []efstypes.Tag{
			{Key: aws.String("Name"), Value: aws.String("lifecycle-fs")},
		},
	})
	require.NoError(t, err)
	fsID := *fsOut.FileSystemId

	// Create mount target
	mtOut, err := client.CreateMountTarget(ctx, &efs.CreateMountTargetInput{
		FileSystemId: aws.String(fsID),
		SubnetId:     aws.String("subnet-lifecycle"),
	})
	require.NoError(t, err)
	mtID := *mtOut.MountTargetId

	// Create access point
	apOut, err := client.CreateAccessPoint(ctx, &efs.CreateAccessPointInput{
		FileSystemId: aws.String(fsID),
		PosixUser: &efstypes.PosixUser{
			Uid: aws.Int64(0),
			Gid: aws.Int64(0),
		},
		RootDirectory: &efstypes.RootDirectory{
			Path: aws.String("/"),
		},
	})
	require.NoError(t, err)
	apID := *apOut.AccessPointId

	// Delete in correct order: access point, mount target, file system
	_, err = client.DeleteAccessPoint(ctx, &efs.DeleteAccessPointInput{
		AccessPointId: aws.String(apID),
	})
	require.NoError(t, err)

	_, err = client.DeleteMountTarget(ctx, &efs.DeleteMountTargetInput{
		MountTargetId: aws.String(mtID),
	})
	require.NoError(t, err)

	_, err = client.DeleteFileSystem(ctx, &efs.DeleteFileSystemInput{
		FileSystemId: aws.String(fsID),
	})
	require.NoError(t, err)
}
