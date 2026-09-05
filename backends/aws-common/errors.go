package awscommon

import core "github.com/sockerless/backend-core"

// awsErrorPatterns are the AWS SDK exception names and phrases for each
// Docker error class.
var awsErrorPatterns = core.CloudErrorPatterns{
	NotFound: []string{"not found", "does not exist", "ResourceNotFoundException"},
	Conflict: []string{"already exists", "ConflictException", "ResourceConflictException"},
	Invalid:  []string{"InvalidParameterValueException", "ValidationException"},
}

// MapAWSError converts an AWS SDK error to the Docker API error type.
func MapAWSError(err error, resource, id string) error {
	return core.MapCloudError(err, resource, id, awsErrorPatterns)
}
