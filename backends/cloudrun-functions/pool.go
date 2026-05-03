package gcf

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	core "github.com/sockerless/backend-core"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ensureOverlayImage builds + pushes the overlay image for `spec` to AR
// if it isn't already present. Returns the fully-qualified AR image URI.
// Cache check uses Cloud Build's Source-deduplication and AR's
// content-addressed tag scheme: a content-tag mismatch always rebuilds,
// a content-tag match short-circuits.
func (s *Server) ensureOverlayImage(ctx context.Context, spec OverlayImageSpec, contentTag string) (string, error) {
	imageURI := fmt.Sprintf(
		"%s-docker.pkg.dev/%s/sockerless-overlay/gcf:%s",
		s.config.Region, s.config.Project, contentTag,
	)
	if s.images.BuildService == nil {
		return "", fmt.Errorf("cloud Build service is required for gcf overlay image (set SOCKERLESS_GCP_BUILD_BUCKET)")
	}
	contextTar, err := TarOverlayContext(spec)
	if err != nil {
		return "", fmt.Errorf("tar overlay context: %w", err)
	}
	result, err := s.images.BuildService.Build(ctx, core.CloudBuildOptions{
		Dockerfile: "Dockerfile",
		ContextTar: bytes.NewReader(contextTar),
		Tags:       []string{imageURI},
		Platform:   "linux/amd64",
	})
	if err != nil {
		return "", fmt.Errorf("cloud build %s: %w", imageURI, err)
	}
	return result.ImageRef, nil
}

// claimFreeFunction lists sockerless-managed Functions for the given
// overlay-content-tag and atomically claims one whose `sockerless_allocation`
// label is empty by setting it to the new container ID via UpdateFunction
// with an etag. Returns the claimed function's full resource name on
// success, or empty string + nil error if no free function exists.
func (s *Server) claimFreeFunction(ctx context.Context, contentTag, containerID, containerName string) (string, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	filter := fmt.Sprintf(
		`labels.sockerless_managed:"true" AND labels.sockerless_overlay_hash:"%s"`,
		contentTag,
	)
	it := s.gcp.Functions.ListFunctions(ctx, &functionspb.ListFunctionsRequest{
		Parent: parent,
		Filter: filter,
	})
	for {
		fn, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", fmt.Errorf("list functions for pool query: %w", err)
		}
		// "Free" = sockerless_allocation label absent or empty.
		if alloc := fn.GetLabels()["sockerless_allocation"]; alloc != "" {
			continue
		}
		// Atomic CAS via UpdateFunction with the function's current Etag.
		// Cloud Functions Gen2 doesn't expose Etag on Function directly in
		// the same way some APIs do; the operation is best-effort: if a
		// concurrent claim wins, our subsequent operation will see
		// allocation already set on next list and we retry.
		updated := proto_clone_function(fn)
		if updated.Labels == nil {
			updated.Labels = make(map[string]string)
		}
		updated.Labels["sockerless_allocation"] = shortAllocLabel(containerID)
		if containerName != "" {
			updated.Labels["sockerless_name"] = sanitizeGCPLabelValue(strings.TrimPrefix(containerName, "/"))
		}
		_, err = s.gcp.Functions.UpdateFunction(ctx, &functionspb.UpdateFunctionRequest{
			Function:   updated,
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"labels"}},
		})
		if err != nil {
			s.Logger.Debug().Err(err).Str("function", fn.GetName()).Msg("pool claim conflict — retrying")
			continue
		}
		return fn.GetName(), nil
	}
	return "", nil // no free function in pool
}

// proto_clone_function returns a Function reference safe for mutation. Cloud
// Functions' Function proto carries a sync.Mutex (via protoimpl.MessageState)
// so a value-copy triggers govet's lockcopy lint. Mutate the original in
// place and rely on the caller never sharing the pointer.
func proto_clone_function(fn *functionspb.Function) *functionspb.Function {
	if fn.Labels == nil {
		fn.Labels = make(map[string]string)
	}
	return fn
}

// sanitizeGCPLabelValue returns v if it matches the GCP label charset
// `[a-z0-9_-]`, else returns the empty string. Sockerless-internal names
// (container short names like "/abc123") need this filter when written
// to a GCP label slot — the canonical container ID is on annotations.
func sanitizeGCPLabelValue(v string) string {
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			continue
		default:
			return ""
		}
	}
	// BUG-930: GCP label values are 63-char limited. gitlab-runner emits
	// permission-container names like
	// `runner-<id>-project-<n>-concurrent-<n>-<hash>-cache-<hash>-set-permission-<hash>`
	// which routinely exceed 130 chars. Truncate to 63 — the full
	// container ID lives in function annotations; this label is only
	// used for human-readable filtering.
	if len(v) > 63 {
		v = v[:63]
	}
	return v
}

// shortAllocLabel returns a GCP-label-safe short form of a 64-char Docker
// container ID. GCP labels are capped at 63 characters; we use the first
// 32 hex chars (128 bits of entropy — collision-resistant for any realistic
// concurrent-container count). The full container ID is preserved on the
// function as the SOCKERLESS_CONTAINER_ID environment variable so cloud-state
// reconstruction can recover it without label-encoded full IDs.
func shortAllocLabel(containerID string) string {
	if len(containerID) > 32 {
		return containerID[:32]
	}
	return containerID
}

