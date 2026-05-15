package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// AWS WAFv2 — JSON 1.1 over POST / + X-Amz-Target=AWSWAF_20190729.<Op>.
// Sim covers the CLOUDFRONT scope (global, us-east-1). REGIONAL scope
// (ALB / API Gateway path) is intentionally out of scope per Phase
// 159 plan — would compose with the same handlers if a backend needs
// it later. Resource bodies (Rules, VisibilityConfig, etc.) pass
// through as opaque json.RawMessage so the sim doesn't need to mirror
// the full SDK type tree — Terraform round-trips correctly because
// the wire bytes are preserved.

// ---------- WebACL ----------

type WAFWebACL struct {
	Name                         string          `json:"Name"`
	Id                           string          `json:"Id"`
	ARN                          string          `json:"ARN"`
	DefaultAction                json.RawMessage `json:"DefaultAction"`
	Description                  string          `json:"Description,omitempty"`
	Rules                        json.RawMessage `json:"Rules,omitempty"`
	VisibilityConfig             json.RawMessage `json:"VisibilityConfig"`
	Capacity                     int64           `json:"Capacity"`
	CustomResponseBodies         json.RawMessage `json:"CustomResponseBodies,omitempty"`
	CaptchaConfig                json.RawMessage `json:"CaptchaConfig,omitempty"`
	ChallengeConfig              json.RawMessage `json:"ChallengeConfig,omitempty"`
	TokenDomains                 []string        `json:"TokenDomains,omitempty"`
	AssociationConfig            json.RawMessage `json:"AssociationConfig,omitempty"`
	LabelNamespace               string          `json:"LabelNamespace,omitempty"`
	ApplicationConfig            json.RawMessage `json:"ApplicationConfig,omitempty"`
	RetrofittedByFirewallManager bool            `json:"RetrofittedByFirewallManager,omitempty"`
}

type WAFIPSet struct {
	Name             string   `json:"Name"`
	Id               string   `json:"Id"`
	ARN              string   `json:"ARN"`
	Description      string   `json:"Description,omitempty"`
	IPAddressVersion string   `json:"IPAddressVersion"`
	Addresses        []string `json:"Addresses"`
}

type WAFRuleGroup struct {
	Name                 string          `json:"Name"`
	Id                   string          `json:"Id"`
	ARN                  string          `json:"ARN"`
	Capacity             int64           `json:"Capacity"`
	Description          string          `json:"Description,omitempty"`
	Rules                json.RawMessage `json:"Rules,omitempty"`
	VisibilityConfig     json.RawMessage `json:"VisibilityConfig"`
	LabelNamespace       string          `json:"LabelNamespace,omitempty"`
	CustomResponseBodies json.RawMessage `json:"CustomResponseBodies,omitempty"`
}

type WAFRegexPatternSet struct {
	Name                  string          `json:"Name"`
	Id                    string          `json:"Id"`
	ARN                   string          `json:"ARN"`
	Description           string          `json:"Description,omitempty"`
	RegularExpressionList json.RawMessage `json:"RegularExpressionList,omitempty"`
}

type wafTag struct {
	Key   string `json:"Key"`
	Value string `json:"Value,omitempty"`
}

// Storage envelopes hold a Scope + LockToken alongside the resource.
type wafStoredWebACL struct {
	WebACL    WAFWebACL
	Scope     string
	LockToken string
	Tags      []wafTag
}

type wafStoredIPSet struct {
	IPSet     WAFIPSet
	Scope     string
	LockToken string
	Tags      []wafTag
}

type wafStoredRuleGroup struct {
	RuleGroup WAFRuleGroup
	Scope     string
	LockToken string
	Tags      []wafTag
}

type wafStoredRegex struct {
	RegexSet  WAFRegexPatternSet
	Scope     string
	LockToken string
	Tags      []wafTag
}

var (
	wafWebACLs    sim.Store[wafStoredWebACL]
	wafIPSets     sim.Store[wafStoredIPSet]
	wafRuleGroups sim.Store[wafStoredRuleGroup]
	wafRegexSets  sim.Store[wafStoredRegex]
	// wafAssociations: resourceARN → webACLARN. Tracks CloudFront
	// distribution → WebACL bindings.
	wafAssociations sync.Map
)

// ---------- Helpers ----------

func wafRandomID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	h := hex.EncodeToString(buf)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func wafLockToken() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	h := hex.EncodeToString(buf)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16]
}

