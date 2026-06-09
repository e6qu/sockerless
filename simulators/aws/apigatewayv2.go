package main

import (
	"net/http"
	"time"

	sim "github.com/sockerless/simulator"
)

// API Gateway v2 (HTTP API + WebSocket) — restJson1 protocol, REST
// path routing under /v2/. Surface scoped to the routes-+-stages
// CRUD that terraform-provider-aws + aws-sdk-go-v2's `apigatewayv2`
// client + the `aws apigatewayv2` CLI exercise for the 90th-
// percentile "deploy an HTTP API + integration + stage" flow.

type APIGWv2Api struct {
	ApiId        string            `json:"apiId"`
	Name         string            `json:"name"`
	ProtocolType string            `json:"protocolType"`
	RouteKey     string            `json:"routeSelectionExpression,omitempty"`
	CreatedDate  string            `json:"createdDate"`
	Tags         map[string]string `json:"tags,omitempty"`
}

// Inner fields whose JSON tag is `apiIdRef` (custom, non-public)
// hold the parent reference for cascade-delete + per-api filtering.
// The SDK ignores unknown fields, so leaking the parent ID on the
// response is harmless and avoids stripping the field during Store
// persistence.
type APIGWv2Route struct {
	RouteId           string `json:"routeId"`
	ApiId             string `json:"apiIdRef,omitempty"`
	RouteKey          string `json:"routeKey"`
	Target            string `json:"target,omitempty"`
	AuthorizationType string `json:"authorizationType,omitempty"`
	ApiKeyRequired    bool   `json:"apiKeyRequired,omitempty"`
	OperationName     string `json:"operationName,omitempty"`
}

type APIGWv2Integration struct {
	IntegrationId        string `json:"integrationId"`
	ApiId                string `json:"apiIdRef,omitempty"`
	IntegrationType      string `json:"integrationType"`
	IntegrationUri       string `json:"integrationUri,omitempty"` // external (operator-supplied): integration target — Lambda ARN, HTTP backend, or VPC link target
	IntegrationMethod    string `json:"integrationMethod,omitempty"`
	PayloadFormatVersion string `json:"payloadFormatVersion,omitempty"`
	TimeoutInMillis      int    `json:"timeoutInMillis,omitempty"`
}

type APIGWv2Stage struct {
	StageName   string `json:"stageName"`
	ApiId       string `json:"apiIdRef,omitempty"`
	Description string `json:"description,omitempty"`
	AutoDeploy  bool   `json:"autoDeploy"`
	CreatedDate string `json:"createdDate"`
}

// APIGWv2Deployment models the snapshot of an HTTP API's routes +
// integrations + stages at a point in time. terraform-provider-aws
// emits `POST /v2/apis/{apiId}/deployments` on `aws_apigatewayv2_deployment`
// — without this route registered the request used to fall through
// to the S3 wildcard dispatcher and 400 with an InvalidRequest envelope.
type APIGWv2Deployment struct {
	DeploymentId     string `json:"deploymentId"`
	ApiId            string `json:"apiIdRef,omitempty"`
	Description      string `json:"description,omitempty"`
	DeploymentStatus string `json:"deploymentStatus"` // DEPLOYED | PENDING | FAILED
	CreatedDate      string `json:"createdDate"`
}

var (
	apigwv2Apis         sim.Store[APIGWv2Api]
	apigwv2Routes       sim.Store[APIGWv2Route]
	apigwv2Integrations sim.Store[APIGWv2Integration]
	apigwv2Stages       sim.Store[APIGWv2Stage]
	apigwv2Deployments  sim.Store[APIGWv2Deployment]
)

