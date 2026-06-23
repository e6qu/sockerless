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

func TestGlue_DatabaseUpdate_CLI(t *testing.T) {
	runCLI(t, awsCLI("glue", "create-database",
		"--database-input", `{"Name":"glue-cli-updb","Description":"before"}`,
	))
	t.Cleanup(func() {
		runCLI(t, awsCLI("glue", "delete-database", "--name", "glue-cli-updb"))
	})

	runCLI(t, awsCLI("glue", "update-database",
		"--name", "glue-cli-updb",
		"--database-input", `{"Name":"glue-cli-updb","Description":"after","Parameters":{"owner":"data-eng"}}`,
	))

	out := runCLI(t, awsCLI("glue", "get-database", "--name", "glue-cli-updb"))
	var get struct {
		Database struct {
			Description string            `json:"Description"`
			Parameters  map[string]string `json:"Parameters"`
		} `json:"Database"`
	}
	parseJSON(t, out, &get)
	assert.Equal(t, "after", get.Database.Description)
	assert.Equal(t, "data-eng", get.Database.Parameters["owner"])
}

func TestGlue_TableUpdateAndBatchDelete_CLI(t *testing.T) {
	db := "glue-cli-tblupd-db"
	runCLI(t, awsCLI("glue", "create-database", "--database-input", `{"Name":"`+db+`"}`))
	t.Cleanup(func() {
		runCLI(t, awsCLI("glue", "batch-delete-table", "--database-name", db, "--tables-to-delete", "t1", "t2"))
		runCLI(t, awsCLI("glue", "delete-database", "--name", db))
	})

	for _, name := range []string{"t1", "t2"} {
		runCLI(t, awsCLI("glue", "create-table",
			"--database-name", db,
			"--table-input", `{"Name":"`+name+`","StorageDescriptor":{"Location":"s3://bucket/`+name+`/"}}`,
		))
	}

	runCLI(t, awsCLI("glue", "update-table",
		"--database-name", db,
		"--table-input", `{"Name":"t1","TableType":"EXTERNAL_TABLE","StorageDescriptor":{"Location":"s3://bucket/t1-new/"}}`,
	))
	out := runCLI(t, awsCLI("glue", "get-table", "--database-name", db, "--name", "t1"))
	var get struct {
		Table struct {
			TableType         string `json:"TableType"`
			StorageDescriptor struct {
				Location string `json:"Location"`
			} `json:"StorageDescriptor"`
		} `json:"Table"`
	}
	parseJSON(t, out, &get)
	assert.Equal(t, "EXTERNAL_TABLE", get.Table.TableType)
	assert.Equal(t, "s3://bucket/t1-new/", get.Table.StorageDescriptor.Location)

	out = runCLI(t, awsCLI("glue", "batch-delete-table",
		"--database-name", db,
		"--tables-to-delete", "t1", "t2", "missing"))
	var bd struct {
		Errors []struct {
			TableName string `json:"TableName"`
		} `json:"Errors"`
	}
	parseJSON(t, out, &bd)
	require.Len(t, bd.Errors, 1)
	assert.Equal(t, "missing", bd.Errors[0].TableName)
}