// wafARN constructs an ARN. Real AWS convention:
//
//	CLOUDFRONT scope: arn:aws:wafv2:us-east-1:<acct>:global/<type>/<name>/<id>
//	REGIONAL scope:   arn:aws:wafv2:<region>:<acct>:regional/<type>/<name>/<id>
//
// The Terraform provider rejects "global" as a region value and expects
// "us-east-1" with "global/" appearing only in the resource path.
func wafARN(scope, resourceType, name, id string) string {
	region := awsRegion()
	scopePath := "regional"
	if scope == "CLOUDFRONT" {
		region = "us-east-1"
		scopePath = "global"
	}
	return fmt.Sprintf("arn:aws:wafv2:%s:%s:%s/%s/%s/%s", region, awsAccountID(), scopePath, resourceType, name, id)
}

// wafKey: stores resources under "<scope>/<id>" so the same Name + Id
// can collide between scopes (real AWS allows it).
func wafKey(scope, id string) string { return scope + "/" + id }

func wafWriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

func wafWriteError(w http.ResponseWriter, code, msg string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"__type":  code,
		"message": msg,
	})
}

// ---------- Registration ----------

func registerWAFv2(r *sim.AWSRouter, srv *sim.Server) {
	wafWebACLs = sim.MakeStore[wafStoredWebACL](srv.DB(), "wafv2_webacls")
	wafIPSets = sim.MakeStore[wafStoredIPSet](srv.DB(), "wafv2_ipsets")
	wafRuleGroups = sim.MakeStore[wafStoredRuleGroup](srv.DB(), "wafv2_rulegroups")
	wafRegexSets = sim.MakeStore[wafStoredRegex](srv.DB(), "wafv2_regex_sets")

	// WebACL
	r.Register("AWSWAF_20190729.CreateWebACL", handleWAFCreateWebACL)
	r.Register("AWSWAF_20190729.GetWebACL", handleWAFGetWebACL)
	r.Register("AWSWAF_20190729.UpdateWebACL", handleWAFUpdateWebACL)
	r.Register("AWSWAF_20190729.DeleteWebACL", handleWAFDeleteWebACL)
	r.Register("AWSWAF_20190729.ListWebACLs", handleWAFListWebACLs)
	// Association
	r.Register("AWSWAF_20190729.AssociateWebACL", handleWAFAssociateWebACL)
	r.Register("AWSWAF_20190729.DisassociateWebACL", handleWAFDisassociateWebACL)
	r.Register("AWSWAF_20190729.GetWebACLForResource", handleWAFGetWebACLForResource)
	r.Register("AWSWAF_20190729.ListResourcesForWebACL", handleWAFListResourcesForWebACL)
	// IPSet
	r.Register("AWSWAF_20190729.CreateIPSet", handleWAFCreateIPSet)
	r.Register("AWSWAF_20190729.GetIPSet", handleWAFGetIPSet)
	r.Register("AWSWAF_20190729.UpdateIPSet", handleWAFUpdateIPSet)
	r.Register("AWSWAF_20190729.DeleteIPSet", handleWAFDeleteIPSet)
	r.Register("AWSWAF_20190729.ListIPSets", handleWAFListIPSets)
	// RuleGroup
	r.Register("AWSWAF_20190729.CreateRuleGroup", handleWAFCreateRuleGroup)
	r.Register("AWSWAF_20190729.GetRuleGroup", handleWAFGetRuleGroup)
	r.Register("AWSWAF_20190729.UpdateRuleGroup", handleWAFUpdateRuleGroup)
	r.Register("AWSWAF_20190729.DeleteRuleGroup", handleWAFDeleteRuleGroup)
	r.Register("AWSWAF_20190729.ListRuleGroups", handleWAFListRuleGroups)
	// RegexPatternSet
	r.Register("AWSWAF_20190729.CreateRegexPatternSet", handleWAFCreateRegexSet)
	r.Register("AWSWAF_20190729.GetRegexPatternSet", handleWAFGetRegexSet)
	r.Register("AWSWAF_20190729.UpdateRegexPatternSet", handleWAFUpdateRegexSet)
	r.Register("AWSWAF_20190729.DeleteRegexPatternSet", handleWAFDeleteRegexSet)
	r.Register("AWSWAF_20190729.ListRegexPatternSets", handleWAFListRegexSets)
	// Tagging
	r.Register("AWSWAF_20190729.TagResource", handleWAFTagResource)
	r.Register("AWSWAF_20190729.UntagResource", handleWAFUntagResource)
	r.Register("AWSWAF_20190729.ListTagsForResource", handleWAFListTagsForResource)
	// Sampled requests (stub)
	r.Register("AWSWAF_20190729.GetSampledRequests", handleWAFGetSampledRequests)
}