// shortFunctionName extracts the trailing function name from a full
// resource path `projects/.../locations/.../functions/<name>`.
func shortFunctionName(fullName string) string {
	parts := strings.Split(fullName, "/")
	return parts[len(parts)-1]
}

// resolveGCFFromCloud is the stateless replacement for `s.GCF.Get(id)`.
// Every read path queries Cloud Functions API for a function tagged with
// `sockerless_allocation=<short(containerID)>` and returns the function's
// name + URL. Backends NEVER hold an in-memory cache of this state — pool
// claims/releases happen across instances, and a transient cache would go
// stale on every concurrent allocation change. Stateless invariant per
// `feedback_stateless_invariant.md`.
func (s *Server) resolveGCFFromCloud(ctx context.Context, containerID string) (GCFState, bool) {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	filter := fmt.Sprintf(`labels.sockerless_allocation:"%s"`, shortAllocLabel(containerID))
	it := s.gcp.Functions.ListFunctions(ctx, &functionspb.ListFunctionsRequest{
		Parent: parent,
		Filter: filter,
	})
	fn, err := it.Next()
	if err != nil || fn == nil {
		return GCFState{}, false
	}
	url := ""
	if fn.ServiceConfig != nil {
		url = fn.ServiceConfig.Uri
	}
	short := shortFunctionName(fn.Name)
	return GCFState{
		FunctionName: short,
		FunctionURL:  url,
		LogResource:  short,
	}, true
}

// swapServiceImage replaces the underlying Cloud Run Service's container
// image with the supplied overlay URI. Called immediately after
// CreateFunction to swap the throwaway Buildpacks-built image.
func (s *Server) swapServiceImage(ctx context.Context, fn *functionspb.Function, overlayImageURI string) error {
	if fn == nil || fn.ServiceConfig == nil || fn.ServiceConfig.Service == "" {
		return fmt.Errorf("function has no underlying service")
	}
	serviceName := fn.ServiceConfig.Service
	svc, err := s.gcp.Services.GetService(ctx, &runpb.GetServiceRequest{Name: serviceName})
	if err != nil {
		return fmt.Errorf("get service %s: %w", serviceName, err)
	}
	if svc.Template == nil || len(svc.Template.Containers) == 0 {
		return fmt.Errorf("service %s has no template containers", serviceName)
	}
	svc.Template.Containers[0].Image = overlayImageURI
	// Cloud Run requires a unique revision name for each update; clear the
	// existing revision name so a fresh one gets auto-generated. Otherwise
	// the API rejects with "Revision … with different configuration already exists".
	svc.Template.Revision = ""
	op, err := s.gcp.Services.UpdateService(ctx, &runpb.UpdateServiceRequest{
		Service: svc,
	})
	if err != nil {
		return fmt.Errorf("update service %s: %w", serviceName, err)
	}
	if _, err := op.Wait(ctx); err != nil {
		return fmt.Errorf("wait service rollout %s: %w", serviceName, err)
	}
	return nil
}

// releaseOrDeleteFunction is the pool-side counterpart to claimFreeFunction.
// On `docker rm <containerID>`:
//   - look up the function claimed by this container (allocation label)
//   - count free functions for the same overlay-hash
//   - if count >= SOCKERLESS_GCF_POOL_MAX, DeleteFunction
//   - otherwise UpdateFunction to clear the allocation label (back to pool)
func (s *Server) releaseOrDeleteFunction(ctx context.Context, fullName string, contentTag string) error {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	freeFilter := fmt.Sprintf(
		`labels.sockerless_managed:"true" AND labels.sockerless_overlay_hash:"%s" AND -labels.sockerless_allocation:*`,
		contentTag,
	)
	it := s.gcp.Functions.ListFunctions(ctx, &functionspb.ListFunctionsRequest{
		Parent: parent,
		Filter: freeFilter,
	})
	freeCount := 0
	for {
		_, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			s.Logger.Warn().Err(err).Msg("count free pool entries failed; defaulting to delete")
			break
		}
		freeCount++
	}

	if freeCount >= s.config.PoolMax {
		// Pool is full — delete this entry instead of returning to pool.
		op, err := s.gcp.Functions.DeleteFunction(ctx, &functionspb.DeleteFunctionRequest{Name: fullName})
		if err != nil {
			return fmt.Errorf("delete function %s: %w", fullName, err)
		}
		if err := op.Wait(ctx); err != nil {
			return fmt.Errorf("wait delete %s: %w", fullName, err)
		}
		return nil
	}

	// Pool has room — release back by clearing the allocation label.
	fn, err := s.gcp.Functions.GetFunction(ctx, &functionspb.GetFunctionRequest{Name: fullName})
	if err != nil {
		return fmt.Errorf("get function %s for release: %w", fullName, err)
	}
	updated := proto_clone_function(fn)
	if updated.Labels != nil {
		delete(updated.Labels, "sockerless_allocation")
		delete(updated.Labels, "sockerless_name")
	}
	_, err = s.gcp.Functions.UpdateFunction(ctx, &functionspb.UpdateFunctionRequest{
		Function:   updated,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"labels"}},
	})
	if err != nil {
		return fmt.Errorf("release function %s: %w", fullName, err)
	}
	return nil
}
