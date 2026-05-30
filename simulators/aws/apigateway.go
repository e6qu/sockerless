package main

import (
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// API Gateway v1 (REST API) — restJson1 protocol, REST path routing
// under /restapis. Surface scoped to the resources / methods /
// integrations / stages / deployments CRUD that
// `terraform-provider-aws::aws_api_gateway_rest_api` exercises.

type APIGWRestApi struct {
	Id          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	CreatedDate int64             `json:"createdDate"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// Inner fields use the `restApiIdRef` tag (or similar non-canonical
// names) instead of `json:"-"` so they survive Store persistence
// round-trips. Real AWS doesn't emit these on per-resource GETs but
// the SDK ignores unknown fields, so the leak is harmless.
type APIGWResource struct {
	Id        string `json:"id"`
	RestApiId string `json:"restApiIdRef,omitempty"`
	ParentId  string `json:"parentId,omitempty"`
	PathPart  string `json:"pathPart,omitempty"`
	Path      string `json:"path"`
}

type APIGWMethod struct {
	HttpMethod        string            `json:"httpMethod"`
	ResourceId        string            `json:"resourceIdRef,omitempty"`
	RestApiId         string            `json:"restApiIdRef,omitempty"`
	AuthorizationType string            `json:"authorizationType,omitempty"`
	ApiKeyRequired    bool              `json:"apiKeyRequired,omitempty"`
	RequestParameters map[string]bool   `json:"requestParameters,omitempty"`
	RequestModels     map[string]string `json:"requestModels,omitempty"`
	MethodResponses   map[string]any    `json:"methodResponses,omitempty"`
	MethodIntegration *APIGWIntegration `json:"methodIntegration,omitempty"`
}

type APIGWIntegration struct {
	HttpMethod            string            `json:"methodRef,omitempty"`
	ResourceId            string            `json:"resourceIdRef,omitempty"`
	RestApiId             string            `json:"restApiIdRef,omitempty"`
	Type                  string            `json:"type"`
	Uri                   string            `json:"uri,omitempty"` // external (operator-supplied): integration target — Lambda ARN, HTTP backend, or VPC link target
	IntegrationHttpMethod string            `json:"httpMethod,omitempty"`
	RequestTemplates      map[string]string `json:"requestTemplates,omitempty"`
	RequestParameters     map[string]string `json:"requestParameters,omitempty"`
	CacheNamespace        string            `json:"cacheNamespace,omitempty"`
	TimeoutInMillis       int               `json:"timeoutInMillis,omitempty"`
	PassthroughBehavior   string            `json:"passthroughBehavior,omitempty"`
	ContentHandling       string            `json:"contentHandling,omitempty"`
}

// APIGWMethodResponse mirrors aws_api_gateway_method_response. Per-
// (method, statusCode) row keyed by `<restApiId>/<resourceId>/<httpMethod>/<statusCode>`.
type APIGWMethodResponse struct {
	StatusCode         string            `json:"statusCode"`
	ResponseModels     map[string]string `json:"responseModels,omitempty"`
	ResponseParameters map[string]bool   `json:"responseParameters,omitempty"`
}

// APIGWIntegrationResponse mirrors aws_api_gateway_integration_response.
// Per-(integration, statusCode) row keyed the same as APIGWMethodResponse.
type APIGWIntegrationResponse struct {
	StatusCode         string            `json:"statusCode"`
	SelectionPattern   string            `json:"selectionPattern,omitempty"`
	ResponseTemplates  map[string]string `json:"responseTemplates,omitempty"`
	ResponseParameters map[string]string `json:"responseParameters,omitempty"`
	ContentHandling    string            `json:"contentHandling,omitempty"`
}

type APIGWDeployment struct {
	Id          string `json:"id"`
	RestApiId   string `json:"restApiIdRef,omitempty"`
	Description string `json:"description,omitempty"`
	CreatedDate int64  `json:"createdDate"`
}

type APIGWStage struct {
	StageName    string `json:"stageName"`
	RestApiId    string `json:"restApiIdRef,omitempty"`
	DeploymentId string `json:"deploymentId"`
	CreatedDate  int64  `json:"createdDate"`
}

var (
	apigwRestApis             sim.Store[APIGWRestApi]
	apigwResources            sim.Store[APIGWResource]
	apigwMethods              sim.Store[APIGWMethod]
	apigwIntegrations         sim.Store[APIGWIntegration]
	apigwDeployments          sim.Store[APIGWDeployment]
	apigwStages               sim.Store[APIGWStage]
	apigwMethodResponses      sim.Store[APIGWMethodResponse]
	apigwIntegrationResponses sim.Store[APIGWIntegrationResponse]
)

func registerAPIGateway(srv *sim.Server) {
	apigwRestApis = sim.MakeStore[APIGWRestApi](srv.DB(), "apigw_restapis")
	apigwResources = sim.MakeStore[APIGWResource](srv.DB(), "apigw_resources")
	apigwMethods = sim.MakeStore[APIGWMethod](srv.DB(), "apigw_methods")
	apigwIntegrations = sim.MakeStore[APIGWIntegration](srv.DB(), "apigw_integrations")
	apigwDeployments = sim.MakeStore[APIGWDeployment](srv.DB(), "apigw_deployments")
	apigwStages = sim.MakeStore[APIGWStage](srv.DB(), "apigw_stages")
	apigwMethodResponses = sim.MakeStore[APIGWMethodResponse](srv.DB(), "apigw_method_responses")
	apigwIntegrationResponses = sim.MakeStore[APIGWIntegrationResponse](srv.DB(), "apigw_integration_responses")

	mux := srv.Mux()
	mux.HandleFunc("POST /restapis", handleAPIGWCreateRestApi)
	mux.HandleFunc("GET /restapis", handleAPIGWListRestApis)
	mux.HandleFunc("GET /restapis/{restApiId}", handleAPIGWGetRestApi)
	mux.HandleFunc("DELETE /restapis/{restApiId}", handleAPIGWDeleteRestApi)
	mux.HandleFunc("POST /restapis/{restApiId}/resources/{parentId}", handleAPIGWCreateResource)
	mux.HandleFunc("GET /restapis/{restApiId}/resources", handleAPIGWListResources)
	mux.HandleFunc("GET /restapis/{restApiId}/resources/{resourceId}", handleAPIGWGetResource)
	mux.HandleFunc("DELETE /restapis/{restApiId}/resources/{resourceId}", handleAPIGWDeleteResource)
	mux.HandleFunc("PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}", handleAPIGWPutMethod)
	mux.HandleFunc("GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}", handleAPIGWGetMethod)
	mux.HandleFunc("DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}", handleAPIGWDeleteMethod)
	mux.HandleFunc("PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration", handleAPIGWPutIntegration)
	mux.HandleFunc("GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration", handleAPIGWGetIntegration)
	mux.HandleFunc("DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration", handleAPIGWDeleteIntegration)
	mux.HandleFunc("POST /restapis/{restApiId}/deployments", handleAPIGWCreateDeployment)
	mux.HandleFunc("GET /restapis/{restApiId}/deployments/{deploymentId}", handleAPIGWGetDeployment)
	mux.HandleFunc("DELETE /restapis/{restApiId}/deployments/{deploymentId}", handleAPIGWDeleteDeployment)
	mux.HandleFunc("POST /restapis/{restApiId}/stages", handleAPIGWCreateStage)
	mux.HandleFunc("GET /restapis/{restApiId}/stages/{stageName}", handleAPIGWGetStage)
	mux.HandleFunc("DELETE /restapis/{restApiId}/stages/{stageName}", handleAPIGWDeleteStage)

	// Method + integration response CRUD per status code.
	// terraform-provider-aws's `aws_api_gateway_method_response` +
	// `aws_api_gateway_integration_response` resources read/write
	// these on every plan; without them the canonical method-create
	// flow never gets past response wiring.
	mux.HandleFunc("PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}", handleAPIGWPutMethodResponse)
	mux.HandleFunc("GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}", handleAPIGWGetMethodResponse)
	mux.HandleFunc("DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}", handleAPIGWDeleteMethodResponse)
	mux.HandleFunc("PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}", handleAPIGWPutIntegrationResponse)
	mux.HandleFunc("GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}", handleAPIGWGetIntegrationResponse)
	mux.HandleFunc("DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}", handleAPIGWDeleteIntegrationResponse)
}

func handleAPIGWCreateRestApi(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Tags        map[string]string `json:"tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	api := APIGWRestApi{
		Id:          generateUUID()[:10],
		Name:        req.Name,
		Description: req.Description,
		CreatedDate: time.Now().Unix(),
		Tags:        req.Tags,
	}
	apigwRestApis.Put(api.Id, api)
	// Real API Gateway auto-creates the root "/" resource on Create.
	root := APIGWResource{
		Id:        generateUUID()[:10],
		RestApiId: api.Id,
		Path:      "/",
	}
	apigwResources.Put(api.Id+"/"+root.Id, root)
	sim.WriteJSON(w, http.StatusCreated, api)
}

