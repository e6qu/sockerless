package aca

import (
	"context"
	"time"

	"github.com/sockerless/api"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

// — single- and multi-container start paths for the Apps code
// path. When Config.UseApp is true, ContainerStart dispatches here
// instead of the Jobs flow in backend_impl.go.
// Apps are long-running with min/max replicas=1, so there's no
// execution-exit poller — the container stays "running" until
// ContainerStop/Remove deletes the ContainerApp and closes WaitChs
// directly (seechanges to Stop/Kill/Remove).

// startSingleContainerApp provisions an ACA ContainerApp for one
// container. Returns after the App resource is created (BeginCreateOrUpdate
// LRO completes); peer reachability kicks in when LatestReadyRevisionName
// is populated.
func (s *Server) startSingleContainerApp(id string, c api.Container, acaState ACAState, _ chan struct{}) error {
	if acaState.AppName != "" {
		s.deleteApp(acaState.AppName)
		s.Registry.MarkCleanedUp(acaState.AppName)
	}

	appName := buildAppName(id)
	appSpec, err := s.buildAppSpec(s.ctx(), []containerInput{
		{ID: id, Container: &c, IsMain: true},
	})
	if err != nil {
		s.Store.WaitChs.Delete(id)
		return err
	}

	createPoller, err := s.azure.ContainerApps.BeginCreateOrUpdate(s.ctx(), s.config.ResourceGroup, appName, appSpec, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("app", appName).Msg("failed to create ACA ContainerApp")
		s.Store.WaitChs.Delete(id)
		return azurecommon.MapAzureError(err, "containerapp", id)
	}

	if _, err := createPoller.PollUntilDone(s.ctx(), nil); err != nil {
		s.deleteApp(appName)
		s.Store.WaitChs.Delete(id)
		s.Logger.Error().Err(err).Str("app", appName).Msg("app creation failed")
		return azurecommon.MapAzureError(err, "containerapp", id)
	}

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "aca",
		ResourceType: "containerapp",
		ResourceID:   appName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": c.Image, "name": c.Name, "appName": appName},
	})

	s.PendingCreates.Delete(id)
	s.ACA.Update(id, func(state *ACAState) {
		state.AppName = appName
	})
	return nil
}

// startMultiContainerAppTyped provisions a single ACA ContainerApp
// whose revision template holds all pod containers. Parallel to
// startMultiContainerJobTyped.
func (s *Server) startMultiContainerAppTyped(_ string, podContainers []api.Container, _ chan struct{}) error {
	var inputs []containerInput
	for i, pc := range podContainers {
		pcCopy := pc
		inputs = append(inputs, containerInput{
			ID:        pc.ID,
			Container: &pcCopy,
			IsMain:    i == 0,
		})
	}
	mainID := podContainers[0].ID

	appName := buildAppName(mainID)
	appSpec, err := s.buildAppSpec(s.ctx(), inputs)
	if err != nil {
		return err
	}

	createPoller, err := s.azure.ContainerApps.BeginCreateOrUpdate(s.ctx(), s.config.ResourceGroup, appName, appSpec, nil)
	if err != nil {
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		s.Logger.Error().Err(err).Str("app", appName).Msg("failed to create multi-container ACA ContainerApp")
		return azurecommon.MapAzureError(err, "containerapp", mainID)
	}

	if _, err := createPoller.PollUntilDone(s.ctx(), nil); err != nil {
		s.deleteApp(appName)
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		s.Logger.Error().Err(err).Str("app", appName).Msg("app creation failed")
		return azurecommon.MapAzureError(err, "containerapp", mainID)
	}

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  mainID,
		Backend:      "aca",
		ResourceType: "containerapp",
		ResourceID:   appName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": podContainers[0].Image, "name": podContainers[0].Name, "appName": appName},
	})

	for _, pc := range podContainers {
		s.PendingCreates.Delete(pc.ID)
		s.ACA.Update(pc.ID, func(state *ACAState) {
			state.AppName = appName
		})
	}
	return nil
}

// deleteApp deletes an ACA ContainerApp. Best-effort: errors are logged
// but not propagated so cleanup paths stay simple.
func (s *Server) deleteApp(appName string) {
	if appName == "" || s.azure == nil || s.azure.ContainerApps == nil {
		return
	}
	poller, err := s.azure.ContainerApps.BeginDelete(context.Background(), s.config.ResourceGroup, appName, nil)
	if err != nil {
		s.Logger.Warn().Err(err).Str("app", appName).Msg("failed to initiate app delete")
		return
	}
	if _, err := poller.PollUntilDone(context.Background(), nil); err != nil {
		s.Logger.Warn().Err(err).Str("app", appName).Msg("app delete wait failed")
	}
}
