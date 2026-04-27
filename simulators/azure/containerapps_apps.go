package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Container Apps "Apps" slice (Microsoft.App/containerApps). Parallel
// to the Jobs slice in containerapps.go. Required for the
// `Config.UseApp=true` aca code path: when set, sockerless creates
// long-running ContainerApps with internal-only ingress instead of
// short-lived Jobs so peers resolve a stable per-revision FQDN.
//
// Wire format mirrors armappcontainers.ContainerApp (azure-sdk-for-go
// v2). Backend reads `properties.provisioningState`,
// `properties.latestReadyRevisionName`, and
// `properties.latestRevisionFqdn` (used to register a Private DNS
// CNAME), so each of those is populated on Create and Get.
//
// Real API: https://learn.microsoft.com/en-us/rest/api/containerapps/container-apps

// ContainerApp represents a Microsoft.App/containerApps resource.
type ContainerApp struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties ContainerAppProps `json:"properties"`
	SystemData *SystemData       `json:"systemData,omitempty"`
}

// ContainerAppProps holds the properties of a ContainerApp. Matches
// the field set armappcontainers.ContainerAppProperties exposes that
// the aca backend reads.
type ContainerAppProps struct {
	ProvisioningState       string                `json:"provisioningState"`
	ManagedEnvironmentID    string                `json:"managedEnvironmentId,omitempty"`
	EnvironmentID           string                `json:"environmentId,omitempty"`
	WorkloadProfileName     string                `json:"workloadProfileName,omitempty"`
	Configuration           *ContainerAppConfig   `json:"configuration,omitempty"`
	Template                *ContainerAppTemplate `json:"template,omitempty"`
	LatestRevisionName      string                `json:"latestRevisionName,omitempty"`
	LatestReadyRevisionName string                `json:"latestReadyRevisionName,omitempty"`
	LatestRevisionFqdn      string                `json:"latestRevisionFqdn,omitempty"`
}

// ContainerAppConfig mirrors armappcontainers.Configuration.
type ContainerAppConfig struct {
	ActiveRevisionsMode string                 `json:"activeRevisionsMode,omitempty"`
	Ingress             *ContainerAppIngress   `json:"ingress,omitempty"`
	Registries          []ContainerAppRegistry `json:"registries,omitempty"`
	Secrets             []ContainerAppSecret   `json:"secrets,omitempty"`
}

// ContainerAppIngress mirrors armappcontainers.Ingress. The backend
// sets External=false + TargetPort=8080 + Transport=auto.
type ContainerAppIngress struct {
	External   *bool  `json:"external,omitempty"`
	TargetPort *int32 `json:"targetPort,omitempty"`
	Transport  string `json:"transport,omitempty"`
	Fqdn       string `json:"fqdn,omitempty"`
}

// ContainerAppRegistry mirrors armappcontainers.RegistryCredentials.
type ContainerAppRegistry struct {
	Server            string `json:"server,omitempty"`
	Username          string `json:"username,omitempty"`
	PasswordSecretRef string `json:"passwordSecretRef,omitempty"`
	Identity          string `json:"identity,omitempty"`
}

// ContainerAppSecret mirrors armappcontainers.Secret.
type ContainerAppSecret struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Identity    string `json:"identity,omitempty"`
	KeyVaultURL string `json:"keyVaultUrl,omitempty"`
}

// ContainerAppTemplate mirrors armappcontainers.Template.
type ContainerAppTemplate struct {
	Containers     []JobContainer     `json:"containers,omitempty"`
	InitContainers []JobContainer     `json:"initContainers,omitempty"`
	Volumes        []JobVolume        `json:"volumes,omitempty"`
	Scale          *ContainerAppScale `json:"scale,omitempty"`
}

// ContainerAppScale mirrors armappcontainers.Scale.
type ContainerAppScale struct {
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
}

// AsyncOperationStatus is the response shape for a polled
// Azure-AsyncOperation status URL — `{"status":"Succeeded","name":...}`
// is what `armappcontainers` (and every azcore poller) reads.
type AsyncOperationStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // InProgress / Succeeded / Failed
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
}

// acaOps stores async-operation status keyed by opId. Initialized lazily
// in registerContainerAppsApps + reused by registerContainerApps (Jobs).
var acaOps sim.Store[AsyncOperationStatus]

// acaIssueAsyncOp records a new operation in the Creating→Succeeded
// state machine. Returns the opId; the caller writes the
// Azure-AsyncOperation header pointing at the polling URL. A goroutine
// flips the op to Succeeded after the configured delay (compresses real
// Azure's 30-60s reconcile window) — SDK pollers see the transition
// without having to wait for a real-cloud-provisioning timeline.
func acaIssueAsyncOp(loc string) string {
	opID := generateUUID()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	acaOps.Put(opID, AsyncOperationStatus{
		Name:      opID,
		Status:    "InProgress",
		StartTime: now,
	})
	go func() {
		time.Sleep(50 * time.Millisecond)
		acaOps.Update(opID, func(o *AsyncOperationStatus) {
			o.Status = "Succeeded"
			o.EndTime = time.Now().UTC().Format(time.RFC3339Nano)
		})
	}()
	return opID
}

