package main

import (
	"fmt"
	"net/http"

	sim "github.com/sockerless/simulator"
)

type ContainerAppEnvironment struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties EnvProperties     `json:"properties"`
}

type EnvProperties struct {
	ProvisioningState          string                      `json:"provisioningState"`
	DefaultDomain              string                      `json:"defaultDomain"`
	StaticIp                   string                      `json:"staticIp"`
	AppLogsConfiguration       *AppLogsConfiguration       `json:"appLogsConfiguration,omitempty"`
	VnetConfiguration          *VnetConfiguration          `json:"vnetConfiguration,omitempty"`
	InfrastructureSubnetId     string                      `json:"infrastructureSubnetId,omitempty"`
	ZoneRedundant              bool                        `json:"zoneRedundant"`
	WorkloadProfiles           []WorkloadProfile           `json:"workloadProfiles,omitempty"`
	CustomDomainConfiguration  *CustomDomainConfiguration  `json:"customDomainConfiguration,omitempty"`
	PeerAuthentication         *PeerAuthentication         `json:"peerAuthentication,omitempty"`
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
	environments := sim.NewStateStore[ContainerAppEnvironment]()

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App"

	// PUT - Create or update container app environment
	srv.HandleFunc("PUT "+armBase+"/managedEnvironments/{envName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		envName := sim.PathParam(r, "envName")

		var req ContainerAppEnvironment
		sim.ReadJSON(r, &req)

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s",
			sub, rg, envName)

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

		environments.Delete(resourceID)
		w.WriteHeader(http.StatusAccepted)
	})
}
