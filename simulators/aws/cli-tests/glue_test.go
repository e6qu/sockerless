package aws_cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlue_DatabaseCRUD_CLI(t *testing.T) {
	runCLI(t, awsCLI("glue", "create-database",
		"--database-input", `{"Name":"glue-cli-db","Description":"cli test"}`,
	))
	t.Cleanup(func() {
		runCLI(t, awsCLI("glue", "delete-database", "--name", "glue-cli-db"))
	})

	out := runCLI(t, awsCLI("glue", "get-database", "--name", "glue-cli-db"))
	var get struct {
		Database struct {
			Name string `json:"Name"`
		} `json:"Database"`
	}
	parseJSON(t, out, &get)
	assert.Equal(t, "glue-cli-db", get.Database.Name)

	out = runCLI(t, awsCLI("glue", "get-databases"))
	var list struct {
		DatabaseList []struct {
			Name string `json:"Name"`
		} `json:"DatabaseList"`
	}
	parseJSON(t, out, &list)
	found := false
	for _, db := range list.DatabaseList {
		if db.Name == "glue-cli-db" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestGlue_TableCRUD_CLI(t *testing.T) {
	runCLI(t, awsCLI("glue", "create-database",
		"--database-input", `{"Name":"glue-cli-tbl-db"}`,
	))
	t.Cleanup(func() {
		runCLI(t, awsCLI("glue", "delete-table",
			"--database-name", "glue-cli-tbl-db",
			"--name", "glue-cli-table"))
		runCLI(t, awsCLI("glue", "delete-database", "--name", "glue-cli-tbl-db"))
	})

	runCLI(t, awsCLI("glue", "create-table",
		"--database-name", "glue-cli-tbl-db",
		"--table-input", `{"Name":"glue-cli-table","StorageDescriptor":{"Location":"s3://bucket/","InputFormat":"org.apache.hadoop.mapred.TextInputFormat","OutputFormat":"org.apache.hadoop.hive.ql.io.HiveIgnoreKeyTextOutputFormat"}}`,
	))

	out := runCLI(t, awsCLI("glue", "get-table",
		"--database-name", "glue-cli-tbl-db",
		"--name", "glue-cli-table"))
	var get struct {
		Table struct {
			Name         string `json:"Name"`
			DatabaseName string `json:"DatabaseName"`
		} `json:"Table"`
	}
	parseJSON(t, out, &get)
	assert.Equal(t, "glue-cli-table", get.Table.Name)
	assert.Equal(t, "glue-cli-tbl-db", get.Table.DatabaseName)

	out = runCLI(t, awsCLI("glue", "get-tables", "--database-name", "glue-cli-tbl-db"))
	var list struct {
		TableList []struct {
			Name string `json:"Name"`
		} `json:"TableList"`
	}
	parseJSON(t, out, &list)
	require.Len(t, list.TableList, 1)
	assert.Equal(t, "glue-cli-table", list.TableList[0].Name)
}

func TestGlue_JobCRUD_CLI(t *testing.T) {
	bucket := "glue-cli-scripts"
	scriptPath := filepath.Join(tmpDir, "glue-cli-script.py")
	require.NoError(t, os.WriteFile(scriptPath, []byte("import sys\nprint('glue-cli-ready')\nsys.exit(0)\n"), 0644))
	runCLI(t, awsCLI("s3api", "create-bucket", "--bucket", bucket))
	runCLI(t, awsCLI("s3api", "put-object", "--bucket", bucket, "--key", "script.py", "--body", scriptPath))
	t.Cleanup(func() {
		runCLI(t, awsCLI("s3api", "delete-object", "--bucket", bucket, "--key", "script.py"))
		runCLI(t, awsCLI("s3api", "delete-bucket", "--bucket", bucket))
	})

	runCLI(t, awsCLI("glue", "create-job",
		"--name", "glue-cli-job",
		"--role", "arn:aws:iam::123456789012:role/glue-role",
		"--command", `{"Name":"pythonshell","ScriptLocation":"s3://`+bucket+`/script.py"}`,
		"--glue-version", "4.0",
		"--worker-type", "G.1X",
		"--number-of-workers", "2",
	))
	t.Cleanup(func() {
		runCLI(t, awsCLI("glue", "delete-job", "--job-name", "glue-cli-job"))
	})

	out := runCLI(t, awsCLI("glue", "get-job", "--job-name", "glue-cli-job"))
	var get struct {
		Job struct {
			Name string `json:"Name"`
		} `json:"Job"`
	}
	parseJSON(t, out, &get)
	assert.Equal(t, "glue-cli-job", get.Job.Name)

	out = runCLI(t, awsCLI("glue", "get-jobs"))
	var list struct {
		Jobs []struct {
			Name string `json:"Name"`
		} `json:"Jobs"`
	}
	parseJSON(t, out, &list)
	found := false
	for _, j := range list.Jobs {
		if j.Name == "glue-cli-job" {
			found = true
		}
	}
	assert.True(t, found)

	out = runCLI(t, awsCLI("glue", "start-job-run", "--job-name", "glue-cli-job"))
	var run struct {
		JobRunID string `json:"JobRunId"`
	}
	parseJSON(t, out, &run)
	require.NotEmpty(t, run.JobRunID)

	var getRun struct {
		JobRun struct {
			ID          string `json:"Id"`
			JobRunState string `json:"JobRunState"`
		} `json:"JobRun"`
	}
	require.Eventually(t, func() bool {
		out = runCLI(t, awsCLI("glue", "get-job-run",
			"--job-name", "glue-cli-job",
			"--run-id", run.JobRunID))
		parseJSON(t, out, &getRun)
		return getRun.JobRun.JobRunState == "SUCCEEDED"
	}, 10*time.Second, 100*time.Millisecond)
	assert.Equal(t, run.JobRunID, getRun.JobRun.ID)
	assert.Equal(t, "SUCCEEDED", getRun.JobRun.JobRunState)

	// get-job-runs for a non-existent job must error (EntityNotFoundException),
	// not return an empty list.
	errOut := runCLIExpectError(t, awsCLI("glue", "get-job-runs", "--job-name", "glue-cli-no-such-job"))
	assert.Contains(t, errOut, "EntityNotFoundException")
}
