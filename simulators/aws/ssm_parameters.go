package main

import (
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// SSM Parameter Store — runner workflows pull configuration values
// (database hosts, feature flags, sometimes secrets via SecureString)
// from the Parameter Store. `aws ssm get-parameter`,
// `aws-actions/aws-ssm-fetch-parameters`, and terraform's
// `aws_ssm_parameter` data source all hit this slice. Without it the
// runner setup fails before any actual workload runs.

// SSMParameter mirrors `aws.ssm.Parameter`. Real Parameter Store
// supports String, StringList, and SecureString types; the sim
// stores all three the same way (the value bytes are just JSON
// strings — SecureString in real AWS is KMS-encrypted but the sim
// doesn't have a KMS slice yet, so SecureString round-trips in the
// clear).
type SSMParameter struct {
	Name             string  `json:"Name"`
	Type             string  `json:"Type"`
	Value            string  `json:"Value"`
	Version          int64   `json:"Version"`
	LastModifiedDate float64 `json:"LastModifiedDate"`
	ARN              string  `json:"ARN"`
	DataType         string  `json:"DataType,omitempty"`
	Description      string  `json:"Description,omitempty"`
	Tier             string  `json:"Tier,omitempty"`
	KeyId            string  `json:"KeyId,omitempty"`
	AllowedPattern   string  `json:"AllowedPattern,omitempty"`
}

// SSMTag mirrors `aws.ssm.Tag` — used by AddTagsToResource /
// RemoveTagsFromResource / ListTagsForResource. Real SSM tags are
// resource-scoped (per Parameter, per Document, etc.), keyed by the
// (ResourceType, ResourceId) pair.
type SSMTag struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

var ssmParams sim.Store[SSMParameter]

// ssmResourceTags maps "<ResourceType>/<ResourceId>" → []SSMTag.
// Real SSM stores tags out-of-band from the resource itself; the sim
// follows suit so tag CRUD doesn't mutate the parameter row.
var ssmResourceTags sim.Store[[]SSMTag]

func ssmTagKey(resourceType, resourceID string) string {
	return resourceType + "/" + resourceID
}

func ssmParamArn(name string) string {
	// Real ARN: arn:aws:ssm:<region>:<account>:parameter<name-with-leading-slash>
	return "arn:aws:ssm:" + awsRegion() + ":" + awsAccountID() + ":parameter" + ensureLeadingSlash(name)
}

func ensureLeadingSlash(s string) string {
	if !strings.HasPrefix(s, "/") {
		return "/" + s
	}
	return s
}

func registerSSMParameterStore(r *sim.AWSRouter, srv *sim.Server) {
	ssmParams = sim.MakeStore[SSMParameter](srv.DB(), "ssm_parameters")
	ssmResourceTags = sim.MakeStore[[]SSMTag](srv.DB(), "ssm_resource_tags")

	r.Register("AmazonSSM.PutParameter", handleSSMPutParameter)
	r.Register("AmazonSSM.GetParameter", handleSSMGetParameter)
	r.Register("AmazonSSM.GetParameters", handleSSMGetParameters)
	r.Register("AmazonSSM.GetParametersByPath", handleSSMGetParametersByPath)
	r.Register("AmazonSSM.DescribeParameters", handleSSMDescribeParameters)
	r.Register("AmazonSSM.DeleteParameter", handleSSMDeleteParameter)
	r.Register("AmazonSSM.DeleteParameters", handleSSMDeleteParameters)
	r.Register("AmazonSSM.AddTagsToResource", handleSSMAddTagsToResource)
	r.Register("AmazonSSM.RemoveTagsFromResource", handleSSMRemoveTagsFromResource)
	r.Register("AmazonSSM.ListTagsForResource", handleSSMListTagsForResource)
}

// handleSSMAddTagsToResource attaches tags to a Parameter / Document /
// MaintenanceWindow / etc. Real SSM accepts any of the documented
// ResourceType values; the sim only models Parameter today (Documents
// + Windows aren't implemented). terraform-provider-aws calls this
// after PutParameter when tags are set.
func handleSSMAddTagsToResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceType string   `json:"ResourceType"`
		ResourceId   string   `json:"ResourceId"`
		Tags         []SSMTag `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ResourceType == "" || req.ResourceId == "" {
		sim.AWSError(w, "InvalidResourceId", "ResourceType and ResourceId are required", http.StatusBadRequest)
		return
	}
	if req.ResourceType == "Parameter" {
		if _, ok := ssmParams.Get(ensureLeadingSlash(req.ResourceId)); !ok {
			sim.AWSErrorf(w, "InvalidResourceId", http.StatusBadRequest,
				"The Parameter %q does not exist", req.ResourceId)
			return
		}
	}
	key := ssmTagKey(req.ResourceType, req.ResourceId)
	existing, _ := ssmResourceTags.Get(key)
	// Real SSM upsert semantics: re-tag with same Key replaces Value.
	merged := make([]SSMTag, 0, len(existing)+len(req.Tags))
	override := map[string]string{}
	for _, t := range req.Tags {
		override[t.Key] = t.Value
	}
	for _, t := range existing {
		if _, replaced := override[t.Key]; !replaced {
			merged = append(merged, t)
		}
	}
	merged = append(merged, req.Tags...)
	ssmResourceTags.Put(key, merged)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

// handleSSMRemoveTagsFromResource removes tag keys from a resource.
// Real SSM silently ignores missing keys; the sim mirrors that.
func handleSSMRemoveTagsFromResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceType string   `json:"ResourceType"`
		ResourceId   string   `json:"ResourceId"`
		TagKeys      []string `json:"TagKeys"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ResourceType == "" || req.ResourceId == "" {
		sim.AWSError(w, "InvalidResourceId", "ResourceType and ResourceId are required", http.StatusBadRequest)
		return
	}
	key := ssmTagKey(req.ResourceType, req.ResourceId)
	existing, _ := ssmResourceTags.Get(key)
	remove := map[string]bool{}
	for _, k := range req.TagKeys {
		remove[k] = true
	}
	filtered := existing[:0]
	for _, t := range existing {
		if !remove[t.Key] {
			filtered = append(filtered, t)
		}
	}
	ssmResourceTags.Put(key, filtered)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

// handleSSMListTagsForResource returns the tag set for a resource.
// terraform-provider-aws calls this after PutParameter / on refresh
// to populate the resource's `tags` attribute. Real SSM returns an
// empty TagList (not nil) when nothing is attached.
func handleSSMListTagsForResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceType string `json:"ResourceType"`
		ResourceId   string `json:"ResourceId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ResourceType == "" || req.ResourceId == "" {
		sim.AWSError(w, "InvalidResourceId", "ResourceType and ResourceId are required", http.StatusBadRequest)
		return
	}
	// Validate the underlying resource exists when we model it.
	if req.ResourceType == "Parameter" {
		if _, ok := ssmParams.Get(ensureLeadingSlash(req.ResourceId)); !ok {
			sim.AWSErrorf(w, "InvalidResourceId", http.StatusBadRequest,
				"The Parameter %q does not exist", req.ResourceId)
			return
		}
	}
	tags, _ := ssmResourceTags.Get(ssmTagKey(req.ResourceType, req.ResourceId))
	if tags == nil {
		tags = []SSMTag{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"TagList": tags,
	})
}

func handleSSMPutParameter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"Name"`
		Type           string `json:"Type"`
		Value          string `json:"Value"`
		Description    string `json:"Description"`
		Overwrite      bool   `json:"Overwrite"`
		KeyId          string `json:"KeyId"`
		AllowedPattern string `json:"AllowedPattern"`
		Tier           string `json:"Tier"`
		DataType       string `json:"DataType"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Value == "" {
		sim.AWSError(w, "ValidationException", "Name and Value are required", http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		req.Type = "String"
	}
	if req.Tier == "" {
		req.Tier = "Standard"
	}
	if req.DataType == "" {
		req.DataType = "text"
	}
	existing, exists := ssmParams.Get(req.Name)
	if exists && !req.Overwrite {
		sim.AWSErrorf(w, "ParameterAlreadyExists", http.StatusBadRequest,
			"The parameter already exists. To overwrite this value, set the overwrite option in the request to true.")
		return
	}

	now := float64(time.Now().Unix())
	version := int64(1)
	if exists {
		version = existing.Version + 1
	}
	param := SSMParameter{
		Name:             req.Name,
		Type:             req.Type,
		Value:            req.Value,
		Version:          version,
		LastModifiedDate: now,
		ARN:              ssmParamArn(req.Name),
		DataType:         req.DataType,
		Description:      req.Description,
		Tier:             req.Tier,
		KeyId:            req.KeyId,
		AllowedPattern:   req.AllowedPattern,
	}
	ssmParams.Put(req.Name, param)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Version": version,
		"Tier":    req.Tier,
	})
}

func handleSSMGetParameter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"Name"`
		WithDecryption bool   `json:"WithDecryption"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	param, ok := ssmParams.Get(req.Name)
	if !ok {
		sim.AWSErrorf(w, "ParameterNotFound", http.StatusBadRequest,
			"Parameter %s not found.", req.Name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Parameter": param,
	})
}

func handleSSMGetParameters(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Names          []string `json:"Names"`
		WithDecryption bool     `json:"WithDecryption"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	var found []SSMParameter
	var invalid []string
	for _, n := range req.Names {
		if p, ok := ssmParams.Get(n); ok {
			found = append(found, p)
		} else {
			invalid = append(invalid, n)
		}
	}
	if found == nil {
		found = []SSMParameter{}
	}
	if invalid == nil {
		invalid = []string{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Parameters":        found,
		"InvalidParameters": invalid,
	})
}

