package aws_cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snsSandboxOTPForTestCLI mirrors the simulator's deterministic OTP derivation
// (sns_mobile_sms.go snsSandboxOTP) so the CLI verify-sms-sandbox-phone-number
// call can complete the Pending → Verified transition.
func snsSandboxOTPForTestCLI(phoneNumber string) string {
	var sum int
	for _, c := range phoneNumber {
		sum = (sum*31 + int(c)) % 1000000
	}
	out := ""
	for i := 0; i < 6; i++ {
		out = string(rune('0'+sum%10)) + out
		sum /= 10
	}
	return out
}

// TestSNSCLI_PlatformApplicationAndEndpoint exercises the mobile-push platform
// application + endpoint CRUD through the aws CLI.
func TestSNSCLI_PlatformApplicationAndEndpoint(t *testing.T) {
	out := runCLI(t, awsCLI("sns", "create-platform-application",
		"--name", "cli-gcm-app",
		"--platform", "GCM",
		"--attributes", "PlatformCredential=fake-key"))
	var app struct {
		PlatformApplicationArn string `json:"PlatformApplicationArn"`
	}
	parseJSON(t, out, &app)
	require.NotEmpty(t, app.PlatformApplicationArn)
	t.Cleanup(func() {
		_ = awsCLI("sns", "delete-platform-application",
			"--platform-application-arn", app.PlatformApplicationArn).Run()
	})

	out = runCLI(t, awsCLI("sns", "get-platform-application-attributes",
		"--platform-application-arn", app.PlatformApplicationArn))
	var getApp struct {
		Attributes map[string]string `json:"Attributes"`
	}
	parseJSON(t, out, &getApp)
	assert.Equal(t, "fake-key", getApp.Attributes["PlatformCredential"])

	runCLI(t, awsCLI("sns", "set-platform-application-attributes",
		"--platform-application-arn", app.PlatformApplicationArn,
		"--attributes", "Enabled=false"))

	out = runCLI(t, awsCLI("sns", "list-platform-applications"))
	assert.Contains(t, out, app.PlatformApplicationArn)

	out = runCLI(t, awsCLI("sns", "create-platform-endpoint",
		"--platform-application-arn", app.PlatformApplicationArn,
		"--token", "device-token-cli"))
	var ep struct {
		EndpointArn string `json:"EndpointArn"`
	}
	parseJSON(t, out, &ep)
	require.NotEmpty(t, ep.EndpointArn)

	out = runCLI(t, awsCLI("sns", "get-endpoint-attributes",
		"--endpoint-arn", ep.EndpointArn))
	var getEp struct {
		Attributes map[string]string `json:"Attributes"`
	}
	parseJSON(t, out, &getEp)
	assert.Equal(t, "device-token-cli", getEp.Attributes["Token"])

	runCLI(t, awsCLI("sns", "set-endpoint-attributes",
		"--endpoint-arn", ep.EndpointArn,
		"--attributes", "Enabled=false"))

	out = runCLI(t, awsCLI("sns", "list-endpoints-by-platform-application",
		"--platform-application-arn", app.PlatformApplicationArn))
	var listEp struct {
		Endpoints []struct {
			EndpointArn string `json:"EndpointArn"`
		} `json:"Endpoints"`
	}
	parseJSON(t, out, &listEp)
	require.Len(t, listEp.Endpoints, 1)
	assert.Equal(t, ep.EndpointArn, listEp.Endpoints[0].EndpointArn)

	runCLI(t, awsCLI("sns", "delete-endpoint", "--endpoint-arn", ep.EndpointArn))
	runCLI(t, awsCLI("sns", "delete-platform-application",
		"--platform-application-arn", app.PlatformApplicationArn))
}

