package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Registry represents an Azure Container Registry.
type Registry struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	Location   string             `json:"location"`
	Sku        *RegistrySku       `json:"sku,omitempty"`
	Tags       map[string]string  `json:"tags,omitempty"`
	Properties RegistryProperties `json:"properties"`
}

// RegistrySku holds the SKU for a container registry.
type RegistrySku struct {
	Name string `json:"name"`
	Tier string `json:"tier,omitempty"`
}

// RegistryProperties holds the properties of a container registry.
type RegistryProperties struct {
	LoginServer         string `json:"loginServer"`
	ProvisioningState   string `json:"provisioningState"`
	AdminUserEnabled    bool   `json:"adminUserEnabled"`
	PublicNetworkAccess string `json:"publicNetworkAccess,omitempty"`
	ZoneRedundancy      string `json:"zoneRedundancy,omitempty"`
}

// ACRCacheRule models an Azure Container Registry cache rule
// (pull-through cache) as returned by the `cacheRules` sub-resource.
// Sockerless and terraform callers register one rule per registered
// upstream (e.g., `docker-hub` → `docker.io/library/*`) so Docker
// Hub references can be rewritten to
// `<acrName>.azurecr.io/<targetRepository>:<tag>` at container launch.
type ACRCacheRule struct {
	ID         string                 `json:"id,omitempty"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type,omitempty"`
	Properties ACRCacheRuleProperties `json:"properties"`
}

// ACRCacheRuleProperties mirrors armcontainerregistry.CacheRuleProperties.
// SourceRepository is the upstream ref (e.g. `docker.io/library/alpine`);
// TargetRepository is the local ACR path (e.g. `docker-hub/library/alpine`).
type ACRCacheRuleProperties struct {
	CredentialSetResourceID string `json:"credentialSetResourceId,omitempty"`
	SourceRepository        string `json:"sourceRepository,omitempty"`
	TargetRepository        string `json:"targetRepository,omitempty"`
	CreationDate            string `json:"creationDate,omitempty"`
	ProvisioningState       string `json:"provisioningState,omitempty"`
}

// Package-level store for dashboard access.
var acrRegistries sim.Store[Registry]

