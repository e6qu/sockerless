package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

type IAMRole struct {
	RoleName                 string
	RoleId                   string
	Arn                      string
	Path                     string
	AssumeRolePolicyDocument string
	CreateDate               string
	MaxSessionDuration       int
}

type IAMRolePolicy struct {
	RoleName       string
	PolicyName     string
	PolicyDocument string
}

type IAMAttachedPolicy struct {
	RoleName   string
	PolicyArn  string
	PolicyName string
}

var (
	iamRoles            *sim.StateStore[IAMRole]
	iamRolePolicies     *sim.StateStore[IAMRolePolicy]
	iamAttachedPolicies *sim.StateStore[IAMAttachedPolicy]
)

func registerIAM(r *sim.AWSQueryRouter) {
	iamRoles = sim.NewStateStore[IAMRole]()
	iamRolePolicies = sim.NewStateStore[IAMRolePolicy]()
	iamAttachedPolicies = sim.NewStateStore[IAMAttachedPolicy]()

	r.Register("CreateRole", handleIAMCreateRole)
	r.Register("GetRole", handleIAMGetRole)
	r.Register("DeleteRole", handleIAMDeleteRole)
	r.Register("UpdateAssumeRolePolicy", handleIAMUpdateAssumeRolePolicy)
	r.Register("PutRolePolicy", handleIAMPutRolePolicy)
	r.Register("GetRolePolicy", handleIAMGetRolePolicy)
	r.Register("DeleteRolePolicy", handleIAMDeleteRolePolicy)
	r.Register("AttachRolePolicy", handleIAMAttachRolePolicy)
	r.Register("DetachRolePolicy", handleIAMDetachRolePolicy)
	r.Register("ListAttachedRolePolicies", handleIAMListAttachedRolePolicies)
	r.Register("ListRolePolicies", handleIAMListRolePolicies)
	r.Register("ListInstanceProfilesForRole", handleIAMListInstanceProfilesForRole)
}

func iamRoleXML(role IAMRole) string {
	doc := url.QueryEscape(role.AssumeRolePolicyDocument)
	maxSession := role.MaxSessionDuration
	if maxSession == 0 {
		maxSession = 3600
	}
	return fmt.Sprintf(`<Role><RoleName>%s</RoleName><RoleId>%s</RoleId><Arn>%s</Arn><Path>%s</Path><AssumeRolePolicyDocument>%s</AssumeRolePolicyDocument><CreateDate>%s</CreateDate><MaxSessionDuration>%d</MaxSessionDuration></Role>`,
		role.RoleName, role.RoleId, role.Arn, role.Path, doc, role.CreateDate, maxSession)
}

func handleIAMCreateRole(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("RoleName")
	path := r.FormValue("Path")
	if path == "" {
		path = "/"
	}
	assumeDoc := r.FormValue("AssumeRolePolicyDocument")
	if decoded, err := url.QueryUnescape(assumeDoc); err == nil {
		assumeDoc = decoded
	}

	role := IAMRole{
		RoleName:                 name,
		RoleId:                   "AROA" + strings.ToUpper(generateUUID()[:16]),
		Arn:                      fmt.Sprintf("arn:aws:iam::123456789012:role/%s", name),
		Path:                     path,
		AssumeRolePolicyDocument: assumeDoc,
		CreateDate:               time.Now().UTC().Format(time.RFC3339),
	}
	iamRoles.Put(name, role)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateRoleResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <CreateRoleResult>%s</CreateRoleResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</CreateRoleResponse>`, iamRoleXML(role), generateUUID())
}

func iamErrorXML(w http.ResponseWriter, code string, message string, statusCode int) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, `<ErrorResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error>
  <RequestId>%s</RequestId>
</ErrorResponse>`, code, message, generateUUID())
}

func handleIAMGetRole(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("RoleName")
	role, ok := iamRoles.Get(name)
	if !ok {
		iamErrorXML(w, "NoSuchEntity", fmt.Sprintf("The role with name %s cannot be found.", name), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<GetRoleResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <GetRoleResult>%s</GetRoleResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</GetRoleResponse>`, iamRoleXML(role), generateUUID())
}

func handleIAMDeleteRole(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("RoleName")
	iamRoles.Delete(name)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteRoleResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</DeleteRoleResponse>`, generateUUID())
}

func handleIAMUpdateAssumeRolePolicy(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("RoleName")
	policyDoc := r.FormValue("PolicyDocument")
	if decoded, err := url.QueryUnescape(policyDoc); err == nil {
		policyDoc = decoded
	}

	if ok := iamRoles.Update(name, func(role *IAMRole) {
		role.AssumeRolePolicyDocument = policyDoc
	}); !ok {
		iamErrorXML(w, "NoSuchEntity", fmt.Sprintf("The role with name %s cannot be found.", name), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<UpdateAssumeRolePolicyResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</UpdateAssumeRolePolicyResponse>`, generateUUID())
}

