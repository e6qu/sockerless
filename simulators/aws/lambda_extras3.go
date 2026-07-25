package main

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// This file completes the Lambda restJson1 control-plane slice: the
// configuration read/update pair (GetFunctionConfiguration / UpdateFunctionCode),
// the 2025-11-30 function-scaling and capacity-provider surfaces, the
// 2025-12-01 durable-execution lifecycle (start via durable Invoke, checkpoint,
// callbacks, history/state read-back), and the two legacy/streaming invoke
// entry points (InvokeAsync, InvokeWithResponseStream). All state lives in
// in-process stores whose lifetime matches the running sim, the same as the
// sibling lambda_*.go files. No fabricated business data: a durable execution
// with no real workload reports honest, empty-shaped history/state.

func registerLambdaExtras3(srv *sim.Server) {
	mux := srv
	lambdaResource := cloudTrailRESTResource("AWS::Lambda::Function", "name", "arn")
	lambdaCapacityProviders = sim.MakeStore[LambdaCapacityProvider](srv.DB(), "lambda_capacity_providers")

	// ListLayers shares GET /2018-10-31/layers with GetLayerVersionByArn; the
	// shared handler (lambda_extras.go) composes the op name per-request via
	// cloudTrailRecordedRESTDynamic, so register the static op name here so it
	// lands in the conformance REST registry the same way GetLayerVersionByArn
	// does in lambda.go.
	restRegisterOp("lambda.amazonaws.com", "ListLayers")

	// Function configuration read + code update.
	mux.HandleFunc("GET /2015-03-31/functions/{name}/configuration", cloudTrailRecordedREST("GetFunctionConfiguration", "lambda.amazonaws.com", lambdaResource, lambdaEnforced("GetFunctionConfiguration", lambdaFunctionResourceARN, handleLambdaGetFunctionConfiguration)))
	mux.HandleFunc("PUT /2015-03-31/functions/{name}/code", cloudTrailRecordedREST("UpdateFunctionCode", "lambda.amazonaws.com", lambdaResource, lambdaEnforced("UpdateFunctionCode", lambdaFunctionResourceARN, handleLambdaUpdateFunctionCode)))

	// Per-function scaling config (2025-11-30).
	mux.HandleFunc("GET /2025-11-30/functions/{name}/function-scaling-config", cloudTrailRecordedREST("GetFunctionScalingConfig", "lambda.amazonaws.com", lambdaResource, lambdaEnforced("GetFunctionScalingConfig", lambdaFunctionResourceARN, handleLambdaGetFunctionScalingConfig)))
	mux.HandleFunc("PUT /2025-11-30/functions/{name}/function-scaling-config", cloudTrailRecordedREST("PutFunctionScalingConfig", "lambda.amazonaws.com", lambdaResource, lambdaEnforced("PutFunctionScalingConfig", lambdaFunctionResourceARN, handleLambdaPutFunctionScalingConfig)))

	// Functions attached to a code-signing config.
	// A CodeSigningConfig ARN has no slashes, so a single {arn} label captures
	// it; a trailing {arn...} wildcard cannot precede the /functions suffix.
	mux.HandleFunc("GET /2020-04-22/code-signing-configs/{arn}/functions", cloudTrailRecordedREST("ListFunctionsByCodeSigningConfig", "lambda.amazonaws.com", nil, lambdaEnforced("ListFunctionsByCodeSigningConfig", nil, handleLambdaListFunctionsByCodeSigningConfig)))

	// Capacity providers (2025-11-30).
	mux.HandleFunc("POST /2025-11-30/capacity-providers", cloudTrailRecordedREST("CreateCapacityProvider", "lambda.amazonaws.com", nil, lambdaEnforced("CreateCapacityProvider", nil, handleLambdaCreateCapacityProvider)))
	mux.HandleFunc("GET /2025-11-30/capacity-providers", cloudTrailRecordedREST("ListCapacityProviders", "lambda.amazonaws.com", nil, lambdaEnforced("ListCapacityProviders", nil, handleLambdaListCapacityProviders)))
	mux.HandleFunc("GET /2025-11-30/capacity-providers/{cpname}", cloudTrailRecordedREST("GetCapacityProvider", "lambda.amazonaws.com", nil, lambdaEnforced("GetCapacityProvider", nil, handleLambdaGetCapacityProvider)))
	mux.HandleFunc("PUT /2025-11-30/capacity-providers/{cpname}", cloudTrailRecordedREST("UpdateCapacityProvider", "lambda.amazonaws.com", nil, lambdaEnforced("UpdateCapacityProvider", nil, handleLambdaUpdateCapacityProvider)))
	mux.HandleFunc("DELETE /2025-11-30/capacity-providers/{cpname}", cloudTrailRecordedREST("DeleteCapacityProvider", "lambda.amazonaws.com", nil, lambdaEnforced("DeleteCapacityProvider", nil, handleLambdaDeleteCapacityProvider)))
	mux.HandleFunc("GET /2025-11-30/capacity-providers/{cpname}/function-versions", cloudTrailRecordedREST("ListFunctionVersionsByCapacityProvider", "lambda.amazonaws.com", nil, lambdaEnforced("ListFunctionVersionsByCapacityProvider", nil, handleLambdaListFunctionVersionsByCapacityProvider)))

	// Legacy + streaming invoke entry points.
	mux.HandleFunc("POST /2014-11-13/functions/{name}/invoke-async", cloudTrailRecordedREST("InvokeAsync", "lambda.amazonaws.com", lambdaResource, lambdaEnforced("InvokeAsync", lambdaFunctionResourceARN, handleLambdaInvokeAsync)))
	mux.HandleFunc("POST /2021-11-15/functions/{name}/response-streaming-invocations", cloudTrailRecordedREST("InvokeWithResponseStream", "lambda.amazonaws.com", lambdaResource, lambdaEnforced("InvokeWithResponseStream", lambdaFunctionResourceARN, handleLambdaInvokeWithResponseStream)))

	// Durable executions (2025-12-01). The DurableExecutionArn is a non-greedy
	// restJson1 path label, so the SDK percent-encodes its embedded slashes
	// (%2F); a single {arn} segment captures the whole arn and ServeMux still
	// resolves the trailing /checkpoint|/history|/state|/stop literal suffix.
	mux.HandleFunc("GET /2025-12-01/functions/{name}/durable-executions", cloudTrailRecordedREST("ListDurableExecutionsByFunction", "lambda.amazonaws.com", lambdaResource, lambdaEnforced("ListDurableExecutionsByFunction", lambdaFunctionResourceARN, handleLambdaListDurableExecutionsByFunction)))
	mux.HandleFunc("GET /2025-12-01/durable-executions/{arn}", cloudTrailRecordedREST("GetDurableExecution", "lambda.amazonaws.com", nil, lambdaEnforced("GetDurableExecution", nil, handleLambdaGetDurableExecution)))
	mux.HandleFunc("POST /2025-12-01/durable-executions/{arn}/checkpoint", cloudTrailRecordedREST("CheckpointDurableExecution", "lambda.amazonaws.com", nil, lambdaEnforced("CheckpointDurableExecution", nil, handleLambdaCheckpointDurableExecution)))
	mux.HandleFunc("GET /2025-12-01/durable-executions/{arn}/history", cloudTrailRecordedREST("GetDurableExecutionHistory", "lambda.amazonaws.com", nil, lambdaEnforced("GetDurableExecutionHistory", nil, handleLambdaGetDurableExecutionHistory)))
	mux.HandleFunc("GET /2025-12-01/durable-executions/{arn}/state", cloudTrailRecordedREST("GetDurableExecutionState", "lambda.amazonaws.com", nil, lambdaEnforced("GetDurableExecutionState", nil, handleLambdaGetDurableExecutionState)))
	mux.HandleFunc("POST /2025-12-01/durable-executions/{arn}/stop", cloudTrailRecordedREST("StopDurableExecution", "lambda.amazonaws.com", nil, lambdaEnforced("StopDurableExecution", nil, handleLambdaStopDurableExecution)))
	mux.HandleFunc("POST /2025-12-01/durable-execution-callbacks/{cbid}/succeed", cloudTrailRecordedREST("SendDurableExecutionCallbackSuccess", "lambda.amazonaws.com", nil, lambdaEnforced("SendDurableExecutionCallbackSuccess", nil, handleLambdaSendDurableCallbackSuccess)))
	mux.HandleFunc("POST /2025-12-01/durable-execution-callbacks/{cbid}/fail", cloudTrailRecordedREST("SendDurableExecutionCallbackFailure", "lambda.amazonaws.com", nil, lambdaEnforced("SendDurableExecutionCallbackFailure", nil, handleLambdaSendDurableCallbackFailure)))
	mux.HandleFunc("POST /2025-12-01/durable-execution-callbacks/{cbid}/heartbeat", cloudTrailRecordedREST("SendDurableExecutionCallbackHeartbeat", "lambda.amazonaws.com", nil, lambdaEnforced("SendDurableExecutionCallbackHeartbeat", nil, handleLambdaSendDurableCallbackHeartbeat)))
}

