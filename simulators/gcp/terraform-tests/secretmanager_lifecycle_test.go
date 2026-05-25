package gcp_tf_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerraformSecretManagerUpdateDelete(t *testing.T) {
	fixtureDir := filepath.Join("fixtures", "secretmanager-lifecycle")

	init := terraformCmdInDir(fixtureDir, "init")
	init.Stdout = nil
	init.Stderr = nil
	out, err := init.CombinedOutput()
	require.NoError(t, err, "terraform init failed:\n%s", out)

	apply := terraformCmdInDir(fixtureDir, "apply", "-auto-approve", "-var", "secret_label_env=dev")
	out, err = apply.CombinedOutput()
	require.NoError(t, err, "terraform apply failed:\n%s", out)

	updateSecret := terraformCmdInDir(fixtureDir, "apply", "-auto-approve", "-var", "secret_label_env=prod")
	out, err = updateSecret.CombinedOutput()
	require.NoError(t, err, "terraform apply with updated Secret Manager labels failed:\n%s", out)

	outputs := readOutputsInDir(t, fixtureDir)
	require.Equal(t, "prod", outputs.must(t, "secret_label_env"))
	require.Contains(t, outputs.must(t, "secret_id"), "projects/test-project/secrets/tf-update-secret")

	destroy := terraformCmdInDir(fixtureDir, "destroy", "-auto-approve", "-var", "secret_label_env=prod")
	out, err = destroy.CombinedOutput()
	require.NoError(t, err, "terraform destroy failed:\n%s", out)
}

func terraformCmdInDir(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("terraform", args...)
	cmd.Dir = filepath.Join(filepath.Dir(mustAbs("main.tf")), dir)
	cmd.Env = append(os.Environ(),
		"TF_VAR_endpoint="+baseURL,
	)
	return cmd
}

func readOutputsInDir(t *testing.T, dir string) tfOutputs {
	t.Helper()
	out, err := terraformCmdInDir(dir, "output", "-json").CombinedOutput()
	require.NoError(t, err, "terraform output failed:\n%s", out)
	var outputs tfOutputs
	require.NoError(t, json.Unmarshal(out, &outputs))
	return outputs
}
