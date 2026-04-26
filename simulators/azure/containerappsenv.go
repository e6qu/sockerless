package main

import (
	"fmt"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

type ContainerAppEnvironment struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties EnvProperties     `json:"properties"`
	// DockerNetworkName is the real Docker user-defined network that
	// backs this environment. Jobs launched in this environment are
	// connected to the network with the job short name as DNS alias,
	// so cross-job DNS works via Docker's embedded resolver. Empty
	// until the env's PUT handler creates the network.
	DockerNetworkName string `json:"dockerNetworkName,omitempty"`
}

// acaEnvironments is the package-level store for Container Apps
// environments. Exposed so containerapps.go can resolve a job's
// environment + backing Docker network when launching executions.
var acaEnvironments sim.Store[ContainerAppEnvironment]

type EnvProperties struct {
	ProvisioningState         string                     `json:"provisioningState"`
	DefaultDomain             string                     `json:"defaultDomain"`
	StaticIp                  string                     `json:"staticIp"`
	AppLogsConfiguration      *AppLogsConfiguration      `json:"appLogsConfiguration,omitempty"`
	VnetConfiguration         *VnetConfiguration         `json:"vnetConfiguration,omitempty"`
	InfrastructureSubnetId    string                     `json:"infrastructureSubnetId,omitempty"`
	ZoneRedundant             bool                       `json:"zoneRedundant"`
	WorkloadProfiles          []WorkloadProfile          `json:"workloadProfiles,omitempty"`
	CustomDomainConfiguration *CustomDomainConfiguration `json:"customDomainConfiguration,omitempty"`
	PeerAuthentication        *PeerAuthentication        `json:"peerAuthentication,omitempty"`
}

type CustomDomainConfiguration struct {
	CustomDomainVerificationId string `json:"customDomainVerificationId"`
}

type PeerAuthentication struct {
	Mtls *Mtls `json:"mtls,omitempty"`
}

type Mtls struct {
	Enabled bool `json:"enabled"`
}

type WorkloadProfile struct {
	Name                string `json:"name"`
	WorkloadProfileType string `json:"workloadProfileType"`
}

type AppLogsConfiguration struct {
	Destination               string                     `json:"destination,omitempty"`
	LogAnalyticsConfiguration *LogAnalyticsConfiguration `json:"logAnalyticsConfiguration,omitempty"`
}

type LogAnalyticsConfiguration struct {
	CustomerId string `json:"customerId,omitempty"`
	SharedKey  string `json:"sharedKey,omitempty"`
}

type VnetConfiguration struct {
	InfrastructureSubnetId string `json:"infrastructureSubnetId,omitempty"`
	Internal               bool   `json:"internal"`
}

func registerContainerAppEnvironment(srv *sim.Server) {
	acaEnvironments = sim.MakeStore[ContainerAppEnvironment](srv.DB(), "aca_environments")
	environments := acaEnvironments

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App"

	// PUT - Create or update container app environment
	srv.HandleFunc("PUT "+armBase+"/managedEnvironments/{envName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		envName := sim.PathParam(r, "envName")

		var req ContainerAppEnvironment
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s",
			sub, rg, envName)

		// Back every environment with a real Docker user-defined
		// network. Jobs created in this environment are connected to
		// the network at execution-start time with the job short name
		// as DNS alias, so cross-job DNS resolves via Docker's
		// embedded resolver. Matches ACA's managed-VNet model where
		// environment = shared networking domain.
		dockerNetName := "sim-env-" + envName
		if _, err := sim.EnsureDockerNetwork(dockerNetName); err != nil {
			dockerNetName = ""
		}

		env := ContainerAppEnvironment{
			ID:       resourceID,
			Name:     envName,
			Type:     "Microsoft.App/managedEnvironments",
			Location: req.Location,
			Tags:     req.Tags,
			Properties: EnvProperties{
				ProvisioningState:      "Succeeded",
				DefaultDomain:          fmt.Sprintf("%s.%s.azurecontainerapps.io", envName, req.Location),
				StaticIp:               "10.0.0.100",
				AppLogsConfiguration:   req.Properties.AppLogsConfiguration,
				VnetConfiguration:      req.Properties.VnetConfiguration,
				InfrastructureSubnetId: req.Properties.InfrastructureSubnetId,
				ZoneRedundant:          req.Properties.ZoneRedundant,
				WorkloadProfiles:       req.Properties.WorkloadProfiles,
				CustomDomainConfiguration: &CustomDomainConfiguration{
					CustomDomainVerificationId: generateUUID(),
				},
				PeerAuthentication: &PeerAuthentication{
					Mtls: &Mtls{Enabled: false},
				},
			},
			DockerNetworkName: dockerNetName,
		}
		environments.Put(resourceID, env)

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, env)
	})

	// GET - Get container app environment
	srv.HandleFunc("GET "+armBase+"/managedEnvironments/{envName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		envName := sim.PathParam(r, "envName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s",
			sub, rg, envName)

		env, ok := environments.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"Managed environment '%s' not found.", envName)
			return
		}
		sim.WriteJSON(w, http.StatusOK, env)
	})

	// DELETE - Delete container app environment
	srv.HandleFunc("DELETE "+armBase+"/managedEnvironments/{envName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		envName := sim.PathParam(r, "envName")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s",
			sub, rg, envName)

		// Drop the backing Docker network when the env is removed.
		if env, ok := environments.Get(resourceID); ok && env.DockerNetworkName != "" {
			_ = sim.RemoveDockerNetwork(env.DockerNetworkName)
		}

		environments.Delete(resourceID)
		w.WriteHeader(http.StatusAccepted)
	})

	registerContainerAppEnvironmentStorages(srv, environments)
}

