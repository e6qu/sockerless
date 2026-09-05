package azurecommon

import core "github.com/sockerless/backend-core"

// SharedVolumeFormat is the shape of SOCKERLESS_ACA_SHARED_VOLUMES and
// SOCKERLESS_AZF_SHARED_VOLUMES: `name=containerPath=share=backing[=storageAccount]`.
// The backing (`azure-files-ephemeral`) is required; the storage account
// defaults to the backend's configured account.
var SharedVolumeFormat = core.SharedVolumeFormat{
	Usage: "name=containerPath=share=backing[=storageAccount]",
	Fields: []core.SharedVolumeField{
		core.SharedVolumeFieldName,
		core.SharedVolumeFieldContainerPath,
		core.SharedVolumeFieldAzureShare,
		core.SharedVolumeFieldBacking,
		core.SharedVolumeFieldAzureAccount,
	},
	Required: 4,
}

// HostBindPolicy names the Azure platform and its shared-volume variable
// in the rejection an unmapped host bind receives.
func HostBindPolicy(platform, envVar string) core.HostBindPolicy {
	return core.HostBindPolicy{
		Platform: platform,
		Backing:  "sockerless-managed Azure Files shares",
		EnvVar:   envVar,
	}
}