// TestSNSCLI_SMSSandbox exercises create → verify → list for an SMS-sandbox
// phone number and the account-status read-back.
func TestSNSCLI_SMSSandbox(t *testing.T) {
	phone := "+12065550111"
	runCLI(t, awsCLI("sns", "create-sms-sandbox-phone-number",
		"--phone-number", phone,
		"--language-code", "en-US"))
	t.Cleanup(func() {
		_ = awsCLI("sns", "delete-sms-sandbox-phone-number", "--phone-number", phone).Run()
	})

	out := runCLI(t, awsCLI("sns", "list-sms-sandbox-phone-numbers"))
	assert.Contains(t, out, phone)
	assert.Contains(t, out, "Pending")

	// Wrong OTP rejected.
	errOut := runCLIExpectError(t, awsCLI("sns", "verify-sms-sandbox-phone-number",
		"--phone-number", phone,
		"--one-time-password", "000000"))
	assert.Contains(t, errOut, "VerificationException")

	// Correct OTP flips to Verified.
	runCLI(t, awsCLI("sns", "verify-sms-sandbox-phone-number",
		"--phone-number", phone,
		"--one-time-password", snsSandboxOTPForTestCLI(phone)))

	out = runCLI(t, awsCLI("sns", "list-sms-sandbox-phone-numbers"))
	var listVerified struct {
		PhoneNumbers []struct {
			PhoneNumber string `json:"PhoneNumber"`
			Status      string `json:"Status"`
		} `json:"PhoneNumbers"`
	}
	parseJSON(t, out, &listVerified)
	verified := false
	for _, n := range listVerified.PhoneNumbers {
		if n.PhoneNumber == phone {
			verified = n.Status == "Verified"
		}
	}
	assert.True(t, verified, "number should be Verified after the matching OTP")

	out = runCLI(t, awsCLI("sns", "get-sms-sandbox-account-status"))
	assert.Contains(t, out, "IsInSandbox")

	runCLI(t, awsCLI("sns", "delete-sms-sandbox-phone-number", "--phone-number", phone))
}

// TestSNSCLI_SMSAttributesAndOptOut exercises the account SMS attribute store,
// the opt-out checks, opt-in, and list-origination-numbers.
func TestSNSCLI_SMSAttributesAndOptOut(t *testing.T) {
	runCLI(t, awsCLI("sns", "set-sms-attributes",
		"--attributes", "DefaultSMSType=Transactional,MonthlySpendLimit=10"))

	out := runCLI(t, awsCLI("sns", "get-sms-attributes"))
	var attrs struct {
		Attributes map[string]string `json:"Attributes"`
	}
	parseJSON(t, out, &attrs)
	assert.Equal(t, "Transactional", attrs.Attributes["DefaultSMSType"])
	assert.Equal(t, "10", attrs.Attributes["MonthlySpendLimit"])

	// Filtered read returns only the requested attribute.
	out = runCLI(t, awsCLI("sns", "get-sms-attributes", "--attributes", "DefaultSMSType"))
	var attrsOne struct {
		Attributes map[string]string `json:"Attributes"`
	}
	parseJSON(t, out, &attrsOne)
	assert.Equal(t, "Transactional", attrsOne.Attributes["DefaultSMSType"])
	_, present := attrsOne.Attributes["MonthlySpendLimit"]
	assert.False(t, present)

	out = runCLI(t, awsCLI("sns", "check-if-phone-number-is-opted-out",
		"--phone-number", "+12065550222"))
	assert.Contains(t, strings.ToLower(out), "false")

	runCLI(t, awsCLI("sns", "list-phone-numbers-opted-out"))

	runCLI(t, awsCLI("sns", "opt-in-phone-number", "--phone-number", "+12065550222"))

	runCLI(t, awsCLI("sns", "list-origination-numbers"))
}

// TestSNSCLI_DataProtectionPolicy stores and reads a per-topic
// data-protection policy via the CLI.
func TestSNSCLI_DataProtectionPolicy(t *testing.T) {
	out := runCLI(t, awsCLI("sns", "create-topic", "--name", "cli-dpp-topic"))
	var topic struct {
		TopicArn string `json:"TopicArn"`
	}
	parseJSON(t, out, &topic)
	require.NotEmpty(t, topic.TopicArn)
	t.Cleanup(func() {
		_ = awsCLI("sns", "delete-topic", "--topic-arn", topic.TopicArn).Run()
	})

	policy := `{"Name":"cli-dpp","Description":"","Version":"2021-06-01","Statement":[{"DataDirection":"Inbound","Principal":["*"],"DataIdentifier":["arn:aws:dataprotection::aws:data-identifier/EmailAddress"],"Operation":{"Deny":{}}}]}`
	runCLI(t, awsCLI("sns", "put-data-protection-policy",
		"--resource-arn", topic.TopicArn,
		"--data-protection-policy", policy))

	out = runCLI(t, awsCLI("sns", "get-data-protection-policy",
		"--resource-arn", topic.TopicArn))
	var got struct {
		DataProtectionPolicy string `json:"DataProtectionPolicy"`
	}
	parseJSON(t, out, &got)
	assert.Equal(t, policy, got.DataProtectionPolicy)
}
