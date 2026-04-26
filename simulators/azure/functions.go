package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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
	State                string                            `json:"state,omitempty"`
	DefaultHostName      string                            `json:"defaultHostName,omitempty"`
	HostNames            []string                          `json:"hostNames,omitempty"`
	Enabled              bool                              `json:"enabled"`
	EnabledHostNames     []string                          `json:"enabledHostNames,omitempty"`
	ServerFarmID         string                            `json:"serverFarmId,omitempty"`
	Reserved             bool                              `json:"reserved,omitempty"`
	SiteConfig           *SiteConfig                       `json:"siteConfig,omitempty"`
	ResourceGroup        string                            `json:"resourceGroup,omitempty"`
	LastModifiedTime     string                            `json:"lastModifiedTimeUtc,omitempty"`
	HTTPSOnly            bool                              `json:"httpsOnly,omitempty"`
	AzureStorageAccounts map[string]*AzureStorageInfoValue `json:"-"`
}

// SiteConfig holds the site configuration for a function app.
type SiteConfig struct {
	AppSettings           []NameValuePair `json:"appSettings,omitempty"`
	LinuxFxVersion        string          `json:"linuxFxVersion,omitempty"`
	FunctionAppScaleLimit int             `json:"functionAppScaleLimit,omitempty"`
	FtpsState             string          `json:"ftpsState,omitempty"`
	SimCommand            []string        `json:"simCommand,omitempty"` // Simulator-only: command to execute on invoke
}

// NameValuePair holds a name-value pair for app settings.
type NameValuePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FunctionEnvelope represents a function within a function app.
type FunctionEnvelope struct {
	ID         string                     `json:"id"`
	Name       string                     `json:"name"`
	Type       string                     `json:"type"`
	Properties FunctionEnvelopeProperties `json:"properties"`
}

// FunctionEnvelopeProperties holds the properties of a function.
type FunctionEnvelopeProperties struct {
	Name              string         `json:"name"`
	FunctionAppID     string         `json:"function_app_id,omitempty"`
	ScriptHref        string         `json:"script_href,omitempty"`
	ConfigHref        string         `json:"config_href,omitempty"`
	Href              string         `json:"href,omitempty"`
	Config            map[string]any `json:"config,omitempty"`
	InvokeURLTemplate string         `json:"invoke_url_template,omitempty"`
	Language          string         `json:"language,omitempty"`
	IsDisabled        bool           `json:"isDisabled"`
}

// Package-level store for dashboard access.
var azfSites sim.Store[Site]

func registerAzureFunctions(srv *sim.Server) {
	sites := sim.MakeStore[Site](srv.DB(), "azf_sites")
	azfSites = sites
	functionConfigs := sim.MakeStore[FunctionEnvelope](srv.DB(), "azf_function_configs")

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

	// GET - List function apps by resource group
	srv.HandleFunc("GET "+armBase+"/sites", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Web/sites/", sub, rg)

		filtered := sites.Filter(func(s Site) bool {
			return strings.HasPrefix(s.ID, prefix)
		})

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": filtered,
		})
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

		responseBody := []byte("{}")
		if matchedSite != nil {
			// Check for SOCKERLESS_CMD / SOCKERLESS_ENTRYPOINT app setting
			// (cloud-native) or SimCommand fallback
			simCmd := false
			if matchedSite.Properties.SiteConfig != nil {
				for _, setting := range matchedSite.Properties.SiteConfig.AppSettings {
					if setting.Name == "SOCKERLESS_CMD" || setting.Name == "SOCKERLESS_ENTRYPOINT" {
						simCmd = true
						break
					}
				}
				if !simCmd && len(matchedSite.Properties.SiteConfig.SimCommand) > 0 {
					simCmd = true
				}
			}

			if simCmd {
				var exitCode int
				responseBody, exitCode = invokeAzureFunctionProcess(matchedSite)
				if exitCode != 0 {
					// Real Azure Functions returns HTTP error when function crashes
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(responseBody)
					return
				}
			} else {
				injectAppTrace(matchedSite.Name, "Function invoked")
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseBody)
	})

	// PUT - Update site's azurestorageaccounts mapping. Backend's
	// volumes.go uses WebApps.UpdateAzureStorageAccounts to bind named
	// docker volumes to Azure Files shares on the function app site.
	// Wire format: AzureStoragePropertyDictionaryResource —
	// `{ "properties": { "<volname>": { "type": "AzureFiles",
	// "accountName": "...", "shareName": "...", "accessKey": "...",
	// "mountPath": "/mnt/<vol>" }, ... } }`. The sim stores the dict
	// onto the site's AzureStorageAccounts field so subsequent GETs
	// round-trip the mapping.
	srv.HandleFunc("PUT "+armBase+"/sites/{siteName}/config/azurestorageaccounts", func(w http.ResponseWriter, r *http.Request) {
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

		var req AzureStoragePropertyDictionaryResource
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		site.Properties.AzureStorageAccounts = req.Properties
		sites.Put(resourceID, site)

		// ARM convention: respond with the resource shape that was PUT.
		sim.WriteJSON(w, http.StatusOK, AzureStoragePropertyDictionaryResource{
			ID:         resourceID + "/config/azurestorageaccounts",
			Name:       "azurestorageaccounts",
			Type:       "Microsoft.Web/sites/config",
			Properties: site.Properties.AzureStorageAccounts,
		})
	})

	// GET — symmetrical read so terraform / inspect tooling can verify
	// the mapping.
	srv.HandleFunc("GET "+armBase+"/sites/{siteName}/config/azurestorageaccounts/list", func(w http.ResponseWriter, r *http.Request) {
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
		sim.WriteJSON(w, http.StatusOK, AzureStoragePropertyDictionaryResource{
			ID:         resourceID + "/config/azurestorageaccounts",
			Name:       "azurestorageaccounts",
			Type:       "Microsoft.Web/sites/config",
			Properties: site.Properties.AzureStorageAccounts,
		})
	})
}