// ManagedEnvironmentStorage pairs a sockerless-known name with a
// storage-account + file-share backing it. ACA jobs/apps reference
// these by storage.name to resolve their VolumeMounts.
type ManagedEnvironmentStorage struct {
	ID         string                              `json:"id"`
	Name       string                              `json:"name"`
	Type       string                              `json:"type"`
	Properties ManagedEnvironmentStorageProperties `json:"properties"`
}

// ManagedEnvironmentStorageProperties mirrors
// Microsoft.App/managedEnvironments/storages resources.
type ManagedEnvironmentStorageProperties struct {
	AzureFile *AzureFileProperties `json:"azureFile,omitempty"`
}

// AzureFileProperties carries the per-storage file-share backing.
type AzureFileProperties struct {
	AccountName string `json:"accountName"`
	ShareName   string `json:"shareName"`
	AccountKey  string `json:"accountKey,omitempty"`
	AccessMode  string `json:"accessMode,omitempty"`
}

// acaEnvStorages is the package-level store for
// managedEnvironments/<env>/storages/<name> resources. Exported for
// use by the ACA Jobs/Apps executor to resolve a VolumeName →
// (account, share) for host-side binding.
var acaEnvStorages sim.Store[ManagedEnvironmentStorage]

func registerContainerAppEnvironmentStorages(srv *sim.Server, envs sim.Store[ContainerAppEnvironment]) {
	acaEnvStorages = sim.MakeStore[ManagedEnvironmentStorage](srv.DB(), "aca_env_storages")

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{envName}/storages/{storageName}"

	srv.HandleFunc("PUT "+armBase, func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		envName := sim.PathParam(r, "envName")
		storageName := sim.PathParam(r, "storageName")

		envID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s",
			sub, rg, envName)
		if _, ok := envs.Get(envID); !ok {
			sim.AzureError(w, "ParentResourceNotFound", "Managed environment not found: "+envID, http.StatusNotFound)
			return
		}

		var req ManagedEnvironmentStorage
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		id := envID + "/storages/" + storageName
		storage := ManagedEnvironmentStorage{
			ID:         id,
			Name:       storageName,
			Type:       "Microsoft.App/managedEnvironments/storages",
			Properties: req.Properties,
		}
		acaEnvStorages.Put(id, storage)
		sim.WriteJSON(w, http.StatusOK, storage)
	})

	srv.HandleFunc("GET "+armBase, func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		envName := sim.PathParam(r, "envName")
		storageName := sim.PathParam(r, "storageName")
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s/storages/%s",
			sub, rg, envName, storageName)
		storage, ok := acaEnvStorages.Get(id)
		if !ok {
			sim.AzureError(w, "ResourceNotFound", "Storage not found: "+id, http.StatusNotFound)
			return
		}
		sim.WriteJSON(w, http.StatusOK, storage)
	})

	srv.HandleFunc("DELETE "+armBase, func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		envName := sim.PathParam(r, "envName")
		storageName := sim.PathParam(r, "storageName")
		id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s/storages/%s",
			sub, rg, envName, storageName)
		acaEnvStorages.Delete(id)
		w.WriteHeader(http.StatusOK)
	})

	srv.HandleFunc("GET /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{envName}/storages", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		envName := sim.PathParam(r, "envName")
		prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s/storages/",
			sub, rg, envName)
		filtered := acaEnvStorages.Filter(func(s ManagedEnvironmentStorage) bool {
			return strings.HasPrefix(s.ID, prefix)
		})
		sim.WriteJSON(w, http.StatusOK, map[string]any{"value": filtered})
	})
}

// LookupEnvStorageBinding returns (storageAccount, shareName, ok) for
// a managed-environment storage reference (envID = the env resource
// path, storageName = the storage sub-resource name). Used by the ACA
// Jobs / Apps executor to resolve Volume{StorageName} → host dir.
func LookupEnvStorageBinding(envID, storageName string) (string, string, bool) {
	id := envID + "/storages/" + storageName
	s, ok := acaEnvStorages.Get(id)
	if !ok || s.Properties.AzureFile == nil {
		return "", "", false
	}
	return s.Properties.AzureFile.AccountName, s.Properties.AzureFile.ShareName, true
}
