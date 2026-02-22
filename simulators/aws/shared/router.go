package simulator

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// AWSRouter routes AWS API requests based on the X-Amz-Target header.
// AWS JSON services use POST with X-Amz-Target: ServiceName.ActionName.
type AWSRouter struct {
	// handlers maps X-Amz-Target values to handler functions.
	handlers map[string]http.HandlerFunc
}

// NewAWSRouter creates a new AWS request router.
func NewAWSRouter() *AWSRouter {
	return &AWSRouter{
		handlers: make(map[string]http.HandlerFunc),
	}
}

// Register adds a handler for an X-Amz-Target value.
// Example target: "AmazonEC2ContainerServiceV20141113.RunTask"
func (r *AWSRouter) Register(target string, handler http.HandlerFunc) {
	r.handlers[target] = handler
}

// ServeHTTP dispatches to the handler matching the X-Amz-Target header.
func (r *AWSRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	target := req.Header.Get("X-Amz-Target")
	if target == "" {
		AWSError(w, "MissingAction", "X-Amz-Target header is required", http.StatusBadRequest)
		return
	}

	handler, ok := r.handlers[target]
	if !ok {
		AWSErrorf(w, "UnknownOperationException", http.StatusBadRequest,
			"Unknown operation: %s", target)
		return
	}

	handler(w, req)
}

// GCPRouter routes GCP API requests based on URL path patterns.
// GCP REST APIs use paths like /v2/projects/{project}/locations/{location}/jobs.
type GCPRouter struct {
	mux *http.ServeMux
}

// NewGCPRouter creates a new GCP request router.
func NewGCPRouter() *GCPRouter {
	return &GCPRouter{
		mux: http.NewServeMux(),
	}
}

// Handle registers a pattern (e.g., "POST /v2/projects/{project}/locations/{location}/jobs").
func (r *GCPRouter) Handle(pattern string, handler http.HandlerFunc) {
	r.mux.HandleFunc(pattern, handler)
}

// ServeHTTP dispatches to the matching path handler.
func (r *GCPRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// AzureRouter routes Azure ARM API requests based on resource provider paths.
// Azure ARM uses paths like /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.App/jobs/{name}.
type AzureRouter struct {
	mux *http.ServeMux
}

// NewAzureRouter creates a new Azure request router.
func NewAzureRouter() *AzureRouter {
	return &AzureRouter{
		mux: http.NewServeMux(),
	}
}

// Handle registers a pattern for ARM resource paths.
func (r *AzureRouter) Handle(pattern string, handler http.HandlerFunc) {
	r.mux.HandleFunc(pattern, handler)
}

// ServeHTTP dispatches to the matching path handler.
// It validates that the api-version query parameter is present.
func (r *AzureRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Skip api-version check for health and metadata endpoints
	if !strings.HasPrefix(req.URL.Path, "/health") && !strings.HasPrefix(req.URL.Path, "/metadata") {
		if req.URL.Query().Get("api-version") == "" {
			AzureError(w, "MissingApiVersion",
				"The api-version query parameter is required for all requests",
				http.StatusBadRequest)
			return
		}
	}
	r.mux.ServeHTTP(w, req)
}

// AWSQueryRouter routes AWS Query Protocol requests based on the Action form parameter.
// EC2, IAM, and STS use POST with form-encoded body containing Action=OperationName.
type AWSQueryRouter struct {
	handlers map[string]http.HandlerFunc
}

// NewAWSQueryRouter creates a new AWS Query Protocol request router.
func NewAWSQueryRouter() *AWSQueryRouter {
	return &AWSQueryRouter{
		handlers: make(map[string]http.HandlerFunc),
	}
}

// Register adds a handler for an Action value.
// Example action: "CreateVpc", "GetCallerIdentity"
func (r *AWSQueryRouter) Register(action string, handler http.HandlerFunc) {
	r.handlers[action] = handler
}

// ServeHTTP dispatches to the handler matching the Action form parameter.
func (r *AWSQueryRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`<Response><Errors><Error><Code>MalformedInput</Code><Message>Could not parse form body</Message></Error></Errors></Response>`))
		return
	}

	action := req.FormValue("Action")
	if action == "" {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`<Response><Errors><Error><Code>MissingAction</Code><Message>Action parameter is required</Message></Error></Errors></Response>`))
		return
	}

	handler, ok := r.handlers[action]
	if !ok {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`<Response><Errors><Error><Code>InvalidAction</Code><Message>The action ` + action + ` is not valid</Message></Error></Errors></Response>`))
		return
	}

	handler(w, req)
}

// ReadJSON reads and decodes a JSON request body into the given value.
func ReadJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

// PathParam extracts a path parameter from the request using Go 1.22+ routing.
func PathParam(r *http.Request, name string) string {
	return r.PathValue(name)
}
