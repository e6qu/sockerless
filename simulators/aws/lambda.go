package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// lambdaProcessHandles tracks running Lambda processes for cancellation.
var lambdaProcessHandles sync.Map // map[requestID]*sim.ProcessHandle

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
	Tags         map[string]string      `json:"Tags,omitempty"`
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
	mux.HandleFunc("GET /2017-03-31/tags/{arn...}", handleLambdaListTags)
	mux.HandleFunc("POST /2017-03-31/tags/{arn...}", handleLambdaTagResource)
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
		Tags          map[string]string   `json:"Tags"`
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
		Tags:          req.Tags,
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

	// Determine invocation type
	invocationType := r.Header.Get("X-Amz-Invocation-Type")
	if invocationType == "" {
		invocationType = "RequestResponse"
	}

	w.Header().Set("X-Amz-Executed-Version", "$LATEST")

	switch strings.ToLower(invocationType) {
	case "event":
		injectLambdaLogs(fn.FunctionName)
		w.WriteHeader(http.StatusAccepted)
	case "dryrun":
		w.WriteHeader(http.StatusNoContent)
	default:
		// RequestResponse — execute real process if image function has a command
		responseBody := []byte("{}")
		if fn.PackageType == "Image" && fn.ImageConfig != nil && len(fn.ImageConfig.Command) > 0 {
			responseBody = invokeLambdaProcess(fn)
		} else {
			injectLambdaLogs(fn.FunctionName)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseBody)
	}
}