// ---------------------------------------------------------------------------
// GetFunctionConfiguration + UpdateFunctionCode
// ---------------------------------------------------------------------------

func handleLambdaGetFunctionConfiguration(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	fn, ok := lambdaFunctions.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}
	// GetFunctionConfiguration returns the FunctionConfiguration shape directly
	// (the same body GetFunction nests under "Configuration").
	sim.WriteJSON(w, http.StatusOK, lambdaConfiguration(fn))
}

func handleLambdaUpdateFunctionCode(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}
	var req struct {
		ZipFile         string   `json:"ZipFile"`
		S3Bucket        string   `json:"S3Bucket"`
		S3Key           string   `json:"S3Key"`
		S3ObjectVersion string   `json:"S3ObjectVersion"`
		ImageUri        string   `json:"ImageUri"`
		Architectures   []string `json:"Architectures"`
		SourceKMSKeyArn string   `json:"SourceKMSKeyArn"`
		DryRun          bool     `json:"DryRun"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	newCode := &LambdaFunctionCode{
		S3Bucket:        req.S3Bucket,
		S3Key:           req.S3Key,
		S3ObjectVersion: req.S3ObjectVersion,
		ImageUri:        req.ImageUri,
		ZipFile:         req.ZipFile,
		SourceKMSKeyArn: req.SourceKMSKeyArn,
	}
	// DryRun validates without persisting; return the current config unchanged.
	if req.DryRun {
		fn, _ := lambdaFunctions.Get(name)
		sim.WriteJSON(w, http.StatusOK, lambdaConfiguration(fn))
		return
	}
	lambdaFunctions.Update(name, func(fn *LambdaFunction) {
		fn.Code = newCode
		if req.ImageUri != "" {
			fn.PackageType = "Image"
			fn.CodeSha256 = ""
		} else {
			fn.CodeSha256 = lambdaCodeSha256(newCode)
		}
		// A code change re-stamps size/revision/last-modified the way real
		// Lambda does after a successful deployment-package swap.
		fn.CodeSize = lambdaCodeSize(newCode)
		if len(req.Architectures) > 0 {
			fn.Architectures = req.Architectures
		}
		fn.LastModified = time.Now().UTC().Format(time.RFC3339)
		fn.LastUpdateStatus = "Successful"
		fn.RevisionId = generateUUID()
	})
	fn, _ := lambdaFunctions.Get(name)
	sim.WriteJSON(w, http.StatusOK, lambdaConfiguration(fn))
}

// lambdaCodeSize derives a deterministic, non-zero package size from the
// supplied code material. Real Lambda reports the unzipped size of the stored
// deployment package; the sim derives it from the material length so a code
// swap visibly changes CodeSize.
func lambdaCodeSize(code *LambdaFunctionCode) int64 {
	if code == nil {
		return 0
	}
	if code.ZipFile != "" {
		return int64(len(code.ZipFile))
	}
	// S3 / image package: keep the same baseline CreateFunction uses so the
	// shape is stable for clients that don't supply inline zip material.
	return 1024
}

// ---------------------------------------------------------------------------
// Per-function scaling config
// ---------------------------------------------------------------------------

type lambdaFunctionScalingConfig struct {
	MinExecutionEnvironments *int32 `json:"MinExecutionEnvironments,omitempty"`
	MaxExecutionEnvironments *int32 `json:"MaxExecutionEnvironments,omitempty"`
}

var (
	lambdaScalingMu sync.Mutex
	// keyed by "<functionName>:<qualifier>" ($LATEST when no qualifier).
	lambdaScalingCfgs = map[string]lambdaFunctionScalingConfig{}
)

func handleLambdaGetFunctionScalingConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}
	qualifier := r.URL.Query().Get("Qualifier")
	lambdaScalingMu.Lock()
	cfg, ok := lambdaScalingCfgs[lambdaEICKey(name, qualifier)]
	lambdaScalingMu.Unlock()
	out := map[string]any{"FunctionArn": lambdaEICArn(name, qualifier)}
	if ok {
		// Applied == Requested once the allocation settles (synchronous here).
		out["RequestedFunctionScalingConfig"] = cfg
		out["AppliedFunctionScalingConfig"] = cfg
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleLambdaPutFunctionScalingConfig(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}
	qualifier := r.URL.Query().Get("Qualifier")
	var req struct {
		FunctionScalingConfig *lambdaFunctionScalingConfig `json:"FunctionScalingConfig"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.FunctionScalingConfig == nil {
		sim.AWSError(w, "InvalidParameterValueException", "FunctionScalingConfig is required", http.StatusBadRequest)
		return
	}
	lambdaScalingMu.Lock()
	lambdaScalingCfgs[lambdaEICKey(name, qualifier)] = *req.FunctionScalingConfig
	lambdaScalingMu.Unlock()
	// PutFunctionScalingConfig returns 202; the config is being applied.
	sim.WriteJSON(w, http.StatusAccepted, map[string]any{"FunctionState": "Active"})
}