func registerAPIGatewayV2(srv *sim.Server) {
	apigwv2Apis = sim.MakeStore[APIGWv2Api](srv.DB(), "apigwv2_apis")
	apigwv2Routes = sim.MakeStore[APIGWv2Route](srv.DB(), "apigwv2_routes")
	apigwv2Integrations = sim.MakeStore[APIGWv2Integration](srv.DB(), "apigwv2_integrations")
	apigwv2Stages = sim.MakeStore[APIGWv2Stage](srv.DB(), "apigwv2_stages")
	apigwv2Deployments = sim.MakeStore[APIGWv2Deployment](srv.DB(), "apigwv2_deployments")

	mux := srv.Mux()
	apiResource := cloudTrailRESTResource("AWS::ApiGatewayV2::Api", "apiId")
	mux.HandleFunc("POST /v2/apis", cloudTrailRecordedREST("CreateApi", "apigateway.amazonaws.com", nil, handleAPIGWv2CreateApi))
	mux.HandleFunc("GET /v2/apis", cloudTrailRecordedREST("GetApis", "apigateway.amazonaws.com", nil, handleAPIGWv2ListApis))
	mux.HandleFunc("GET /v2/apis/{apiId}", cloudTrailRecordedREST("GetApi", "apigateway.amazonaws.com", apiResource, handleAPIGWv2GetApi))
	mux.HandleFunc("DELETE /v2/apis/{apiId}", cloudTrailRecordedREST("DeleteApi", "apigateway.amazonaws.com", apiResource, handleAPIGWv2DeleteApi))
	mux.HandleFunc("POST /v2/apis/{apiId}/routes", cloudTrailRecordedREST("CreateRoute", "apigateway.amazonaws.com", apiResource, handleAPIGWv2CreateRoute))
	mux.HandleFunc("GET /v2/apis/{apiId}/routes", cloudTrailRecordedREST("GetRoutes", "apigateway.amazonaws.com", apiResource, handleAPIGWv2ListRoutes))
	mux.HandleFunc("GET /v2/apis/{apiId}/routes/{routeId}", cloudTrailRecordedREST("GetRoute", "apigateway.amazonaws.com", apiResource, handleAPIGWv2GetRoute))
	mux.HandleFunc("DELETE /v2/apis/{apiId}/routes/{routeId}", cloudTrailRecordedREST("DeleteRoute", "apigateway.amazonaws.com", apiResource, handleAPIGWv2DeleteRoute))
	mux.HandleFunc("POST /v2/apis/{apiId}/integrations", cloudTrailRecordedREST("CreateIntegration", "apigateway.amazonaws.com", apiResource, handleAPIGWv2CreateIntegration))
	mux.HandleFunc("GET /v2/apis/{apiId}/integrations", cloudTrailRecordedREST("GetIntegrations", "apigateway.amazonaws.com", apiResource, handleAPIGWv2ListIntegrations))
	mux.HandleFunc("GET /v2/apis/{apiId}/integrations/{integrationId}", cloudTrailRecordedREST("GetIntegration", "apigateway.amazonaws.com", apiResource, handleAPIGWv2GetIntegration))
	mux.HandleFunc("DELETE /v2/apis/{apiId}/integrations/{integrationId}", cloudTrailRecordedREST("DeleteIntegration", "apigateway.amazonaws.com", apiResource, handleAPIGWv2DeleteIntegration))
	mux.HandleFunc("POST /v2/apis/{apiId}/stages", cloudTrailRecordedREST("CreateStage", "apigateway.amazonaws.com", apiResource, handleAPIGWv2CreateStage))
	mux.HandleFunc("GET /v2/apis/{apiId}/stages", cloudTrailRecordedREST("GetStages", "apigateway.amazonaws.com", apiResource, handleAPIGWv2ListStages))
	mux.HandleFunc("GET /v2/apis/{apiId}/stages/{stageName}", cloudTrailRecordedREST("GetStage", "apigateway.amazonaws.com", apiResource, handleAPIGWv2GetStage))
	mux.HandleFunc("DELETE /v2/apis/{apiId}/stages/{stageName}", cloudTrailRecordedREST("DeleteStage", "apigateway.amazonaws.com", apiResource, handleAPIGWv2DeleteStage))
	mux.HandleFunc("POST /v2/apis/{apiId}/deployments", cloudTrailRecordedREST("CreateDeployment", "apigateway.amazonaws.com", apiResource, handleAPIGWv2CreateDeployment))
	mux.HandleFunc("GET /v2/apis/{apiId}/deployments", cloudTrailRecordedREST("GetDeployments", "apigateway.amazonaws.com", apiResource, handleAPIGWv2ListDeployments))
	mux.HandleFunc("GET /v2/apis/{apiId}/deployments/{deploymentId}", cloudTrailRecordedREST("GetDeployment", "apigateway.amazonaws.com", apiResource, handleAPIGWv2GetDeployment))
	mux.HandleFunc("DELETE /v2/apis/{apiId}/deployments/{deploymentId}", cloudTrailRecordedREST("DeleteDeployment", "apigateway.amazonaws.com", apiResource, handleAPIGWv2DeleteDeployment))
}

func apigwv2StoreKey(apiId, resource string) string { return apiId + "/" + resource }

