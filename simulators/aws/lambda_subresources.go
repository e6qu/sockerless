package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// LambdaVersion is an immutable snapshot of a function created by
// PublishVersion. Real Lambda increments the version monotonically
// from "1"; each version carries the same metadata as the function
// but with a frozen Code reference and an updated FunctionArn:
// `arn:aws:lambda:<region>:<account>:function:<name>:<version>`.
type LambdaVersion struct {
	FunctionName string              `json:"FunctionName"`
	FunctionArn  string              `json:"FunctionArn"`
	Version      string              `json:"Version"`
	Runtime      string              `json:"Runtime,omitempty"`
	Role         string              `json:"Role"`
	Handler      string              `json:"Handler,omitempty"`
	Code         *LambdaFunctionCode `json:"Code,omitempty"`
	CodeSize     int64               `json:"CodeSize"`
	MemorySize   int                 `json:"MemorySize"`
	Timeout      int                 `json:"Timeout"`
	State        string              `json:"State"`
	LastModified string              `json:"LastModified"`
	RevisionId   string              `json:"RevisionId"`
	PackageType  string              `json:"PackageType,omitempty"`
	Description  string              `json:"Description,omitempty"`
}

// LambdaAlias maps a name (e.g. "live") to a function version.
type LambdaAlias struct {
	AliasArn        string                    `json:"AliasArn"`
	Name            string                    `json:"Name"`
	FunctionVersion string                    `json:"FunctionVersion"`
	Description     string                    `json:"Description,omitempty"`
	RevisionId      string                    `json:"RevisionId"`
	RoutingConfig   *LambdaAliasRoutingConfig `json:"RoutingConfig,omitempty"`
}

type LambdaAliasRoutingConfig struct {
	AdditionalVersionWeights map[string]float64 `json:"AdditionalVersionWeights,omitempty"`
}

// LambdaPolicyStatement is one entry in the function's resource-policy
// document. AddPermission appends, RemovePermission removes by Sid.
type LambdaPolicyStatement struct {
	Sid       string         `json:"Sid"`
	Effect    string         `json:"Effect"`
	Principal map[string]any `json:"Principal"`
	Action    string         `json:"Action"`
	Resource  string         `json:"Resource"`
	Condition map[string]any `json:"Condition,omitempty"`
}

// LambdaFunctionUrlConfig is the per-function URL config. Real Lambda
// returns a canonical `FunctionUrl` like
// `https://<id>.lambda-url.<region>.on.aws/`; the SDK + Terraform
// provider read it as an opaque advertised URL. The sim emits the
// same canonical shape — external by design (sockerless does not
// host the `*.lambda-url.<region>.on.aws` subdomain).
type LambdaFunctionUrlConfig struct {
	FunctionArn      string `json:"FunctionArn"`
	FunctionUrl      string `json:"FunctionUrl"` // external: real-AWS canonical `<id>.lambda-url.<region>.on.aws`
	AuthType         string `json:"AuthType"`
	CreationTime     string `json:"CreationTime"`
	LastModifiedTime string `json:"LastModifiedTime"`
	InvokeMode       string `json:"InvokeMode,omitempty"`
	Cors             any    `json:"Cors,omitempty"`
}

// lambdaVersionExists reports whether the function has a published
// version with the given ID. `$LATEST` is the implicit always-live
// version every function carries; explicit IDs must match one
// PublishVersion call.
func lambdaVersionExists(fn, version string) bool {
	if version == "" || version == "$LATEST" {
		return true
	}
	lambdaVersionsMu.Lock()
	defer lambdaVersionsMu.Unlock()
	for _, v := range lambdaVersions[fn] {
		if v.Version == version {
			return true
		}
	}
	return false
}

// Stores. In-process maps; lifetime matches the running sim process,
// same as the function runtime state these subresources annotate.
var (
	lambdaVersionsMu   sync.Mutex
	lambdaVersions     = map[string][]LambdaVersion{}
	lambdaAliasesMu    sync.Mutex
	lambdaAliases      = map[string]map[string]LambdaAlias{}
	lambdaPoliciesMu   sync.Mutex
	lambdaPolicies     = map[string][]LambdaPolicyStatement{}
	lambdaURLConfigsMu sync.Mutex
	lambdaURLConfigs   = map[string]LambdaFunctionUrlConfig{}
)