// ---------- WebACL handlers ----------

type wafCreateWebACLReq struct {
	Name                 string          `json:"Name"`
	Scope                string          `json:"Scope"`
	DefaultAction        json.RawMessage `json:"DefaultAction"`
	Description          string          `json:"Description,omitempty"`
	Rules                json.RawMessage `json:"Rules,omitempty"`
	VisibilityConfig     json.RawMessage `json:"VisibilityConfig"`
	Tags                 []wafTag        `json:"Tags,omitempty"`
	CaptchaConfig        json.RawMessage `json:"CaptchaConfig,omitempty"`
	ChallengeConfig      json.RawMessage `json:"ChallengeConfig,omitempty"`
	TokenDomains         []string        `json:"TokenDomains,omitempty"`
	CustomResponseBodies json.RawMessage `json:"CustomResponseBodies,omitempty"`
	AssociationConfig    json.RawMessage `json:"AssociationConfig,omitempty"`
}

func handleWAFCreateWebACL(w http.ResponseWriter, r *http.Request) {
	var req wafCreateWebACLReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	if req.Name == "" || req.Scope == "" {
		wafWriteError(w, "WAFInvalidParameterException", "Name and Scope are required")
		return
	}
	id := wafRandomID()
	lock := wafLockToken()
	acl := WAFWebACL{
		Name:                 req.Name,
		Id:                   id,
		ARN:                  wafARN(req.Scope, "webacl", req.Name, id),
		DefaultAction:        req.DefaultAction,
		Description:          req.Description,
		Rules:                req.Rules,
		VisibilityConfig:     req.VisibilityConfig,
		CustomResponseBodies: req.CustomResponseBodies,
		CaptchaConfig:        req.CaptchaConfig,
		ChallengeConfig:      req.ChallengeConfig,
		TokenDomains:         req.TokenDomains,
		AssociationConfig:    req.AssociationConfig,
		LabelNamespace:       "awswaf:" + awsAccountID() + ":webacl:" + req.Name + ":",
		Capacity:             100, // sim doesn't compute real capacity
	}
	wafWebACLs.Put(wafKey(req.Scope, id), wafStoredWebACL{WebACL: acl, Scope: req.Scope, LockToken: lock, Tags: req.Tags})
	wafWriteJSON(w, map[string]any{
		"Summary": map[string]any{
			"Name":        acl.Name,
			"Id":          acl.Id,
			"Description": acl.Description,
			"LockToken":   lock,
			"ARN":         acl.ARN,
		},
	})
}

type wafGetReq struct {
	Name  string `json:"Name"`
	Scope string `json:"Scope"`
	Id    string `json:"Id"`
}

func handleWAFGetWebACL(w http.ResponseWriter, r *http.Request) {
	var req wafGetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	stored, ok := wafWebACLs.Get(wafKey(req.Scope, req.Id))
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "WebACL not found")
		return
	}
	wafWriteJSON(w, map[string]any{
		"WebACL":                    stored.WebACL,
		"LockToken":                 stored.LockToken,
		"ApplicationIntegrationURL": "https://" + awsRegion() + ".webacl-sim.example.com/" + stored.WebACL.Id,
	})
}

type wafUpdateWebACLReq struct {
	Name                 string          `json:"Name"`
	Scope                string          `json:"Scope"`
	Id                   string          `json:"Id"`
	LockToken            string          `json:"LockToken"`
	DefaultAction        json.RawMessage `json:"DefaultAction"`
	Description          string          `json:"Description,omitempty"`
	Rules                json.RawMessage `json:"Rules,omitempty"`
	VisibilityConfig     json.RawMessage `json:"VisibilityConfig"`
	CustomResponseBodies json.RawMessage `json:"CustomResponseBodies,omitempty"`
	CaptchaConfig        json.RawMessage `json:"CaptchaConfig,omitempty"`
	ChallengeConfig      json.RawMessage `json:"ChallengeConfig,omitempty"`
	TokenDomains         []string        `json:"TokenDomains,omitempty"`
	AssociationConfig    json.RawMessage `json:"AssociationConfig,omitempty"`
}

