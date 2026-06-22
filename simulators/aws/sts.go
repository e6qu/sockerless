package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// STS: AssumeRole / AssumeRoleWithWebIdentity / GetSessionToken / GetCallerIdentity.
// Assume-role and get-session-token mint temporary credentials (ASIA… access
// keys) and record, in iamTempCreds, which principal they stand in for, so the
// call-time IAM enforcement gate (iam_enforcement.go) can resolve a temporary
// key back to the role (or user) whose policies should be evaluated.

// IAMTempCred binds a temporary (ASIA…) access key to the principal it
// represents: an assumed role (RoleName set) or a user session (UserName set).
type IAMTempCred struct {
	AccessKeyID  string
	UserName     string // set for GetSessionToken (the caller's user)
	RoleName     string // set for AssumeRole / AssumeRoleWithWebIdentity
	PrincipalArn string // the caller-facing ARN (assumed-role/… or user/…)
	Expiration   string
}

var iamTempCreds sim.Store[IAMTempCred]

func registerSTS(r *sim.AWSQueryRouter, srv *sim.Server) {
	iamTempCreds = sim.MakeStore[IAMTempCred](srv.DB(), "iam_temp_creds")
	r.Register("GetCallerIdentity", handleGetCallerIdentity)
	r.Register("AssumeRole", handleSTSAssumeRole)
	r.Register("AssumeRoleWithWebIdentity", handleSTSAssumeRoleWithWebIdentity)
	r.Register("GetSessionToken", handleSTSGetSessionToken)
}

// iamPrincipalForAccessKey resolves a SigV4 access-key id to the caller-facing
// ARN and the policy documents that govern it: a registered IAM user (AKIA…),
// or a temporary credential's role/user (ASIA…). ok is false for unknown/test
// credentials (the permissive default).
func iamPrincipalForAccessKey(akid string) (arn string, docs []iamPolicyDoc, userName string, ok bool) {
	if akid == "" {
		return "", nil, "", false
	}
	if tc, found := iamTempCreds.Get(akid); found {
		if tc.RoleName != "" {
			return tc.PrincipalArn, iamPolicyDocsForRole(tc.RoleName), "", true
		}
		if tc.UserName != "" {
			if u, uok := iamUsers.Get(tc.UserName); uok {
				return tc.PrincipalArn, iamEffectivePolicyDocsForUser(u.UserName), u.UserName, true
			}
		}
		return tc.PrincipalArn, nil, "", true
	}
	if key, found := iamAccessKeys.Get(akid); found {
		if u, uok := iamUsers.Get(key.UserName); uok {
			return u.Arn, iamEffectivePolicyDocsForUser(u.UserName), u.UserName, true
		}
	}
	return "", nil, "", false
}

func handleGetCallerIdentity(w http.ResponseWriter, r *http.Request) {
	acct := awsAccountID()
	arn := fmt.Sprintf("arn:aws:iam::%s:user/simulator", acct)
	userID := "AIDASIMULATORCALLER0"
	if principalArn, _, _, ok := iamPrincipalForAccessKey(iamAccessKeyIDFromRequest(r)); ok {
		arn = principalArn
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>%s</Arn>
    <UserId>%s</UserId>
    <Account>%s</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</GetCallerIdentityResponse>`, xmlEscape(arn), userID, acct, generateUUID())
}

func stsDurationSeconds(r *http.Request) int {
	d := atoiDefault(r.FormValue("DurationSeconds"), 3600)
	if d < 900 {
		d = 900
	}
	if d > 43200 {
		d = 43200
	}
	return d
}

func stsMintTempCred() (akid, secret, token string) {
	return "ASIA" + strings.ToUpper(iamRandomB32(16)), iamRandomSecret(), iamRandomB32(64)
}

func handleSTSAssumeRole(w http.ResponseWriter, r *http.Request) {
	roleArn := r.FormValue("RoleArn")
	sessionName := r.FormValue("RoleSessionName")
	if roleArn == "" || sessionName == "" {
		stsErrorXML(w, "ValidationError", "RoleArn and RoleSessionName are required", http.StatusBadRequest)
		return
	}
	roleName := iamRoleNameFromArn(roleArn)
	role, ok := iamRoles.Get(roleName)
	if !ok {
		stsErrorXML(w, "AccessDenied",
			fmt.Sprintf("User is not authorized to perform: sts:AssumeRole on resource: %s (role not found)", roleArn),
			http.StatusForbidden)
		return
	}
	akid, secret, token := stsMintTempCred()
	exp := time.Now().UTC().Add(time.Duration(stsDurationSeconds(r)) * time.Second)
	assumedArn := fmt.Sprintf("arn:aws:sts::%s:assumed-role/%s/%s", awsAccountID(), role.RoleName, sessionName)
	iamTempCreds.Put(akid, IAMTempCred{
		AccessKeyID: akid, RoleName: role.RoleName, PrincipalArn: assumedArn,
		Expiration: exp.Format(time.RFC3339),
	})
	assumedRoleID := role.RoleId + ":" + sessionName
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>%s</AccessKeyId>
      <SecretAccessKey>%s</SecretAccessKey>
      <SessionToken>%s</SessionToken>
      <Expiration>%s</Expiration>
    </Credentials>
    <AssumedRoleUser>
      <Arn>%s</Arn>
      <AssumedRoleId>%s</AssumedRoleId>
    </AssumedRoleUser>
  </AssumeRoleResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</AssumeRoleResponse>`, akid, xmlEscape(secret), xmlEscape(token), exp.Format(time.RFC3339),
		xmlEscape(assumedArn), assumedRoleID, generateUUID())
}

