package azurecommon

import (
	"errors"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

// IsNotFound returns true when the error represents "resource gone"
// for the Azure SDK shapes sockerless uses. Typed check (errors.As
// to *azcore.ResponseError + StatusCode == 404) replaces the previous
// substring-based pattern that was brittle across SDK version bumps
// (BUG-1063).
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		if respErr.StatusCode == http.StatusNotFound {
			return true
		}
		if respErr.ErrorCode == "ResourceNotFound" || respErr.ErrorCode == "NotFound" {
			return true
		}
	}
	return false
}