func handleWAFUpdateWebACL(w http.ResponseWriter, r *http.Request) {
	var req wafUpdateWebACLReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	key := wafKey(req.Scope, req.Id)
	stored, ok := wafWebACLs.Get(key)
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "WebACL not found")
		return
	}
	if req.LockToken != stored.LockToken {
		wafWriteError(w, "WAFOptimisticLockException", "LockToken does not match current value")
		return
	}
	stored.WebACL.DefaultAction = req.DefaultAction
	stored.WebACL.Description = req.Description
	stored.WebACL.Rules = req.Rules
	stored.WebACL.VisibilityConfig = req.VisibilityConfig
	stored.WebACL.CustomResponseBodies = req.CustomResponseBodies
	stored.WebACL.CaptchaConfig = req.CaptchaConfig
	stored.WebACL.ChallengeConfig = req.ChallengeConfig
	stored.WebACL.TokenDomains = req.TokenDomains
	stored.WebACL.AssociationConfig = req.AssociationConfig
	stored.LockToken = wafLockToken()
	wafWebACLs.Put(key, stored)
	wafWriteJSON(w, map[string]string{"NextLockToken": stored.LockToken})
}

type wafDeleteReq struct {
	Name      string `json:"Name"`
	Scope     string `json:"Scope"`
	Id        string `json:"Id"`
	LockToken string `json:"LockToken"`
}

func handleWAFDeleteWebACL(w http.ResponseWriter, r *http.Request) {
	var req wafDeleteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	key := wafKey(req.Scope, req.Id)
	stored, ok := wafWebACLs.Get(key)
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "WebACL not found")
		return
	}
	if req.LockToken != stored.LockToken {
		wafWriteError(w, "WAFOptimisticLockException", "LockToken does not match current value")
		return
	}
	// Refuse delete when associated. Mirrors real AWS.
	wafAssociations.Range(func(k, v any) bool {
		if v.(string) == stored.WebACL.ARN {
			wafWriteError(w, "WAFAssociatedItemException", "WebACL is still associated with one or more resources")
			ok = false
			return false
		}
		return true
	})
	if !ok {
		return
	}
	wafWebACLs.Delete(key)
	wafWriteJSON(w, struct{}{})
}

type wafListReq struct {
	Scope      string `json:"Scope"`
	NextMarker string `json:"NextMarker,omitempty"`
	Limit      int    `json:"Limit,omitempty"`
}

func handleWAFListWebACLs(w http.ResponseWriter, r *http.Request) {
	var req wafListReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	type summary struct {
		Name        string `json:"Name"`
		Id          string `json:"Id"`
		Description string `json:"Description,omitempty"`
		LockToken   string `json:"LockToken"`
		ARN         string `json:"ARN"`
	}
	items := []summary{}
	for _, s := range wafWebACLs.List() {
		if s.Scope != req.Scope {
			continue
		}
		items = append(items, summary{
			Name: s.WebACL.Name, Id: s.WebACL.Id,
			Description: s.WebACL.Description, LockToken: s.LockToken, ARN: s.WebACL.ARN,
		})
	}
	wafWriteJSON(w, map[string]any{"WebACLs": items})
}

// ---------- Association handlers ----------

type wafAssocReq struct {
	WebACLArn   string `json:"WebACLArn"`
	ResourceArn string `json:"ResourceArn"`
}

func handleWAFAssociateWebACL(w http.ResponseWriter, r *http.Request) {
	var req wafAssocReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	if req.WebACLArn == "" || req.ResourceArn == "" {
		wafWriteError(w, "WAFInvalidParameterException", "WebACLArn and ResourceArn are required")
		return
	}
	wafAssociations.Store(req.ResourceArn, req.WebACLArn)
	wafWriteJSON(w, struct{}{})
}

type wafDisassocReq struct {
	ResourceArn string `json:"ResourceArn"`
}

func handleWAFDisassociateWebACL(w http.ResponseWriter, r *http.Request) {
	var req wafDisassocReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	wafAssociations.Delete(req.ResourceArn)
	wafWriteJSON(w, struct{}{})
}

func handleWAFGetWebACLForResource(w http.ResponseWriter, r *http.Request) {
	var req wafDisassocReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	arnAny, _ := wafAssociations.Load(req.ResourceArn)
	arn, _ := arnAny.(string)
	if arn == "" {
		wafWriteJSON(w, struct{}{})
		return
	}
	// Find the WebACL by ARN
	for _, s := range wafWebACLs.List() {
		if s.WebACL.ARN == arn {
			wafWriteJSON(w, map[string]any{"WebACL": s.WebACL})
			return
		}
	}
	wafWriteJSON(w, struct{}{})
}

type wafListResourcesReq struct {
	WebACLArn    string `json:"WebACLArn"`
	ResourceType string `json:"ResourceType,omitempty"`
}