// ---------------------------------------------------------------------------
// ListFunctionsByCodeSigningConfig
// ---------------------------------------------------------------------------

func handleLambdaListFunctionsByCodeSigningConfig(w http.ResponseWriter, r *http.Request) {
	arn := r.PathValue("arn")
	if _, ok := lambdaCSCStore.Get(arn); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"The code signing configuration cannot be found.")
		return
	}
	lambdaFnCSCMu.Lock()
	names := make([]string, 0)
	for fn, cscArn := range lambdaFnCSC {
		if cscArn == arn {
			names = append(names, lambdaArn(fn))
		}
	}
	lambdaFnCSCMu.Unlock()
	sort.Strings(names)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"FunctionArns": names})
}

// ---------------------------------------------------------------------------
// Capacity providers
// ---------------------------------------------------------------------------

// LambdaCapacityProvider mirrors the CapacityProvider shape. The members are
// echoed back exactly as supplied on create; the sim settles a created
// provider straight to Active (no asynchronous Pending window).
type LambdaCapacityProvider struct {
	name                          string
	CapacityProviderArn           string         `json:"CapacityProviderArn"`
	State                         string         `json:"State"`
	VpcConfig                     map[string]any `json:"VpcConfig,omitempty"`
	PermissionsConfig             map[string]any `json:"PermissionsConfig,omitempty"`
	InstanceRequirements          map[string]any `json:"InstanceRequirements,omitempty"`
	CapacityProviderScalingConfig map[string]any `json:"CapacityProviderScalingConfig,omitempty"`
	KmsKeyArn                     string         `json:"KmsKeyArn,omitempty"`
	LastModified                  string         `json:"LastModified"`
	PropagateTags                 map[string]any `json:"PropagateTags,omitempty"`
}

