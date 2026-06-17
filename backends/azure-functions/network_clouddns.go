package azf

import (
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"github.com/sockerless/api"
)

// startCloudDNSSite deploys a container on a user-defined network as its own
// App Service site under cloud-dns discovery: the site joins the network's VNet
// via App Service regional VNet integration and its --network-alias names are
// registered in the linked Private DNS zone, so peer sites resolve it by name.
//
// A script-runner (OpenStdin, e.g. the gitlab-runner build container) deploys
// the overlay image and is invoked over HTTP per stage. A service (non-OpenStdin,
// e.g. a `services:` redis) deploys its raw image and runs its own process on
// the VNet — reached container-to-container on its own port (6379), never
// HTTP-invoked.
func (s *Server) startCloudDNSSite(id string, c api.Container, netID string) error {
	var aliasList []string
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil {
			aliasList = append(aliasList, ep.Aliases...)
		}
	}
	s.Logger.Info().Str("container", id[:min(12, len(id))]).Str("name", c.Name).Bool("openStdin", c.Config.OpenStdin).Str("netID", netID[:min(12, len(netID))]).Strs("aliases", aliasList).Msg("cloud-dns: starting site")

	state, ok := s.resolveNetworkState(s.ctx(), netID)
	if !ok || state.SubnetID == "" {
		return &api.ServerError{Message: fmt.Sprintf(
			"network %s has no VNet/subnet provisioned for cloud-dns service discovery; was the docker network created through NetworkCreate?", netID[:min(12, len(netID))])}
	}

	// Deploy the site. A service runs its raw image (no function bootstrap); the
	// script-runner keeps the overlay image it was created with.
	deployContainer := c
	if !c.Config.OpenStdin {
		deployContainer.Config.Image = podMemberDisplayImage(c)
	}

	azfState, hasState := s.AZF.Get(id)
	if !hasState || azfState.FunctionURL == "" {
		// A script-runner deploys with the function bootstrap app settings; a
		// service deploys clean (it runs its raw image, with no SOCKERLESS_*
		// bootstrap env — those would make the App Service site look like an
		// HTTP function and never run the service process).
		appSettings := s.buildAZFServiceAppSettings(deployContainer)
		if c.Config.OpenStdin {
			appSettings = s.buildAZFAppSettings(id, deployContainer.Config)
		}
		var err error
		azfState, err = s.createFunctionSite(s.ctx(), id, "skls-"+id[:12], deployContainer, appSettings)
		if err != nil {
			return err
		}
		s.AZF.Put(id, azfState)
	}

	// Regional VNet integration: join the site to the Microsoft.Web-delegated
	// subnet so it shares the network with its peers. This also brings up a
	// service container (it has no HTTP invoke to start it).
	if err := s.integrateSiteVNet(azfState.FunctionAppName, state.SubnetID); err != nil {
		return &api.ServerError{Message: fmt.Sprintf("VNet-integrate site %s: %v", azfState.FunctionAppName, err)}
	}

	// Register each --network-alias as a Private DNS CNAME → the site's default
	// hostname, so peers on the linked VNet resolve the alias to this site.
	s.registerSiteServiceDiscovery(c, azfState, state.DNSZoneName)

	if c.Config.OpenStdin {
		// Script-runner: invoke the stage over HTTP.
		return s.invokeFunctionAsync(id, c, azfState)
	}

	// Service: it now runs on the VNet. Mark it running so the runner's service
	// health-check (docker inspect for State.Running + ExposedPorts) passes; it
	// has no exit to wait on.
	s.PendingCreates.Update(id, func(pc *api.Container) {
		pc.State.Status = "running"
		pc.State.Running = true
		pc.State.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	})
	s.EmitEvent("container", "start", id, map[string]string{"name": trimSlash(c.Name)})
	return nil
}

// buildAZFServiceAppSettings builds App Service app settings for a raw
// `services:` container (e.g. redis). It carries the registry + the service's
// own non-sockerless env, but NONE of the function-bootstrap vars
// (SOCKERLESS_*) — the service runs its raw image directly on the VNet, so
// bootstrap settings would both be meaningless and (via SOCKERLESS_USER_*) make
// the sim treat the site as an HTTP function instead of starting the service.
func (s *Server) buildAZFServiceAppSettings(c api.Container) []*armappservice.NameValuePair {
	settings := []*armappservice.NameValuePair{
		{Name: to.Ptr("WEBSITES_ENABLE_APP_SERVICE_STORAGE"), Value: to.Ptr("false")},
	}
	if s.config.Registry != "" {
		settings = append(settings, &armappservice.NameValuePair{Name: to.Ptr("DOCKER_REGISTRY_SERVER_URL"), Value: to.Ptr(s.config.Registry)})
	}
	for _, e := range c.Config.Env {
		k, v, ok := strings.Cut(e, "=")
		if !ok || strings.HasPrefix(k, "SOCKERLESS_") {
			continue
		}
		settings = append(settings, &armappservice.NameValuePair{Name: to.Ptr(k), Value: to.Ptr(v)})
	}
	return settings
}

// integrateSiteVNet joins an App Service site to a subnet via regional VNet
// (swift) integration — the App Service container then shares the subnet's VNet
// with its peer sites.
func (s *Server) integrateSiteVNet(siteName, subnetID string) error {
	_, err := s.azure.WebApps.CreateOrUpdateSwiftVirtualNetworkConnectionWithCheck(s.ctx(), s.config.ResourceGroup, siteName, armappservice.SwiftVirtualNetwork{
		Properties: &armappservice.SwiftVirtualNetworkProperties{
			SubnetResourceID: to.Ptr(subnetID),
			SwiftSupported:   to.Ptr(true),
		},
	}, nil)
	return err
}

// registerSiteServiceDiscovery registers each of the container's --network-alias
// names (per attached network) as a Private DNS CNAME pointing at the site's
// default hostname, in the zone linked to that network's VNet. App Service sites
// have stable per-site FQDNs, so a CNAME (not an A-record) is the faithful
// service-discovery record — mirroring the ACA Apps path.
func (s *Server) registerSiteServiceDiscovery(c api.Container, azfState AZFState, zoneName string) {
	fqdn := azfState.FunctionHost
	if fqdn == "" || zoneName == "" {
		return
	}
	for _, ep := range c.NetworkSettings.Networks {
		if ep == nil || len(ep.Aliases) == 0 {
			continue
		}
		for _, alias := range ep.Aliases {
			if alias == "" {
				continue
			}
			if _, err := s.azure.PrivateDNSRecords.CreateOrUpdate(s.ctx(), s.config.ResourceGroup, zoneName, armprivatedns.RecordTypeCNAME, alias, armprivatedns.RecordSet{
				Properties: &armprivatedns.RecordSetProperties{
					TTL:         to.Ptr(int64(3600)),
					CnameRecord: &armprivatedns.CnameRecord{Cname: to.Ptr(fqdn)},
				},
			}, nil); err != nil {
				s.Logger.Warn().Err(err).Str("alias", alias).Str("fqdn", fqdn).Msg("register service CNAME in Private DNS failed")
			} else {
				s.Logger.Info().Str("alias", alias).Str("fqdn", fqdn).Str("zone", zoneName).Msg("cloud-dns: registered service CNAME")
			}
		}
	}
}

func trimSlash(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}