func handleSTSAssumeRoleWithWebIdentity(w http.ResponseWriter, r *http.Request) {
	roleArn := r.FormValue("RoleArn")
	sessionName := r.FormValue("RoleSessionName")
	if roleArn == "" || sessionName == "" || r.FormValue("WebIdentityToken") == "" {
		stsErrorXML(w, "ValidationError", "RoleArn, RoleSessionName and WebIdentityToken are required", http.StatusBadRequest)
		return
	}
	roleName := iamRoleNameFromArn(roleArn)
	role, ok := iamRoles.Get(roleName)
	if !ok {
		stsErrorXML(w, "AccessDenied", fmt.Sprintf("Not authorized to perform sts:AssumeRoleWithWebIdentity on %s", roleArn), http.StatusForbidden)
		return
	}
	akid, secret, token := stsMintTempCred()
	exp := time.Now().UTC().Add(time.Duration(stsDurationSeconds(r)) * time.Second)
	assumedArn := fmt.Sprintf("arn:aws:sts::%s:assumed-role/%s/%s", awsAccountID(), role.RoleName, sessionName)
	iamTempCreds.Put(akid, IAMTempCred{AccessKeyID: akid, RoleName: role.RoleName, PrincipalArn: assumedArn, Expiration: exp.Format(time.RFC3339)})
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <Credentials><AccessKeyId>%s</AccessKeyId><SecretAccessKey>%s</SecretAccessKey><SessionToken>%s</SessionToken><Expiration>%s</Expiration></Credentials>
    <AssumedRoleUser><Arn>%s</Arn><AssumedRoleId>%s</AssumedRoleId></AssumedRoleUser>
    <SubjectFromWebIdentityToken>%s</SubjectFromWebIdentityToken>
  </AssumeRoleWithWebIdentityResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</AssumeRoleWithWebIdentityResponse>`, akid, xmlEscape(secret), xmlEscape(token), exp.Format(time.RFC3339),
		xmlEscape(assumedArn), role.RoleId+":"+sessionName, "sim-web-identity-subject", generateUUID())
}

func handleSTSGetSessionToken(w http.ResponseWriter, r *http.Request) {
	akid, secret, token := stsMintTempCred()
	exp := time.Now().UTC().Add(time.Duration(stsDurationSeconds(r)) * time.Second)
	// Bind the session token to the caller's user (if registered) so it inherits
	// the user's policies under enforcement.
	tc := IAMTempCred{AccessKeyID: akid, Expiration: exp.Format(time.RFC3339)}
	if _, _, userName, ok := iamPrincipalForAccessKey(iamAccessKeyIDFromRequest(r)); ok && userName != "" {
		if u, uok := iamUsers.Get(userName); uok {
			tc.UserName = userName
			tc.PrincipalArn = u.Arn
		}
	}
	iamTempCreds.Put(akid, tc)
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<GetSessionTokenResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetSessionTokenResult><Credentials><AccessKeyId>%s</AccessKeyId><SecretAccessKey>%s</SecretAccessKey><SessionToken>%s</SessionToken><Expiration>%s</Expiration></Credentials></GetSessionTokenResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</GetSessionTokenResponse>`, akid, xmlEscape(secret), xmlEscape(token), exp.Format(time.RFC3339), generateUUID())
}

func stsErrorXML(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>%s</RequestId></ErrorResponse>`,
		code, xmlEscape(message), generateUUID())
}
