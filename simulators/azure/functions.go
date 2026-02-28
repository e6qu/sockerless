package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Site represents an Azure Function App (Web App).
type Site struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Kind       string            `json:"kind,omitempty"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties SiteProperties    `json:"properties"`
}

// SiteProperties holds the properties of a function app.
type SiteProperties struct {
	State            string      `json:"state,omitempty"`
	DefaultHostName  string      `json:"defaultHostName,omitempty"`
	HostNames        []string    `json:"hostNames,omitempty"`
	Enabled          bool        `json:"enabled"`
	EnabledHostNames []string    `json:"enabledHostNames,omitempty"`
	ServerFarmID     string      `json:"serverFarmId,omitempty"`
	Reserved         bool        `json:"reserved,omitempty"`
	SiteConfig       *SiteConfig `json:"siteConfig,omitempty"`
	ResourceGroup    string      `json:"resourceGroup,omitempty"`
	LastModifiedTime string      `json:"lastModifiedTimeUtc,omitempty"`
	HTTPSOnly        bool        `json:"httpsOnly,omitempty"`
}

// SiteConfig holds the site configuration for a function app.
type SiteConfig struct {
	AppSettings          []NameValuePair `json:"appSettings,omitempty"`
	LinuxFxVersion       string          `json:"linuxFxVersion,omitempty"`
	FunctionAppScaleLimit int            `json:"functionAppScaleLimit,omitempty"`
	FtpsState            string          `json:"ftpsState,omitempty"`
}

// NameValuePair holds a name-value pair for app settings.
type NameValuePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FunctionEnvelope represents a function within a function app.
type FunctionEnvelope struct {
	ID         string                    `json:"id"`
	Name       string                    `json:"name"`
	Type       string                    `json:"type"`
	Properties FunctionEnvelopeProperties `json:"properties"`
}

// FunctionEnvelopeProperties holds the properties of a function.
type FunctionEnvelopeProperties struct {
	Name           string         `json:"name"`
	FunctionAppID  string         `json:"function_app_id,omitempty"`
	ScriptHref     string         `json:"script_href,omitempty"`
	ConfigHref     string         `json:"config_href,omitempty"`
	Href           string         `json:"href,omitempty"`
	Config         map[string]any `json:"config,omitempty"`
	InvokeURLTemplate string      `json:"invoke_url_template,omitempty"`
	Language       string         `json:"language,omitempty"`
	IsDisabled     bool           `json:"isDisabled"`
}

func registerAzureFunctions(srv *sim.Server) {
	sites := sim.NewStateStore[Site]()
	functionConfigs := sim.NewStateStore[FunctionEnvelope]()

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Web"

	// PUT - Create or update function app
	srv.HandleFunc("PUT "+armBase+"/sites/{siteName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "siteName")

		var req Site
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Location == "" {
			sim.AzureError(w, "InvalidRequestContent", "The 'location' property is required.", http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Web/sites/%s", sub, rg, name)

		kind := req.Kind
		if kind == "" {
			kind = "functionapp"
		}

		// Use the simulator's own host as the default hostname so function
		// invocations in simulator mode route back to us.
		defaultHostName := r.Host

		site := Site{
			ID:       resourceID,
			Name:     name,
			Type:     "Microsoft.Web/sites",
			Kind:     kind,
			Location: req.Location,
			Tags:     req.Tags,
			Properties: SiteProperties{
				State:            "Running",
				DefaultHostName:  defaultHostName,
				HostNames:        []string{defaultHostName},
				Enabled:          true,
				EnabledHostNames: []string{defaultHostName, name + ".scm.azurewebsites.net"},
				ServerFarmID:     req.Properties.ServerFarmID,
				Reserved:         req.Properties.Reserved,
				SiteConfig:       req.Properties.SiteConfig,
				ResourceGroup:    rg,
				LastModifiedTime: time.Now().UTC().Format(time.RFC3339),
				HTTPSOnly:        req.Properties.HTTPSOnly,
			},
		}

		sites.Put(resourceID, site)

		// Always return 200 OK so the ARM SDK's BeginCreateOrUpdate poller
		// treats this as an immediately completed operation.
		sim.WriteJSON(w, http.StatusOK, site)
	})

	// GET - Get function app
	srv.HandleFunc("GET "+armBase+"/sites/{siteName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "siteName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Web/sites/%s", sub, rg, name)

		site, ok := sites.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.Web/sites/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		sim.WriteJSON(w, http.StatusOK, site)
	})

	// DELETE - Delete function app
	srv.HandleFunc("DELETE "+armBase+"/sites/{siteName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "siteName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Web/sites/%s", sub, rg, name)

		if sites.Delete(resourceID) {
			// Clean up associated functions
			funcs := functionConfigs.Filter(func(f FunctionEnvelope) bool {
				return strings.HasPrefix(f.ID, resourceID+"/functions/")
			})
			for _, f := range funcs {
				functionConfigs.Delete(f.ID)
			}

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// GET - List functions
	srv.HandleFunc("GET "+armBase+"/sites/{siteName}/functions", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "siteName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Web/sites/%s", sub, rg, name)

		// Verify site exists
		if _, ok := sites.Get(resourceID); !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.Web/sites/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		filtered := functionConfigs.Filter(func(f FunctionEnvelope) bool {
			return strings.HasPrefix(f.ID, resourceID+"/functions/")
		})

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": filtered,
		})
	})

	// GET - Get function
	srv.HandleFunc("GET "+armBase+"/sites/{siteName}/functions/{functionName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		siteName := sim.PathParam(r, "siteName")
		funcName := sim.PathParam(r, "functionName")

		funcID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Web/sites/%s/functions/%s",
			sub, rg, siteName, funcName)

		fn, ok := functionConfigs.Get(funcID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The function '%s' in site '%s' was not found.", funcName, siteName)
			return
		}

		sim.WriteJSON(w, http.StatusOK, fn)
	})

	// POST - Invoke function (simulator-only endpoint for function URL invocation)
	srv.HandleFunc("POST /api/function", func(w http.ResponseWriter, r *http.Request) {
		// Find the site that matches this request's Host header.
		// The backend constructs the function URL using the site's DefaultHostName.
		host := r.Host
		var matchedSite *Site
		for _, s := range sites.List() {
			if s.Properties.DefaultHostName == host {
				s := s // copy
				matchedSite = &s
				break
			}
		}

		// Inject log entry for the invocation
		if matchedSite != nil {
			injectAppTrace(matchedSite.Name, "Function invoked")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	})
}