// AzureStoragePropertyDictionaryResource is the wire shape for
// WebApps.UpdateAzureStorageAccounts. Mirrors
// armappservice.AzureStoragePropertyDictionaryResource — a flat
// dictionary of volume-name → Azure Files mount info.
type AzureStoragePropertyDictionaryResource struct {
	ID         string                            `json:"id,omitempty"`
	Name       string                            `json:"name,omitempty"`
	Type       string                            `json:"type,omitempty"`
	Properties map[string]*AzureStorageInfoValue `json:"properties,omitempty"`
}

// AzureStorageInfoValue mirrors armappservice.AzureStorageInfoValue.
type AzureStorageInfoValue struct {
	Type        string `json:"type,omitempty"`
	AccountName string `json:"accountName,omitempty"`
	ShareName   string `json:"shareName,omitempty"`
	AccessKey   string `json:"accessKey,omitempty"`
	MountPath   string `json:"mountPath,omitempty"`
}

// invokeAzureFunctionProcess executes a function app's container via sim.StartContainerSync
// and returns the stdout output as the response body plus the process exit code.
func invokeAzureFunctionProcess(site *Site) ([]byte, int) {
	var entrypoint, cmd []string
	if site.Properties.SiteConfig != nil {
		// Cloud-native: read SOCKERLESS_ENTRYPOINT + SOCKERLESS_CMD
		// separately so docker's ENTRYPOINT vs CMD semantics are preserved.
		for _, s := range site.Properties.SiteConfig.AppSettings {
			switch s.Name {
			case "SOCKERLESS_ENTRYPOINT":
				if decoded, err := base64.StdEncoding.DecodeString(s.Value); err == nil {
					json.Unmarshal(decoded, &entrypoint)
				}
			case "SOCKERLESS_CMD":
				if decoded, err := base64.StdEncoding.DecodeString(s.Value); err == nil {
					json.Unmarshal(decoded, &cmd)
				}
			}
		}
		// Fallback: SimCommand (backward compat for SDK tests)
		if len(entrypoint) == 0 && len(cmd) == 0 {
			cmd = site.Properties.SiteConfig.SimCommand
		}
	}
	if len(entrypoint) == 0 && len(cmd) == 0 {
		return []byte("{}"), 0
	}

	// Derive container image from LinuxFxVersion (e.g., "DOCKER|myimage:latest")
	var containerImage string
	if site.Properties.SiteConfig != nil && site.Properties.SiteConfig.LinuxFxVersion != "" {
		parts := strings.SplitN(site.Properties.SiteConfig.LinuxFxVersion, "|", 2)
		if len(parts) == 2 {
			containerImage = parts[1]
		}
	}
	if containerImage == "" {
		// No container image configured — cannot run
		return []byte("{}"), 0
	}

	// Extract environment from app settings
	var cmdEnv map[string]string
	if site.Properties.SiteConfig != nil && len(site.Properties.SiteConfig.AppSettings) > 0 {
		cmdEnv = make(map[string]string, len(site.Properties.SiteConfig.AppSettings))
		for _, s := range site.Properties.SiteConfig.AppSettings {
			cmdEnv[s.Name] = s.Value
		}
	}

	timeout := 230 * time.Second // Azure Functions default timeout
	sink := &funcLogSink{appName: site.Name}
	var stdout bytes.Buffer
	collectSink := sim.FuncSink(func(line sim.LogLine) {
		sink.WriteLog(line)
		if line.Stream == "stdout" {
			stdout.WriteString(line.Text)
			stdout.WriteByte('\n')
		}
	})

	containerName := fmt.Sprintf("sockerless-sim-azure-func-%s-%d", site.Name, time.Now().UnixNano())

	handle, err := sim.StartContainerSync(sim.ContainerConfig{
		Image:   sim.ResolveLocalImage(containerImage),
		Command: entrypoint,
		Args:    cmd,
		Env:     cmdEnv,
		Timeout: timeout,
		Name:    containerName,
		Labels: map[string]string{
			"sockerless-sim-type": "azure-function-invocation",
			"sockerless-site":     site.Name,
		},
	}, collectSink)
	if err != nil {
		injectAppTrace(site.Name,
			fmt.Sprintf("Function execution error: container start failed: %v", err))
		return []byte("{}"), -1
	}
	result := handle.Wait()

	if result.ExitCode != 0 {
		injectAppTrace(site.Name,
			fmt.Sprintf("Function execution error: process exited with code %d", result.ExitCode))
	}

	output := strings.TrimRight(stdout.String(), "\n")
	if output == "" {
		return []byte("{}"), result.ExitCode
	}
	return []byte(output), result.ExitCode
}

// funcLogSink implements sim.LogSink and writes log lines to AppTraces
// for Azure Function invocations.
type funcLogSink struct {
	appName string
}

func (s *funcLogSink) WriteLog(line sim.LogLine) {
	injectAppTrace(s.appName, line.Text)
}
