package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// Lambda types

type LambdaFunction struct {
	FunctionName string                 `json:"FunctionName"`
	FunctionArn  string                 `json:"FunctionArn"`
	Runtime      string                 `json:"Runtime,omitempty"`
	Role         string                 `json:"Role"`
	Handler      string                 `json:"Handler,omitempty"`
	Code         *LambdaFunctionCode    `json:"Code,omitempty"`
	CodeSize     int64                  `json:"CodeSize"`
	Description  string                 `json:"Description,omitempty"`
	MemorySize   int                    `json:"MemorySize"`
	Timeout      int                    `json:"Timeout"`
	Environment  *LambdaEnvironment     `json:"Environment,omitempty"`
	State        string                 `json:"State"`
	LastModified string                 `json:"LastModified"`
	RevisionId   string                 `json:"RevisionId"`
	Version      string                 `json:"Version"`
	PackageType  string                 `json:"PackageType,omitempty"`
	Architectures []string              `json:"Architectures,omitempty"`
	ImageConfig  *LambdaImageConfig     `json:"ImageConfig,omitempty"`
}

type LambdaFunctionCode struct {
	S3Bucket        string `json:"S3Bucket,omitempty"`
	S3Key           string `json:"S3Key,omitempty"`
	S3ObjectVersion string `json:"S3ObjectVersion,omitempty"`
	ImageUri        string `json:"ImageUri,omitempty"`
	ZipFile         string `json:"ZipFile,omitempty"`
}

type LambdaEnvironment struct {
	Variables map[string]string `json:"Variables,omitempty"`
}

type LambdaImageConfig struct {
	EntryPoint       []string `json:"EntryPoint,omitempty"`
	Command          []string `json:"Command,omitempty"`
	WorkingDirectory string   `json:"WorkingDirectory,omitempty"`
}

// State store
var lambdaFunctions *sim.StateStore[LambdaFunction]

// Agent subprocess tracker
var lambdaAgentProcs sync.Map // map[functionName]*exec.Cmd

func lambdaArn(name string) string {
	return fmt.Sprintf("arn:aws:lambda:us-east-1:123456789012:function:%s", name)
}

func registerLambda(srv *sim.Server) {
	lambdaFunctions = sim.NewStateStore[LambdaFunction]()

	mux := srv.Mux()

	mux.HandleFunc("POST /2015-03-31/functions", handleLambdaCreateFunction)
	mux.HandleFunc("GET /2015-03-31/functions/{name}", handleLambdaGetFunction)
	mux.HandleFunc("DELETE /2015-03-31/functions/{name}", handleLambdaDeleteFunction)
	mux.HandleFunc("PUT /2015-03-31/functions/{name}/configuration", handleLambdaUpdateFunctionConfiguration)
	mux.HandleFunc("POST /2015-03-31/functions/{name}/invocations", handleLambdaInvoke)
	mux.HandleFunc("GET /2015-03-31/functions", handleLambdaListFunctions)
	mux.HandleFunc("GET /2015-03-31/functions/", handleLambdaListFunctions)
}

