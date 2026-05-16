package azure_tf_test

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerraformApplyDestroy(t *testing.T) {
	// The azurestack terraform provider (and the azurerm sibling)
	// validates the sim's self-signed HTTPS cert against the OS trust
	// store. On Linux it honours SSL_CERT_FILE (set by terraformCmd).
	// On darwin, Go's cgo-backed crypto/x509.SystemCertPool() reads
	// from the Security framework keychain and ignores SSL_CERT_FILE —
	// so terraform's outbound calls to the sim fail with `x509:
	// "localhost" certificate is not trusted`. We don't ship a darwin
	// workaround (would require sudo + persistent keychain trust).
	// Fail loud so the dev sees the limitation explicitly instead of
	// silently skipping. Run these tests in a Linux container or in CI.
	if runtime.GOOS == "darwin" {
		t.Fatal("darwin: SSL_CERT_FILE ignored by Go cgo SystemCertPool — terraform azurerm cannot validate the sim's self-signed cert. Run via a Linux Docker container or in CI.")
	}

	init := terraformCmd("init")
	init.Stdout = nil
	init.Stderr = nil
	out, err := init.CombinedOutput()
	require.NoError(t, err, "terraform init failed:\n%s", out)

	apply := terraformCmd("apply", "-auto-approve")
	out, err = apply.CombinedOutput()
	require.NoError(t, err, "terraform apply failed:\n%s", out)

	destroy := terraformCmd("destroy", "-auto-approve")
	out, err = destroy.CombinedOutput()
	require.NoError(t, err, "terraform destroy failed:\n%s", out)
}
