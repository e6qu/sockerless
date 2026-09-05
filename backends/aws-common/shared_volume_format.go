package awscommon

import core "github.com/sockerless/backend-core"

// Shared-volume declarations on the AWS backends name an Amazon Elastic
// File System access point the calling workload already mounts.

// ECSSharedVolumeFormat is SOCKERLESS_ECS_SHARED_VOLUMES:
// `name=containerPath=fsap-XXXX[=fs-YYYY]`. The file system defaults to
// the backend's configured agent file system.
var ECSSharedVolumeFormat = core.SharedVolumeFormat{
	Usage: "name=containerPath=fsap-XXXX[=fs-YYYY]",
	Fields: []core.SharedVolumeField{
		core.SharedVolumeFieldName,
		core.SharedVolumeFieldContainerPath,
		core.SharedVolumeFieldEFSAccessPoint,
		core.SharedVolumeFieldEFSFileSystem,
	},
	Required: 3,
}

// LambdaSharedVolumeFormat is SOCKERLESS_LAMBDA_SHARED_VOLUMES:
// `name=containerPath=fsap-XXXX[=fs-YYYY[=subpath]]`. AWS Lambda mounts one
// access point per function, so volumes sharing an access point are told
// apart by the trailing sub-path; the file system may be left empty
// (`fsap-XXXX==subpath`) to declare only the sub-path.
var LambdaSharedVolumeFormat = core.SharedVolumeFormat{
	Usage: "name=containerPath=fsap-XXXX[=fs-YYYY[=subpath]]",
	Fields: []core.SharedVolumeField{
		core.SharedVolumeFieldName,
		core.SharedVolumeFieldContainerPath,
		core.SharedVolumeFieldEFSAccessPoint,
		core.SharedVolumeFieldEFSFileSystem,
		core.SharedVolumeFieldEFSSubpath,
	},
	Required: 3,
}

// ECSHostBindPolicy names Amazon ECS in the rejection an unmapped host
// bind receives.
var ECSHostBindPolicy = core.HostBindPolicy{
	Platform: "Amazon ECS",
	Backing:  "sockerless-managed Amazon EFS access points",
	EnvVar:   "SOCKERLESS_ECS_SHARED_VOLUMES",
}
