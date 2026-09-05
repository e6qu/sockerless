package gcpcommon

import core "github.com/sockerless/backend-core"

// SharedVolumeFormat is SOCKERLESS_GCP_SHARED_VOLUMES, read by both the
// Google Cloud Run and the Cloud Run Functions backend:
// `name=containerPath=bucket=backing`. The backing (`gcs-sync`,
// `gcs-fuse`, or `emptyDir`) is required: each has different cost, scale,
// and consistency, and a silent default would hide a misconfiguration.
var SharedVolumeFormat = core.SharedVolumeFormat{
	Usage: "name=containerPath=bucket=backing",
	Fields: []core.SharedVolumeField{
		core.SharedVolumeFieldName,
		core.SharedVolumeFieldContainerPath,
		core.SharedVolumeFieldGCSBucket,
		core.SharedVolumeFieldBacking,
	},
	Required: 4,
}

// HostBindPolicy names the Google Cloud platform in the rejection an
// unmapped host bind receives.
func HostBindPolicy(platform string) core.HostBindPolicy {
	return core.HostBindPolicy{
		Platform: platform,
		Backing:  "sockerless-managed Cloud Storage buckets",
		EnvVar:   "SOCKERLESS_GCP_SHARED_VOLUMES",
	}
}
