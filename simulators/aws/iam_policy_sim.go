package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// IAM policy simulation: SimulateCustomPolicy / SimulatePrincipalPolicy.
// Implements a real (if compact) policy-evaluation engine over the IAM policy
// JSON the consumer renders from terraform, so least-privilege IAM can be
// asserted against the sim. Real API:
// https://docs.aws.amazon.com/IAM/latest/APIReference/API_SimulateCustomPolicy.html

// ---------- policy document model ----------

// iamStringOrList unmarshals an IAM field that may be a single string or a
// JSON array of strings (Action/Resource/condition values all allow both).
type iamStringOrList []string

func (s *iamStringOrList) UnmarshalJSON(b []byte) error {
	b = []byte(strings.TrimSpace(string(b)))
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '[' {
		var arr []string
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		*s = arr
		return nil
	}
	var one string
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*s = []string{one}
	return nil
}

type iamStatement struct {
	Sid         string                                `json:"Sid"`
	Effect      string                                `json:"Effect"`
	Action      iamStringOrList                       `json:"Action"`
	NotAction   iamStringOrList                       `json:"NotAction"`
	Resource    iamStringOrList                       `json:"Resource"`
	NotResource iamStringOrList                       `json:"NotResource"`
	Condition   map[string]map[string]iamStringOrList `json:"Condition"`
}

type iamPolicyDoc struct {
	Version   string
	Statement []iamStatement
}

// parseIAMPolicy parses a policy document whose `Statement` may be a single
// object or an array.
func parseIAMPolicy(s string) (iamPolicyDoc, error) {
	var raw struct {
		Version   string          `json:"Version"`
		Statement json.RawMessage `json:"Statement"`
	}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return iamPolicyDoc{}, err
	}
	doc := iamPolicyDoc{Version: raw.Version}
	trimmed := strings.TrimSpace(string(raw.Statement))
	if trimmed == "" || trimmed == "null" {
		return doc, nil
	}
	if trimmed[0] == '[' {
		if err := json.Unmarshal(raw.Statement, &doc.Statement); err != nil {
			return doc, err
		}
	} else {
		var one iamStatement
		if err := json.Unmarshal(raw.Statement, &one); err != nil {
			return doc, err
		}
		doc.Statement = []iamStatement{one}
	}
	return doc, nil
}

// ---------- matching ----------