func handleAPIGWv2CreateApi(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                     string            `json:"Name"`
		ProtocolType             string            `json:"ProtocolType"`
		RouteSelectionExpression string            `json:"RouteSelectionExpression"`
		Tags                     map[string]string `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", "Invalid request body", http.StatusBadRequest)
		return
	}
	api := APIGWv2Api{
		ApiId:        generateUUID()[:10],
		Name:         req.Name,
		ProtocolType: req.ProtocolType,
		RouteKey:     req.RouteSelectionExpression,
		CreatedDate:  time.Now().UTC().Format(time.RFC3339),
		Tags:         req.Tags,
	}
	apigwv2Apis.Put(api.ApiId, api)
	sim.WriteJSON(w, http.StatusCreated, api)
}

func handleAPIGWv2ListApis(w http.ResponseWriter, r *http.Request) {
	all := apigwv2Apis.List()
	if all == nil {
		all = []APIGWv2Api{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"items": all})
}

func handleAPIGWv2GetApi(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	api, ok := apigwv2Apis.Get(apiId)
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid API identifier")
		return
	}
	sim.WriteJSON(w, http.StatusOK, api)
}

func handleAPIGWv2DeleteApi(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	if !apigwv2Apis.Delete(apiId) {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid API identifier")
		return
	}
	// Cascade-delete children.
	for _, rt := range apigwv2Routes.List() {
		if rt.ApiId == apiId {
			apigwv2Routes.Delete(apigwv2StoreKey(apiId, rt.RouteId))
		}
	}
	for _, in := range apigwv2Integrations.List() {
		if in.ApiId == apiId {
			apigwv2Integrations.Delete(apigwv2StoreKey(apiId, in.IntegrationId))
		}
	}
	for _, s := range apigwv2Stages.List() {
		if s.ApiId == apiId {
			apigwv2Stages.Delete(apigwv2StoreKey(apiId, s.StageName))
		}
	}
	for _, d := range apigwv2Deployments.List() {
		if d.ApiId == apiId {
			apigwv2Deployments.Delete(apigwv2StoreKey(apiId, d.DeploymentId))
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleAPIGWv2CreateRoute(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	if _, ok := apigwv2Apis.Get(apiId); !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid API identifier")
		return
	}
	var req struct {
		RouteKey          string `json:"RouteKey"`
		Target            string `json:"Target"`
		AuthorizationType string `json:"AuthorizationType"`
		ApiKeyRequired    bool   `json:"ApiKeyRequired"`
		OperationName     string `json:"OperationName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	route := APIGWv2Route{
		RouteId:           generateUUID()[:10],
		ApiId:             apiId,
		RouteKey:          req.RouteKey,
		Target:            req.Target,
		AuthorizationType: req.AuthorizationType,
		ApiKeyRequired:    req.ApiKeyRequired,
		OperationName:     req.OperationName,
	}
	apigwv2Routes.Put(apigwv2StoreKey(apiId, route.RouteId), route)
	sim.WriteJSON(w, http.StatusCreated, route)
}