func TestGlue_PartitionLifecycle_CLI(t *testing.T) {
	db := "glue-cli-part-db"
	tbl := "glue-cli-part-tbl"
	runCLI(t, awsCLI("glue", "create-database", "--database-input", `{"Name":"`+db+`"}`))
	t.Cleanup(func() {
		runCLI(t, awsCLI("glue", "delete-table", "--database-name", db, "--name", tbl))
		runCLI(t, awsCLI("glue", "delete-database", "--name", db))
	})
	runCLI(t, awsCLI("glue", "create-table",
		"--database-name", db,
		"--table-input", `{"Name":"`+tbl+`","StorageDescriptor":{"Location":"s3://bucket/part/"},"PartitionKeys":[{"Name":"dt","Type":"string"}]}`,
	))

	runCLI(t, awsCLI("glue", "create-partition",
		"--database-name", db,
		"--table-name", tbl,
		"--partition-input", `{"Values":["2024-01-01"],"StorageDescriptor":{"Location":"s3://bucket/part/dt=2024-01-01/"}}`,
	))

	out := runCLI(t, awsCLI("glue", "batch-create-partition",
		"--database-name", db,
		"--table-name", tbl,
		"--partition-input-list", `[{"Values":["2024-01-02"],"StorageDescriptor":{"Location":"s3://bucket/part/dt=2024-01-02/"}},{"Values":["2024-01-01"]}]`,
	))
	var bcp struct {
		Errors []struct {
			PartitionValues []string `json:"PartitionValues"`
		} `json:"Errors"`
	}
	parseJSON(t, out, &bcp)
	require.Len(t, bcp.Errors, 1)
	assert.Equal(t, []string{"2024-01-01"}, bcp.Errors[0].PartitionValues)

	out = runCLI(t, awsCLI("glue", "get-partition",
		"--database-name", db, "--table-name", tbl, "--partition-values", "2024-01-01"))
	var gp struct {
		Partition struct {
			Values       []string `json:"Values"`
			DatabaseName string   `json:"DatabaseName"`
			TableName    string   `json:"TableName"`
		} `json:"Partition"`
	}
	parseJSON(t, out, &gp)
	assert.Equal(t, []string{"2024-01-01"}, gp.Partition.Values)
	assert.Equal(t, db, gp.Partition.DatabaseName)
	assert.Equal(t, tbl, gp.Partition.TableName)

	out = runCLI(t, awsCLI("glue", "get-partitions", "--database-name", db, "--table-name", tbl))
	var gps struct {
		Partitions []struct {
			Values []string `json:"Values"`
		} `json:"Partitions"`
	}
	parseJSON(t, out, &gps)
	assert.Len(t, gps.Partitions, 2)

	out = runCLI(t, awsCLI("glue", "batch-get-partition",
		"--database-name", db, "--table-name", tbl,
		"--partitions-to-get", `[{"Values":["2024-01-02"]},{"Values":["2099-12-31"]}]`))
	var bgp struct {
		Partitions []struct {
			Values []string `json:"Values"`
		} `json:"Partitions"`
		UnprocessedKeys []struct {
			Values []string `json:"Values"`
		} `json:"UnprocessedKeys"`
	}
	parseJSON(t, out, &bgp)
	require.Len(t, bgp.Partitions, 1)
	assert.Equal(t, []string{"2024-01-02"}, bgp.Partitions[0].Values)
	require.Len(t, bgp.UnprocessedKeys, 1)
	assert.Equal(t, []string{"2099-12-31"}, bgp.UnprocessedKeys[0].Values)

	runCLI(t, awsCLI("glue", "update-partition",
		"--database-name", db, "--table-name", tbl,
		"--partition-value-list", "2024-01-01",
		"--partition-input", `{"Values":["2024-01-01"],"Parameters":{"rows":"100"}}`))
	out = runCLI(t, awsCLI("glue", "get-partition",
		"--database-name", db, "--table-name", tbl, "--partition-values", "2024-01-01"))
	var gp2 struct {
		Partition struct {
			Parameters map[string]string `json:"Parameters"`
		} `json:"Partition"`
	}
	parseJSON(t, out, &gp2)
	assert.Equal(t, "100", gp2.Partition.Parameters["rows"])

	runCLI(t, awsCLI("glue", "delete-partition",
		"--database-name", db, "--table-name", tbl, "--partition-values", "2024-01-01"))

	out = runCLI(t, awsCLI("glue", "batch-delete-partition",
		"--database-name", db, "--table-name", tbl,
		"--partitions-to-delete", `[{"Values":["2024-01-02"]},{"Values":["2099-12-31"]}]`))
	var bdp struct {
		Errors []struct {
			PartitionValues []string `json:"PartitionValues"`
		} `json:"Errors"`
	}
	parseJSON(t, out, &bdp)
	require.Len(t, bdp.Errors, 1)
	assert.Equal(t, []string{"2099-12-31"}, bdp.Errors[0].PartitionValues)

	out = runCLI(t, awsCLI("glue", "get-partitions", "--database-name", db, "--table-name", tbl))
	parseJSON(t, out, &gps)
	assert.Empty(t, gps.Partitions)
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