func handleWAFListResourcesForWebACL(w http.ResponseWriter, r *http.Request) {
	var req wafListResourcesReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	arns := []string{}
	wafAssociations.Range(func(k, v any) bool {
		if v.(string) == req.WebACLArn {
			arns = append(arns, k.(string))
		}
		return true
	})
	wafWriteJSON(w, map[string]any{"ResourceArns": arns})
}

// ---------- IPSet handlers ----------

type wafCreateIPSetReq struct {
	Name             string   `json:"Name"`
	Scope            string   `json:"Scope"`
	Description      string   `json:"Description,omitempty"`
	IPAddressVersion string   `json:"IPAddressVersion"`
	Addresses        []string `json:"Addresses"`
	Tags             []wafTag `json:"Tags,omitempty"`
}

func handleWAFCreateIPSet(w http.ResponseWriter, r *http.Request) {
	var req wafCreateIPSetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	if req.Name == "" || req.Scope == "" || req.IPAddressVersion == "" {
		wafWriteError(w, "WAFInvalidParameterException", "Name, Scope, IPAddressVersion are required")
		return
	}
	if req.Addresses == nil {
		req.Addresses = []string{}
	}
	id := wafRandomID()
	lock := wafLockToken()
	ipset := WAFIPSet{
		Name: req.Name, Id: id,
		ARN:              wafARN(req.Scope, "ipset", req.Name, id),
		Description:      req.Description,
		IPAddressVersion: req.IPAddressVersion,
		Addresses:        req.Addresses,
	}
	wafIPSets.Put(wafKey(req.Scope, id), wafStoredIPSet{IPSet: ipset, Scope: req.Scope, LockToken: lock, Tags: req.Tags})
	wafWriteJSON(w, map[string]any{
		"Summary": map[string]any{
			"Name":        ipset.Name,
			"Id":          ipset.Id,
			"Description": ipset.Description,
			"LockToken":   lock,
			"ARN":         ipset.ARN,
		},
	})
}

func handleWAFGetIPSet(w http.ResponseWriter, r *http.Request) {
	var req wafGetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	stored, ok := wafIPSets.Get(wafKey(req.Scope, req.Id))
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "IPSet not found")
		return
	}
	wafWriteJSON(w, map[string]any{"IPSet": stored.IPSet, "LockToken": stored.LockToken})
}

type wafUpdateIPSetReq struct {
	Name        string   `json:"Name"`
	Scope       string   `json:"Scope"`
	Id          string   `json:"Id"`
	Description string   `json:"Description,omitempty"`
	Addresses   []string `json:"Addresses"`
	LockToken   string   `json:"LockToken"`
}

func handleWAFUpdateIPSet(w http.ResponseWriter, r *http.Request) {
	var req wafUpdateIPSetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	key := wafKey(req.Scope, req.Id)
	stored, ok := wafIPSets.Get(key)
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "IPSet not found")
		return
	}
	if req.LockToken != stored.LockToken {
		wafWriteError(w, "WAFOptimisticLockException", "LockToken does not match")
		return
	}
	if req.Addresses == nil {
		req.Addresses = []string{}
	}
	stored.IPSet.Description = req.Description
	stored.IPSet.Addresses = req.Addresses
	stored.LockToken = wafLockToken()
	wafIPSets.Put(key, stored)
	wafWriteJSON(w, map[string]string{"NextLockToken": stored.LockToken})
}

func handleWAFDeleteIPSet(w http.ResponseWriter, r *http.Request) {
	var req wafDeleteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	key := wafKey(req.Scope, req.Id)
	stored, ok := wafIPSets.Get(key)
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "IPSet not found")
		return
	}
	if req.LockToken != stored.LockToken {
		wafWriteError(w, "WAFOptimisticLockException", "LockToken does not match")
		return
	}
	wafIPSets.Delete(key)
	wafWriteJSON(w, struct{}{})
}

func handleWAFListIPSets(w http.ResponseWriter, r *http.Request) {
	var req wafListReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	type summary struct {
		Name        string `json:"Name"`
		Id          string `json:"Id"`
		Description string `json:"Description,omitempty"`
		LockToken   string `json:"LockToken"`
		ARN         string `json:"ARN"`
	}
	items := []summary{}
	for _, s := range wafIPSets.List() {
		if s.Scope != req.Scope {
			continue
		}
		items = append(items, summary{
			Name: s.IPSet.Name, Id: s.IPSet.Id,
			Description: s.IPSet.Description, LockToken: s.LockToken, ARN: s.IPSet.ARN,
		})
	}
	wafWriteJSON(w, map[string]any{"IPSets": items})
}

