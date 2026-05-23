package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Microsoft.ApiManagement ARM control plane. Real Azure exposes
// ~60 ops across Service / Apis / Operations / Products /
// Subscriptions / Backends / NamedValues / Policy. The sim
// implements the Service + Api + Product + Subscription slice —
// sufficient for terraform-provider-azurerm `azurerm_api_management*`
// resources.

type APIMService struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location,omitempty"`
	Sku        map[string]any    `json:"sku,omitempty"`
	Properties map[string]any    `json:"properties,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
}

type APIMApi struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

type APIMProduct struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

type APIMSubscription struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

var (
	apimServices      sim.Store[APIMService]
	apimApis          sim.Store[APIMApi]
	apimProducts      sim.Store[APIMProduct]
	apimSubscriptions sim.Store[APIMSubscription]
)

func registerAPIM(srv *sim.Server) {
	apimServices = sim.MakeStore[APIMService](srv.DB(), "apim_services")
	apimApis = sim.MakeStore[APIMApi](srv.DB(), "apim_apis")
	apimProducts = sim.MakeStore[APIMProduct](srv.DB(), "apim_products")
	apimSubscriptions = sim.MakeStore[APIMSubscription](srv.DB(), "apim_subscriptions")

	const base = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ApiManagement/service"

	srv.HandleFunc("PUT "+base+"/{name}", handleAPIMCreateService)
	srv.HandleFunc("GET "+base+"/{name}", handleAPIMGetService)
	srv.HandleFunc("DELETE "+base+"/{name}", handleAPIMDeleteService)
	srv.HandleFunc("GET "+base, handleAPIMListServicesByRG)

	srv.HandleFunc("PUT "+base+"/{name}/apis/{api}", handleAPIMCreateApi)
	srv.HandleFunc("GET "+base+"/{name}/apis/{api}", handleAPIMGetApi)
	srv.HandleFunc("DELETE "+base+"/{name}/apis/{api}", handleAPIMDeleteApi)
	srv.HandleFunc("GET "+base+"/{name}/apis", handleAPIMListApis)

	srv.HandleFunc("PUT "+base+"/{name}/products/{product}", handleAPIMCreateProduct)
	srv.HandleFunc("GET "+base+"/{name}/products/{product}", handleAPIMGetProduct)
	srv.HandleFunc("DELETE "+base+"/{name}/products/{product}", handleAPIMDeleteProduct)
	srv.HandleFunc("GET "+base+"/{name}/products", handleAPIMListProducts)

	srv.HandleFunc("PUT "+base+"/{name}/subscriptions/{sub}", handleAPIMCreateSubscription)
	srv.HandleFunc("GET "+base+"/{name}/subscriptions/{sub}", handleAPIMGetSubscription)
	srv.HandleFunc("DELETE "+base+"/{name}/subscriptions/{sub}", handleAPIMDeleteSubscription)
	srv.HandleFunc("GET "+base+"/{name}/subscriptions", handleAPIMListSubscriptions)
}

func apimServiceID(sub, rg, name string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ApiManagement/service/%s", sub, rg, name)
}

func handleAPIMCreateService(w http.ResponseWriter, r *http.Request) {
	sub := sim.PathParam(r, "subscriptionId")
	rg := sim.PathParam(r, "resourceGroupName")
	name := sim.PathParam(r, "name")
	var req APIMService
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := apimServiceID(sub, rg, name)
	s := APIMService{
		ID:       id,
		Name:     name,
		Type:     "Microsoft.ApiManagement/service",
		Location: req.Location,
		Sku:      req.Sku,
		Tags:     req.Tags,
		Properties: map[string]any{
			"provisioningState": "Succeeded",
			"gatewayUrl":        "https://" + name + ".azure-api.net",
			"portalUrl":         "https://" + name + ".portal.azure-api.net",
			"managementApiUrl":  "https://" + name + ".management.azure-api.net",
		},
	}
	if req.Properties != nil {
		for k, v := range req.Properties {
			s.Properties[k] = v
		}
		s.Properties["provisioningState"] = "Succeeded"
	}
	if s.Sku == nil {
		s.Sku = map[string]any{"name": "Developer", "capacity": 1}
	}
	apimServices.Put(id, s)
	sim.WriteJSON(w, http.StatusOK, s)
}

func handleAPIMGetService(w http.ResponseWriter, r *http.Request) {
	id := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	s, ok := apimServices.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "service not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, s)
}

func handleAPIMDeleteService(w http.ResponseWriter, r *http.Request) {
	id := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if !apimServices.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "service not found")
		return
	}
	prefix := id + "/"
	for _, a := range apimApis.List() {
		if strings.HasPrefix(a.ID, prefix) {
			apimApis.Delete(a.ID)
		}
	}
	for _, p := range apimProducts.List() {
		if strings.HasPrefix(p.ID, prefix) {
			apimProducts.Delete(p.ID)
		}
	}
	for _, s := range apimSubscriptions.List() {
		if strings.HasPrefix(s.ID, prefix) {
			apimSubscriptions.Delete(s.ID)
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleAPIMListServicesByRG(w http.ResponseWriter, r *http.Request) {
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ApiManagement/service/",
		sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"))
	var out []APIMService
	for _, s := range apimServices.List() {
		if strings.HasPrefix(s.ID, prefix) {
			out = append(out, s)
		}
	}
	if out == nil {
		out = []APIMService{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleAPIMCreateApi(w http.ResponseWriter, r *http.Request) {
	parent := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if _, ok := apimServices.Get(parent); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "service not found")
		return
	}
	apiName := sim.PathParam(r, "api")
	var req APIMApi
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := parent + "/apis/" + apiName
	a := APIMApi{
		ID: id, Name: apiName, Type: "Microsoft.ApiManagement/service/apis",
		Properties: map[string]any{"path": apiName},
	}
	if req.Properties != nil {
		for k, v := range req.Properties {
			a.Properties[k] = v
		}
	}
	apimApis.Put(id, a)
	sim.WriteJSON(w, http.StatusOK, a)
}

func handleAPIMGetApi(w http.ResponseWriter, r *http.Request) {
	id := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/apis/" + sim.PathParam(r, "api")
	a, ok := apimApis.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "api not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, a)
}

func handleAPIMDeleteApi(w http.ResponseWriter, r *http.Request) {
	id := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/apis/" + sim.PathParam(r, "api")
	if !apimApis.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "api not found")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleAPIMListApis(w http.ResponseWriter, r *http.Request) {
	prefix := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) + "/apis/"
	var out []APIMApi
	for _, a := range apimApis.List() {
		if strings.HasPrefix(a.ID, prefix) {
			out = append(out, a)
		}
	}
	if out == nil {
		out = []APIMApi{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleAPIMCreateProduct(w http.ResponseWriter, r *http.Request) {
	parent := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if _, ok := apimServices.Get(parent); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "service not found")
		return
	}
	pName := sim.PathParam(r, "product")
	var req APIMProduct
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := parent + "/products/" + pName
	p := APIMProduct{
		ID: id, Name: pName, Type: "Microsoft.ApiManagement/service/products",
		Properties: map[string]any{"displayName": pName, "state": "published"},
	}
	if req.Properties != nil {
		for k, v := range req.Properties {
			p.Properties[k] = v
		}
	}
	apimProducts.Put(id, p)
	sim.WriteJSON(w, http.StatusOK, p)
}

func handleAPIMGetProduct(w http.ResponseWriter, r *http.Request) {
	id := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/products/" + sim.PathParam(r, "product")
	p, ok := apimProducts.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "product not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, p)
}

func handleAPIMDeleteProduct(w http.ResponseWriter, r *http.Request) {
	id := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/products/" + sim.PathParam(r, "product")
	if !apimProducts.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "product not found")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleAPIMListProducts(w http.ResponseWriter, r *http.Request) {
	prefix := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) + "/products/"
	var out []APIMProduct
	for _, p := range apimProducts.List() {
		if strings.HasPrefix(p.ID, prefix) {
			out = append(out, p)
		}
	}
	if out == nil {
		out = []APIMProduct{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleAPIMCreateSubscription(w http.ResponseWriter, r *http.Request) {
	parent := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name"))
	if _, ok := apimServices.Get(parent); !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "service not found")
		return
	}
	sName := sim.PathParam(r, "sub")
	var req APIMSubscription
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AzureErrorf(w, "BadRequest", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	id := parent + "/subscriptions/" + sName
	s := APIMSubscription{
		ID: id, Name: sName, Type: "Microsoft.ApiManagement/service/subscriptions",
		Properties: map[string]any{"state": "active"},
	}
	if req.Properties != nil {
		for k, v := range req.Properties {
			s.Properties[k] = v
		}
	}
	apimSubscriptions.Put(id, s)
	sim.WriteJSON(w, http.StatusOK, s)
}

func handleAPIMGetSubscription(w http.ResponseWriter, r *http.Request) {
	id := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/subscriptions/" + sim.PathParam(r, "sub")
	s, ok := apimSubscriptions.Get(id)
	if !ok {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "subscription not found")
		return
	}
	sim.WriteJSON(w, http.StatusOK, s)
}

func handleAPIMDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	id := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) +
		"/subscriptions/" + sim.PathParam(r, "sub")
	if !apimSubscriptions.Delete(id) {
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound, "subscription not found")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func handleAPIMListSubscriptions(w http.ResponseWriter, r *http.Request) {
	prefix := apimServiceID(sim.PathParam(r, "subscriptionId"), sim.PathParam(r, "resourceGroupName"), sim.PathParam(r, "name")) + "/subscriptions/"
	var out []APIMSubscription
	for _, s := range apimSubscriptions.List() {
		if strings.HasPrefix(s.ID, prefix) {
			out = append(out, s)
		}
	}
	if out == nil {
		out = []APIMSubscription{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}
