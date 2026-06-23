package aws_cli_test

import (
	"strings"
	"testing"
)

// TestOrganizationsCLI_DescribeAndList exercises the Organizations slice over the
// aws CLI (describe-organization / list-accounts).
func TestOrganizationsCLI_DescribeAndList(t *testing.T) {
	out := runCLI(t, awsCLI("organizations", "describe-organization", "--output", "json"))
	var desc struct {
		Organization struct {
			Id              string `json:"Id"`
			FeatureSet      string `json:"FeatureSet"`
			MasterAccountId string `json:"MasterAccountId"`
		} `json:"Organization"`
	}
	parseJSON(t, out, &desc)
	if !strings.HasPrefix(desc.Organization.Id, "o-") {
		t.Fatalf("describe-organization Id = %q, want o-…", desc.Organization.Id)
	}

	la := runCLI(t, awsCLI("organizations", "list-accounts", "--output", "json"))
	if !strings.Contains(la, desc.Organization.MasterAccountId) {
		t.Fatalf("list-accounts missing the management account %q: %s", desc.Organization.MasterAccountId, la)
	}
}
