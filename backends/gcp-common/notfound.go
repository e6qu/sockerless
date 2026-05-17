package gcpcommon

import (
	"errors"

	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsNotFound returns true when the error represents "resource gone"
// across the GCP SDK shapes sockerless uses: gRPC status codes
// (cloud.google.com/go/run/apiv2 + cloud.google.com/go/functions/apiv2)
// and the legacy googleapi.Error (cloud.google.com/go/storage etc).
// Substring-based fallback removed (BUG-1063 — brittle across SDK
// version bumps).
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if st, ok := status.FromError(err); ok {
		return st.Code() == codes.NotFound
	}
	var gErr *googleapi.Error
	if errors.As(err, &gErr) && gErr.Code == 404 {
		return true
	}
	return false
}