func handleAPIGWListRestApis(w http.ResponseWriter, r *http.Request) {
	all := apigwRestApis.List()
	if all == nil {
		all = []APIGWRestApi{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"item": all})
}

func handleAPIGWGetRestApi(w http.ResponseWriter, r *http.Request) {
	id := sim.PathParam(r, "restApiId")
	api, ok := apigwRestApis.Get(id)
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Rest API identifier specified")
		return
	}
	sim.WriteJSON(w, http.StatusOK, api)
}

func handleAPIGWDeleteRestApi(w http.ResponseWriter, r *http.Request) {
	id := sim.PathParam(r, "restApiId")
	if !apigwRestApis.Delete(id) {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Rest API identifier specified")
		return
	}
	for _, res := range apigwResources.List() {
		if res.RestApiId == id {
			apigwResources.Delete(id + "/" + res.Id)
		}
	}
	for _, m := range apigwMethods.List() {
		if m.RestApiId == id {
			apigwMethods.Delete(apigwMethodKey(id, m.ResourceId, m.HttpMethod))
		}
	}
	for _, in := range apigwIntegrations.List() {
		if in.RestApiId == id {
			apigwIntegrations.Delete(apigwMethodKey(id, in.ResourceId, in.HttpMethod))
		}
	}
	for _, d := range apigwDeployments.List() {
		if d.RestApiId == id {
			apigwDeployments.Delete(apigwDeploymentKey(id, d.Id))
		}
	}
	for _, s := range apigwStages.List() {
		if s.RestApiId == id {
			apigwStages.Delete(apigwStageKey(id, s.StageName))
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleAPIGWCreateResource(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	parentId := sim.PathParam(r, "parentId")
	if _, ok := apigwRestApis.Get(apiId); !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Rest API identifier specified")
		return
	}
	var req struct {
		PathPart string `json:"pathPart"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	parent, ok := apigwResources.Get(apiId + "/" + parentId)
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Resource identifier specified")
		return
	}
	res := APIGWResource{
		Id:        generateUUID()[:10],
		RestApiId: apiId,
		ParentId:  parentId,
		PathPart:  req.PathPart,
		Path:      apigwChildPath(parent.Path, req.PathPart),
	}
	apigwResources.Put(apiId+"/"+res.Id, res)
	sim.WriteJSON(w, http.StatusCreated, res)
}

func handleAPIGWListResources(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	all := apigwResources.List()
	var out []APIGWResource
	for _, res := range all {
		if res.RestApiId == apiId {
			out = append(out, res)
		}
	}
	if out == nil {
		out = []APIGWResource{}
	}
	_ = all
	sim.WriteJSON(w, http.StatusOK, map[string]any{"item": out})
}

func handleAPIGWGetResource(w http.ResponseWriter, r *http.Request) {
	res, ok := apigwResources.Get(sim.PathParam(r, "restApiId") + "/" + sim.PathParam(r, "resourceId"))
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Resource identifier specified")
		return
	}
	sim.WriteJSON(w, http.StatusOK, res)
}

func handleAPIGWDeleteResource(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	resourceId := sim.PathParam(r, "resourceId")
	if !apigwResources.Delete(apiId + "/" + resourceId) {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Resource identifier specified")
		return
	}
	for _, m := range apigwMethods.List() {
		if m.RestApiId == apiId && m.ResourceId == resourceId {
			apigwMethods.Delete(apigwMethodKey(apiId, resourceId, m.HttpMethod))
		}
	}
	for _, in := range apigwIntegrations.List() {
		if in.RestApiId == apiId && in.ResourceId == resourceId {
			apigwIntegrations.Delete(apigwMethodKey(apiId, resourceId, in.HttpMethod))
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleAPIGWPutMethod(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	resourceId := sim.PathParam(r, "resourceId")
	httpMethod := sim.PathParam(r, "httpMethod")
	var req struct {
		AuthorizationType string            `json:"authorizationType"`
		ApiKeyRequired    bool              `json:"apiKeyRequired"`
		RequestParameters map[string]bool   `json:"requestParameters"`
		RequestModels     map[string]string `json:"requestModels"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	if _, ok := apigwResources.Get(apiId + "/" + resourceId); !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Resource identifier specified")
		return
	}
	m := APIGWMethod{
		HttpMethod:        httpMethod,
		ResourceId:        resourceId,
		RestApiId:         apiId,
		AuthorizationType: req.AuthorizationType,
		ApiKeyRequired:    req.ApiKeyRequired,
		RequestParameters: req.RequestParameters,
		RequestModels:     req.RequestModels,
	}
	apigwMethods.Put(apigwMethodKey(apiId, resourceId, httpMethod), m)
	sim.WriteJSON(w, http.StatusCreated, m)
}

func handleAPIGWGetMethod(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	resourceId := sim.PathParam(r, "resourceId")
	httpMethod := sim.PathParam(r, "httpMethod")
	m, ok := apigwMethods.Get(apigwMethodKey(apiId, resourceId, httpMethod))
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Method identifier specified")
		return
	}
	if in, ok := apigwIntegrations.Get(apigwMethodKey(apiId, resourceId, httpMethod)); ok {
		m.MethodIntegration = &in
	}
	sim.WriteJSON(w, http.StatusOK, m)
}

func handleAPIGWDeleteMethod(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	resourceId := sim.PathParam(r, "resourceId")
	httpMethod := sim.PathParam(r, "httpMethod")
	if !apigwMethods.Delete(apigwMethodKey(apiId, resourceId, httpMethod)) {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Method identifier specified")
		return
	}
	apigwIntegrations.Delete(apigwMethodKey(apiId, resourceId, httpMethod))
	w.WriteHeader(http.StatusAccepted)
}

func handleAPIGWPutIntegration(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	resourceId := sim.PathParam(r, "resourceId")
	httpMethod := sim.PathParam(r, "httpMethod")
	var req struct {
		Type                  string            `json:"type"`
		Uri                   string            `json:"uri"`
		IntegrationHttpMethod string            `json:"httpMethod"`
		RequestTemplates      map[string]string `json:"requestTemplates"`
		RequestParameters     map[string]string `json:"requestParameters"`
		CacheNamespace        string            `json:"cacheNamespace"`
		TimeoutInMillis       int               `json:"timeoutInMillis"`
		PassthroughBehavior   string            `json:"passthroughBehavior"`
		ContentHandling       string            `json:"contentHandling"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	in := APIGWIntegration{
		HttpMethod:            httpMethod,
		ResourceId:            resourceId,
		RestApiId:             apiId,
		Type:                  req.Type,
		Uri:                   req.Uri,
		IntegrationHttpMethod: req.IntegrationHttpMethod,
		RequestTemplates:      req.RequestTemplates,
		RequestParameters:     req.RequestParameters,
		CacheNamespace:        req.CacheNamespace,
		TimeoutInMillis:       req.TimeoutInMillis,
		PassthroughBehavior:   req.PassthroughBehavior,
		ContentHandling:       req.ContentHandling,
	}
	apigwIntegrations.Put(apigwMethodKey(apiId, resourceId, httpMethod), in)
	sim.WriteJSON(w, http.StatusCreated, in)
}

func handleAPIGWGetIntegration(w http.ResponseWriter, r *http.Request) {
	in, ok := apigwIntegrations.Get(apigwMethodKey(sim.PathParam(r, "restApiId"), sim.PathParam(r, "resourceId"), sim.PathParam(r, "httpMethod")))
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Integration identifier specified")
		return
	}
	sim.WriteJSON(w, http.StatusOK, in)
}

func handleAPIGWDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	if !apigwIntegrations.Delete(apigwMethodKey(sim.PathParam(r, "restApiId"), sim.PathParam(r, "resourceId"), sim.PathParam(r, "httpMethod"))) {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Integration identifier specified")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleAPIGWCreateDeployment(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	var req struct {
		Description string `json:"description"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	d := APIGWDeployment{
		Id:          generateUUID()[:10],
		RestApiId:   apiId,
		Description: req.Description,
		CreatedDate: time.Now().Unix(),
	}
	apigwDeployments.Put(apiId+"/"+d.Id, d)
	sim.WriteJSON(w, http.StatusCreated, d)
}

func handleAPIGWGetDeployment(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	deploymentId := sim.PathParam(r, "deploymentId")
	d, ok := apigwDeployments.Get(apigwDeploymentKey(apiId, deploymentId))
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Deployment identifier specified")
		return
	}
	sim.WriteJSON(w, http.StatusOK, d)
}

func handleAPIGWDeleteDeployment(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	deploymentId := sim.PathParam(r, "deploymentId")
	if !apigwDeployments.Delete(apigwDeploymentKey(apiId, deploymentId)) {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Deployment identifier specified")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleAPIGWCreateStage(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	var req struct {
		StageName    string `json:"stageName"`
		DeploymentId string `json:"deploymentId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	s := APIGWStage{
		StageName:    req.StageName,
		RestApiId:    apiId,
		DeploymentId: req.DeploymentId,
		CreatedDate:  time.Now().Unix(),
	}
	apigwStages.Put(apiId+"/"+s.StageName, s)
	sim.WriteJSON(w, http.StatusCreated, s)
}

func handleAPIGWGetStage(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	stageName := sim.PathParam(r, "stageName")
	s, ok := apigwStages.Get(apigwStageKey(apiId, stageName))
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Stage identifier specified")
		return
	}
	sim.WriteJSON(w, http.StatusOK, s)
}

func handleAPIGWDeleteStage(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	stageName := sim.PathParam(r, "stageName")
	if !apigwStages.Delete(apigwStageKey(apiId, stageName)) {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid Stage identifier specified")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func apigwMethodKey(restApiId, resourceId, httpMethod string) string {
	return restApiId + "/" + resourceId + "/" + httpMethod
}

func apigwDeploymentKey(restApiId, deploymentId string) string {
	return restApiId + "/" + deploymentId
}

func apigwStageKey(restApiId, stageName string) string {
	return restApiId + "/" + stageName
}

func apigwChildPath(parentPath, pathPart string) string {
	part := strings.Trim(pathPart, "/")
	if parentPath == "/" {
		return "/" + part
	}
	return strings.TrimRight(parentPath, "/") + "/" + part
}

func apigwMethodResponseKey(restApiId, resourceId, httpMethod, statusCode string) string {
	return restApiId + "/" + resourceId + "/" + httpMethod + "/" + statusCode
}

func handleAPIGWPutMethodResponse(w http.ResponseWriter, r *http.Request) {
	restApiId := sim.PathParam(r, "restApiId")
	resourceId := sim.PathParam(r, "resourceId")
	httpMethod := sim.PathParam(r, "httpMethod")
	statusCode := sim.PathParam(r, "statusCode")
	var req APIGWMethodResponse
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.WriteJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body: " + err.Error()})
		return
	}
	req.StatusCode = statusCode
	key := apigwMethodResponseKey(restApiId, resourceId, httpMethod, statusCode)
	apigwMethodResponses.Put(key, req)
	sim.WriteJSON(w, http.StatusCreated, req)
}

func handleAPIGWGetMethodResponse(w http.ResponseWriter, r *http.Request) {
	key := apigwMethodResponseKey(sim.PathParam(r, "restApiId"), sim.PathParam(r, "resourceId"), sim.PathParam(r, "httpMethod"), sim.PathParam(r, "statusCode"))
	mr, ok := apigwMethodResponses.Get(key)
	if !ok {
		sim.WriteJSON(w, http.StatusNotFound, map[string]string{"message": "method response not found"})
		return
	}
	sim.WriteJSON(w, http.StatusOK, mr)
}

func handleAPIGWDeleteMethodResponse(w http.ResponseWriter, r *http.Request) {
	key := apigwMethodResponseKey(sim.PathParam(r, "restApiId"), sim.PathParam(r, "resourceId"), sim.PathParam(r, "httpMethod"), sim.PathParam(r, "statusCode"))
	if !apigwMethodResponses.Delete(key) {
		sim.WriteJSON(w, http.StatusNotFound, map[string]string{"message": "method response not found"})
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleAPIGWPutIntegrationResponse(w http.ResponseWriter, r *http.Request) {
	restApiId := sim.PathParam(r, "restApiId")
	resourceId := sim.PathParam(r, "resourceId")
	httpMethod := sim.PathParam(r, "httpMethod")
	statusCode := sim.PathParam(r, "statusCode")
	var req APIGWIntegrationResponse
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.WriteJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body: " + err.Error()})
		return
	}
	req.StatusCode = statusCode
	key := apigwMethodResponseKey(restApiId, resourceId, httpMethod, statusCode)
	apigwIntegrationResponses.Put(key, req)
	sim.WriteJSON(w, http.StatusCreated, req)
}

func handleAPIGWGetIntegrationResponse(w http.ResponseWriter, r *http.Request) {
	key := apigwMethodResponseKey(sim.PathParam(r, "restApiId"), sim.PathParam(r, "resourceId"), sim.PathParam(r, "httpMethod"), sim.PathParam(r, "statusCode"))
	ir, ok := apigwIntegrationResponses.Get(key)
	if !ok {
		sim.WriteJSON(w, http.StatusNotFound, map[string]string{"message": "integration response not found"})
		return
	}
	sim.WriteJSON(w, http.StatusOK, ir)
}

func handleAPIGWDeleteIntegrationResponse(w http.ResponseWriter, r *http.Request) {
	key := apigwMethodResponseKey(sim.PathParam(r, "restApiId"), sim.PathParam(r, "resourceId"), sim.PathParam(r, "httpMethod"), sim.PathParam(r, "statusCode"))
	if !apigwIntegrationResponses.Delete(key) {
		sim.WriteJSON(w, http.StatusNotFound, map[string]string{"message": "integration response not found"})
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
