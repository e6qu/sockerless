package gcp_cli_test

import (
	"strings"
	"testing"
)

func TestSpannerCLI_InstanceDatabaseList(t *testing.T) {
	runCLI(t, gcloudCLI("spanner", "instances", "create", "cli-spanner",
		"--config=regional-us-central1",
		"--description=CLI Spanner",
		"--nodes=1",
		"--format=json"))
	runCLI(t, gcloudCLI("spanner", "databases", "create", "clidb",
		"--instance=cli-spanner",
		"--format=json"))

	out := runCLI(t, gcloudCLI("spanner", "databases", "list", "--instance=cli-spanner", "--format=value(name)"))
	if !strings.Contains(out, "clidb") {
		t.Fatalf("spanner databases list missing clidb: %s", out)
	}
}

func TestDataflowCLI_ListRegionalJobs(t *testing.T) {
	httpDoJSON(t, "POST", baseURL+"/v1b3/projects/"+project+"/locations/"+location+"/jobs",
		`{"name":"cli-dataflow-job","type":"JOB_TYPE_BATCH"}`)

	out := runCLI(t, gcloudCLI("dataflow", "jobs", "list", "--region="+location, "--format=value(name)"))
	if !strings.Contains(out, "cli-dataflow-job") {
		t.Fatalf("dataflow jobs list missing seeded job: %s", out)
	}
}

func TestBigtableCLI_InstanceTableList(t *testing.T) {
	runCLI(t, gcloudCLI("bigtable", "instances", "create", "cli-bt",
		"--display-name=CLI Bigtable",
		"--cluster=cli-bt-c1",
		"--cluster-zone=us-central1-a",
		"--cluster-num-nodes=1",
		"--instance-type=PRODUCTION",
		"--format=json"))
	runCLI(t, gcloudCLI("bigtable", "tables", "create", "cli_table",
		"--instance=cli-bt",
		"--column-families=cf",
		"--format=json"))

	out := runCLI(t, gcloudCLI("bigtable", "tables", "list", "--instances=cli-bt", "--format=value(name)"))
	if !strings.Contains(out, "cli_table") {
		t.Fatalf("bigtable tables list missing cli_table: %s", out)
	}
}