func handleLambdaPublishVersion(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	fn, ok := lambdaFunctions.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", name)
		return
	}
	var req struct {
		Description string `json:"Description"`
		RevisionId  string `json:"RevisionId"`
	}
	if r.ContentLength > 0 {
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AWSError(w, "InvalidParameterValueException",
				"Invalid request body", http.StatusBadRequest)
			return
		}
	}

	lambdaVersionsMu.Lock()
	versions := lambdaVersions[name]
	nextNum := len(versions) + 1
	version := strconv.Itoa(nextNum)
	v := LambdaVersion{
		FunctionName: fn.FunctionName,
		FunctionArn:  fn.FunctionArn + ":" + version,
		Version:      version,
		Runtime:      fn.Runtime,
		Role:         fn.Role,
		Handler:      fn.Handler,
		Code:         fn.Code,
		CodeSize:     fn.CodeSize,
		MemorySize:   fn.MemorySize,
		Timeout:      fn.Timeout,
		State:        "Active",
		LastModified: time.Now().UTC().Format(time.RFC3339),
		RevisionId:   generateUUID(),
		PackageType:  fn.PackageType,
		Description:  req.Description,
	}
	lambdaVersions[name] = append(versions, v)
	lambdaVersionsMu.Unlock()
	sim.WriteJSON(w, http.StatusCreated, v)
}

func handleLambdaListVersions(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", name)
		return
	}
	lambdaVersionsMu.Lock()
	versions := append([]LambdaVersion(nil), lambdaVersions[name]...)
	lambdaVersionsMu.Unlock()
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Versions": versions})
}

func handleLambdaCreateAlias(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	fn, ok := lambdaFunctions.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", name)
		return
	}
	var req struct {
		Name            string                    `json:"Name"`
		FunctionVersion string                    `json:"FunctionVersion"`
		Description     string                    `json:"Description"`
		RoutingConfig   *LambdaAliasRoutingConfig `json:"RoutingConfig"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException",
			"Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		sim.AWSError(w, "InvalidParameterValueException",
			"Name is required", http.StatusBadRequest)
		return
	}
	if !lambdaVersionExists(name, req.FunctionVersion) {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function version not found: %s on %s", req.FunctionVersion, name)
		return
	}
	alias := LambdaAlias{
		AliasArn:        fn.FunctionArn + ":" + req.Name,
		Name:            req.Name,
		FunctionVersion: req.FunctionVersion,
		Description:     req.Description,
		RevisionId:      generateUUID(),
		RoutingConfig:   req.RoutingConfig,
	}
	lambdaAliasesMu.Lock()
	defer lambdaAliasesMu.Unlock()
	if lambdaAliases[name] == nil {
		lambdaAliases[name] = map[string]LambdaAlias{}
	}
	if _, exists := lambdaAliases[name][req.Name]; exists {
		sim.AWSErrorf(w, "ResourceConflictException", http.StatusConflict,
			"Alias already exists: %s", req.Name)
		return
	}
	lambdaAliases[name][req.Name] = alias
	sim.WriteJSON(w, http.StatusCreated, alias)
}

func handleLambdaListAliases(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", name)
		return
	}
	lambdaAliasesMu.Lock()
	defer lambdaAliasesMu.Unlock()
	out := make([]LambdaAlias, 0, len(lambdaAliases[name]))
	for _, a := range lambdaAliases[name] {
		out = append(out, a)
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Aliases": out})
}

func handleLambdaGetAlias(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	aliasName := sim.PathParam(r, "alias")
	lambdaAliasesMu.Lock()
	defer lambdaAliasesMu.Unlock()
	if as, ok := lambdaAliases[name]; ok {
		if a, ok := as[aliasName]; ok {
			sim.WriteJSON(w, http.StatusOK, a)
			return
		}
	}
	sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
		"Alias %s not found on function %s", aliasName, name)
}

func handleLambdaUpdateAlias(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	aliasName := sim.PathParam(r, "alias")
	var req struct {
		FunctionVersion string                    `json:"FunctionVersion"`
		Description     string                    `json:"Description"`
		RoutingConfig   *LambdaAliasRoutingConfig `json:"RoutingConfig"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException",
			"Invalid request body", http.StatusBadRequest)
		return
	}
	lambdaAliasesMu.Lock()
	defer lambdaAliasesMu.Unlock()
	as, ok := lambdaAliases[name]
	if !ok || as[aliasName].Name == "" {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Alias %s not found on function %s", aliasName, name)
		return
	}
	a := as[aliasName]
	if req.FunctionVersion != "" {
		if !lambdaVersionExists(name, req.FunctionVersion) {
			sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
				"Function version not found: %s on %s", req.FunctionVersion, name)
			return
		}
		a.FunctionVersion = req.FunctionVersion
	}
	if req.Description != "" {
		a.Description = req.Description
	}
	if req.RoutingConfig != nil {
		a.RoutingConfig = req.RoutingConfig
	}
	a.RevisionId = generateUUID()
	as[aliasName] = a
	sim.WriteJSON(w, http.StatusOK, a)
}