var lambdaCapacityProviders sim.Store[LambdaCapacityProvider]

func lambdaCapacityProviderArn(name string) string {
	return fmt.Sprintf("arn:aws:lambda:%s:%s:capacity-provider:%s", awsRegion(), awsAccountID(), name)
}

func lambdaCapacityProviderBody(cp LambdaCapacityProvider) map[string]any {
	out := map[string]any{
		"CapacityProviderArn": cp.CapacityProviderArn,
		"State":               cp.State,
		"LastModified":        cp.LastModified,
	}
	if cp.VpcConfig != nil {
		out["VpcConfig"] = cp.VpcConfig
	}
	if cp.PermissionsConfig != nil {
		out["PermissionsConfig"] = cp.PermissionsConfig
	}
	if cp.InstanceRequirements != nil {
		out["InstanceRequirements"] = cp.InstanceRequirements
	}
	if cp.CapacityProviderScalingConfig != nil {
		out["CapacityProviderScalingConfig"] = cp.CapacityProviderScalingConfig
	}
	if cp.KmsKeyArn != "" {
		out["KmsKeyArn"] = cp.KmsKeyArn
	}
	if cp.PropagateTags != nil {
		out["PropagateTags"] = cp.PropagateTags
	}
	return out
}

func handleLambdaCreateCapacityProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CapacityProviderName          string         `json:"CapacityProviderName"`
		VpcConfig                     map[string]any `json:"VpcConfig"`
		PermissionsConfig             map[string]any `json:"PermissionsConfig"`
		InstanceRequirements          map[string]any `json:"InstanceRequirements"`
		CapacityProviderScalingConfig map[string]any `json:"CapacityProviderScalingConfig"`
		KmsKeyArn                     string         `json:"KmsKeyArn"`
		PropagateTags                 map[string]any `json:"PropagateTags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.CapacityProviderName == "" {
		sim.AWSError(w, "InvalidParameterValueException", "CapacityProviderName is required", http.StatusBadRequest)
		return
	}
	if _, exists := lambdaCapacityProviders.Get(req.CapacityProviderName); exists {
		sim.AWSErrorf(w, "ResourceConflictException", http.StatusConflict,
			"Capacity provider already exists: %s", req.CapacityProviderName)
		return
	}
	cp := LambdaCapacityProvider{
		name:                          req.CapacityProviderName,
		CapacityProviderArn:           lambdaCapacityProviderArn(req.CapacityProviderName),
		State:                         "Active",
		VpcConfig:                     req.VpcConfig,
		PermissionsConfig:             req.PermissionsConfig,
		InstanceRequirements:          req.InstanceRequirements,
		CapacityProviderScalingConfig: req.CapacityProviderScalingConfig,
		KmsKeyArn:                     req.KmsKeyArn,
		LastModified:                  time.Now().UTC().Format(time.RFC3339),
		PropagateTags:                 req.PropagateTags,
	}
	lambdaCapacityProviders.Put(req.CapacityProviderName, cp)
	// CreateCapacityProvider returns 202: the provider is being provisioned.
	sim.WriteJSON(w, http.StatusAccepted, map[string]any{"CapacityProvider": lambdaCapacityProviderBody(cp)})
}

func handleLambdaGetCapacityProvider(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "cpname")
	cp, ok := lambdaCapacityProviders.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Capacity provider not found: %s", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"CapacityProvider": lambdaCapacityProviderBody(cp)})
}

func handleLambdaListCapacityProviders(w http.ResponseWriter, r *http.Request) {
	stored := lambdaCapacityProviders.List()
	stateFilter := r.URL.Query().Get("State")
	sortBy(stored, func(c LambdaCapacityProvider) string { return c.CapacityProviderArn })
	providers := make([]map[string]any, 0, len(stored))
	for _, cp := range stored {
		if stateFilter != "" && cp.State != stateFilter {
			continue
		}
		providers = append(providers, lambdaCapacityProviderBody(cp))
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"CapacityProviders": providers})
}

func handleLambdaUpdateCapacityProvider(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "cpname")
	if _, ok := lambdaCapacityProviders.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Capacity provider not found: %s", name)
		return
	}
	var req struct {
		CapacityProviderScalingConfig map[string]any `json:"CapacityProviderScalingConfig"`
		PropagateTags                 map[string]any `json:"PropagateTags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	lambdaCapacityProviders.Update(name, func(cp *LambdaCapacityProvider) {
		if req.CapacityProviderScalingConfig != nil {
			cp.CapacityProviderScalingConfig = req.CapacityProviderScalingConfig
		}
		if req.PropagateTags != nil {
			cp.PropagateTags = req.PropagateTags
		}
		cp.LastModified = time.Now().UTC().Format(time.RFC3339)
	})
	cp, _ := lambdaCapacityProviders.Get(name)
	// UpdateCapacityProvider returns 202.
	sim.WriteJSON(w, http.StatusAccepted, map[string]any{"CapacityProvider": lambdaCapacityProviderBody(cp)})
}

