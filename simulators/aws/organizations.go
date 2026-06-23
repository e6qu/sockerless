package main

import (
	"net/http"
	"os"

	sim "github.com/sockerless/simulator"
)

// AWS Organizations — a minimal slice so the account belongs to an organization,
// which backs the aws:PrincipalOrgID IAM condition key. awsJson1.1, dispatched by
// X-Amz-Target AWSOrganizationsV20161128.<Op>. The org id is stable per process
// (override with SOCKERLESS_AWS_ORG_ID); the account the sim models is the
// organization's management account.

func awsOrgID() string {
	if id := os.Getenv("SOCKERLESS_AWS_ORG_ID"); id != "" {
		return id
	}
	return "o-sim000000"
}

func awsOrgArn() string {
	return "arn:aws:organizations::" + awsAccountID() + ":organization/" + awsOrgID()
}

func registerOrganizations(r *sim.AWSRouter, srv *sim.Server) {
	r.Register("AWSOrganizationsV20161128.DescribeOrganization", handleOrgDescribeOrganization)
	r.Register("AWSOrganizationsV20161128.ListAccounts", handleOrgListAccounts)
}

func orgManagementAccount() map[string]any {
	acct := awsAccountID()
	return map[string]any{
		"Id":              acct,
		"Arn":             "arn:aws:organizations::" + acct + ":account/" + awsOrgID() + "/" + acct,
		"Email":           "management@sim.invalid",
		"Name":            "Management",
		"Status":          "ACTIVE",
		"JoinedMethod":    "INVITED",
		"JoinedTimestamp": 0,
	}
}

func handleOrgDescribeOrganization(w http.ResponseWriter, _ *http.Request) {
	acct := awsAccountID()
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Organization": map[string]any{
			"Id":                 awsOrgID(),
			"Arn":                awsOrgArn(),
			"FeatureSet":         "ALL",
			"MasterAccountId":    acct,
			"MasterAccountArn":   "arn:aws:organizations::" + acct + ":account/" + awsOrgID() + "/" + acct,
			"MasterAccountEmail": "management@sim.invalid",
			"AvailablePolicyTypes": []map[string]any{
				{"Type": "SERVICE_CONTROL_POLICY", "Status": "ENABLED"},
			},
		},
	})
}

func handleOrgListAccounts(w http.ResponseWriter, _ *http.Request) {
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Accounts": []map[string]any{orgManagementAccount()},
	})
}