func handleLambdaDeleteAlias(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	aliasName := sim.PathParam(r, "alias")
	lambdaAliasesMu.Lock()
	defer lambdaAliasesMu.Unlock()
	if as, ok := lambdaAliases[name]; ok {
		delete(as, aliasName)
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleLambdaAddPermission(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	fn, ok := lambdaFunctions.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", name)
		return
	}
	var req struct {
		StatementId   string `json:"StatementId"`
		Action        string `json:"Action"`
		Principal     string `json:"Principal"`
		SourceArn     string `json:"SourceArn"`
		SourceAccount string `json:"SourceAccount"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException",
			"Invalid request body", http.StatusBadRequest)
		return
	}
	if req.StatementId == "" || req.Action == "" || req.Principal == "" {
		sim.AWSError(w, "InvalidParameterValueException",
			"StatementId, Action, Principal are required",
			http.StatusBadRequest)
		return
	}
	stmt := LambdaPolicyStatement{
		Sid:       req.StatementId,
		Effect:    "Allow",
		Principal: map[string]any{"Service": req.Principal},
		Action:    req.Action,
		Resource:  fn.FunctionArn,
	}
	if req.SourceArn != "" || req.SourceAccount != "" {
		cond := map[string]any{}
		if req.SourceArn != "" {
			cond["ArnLike"] = map[string]any{"AWS:SourceArn": req.SourceArn}
		}
		if req.SourceAccount != "" {
			cond["StringEquals"] = map[string]any{"AWS:SourceAccount": req.SourceAccount}
		}
		stmt.Condition = cond
	}
	lambdaPoliciesMu.Lock()
	for _, existing := range lambdaPolicies[name] {
		if existing.Sid == req.StatementId {
			lambdaPoliciesMu.Unlock()
			sim.AWSErrorf(w, "ResourceConflictException", http.StatusConflict,
				"Statement %s already exists", req.StatementId)
			return
		}
	}
	lambdaPolicies[name] = append(lambdaPolicies[name], stmt)
	lambdaPoliciesMu.Unlock()
	stmtJSON, err := json.Marshal(stmt)
	if err != nil {
		sim.AWSError(w, "InternalServerError",
			"failed to serialise policy statement: "+err.Error(),
			http.StatusInternalServerError)
		return
	}
	sim.WriteJSON(w, http.StatusCreated, map[string]any{
		"Statement": string(stmtJSON),
	})
}

func handleLambdaGetPolicy(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", name)
		return
	}
	lambdaPoliciesMu.Lock()
	stmts := append([]LambdaPolicyStatement(nil), lambdaPolicies[name]...)
	lambdaPoliciesMu.Unlock()
	if len(stmts) == 0 {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"No policy on function: %s", name)
		return
	}
	policyDoc := map[string]any{
		"Version":   "2012-10-17",
		"Id":        "default",
		"Statement": stmts,
	}
	docJSON, err := json.Marshal(policyDoc)
	if err != nil {
		sim.AWSError(w, "InternalServerError",
			"failed to serialise policy document: "+err.Error(),
			http.StatusInternalServerError)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Policy":     string(docJSON),
		"RevisionId": generateUUID(),
	})
}

func handleLambdaRemovePermission(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	sid := sim.PathParam(r, "statement")
	lambdaPoliciesMu.Lock()
	defer lambdaPoliciesMu.Unlock()
	stmts := lambdaPolicies[name]
	out := stmts[:0]
	found := false
	for _, s := range stmts {
		if s.Sid == sid {
			found = true
			continue
		}
		out = append(out, s)
	}
	if !found {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Statement %s not found on function %s", sid, name)
		return
	}
	lambdaPolicies[name] = out
	w.WriteHeader(http.StatusNoContent)
}

func handleLambdaCreateFunctionUrlConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	fn, ok := lambdaFunctions.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", name)
		return
	}
	var req struct {
		AuthType   string `json:"AuthType"`
		InvokeMode string `json:"InvokeMode"`
		Cors       any    `json:"Cors"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException",
			"Invalid request body", http.StatusBadRequest)
		return
	}
	if req.AuthType == "" {
		req.AuthType = "NONE"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	urlID := strings.ToLower(generateUUID()[:24])
	urlConfig := LambdaFunctionUrlConfig{
		FunctionArn:      fn.FunctionArn,
		FunctionUrl:      fmt.Sprintf("https://%s.lambda-url.%s.on.aws/", urlID, awsRegion()),
		AuthType:         req.AuthType,
		CreationTime:     now,
		LastModifiedTime: now,
		InvokeMode:       req.InvokeMode,
		Cors:             req.Cors,
	}
	lambdaURLConfigsMu.Lock()
	if _, exists := lambdaURLConfigs[name]; exists {
		lambdaURLConfigsMu.Unlock()
		sim.AWSErrorf(w, "ResourceConflictException", http.StatusConflict,
			"FunctionUrlConfig already exists for %s", name)
		return
	}
	lambdaURLConfigs[name] = urlConfig
	lambdaURLConfigsMu.Unlock()
	sim.WriteJSON(w, http.StatusCreated, urlConfig)
}

func handleLambdaGetFunctionUrlConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	lambdaURLConfigsMu.Lock()
	cfg, ok := lambdaURLConfigs[name]
	lambdaURLConfigsMu.Unlock()
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"FunctionUrlConfig not found for %s", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, cfg)
}

func handleLambdaUpdateFunctionUrlConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	var req struct {
		AuthType   string `json:"AuthType"`
		InvokeMode string `json:"InvokeMode"`
		Cors       any    `json:"Cors"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException",
			"Invalid request body", http.StatusBadRequest)
		return
	}
	lambdaURLConfigsMu.Lock()
	defer lambdaURLConfigsMu.Unlock()
	cfg, ok := lambdaURLConfigs[name]
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"FunctionUrlConfig not found for %s", name)
		return
	}
	if req.AuthType != "" {
		cfg.AuthType = req.AuthType
	}
	if req.InvokeMode != "" {
		cfg.InvokeMode = req.InvokeMode
	}
	if req.Cors != nil {
		cfg.Cors = req.Cors
	}
	cfg.LastModifiedTime = time.Now().UTC().Format(time.RFC3339)
	lambdaURLConfigs[name] = cfg
	sim.WriteJSON(w, http.StatusOK, cfg)
}

func handleLambdaDeleteFunctionUrlConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	lambdaURLConfigsMu.Lock()
	defer lambdaURLConfigsMu.Unlock()
	if _, ok := lambdaURLConfigs[name]; !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"FunctionUrlConfig not found for %s", name)
		return
	}
	delete(lambdaURLConfigs, name)
	w.WriteHeader(http.StatusNoContent)
}
