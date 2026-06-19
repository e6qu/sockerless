package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// lambdaProcessHandles tracks running Lambda containers for cancellation.
var lambdaProcessHandles sync.Map // map[requestID]*sim.ContainerHandle

// Lambda types

type LambdaFunction struct {
	FunctionName     string              `json:"FunctionName"`
	FunctionArn      string              `json:"FunctionArn"`
	Runtime          string              `json:"Runtime,omitempty"`
	Role             string              `json:"Role"`
	Handler          string              `json:"Handler,omitempty"`
	Code             *LambdaFunctionCode `json:"Code,omitempty"`
	CodeSha256       string              `json:"CodeSha256,omitempty"`
	CodeSize         int64               `json:"CodeSize"`
	Description      string              `json:"Description,omitempty"`
	MemorySize       int                 `json:"MemorySize"`
	Timeout          int                 `json:"Timeout"`
	Environment      *LambdaEnvironment  `json:"Environment,omitempty"`
	Tags             map[string]string   `json:"Tags,omitempty"`
	State            string              `json:"State"`
	LastUpdateStatus string              `json:"LastUpdateStatus,omitempty"`
	LastModified     string              `json:"LastModified"`
	RevisionId       string              `json:"RevisionId"`
	Version          string              `json:"Version"`
	PackageType      string              `json:"PackageType,omitempty"`
	Architectures    []string            `json:"Architectures,omitempty"`
	ImageConfig      *LambdaImageConfig  `json:"ImageConfig,omitempty"`
	VpcConfig        *LambdaVpcConfig    `json:"VpcConfig,omitempty"`
}

// LambdaVpcConfig matches the real Lambda CreateFunction shape. When
// SubnetIds is set, AWS allocates a Hyperplane ENI per subnet for the
// function's outbound traffic; the sim allocates an IP from each
// subnet's CidrBlock so DescribeFunction returns an accurate Ipv4 list.
type LambdaVpcConfig struct {
	SubnetIds               []string `json:"SubnetIds,omitempty"`
	SecurityGroupIds        []string `json:"SecurityGroupIds,omitempty"`
	VpcId                   string   `json:"VpcId,omitempty"`
	Ipv6AllowedForDualStack bool     `json:"Ipv6AllowedForDualStack,omitempty"`
	// SubnetIPv4Allocations: one entry per SubnetId, in matching order.
	// Real Lambda's DescribeFunction doesn't expose ENI IPs directly,
	// but Hyperplane creates them in the listed subnets — backends that
	// need to verify ENI provisioning consume this slice. Empty when no
	// VpcConfig is set on CreateFunction.
	SubnetIPv4Allocations []string `json:"SubnetIPv4Allocations,omitempty"`
}

type lambdaVpcConfigConfiguration struct {
	SubnetIds               []string `json:"SubnetIds,omitempty"`
	SecurityGroupIds        []string `json:"SecurityGroupIds,omitempty"`
	VpcId                   string   `json:"VpcId,omitempty"`
	Ipv6AllowedForDualStack bool     `json:"Ipv6AllowedForDualStack,omitempty"`
}

func lambdaVpcConfiguration(vpc *LambdaVpcConfig) *lambdaVpcConfigConfiguration {
	if vpc == nil {
		return nil
	}
	return &lambdaVpcConfigConfiguration{
		SubnetIds:               vpc.SubnetIds,
		SecurityGroupIds:        vpc.SecurityGroupIds,
		VpcId:                   vpc.VpcId,
		Ipv6AllowedForDualStack: vpc.Ipv6AllowedForDualStack,
	}
}

type LambdaFunctionCode struct {
	S3Bucket        string `json:"S3Bucket,omitempty"`
	S3Key           string `json:"S3Key,omitempty"`
	S3ObjectVersion string `json:"S3ObjectVersion,omitempty"`
	ImageUri        string `json:"ImageUri,omitempty"` // external (operator-supplied): OCI image reference, any registry
	ZipFile         string `json:"ZipFile,omitempty"`
	SourceKMSKeyArn string `json:"SourceKMSKeyArn,omitempty"`
}

type LambdaEnvironment struct {
	Variables map[string]string `json:"Variables,omitempty"`
}

type LambdaImageConfig struct {
	EntryPoint       []string `json:"EntryPoint,omitempty"`
	Command          []string `json:"Command,omitempty"`
	WorkingDirectory string   `json:"WorkingDirectory,omitempty"`
}

