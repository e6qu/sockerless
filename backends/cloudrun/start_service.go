package cloudrun

import (
	"context"
	"fmt"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
)

// — single- and multi-container start paths for the Services
// code path. When Config.UseService is true, ContainerStart dispatches
// here instead of the Jobs flow in backend_impl.go.
// Services are long-running with MinInstanceCount=1, so there's no
// execution-exit poller — the container stays "running" until
// ContainerStop/Remove deletes the Service and closes WaitChs
// directly.

// startSingleContainerService provisions a Cloud Run Service for one
// container. Returns after the Service is created and the first
// revision has a terminal Ready condition (or we give up waiting and
// return an error with the failed condition message).
func (s *Server) startSingleContainerService(id string, c api.Container, crState CloudRunState, _ chan struct{}) error {
	// Clean up any existing resources from a previous start.
	if crState.ServiceName != "" {
		s.deleteService(crState.ServiceName)
		s.Registry.MarkCleanedUp(crState.ServiceName)
	}

	svcName := buildServiceName(id)
	svcSpec, err := s.buildServiceSpec(s.ctx(), []containerInput{
		{ID: id, Container: &c, IsMain: true},
	})
	if err != nil {
		s.Store.WaitChs.Delete(id)
		return err
	}

	createOp, err := s.gcp.Services.CreateService(s.ctx(), &runpb.CreateServiceRequest{
		Parent:    s.buildServiceParent(),
		ServiceId: svcName,
		Service:   svcSpec,
	})
	if err != nil {
		s.Logger.Error().Err(err).Str("service", svcName).Msg("failed to create Cloud Run Service")
		s.Store.WaitChs.Delete(id)
		return gcpcommon.MapGCPError(err, "service", id)
	}

	svc, err := createOp.Wait(s.ctx())
	if err != nil {
		s.deleteService(fmt.Sprintf("%s/services/%s", s.buildServiceParent(), svcName))
		s.Store.WaitChs.Delete(id)
		s.Logger.Error().Err(err).Str("service", svcName).Msg("service creation failed")
		return gcpcommon.MapGCPError(err, "service", id)
	}
	svcFullName := svc.Name

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "cloudrun",
		ResourceType: "service",
		ResourceID:   svcFullName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": c.Image, "name": c.Name, "serviceName": svcName},
	})

	s.PendingCreates.Delete(id)
	s.CloudRun.Update(id, func(state *CloudRunState) {
		state.ServiceName = svcFullName
	})
	return nil
}

// startMultiContainerServiceTyped provisions a single Cloud Run Service
// whose revision template holds all pod containers. Parallels
// startMultiContainerJobTyped.
func (s *Server) startMultiContainerServiceTyped(_ string, podContainers []api.Container, _ chan struct{}) error {
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

	svcName := buildServiceName(mainID)
	svcSpec, err := s.buildServiceSpec(s.ctx(), inputs)
	if err != nil {
		return err
	}

	createOp, err := s.gcp.Services.CreateService(s.ctx(), &runpb.CreateServiceRequest{
		Parent:    s.buildServiceParent(),
		ServiceId: svcName,
		Service:   svcSpec,
	})
	if err != nil {
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		s.Logger.Error().Err(err).Str("service", svcName).Msg("failed to create multi-container Cloud Run Service")
		return gcpcommon.MapGCPError(err, "service", mainID)
	}

	svc, err := createOp.Wait(s.ctx())
	if err != nil {
		s.deleteService(fmt.Sprintf("%s/services/%s", s.buildServiceParent(), svcName))
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		s.Logger.Error().Err(err).Str("service", svcName).Msg("service creation failed")
		return gcpcommon.MapGCPError(err, "service", mainID)
	}
	svcFullName := svc.Name

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  mainID,
		Backend:      "cloudrun",
		ResourceType: "service",
		ResourceID:   svcFullName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": podContainers[0].Image, "name": podContainers[0].Name, "serviceName": svcName},
	})

	for _, pc := range podContainers {
		s.PendingCreates.Delete(pc.ID)
		s.CloudRun.Update(pc.ID, func(state *CloudRunState) {
			state.ServiceName = svcFullName
		})
	}
	return nil
}

// deleteService deletes a Cloud Run Service and waits for the LRO to
// complete. Best-effort: errors are logged, not propagated, so caller
// cleanup paths stay simple.
func (s *Server) deleteService(serviceName string) {
	if serviceName == "" || s.gcp == nil || s.gcp.Services == nil {
		return
	}
	op, err := s.gcp.Services.DeleteService(context.Background(), &runpb.DeleteServiceRequest{
		Name: serviceName,
	})
	if err != nil {
		s.Logger.Warn().Err(err).Str("service", serviceName).Msg("failed to initiate service delete")
		return
	}
	if _, err := op.Wait(context.Background()); err != nil {
		s.Logger.Warn().Err(err).Str("service", serviceName).Msg("service delete wait failed")
	}
}
