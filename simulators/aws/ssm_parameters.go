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

var ssmParams sim.Store[SSMParameter]

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

	r.Register("AmazonSSM.PutParameter", handleSSMPutParameter)
	r.Register("AmazonSSM.GetParameter", handleSSMGetParameter)
	r.Register("AmazonSSM.GetParameters", handleSSMGetParameters)
	r.Register("AmazonSSM.GetParametersByPath", handleSSMGetParametersByPath)
	r.Register("AmazonSSM.DescribeParameters", handleSSMDescribeParameters)
	r.Register("AmazonSSM.DeleteParameter", handleSSMDeleteParameter)
	r.Register("AmazonSSM.DeleteParameters", handleSSMDeleteParameters)
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