// acaAsyncOpHeader returns the Azure-AsyncOperation header value for a
// given operation. Backend SDK pollers GET this URL until status=
// Succeeded (or Failed), then do a final GET on the resource. The path
// matches real ARM conventions for the Microsoft.App provider.
func acaAsyncOpHeader(r *http.Request, sub, loc, opID string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/subscriptions/%s/providers/Microsoft.App/locations/%s/operationStatuses/%s?api-version=2024-03-01",
		scheme, r.Host, sub, loc, opID)
}

func registerContainerAppsApps(srv *sim.Server) {
	apps := sim.MakeStore[ContainerApp](srv.DB(), "aca_apps")
	acaOps = sim.MakeStore[AsyncOperationStatus](srv.DB(), "aca_ops")

	const basePath = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App"

	// Operation status endpoint — Azure-AsyncOperation header points
	// here. SDK pollers GET this URL with the api-version query param;
	// we match real ARM by ignoring the api-version on read.
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/providers/Microsoft.App/locations/{location}/operationStatuses/{opId}",
		func(w http.ResponseWriter, r *http.Request) {
			opID := sim.PathParam(r, "opId")
			op, ok := acaOps.Get(opID)
			if !ok {
				sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
					"Operation %q not found.", opID)
				return
			}
			sim.WriteJSON(w, http.StatusOK, op)
		})

	// PUT - Create or update containerApp
	srv.HandleFunc("PUT "+basePath+"/containerApps/{appName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "appName")

		var req ContainerApp
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Location == "" {
			sim.AzureError(w, "InvalidRequestContent", "The 'location' property is required.", http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/containerApps/%s", sub, rg, name)
		_, exists := apps.Get(resourceID)

		// Real ACA: PUT returns 201 Created with provisioningState=Creating
		// + an Azure-AsyncOperation header pointing at an
		// /providers/Microsoft.App/locations/{loc}/operationStatuses/{id}
		// URL; the SDK poller (azcore.NewPoller) GETs that URL until
		// status=Succeeded, then does a final GET on the resource itself.
		// We match that flow: store the resource with Succeeded directly
		// (so the final GET always returns the desired state), record an
		// async operation that flips Creating→Succeeded after a small
		// delay (50ms — compresses real Azure's 30-60s reconcile window),
		// and emit Azure-AsyncOperation in the response header. The body
		// still echoes Succeeded so SDK clients that bypass polling and
		// read the body directly also see the right state.
		// The internal FQDN format mirrors real ACA:
		// <app>.internal.<env-id>.<region>.azurecontainerapps.io.
		// Backend's cloudServiceRegisterCNAME reads LatestRevisionFqdn to
		// seed the Private DNS A/CNAME record for peer discovery.
		revName := fmt.Sprintf("%s--00001", name)
		fqdn := fmt.Sprintf("%s.internal.sim-env.%s.azurecontainerapps.io", name, req.Location)

		app := ContainerApp{
			ID:       resourceID,
			Name:     name,
			Type:     "Microsoft.App/containerApps",
			Location: req.Location,
			Tags:     req.Tags,
			Properties: ContainerAppProps{
				ProvisioningState:       "Succeeded",
				EnvironmentID:           req.Properties.EnvironmentID,
				ManagedEnvironmentID:    req.Properties.ManagedEnvironmentID,
				WorkloadProfileName:     req.Properties.WorkloadProfileName,
				Configuration:           req.Properties.Configuration,
				Template:                req.Properties.Template,
				LatestRevisionName:      revName,
				LatestReadyRevisionName: revName,
				LatestRevisionFqdn:      fqdn,
			},
			SystemData: &SystemData{
				CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			},
		}
		if app.Properties.Configuration != nil && app.Properties.Configuration.ActiveRevisionsMode == "" {
			app.Properties.Configuration.ActiveRevisionsMode = "Single"
		}
		if app.Properties.Configuration != nil && app.Properties.Configuration.Ingress != nil {
			app.Properties.Configuration.Ingress.Fqdn = fqdn
		}

		apps.Put(resourceID, app)

		// Set Azure-AsyncOperation so SDK pollers exercise the real flow.
		opID := acaIssueAsyncOp(req.Location)
		w.Header().Set("Azure-AsyncOperation", acaAsyncOpHeader(r, sub, req.Location, opID))

		if exists {
			sim.WriteJSON(w, http.StatusOK, app)
		} else {
			sim.WriteJSON(w, http.StatusCreated, app)
		}
	})

	// GET - Get containerApp
	srv.HandleFunc("GET "+basePath+"/containerApps/{appName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "appName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/containerApps/%s", sub, rg, name)
		app, ok := apps.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.App/containerApps/%s' under resource group '%s' was not found.", name, rg)
			return
		}
		sim.WriteJSON(w, http.StatusOK, app)
	})

	// GET - List containerApps in resource group
	srv.HandleFunc("GET "+basePath+"/containerApps", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/containerApps/", sub, rg)
		filtered := apps.Filter(func(a ContainerApp) bool {
			return strings.HasPrefix(a.ID, prefix)
		})
		sim.WriteJSON(w, http.StatusOK, map[string]any{"value": filtered})
	})

	// DELETE - Delete containerApp
	srv.HandleFunc("DELETE "+basePath+"/containerApps/{appName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "appName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/containerApps/%s", sub, rg, name)
		if !apps.Delete(resourceID) {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.App/containerApps/%s' under resource group '%s' was not found.", name, rg)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}