func handleAPIGWv2ListRoutes(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	var out []APIGWv2Route
	for _, rt := range apigwv2Routes.List() {
		if rt.ApiId == apiId {
			out = append(out, rt)
		}
	}
	if out == nil {
		out = []APIGWv2Route{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func handleAPIGWv2GetRoute(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	routeId := sim.PathParam(r, "routeId")
	route, ok := apigwv2Routes.Get(apigwv2StoreKey(apiId, routeId))
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound,
			"Invalid Route identifier specified %s", routeId)
		return
	}
	sim.WriteJSON(w, http.StatusOK, route)
}

func handleAPIGWv2DeleteRoute(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	routeId := sim.PathParam(r, "routeId")
	if !apigwv2Routes.Delete(apigwv2StoreKey(apiId, routeId)) {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound,
			"Invalid Route identifier specified %s", routeId)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleAPIGWv2CreateIntegration(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	if _, ok := apigwv2Apis.Get(apiId); !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid API identifier")
		return
	}
	var req struct {
		IntegrationType      string `json:"IntegrationType"`
		IntegrationUri       string `json:"IntegrationUri"`
		IntegrationMethod    string `json:"IntegrationMethod"`
		PayloadFormatVersion string `json:"PayloadFormatVersion"`
		TimeoutInMillis      int    `json:"TimeoutInMillis"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	in := APIGWv2Integration{
		IntegrationId:        generateUUID()[:10],
		ApiId:                apiId,
		IntegrationType:      req.IntegrationType,
		IntegrationUri:       req.IntegrationUri,
		IntegrationMethod:    req.IntegrationMethod,
		PayloadFormatVersion: req.PayloadFormatVersion,
		TimeoutInMillis:      req.TimeoutInMillis,
	}
	apigwv2Integrations.Put(apigwv2StoreKey(apiId, in.IntegrationId), in)
	sim.WriteJSON(w, http.StatusCreated, in)
}

func handleAPIGWv2ListIntegrations(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	var out []APIGWv2Integration
	for _, in := range apigwv2Integrations.List() {
		if in.ApiId == apiId {
			out = append(out, in)
		}
	}
	if out == nil {
		out = []APIGWv2Integration{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func handleAPIGWv2GetIntegration(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	integrationId := sim.PathParam(r, "integrationId")
	in, ok := apigwv2Integrations.Get(apigwv2StoreKey(apiId, integrationId))
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound,
			"Invalid Integration identifier specified %s", integrationId)
		return
	}
	sim.WriteJSON(w, http.StatusOK, in)
}

func handleAPIGWv2DeleteIntegration(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	integrationId := sim.PathParam(r, "integrationId")
	if !apigwv2Integrations.Delete(apigwv2StoreKey(apiId, integrationId)) {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound,
			"Invalid Integration identifier specified %s", integrationId)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleAPIGWv2CreateStage(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	if _, ok := apigwv2Apis.Get(apiId); !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid API identifier")
		return
	}
	var req struct {
		StageName   string `json:"StageName"`
		Description string `json:"Description"`
		AutoDeploy  bool   `json:"AutoDeploy"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	s := APIGWv2Stage{
		StageName:   req.StageName,
		ApiId:       apiId,
		Description: req.Description,
		AutoDeploy:  req.AutoDeploy,
		CreatedDate: time.Now().UTC().Format(time.RFC3339),
	}
	apigwv2Stages.Put(apigwv2StoreKey(apiId, s.StageName), s)
	sim.WriteJSON(w, http.StatusCreated, s)
}

func handleAPIGWv2ListStages(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	var out []APIGWv2Stage
	for _, s := range apigwv2Stages.List() {
		if s.ApiId == apiId {
			out = append(out, s)
		}
	}
	if out == nil {
		out = []APIGWv2Stage{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func handleAPIGWv2GetStage(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	stageName := sim.PathParam(r, "stageName")
	stage, ok := apigwv2Stages.Get(apigwv2StoreKey(apiId, stageName))
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound,
			"Invalid Stage identifier specified %s", stageName)
		return
	}
	sim.WriteJSON(w, http.StatusOK, stage)
}

func handleAPIGWv2DeleteStage(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	stageName := sim.PathParam(r, "stageName")
	if !apigwv2Stages.Delete(apigwv2StoreKey(apiId, stageName)) {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound,
			"Invalid Stage identifier specified %s", stageName)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleAPIGWv2CreateDeployment(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	if _, ok := apigwv2Apis.Get(apiId); !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound, "Invalid API identifier")
		return
	}
	var req struct {
		Description string `json:"Description"`
		StageName   string `json:"StageName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	d := APIGWv2Deployment{
		DeploymentId:     generateUUID()[:10],
		ApiId:            apiId,
		Description:      req.Description,
		DeploymentStatus: "DEPLOYED",
		CreatedDate:      time.Now().UTC().Format(time.RFC3339),
	}
	apigwv2Deployments.Put(apigwv2StoreKey(apiId, d.DeploymentId), d)
	sim.WriteJSON(w, http.StatusCreated, d)
}

func handleAPIGWv2ListDeployments(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	var out []APIGWv2Deployment
	for _, d := range apigwv2Deployments.List() {
		if d.ApiId == apiId {
			out = append(out, d)
		}
	}
	if out == nil {
		out = []APIGWv2Deployment{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func handleAPIGWv2GetDeployment(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	deploymentId := sim.PathParam(r, "deploymentId")
	d, ok := apigwv2Deployments.Get(apigwv2StoreKey(apiId, deploymentId))
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound,
			"Invalid Deployment identifier specified %s", deploymentId)
		return
	}
	sim.WriteJSON(w, http.StatusOK, d)
}

func handleAPIGWv2DeleteDeployment(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "apiId")
	deploymentId := sim.PathParam(r, "deploymentId")
	if !apigwv2Deployments.Delete(apigwv2StoreKey(apiId, deploymentId)) {
		sim.AWSErrorf(w, "NotFoundException", http.StatusNotFound,
			"Invalid Deployment identifier specified %s", deploymentId)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