// invokeLambdaProcess executes a Lambda function's image command via sim.StartProcess
// and returns the stdout output as the response body.
func invokeLambdaProcess(fn LambdaFunction) []byte {
	// Build command from EntryPoint + Command (mirrors real Lambda container image support)
	var fullCmd []string
	if fn.ImageConfig != nil {
		fullCmd = append(fullCmd, fn.ImageConfig.EntryPoint...)
		fullCmd = append(fullCmd, fn.ImageConfig.Command...)
	}
	if len(fullCmd) == 0 {
		return []byte("{}")
	}

	// Extract environment variables
	var cmdEnv map[string]string
	if fn.Environment != nil && len(fn.Environment.Variables) > 0 {
		cmdEnv = fn.Environment.Variables
	}

	// Set up CloudWatch log group and stream
	logGroup := fmt.Sprintf("/aws/lambda/%s", fn.FunctionName)
	now := time.Now()
	nowMs := now.UnixMilli()

	if _, exists := cwLogGroups.Get(logGroup); !exists {
		cwLogGroups.Put(logGroup, CWLogGroup{
			LogGroupName: logGroup,
			Arn:          cwLogGroupArn(logGroup),
			CreationTime: nowMs,
		})
	}

	hexBytes := make([]byte, 8)
	if _, err := rand.Read(hexBytes); err != nil {
		hexBytes = []byte{0, 0, 0, 0, 0, 0, 0, 0}
	}
	hexSuffix := hex.EncodeToString(hexBytes)
	logStreamName := fmt.Sprintf("%s/[$LATEST]%s", now.Format("2006/01/02"), hexSuffix)

	key := cwEventsKey(logGroup, logStreamName)
	cwLogStreams.Put(key, CWLogStream{
		LogStreamName:       logStreamName,
		LogGroupName:        logGroup,
		CreationTime:        nowMs,
		FirstEventTimestamp: nowMs,
		LastEventTimestamp:  nowMs,
		Arn:                 cwLogStreamArn(logGroup, logStreamName),
		UploadSequenceToken: "1",
	})

	// Inject START log entry
	requestID := generateUUID()
	cwLogEvents.Put(key, []CWLogEvent{
		{Timestamp: nowMs, Message: fmt.Sprintf("START RequestId: %s Version: $LATEST", requestID), IngestionTime: nowMs},
	})

	// Execute the command
	timeout := time.Duration(fn.Timeout) * time.Second
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	sink := &lambdaLogSink{logGroup: logGroup, logStream: logStreamName}
	var stdout bytes.Buffer
	collectSink := sim.FuncSink(func(line sim.LogLine) {
		sink.WriteLog(line)
		if line.Stream == "stdout" {
			stdout.WriteString(line.Text)
			stdout.WriteByte('\n')
		}
	})

	handle := sim.StartProcess(sim.ProcessConfig{
		Command: fullCmd,
		Env:     cmdEnv,
		Timeout: timeout,
	}, collectSink)
	lambdaProcessHandles.Store(requestID, handle)
	result := handle.Wait()
	lambdaProcessHandles.Delete(requestID)

	// Inject END and REPORT log entries
	endMs := time.Now().UnixMilli()
	duration := float64(result.StoppedAt.Sub(result.StartedAt).Microseconds()) / 1000.0
	cwLogEvents.Update(key, func(events *[]CWLogEvent) {
		*events = append(*events,
			CWLogEvent{Timestamp: endMs, Message: fmt.Sprintf("END RequestId: %s", requestID), IngestionTime: endMs},
			CWLogEvent{Timestamp: endMs + 1, Message: fmt.Sprintf("REPORT RequestId: %s\tDuration: %.2f ms\tBilled Duration: %d ms\tMemory Size: %d MB\tMax Memory Used: %d MB",
				requestID, duration, int64(duration)+1, fn.MemorySize, fn.MemorySize/2), IngestionTime: endMs + 1},
		)
	})

	if result.ExitCode != 0 {
		cwLogEvents.Update(key, func(events *[]CWLogEvent) {
			*events = append(*events, CWLogEvent{
				Timestamp:     endMs + 2,
				Message:       fmt.Sprintf("ERROR RequestId: %s Process exited with code %d", requestID, result.ExitCode),
				IngestionTime: endMs + 2,
			})
		})
	}

	output := strings.TrimRight(stdout.String(), "\n")
	if output == "" {
		return []byte("{}")
	}
	return []byte(output)
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

// injectLambdaLogs creates a CloudWatch log group, stream, and initial log
// entries for a Lambda function invocation, mirroring the ECS pattern in ecs.go.
func injectLambdaLogs(functionName string) {
	logGroup := fmt.Sprintf("/aws/lambda/%s", functionName)
	now := time.Now()
	nowMs := now.UnixMilli()

	// Create log group if not exists
	if _, exists := cwLogGroups.Get(logGroup); !exists {
		cwLogGroups.Put(logGroup, CWLogGroup{
			LogGroupName: logGroup,
			Arn:          cwLogGroupArn(logGroup),
			CreationTime: nowMs,
		})
	}

	// Build stream name: YYYY/MM/DD/[$LATEST]<16-char hex>
	hexBytes := make([]byte, 8)
	if _, err := rand.Read(hexBytes); err != nil {
		hexBytes = []byte{0, 0, 0, 0, 0, 0, 0, 0}
	}
	hexSuffix := hex.EncodeToString(hexBytes)
	logStreamName := fmt.Sprintf("%s/[$LATEST]%s", now.Format("2006/01/02"), hexSuffix)

	// Create log stream
	key := cwEventsKey(logGroup, logStreamName)
	cwLogStreams.Put(key, CWLogStream{
		LogStreamName:       logStreamName,
		LogGroupName:        logGroup,
		CreationTime:        nowMs,
		FirstEventTimestamp: nowMs,
		LastEventTimestamp:  nowMs,
		Arn:                 cwLogStreamArn(logGroup, logStreamName),
		UploadSequenceToken: "1",
	})

	// Inject log entries mimicking real Lambda output
	requestID := generateUUID()
	cwLogEvents.Put(key, []CWLogEvent{
		{Timestamp: nowMs, Message: fmt.Sprintf("START RequestId: %s Version: $LATEST", requestID), IngestionTime: nowMs},
		{Timestamp: nowMs + 1, Message: fmt.Sprintf("END RequestId: %s", requestID), IngestionTime: nowMs + 1},
		{Timestamp: nowMs + 2, Message: fmt.Sprintf("REPORT RequestId: %s\tDuration: 1.00 ms\tBilled Duration: 1 ms\tMemory Size: 128 MB\tMax Memory Used: 64 MB", requestID), IngestionTime: nowMs + 2},
	})
}

func handleLambdaListTags(w http.ResponseWriter, r *http.Request) {
	arn := r.PathValue("arn")
	// Extract function name from ARN
	name := arn
	if strings.Contains(arn, ":function:") {
		parts := strings.SplitN(arn, ":function:", 2)
		if len(parts) == 2 {
			name = parts[1]
		}
	}

	fn, ok := lambdaFunctions.Get(name)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}

	tags := fn.Tags
	if tags == nil {
		tags = map[string]string{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Tags": tags,
	})
}

// lambdaLogSink implements sim.LogSink and writes log lines to CloudWatch
// for Lambda function invocations.
type lambdaLogSink struct {
	logGroup  string
	logStream string
}

func (s *lambdaLogSink) WriteLog(line sim.LogLine) {
	key := cwEventsKey(s.logGroup, s.logStream)
	nowMs := time.Now().UnixMilli()
	cwLogEvents.Update(key, func(events *[]CWLogEvent) {
		*events = append(*events, CWLogEvent{
			Timestamp:     nowMs,
			Message:       line.Text,
			IngestionTime: nowMs,
		})
	})
}

func handleLambdaTagResource(w http.ResponseWriter, r *http.Request) {
	arn := r.PathValue("arn")
	name := arn
	if strings.Contains(arn, ":function:") {
		parts := strings.SplitN(arn, ":function:", 2)
		if len(parts) == 2 {
			name = parts[1]
		}
	}

	var req struct {
		Tags map[string]string `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}

	found := lambdaFunctions.Update(name, func(fn *LambdaFunction) {
		if fn.Tags == nil {
			fn.Tags = make(map[string]string)
		}
		for k, v := range req.Tags {
			fn.Tags[k] = v
		}
	})

	if !found {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
