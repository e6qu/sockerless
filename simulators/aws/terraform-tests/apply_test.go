package aws_tf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerraformApplyDestroy(t *testing.T) {
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
