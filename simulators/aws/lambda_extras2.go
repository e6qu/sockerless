package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// This file implements the remaining Lambda control-plane slices the AWS SDK,
// CLI, and Terraform exercise: function event-invoke configs, provisioned
// concurrency configs, code-signing configs (account-level + per-function
// attachment), runtime-management config, account settings, function recursion
// config, and layer-version permissions. All state lives in in-process maps
// whose lifetime matches the running sim, the same as the function / version /
// alias / layer state in the sibling lambda_*.go files.

func registerLambdaExtras2(srv *sim.Server) {
	mux := srv
	lambdaResource := cloudTrailRESTResource("AWS::Lambda::Function", "name", "arn")

	// Function event-invoke config (async invoke retry/age/destinations).
	mux.HandleFunc("PUT /2019-09-25/functions/{name}/event-invoke-config", cloudTrailRecordedREST("PutFunctionEventInvokeConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaPutFunctionEventInvokeConfig))
	mux.HandleFunc("GET /2019-09-25/functions/{name}/event-invoke-config", cloudTrailRecordedREST("GetFunctionEventInvokeConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaGetFunctionEventInvokeConfig))
	mux.HandleFunc("POST /2019-09-25/functions/{name}/event-invoke-config", cloudTrailRecordedREST("UpdateFunctionEventInvokeConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaUpdateFunctionEventInvokeConfig))
	mux.HandleFunc("DELETE /2019-09-25/functions/{name}/event-invoke-config", cloudTrailRecordedREST("DeleteFunctionEventInvokeConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaDeleteFunctionEventInvokeConfig))
	mux.HandleFunc("GET /2019-09-25/functions/{name}/event-invoke-config/list", cloudTrailRecordedREST("ListFunctionEventInvokeConfigs", "lambda.amazonaws.com", lambdaResource, handleLambdaListFunctionEventInvokeConfigs))

	// Provisioned concurrency config (per qualifier).
	mux.HandleFunc("PUT /2019-09-30/functions/{name}/provisioned-concurrency", cloudTrailRecordedREST("PutProvisionedConcurrencyConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaPutProvisionedConcurrencyConfig))
	mux.HandleFunc("GET /2019-09-30/functions/{name}/provisioned-concurrency", cloudTrailRecordedREST("GetProvisionedConcurrencyConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaGetProvisionedConcurrencyConfig))
	mux.HandleFunc("DELETE /2019-09-30/functions/{name}/provisioned-concurrency", cloudTrailRecordedREST("DeleteProvisionedConcurrencyConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaDeleteProvisionedConcurrencyConfig))
	// ListProvisionedConcurrencyConfigs shares the base path keyed on ?List=ALL;
	// the SDK serializes that query param. Go's ServeMux ignores the query when
	// matching, so a distinct registration would collide with GET above. The
	// GET handler dispatches on the List selector instead; register the op name
	// so it lands in the conformance REST registry.
	restRegisterOp("lambda.amazonaws.com", "ListProvisionedConcurrencyConfigs")

	// Account-level code-signing configs.
	mux.HandleFunc("POST /2020-04-22/code-signing-configs", cloudTrailRecordedREST("CreateCodeSigningConfig", "lambda.amazonaws.com", nil, handleLambdaCreateCodeSigningConfig))
	mux.HandleFunc("GET /2020-04-22/code-signing-configs", cloudTrailRecordedREST("ListCodeSigningConfigs", "lambda.amazonaws.com", nil, handleLambdaListCodeSigningConfigs))
	mux.HandleFunc("GET /2020-04-22/code-signing-configs/{arn...}", cloudTrailRecordedREST("GetCodeSigningConfig", "lambda.amazonaws.com", nil, handleLambdaGetCodeSigningConfig))
	mux.HandleFunc("PUT /2020-04-22/code-signing-configs/{arn...}", cloudTrailRecordedREST("UpdateCodeSigningConfig", "lambda.amazonaws.com", nil, handleLambdaUpdateCodeSigningConfig))
	mux.HandleFunc("DELETE /2020-04-22/code-signing-configs/{arn...}", cloudTrailRecordedREST("DeleteCodeSigningConfig", "lambda.amazonaws.com", nil, handleLambdaDeleteCodeSigningConfig))

	// Per-function code-signing config attachment.
	mux.HandleFunc("PUT /2020-06-30/functions/{name}/code-signing-config", cloudTrailRecordedREST("PutFunctionCodeSigningConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaPutFunctionCodeSigningConfig))
	mux.HandleFunc("GET /2020-06-30/functions/{name}/code-signing-config", cloudTrailRecordedREST("GetFunctionCodeSigningConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaGetFunctionCodeSigningConfig))
	mux.HandleFunc("DELETE /2020-06-30/functions/{name}/code-signing-config", cloudTrailRecordedREST("DeleteFunctionCodeSigningConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaDeleteFunctionCodeSigningConfig))

	// Runtime-management config.
	mux.HandleFunc("GET /2021-07-20/functions/{name}/runtime-management-config", cloudTrailRecordedREST("GetRuntimeManagementConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaGetRuntimeManagementConfig))
	mux.HandleFunc("PUT /2021-07-20/functions/{name}/runtime-management-config", cloudTrailRecordedREST("PutRuntimeManagementConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaPutRuntimeManagementConfig))

	// Account settings.
	mux.HandleFunc("GET /2016-08-19/account-settings", cloudTrailRecordedREST("GetAccountSettings", "lambda.amazonaws.com", nil, handleLambdaGetAccountSettings))
	mux.HandleFunc("GET /2016-08-19/account-settings/", cloudTrailRecordedREST("GetAccountSettings", "lambda.amazonaws.com", nil, handleLambdaGetAccountSettings))

	// Function recursion config.
	mux.HandleFunc("GET /2024-08-31/functions/{name}/recursion-config", cloudTrailRecordedREST("GetFunctionRecursionConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaGetFunctionRecursionConfig))
	mux.HandleFunc("PUT /2024-08-31/functions/{name}/recursion-config", cloudTrailRecordedREST("PutFunctionRecursionConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaPutFunctionRecursionConfig))

	// Layer-version permissions.
	mux.HandleFunc("POST /2018-10-31/layers/{layer}/versions/{version}/policy", cloudTrailRecordedREST("AddLayerVersionPermission", "lambda.amazonaws.com", nil, handleLambdaAddLayerVersionPermission))
	mux.HandleFunc("GET /2018-10-31/layers/{layer}/versions/{version}/policy", cloudTrailRecordedREST("GetLayerVersionPolicy", "lambda.amazonaws.com", nil, handleLambdaGetLayerVersionPolicy))
	mux.HandleFunc("DELETE /2018-10-31/layers/{layer}/versions/{version}/policy/{statement}", cloudTrailRecordedREST("RemoveLayerVersionPermission", "lambda.amazonaws.com", nil, handleLambdaRemoveLayerVersionPermission))
}

// ---------------------------------------------------------------------------
// Function event-invoke config
// ---------------------------------------------------------------------------

type lambdaDestination struct {
	Destination string `json:"Destination,omitempty"`
}

type lambdaDestinationConfig struct {
	OnSuccess *lambdaDestination `json:"OnSuccess,omitempty"`
	OnFailure *lambdaDestination `json:"OnFailure,omitempty"`
}

// LambdaEventInvokeConfig mirrors the FunctionEventInvokeConfig shape.
type LambdaEventInvokeConfig struct {
	LastModified             float64                  `json:"LastModified"`
	FunctionArn              string                   `json:"FunctionArn"`
	MaximumRetryAttempts     *int                     `json:"MaximumRetryAttempts,omitempty"`
	MaximumEventAgeInSeconds *int                     `json:"MaximumEventAgeInSeconds,omitempty"`
	DestinationConfig        *lambdaDestinationConfig `json:"DestinationConfig,omitempty"`
}

var (
	lambdaEICMu sync.Mutex
	// keyed by "<functionName>:<qualifier>" ($LATEST when no qualifier).
	lambdaEICs = map[string]LambdaEventInvokeConfig{}
)

func lambdaEICKey(name, qualifier string) string {
	if qualifier == "" {
		qualifier = "$LATEST"
	}
	return name + ":" + qualifier
}

// lambdaEICArn returns the FunctionArn echoed in an event-invoke config; the
// arn carries the qualifier suffix when one is set, matching real Lambda.
func lambdaEICArn(name, qualifier string) string {
	arn := lambdaArn(name)
	if qualifier != "" && qualifier != "$LATEST" {
		arn += ":" + qualifier
	}
	return arn
}

func handleLambdaPutFunctionEventInvokeConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	qualifier := r.URL.Query().Get("Qualifier")
	var req struct {
		MaximumRetryAttempts     *int                     `json:"MaximumRetryAttempts"`
		MaximumEventAgeInSeconds *int                     `json:"MaximumEventAgeInSeconds"`
		DestinationConfig        *lambdaDestinationConfig `json:"DestinationConfig"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	cfg := LambdaEventInvokeConfig{
		LastModified:             lambdaNowEpoch(),
		FunctionArn:              lambdaEICArn(name, qualifier),
		MaximumRetryAttempts:     req.MaximumRetryAttempts,
		MaximumEventAgeInSeconds: req.MaximumEventAgeInSeconds,
		DestinationConfig:        req.DestinationConfig,
	}
	lambdaEICMu.Lock()
	lambdaEICs[lambdaEICKey(name, qualifier)] = cfg
	lambdaEICMu.Unlock()
	sim.WriteJSON(w, http.StatusOK, cfg)
}

func handleLambdaUpdateFunctionEventInvokeConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	qualifier := r.URL.Query().Get("Qualifier")
	var req struct {
		MaximumRetryAttempts     *int                     `json:"MaximumRetryAttempts"`
		MaximumEventAgeInSeconds *int                     `json:"MaximumEventAgeInSeconds"`
		DestinationConfig        *lambdaDestinationConfig `json:"DestinationConfig"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	key := lambdaEICKey(name, qualifier)
	lambdaEICMu.Lock()
	cfg, ok := lambdaEICs[key]
	if !ok {
		lambdaEICMu.Unlock()
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s does not have an EventInvokeConfig.", lambdaEICArn(name, qualifier))
		return
	}
	// Update merges: only members present in the request overwrite.
	if req.MaximumRetryAttempts != nil {
		cfg.MaximumRetryAttempts = req.MaximumRetryAttempts
	}
	if req.MaximumEventAgeInSeconds != nil {
		cfg.MaximumEventAgeInSeconds = req.MaximumEventAgeInSeconds
	}
	if req.DestinationConfig != nil {
		cfg.DestinationConfig = req.DestinationConfig
	}
	cfg.LastModified = lambdaNowEpoch()
	lambdaEICs[key] = cfg
	lambdaEICMu.Unlock()
	sim.WriteJSON(w, http.StatusOK, cfg)
}

func handleLambdaGetFunctionEventInvokeConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	qualifier := r.URL.Query().Get("Qualifier")
	lambdaEICMu.Lock()
	cfg, ok := lambdaEICs[lambdaEICKey(name, qualifier)]
	lambdaEICMu.Unlock()
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s does not have an EventInvokeConfig.", lambdaEICArn(name, qualifier))
		return
	}
	sim.WriteJSON(w, http.StatusOK, cfg)
}

func handleLambdaDeleteFunctionEventInvokeConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	qualifier := r.URL.Query().Get("Qualifier")
	lambdaEICMu.Lock()
	delete(lambdaEICs, lambdaEICKey(name, qualifier))
	lambdaEICMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func handleLambdaListFunctionEventInvokeConfigs(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	prefix := name + ":"
	lambdaEICMu.Lock()
	configs := make([]LambdaEventInvokeConfig, 0)
	for k, cfg := range lambdaEICs {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			configs = append(configs, cfg)
		}
	}
	lambdaEICMu.Unlock()
	sort.Slice(configs, func(i, j int) bool { return configs[i].FunctionArn < configs[j].FunctionArn })
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"FunctionEventInvokeConfigs": configs,
	})
}

// ---------------------------------------------------------------------------
// Provisioned concurrency config
// ---------------------------------------------------------------------------

// LambdaProvisionedConcurrency mirrors the per-qualifier provisioned
// concurrency config. Real Lambda reports READY once allocation completes; the
// sim allocates synchronously so requested == available == allocated.
type LambdaProvisionedConcurrency struct {
	functionName string
	qualifier    string
	Requested    int
	LastModified string
}

var (
	lambdaPCMu sync.Mutex
	// keyed by "<functionName>:<qualifier>".
	lambdaPCs = map[string]LambdaProvisionedConcurrency{}
)

func lambdaProvisionedConcurrencyBody(pc LambdaProvisionedConcurrency, includeArn bool) map[string]any {
	out := map[string]any{
		"RequestedProvisionedConcurrentExecutions": pc.Requested,
		"AvailableProvisionedConcurrentExecutions": pc.Requested,
		"AllocatedProvisionedConcurrentExecutions": pc.Requested,
		"Status":       "READY",
		"LastModified": pc.LastModified,
	}
	if includeArn {
		out["FunctionArn"] = lambdaEICArn(pc.functionName, pc.qualifier)
	}
	return out
}

func handleLambdaPutProvisionedConcurrencyConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	qualifier := r.URL.Query().Get("Qualifier")
	if qualifier == "" {
		// Real Lambda rejects provisioned concurrency on the unpublished
		// $LATEST version — a qualifier is required.
		sim.AWSError(w, "InvalidParameterValueException",
			"Qualifier is required for provisioned concurrency", http.StatusBadRequest)
		return
	}
	var req struct {
		ProvisionedConcurrentExecutions *int `json:"ProvisionedConcurrentExecutions"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ProvisionedConcurrentExecutions == nil || *req.ProvisionedConcurrentExecutions < 1 {
		sim.AWSError(w, "InvalidParameterValueException",
			"'ProvisionedConcurrentExecutions' failed to satisfy constraint: Member must have value greater than or equal to 1",
			http.StatusBadRequest)
		return
	}
	pc := LambdaProvisionedConcurrency{
		functionName: name,
		qualifier:    qualifier,
		Requested:    *req.ProvisionedConcurrentExecutions,
		LastModified: time.Now().UTC().Format(time.RFC3339),
	}
	lambdaPCMu.Lock()
	lambdaPCs[name+":"+qualifier] = pc
	lambdaPCMu.Unlock()
	// PutProvisionedConcurrencyConfig returns 202 (the config is being set up).
	sim.WriteJSON(w, http.StatusAccepted, lambdaProvisionedConcurrencyBody(pc, false))
}

func handleLambdaGetProvisionedConcurrencyConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	// ListProvisionedConcurrencyConfigs shares this path keyed on ?List=ALL.
	if r.URL.Query().Get("List") == "ALL" {
		lambdaListProvisionedConcurrencyConfigs(w, name)
		return
	}
	qualifier := r.URL.Query().Get("Qualifier")
	lambdaPCMu.Lock()
	pc, ok := lambdaPCs[name+":"+qualifier]
	lambdaPCMu.Unlock()
	if !ok {
		sim.AWSErrorf(w, "ProvisionedConcurrencyConfigNotFoundException", http.StatusNotFound,
			"No Provisioned Concurrency Config found for this function")
		return
	}
	sim.WriteJSON(w, http.StatusOK, lambdaProvisionedConcurrencyBody(pc, false))
}

func lambdaListProvisionedConcurrencyConfigs(w http.ResponseWriter, name string) {
	prefix := name + ":"
	lambdaPCMu.Lock()
	configs := make([]map[string]any, 0)
	for k, pc := range lambdaPCs {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			configs = append(configs, lambdaProvisionedConcurrencyBody(pc, true))
		}
	}
	lambdaPCMu.Unlock()
	sort.Slice(configs, func(i, j int) bool {
		return fmt.Sprint(configs[i]["FunctionArn"]) < fmt.Sprint(configs[j]["FunctionArn"])
	})
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"ProvisionedConcurrencyConfigs": configs,
	})
}

func handleLambdaDeleteProvisionedConcurrencyConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	qualifier := r.URL.Query().Get("Qualifier")
	lambdaPCMu.Lock()
	delete(lambdaPCs, name+":"+qualifier)
	lambdaPCMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Code-signing configs (account-level)
// ---------------------------------------------------------------------------

type lambdaAllowedPublishers struct {
	SigningProfileVersionArns []string `json:"SigningProfileVersionArns,omitempty"`
}

type lambdaCodeSigningPolicies struct {
	UntrustedArtifactOnDeployment string `json:"UntrustedArtifactOnDeployment,omitempty"`
}

// LambdaCodeSigningConfig mirrors the CodeSigningConfig shape.
type LambdaCodeSigningConfig struct {
	CodeSigningConfigId  string                     `json:"CodeSigningConfigId"`
	CodeSigningConfigArn string                     `json:"CodeSigningConfigArn"`
	Description          string                     `json:"Description,omitempty"`
	AllowedPublishers    *lambdaAllowedPublishers   `json:"AllowedPublishers,omitempty"`
	CodeSigningPolicies  *lambdaCodeSigningPolicies `json:"CodeSigningPolicies,omitempty"`
	LastModified         string                     `json:"LastModified"`
}

var lambdaCSCStore sim.Store[LambdaCodeSigningConfig]

// lambdaCSCID generates a csc-<17 lowercase alphanumeric> identifier matching
// the CodeSigningConfigArn pattern in the smithy model.
func lambdaCSCID() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 17)
	raw := make([]byte, 17)
	_, _ = rand.Read(raw)
	for i := range b {
		b[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return "csc-" + string(b)
}

func lambdaCSCArn(id string) string {
	return fmt.Sprintf("arn:aws:lambda:%s:%s:code-signing-config:%s", awsRegion(), awsAccountID(), id)
}

func handleLambdaCreateCodeSigningConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Description         string                     `json:"Description"`
		AllowedPublishers   *lambdaAllowedPublishers   `json:"AllowedPublishers"`
		CodeSigningPolicies *lambdaCodeSigningPolicies `json:"CodeSigningPolicies"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.AllowedPublishers == nil || len(req.AllowedPublishers.SigningProfileVersionArns) == 0 {
		sim.AWSError(w, "InvalidParameterValueException",
			"AllowedPublishers must specify at least one SigningProfileVersionArn", http.StatusBadRequest)
		return
	}
	policies := req.CodeSigningPolicies
	if policies == nil {
		// Real Lambda defaults UntrustedArtifactOnDeployment to Warn.
		policies = &lambdaCodeSigningPolicies{UntrustedArtifactOnDeployment: "Warn"}
	}
	id := lambdaCSCID()
	csc := LambdaCodeSigningConfig{
		CodeSigningConfigId:  id,
		CodeSigningConfigArn: lambdaCSCArn(id),
		Description:          req.Description,
		AllowedPublishers:    req.AllowedPublishers,
		CodeSigningPolicies:  policies,
		LastModified:         time.Now().UTC().Format(time.RFC3339),
	}
	lambdaCSCStore.Put(csc.CodeSigningConfigArn, csc)
	sim.WriteJSON(w, http.StatusCreated, map[string]any{"CodeSigningConfig": csc})
}

func handleLambdaGetCodeSigningConfig(w http.ResponseWriter, r *http.Request) {
	arn := r.PathValue("arn")
	csc, ok := lambdaCSCStore.Get(arn)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The code signing configuration cannot be found.")
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"CodeSigningConfig": csc})
}

func handleLambdaUpdateCodeSigningConfig(w http.ResponseWriter, r *http.Request) {
	arn := r.PathValue("arn")
	var req struct {
		Description         *string                    `json:"Description"`
		AllowedPublishers   *lambdaAllowedPublishers   `json:"AllowedPublishers"`
		CodeSigningPolicies *lambdaCodeSigningPolicies `json:"CodeSigningPolicies"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	found := lambdaCSCStore.Update(arn, func(csc *LambdaCodeSigningConfig) {
		if req.Description != nil {
			csc.Description = *req.Description
		}
		if req.AllowedPublishers != nil {
			csc.AllowedPublishers = req.AllowedPublishers
		}
		if req.CodeSigningPolicies != nil {
			csc.CodeSigningPolicies = req.CodeSigningPolicies
		}
		csc.LastModified = time.Now().UTC().Format(time.RFC3339)
	})
	if !found {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The code signing configuration cannot be found.")
		return
	}
	csc, _ := lambdaCSCStore.Get(arn)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"CodeSigningConfig": csc})
}

func handleLambdaDeleteCodeSigningConfig(w http.ResponseWriter, r *http.Request) {
	arn := r.PathValue("arn")
	if !lambdaCSCStore.Delete(arn) {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The code signing configuration cannot be found.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleLambdaListCodeSigningConfigs(w http.ResponseWriter, r *http.Request) {
	stored := lambdaCSCStore.List()
	sortBy(stored, func(c LambdaCodeSigningConfig) string { return c.CodeSigningConfigArn })
	sim.WriteJSON(w, http.StatusOK, map[string]any{"CodeSigningConfigs": stored})
}

// ---------------------------------------------------------------------------
// Per-function code-signing config attachment
// ---------------------------------------------------------------------------

var (
	lambdaFnCSCMu sync.Mutex
	// function name -> CodeSigningConfigArn.
	lambdaFnCSC = map[string]string{}
)

func handleLambdaPutFunctionCodeSigningConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	var req struct {
		CodeSigningConfigArn string `json:"CodeSigningConfigArn"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.CodeSigningConfigArn == "" {
		sim.AWSError(w, "InvalidParameterValueException", "CodeSigningConfigArn is required", http.StatusBadRequest)
		return
	}
	if _, ok := lambdaCSCStore.Get(req.CodeSigningConfigArn); !ok {
		sim.AWSErrorf(w, "CodeSigningConfigNotFoundException", http.StatusNotFound,
			"The code signing configuration %s cannot be found.", req.CodeSigningConfigArn)
		return
	}
	lambdaFnCSCMu.Lock()
	lambdaFnCSC[name] = req.CodeSigningConfigArn
	lambdaFnCSCMu.Unlock()
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"CodeSigningConfigArn": req.CodeSigningConfigArn,
		"FunctionName":         name,
	})
}

func handleLambdaGetFunctionCodeSigningConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	lambdaFnCSCMu.Lock()
	arn, ok := lambdaFnCSC[name]
	lambdaFnCSCMu.Unlock()
	if !ok {
		// No CSC attached: real Lambda returns 404.
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s does not have a code signing configuration.", lambdaArn(name))
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"CodeSigningConfigArn": arn,
		"FunctionName":         name,
	})
}

func handleLambdaDeleteFunctionCodeSigningConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	lambdaFnCSCMu.Lock()
	delete(lambdaFnCSC, name)
	lambdaFnCSCMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Runtime-management config
// ---------------------------------------------------------------------------

type lambdaRuntimeMgmt struct {
	UpdateRuntimeOn   string
	RuntimeVersionArn string
}

var (
	lambdaRTMMu sync.Mutex
	// keyed by "<functionName>:<qualifier>".
	lambdaRTMs = map[string]lambdaRuntimeMgmt{}
)

func handleLambdaGetRuntimeManagementConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	qualifier := r.URL.Query().Get("Qualifier")
	lambdaRTMMu.Lock()
	rtm, ok := lambdaRTMs[name+":"+qualifier]
	lambdaRTMMu.Unlock()
	out := map[string]any{
		"FunctionArn": lambdaEICArn(name, qualifier),
	}
	if ok {
		out["UpdateRuntimeOn"] = rtm.UpdateRuntimeOn
		if rtm.RuntimeVersionArn != "" {
			out["RuntimeVersionArn"] = rtm.RuntimeVersionArn
		}
	} else {
		// Default for a function with no explicit config is Auto.
		out["UpdateRuntimeOn"] = "Auto"
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleLambdaPutRuntimeManagementConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	qualifier := r.URL.Query().Get("Qualifier")
	var req struct {
		UpdateRuntimeOn   string `json:"UpdateRuntimeOn"`
		RuntimeVersionArn string `json:"RuntimeVersionArn"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.UpdateRuntimeOn == "" {
		sim.AWSError(w, "InvalidParameterValueException", "UpdateRuntimeOn is required", http.StatusBadRequest)
		return
	}
	// When UpdateRuntimeOn is Manual, RuntimeVersionArn is required.
	if req.UpdateRuntimeOn == "Manual" && req.RuntimeVersionArn == "" {
		sim.AWSError(w, "InvalidParameterValueException",
			"RuntimeVersionArn is required when UpdateRuntimeOn is Manual", http.StatusBadRequest)
		return
	}
	lambdaRTMMu.Lock()
	lambdaRTMs[name+":"+qualifier] = lambdaRuntimeMgmt{
		UpdateRuntimeOn:   req.UpdateRuntimeOn,
		RuntimeVersionArn: req.RuntimeVersionArn,
	}
	lambdaRTMMu.Unlock()
	out := map[string]any{
		"UpdateRuntimeOn": req.UpdateRuntimeOn,
		"FunctionArn":     lambdaEICArn(name, qualifier),
	}
	if req.RuntimeVersionArn != "" {
		out["RuntimeVersionArn"] = req.RuntimeVersionArn
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// Account settings
// ---------------------------------------------------------------------------

func handleLambdaGetAccountSettings(w http.ResponseWriter, _ *http.Request) {
	// Report real usage drawn from the function store; the limits match the
	// AWS-published defaults so the response shape is faithful.
	functions := lambdaFunctions.List()
	var totalCodeSize int64
	for _, fn := range functions {
		totalCodeSize += fn.CodeSize
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"AccountLimit": map[string]any{
			"TotalCodeSize":                  int64(80530636800),
			"CodeSizeUnzipped":               int64(262144000),
			"CodeSizeZipped":                 int64(52428800),
			"ConcurrentExecutions":           1000,
			"UnreservedConcurrentExecutions": 1000,
		},
		"AccountUsage": map[string]any{
			"TotalCodeSize": totalCodeSize,
			"FunctionCount": int64(len(functions)),
		},
	})
}

// ---------------------------------------------------------------------------
// Function recursion config
// ---------------------------------------------------------------------------

var (
	lambdaRecursionMu sync.Mutex
	// function name -> RecursiveLoop (Allow|Terminate).
	lambdaRecursion = map[string]string{}
)

func handleLambdaGetFunctionRecursionConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	lambdaRecursionMu.Lock()
	loop, ok := lambdaRecursion[name]
	lambdaRecursionMu.Unlock()
	if !ok {
		// New functions default to Terminate.
		loop = "Terminate"
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"RecursiveLoop": loop})
}

func handleLambdaPutFunctionRecursionConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The function %s could not be found.", lambdaArn(name))
		return
	}
	var req struct {
		RecursiveLoop string `json:"RecursiveLoop"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.RecursiveLoop != "Allow" && req.RecursiveLoop != "Terminate" {
		sim.AWSError(w, "InvalidParameterValueException",
			"RecursiveLoop must be one of Allow or Terminate", http.StatusBadRequest)
		return
	}
	lambdaRecursionMu.Lock()
	lambdaRecursion[name] = req.RecursiveLoop
	lambdaRecursionMu.Unlock()
	sim.WriteJSON(w, http.StatusOK, map[string]any{"RecursiveLoop": req.RecursiveLoop})
}

// ---------------------------------------------------------------------------
// Layer-version permissions
// ---------------------------------------------------------------------------

type lambdaLayerPermission struct {
	StatementId  string
	Action       string
	Principal    string
	Organization string
	Statement    string // serialized JSON policy statement
}

var (
	lambdaLayerPolicyMu sync.Mutex
	// keyed by "<layerName>:<version>"; an ordered list of statements.
	lambdaLayerPolicies = map[string][]lambdaLayerPermission{}
	// revision id per "<layerName>:<version>".
	lambdaLayerPolicyRev = map[string]string{}
)

func lambdaLayerPolicyKey(layer string, version int64) string {
	return fmt.Sprintf("%s:%d", layer, version)
}

// lambdaLayerPolicyDocument renders the IAM-style policy document the
// GetLayerVersionPolicy response returns under Policy.
func lambdaLayerPolicyDocument(layer string, version int64, stmts []lambdaLayerPermission) string {
	resource := lambdaLayerVersionArn(layer, version)
	var sb []string
	for _, s := range stmts {
		cond := ""
		if s.Organization != "" {
			cond = fmt.Sprintf(`,"Condition":{"StringEquals":{"aws:PrincipalOrgID":%q}}`, s.Organization)
		}
		sb = append(sb, fmt.Sprintf(
			`{"Sid":%q,"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":%q,"Resource":%q%s}`,
			s.StatementId, s.Principal, s.Action, resource, cond))
	}
	joined := ""
	for i, s := range sb {
		if i > 0 {
			joined += ","
		}
		joined += s
	}
	return fmt.Sprintf(`{"Version":"2012-10-17","Id":"default","Statement":[%s]}`, joined)
}

func lambdaLayerStatementJSON(layer string, version int64, p lambdaLayerPermission) string {
	resource := lambdaLayerVersionArn(layer, version)
	cond := ""
	if p.Organization != "" {
		cond = fmt.Sprintf(`,"Condition":{"StringEquals":{"aws:PrincipalOrgID":%q}}`, p.Organization)
	}
	return fmt.Sprintf(
		`{"Sid":%q,"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":%q,"Resource":%q%s}`,
		p.StatementId, p.Principal, p.Action, resource, cond)
}

func lambdaNewRevisionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func handleLambdaAddLayerVersionPermission(w http.ResponseWriter, r *http.Request) {
	layer := sim.PathParam(r, "layer")
	version, ok := lambdaParseLayerVersion(w, r)
	if !ok {
		return
	}
	if _, found := lambdaFindLayerVersion(layer, version); !found {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The layer version %s:%d could not be found.", layer, version)
		return
	}
	var req struct {
		StatementId    string `json:"StatementId"`
		Action         string `json:"Action"`
		Principal      string `json:"Principal"`
		OrganizationId string `json:"OrganizationId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.StatementId == "" || req.Action == "" || req.Principal == "" {
		sim.AWSError(w, "InvalidParameterValueException",
			"StatementId, Action, and Principal are required", http.StatusBadRequest)
		return
	}
	key := lambdaLayerPolicyKey(layer, version)
	lambdaLayerPolicyMu.Lock()
	for _, s := range lambdaLayerPolicies[key] {
		if s.StatementId == req.StatementId {
			lambdaLayerPolicyMu.Unlock()
			sim.AWSErrorf(w, "ResourceConflictException", http.StatusConflict,
				"The statement id (%s) provided already exists.", req.StatementId)
			return
		}
	}
	perm := lambdaLayerPermission{
		StatementId:  req.StatementId,
		Action:       req.Action,
		Principal:    req.Principal,
		Organization: req.OrganizationId,
	}
	perm.Statement = lambdaLayerStatementJSON(layer, version, perm)
	lambdaLayerPolicies[key] = append(lambdaLayerPolicies[key], perm)
	rev := lambdaNewRevisionID()
	lambdaLayerPolicyRev[key] = rev
	lambdaLayerPolicyMu.Unlock()
	sim.WriteJSON(w, http.StatusCreated, map[string]any{
		"Statement":  perm.Statement,
		"RevisionId": rev,
	})
}

func handleLambdaGetLayerVersionPolicy(w http.ResponseWriter, r *http.Request) {
	layer := sim.PathParam(r, "layer")
	version, ok := lambdaParseLayerVersion(w, r)
	if !ok {
		return
	}
	key := lambdaLayerPolicyKey(layer, version)
	lambdaLayerPolicyMu.Lock()
	stmts, ok := lambdaLayerPolicies[key]
	rev := lambdaLayerPolicyRev[key]
	lambdaLayerPolicyMu.Unlock()
	if !ok || len(stmts) == 0 {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"No policy is associated with the given resource.")
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Policy":     lambdaLayerPolicyDocument(layer, version, stmts),
		"RevisionId": rev,
	})
}

func handleLambdaRemoveLayerVersionPermission(w http.ResponseWriter, r *http.Request) {
	layer := sim.PathParam(r, "layer")
	version, ok := lambdaParseLayerVersion(w, r)
	if !ok {
		return
	}
	statementID := sim.PathParam(r, "statement")
	key := lambdaLayerPolicyKey(layer, version)
	lambdaLayerPolicyMu.Lock()
	stmts := lambdaLayerPolicies[key]
	idx := -1
	for i, s := range stmts {
		if s.StatementId == statementID {
			idx = i
			break
		}
	}
	if idx < 0 {
		lambdaLayerPolicyMu.Unlock()
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"No statement (%s) is associated with the given resource.", statementID)
		return
	}
	lambdaLayerPolicies[key] = append(stmts[:idx], stmts[idx+1:]...)
	lambdaLayerPolicyRev[key] = lambdaNewRevisionID()
	lambdaLayerPolicyMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// lambdaParseLayerVersion reads the {version} path label as an int64,
// emitting a 400 when it isn't numeric.
func lambdaParseLayerVersion(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := sim.PathParam(r, "version")
	var v int64
	if _, err := fmt.Sscanf(raw, "%d", &v); err != nil {
		sim.AWSError(w, "InvalidParameterValueException",
			fmt.Sprintf("Invalid version number: %q", raw), http.StatusBadRequest)
		return 0, false
	}
	return v, true
}
