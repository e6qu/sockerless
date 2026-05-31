package aws_cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSSMParameterCLI_PutGetDelete(t *testing.T) {
	name := "/cli/param/" + strings.ReplaceAll(time.Now().Format("150405.000000"), ".", "-")
	putOut := runCLI(t, awsCLI("ssm", "put-parameter",
		"--name", name,
		"--type", "String",
		"--value", "first",
		"--output", "json"))
	var putResult struct {
		Version int64 `json:"Version"`
	}
	parseJSON(t, putOut, &putResult)
	require.Equal(t, int64(1), putResult.Version)
	t.Cleanup(func() {
		_ = awsCLI("ssm", "delete-parameter", "--name", name).Run()
	})

	getOut := runCLI(t, awsCLI("ssm", "get-parameter",
		"--name", name,
		"--output", "json"))
	var getResult struct {
		Parameter struct {
			Name    string `json:"Name"`
			Type    string `json:"Type"`
			Value   string `json:"Value"`
			Version int64  `json:"Version"`
			ARN     string `json:"ARN"`
		} `json:"Parameter"`
	}
	parseJSON(t, getOut, &getResult)
	require.Equal(t, name, getResult.Parameter.Name)
	require.Equal(t, "String", getResult.Parameter.Type)
	require.Equal(t, "first", getResult.Parameter.Value)
	require.Equal(t, int64(1), getResult.Parameter.Version)
	require.Contains(t, getResult.Parameter.ARN, ":parameter"+name)

	overwriteOut := runCLI(t, awsCLI("ssm", "put-parameter",
		"--name", name,
		"--type", "String",
		"--value", "second",
		"--overwrite",
		"--output", "json"))
	var overwriteResult struct {
		Version int64 `json:"Version"`
	}
	parseJSON(t, overwriteOut, &overwriteResult)
	require.Equal(t, int64(2), overwriteResult.Version)

	runCLI(t, awsCLI("ssm", "delete-parameter",
		"--name", name,
		"--output", "json"))
}