func handleSSMGetParametersByPath(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path           string `json:"Path"`
		Recursive      bool   `json:"Recursive"`
		WithDecryption bool   `json:"WithDecryption"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	prefix := ensureLeadingSlash(req.Path)
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	all := ssmParams.Filter(func(p SSMParameter) bool {
		paramName := ensureLeadingSlash(p.Name)
		if !strings.HasPrefix(paramName, prefix) {
			return false
		}
		if !req.Recursive {
			// Direct children only — no further `/` after the prefix.
			rest := paramName[len(prefix):]
			if strings.Contains(rest, "/") {
				return false
			}
		}
		return true
	})
	if all == nil {
		all = []SSMParameter{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Parameters": all})
}

func handleSSMDescribeParameters(w http.ResponseWriter, r *http.Request) {
	all := ssmParams.List()
	if all == nil {
		all = []SSMParameter{}
	}
	out := make([]map[string]any, 0, len(all))
	for _, p := range all {
		out = append(out, map[string]any{
			"Name":             p.Name,
			"Type":             p.Type,
			"Version":          p.Version,
			"LastModifiedDate": p.LastModifiedDate,
			"Description":      p.Description,
			"Tier":             p.Tier,
			"DataType":         p.DataType,
		})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Parameters": out})
}

func handleSSMDeleteParameter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"Name"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	if _, ok := ssmParams.Get(req.Name); !ok {
		sim.AWSErrorf(w, "ParameterNotFound", http.StatusBadRequest,
			"Parameter %s not found.", req.Name)
		return
	}
	ssmParams.Delete(req.Name)
	w.WriteHeader(http.StatusOK)
}

func handleSSMDeleteParameters(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Names []string `json:"Names"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	var deleted, invalid []string
	for _, n := range req.Names {
		if _, ok := ssmParams.Get(n); ok {
			ssmParams.Delete(n)
			deleted = append(deleted, n)
		} else {
			invalid = append(invalid, n)
		}
	}
	if deleted == nil {
		deleted = []string{}
	}
	if invalid == nil {
		invalid = []string{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"DeletedParameters": deleted,
		"InvalidParameters": invalid,
	})
}
