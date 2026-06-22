package main

import (
	"net"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Call-time IAM enforcement (GitHub issue #657). The policy evaluator
// (iamEvalDecision) already existed but was wired only into the diagnostic
// SimulatePrincipalPolicy endpoint. This gate runs it on real API calls: it
// resolves the caller's SigV4 access-key id to a registered IAM user, evaluates
// the user's effective policies for the request's action, and denies with the
// correct per-service error shape when the action isn't allowed.
//
// Enforcement applies ONLY to access keys that resolve to a registered IAM user
// (created via CreateUser + CreateAccessKey). Unknown / static test credentials
// are treated as permissive (the sim's existing default) — so a consumer proving
// least-privilege mints a real restricted key, while every other test keeps
// working unchanged.
//
// Scope (phase 1): action-level Allow/Deny with the request-derivable condition
// context (aws:username/userid/SourceIp/RequestedRegion). Resource-ARN scoping
// and per-resource condition keys (aws:ResourceTag/*, ecs:cluster) are staged —
// the resource is evaluated as "*" today, so an explicit Deny or a missing Allow
// is caught, which is the issue's primary case.

// iamEnforce returns true if the request is authorized (the handler should run)
// and false if it was denied (a response has already been written).
func iamEnforce(w http.ResponseWriter, r *http.Request) bool {
	akid := iamAccessKeyIDFromRequest(r)
	if akid == "" {
		return true // unsigned request — permissive (matches AuthPassthrough)
	}
	key, ok := iamAccessKeys.Get(akid)
	if !ok {
		return true // not a registered IAM credential — permissive test default
	}
	user, ok := iamUsers.Get(key.UserName)
	if !ok {
		return true // dangling key (user deleted) — don't block
	}
	action, ok := iamActionForRequest(r)
	if !ok {
		return true // operation we can't classify — don't block on an unknown
	}

	docs := iamPolicyDocsForUser(user.UserName)
	ctx := map[string][]string{
		"aws:username": {user.UserName},
		"aws:userid":   {user.UserId},
	}
	if ip := iamSourceIP(r); ip != "" {
		ctx["aws:SourceIp"] = []string{ip}
	}
	if region := iamRequestedRegion(r); region != "" {
		ctx["aws:RequestedRegion"] = []string{region}
	}

	decision, _ := iamEvalDecision(docs, action, "*", ctx)
	if decision == "allowed" {
		return true
	}
	iamWriteDeny(w, r, user.Arn, action)
	return false
}

// iamAccessKeyIDFromRequest extracts the SigV4 access-key id from the
// Authorization header (Credential=AKID/date/region/service/aws4_request).
func iamAccessKeyIDFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256") {
		return ""
	}
	i := strings.Index(auth, "Credential=")
	if i < 0 {
		return ""
	}
	cred := auth[i+len("Credential="):]
	if s := strings.IndexAny(cred, "/,"); s > 0 {
		return cred[:s]
	}
	return ""
}

func iamRequestedRegion(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	i := strings.Index(auth, "Credential=")
	if i < 0 {
		return ""
	}
	parts := strings.Split(auth[i+len("Credential="):], "/")
	if len(parts) >= 3 {
		return parts[2] // AKID / date / region / service / aws4_request
	}
	return ""
}

func iamSourceIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// iamActionForRequest derives the IAM action string (e.g. "ec2:CreateVolume",
// "ecs:RunTask") from an awsJson X-Amz-Target or an awsQuery Action, reusing the
// service-source mapping CloudTrail already maintains.
func iamActionForRequest(r *http.Request) (string, bool) {
	src, ok := awsEventSource(r)
	if !ok {
		return "", false
	}
	service := strings.SplitN(src, ".", 2)[0] // "ecs.amazonaws.com" → "ecs"
	var op string
	if target := r.Header.Get("X-Amz-Target"); target != "" {
		if i := strings.LastIndex(target, "."); i >= 0 {
			op = target[i+1:]
		}
	} else {
		op = r.FormValue("Action")
	}
	if service == "" || op == "" {
		return "", false
	}
	return service + ":" + op, true
}

// iamWriteDeny emits the deny error in the shape the calling service uses: EC2's
// query protocol returns UnauthorizedOperation (XML 403); other query services
// return AccessDenied (XML 403); awsJson services return AccessDeniedException
// (JSON 403).
func iamWriteDeny(w http.ResponseWriter, r *http.Request, principalArn, action string) {
	msg := "User: " + principalArn + " is not authorized to perform: " + action +
		" because no identity-based policy allows the " + action + " action"
	if r.Header.Get("X-Amz-Target") != "" {
		sim.AWSError(w, "AccessDeniedException", msg, http.StatusForbidden)
		return
	}
	if strings.HasPrefix(action, "ec2:") {
		ec2ErrorXML(w, "UnauthorizedOperation", msg, http.StatusForbidden)
		return
	}
	iamErrorXML(w, "AccessDenied", msg, http.StatusForbidden)
}