func handleLambdaDeleteCapacityProvider(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "cpname")
	if !lambdaCapacityProviders.Delete(name) {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Capacity provider not found: %s", name)
		return
	}
	// DeleteCapacityProvider returns 202 (deletion is asynchronous).
	w.WriteHeader(http.StatusAccepted)
}

func handleLambdaListFunctionVersionsByCapacityProvider(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "cpname")
	cp, ok := lambdaCapacityProviders.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Capacity provider not found: %s", name)
		return
	}
	// No function versions are pinned to a capacity provider in the sim; report
	// the honest empty list under the provider's ARN rather than fabricating
	// associations.
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"CapacityProviderArn": cp.CapacityProviderArn,
		"FunctionVersions":    []any{},
	})
}

// ---------------------------------------------------------------------------
// InvokeAsync (deprecated) + InvokeWithResponseStream
// ---------------------------------------------------------------------------

func handleLambdaInvokeAsync(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	fn, ok := lambdaFunctions.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		sim.AWSError(w, "InvalidRequestContentException",
			"failed to read invoke args: "+err.Error(), http.StatusBadRequest)
		return
	}
	// InvokeAsync buffers the invocation and runs it for real in the
	// background (which produces real logs), the same async path Invoke
	// with InvocationType=Event takes.
	go func() { _, _, _ = invokeLambdaViaRuntimeAPI(fn, payload) }()
	// The deprecated InvokeAsync response binds Status to the HTTP code (202)
	// and carries no body.
	w.WriteHeader(http.StatusAccepted)
}

// awsEventStreamMessage encodes a single AWS event-stream (vnd.amazon.eventstream)
// frame: total-len, headers-len, prelude-CRC, headers, payload, message-CRC.
// Each header is name-len(1) + name + value-type(1) + value-len(2) + value.
func awsEventStreamMessage(headers map[string]string, payload []byte) []byte {
	var hb []byte
	for name, val := range headers {
		hb = append(hb, byte(len(name)))
		hb = append(hb, name...)
		hb = append(hb, 7) // value type 7 = string
		var vl [2]byte
		binary.BigEndian.PutUint16(vl[:], uint16(len(val)))
		hb = append(hb, vl[:]...)
		hb = append(hb, val...)
	}
	totalLen := uint32(16 + len(hb) + len(payload))
	msg := make([]byte, 0, totalLen)
	var prelude [8]byte
	binary.BigEndian.PutUint32(prelude[0:4], totalLen)
	binary.BigEndian.PutUint32(prelude[4:8], uint32(len(hb)))
	msg = append(msg, prelude[:]...)
	var preludeCRC [4]byte
	binary.BigEndian.PutUint32(preludeCRC[:], crc32.ChecksumIEEE(prelude[:]))
	msg = append(msg, preludeCRC[:]...)
	msg = append(msg, hb...)
	msg = append(msg, payload...)
	var msgCRC [4]byte
	binary.BigEndian.PutUint32(msgCRC[:], crc32.ChecksumIEEE(msg))
	msg = append(msg, msgCRC[:]...)
	return msg
}