// LambdaImageConfigResponse wraps the image config in FunctionConfiguration
// responses. AWS accepts ImageConfig as CreateFunction input but returns it
// under ImageConfigResponse on GetFunction/CreateFunction, which is the field
// the SDK FunctionConfiguration shape reads.
type LambdaImageConfigResponse struct {
	ImageConfig *LambdaImageConfig `json:"ImageConfig,omitempty"`
}

type lambdaFunctionConfiguration struct {
	FunctionName        string                        `json:"FunctionName"`
	FunctionArn         string                        `json:"FunctionArn"`
	Runtime             string                        `json:"Runtime,omitempty"`
	Role                string                        `json:"Role"`
	Handler             string                        `json:"Handler,omitempty"`
	CodeSha256          string                        `json:"CodeSha256,omitempty"`
	CodeSize            int64                         `json:"CodeSize"`
	Description         string                        `json:"Description,omitempty"`
	MemorySize          int                           `json:"MemorySize"`
	Timeout             int                           `json:"Timeout"`
	Environment         *LambdaEnvironment            `json:"Environment,omitempty"`
	State               string                        `json:"State"`
	LastUpdateStatus    string                        `json:"LastUpdateStatus,omitempty"`
	LastModified        string                        `json:"LastModified"`
	RevisionId          string                        `json:"RevisionId"`
	Version             string                        `json:"Version"`
	PackageType         string                        `json:"PackageType,omitempty"`
	Architectures       []string                      `json:"Architectures,omitempty"`
	ImageConfigResponse *LambdaImageConfigResponse    `json:"ImageConfigResponse,omitempty"`
	VpcConfig           *lambdaVpcConfigConfiguration `json:"VpcConfig,omitempty"`
}

func lambdaConfiguration(fn LambdaFunction) lambdaFunctionConfiguration {
	cfg := lambdaFunctionConfiguration{
		FunctionName:     fn.FunctionName,
		FunctionArn:      fn.FunctionArn,
		Runtime:          fn.Runtime,
		Role:             fn.Role,
		Handler:          fn.Handler,
		CodeSha256:       fn.CodeSha256,
		CodeSize:         fn.CodeSize,
		Description:      fn.Description,
		MemorySize:       fn.MemorySize,
		Timeout:          fn.Timeout,
		Environment:      fn.Environment,
		State:            fn.State,
		LastUpdateStatus: fn.LastUpdateStatus,
		LastModified:     fn.LastModified,
		RevisionId:       fn.RevisionId,
		Version:          fn.Version,
		PackageType:      fn.PackageType,
		Architectures:    fn.Architectures,
		VpcConfig:        lambdaVpcConfiguration(fn.VpcConfig),
	}
	if fn.ImageConfig != nil {
		cfg.ImageConfigResponse = &LambdaImageConfigResponse{ImageConfig: fn.ImageConfig}
	}
	return cfg
}

// State store
var lambdaFunctions sim.Store[LambdaFunction]

func lambdaArn(name string) string {
	return fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s", awsRegion(), awsAccountID(), name)
}

