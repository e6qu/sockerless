package main

import (
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// TagsResource mirrors Microsoft.Resources/tags/default — the canonical
// surface for managing tags on any ARM scope (subscription, resource
// group, or individual resource) without having to PUT the full
// resource. Keyed by the lowercased scope resource ID.
type TagsResource struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Properties TagsBody `json:"properties"`
}

// TagsBody is the `properties` envelope returned by every operation
// against /providers/Microsoft.Resources/tags/default.
type TagsBody struct {
	Tags map[string]string `json:"tags"`
}

// TagsPatchRequest is the body of a PATCH against
// /providers/Microsoft.Resources/tags/default.
type TagsPatchRequest struct {
	Operation  string   `json:"operation"`
	Properties TagsBody `json:"properties"`
}

var tagsStore sim.Store[TagsResource]

const tagsDefaultMarker = "/providers/microsoft.resources/tags/default"

func registerTags(srv *sim.Server) {
	tagsStore = sim.MakeStore[TagsResource](srv.DB(), "azure_tags_default")

	// Real Azure's `Microsoft.Resources/tags/default` endpoint sits
	// at the end of any ARM scope path: subscription, resource group,
	// or individual resource. Go 1.22 ServeMux can't match a
	// variable-depth scope prefix with a fixed suffix, so use the
	// WrapHandler middleware approach (same shape as
	// authorization.go's role-definitions dispatcher).
	srv.WrapHandler(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lowerPath := strings.ToLower(r.URL.Path)
			if !strings.HasSuffix(lowerPath, tagsDefaultMarker) {
				next.ServeHTTP(w, r)
				return
			}
			scope := strings.ToLower(strings.TrimSuffix(r.URL.Path, r.URL.Path[len(r.URL.Path)-len(tagsDefaultMarker):]))
			scope = strings.TrimSuffix(scope, "/")
			handleTagsDefault(w, r, scope)
		})
	})
}

func handleTagsDefault(w http.ResponseWriter, r *http.Request, scope string) {
	id := scope + "/providers/Microsoft.Resources/tags/default"
	switch r.Method {
	case http.MethodGet:
		tags, ok := tagsStore.Get(scope)
		if !ok {
			tags = TagsResource{
				ID:         id,
				Name:       "default",
				Type:       "Microsoft.Resources/tags",
				Properties: TagsBody{Tags: map[string]string{}},
			}
		}
		sim.WriteJSON(w, http.StatusOK, tags)
	case http.MethodPut:
		var req TagsResource
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Properties.Tags == nil {
			req.Properties.Tags = map[string]string{}
		}
		stored := TagsResource{
			ID:         id,
			Name:       "default",
			Type:       "Microsoft.Resources/tags",
			Properties: req.Properties,
		}
		tagsStore.Put(scope, stored)
		sim.WriteJSON(w, http.StatusOK, stored)
	case http.MethodPatch:
		var req TagsPatchRequest
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		current, _ := tagsStore.Get(scope)
		if current.Properties.Tags == nil {
			current.Properties.Tags = map[string]string{}
		}
		switch strings.ToLower(req.Operation) {
		case "merge":
			for k, v := range req.Properties.Tags {
				current.Properties.Tags[k] = v
			}
		case "replace":
			tags := map[string]string{}
			for k, v := range req.Properties.Tags {
				tags[k] = v
			}
			current.Properties.Tags = tags
		case "delete":
			for k := range req.Properties.Tags {
				delete(current.Properties.Tags, k)
			}
		default:
			sim.AzureError(w, "InvalidRequestContent",
				"tags PATCH `operation` must be one of Merge, Replace, Delete; got "+req.Operation,
				http.StatusBadRequest)
			return
		}
		stored := TagsResource{
			ID:         id,
			Name:       "default",
			Type:       "Microsoft.Resources/tags",
			Properties: TagsBody{Tags: current.Properties.Tags},
		}
		tagsStore.Put(scope, stored)
		sim.WriteJSON(w, http.StatusOK, stored)
	case http.MethodDelete:
		tagsStore.Delete(scope)
		sim.WriteJSON(w, http.StatusOK, TagsResource{
			ID:         id,
			Name:       "default",
			Type:       "Microsoft.Resources/tags",
			Properties: TagsBody{Tags: map[string]string{}},
		})
	default:
		sim.AzureError(w, "MethodNotAllowed", "method "+r.Method+" not supported on tags/default", http.StatusMethodNotAllowed)
	}
}