// ---------- RuleGroup handlers ----------

type wafCreateRuleGroupReq struct {
	Name                 string          `json:"Name"`
	Scope                string          `json:"Scope"`
	Capacity             int64           `json:"Capacity"`
	Description          string          `json:"Description,omitempty"`
	Rules                json.RawMessage `json:"Rules,omitempty"`
	VisibilityConfig     json.RawMessage `json:"VisibilityConfig"`
	Tags                 []wafTag        `json:"Tags,omitempty"`
	CustomResponseBodies json.RawMessage `json:"CustomResponseBodies,omitempty"`
}

func handleWAFCreateRuleGroup(w http.ResponseWriter, r *http.Request) {
	var req wafCreateRuleGroupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	if req.Name == "" || req.Scope == "" {
		wafWriteError(w, "WAFInvalidParameterException", "Name and Scope are required")
		return
	}
	id := wafRandomID()
	lock := wafLockToken()
	rg := WAFRuleGroup{
		Name: req.Name, Id: id,
		ARN:                  wafARN(req.Scope, "rulegroup", req.Name, id),
		Capacity:             req.Capacity,
		Description:          req.Description,
		Rules:                req.Rules,
		VisibilityConfig:     req.VisibilityConfig,
		CustomResponseBodies: req.CustomResponseBodies,
		LabelNamespace:       "awswaf:" + awsAccountID() + ":rulegroup:" + req.Name + ":",
	}
	wafRuleGroups.Put(wafKey(req.Scope, id), wafStoredRuleGroup{RuleGroup: rg, Scope: req.Scope, LockToken: lock, Tags: req.Tags})
	wafWriteJSON(w, map[string]any{
		"Summary": map[string]any{
			"Name": rg.Name, "Id": rg.Id, "Description": rg.Description,
			"LockToken": lock, "ARN": rg.ARN,
		},
	})
}

func handleWAFGetRuleGroup(w http.ResponseWriter, r *http.Request) {
	var req wafGetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	stored, ok := wafRuleGroups.Get(wafKey(req.Scope, req.Id))
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "RuleGroup not found")
		return
	}
	wafWriteJSON(w, map[string]any{"RuleGroup": stored.RuleGroup, "LockToken": stored.LockToken})
}

func handleWAFUpdateRuleGroup(w http.ResponseWriter, r *http.Request) {
	var req wafCreateRuleGroupReq
	var idReq struct {
		Id        string `json:"Id"`
		LockToken string `json:"LockToken"`
	}
	body, _ := readBodyJSON(r.Body)
	_ = json.Unmarshal(body, &req)
	_ = json.Unmarshal(body, &idReq)
	key := wafKey(req.Scope, idReq.Id)
	stored, ok := wafRuleGroups.Get(key)
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "RuleGroup not found")
		return
	}
	if idReq.LockToken != stored.LockToken {
		wafWriteError(w, "WAFOptimisticLockException", "LockToken does not match")
		return
	}
	stored.RuleGroup.Description = req.Description
	stored.RuleGroup.Rules = req.Rules
	stored.RuleGroup.VisibilityConfig = req.VisibilityConfig
	stored.RuleGroup.CustomResponseBodies = req.CustomResponseBodies
	stored.LockToken = wafLockToken()
	wafRuleGroups.Put(key, stored)
	wafWriteJSON(w, map[string]string{"NextLockToken": stored.LockToken})
}

func readBodyJSON(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	// Drain the body into memory so we can parse it twice.
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return buf, err
		}
	}
	return buf, nil
}

func handleWAFDeleteRuleGroup(w http.ResponseWriter, r *http.Request) {
	var req wafDeleteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	key := wafKey(req.Scope, req.Id)
	stored, ok := wafRuleGroups.Get(key)
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "RuleGroup not found")
		return
	}
	if req.LockToken != stored.LockToken {
		wafWriteError(w, "WAFOptimisticLockException", "LockToken does not match")
		return
	}
	wafRuleGroups.Delete(key)
	wafWriteJSON(w, struct{}{})
}

