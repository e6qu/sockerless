package gcpcommon

import core "github.com/sockerless/backend-core"

// gcpErrorPatterns are the Google Cloud status names and codes for each
// Docker error class.
var gcpErrorPatterns = core.CloudErrorPatterns{
	NotFound: []string{"not found", "NotFound", "404"},
	Conflict: []string{"already exists", "AlreadyExists", "409"},
	Invalid:  []string{"InvalidArgument", "invalid", "400"},
}

// MapGCPError converts a Google Cloud SDK error to the Docker API error type.
func MapGCPError(err error, resource, id string) error {
	return core.MapCloudError(err, resource, id, gcpErrorPatterns)
}