func handleIAMPutRolePolicy(w http.ResponseWriter, r *http.Request) {
	roleName := r.FormValue("RoleName")
	policyName := r.FormValue("PolicyName")
	policyDoc := r.FormValue("PolicyDocument")
	if decoded, err := url.QueryUnescape(policyDoc); err == nil {
		policyDoc = decoded
	}

	key := roleName + "/" + policyName
	iamRolePolicies.Put(key, IAMRolePolicy{
		RoleName:       roleName,
		PolicyName:     policyName,
		PolicyDocument: policyDoc,
	})

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<PutRolePolicyResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</PutRolePolicyResponse>`, generateUUID())
}

func handleIAMGetRolePolicy(w http.ResponseWriter, r *http.Request) {
	roleName := r.FormValue("RoleName")
	policyName := r.FormValue("PolicyName")
	key := roleName + "/" + policyName

	policy, ok := iamRolePolicies.Get(key)
	if !ok {
		iamErrorXML(w, "NoSuchEntity", fmt.Sprintf("The role policy with name %s cannot be found.", policyName), http.StatusNotFound)
		return
	}

	doc := url.QueryEscape(policy.PolicyDocument)
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<GetRolePolicyResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <GetRolePolicyResult>
    <RoleName>%s</RoleName>
    <PolicyName>%s</PolicyName>
    <PolicyDocument>%s</PolicyDocument>
  </GetRolePolicyResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</GetRolePolicyResponse>`, roleName, policyName, doc, generateUUID())
}

func handleIAMDeleteRolePolicy(w http.ResponseWriter, r *http.Request) {
	roleName := r.FormValue("RoleName")
	policyName := r.FormValue("PolicyName")
	iamRolePolicies.Delete(roleName + "/" + policyName)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteRolePolicyResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</DeleteRolePolicyResponse>`, generateUUID())
}

func handleIAMAttachRolePolicy(w http.ResponseWriter, r *http.Request) {
	roleName := r.FormValue("RoleName")
	policyArn := r.FormValue("PolicyArn")
	policyName := policyArn
	if idx := strings.LastIndex(policyArn, "/"); idx >= 0 {
		policyName = policyArn[idx+1:]
	}

	key := roleName + "/" + policyArn
	iamAttachedPolicies.Put(key, IAMAttachedPolicy{
		RoleName:   roleName,
		PolicyArn:  policyArn,
		PolicyName: policyName,
	})

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<AttachRolePolicyResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</AttachRolePolicyResponse>`, generateUUID())
}

func handleIAMDetachRolePolicy(w http.ResponseWriter, r *http.Request) {
	roleName := r.FormValue("RoleName")
	policyArn := r.FormValue("PolicyArn")
	iamAttachedPolicies.Delete(roleName + "/" + policyArn)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DetachRolePolicyResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</DetachRolePolicyResponse>`, generateUUID())
}

func handleIAMListAttachedRolePolicies(w http.ResponseWriter, r *http.Request) {
	roleName := r.FormValue("RoleName")
	policies := iamAttachedPolicies.Filter(func(p IAMAttachedPolicy) bool {
		return p.RoleName == roleName
	})

	var members strings.Builder
	for _, p := range policies {
		fmt.Fprintf(&members, "<member><PolicyName>%s</PolicyName><PolicyArn>%s</PolicyArn></member>", p.PolicyName, p.PolicyArn)
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ListAttachedRolePoliciesResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ListAttachedRolePoliciesResult>
    <AttachedPolicies>%s</AttachedPolicies>
    <IsTruncated>false</IsTruncated>
  </ListAttachedRolePoliciesResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</ListAttachedRolePoliciesResponse>`, members.String(), generateUUID())
}

func handleIAMListInstanceProfilesForRole(w http.ResponseWriter, r *http.Request) {
	// The simulator doesn't create instance profiles. Return empty list
	// so terraform can proceed with deleting IAM roles during destroy.
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ListInstanceProfilesForRoleResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ListInstanceProfilesForRoleResult>
    <InstanceProfiles/>
    <IsTruncated>false</IsTruncated>
  </ListInstanceProfilesForRoleResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</ListInstanceProfilesForRoleResponse>`, generateUUID())
}

func handleIAMListRolePolicies(w http.ResponseWriter, r *http.Request) {
	roleName := r.FormValue("RoleName")
	policies := iamRolePolicies.Filter(func(p IAMRolePolicy) bool {
		return p.RoleName == roleName
	})

	var members strings.Builder
	for _, p := range policies {
		fmt.Fprintf(&members, "<member>%s</member>", p.PolicyName)
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ListRolePoliciesResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ListRolePoliciesResult>
    <PolicyNames>%s</PolicyNames>
    <IsTruncated>false</IsTruncated>
  </ListRolePoliciesResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</ListRolePoliciesResponse>`, members.String(), generateUUID())
}