func handleLambdaCreateFunction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FunctionName  string              `json:"FunctionName"`
		Runtime       string              `json:"Runtime"`
		Role          string              `json:"Role"`
		Handler       string              `json:"Handler"`
		Code          *LambdaFunctionCode `json:"Code"`
		Description   string              `json:"Description"`
		MemorySize    int                 `json:"MemorySize"`
		Timeout       int                 `json:"Timeout"`
		Environment   *LambdaEnvironment  `json:"Environment"`
		PackageType   string              `json:"PackageType"`
		Architectures []string            `json:"Architectures"`
		ImageConfig   *LambdaImageConfig  `json:"ImageConfig"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.FunctionName == "" {
		sim.AWSError(w, "InvalidParameterValueException", "FunctionName is required", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		sim.AWSError(w, "InvalidParameterValueException", "Role is required", http.StatusBadRequest)
		return
	}

	if _, exists := lambdaFunctions.Get(req.FunctionName); exists {
		sim.AWSErrorf(w, "ResourceConflictException", http.StatusConflict,
			"Function already exist: %s", req.FunctionName)
		return
	}

	if req.MemorySize == 0 {
		req.MemorySize = 128
	}
	if req.Timeout == 0 {
		req.Timeout = 3
	}
	if req.PackageType == "" {
		req.PackageType = "Zip"
	}
	if len(req.Architectures) == 0 {
		req.Architectures = []string{"x86_64"}
	}

	fn := LambdaFunction{
		FunctionName:  req.FunctionName,
		FunctionArn:   lambdaArn(req.FunctionName),
		Runtime:       req.Runtime,
		Role:          req.Role,
		Handler:       req.Handler,
		Code:          req.Code,
		CodeSize:      1024,
		Description:   req.Description,
		MemorySize:    req.MemorySize,
		Timeout:       req.Timeout,
		Environment:   req.Environment,
		State:         "Active",
		LastModified:  time.Now().UTC().Format(time.RFC3339),
		RevisionId:    generateUUID(),
		Version:       "$LATEST",
		PackageType:   req.PackageType,
		Architectures: req.Architectures,
		ImageConfig:   req.ImageConfig,
	}
	lambdaFunctions.Put(req.FunctionName, fn)

	sim.WriteJSON(w, http.StatusCreated, fn)
}

func handleLambdaGetFunction(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if name == "" {
		sim.AWSError(w, "InvalidParameterValueException", "Function name is required", http.StatusBadRequest)
		return
	}

	fn, ok := lambdaFunctions.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Configuration": fn,
		"Code": map[string]string{
			"Location": fmt.Sprintf("https://awslambda-us-east-1-tasks.s3.us-east-1.amazonaws.com/snapshots/%s", name),
		},
	})
}

func handleLambdaDeleteFunction(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if name == "" {
		sim.AWSError(w, "InvalidParameterValueException", "Function name is required", http.StatusBadRequest)
		return
	}

	if !lambdaFunctions.Delete(name) {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}

	// Kill any agent subprocess for this function
	stopAgentProcess(&lambdaAgentProcs, name)

	w.WriteHeader(http.StatusNoContent)
}

func handleLambdaUpdateFunctionConfiguration(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if name == "" {
		sim.AWSError(w, "InvalidParameterValueException", "Function name is required", http.StatusBadRequest)
		return
	}

	var req struct {
		Runtime     string             `json:"Runtime"`
		Handler     string             `json:"Handler"`
		Description string             `json:"Description"`
		MemorySize  *int               `json:"MemorySize"`
		Timeout     *int               `json:"Timeout"`
		Environment *LambdaEnvironment `json:"Environment"`
		Role        string             `json:"Role"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}

	found := lambdaFunctions.Update(name, func(fn *LambdaFunction) {
		if req.Runtime != "" {
			fn.Runtime = req.Runtime
		}
		if req.Handler != "" {
			fn.Handler = req.Handler
		}
		if req.Description != "" {
			fn.Description = req.Description
		}
		if req.MemorySize != nil {
			fn.MemorySize = *req.MemorySize
		}
		if req.Timeout != nil {
			fn.Timeout = *req.Timeout
		}
		if req.Environment != nil {
			fn.Environment = req.Environment
		}
		if req.Role != "" {
			fn.Role = req.Role
		}
		fn.LastModified = time.Now().UTC().Format(time.RFC3339)
		fn.RevisionId = generateUUID()
	})

	if !found {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}

	fn, _ := lambdaFunctions.Get(name)
	sim.WriteJSON(w, http.StatusOK, fn)
}

func handleLambdaInvoke(w http.ResponseWriter, r *http.Request) {
	name := sim.PathParam(r, "name")
	if name == "" {
		sim.AWSError(w, "InvalidParameterValueException", "Function name is required", http.StatusBadRequest)
		return
	}

	fn, ok := lambdaFunctions.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}

	// Start agent subprocess if the function has a callback URL configured
	if callbackURL := lambdaGetAgentCallbackURL(fn); callbackURL != "" {
		startAgentProcess(&lambdaAgentProcs, name, callbackURL)
	}

	// Determine invocation type
	invocationType := r.Header.Get("X-Amz-Invocation-Type")
	if invocationType == "" {
		invocationType = "RequestResponse"
	}

	w.Header().Set("X-Amz-Executed-Version", "$LATEST")

	switch strings.ToLower(invocationType) {
	case "event":
		w.WriteHeader(http.StatusAccepted)
	case "dryrun":
		w.WriteHeader(http.StatusNoContent)
	default:
		// RequestResponse
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}
}

func handleLambdaListFunctions(w http.ResponseWriter, r *http.Request) {
	functions := lambdaFunctions.List()
	if functions == nil {
		functions = []LambdaFunction{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Functions": functions,
	})
}

// lambdaGetAgentCallbackURL extracts the agent callback URL from the function's
// environment variables, if present.
func lambdaGetAgentCallbackURL(fn LambdaFunction) string {
	if fn.Environment == nil || fn.Environment.Variables == nil {
		return ""
	}
	return fn.Environment.Variables["SOCKERLESS_AGENT_CALLBACK_URL"]
}

// startAgentProcess starts a sockerless-agent subprocess that dials back to the
// backend at callbackURL. The subprocess is tracked in procs for later cleanup.
func startAgentProcess(procs *sync.Map, key, callbackURL string) {
	// Don't start a second agent if one is already running
	if _, loaded := procs.Load(key); loaded {
		return
	}

	agentBin, err := exec.LookPath("sockerless-agent")
	if err != nil {
		log.Printf("[lambda] agent binary not found in PATH, skipping agent start for %s", key)
		return
	}

	cmd := exec.Command(agentBin, "--callback", callbackURL, "--keep-alive", "--", "tail", "-f", "/dev/null")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("[lambda] failed to start agent for %s: %v", key, err)
		return
	}

	log.Printf("[lambda] started agent subprocess for %s (pid=%d)", key, cmd.Process.Pid)
	procs.Store(key, cmd)

	go func() {
		_ = cmd.Wait()
		procs.Delete(key)
	}()
}

// stopAgentProcess kills an agent subprocess for the given key.
func stopAgentProcess(procs *sync.Map, key string) {
	v, ok := procs.LoadAndDelete(key)
	if !ok {
		return
	}
	cmd := v.(*exec.Cmd)
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
		log.Printf("[lambda] stopped agent subprocess for %s", key)
	}
}