func handleLambdaInvokeWithResponseStream(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	fn, ok := lambdaFunctions.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}
	invType := r.Header.Get("X-Amz-Invocation-Type")
	if strings.EqualFold(invType, "DryRun") {
		w.Header().Set("X-Amz-Executed-Version", "$LATEST")
		w.WriteHeader(http.StatusOK)
		return
	}
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		sim.AWSError(w, "InvalidRequestContentException",
			"failed to read payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Run the function for real and stream its response back over the AWS
	// event-stream framing: one PayloadChunk carrying the handler's bytes,
	// then a terminal InvokeComplete event. This is the same wire framing
	// real Lambda uses for response streaming, so aws-sdk-go-v2's
	// eventstream decoder reassembles it natively.
	responseBody, unhandled, _ := invokeLambdaViaRuntimeAPI(fn, payload)
	w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
	w.Header().Set("X-Amz-Executed-Version", "$LATEST")
	w.WriteHeader(http.StatusOK)
	if len(responseBody) > 0 {
		_, _ = w.Write(awsEventStreamMessage(map[string]string{
			":event-type":   "PayloadChunk",
			":message-type": "event",
			":content-type": "application/octet-stream",
		}, responseBody))
	}
	complete := []byte("{}")
	if unhandled {
		complete = []byte(`{"ErrorCode":"Unhandled","ErrorDetails":"The function returned an error."}`)
	}
	_, _ = w.Write(awsEventStreamMessage(map[string]string{
		":event-type":   "InvokeComplete",
		":message-type": "event",
		":content-type": "application/json",
	}, complete))
}

// ---------------------------------------------------------------------------
// Durable executions
// ---------------------------------------------------------------------------

// lambdaDurableEvent is one history entry. Only fields the sim has honest data
// for are populated; detail unions are emitted with their real payloads.
type lambdaDurableEvent struct {
	EventType      string         `json:"EventType"`
	EventId        int64          `json:"EventId"`
	EventTimestamp float64        `json:"EventTimestamp"`
	StartedDetails map[string]any `json:"ExecutionStartedDetails,omitempty"`
	SucceededDet   map[string]any `json:"ExecutionSucceededDetails,omitempty"`
	StoppedDet     map[string]any `json:"ExecutionStoppedDetails,omitempty"`
}

// lambdaDurableOperation is one entry in the execution-state operation list.
type lambdaDurableOperation struct {
	Id             string  `json:"Id"`
	Name           string  `json:"Name,omitempty"`
	Type           string  `json:"Type,omitempty"`
	StartTimestamp float64 `json:"StartTimestamp,omitempty"`
	EndTimestamp   float64 `json:"EndTimestamp,omitempty"`
	Status         string  `json:"Status,omitempty"`
}

type lambdaDurableExecution struct {
	Arn          string
	Name         string
	FunctionArn  string
	Version      string
	InputPayload string
	Result       string
	ErrorObj     map[string]any
	Status       string // RUNNING|SUCCEEDED|FAILED|STOPPED|TIMED_OUT
	StartTS      float64
	EndTS        float64
	Events       []lambdaDurableEvent
	Operations   []lambdaDurableOperation
}

var (
	lambdaDurableMu sync.Mutex
	// keyed by DurableExecutionArn.
	lambdaDurableExecs = map[string]*lambdaDurableExecution{}
	// CallbackId -> DurableExecutionArn, registered by a checkpoint that
	// records a CALLBACK operation; the callback ops advance that execution.
	lambdaDurableCallbacks = map[string]string{}
	lambdaDurableEventSeq  int64
)

// lambdaParseDurableArn validates the DurableExecutionArn shape and extracts
// the embedded function name and version. Real arns look like
// arn:aws:lambda:<region>:<acct>:function:<name>:<version>/durable-execution/<name>/<id>.
func lambdaParseDurableArn(arn string) (fnName, version string, ok bool) {
	slash := strings.Index(arn, "/durable-execution/")
	if slash < 0 {
		return "", "", false
	}
	head := arn[:slash] // arn:...:function:<name>:<version>
	marker := ":function:"
	fi := strings.Index(head, marker)
	if fi < 0 {
		return "", "", false
	}
	rest := head[fi+len(marker):] // <name>:<version>
	colon := strings.LastIndex(rest, ":")
	if colon < 0 {
		return "", "", false
	}
	return rest[:colon], rest[colon+1:], true
}

