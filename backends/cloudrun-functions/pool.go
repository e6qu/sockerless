package gcf

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
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
	// AR tag-existence precheck: HEAD /v2/<repo>/manifests/<tag>. On
	// cache hit (the common case for prewarmed overlays + same-revision
	// rebuilds) we skip Cloud Build's ~25-30s tag-rebuild overhead. The
	// HEAD takes well under a second.
	if gcpcommon.CheckTagExists(ctx, imageURI) {
		s.Logger.Info().Str("image", imageURI).Msg("ensureOverlayImage: AR tag already present — skipping Cloud Build")
		return imageURI, nil
	}
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
//
// Wraps tryClaimOnce in a short retry loop with poll-and-wait. When a
// caller burst (e.g., gitlab-runner spawning multiple cache-permission
// containers concurrently) finds the pool empty because peers haven't
// released yet, the wait gives those peers time to release their
// allocations before this call falls back to creating a new function.
// Each new-function deploy costs ~1 vCPU-min against the regional
// CpuAllocPerProjectRegion quota, so reuse is the architectural fix to
// the quota pressure.

// tryClaimOnce performs a single list-and-claim pass. Returns ("", nil) when
// no free function exists; ("", err) on API errors; (name, nil) on success.

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
//
// Cloud Functions Gen2's ListFunctions response returns an abbreviated
// Function shape — ServiceConfig.Uri may be empty in list results even
// when the function is ACTIVE with URI populated. Follow up the
// list-by-label match with a GetFunction by exact name to retrieve the
// full ServiceConfig including the URI used for invoke.
func (s *Server) resolveGCFFromCloud(ctx context.Context, containerID string) (GCFState, bool) {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	filter := fmt.Sprintf(`labels.sockerless_allocation:"%s"`, shortAllocLabel(containerID))
	it := s.gcp.Functions.ListFunctions(ctx, &functionspb.ListFunctionsRequest{
		Parent: parent,
		Filter: filter,
	})
	fn, err := it.Next()
	if err != nil || fn == nil {
		// BUG-953: pod-mode resources are Cloud Run Services (not
		// Functions). When no Function matches the allocation label,
		// fall through to a Service lookup — pod members live there
		// after materializePodService runs.
		if state, ok := s.resolvePodServiceFromCloud(ctx, containerID); ok {
			return state, true
		}
		return GCFState{}, false
	}
	full, getErr := s.gcp.Functions.GetFunction(ctx, &functionspb.GetFunctionRequest{Name: fn.Name})
	if getErr != nil {
		s.Logger.Warn().Err(getErr).Str("function", fn.Name).Msg("resolveGCFFromCloud: GetFunction failed; falling back to ListFunctions abbreviated result")
	} else if full != nil {
		fn = full
	}
	url := ""
	svcName := ""
	if fn.ServiceConfig != nil {
		url = fn.ServiceConfig.Uri
		svcName = fn.ServiceConfig.Service
	}
	// Last-resort fallback: derive the URL from the underlying Cloud
	// Run Service if Cloud Functions still hasn't propagated the URI
	// onto the Function object. Real GCP populates this synchronously
	// on Service ACTIVE, but we've observed lag windows where Function
	// .ServiceConfig.uri is empty even after a successful GetFunction.
	if url == "" && svcName != "" {
		if svc, sErr := s.gcp.Services.GetService(ctx, &runpb.GetServiceRequest{Name: svcName}); sErr == nil && svc != nil {
			url = svc.Uri
			s.Logger.Info().Str("function", fn.Name).Str("derived_url", url).Msg("resolveGCFFromCloud: derived URL from underlying Cloud Run Service (Function.ServiceConfig.uri was empty)")
		} else if sErr != nil {
			s.Logger.Warn().Err(sErr).Str("service", svcName).Msg("resolveGCFFromCloud: GetService fallback failed")
		}
	}
	if url == "" {
		s.Logger.Warn().Str("function", fn.Name).Bool("has_service_config", fn.ServiceConfig != nil).Str("service", svcName).Msg("resolveGCFFromCloud: returning empty URL — neither Function.ServiceConfig.uri nor Service.uri populated")
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

// prewarmAllOverlays materialises the operator-configured prewarm pool
// (BUG-948). For each PrewarmOverlays entry: build the overlay image,
// then create up to N free Functions tagged with the overlay's
// content-hash. Any Functions already in the pool from a prior backend
// boot are counted, so a restart doesn't re-deploy a full set.
//
// Failures are logged + skipped per-entry; one bad image must not abort
// the whole prewarm. Each Function deploy debits the regional CPU
// quota — prewarm is a startup cost. Operators size the pool to match
// expected concurrency for the most common workload (e.g. gitlab-runner
// cache-permission containers) so the live path never pays the deploy
// cost on the critical path.
func (s *Server) prewarmAllOverlays(ctx context.Context) {
	for _, entry := range s.config.PrewarmOverlays {
		if err := s.prewarmOverlay(ctx, entry); err != nil {
			s.Logger.Warn().Str("image", entry.Image).Int("size", entry.Size).Err(err).Msg("prewarm overlay failed; pool will fill lazily")
			continue
		}
	}
}

// prewarmOverlay materialises one prewarm entry. Builds the overlay
// image, counts existing free Functions for the content-hash, and
// deploys (size - existing) new ones. Idempotent: subsequent boots
// only top up the pool to the configured size.
//
// The image ref is run through ResolveGCPImageURI so the BaseImageRef
// that feeds into OverlayContentTag matches what live ContainerCreate
// produces (which also resolves Docker Hub / external refs to AR
// proxy URIs in backend_impl.go::ContainerCreate). Without this,
// prewarm content-hashes would never match the live workload's
// content-hash and the warm pool would be invisible.
func (s *Server) prewarmOverlay(ctx context.Context, entry PrewarmOverlay) error {
	if entry.Size <= 0 || entry.Image == "" {
		return nil
	}
	resolved := gcpcommon.ResolveGCPImageURI(entry.Image, s.config.Project, s.config.Region)
	spec := OverlayImageSpec{
		BaseImageRef:        resolved,
		BootstrapBinaryPath: s.config.BootstrapBinaryPath,
		BootstrapBinaryHash: s.config.BootstrapBinaryHash,
	}
	contentTag := OverlayContentTag(spec)
	overlayURI, err := s.ensureOverlayImage(ctx, spec, contentTag)
	if err != nil {
		return fmt.Errorf("ensure overlay image: %w", err)
	}
	existing := s.countFreePoolEntries(ctx, contentTag)
	deficit := entry.Size - existing
	if deficit <= 0 {
		s.Logger.Info().Str("image", entry.Image).Str("contentTag", contentTag).Int("existing", existing).Int("size", entry.Size).Msg("prewarm: pool already at or above target; skipping")
		return nil
	}
	s.Logger.Info().Str("image", entry.Image).Str("contentTag", contentTag).Int("deficit", deficit).Msg("prewarm: deploying free pool entries")
	for i := 0; i < deficit; i++ {
		if err := s.deployFreePoolEntry(ctx, contentTag, overlayURI, i); err != nil {
			s.Logger.Warn().Str("image", entry.Image).Int("index", i).Err(err).Msg("prewarm: free pool entry deploy failed; aborting remainder of this overlay")
			return err
		}
	}
	return nil
}

// countFreePoolEntries counts Functions already in the pool for the
// content-hash with no allocation. A boot-time count avoids over-provisioning
// when a previous backend instance left a populated pool.
func (s *Server) countFreePoolEntries(ctx context.Context, contentTag string) int {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	filter := fmt.Sprintf(
		`labels.sockerless_managed:"true" AND labels.sockerless_overlay_hash:"%s" AND -labels.sockerless_allocation:*`,
		contentTag,
	)
	it := s.gcp.Functions.ListFunctions(ctx, &functionspb.ListFunctionsRequest{
		Parent: parent,
		Filter: filter,
	})
	count := 0
	for {
		_, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return count
		}
		count++
	}
	return count
}

// deployFreePoolEntry creates one free Function tagged for the overlay
// content-hash with no allocation. Mirrors the fresh-deploy path in
// backend_impl.go::deployFunction but skips the volume-attach step (no
// caller; volumes get attached on claim) and uses a deterministic
// per-prewarm name suffix ("pw" + index). The function ends up with
// the overlay image (UpdateService swap) but no allocation, so
// claimFreeFunction will pick it up on first request.
func (s *Server) deployFreePoolEntry(ctx context.Context, contentTag, overlayURI string, index int) error {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	stubObject := "sockerless-stub/sockerless-gcf-stub.zip"
	if err := stageStubSourceIfMissing(ctx, s.gcp.Storage, s.config.BuildBucket, stubObject); err != nil {
		return fmt.Errorf("stage stub source: %w", err)
	}
	funcName := fmt.Sprintf("skls-%s-pw%02d", contentTag, index)
	fullFunctionName := fmt.Sprintf("%s/functions/%s", parent, funcName)

	labels := map[string]string{
		"sockerless_managed":      "true",
		"sockerless_overlay_hash": contentTag,
		// No sockerless_allocation — the pool entry is FREE and ready
		// to be claimed by the next ContainerCreate that wants this
		// content-hash.
	}
	createReq := &functionspb.CreateFunctionRequest{
		Parent:     parent,
		FunctionId: funcName,
		Function: &functionspb.Function{
			Name:   fullFunctionName,
			Labels: labels,
			BuildConfig: &functionspb.BuildConfig{
				Runtime:    "go124",
				EntryPoint: "Stub",
				Source: &functionspb.Source{
					Source: &functionspb.Source_StorageSource{
						StorageSource: &functionspb.StorageSource{
							Bucket: s.config.BuildBucket,
							Object: stubObject,
						},
					},
				},
			},
			ServiceConfig: &functionspb.ServiceConfig{
				AvailableCpu:    s.config.CPU,
				AvailableMemory: s.config.Memory,
				TimeoutSeconds:  int32(s.config.Timeout),
			},
		},
	}
	op, err := s.gcp.Functions.CreateFunction(ctx, createReq)
	if err != nil {
		return fmt.Errorf("create prewarm function: %w", err)
	}
	fn, err := op.Wait(ctx)
	if err != nil {
		// Best-effort delete; skipping if it fails (sweeper will reclaim later).
		if delOp, delErr := s.gcp.Functions.DeleteFunction(context.Background(), &functionspb.DeleteFunctionRequest{Name: fullFunctionName}); delErr == nil {
			_ = delOp.Wait(context.Background())
		}
		return fmt.Errorf("wait prewarm function create: %w", err)
	}
	if err := s.swapServiceImage(ctx, fn, overlayURI); err != nil {
		if delOp, delErr := s.gcp.Functions.DeleteFunction(context.Background(), &functionspb.DeleteFunctionRequest{Name: fullFunctionName}); delErr == nil {
			_ = delOp.Wait(context.Background())
		}
		return fmt.Errorf("swap prewarm function image: %w", err)
	}
	return nil
}

// resolvePodServiceFromCloud finds the multi-container pod Cloud Run
// Service backing this container ID. Pod members are tracked via:
//
//   - Service.labels.sockerless_allocation = short(MAIN container ID)
//   - Service.annotations.sockerless_pod_members = "<id1>,<id2>,..."
//
// MAIN members hit the allocation label (server-side filter); sidecar
// members hit the annotation match (client-side scan). Both return
// the same Service URL — gcf invokes only the main bootstrap, which
// dispatches to sidecars via shared loopback.
func (s *Server) resolvePodServiceFromCloud(ctx context.Context, containerID string) (GCFState, bool) {
	if s.gcp == nil || s.gcp.Services == nil {
		return GCFState{}, false
	}
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	short := shortAllocLabel(containerID)

	it := s.gcp.Services.ListServices(ctx, &runpb.ListServicesRequest{Parent: parent})
	for {
		svc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return GCFState{}, false
		}
		if svc.Labels["sockerless_managed"] != "true" {
			continue
		}
		// Match by allocation label first (cheap, server-side filterable).
		labelMatch := svc.Labels["sockerless_allocation"] == short
		// ListServices can return abbreviated Annotations in some pagination
		// modes; if the candidate Service has the right name shape but no
		// annotation visible, follow up with GetService for the full proto
		// so the sidecar (annotation-only) lookup doesn't miss.
		if !labelMatch && svc.Annotations["sockerless_pod_members"] == "" &&
			strings.Contains(svc.Name, "/services/sockerless-svc-") {
			if full, ferr := s.gcp.Services.GetService(ctx, &runpb.GetServiceRequest{Name: svc.Name}); ferr == nil && full != nil {
				svc = full
			}
		}
		if !labelMatch && !annotationContainsContainer(svc.Annotations["sockerless_pod_members"], containerID) {
			continue
		}
		shortName := shortFunctionName(svc.Name)
		return GCFState{
			FunctionName: shortName,
			FunctionURL:  svc.Uri,
			LogResource:  shortName,
		}, true
	}
	return GCFState{}, false
}

// annotationContainsContainer returns true if the comma-separated
// container-ID list in `annotation` contains `containerID` (full or
// 32-char short form). Used to match sidecar pod members during
// resolveGCFFromCloud's fallback path.
func annotationContainsContainer(annotation, containerID string) bool {
	if annotation == "" || containerID == "" {
		return false
	}
	short := shortAllocLabel(containerID)
	for _, mid := range strings.Split(annotation, ",") {
		mid = strings.TrimSpace(mid)
		if mid == containerID || mid == short {
			return true
		}
	}
	return false
}
