package main

import (
	"net/http"
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
	HttpMethod    string `json:"httpMethod"`
	ResourceId    string `json:"resourceIdRef,omitempty"`
	RestApiId     string `json:"restApiIdRef,omitempty"`
	Authorization string `json:"authorizationType,omitempty"`
}

type APIGWIntegration struct {
	HttpMethod string `json:"httpMethod"`
	ResourceId string `json:"resourceIdRef,omitempty"`
	RestApiId  string `json:"restApiIdRef,omitempty"`
	Type       string `json:"type"`
	Uri        string `json:"uri,omitempty"`
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
	apigwRestApis     sim.Store[APIGWRestApi]
	apigwResources    sim.Store[APIGWResource]
	apigwMethods      sim.Store[APIGWMethod]
	apigwIntegrations sim.Store[APIGWIntegration]
	apigwDeployments  sim.Store[APIGWDeployment]
	apigwStages       sim.Store[APIGWStage]
)

func registerAPIGateway(srv *sim.Server) {
	apigwRestApis = sim.MakeStore[APIGWRestApi](srv.DB(), "apigw_restapis")
	apigwResources = sim.MakeStore[APIGWResource](srv.DB(), "apigw_resources")
	apigwMethods = sim.MakeStore[APIGWMethod](srv.DB(), "apigw_methods")
	apigwIntegrations = sim.MakeStore[APIGWIntegration](srv.DB(), "apigw_integrations")
	apigwDeployments = sim.MakeStore[APIGWDeployment](srv.DB(), "apigw_deployments")
	apigwStages = sim.MakeStore[APIGWStage](srv.DB(), "apigw_stages")

	mux := srv.Mux()
	mux.HandleFunc("POST /restapis", handleAPIGWCreateRestApi)
	mux.HandleFunc("GET /restapis", handleAPIGWListRestApis)
	mux.HandleFunc("GET /restapis/{restApiId}", handleAPIGWGetRestApi)
	mux.HandleFunc("DELETE /restapis/{restApiId}", handleAPIGWDeleteRestApi)
	mux.HandleFunc("POST /restapis/{restApiId}/resources/{parentId}", handleAPIGWCreateResource)
	mux.HandleFunc("GET /restapis/{restApiId}/resources", handleAPIGWListResources)
	mux.HandleFunc("PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}", handleAPIGWPutMethod)
	mux.HandleFunc("PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration", handleAPIGWPutIntegration)
	mux.HandleFunc("POST /restapis/{restApiId}/deployments", handleAPIGWCreateDeployment)
	mux.HandleFunc("POST /restapis/{restApiId}/stages", handleAPIGWCreateStage)
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
	parent, _ := apigwResources.Get(apiId + "/" + parentId)
	res := APIGWResource{
		Id:        generateUUID()[:10],
		RestApiId: apiId,
		ParentId:  parentId,
		PathPart:  req.PathPart,
		Path:      parent.Path + req.PathPart + "/",
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

func handleAPIGWPutMethod(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	resourceId := sim.PathParam(r, "resourceId")
	httpMethod := sim.PathParam(r, "httpMethod")
	var req struct {
		AuthorizationType string `json:"authorizationType"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	m := APIGWMethod{
		HttpMethod:    httpMethod,
		ResourceId:    resourceId,
		RestApiId:     apiId,
		Authorization: req.AuthorizationType,
	}
	apigwMethods.Put(apiId+"/"+resourceId+"/"+httpMethod, m)
	sim.WriteJSON(w, http.StatusCreated, m)
}

func handleAPIGWPutIntegration(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	resourceId := sim.PathParam(r, "resourceId")
	httpMethod := sim.PathParam(r, "httpMethod")
	var req struct {
		Type string `json:"type"`
		Uri  string `json:"uri"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "BadRequestException", err.Error(), http.StatusBadRequest)
		return
	}
	in := APIGWIntegration{
		HttpMethod: httpMethod,
		ResourceId: resourceId,
		RestApiId:  apiId,
		Type:       req.Type,
		Uri:        req.Uri,
	}
	apigwIntegrations.Put(apiId+"/"+resourceId+"/"+httpMethod, in)
	sim.WriteJSON(w, http.StatusCreated, in)
}

func handleAPIGWCreateDeployment(w http.ResponseWriter, r *http.Request) {
	apiId := sim.PathParam(r, "restApiId")
	var req struct {
		Description string `json:"description"`
	}
	_ = sim.ReadJSON(r, &req)
	d := APIGWDeployment{
		Id:          generateUUID()[:10],
		RestApiId:   apiId,
		Description: req.Description,
		CreatedDate: time.Now().Unix(),
	}
	apigwDeployments.Put(apiId+"/"+d.Id, d)
	sim.WriteJSON(w, http.StatusCreated, d)
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
