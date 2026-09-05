package awscommon

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/sockerless/api"
)

// AccessPointToVolume shapes a sockerless-managed EFS access point as the
// Docker volume it backs.
func AccessPointToVolume(ap efstypes.AccessPointDescription) *api.Volume {
	root := ""
	if ap.RootDirectory != nil {
		root = aws.ToString(ap.RootDirectory.Path)
	}
	return &api.Volume{
		Name:       APVolumeName(ap),
		Driver:     "efs",
		Mountpoint: root,
		Scope:      "local",
		Options: map[string]string{
			"accessPointId": aws.ToString(ap.AccessPointId),
			"fileSystemId":  aws.ToString(ap.FileSystemId),
		},
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}
