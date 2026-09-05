package azurecommon

import core "github.com/sockerless/backend-core"

// azureErrorPatterns are the Azure Resource Manager error codes and
// phrases for each Docker error class.
var azureErrorPatterns = core.CloudErrorPatterns{
	NotFound: []string{"not found", "NotFound", "ResourceNotFound"},
	Conflict: []string{"already exists", "Conflict"},
	Invalid:  []string{"InvalidParameter", "BadRequest"},
}

// MapAzureError converts an Azure SDK error to the Docker API error type.
func MapAzureError(err error, resource, id string) error {
	return core.MapCloudError(err, resource, id, azureErrorPatterns)
}
