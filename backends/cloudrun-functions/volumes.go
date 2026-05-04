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

	// Index existing volumes by name + capture their current GCS bucket +
	// MountOptions so we can detect "same name but stale config" cases.
	// Pool-reused functions inherited volumes from a prior deploy may
	// have outdated mount options (e.g. missing the runner-workspace
	// strong-consistency opts) — replace those entries so the next
	// revision picks up the corrected config.
	wantOpts := gcpcommon.RunnerWorkspaceMountOptions()
	wantOptsKey := strings.Join(wantOpts, ",")
	existingByName := map[string]*runpb.Volume{}
	for _, v := range svc.Template.Volumes {
		existingByName[v.Name] = v
	}
	existingMountByName := map[string]*runpb.VolumeMount{}
	if len(svc.Template.Containers) > 0 {
		for _, m := range svc.Template.Containers[0].VolumeMounts {
			existingMountByName[m.Name] = m
		}
	}

	changed := false
	for name, bucket := range volumesByName {
		want := &runpb.Volume{
			Name: name,
			VolumeType: &runpb.Volume_Gcs{
				Gcs: &runpb.GCSVolumeSource{
					Bucket:       bucket,
					MountOptions: wantOpts,
				},
			},
		}
		existing, ok := existingByName[name]
		if !ok {
			svc.Template.Volumes = append(svc.Template.Volumes, want)
			changed = true
			continue
		}
		// Same name — compare bucket + mount opts to detect stale config.
		// Either GCS field present (proto union) OR the read-back-as-CSI
		// shape (Cloud Run server normalises to CSI). For simplicity we
		// drop+replace if anything observable about the existing entry
		// doesn't match what we want.
		matches := false
		if g := existing.GetGcs(); g != nil {
			gotKey := strings.Join(g.GetMountOptions(), ",")
			if g.GetBucket() == bucket && gotKey == wantOptsKey {
				matches = true
			}
		}
		if !matches {
			// Replace in-place (preserve order so the diff is minimal).
			for i, v := range svc.Template.Volumes {
				if v.Name == name {
					svc.Template.Volumes[i] = want
					break
				}
			}
			changed = true
		}
	}
	if len(svc.Template.Containers) > 0 {
		for name, mountPath := range mountsByName {
			existing, ok := existingMountByName[name]
			if !ok {
				svc.Template.Containers[0].VolumeMounts = append(svc.Template.Containers[0].VolumeMounts, &runpb.VolumeMount{
					Name:      name,
					MountPath: mountPath,
				})
				changed = true
				continue
			}
			if existing.GetMountPath() != mountPath {
				existing.MountPath = mountPath
				changed = true
			}
		}
	}
	if !changed {
		// Service already has every requested volume + mount in the right
		// shape. Skip the UpdateService rollout entirely.
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
