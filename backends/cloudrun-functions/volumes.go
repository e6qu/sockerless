package gcf

import (
	"context"
	"fmt"
	"strings"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	"cloud.google.com/go/storage"
	gcpcommon "github.com/sockerless/gcp-common"
)

// GCS-backed named-volume provisioning for Cloud Functions v2.
//
// Cloud Functions v2's public API (functionspb.ServiceConfig) exposes
// only SecretVolumes — no first-class GCS volume primitive. But v2 IS a
// Cloud Run Service underneath: `Function.ServiceConfig.Service` returns
// the resource name of the backing service. The documented escape hatch
// is to call Services.GetService on that name, modify the
// RevisionTemplate.Volumes + Container.VolumeMounts, and UpdateService.
//
// Volume CRUD reuses gcpcommon.BucketManager shared with the Cloud Run
// backend. ContainerStart (backend_impl.go) calls this file's helpers
// to provision buckets up-front and attach them to the underlying CR
// Service after Functions.CreateFunction returns.
//
// Host-path bind specs (`/h:/c`) stay rejected — Cloud Functions
// containers have no host filesystem to bind from.

// gcsVolumeState embeds the shared BucketManager. Initialised by
// NewServer once the storage client is available.
type gcsVolumeState struct {
	buckets *gcpcommon.BucketManager
}

func (s *Server) bucketForVolume(ctx context.Context, volName string) (string, error) {
	// SharedVolumes (operator-configured via SOCKERLESS_GCP_SHARED_VOLUMES)
	// pin the GCS bucket directly so the runner-task and sub-task land on
	// the same pre-created bucket the dispatcher mounted.
	if sv := s.config.LookupSharedVolumeByName(volName); sv != nil {
		return sv.Bucket, nil
	}
	return s.buckets.ForVolume(ctx, volName)
}

func (s *Server) deleteBucketForVolume(ctx context.Context, volName string, force bool) error {
	return s.buckets.DeleteForVolume(ctx, volName, force)
}

func (s *Server) listManagedBuckets(ctx context.Context) ([]*storage.BucketAttrs, error) {
	return s.buckets.ListManaged(ctx)
}

// attachVolumesToFunctionService parses a slice of Docker bind specs
// (`volName:/mnt[:ro]`), provisions a GCS bucket per unique named
// volume, fetches the underlying Cloud Run Service backing the
// function, and ensures the matching Volume + VolumeMount entries are
// present in its RevisionTemplate. Escape hatch because Functions v2's
// ServiceConfig has no first-class Volumes primitive.
//
// Idempotent: existing volumes/mounts with matching names are kept
// (not duplicated); missing ones are appended; non-matching entries
// stay untouched. Required because the pool-reuse path calls this on
// functions that may already carry volumes from prior allocations —
// before this change the pool path skipped attach entirely (leaving
// `volumes: null` on reused functions, which broke /__w bind mounts
// for the runner-task → spawned container script handoff).
//
// Skips the UpdateService call entirely when the existing service
// already has every requested volume + mount — saves a pointless
// revision rollout that would only re-confirm the current state.
func (s *Server) attachVolumesToFunctionService(ctx context.Context, fn *functionspb.Function, binds []string) error {
	if fn == nil || fn.ServiceConfig == nil || fn.ServiceConfig.Service == "" {
		return fmt.Errorf("function has no underlying Cloud Run Service — cannot attach volumes")
	}
	svcName := fn.ServiceConfig.Service

	// Build volume + mount lists from the binds, deduping by volume name.
	volumesByName := map[string]string{} // volName → bucket
	mountsByName := map[string]string{}  // volName → mountPath
	for _, b := range binds {
		parts := strings.SplitN(b, ":", 3)
		if len(parts) < 2 {
			return fmt.Errorf("invalid bind %q", b)
		}
		volName, mountPath := parts[0], parts[1]
		bucket, err := s.bucketForVolume(ctx, volName)
		if err != nil {
			return fmt.Errorf("provision bucket for %q: %w", volName, err)
		}
		volumesByName[volName] = bucket
		mountsByName[volName] = mountPath
	}

	svc, err := s.gcp.Services.GetService(ctx, &runpb.GetServiceRequest{Name: svcName})
	if err != nil {
		return fmt.Errorf("get underlying Cloud Run Service %q: %w", svcName, err)
	}
	if svc.Template == nil {
		return fmt.Errorf("underlying Cloud Run Service %q has no RevisionTemplate", svcName)
	}

	// Index existing volumes + mounts by name for idempotent merge.
	existingVolumes := map[string]bool{}
	for _, v := range svc.Template.Volumes {
		existingVolumes[v.Name] = true
	}
	existingMounts := map[string]bool{}
	if len(svc.Template.Containers) > 0 {
		for _, m := range svc.Template.Containers[0].VolumeMounts {
			existingMounts[m.Name] = true
		}
	}

	added := 0
	for name, bucket := range volumesByName {
		if !existingVolumes[name] {
			svc.Template.Volumes = append(svc.Template.Volumes, &runpb.Volume{
				Name: name,
				VolumeType: &runpb.Volume_Gcs{
					Gcs: &runpb.GCSVolumeSource{
						Bucket:       bucket,
						MountOptions: gcpcommon.RunnerWorkspaceMountOptions(),
					},
				},
			})
			added++
		}
	}
	if len(svc.Template.Containers) > 0 {
		for name, mountPath := range mountsByName {
			if !existingMounts[name] {
				svc.Template.Containers[0].VolumeMounts = append(svc.Template.Containers[0].VolumeMounts, &runpb.VolumeMount{
					Name:      name,
					MountPath: mountPath,
				})
				added++
			}
		}
	}
	if added == 0 {
		// Service already has every requested volume + mount. Skip the
		// UpdateService rollout entirely.
		return nil
	}

	// Cloud Run requires a unique revision name for each update; clear
	// the existing one so a fresh suffix gets auto-generated.
	svc.Template.Revision = ""
	updateOp, err := s.gcp.Services.UpdateService(ctx, &runpb.UpdateServiceRequest{Service: svc})
	if err != nil {
		return fmt.Errorf("update Cloud Run Service %q: %w", svcName, err)
	}
	if _, err := updateOp.Wait(ctx); err != nil {
		return fmt.Errorf("wait for Cloud Run Service %q update: %w", svcName, err)
	}
	return nil
}