// lambdaDurableExecKey resolves the {arn...} label, which the mux delivers with
// its colons intact, into the canonical DurableExecutionArn.
func lambdaDurableArnFromLabel(r *http.Request) string {
	return r.PathValue("arn")
}

func handleLambdaGetDurableExecution(w http.ResponseWriter, r *http.Request) {
	arn := lambdaDurableArnFromLabel(r)
	lambdaDurableMu.Lock()
	de, ok := lambdaDurableExecs[arn]
	lambdaDurableMu.Unlock()
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Durable execution not found: %s", arn)
		return
	}
	out := map[string]any{
		"DurableExecutionArn":  de.Arn,
		"DurableExecutionName": de.Name,
		"FunctionArn":          de.FunctionArn,
		"Status":               de.Status,
		"StartTimestamp":       de.StartTS,
	}
	if de.Version != "" {
		out["Version"] = de.Version
	}
	if de.InputPayload != "" {
		out["InputPayload"] = de.InputPayload
	}
	if de.Result != "" {
		out["Result"] = de.Result
	}
	if de.ErrorObj != nil {
		out["Error"] = de.ErrorObj
	}
	if de.EndTS != 0 {
		out["EndTimestamp"] = de.EndTS
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleLambdaCheckpointDurableExecution(w http.ResponseWriter, r *http.Request) {
	arn := lambdaDurableArnFromLabel(r)
	fnName, version, ok := lambdaParseDurableArn(arn)
	if !ok {
		sim.AWSError(w, "InvalidParameterValueException",
			"Invalid DurableExecutionArn: "+arn, http.StatusBadRequest)
		return
	}
	if _, exists := lambdaFunctions.Get(fnName); !exists {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(fnName))
		return
	}
	var req struct {
		CheckpointToken string `json:"CheckpointToken"`
		Updates         []struct {
			Id              string         `json:"Id"`
			ParentId        string         `json:"ParentId"`
			Name            string         `json:"Name"`
			Type            string         `json:"Type"`
			Action          string         `json:"Action"`
			Payload         string         `json:"Payload"`
			Error           map[string]any `json:"Error"`
			CallbackOptions map[string]any `json:"CallbackOptions"`
		} `json:"Updates"`
		ClientToken string `json:"ClientToken"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	now := lambdaNowEpoch()
	lambdaDurableMu.Lock()
	de, exists := lambdaDurableExecs[arn]
	if !exists {
		// First checkpoint materializes the execution. Its identity (arn, name,
		// version) comes entirely from the caller; the runtime that is
		// checkpointing supplied them. No identity is fabricated.
		name := arn
		if i := strings.LastIndex(arn, "/"); i >= 0 {
			name = arn[i+1:]
		}
		de = &lambdaDurableExecution{
			Arn:         arn,
			Name:        name,
			FunctionArn: lambdaArn(fnName),
			Version:     version,
			Status:      "RUNNING",
			StartTS:     now,
		}
		lambdaDurableEventSeq++
		de.Events = append(de.Events, lambdaDurableEvent{
			EventType:      "ExecutionStarted",
			EventId:        lambdaDurableEventSeq,
			EventTimestamp: now,
			StartedDetails: map[string]any{},
		})
		lambdaDurableExecs[arn] = de
	}
	// Apply each operation update onto the real state + history.
	for _, u := range req.Updates {
		op := lambdaDurableOperation{
			Id:             u.Id,
			Name:           u.Name,
			Type:           u.Type,
			StartTimestamp: now,
			Status:         "STARTED",
		}
		if u.Action != "" {
			switch strings.ToUpper(u.Action) {
			case "SUCCEED", "SUCCEEDED", "COMPLETE":
				op.Status = "SUCCEEDED"
				op.EndTimestamp = now
			case "FAIL", "FAILED":
				op.Status = "FAILED"
				op.EndTimestamp = now
			}
		}
		// A CALLBACK-typed operation registers a callback the SendCallback ops
		// can advance; its id is the operation id.
		if strings.EqualFold(u.Type, "CALLBACK") && u.Id != "" {
			lambdaDurableCallbacks[u.Id] = arn
		}
		de.Operations = append(de.Operations, op)
	}
	token := req.CheckpointToken
	if token == "" {
		token = lambdaNewRevisionID()
	}
	ops := append([]lambdaDurableOperation(nil), de.Operations...)
	lambdaDurableMu.Unlock()
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"CheckpointToken": token,
		"NewExecutionState": map[string]any{
			"Operations": ops,
		},
	})
}

func handleLambdaGetDurableExecutionHistory(w http.ResponseWriter, r *http.Request) {
	arn := lambdaDurableArnFromLabel(r)
	lambdaDurableMu.Lock()
	de, ok := lambdaDurableExecs[arn]
	var events []lambdaDurableEvent
	if ok {
		events = append([]lambdaDurableEvent(nil), de.Events...)
	}
	lambdaDurableMu.Unlock()
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Durable execution not found: %s", arn)
		return
	}
	if r.URL.Query().Get("ReverseOrder") == "true" {
		for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
			events[i], events[j] = events[j], events[i]
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Events": events})
}

func handleLambdaGetDurableExecutionState(w http.ResponseWriter, r *http.Request) {
	arn := lambdaDurableArnFromLabel(r)
	lambdaDurableMu.Lock()
	de, ok := lambdaDurableExecs[arn]
	var ops []lambdaDurableOperation
	if ok {
		ops = append([]lambdaDurableOperation(nil), de.Operations...)
	}
	lambdaDurableMu.Unlock()
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Durable execution not found: %s", arn)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Operations": ops})
}

func handleLambdaListDurableExecutionsByFunction(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if _, ok := lambdaFunctions.Get(name); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}
	fnArn := lambdaArn(name)
	statusFilter := r.URL.Query()["Statuses"]
	lambdaDurableMu.Lock()
	executions := make([]map[string]any, 0)
	for _, de := range lambdaDurableExecs {
		if de.FunctionArn != fnArn {
			continue
		}
		if len(statusFilter) > 0 {
			match := false
			for _, s := range statusFilter {
				if s == de.Status {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		item := map[string]any{
			"DurableExecutionArn":  de.Arn,
			"DurableExecutionName": de.Name,
			"FunctionArn":          de.FunctionArn,
			"Status":               de.Status,
			"StartTimestamp":       de.StartTS,
		}
		if de.EndTS != 0 {
			item["EndTimestamp"] = de.EndTS
		}
		executions = append(executions, item)
	}
	lambdaDurableMu.Unlock()
	sort.Slice(executions, func(i, j int) bool {
		return fmt.Sprint(executions[i]["DurableExecutionArn"]) < fmt.Sprint(executions[j]["DurableExecutionArn"])
	})
	sim.WriteJSON(w, http.StatusOK, map[string]any{"DurableExecutions": executions})
}

func handleLambdaStopDurableExecution(w http.ResponseWriter, r *http.Request) {
	arn := lambdaDurableArnFromLabel(r)
	var req struct {
		Error map[string]any `json:"Error"`
	}
	_ = sim.ReadJSON(r, &req)
	now := lambdaNowEpoch()
	lambdaDurableMu.Lock()
	de, ok := lambdaDurableExecs[arn]
	if ok {
		de.Status = "STOPPED"
		de.EndTS = now
		lambdaDurableEventSeq++
		de.Events = append(de.Events, lambdaDurableEvent{
			EventType:      "ExecutionStopped",
			EventId:        lambdaDurableEventSeq,
			EventTimestamp: now,
			StoppedDet:     map[string]any{},
		})
	}
	lambdaDurableMu.Unlock()
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Durable execution not found: %s", arn)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"StopTimestamp": now})
}

// lambdaAdvanceCallback resolves a CallbackId to its execution and marks the
// matching CALLBACK operation in the desired terminal state. Returns false when
// the callback id is unknown.
func lambdaAdvanceCallback(callbackID, status string) bool {
	lambdaDurableMu.Lock()
	defer lambdaDurableMu.Unlock()
	arn, ok := lambdaDurableCallbacks[callbackID]
	if !ok {
		return false
	}
	de, ok := lambdaDurableExecs[arn]
	if !ok {
		return false
	}
	now := lambdaNowEpoch()
	for i := range de.Operations {
		if de.Operations[i].Id == callbackID {
			if status != "" {
				de.Operations[i].Status = status
				de.Operations[i].EndTimestamp = now
			}
		}
	}
	return true
}

func handleLambdaSendDurableCallbackSuccess(w http.ResponseWriter, r *http.Request) {
	id := sim.PathParam(r, "cbid")
	if !lambdaAdvanceCallback(id, "SUCCEEDED") {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Callback not found: %s", id)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleLambdaSendDurableCallbackFailure(w http.ResponseWriter, r *http.Request) {
	id := sim.PathParam(r, "cbid")
	if !lambdaAdvanceCallback(id, "FAILED") {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Callback not found: %s", id)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleLambdaSendDurableCallbackHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := sim.PathParam(r, "cbid")
	// Heartbeat keeps a pending callback alive without changing its status.
	if !lambdaAdvanceCallback(id, "") {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Callback not found: %s", id)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}