// iamGlobMatch matches an IAM pattern (with `*` and `?`) against value.
func iamGlobMatch(pattern, value string) bool {
	// Classic two-pointer glob with backtracking on `*`.
	var p, v, star, mark int
	star = -1
	for v < len(value) {
		if p < len(pattern) && (pattern[p] == '?' || pattern[p] == value[v]) {
			p++
			v++
		} else if p < len(pattern) && pattern[p] == '*' {
			star = p
			mark = v
			p++
		} else if star != -1 {
			p = star + 1
			mark++
			v = mark
		} else {
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

func iamActionMatches(stmt iamStatement, action string) bool {
	lower := strings.ToLower(action)
	if len(stmt.NotAction) > 0 {
		for _, p := range stmt.NotAction {
			if iamGlobMatch(strings.ToLower(p), lower) {
				return false
			}
		}
		return true
	}
	for _, p := range stmt.Action {
		if iamGlobMatch(strings.ToLower(p), lower) {
			return true
		}
	}
	return false
}

func iamResourceMatches(stmt iamStatement, resource string) bool {
	if len(stmt.NotResource) > 0 {
		for _, p := range stmt.NotResource {
			if iamGlobMatch(p, resource) {
				return false
			}
		}
		return true
	}
	if len(stmt.Resource) == 0 {
		return true // no resource constraint (e.g. an action with "*" resource)
	}
	for _, p := range stmt.Resource {
		if iamGlobMatch(p, resource) {
			return true
		}
	}
	return false
}

func iamAnyMatch(ctxVals, wantVals []string, eq func(ctx, want string) bool) bool {
	for _, c := range ctxVals {
		for _, want := range wantVals {
			if eq(c, want) {
				return true
			}
		}
	}
	return false
}

// iamEvalConditionOp evaluates one condition operator. Unsupported operators
// return false (the gated statement does not apply) so an Allow can't
// spuriously grant on a condition the sim doesn't model.
func iamEvalConditionOp(op string, ctxVals, wantVals []string) bool {
	switch op {
	case "StringEquals", "ArnEquals":
		return iamAnyMatch(ctxVals, wantVals, func(c, w string) bool { return c == w })
	case "StringNotEquals":
		return !iamAnyMatch(ctxVals, wantVals, func(c, w string) bool { return c == w })
	case "StringEqualsIgnoreCase":
		return iamAnyMatch(ctxVals, wantVals, strings.EqualFold)
	case "StringLike", "ArnLike":
		return iamAnyMatch(ctxVals, wantVals, func(c, w string) bool { return iamGlobMatch(w, c) })
	case "StringNotLike":
		return !iamAnyMatch(ctxVals, wantVals, func(c, w string) bool { return iamGlobMatch(w, c) })
	case "Bool":
		return iamAnyMatch(ctxVals, wantVals, strings.EqualFold)
	default:
		return false
	}
}

// iamConditionMatches evaluates a statement's Condition block. Returns whether
// it is satisfied plus any context keys that were referenced but not supplied.
func iamConditionMatches(stmt iamStatement, ctx map[string][]string) (bool, []string) {
	var missing []string
	for op, kv := range stmt.Condition {
		ifExists := strings.HasSuffix(op, "IfExists")
		baseOp := strings.TrimSuffix(op, "IfExists")
		for key, wantVals := range kv {
			ctxVals, present := ctx[key]
			if !present {
				if ifExists {
					continue
				}
				missing = append(missing, key)
				return false, missing
			}
			if !iamEvalConditionOp(baseOp, ctxVals, wantVals) {
				return false, missing
			}
		}
	}
	return true, missing
}

// iamEvalDecision evaluates an action/resource against the policies. Explicit
// deny always wins; otherwise any matching allow grants; otherwise implicit
// deny.
func iamEvalDecision(docs []iamPolicyDoc, action, resource string, ctx map[string][]string) (decision string, missing []string) {
	allowed := false
	for _, doc := range docs {
		for _, stmt := range doc.Statement {
			if !iamActionMatches(stmt, action) || !iamResourceMatches(stmt, resource) {
				continue
			}
			ok, miss := iamConditionMatches(stmt, ctx)
			missing = append(missing, miss...)
			if !ok {
				continue
			}
			switch strings.ToLower(stmt.Effect) {
			case "deny":
				return "explicitDeny", missing
			case "allow":
				allowed = true
			}
		}
	}
	if allowed {
		return "allowed", missing
	}
	return "implicitDeny", missing
}

// ---------- request parsing ----------

func iamQueryList(r *http.Request, key string) []string {
	var out []string
	for i := 1; ; i++ {
		v := r.FormValue(fmt.Sprintf("%s.member.%d", key, i))
		if v == "" {
			break
		}
		out = append(out, v)
	}
	return out
}

func iamParseContextEntries(r *http.Request) map[string][]string {
	ctx := map[string][]string{}
	for i := 1; ; i++ {
		name := r.FormValue(fmt.Sprintf("ContextEntries.member.%d.ContextKeyName", i))
		if name == "" {
			break
		}
		var vals []string
		for j := 1; ; j++ {
			v := r.FormValue(fmt.Sprintf("ContextEntries.member.%d.ContextKeyValues.member.%d", i, j))
			if v == "" {
				break
			}
			vals = append(vals, v)
		}
		ctx[name] = vals
	}
	return ctx
}

// ---------- handlers ----------

func handleIAMSimulateCustomPolicy(w http.ResponseWriter, r *http.Request) {
	docs := make([]iamPolicyDoc, 0)
	for _, p := range iamQueryList(r, "PolicyInputList") {
		doc, err := parseIAMPolicy(p)
		if err != nil {
			iamErrorXML(w, "InvalidInput", "Invalid policy document: "+err.Error(), http.StatusBadRequest)
			return
		}
		docs = append(docs, doc)
	}
	iamWriteSimulationResponse(w, "SimulateCustomPolicy", docs,
		iamQueryList(r, "ActionNames"), iamQueryList(r, "ResourceArns"), iamParseContextEntries(r))
}

func handleIAMSimulatePrincipalPolicy(w http.ResponseWriter, r *http.Request) {
	roleName := iamRoleNameFromArn(r.FormValue("PolicySourceArn"))
	docs := make([]iamPolicyDoc, 0)
	for _, rp := range iamRolePolicies.List() {
		if rp.RoleName == roleName {
			if doc, err := parseIAMPolicy(rp.PolicyDocument); err == nil {
				docs = append(docs, doc)
			}
		}
	}
	for _, ap := range iamAttachedPolicies.List() {
		if ap.RoleName != roleName {
			continue
		}
		if mp, ok := iamPolicies.Get(ap.PolicyArn); ok {
			if doc, err := parseIAMPolicy(mp.PolicyDocument); err == nil {
				docs = append(docs, doc)
			}
		}
	}
	// Additional inline policies to evaluate alongside the principal's.
	for _, p := range iamQueryList(r, "PolicyInputList") {
		if doc, err := parseIAMPolicy(p); err == nil {
			docs = append(docs, doc)
		}
	}
	iamWriteSimulationResponse(w, "SimulatePrincipalPolicy", docs,
		iamQueryList(r, "ActionNames"), iamQueryList(r, "ResourceArns"), iamParseContextEntries(r))
}

func iamWriteSimulationResponse(w http.ResponseWriter, op string, docs []iamPolicyDoc, actions, resources []string, ctx map[string][]string) {
	if len(resources) == 0 {
		resources = []string{"*"}
	}
	var members strings.Builder
	for _, action := range actions {
		for _, resource := range resources {
			decision, missing := iamEvalDecision(docs, action, resource, ctx)
			var missingXML strings.Builder
			for _, m := range iamUniqueStrings(missing) {
				fmt.Fprintf(&missingXML, "<member>%s</member>", iamXMLEscape(m))
			}
			fmt.Fprintf(&members, `<member>
        <EvalActionName>%s</EvalActionName>
        <EvalResourceName>%s</EvalResourceName>
        <EvalDecision>%s</EvalDecision>
        <MatchedStatements/>
        <MissingContextValues>%s</MissingContextValues>
      </member>`, iamXMLEscape(action), iamXMLEscape(resource), decision, missingXML.String())
		}
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<%sResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <%sResult>
    <IsTruncated>false</IsTruncated>
    <EvaluationResults>%s</EvaluationResults>
  </%sResult>
  <ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata>
</%sResponse>`, op, op, members.String(), op, generateUUID(), op)
}

// iamRoleNameFromArn extracts the role name from an arn:aws:iam::acct:role/Name
// (or role/path/Name) source ARN.
func iamRoleNameFromArn(arn string) string {
	const marker = ":role/"
	i := strings.Index(arn, marker)
	if i < 0 {
		return arn
	}
	rest := arn[i+len(marker):]
	// A path may precede the name; the name is the final segment.
	if slash := strings.LastIndex(rest, "/"); slash >= 0 {
		return rest[slash+1:]
	}
	return rest
}

func iamUniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func iamXMLEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func registerIAMPolicySimulation(r *sim.AWSQueryRouter) {
	r.Register("SimulateCustomPolicy", handleIAMSimulateCustomPolicy)
	r.Register("SimulatePrincipalPolicy", handleIAMSimulatePrincipalPolicy)
}