func registerLambda(srv *sim.Server) {
	lambdaFunctions = sim.MakeStore[LambdaFunction](srv.DB(), "lambda_functions")

	mux := srv

	lambdaResource := cloudTrailRESTResource("AWS::Lambda::Function", "name", "arn")
	mux.HandleFunc("POST /2015-03-31/functions", cloudTrailRecordedREST("CreateFunction", "lambda.amazonaws.com", nil, handleLambdaCreateFunction))
	mux.HandleFunc("GET /2015-03-31/functions/{name}", cloudTrailRecordedREST("GetFunction", "lambda.amazonaws.com", lambdaResource, handleLambdaGetFunction))
	mux.HandleFunc("DELETE /2015-03-31/functions/{name}", cloudTrailRecordedREST("DeleteFunction", "lambda.amazonaws.com", lambdaResource, handleLambdaDeleteFunction))
	mux.HandleFunc("PUT /2015-03-31/functions/{name}/configuration", cloudTrailRecordedREST("UpdateFunctionConfiguration", "lambda.amazonaws.com", lambdaResource, handleLambdaUpdateFunctionConfiguration))
	mux.HandleFunc("POST /2015-03-31/functions/{name}/invocations", cloudTrailRecordedREST("Invoke", "lambda.amazonaws.com", lambdaResource, handleLambdaInvoke))
	mux.HandleFunc("GET /2015-03-31/functions", cloudTrailRecordedREST("ListFunctions", "lambda.amazonaws.com", nil, handleLambdaListFunctions))
	mux.HandleFunc("GET /2015-03-31/functions/", cloudTrailRecordedREST("ListFunctions", "lambda.amazonaws.com", nil, handleLambdaListFunctions))
	mux.HandleFunc("GET /2017-03-31/tags/{arn...}", cloudTrailRecordedREST("ListTags", "lambda.amazonaws.com", lambdaResource, handleLambdaListTags))
	mux.HandleFunc("POST /2017-03-31/tags/{arn...}", cloudTrailRecordedREST("TagResource", "lambda.amazonaws.com", lambdaResource, handleLambdaTagResource))
	mux.HandleFunc("DELETE /2017-03-31/tags/{arn...}", cloudTrailRecordedREST("UntagResource", "lambda.amazonaws.com", lambdaResource, handleLambdaUntagResource))

	// Versions + aliases + permissions + function URL config.
	mux.HandleFunc("POST /2015-03-31/functions/{name}/versions", cloudTrailRecordedREST("PublishVersion", "lambda.amazonaws.com", lambdaResource, handleLambdaPublishVersion))
	mux.HandleFunc("GET /2015-03-31/functions/{name}/versions", cloudTrailRecordedREST("ListVersionsByFunction", "lambda.amazonaws.com", lambdaResource, handleLambdaListVersions))
	mux.HandleFunc("POST /2015-03-31/functions/{name}/aliases", cloudTrailRecordedREST("CreateAlias", "lambda.amazonaws.com", lambdaResource, handleLambdaCreateAlias))
	mux.HandleFunc("GET /2015-03-31/functions/{name}/aliases", cloudTrailRecordedREST("ListAliases", "lambda.amazonaws.com", lambdaResource, handleLambdaListAliases))
	mux.HandleFunc("GET /2015-03-31/functions/{name}/aliases/{alias}", cloudTrailRecordedREST("GetAlias", "lambda.amazonaws.com", lambdaResource, handleLambdaGetAlias))
	mux.HandleFunc("PUT /2015-03-31/functions/{name}/aliases/{alias}", cloudTrailRecordedREST("UpdateAlias", "lambda.amazonaws.com", lambdaResource, handleLambdaUpdateAlias))
	mux.HandleFunc("DELETE /2015-03-31/functions/{name}/aliases/{alias}", cloudTrailRecordedREST("DeleteAlias", "lambda.amazonaws.com", lambdaResource, handleLambdaDeleteAlias))
	mux.HandleFunc("POST /2015-03-31/functions/{name}/policy", cloudTrailRecordedREST("AddPermission", "lambda.amazonaws.com", lambdaResource, handleLambdaAddPermission))
	mux.HandleFunc("GET /2015-03-31/functions/{name}/policy", cloudTrailRecordedREST("GetPolicy", "lambda.amazonaws.com", lambdaResource, handleLambdaGetPolicy))
	mux.HandleFunc("DELETE /2015-03-31/functions/{name}/policy/{statement}", cloudTrailRecordedREST("RemovePermission", "lambda.amazonaws.com", lambdaResource, handleLambdaRemovePermission))
	mux.HandleFunc("POST /2021-10-31/functions/{name}/url", cloudTrailRecordedREST("CreateFunctionUrlConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaCreateFunctionUrlConfig))
	mux.HandleFunc("GET /2021-10-31/functions/{name}/url", cloudTrailRecordedREST("GetFunctionUrlConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaGetFunctionUrlConfig))
	mux.HandleFunc("PUT /2021-10-31/functions/{name}/url", cloudTrailRecordedREST("UpdateFunctionUrlConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaUpdateFunctionUrlConfig))
	mux.HandleFunc("DELETE /2021-10-31/functions/{name}/url", cloudTrailRecordedREST("DeleteFunctionUrlConfig", "lambda.amazonaws.com", lambdaResource, handleLambdaDeleteFunctionUrlConfig))
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
		Publish       bool                `json:"Publish"`
		Architectures []string            `json:"Architectures"`
		ImageConfig   *LambdaImageConfig  `json:"ImageConfig"`
		VpcConfig     *LambdaVpcConfig    `json:"VpcConfig"`
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

	// Real Lambda allocates a Hyperplane ENI per VpcConfig.SubnetId, with
	// an IP drawn from the subnet's CIDR. Validate the subnet exists and
	// allocate one IP per subnet up front so DescribeFunction reflects
	// the real attached-IP list.
	vpcConfig := req.VpcConfig
	if vpcConfig != nil && len(vpcConfig.SubnetIds) > 0 {
		ips := make([]string, 0, len(vpcConfig.SubnetIds))
		for _, subnetID := range vpcConfig.SubnetIds {
			ip, ipErr := AllocateSubnetIP(subnetID)
			if ipErr != nil {
				sim.AWSError(w, "InvalidParameterValueException", ipErr.Error(), http.StatusBadRequest)
				return
			}
			ips = append(ips, ip)
		}
		vpcConfig.SubnetIPv4Allocations = ips
		// Real Lambda echoes the VPC ID back; resolve it from the first
		// subnet's stored VpcId so the response shape matches CreateFunction
		// from the AWS SDK.
		if first, ok := ec2Subnets.Get(vpcConfig.SubnetIds[0]); ok {
			vpcConfig.VpcId = first.VpcId
		}
	}

	fn := LambdaFunction{
		FunctionName:     req.FunctionName,
		FunctionArn:      lambdaArn(req.FunctionName),
		Runtime:          req.Runtime,
		Role:             req.Role,
		Handler:          req.Handler,
		Code:             req.Code,
		CodeSha256:       lambdaCodeSha256(req.Code),
		CodeSize:         1024,
		Description:      req.Description,
		MemorySize:       req.MemorySize,
		Timeout:          req.Timeout,
		Environment:      req.Environment,
		Tags:             req.Tags,
		State:            "Active",
		LastUpdateStatus: "Successful",
		LastModified:     time.Now().UTC().Format(time.RFC3339),
		RevisionId:       generateUUID(),
		Version:          "$LATEST",
		PackageType:      req.PackageType,
		Architectures:    req.Architectures,
		ImageConfig:      req.ImageConfig,
		VpcConfig:        vpcConfig,
	}
	lambdaFunctions.Put(req.FunctionName, fn)
	if req.Publish {
		publishLambdaVersion(req.FunctionName, "", fn)
	}

	sim.WriteJSON(w, http.StatusCreated, lambdaConfiguration(fn))
}

func lambdaCodeSha256(code *LambdaFunctionCode) string {
	if code == nil || code.ImageUri != "" {
		return ""
	}
	material := code.ZipFile
	if material == "" {
		material = code.S3Bucket + "/" + code.S3Key + "@" + code.S3ObjectVersion
	}
	if material == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(material))
	return base64.StdEncoding.EncodeToString(sum[:])
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

	// Code.Location is external: real Lambda returns a presigned S3
	// URL (`https://awslambda-<region>-tasks.s3.<region>.amazonaws.com/snapshots/<fn>`)
	// that aws-sdk-go-v2 + terraform-provider-aws + the Lambda console
	// dereference to fetch the deployment package. The sim does not
	// host an equivalent surface; clients that follow the URL will
	// receive whatever real AWS returns for an unknown snapshot.
	code := map[string]string{
		"Location": fmt.Sprintf("https://awslambda-%s-tasks.s3.%s.amazonaws.com/snapshots/%s", awsRegion(), awsRegion(), name),
		// RepositoryType is a documented GetFunction response field: "S3" for a
		// ZIP package, "ECR" for a container-image package (overridden below).
		"RepositoryType": "S3",
	}
	if fn.Code != nil {
		if fn.Code.ImageUri != "" {
			code["ImageUri"] = fn.Code.ImageUri
			code["ResolvedImageUri"] = fn.Code.ImageUri
			code["RepositoryType"] = "ECR"
		}
		if fn.Code.SourceKMSKeyArn != "" {
			code["SourceKMSKeyArn"] = fn.Code.SourceKMSKeyArn
		}
	}
	tags := fn.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Configuration": lambdaConfiguration(fn),
		"Code":          code,
		"Tags":          tags,
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
		VpcConfig   *LambdaVpcConfig   `json:"VpcConfig"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterValueException", "Invalid request body", http.StatusBadRequest)
		return
	}

	// Real Lambda re-allocates Hyperplane ENIs when SubnetIds change.
	// Validate the new subnets exist before applying any update so a
	// half-applied configuration can't slip through.
	var newAllocations []string
	var newVpcID string
	if req.VpcConfig != nil && len(req.VpcConfig.SubnetIds) > 0 {
		newAllocations = make([]string, 0, len(req.VpcConfig.SubnetIds))
		for _, subnetID := range req.VpcConfig.SubnetIds {
			ip, ipErr := AllocateSubnetIP(subnetID)
			if ipErr != nil {
				sim.AWSError(w, "InvalidParameterValueException", ipErr.Error(), http.StatusBadRequest)
				return
			}
			newAllocations = append(newAllocations, ip)
		}
		if first, ok := ec2Subnets.Get(req.VpcConfig.SubnetIds[0]); ok {
			newVpcID = first.VpcId
		}
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
		if req.VpcConfig != nil {
			req.VpcConfig.SubnetIPv4Allocations = newAllocations
			if req.VpcConfig.VpcId == "" {
				req.VpcConfig.VpcId = newVpcID
			}
			fn.VpcConfig = req.VpcConfig
		}
		fn.LastModified = time.Now().UTC().Format(time.RFC3339)
		fn.LastUpdateStatus = "Successful"
		fn.RevisionId = generateUUID()
	})

	if !found {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}

	fn, _ := lambdaFunctions.Get(name)
	sim.WriteJSON(w, http.StatusOK, lambdaConfiguration(fn))
}

// handleLambdaInvoke implements the AWS Lambda Invoke API. For Image
// package-type functions, it routes through the Runtime API slice
// (see lambda_runtime.go): the simulator stands up a
// per-invocation Runtime API listener, launches the container with
// AWS_LAMBDA_RUNTIME_API pointing at it, and returns whatever the
// handler posts back to /response (or /error → X-Amz-Function-Error:
// Unhandled). Matches real Lambda; no synthetic stdout capture.
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
		// RequestResponse — Image-package functions go through the
		// Runtime API slice; Zip-package functions stay on the
		// control-plane synthetic path (no container launched).
		//
		// aws-sdk-go-v2 gates request-compression via the
		// `requestcompression` trait; Lambda Invoke isn't in that
		// set today but has been flagged for future inclusion. If a
		// caller sends a `Content-Encoding`, fail loud rather than
		// forward the gzipped envelope into the runtime.
		if ce := r.Header.Get("Content-Encoding"); ce != "" {
			sim.AWSError(w, "InvalidRequest",
				fmt.Sprintf("Content-Encoding %q not supported on Lambda Invoke", ce),
				http.StatusUnsupportedMediaType)
			return
		}
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			sim.AWSError(w, "RequestBodyInvalid",
				"failed to read invocation payload: "+err.Error(),
				http.StatusBadRequest)
			return
		}
		responseBody, unhandled, _ := invokeLambdaViaRuntimeAPI(fn, payload)
		if unhandled {
			w.Header().Set("X-Amz-Function-Error", "Unhandled")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(responseBody)
	}
}

func handleLambdaListFunctions(w http.ResponseWriter, r *http.Request) {
	stored := lambdaFunctions.List()
	sortBy(stored, func(f LambdaFunction) string { return f.FunctionName })

	marker := r.URL.Query().Get("Marker")
	maxItems := 50
	if raw := r.URL.Query().Get("MaxItems"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			maxItems = n
		}
	}
	page, next := awsPage(stored, marker, maxItems, 50)

	functions := make([]lambdaFunctionConfiguration, 0, len(page))
	for _, fn := range page {
		functions = append(functions, lambdaConfiguration(fn))
	}
	out := map[string]any{"Functions": functions}
	if next != "" {
		out["NextMarker"] = next
	}
	sim.WriteJSON(w, http.StatusOK, out)
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

// handleLambdaUntagResource implements DELETE /2017-03-31/tags/{arn}?tagKeys=a&tagKeys=b.
// Real Lambda's UntagResource takes ARN + list of tag keys to remove;
// idempotent (deleting a non-existent key returns 204). The sim
// previously had only GET + POST for this path; sockerless's pool
// release path calls UntagResource and was getting a generic 405 from
// the default mux, surfaced as a JSON-decode error in the SDK.
//
// Pattern carry-forward: ARN parsing is lenient — the function-name
// portion is the key, and any account-ID embedded in the ARN is
// ignored. Matches handleLambdaTagResource's behaviour so a single
// sim can serve calls with different operator-supplied account IDs
// (the sim's lambdaArn helper hardcodes 123456789012).
func handleLambdaUntagResource(w http.ResponseWriter, r *http.Request) {
	arn := r.PathValue("arn")
	name := arn
	if strings.Contains(arn, ":function:") {
		parts := strings.SplitN(arn, ":function:", 2)
		if len(parts) == 2 {
			name = parts[1]
		}
	}

	tagKeys := r.URL.Query()["tagKeys"]

	found := lambdaFunctions.Update(name, func(fn *LambdaFunction) {
		if fn.Tags == nil {
			return
		}
		for _, k := range tagKeys {
			delete(fn.Tags, k)
		}
	})

	if !found {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusNotFound,
			"Function not found: %s", lambdaArn(name))
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
