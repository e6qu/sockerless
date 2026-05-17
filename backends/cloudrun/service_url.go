package cloudrun

import (
	"context"

	runpb "cloud.google.com/go/run/apiv2/runpb"
)

// serviceInvokeURL resolves the Cloud Run Service's invoke URL for the
// container. Returns ("", false) when the Service hasn't materialised
// yet (Uri unset) or when the cloud client isn't available. Used by
// the start-service goroutine to know when the Service is reachable
// for downstream operations (e.g. waiting for the bootstrap to dial
// back, materialising peer-pod members).
func (s *Server) serviceInvokeURL(ctx context.Context, containerID string) (string, bool) {
	state, ok := s.resolveServiceCloudRunState(ctx, containerID)
	if !ok || state.ServiceName == "" {
		return "", false
	}
	if s.gcp == nil || s.gcp.Services == nil {
		return "", false
	}
	svc, err := s.gcp.Services.GetService(ctx, &runpb.GetServiceRequest{Name: state.ServiceName})
	if err != nil || svc.Uri == "" {
		return "", false
	}
	return svc.Uri, true
}