func handleWAFListRuleGroups(w http.ResponseWriter, r *http.Request) {
	var req wafListReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	type summary struct {
		Name        string `json:"Name"`
		Id          string `json:"Id"`
		Description string `json:"Description,omitempty"`
		LockToken   string `json:"LockToken"`
		ARN         string `json:"ARN"`
	}
	items := []summary{}
	for _, s := range wafRuleGroups.List() {
		if s.Scope != req.Scope {
			continue
		}
		items = append(items, summary{Name: s.RuleGroup.Name, Id: s.RuleGroup.Id, Description: s.RuleGroup.Description, LockToken: s.LockToken, ARN: s.RuleGroup.ARN})
	}
	wafWriteJSON(w, map[string]any{"RuleGroups": items})
}

// ---------- RegexPatternSet handlers ----------

type wafCreateRegexReq struct {
	Name                  string          `json:"Name"`
	Scope                 string          `json:"Scope"`
	Description           string          `json:"Description,omitempty"`
	RegularExpressionList json.RawMessage `json:"RegularExpressionList,omitempty"`
	Tags                  []wafTag        `json:"Tags,omitempty"`
}

func handleWAFCreateRegexSet(w http.ResponseWriter, r *http.Request) {
	var req wafCreateRegexReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	if req.Name == "" || req.Scope == "" {
		wafWriteError(w, "WAFInvalidParameterException", "Name and Scope are required")
		return
	}
	id := wafRandomID()
	lock := wafLockToken()
	rs := WAFRegexPatternSet{
		Name: req.Name, Id: id,
		ARN:                   wafARN(req.Scope, "regexpatternset", req.Name, id),
		Description:           req.Description,
		RegularExpressionList: req.RegularExpressionList,
	}
	wafRegexSets.Put(wafKey(req.Scope, id), wafStoredRegex{RegexSet: rs, Scope: req.Scope, LockToken: lock, Tags: req.Tags})
	wafWriteJSON(w, map[string]any{
		"Summary": map[string]any{"Name": rs.Name, "Id": rs.Id, "Description": rs.Description, "LockToken": lock, "ARN": rs.ARN},
	})
}

func handleWAFGetRegexSet(w http.ResponseWriter, r *http.Request) {
	var req wafGetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	stored, ok := wafRegexSets.Get(wafKey(req.Scope, req.Id))
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "RegexPatternSet not found")
		return
	}
	wafWriteJSON(w, map[string]any{"RegexPatternSet": stored.RegexSet, "LockToken": stored.LockToken})
}

type wafUpdateRegexReq struct {
	Name                  string          `json:"Name"`
	Scope                 string          `json:"Scope"`
	Id                    string          `json:"Id"`
	Description           string          `json:"Description,omitempty"`
	RegularExpressionList json.RawMessage `json:"RegularExpressionList,omitempty"`
	LockToken             string          `json:"LockToken"`
}

func handleWAFUpdateRegexSet(w http.ResponseWriter, r *http.Request) {
	var req wafUpdateRegexReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	key := wafKey(req.Scope, req.Id)
	stored, ok := wafRegexSets.Get(key)
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "RegexPatternSet not found")
		return
	}
	if req.LockToken != stored.LockToken {
		wafWriteError(w, "WAFOptimisticLockException", "LockToken does not match")
		return
	}
	stored.RegexSet.Description = req.Description
	stored.RegexSet.RegularExpressionList = req.RegularExpressionList
	stored.LockToken = wafLockToken()
	wafRegexSets.Put(key, stored)
	wafWriteJSON(w, map[string]string{"NextLockToken": stored.LockToken})
}

func handleWAFDeleteRegexSet(w http.ResponseWriter, r *http.Request) {
	var req wafDeleteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	key := wafKey(req.Scope, req.Id)
	stored, ok := wafRegexSets.Get(key)
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "RegexPatternSet not found")
		return
	}
	if req.LockToken != stored.LockToken {
		wafWriteError(w, "WAFOptimisticLockException", "LockToken does not match")
		return
	}
	wafRegexSets.Delete(key)
	wafWriteJSON(w, struct{}{})
}

func handleWAFListRegexSets(w http.ResponseWriter, r *http.Request) {
	var req wafListReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	type summary struct {
		Name        string `json:"Name"`
		Id          string `json:"Id"`
		Description string `json:"Description,omitempty"`
		LockToken   string `json:"LockToken"`
		ARN         string `json:"ARN"`
	}
	items := []summary{}
	for _, s := range wafRegexSets.List() {
		if s.Scope != req.Scope {
			continue
		}
		items = append(items, summary{Name: s.RegexSet.Name, Id: s.RegexSet.Id, Description: s.RegexSet.Description, LockToken: s.LockToken, ARN: s.RegexSet.ARN})
	}
	wafWriteJSON(w, map[string]any{"RegexPatternSets": items})
}