func registerACR(srv *sim.Server) {
	registries := sim.MakeStore[Registry](srv.DB(), "acr_registries")
	acrRegistries = registries
	// OCI Distribution data plane (shared registry library). ACR has no
	// pull-through hydration here; the catalog API below reads reg.Manifests.
	reg := &sim.OCIRegistry{
		Manifests: sim.MakeStore[sim.OCIManifest](srv.DB(), "acr_manifests"),
		Blobs:     sim.MakeStore[sim.OCIBlob](srv.DB(), "acr_blobs"),
		Uploads:   sim.MakeStore[sim.OCIUpload](srv.DB(), "acr_uploads"),
	}
	// cacheRules stores pull-through cache rules keyed by ARM resource ID.
	cacheRules := sim.MakeStore[ACRCacheRule](srv.DB(), "acr_cache_rules")

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerRegistry"

	// POST - Check name availability (azurerm v3 calls this before
	// creating a registry). Lowercase registration; the middleware
	// canonicalizes camelCase → lowercase before dispatch.
	srv.HandleFunc("POST /subscriptions/{subscriptionId}/providers/Microsoft.ContainerRegistry/checknameavailability", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"nameAvailable": true,
		})
	})

	// PUT - Create or update registry
	srv.HandleFunc("PUT "+armBase+"/registries/{registryName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "registryName")

		var req Registry
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Location == "" {
			sim.AzureError(w, "InvalidRequestContent", "The 'location' property is required.", http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s", sub, rg, name)

		sku := req.Sku
		if sku == nil {
			sku = &RegistrySku{Name: "Basic", Tier: "Basic"}
		}

		reg := Registry{
			ID:       resourceID,
			Name:     name,
			Type:     "Microsoft.ContainerRegistry/registries",
			Location: req.Location,
			Sku:      sku,
			Tags:     req.Tags,
			Properties: RegistryProperties{
				LoginServer:         strings.ToLower(name) + ".azurecr.io",
				ProvisioningState:   "Succeeded",
				AdminUserEnabled:    req.Properties.AdminUserEnabled,
				PublicNetworkAccess: "Enabled",
				ZoneRedundancy:      "Disabled",
			},
		}

		registries.Put(resourceID, reg)

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, reg)
	})

	// GET - Get registry
	srv.HandleFunc("GET "+armBase+"/registries/{registryName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "registryName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s", sub, rg, name)

		reg, ok := registries.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.ContainerRegistry/registries/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		sim.WriteJSON(w, http.StatusOK, reg)
	})

	// GET - List replications (azurerm provider reads this after creating a registry)
	srv.HandleFunc("GET "+armBase+"/registries/{registryName}/replications", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "registryName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s", sub, rg, name)
		if _, ok := registries.Get(resourceID); !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.ContainerRegistry/registries/%s' under resource group '%s' was not found.", name, rg)
			return
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": []any{},
		})
	})

	// DELETE - Delete registry
	srv.HandleFunc("DELETE "+armBase+"/registries/{registryName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "registryName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s", sub, rg, name)

		if registries.Delete(resourceID) {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// --- Cache Rules (pull-through cache) ---
	//
	// Matches armcontainerregistry.CacheRulesClient endpoints. Reference:
	// subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.ContainerRegistry
	//   registries/{registry}/cacheRules[/{rule}]
	// BeginCreate accepts 200/201 (we return 200 sync with final body).
	// BeginDelete accepts 202/204 (we return 204 sync).
	// Parallels the AWS ECR pull-through + GCP Artifact Registry slices.

	// PUT cache rule (Create or Update — LRO collapsed to sync 200).
	srv.HandleFunc("PUT "+armBase+"/registries/{registryName}/cacheRules/{cacheRuleName}",
		func(w http.ResponseWriter, r *http.Request) {
			sub := sim.PathParam(r, "subscriptionId")
			rg := sim.PathParam(r, "resourceGroupName")
			regName := sim.PathParam(r, "registryName")
			ruleName := sim.PathParam(r, "cacheRuleName")

			regID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s",
				sub, rg, regName)
			if _, ok := registries.Get(regID); !ok {
				sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
					"Registry '%s' under resource group '%s' was not found.", regName, rg)
				return
			}

			var req ACRCacheRule
			if err := sim.ReadJSON(r, &req); err != nil {
				sim.AzureError(w, "InvalidRequestContent",
					"Failed to parse request body: "+err.Error(), http.StatusBadRequest)
				return
			}
			if req.Properties.SourceRepository == "" || req.Properties.TargetRepository == "" {
				sim.AzureError(w, "InvalidRequestContent",
					"properties.sourceRepository and properties.targetRepository are required",
					http.StatusBadRequest)
				return
			}

			ruleID := fmt.Sprintf("%s/cacheRules/%s", regID, ruleName)
			rule := ACRCacheRule{
				ID:   ruleID,
				Name: ruleName,
				Type: "Microsoft.ContainerRegistry/registries/cacheRules",
				Properties: ACRCacheRuleProperties{
					CredentialSetResourceID: req.Properties.CredentialSetResourceID,
					SourceRepository:        req.Properties.SourceRepository,
					TargetRepository:        req.Properties.TargetRepository,
					CreationDate:            req.Properties.CreationDate,
					ProvisioningState:       "Succeeded",
				},
			}
			cacheRules.Put(ruleID, rule)

			sim.WriteJSON(w, http.StatusOK, rule)
		})

	// GET cache rule.
	srv.HandleFunc("GET "+armBase+"/registries/{registryName}/cacheRules/{cacheRuleName}",
		func(w http.ResponseWriter, r *http.Request) {
			sub := sim.PathParam(r, "subscriptionId")
			rg := sim.PathParam(r, "resourceGroupName")
			regName := sim.PathParam(r, "registryName")
			ruleName := sim.PathParam(r, "cacheRuleName")

			ruleID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s/cacheRules/%s",
				sub, rg, regName, ruleName)
			rule, ok := cacheRules.Get(ruleID)
			if !ok {
				sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
					"Cache rule '%s' under registry '%s' was not found.", ruleName, regName)
				return
			}
			sim.WriteJSON(w, http.StatusOK, rule)
		})

	// LIST cache rules under a registry.
	srv.HandleFunc("GET "+armBase+"/registries/{registryName}/cacheRules",
		func(w http.ResponseWriter, r *http.Request) {
			sub := sim.PathParam(r, "subscriptionId")
			rg := sim.PathParam(r, "resourceGroupName")
			regName := sim.PathParam(r, "registryName")

			regPrefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s/cacheRules/",
				sub, rg, regName)
			matched := cacheRules.Filter(func(cr ACRCacheRule) bool {
				return strings.HasPrefix(cr.ID, regPrefix)
			})
			if matched == nil {
				matched = []ACRCacheRule{}
			}
			sim.WriteJSON(w, http.StatusOK, map[string]any{
				"value": matched,
			})
		})

	// DELETE cache rule.
	srv.HandleFunc("DELETE "+armBase+"/registries/{registryName}/cacheRules/{cacheRuleName}",
		func(w http.ResponseWriter, r *http.Request) {
			sub := sim.PathParam(r, "subscriptionId")
			rg := sim.PathParam(r, "resourceGroupName")
			regName := sim.PathParam(r, "registryName")
			ruleName := sim.PathParam(r, "cacheRuleName")

			ruleID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s/cacheRules/%s",
				sub, rg, regName, ruleName)
			cacheRules.Delete(ruleID)
			w.WriteHeader(http.StatusNoContent)
		})

	// OCI Distribution data plane — mounted from the shared registry library.
	reg.Register(srv)

	// GET /acr/v1/_catalog - List all repositories (ACR data-plane catalog API)
	srv.HandleFunc("GET /acr/v1/_catalog", func(w http.ResponseWriter, r *http.Request) {
		all := reg.Manifests.List()
		seen := map[string]bool{}
		var repos []string
		for _, m := range all {
			if m.Repo != "" && !seen[m.Repo] {
				seen[m.Repo] = true
				repos = append(repos, m.Repo)
			}
		}
		page, last := acrCatalogPage(r, repos)
		if page == nil {
			page = []string{}
		}
		if last != "" {
			q := r.URL.Query()
			q.Set("last", last)
			link := fmt.Sprintf("</acr/v1/_catalog?%s>; rel=\"next\"", q.Encode())
			w.Header().Set("Link", link)
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"repositories": page,
		})
	})

	// GET /acr/v1/{name}/_tags - List tags for a repository (ACR data-plane tags API)
	// {name} can contain slashes (e.g. "myrepo/myimage"), so matched via {path...}.
	srv.HandleFunc("GET /acr/v1/{path...}", func(w http.ResponseWriter, r *http.Request) {
		fullPath := sim.PathParam(r, "path")
		const tagsSuffix = "/_tags"
		if !strings.HasSuffix(fullPath, tagsSuffix) {
			http.NotFound(w, r)
			return
		}
		repoName := fullPath[:len(fullPath)-len(tagsSuffix)]
		if repoName == "" {
			http.NotFound(w, r)
			return
		}
		tags := reg.Manifests.Filter(func(m sim.OCIManifest) bool {
			return m.Repo == repoName && m.Ref != "" && !strings.HasPrefix(m.Ref, "sha256:")
		})
		tagList := make([]map[string]any, 0, len(tags))
		for _, m := range tags {
			tagList = append(tagList, map[string]any{
				"name":   m.Ref,
				"digest": m.Digest,
				"changeableAttributes": map[string]any{
					"deleteEnabled": true,
					"writeEnabled":  true,
					"readEnabled":   true,
					"listEnabled":   true,
				},
			})
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"imageName": repoName,
			"tags":      tagList,
		})
	})

}