// ---------- Tagging handlers ----------

func wafGetTagsByARN(arn string) ([]wafTag, bool) {
	for _, s := range wafWebACLs.List() {
		if s.WebACL.ARN == arn {
			return s.Tags, true
		}
	}
	for _, s := range wafIPSets.List() {
		if s.IPSet.ARN == arn {
			return s.Tags, true
		}
	}
	for _, s := range wafRuleGroups.List() {
		if s.RuleGroup.ARN == arn {
			return s.Tags, true
		}
	}
	for _, s := range wafRegexSets.List() {
		if s.RegexSet.ARN == arn {
			return s.Tags, true
		}
	}
	return nil, false
}

func wafSetTagsByARN(arn string, tags []wafTag) bool {
	for _, s := range wafWebACLs.List() {
		if s.WebACL.ARN == arn {
			s.Tags = tags
			wafWebACLs.Put(wafKey(s.Scope, s.WebACL.Id), s)
			return true
		}
	}
	for _, s := range wafIPSets.List() {
		if s.IPSet.ARN == arn {
			s.Tags = tags
			wafIPSets.Put(wafKey(s.Scope, s.IPSet.Id), s)
			return true
		}
	}
	for _, s := range wafRuleGroups.List() {
		if s.RuleGroup.ARN == arn {
			s.Tags = tags
			wafRuleGroups.Put(wafKey(s.Scope, s.RuleGroup.Id), s)
			return true
		}
	}
	for _, s := range wafRegexSets.List() {
		if s.RegexSet.ARN == arn {
			s.Tags = tags
			wafRegexSets.Put(wafKey(s.Scope, s.RegexSet.Id), s)
			return true
		}
	}
	return false
}

type wafTagReq struct {
	ResourceARN string   `json:"ResourceARN"`
	Tags        []wafTag `json:"Tags"`
}

func handleWAFTagResource(w http.ResponseWriter, r *http.Request) {
	var req wafTagReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	existing, ok := wafGetTagsByARN(req.ResourceARN)
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "Resource not found")
		return
	}
	tagMap := map[string]string{}
	for _, t := range existing {
		tagMap[t.Key] = t.Value
	}
	for _, t := range req.Tags {
		tagMap[t.Key] = t.Value
	}
	merged := make([]wafTag, 0, len(tagMap))
	for k, v := range tagMap {
		merged = append(merged, wafTag{Key: k, Value: v})
	}
	wafSetTagsByARN(req.ResourceARN, merged)
	wafWriteJSON(w, struct{}{})
}

type wafUntagReq struct {
	ResourceARN string   `json:"ResourceARN"`
	TagKeys     []string `json:"TagKeys"`
}

func handleWAFUntagResource(w http.ResponseWriter, r *http.Request) {
	var req wafUntagReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	existing, ok := wafGetTagsByARN(req.ResourceARN)
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "Resource not found")
		return
	}
	drop := map[string]bool{}
	for _, k := range req.TagKeys {
		drop[k] = true
	}
	kept := existing[:0]
	for _, t := range existing {
		if !drop[t.Key] {
			kept = append(kept, t)
		}
	}
	wafSetTagsByARN(req.ResourceARN, kept)
	wafWriteJSON(w, struct{}{})
}

type wafListTagsReq struct {
	ResourceARN string `json:"ResourceARN"`
}

func handleWAFListTagsForResource(w http.ResponseWriter, r *http.Request) {
	var req wafListTagsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wafWriteError(w, "WAFInvalidParameterException", "could not decode: "+err.Error())
		return
	}
	tags, ok := wafGetTagsByARN(req.ResourceARN)
	if !ok {
		wafWriteError(w, "WAFNonexistentItemException", "Resource not found")
		return
	}
	tagList := tags
	if tagList == nil {
		tagList = []wafTag{}
	}
	wafWriteJSON(w, map[string]any{
		"TagInfoForResource": map[string]any{
			"ResourceARN": req.ResourceARN,
			"TagList":     tagList,
		},
	})
}

// ---------- GetSampledRequests (stub) ----------

func handleWAFGetSampledRequests(w http.ResponseWriter, r *http.Request) {
	// Stub: sim doesn't simulate WAF traffic.
	wafWriteJSON(w, map[string]any{
		"SampledRequests": []any{},
		"PopulationSize":  0,
		"TimeWindow":      map[string]int64{"StartTime": time.Now().Add(-time.Hour).Unix(), "EndTime": time.Now().Unix()},
	})
}
