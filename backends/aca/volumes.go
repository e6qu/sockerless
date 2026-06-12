package aca

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	azurecommon "github.com/sockerless/azure-common"
)

// Azure Files-backed named-volume + bind-mount provisioning for ACA
// (Jobs today; Apps via the UseApp path).
//
// Docker volume semantics on ACA map to:
//
//  - One Azure Files share per named volume (managed by
//    azurecommon.FileShareManager, shared with AZF).
//  - One ManagedEnvironmentsStorages entry per share, linking the share
//    to the managed environment so Jobs/Apps can reference it by name.
//    This linkage is ACA-specific and lives in this file; the share
//    itself is identical across backends so it's managed in common.
//
// Host-path bind specs (`/h:/c`) stay rejected — ACA containers have
// no host filesystem to bind from.

// azureVolumeState embeds the shared FileShareManager. Initialised by
// NewServer once the storage + env-storage clients are available.
type azureVolumeState struct {
	shares *azurecommon.FileShareManager
}

// shareForVolume returns the share name bound to a Docker volume,
// provisioning the share + the matching managed-env storage entry on
// first call. Safe for concurrent callers.
//
// Operator-declared SharedVolumes (SOCKERLESS_ACA_SHARED_VOLUMES) pin
// the Azure Files share directly — that's how the runner-task and the
// spawned sub-task land on the SAME pre-provisioned share without
// sockerless creating a fresh one per docker run. The managed-env
// storage linkage is still ensured (idempotent CreateOrUpdate) so the
// spawned job/app can reference the share by name.
func (s *Server) shareForVolume(ctx context.Context, volName string) (string, error) {
	if sv := s.config.LookupSharedVolumeByName(volName); sv != nil {
		if err := s.ensureEnvStorage(ctx, sv.ShareName, sv.AccountOrDefault(s.config.StorageAccount)); err != nil {
			return "", err
		}
		return sv.ShareName, nil
	}
	shareName, err := s.shares.EnsureShare(ctx, volName)
	if err != nil {
		return "", err
	}
	if err := s.ensureEnvStorage(ctx, shareName, s.config.StorageAccount); err != nil {
		// Share is present but the ACA linkage failed — invalidate the
		// cache so the next call retries both steps.
		s.shares.InvalidateCache(volName)
		return "", err
	}
	return shareName, nil
}

func (s *Server) ensureEnvStorage(ctx context.Context, shareName, accountName string) error {
	_, err := s.azure.EnvStorages.CreateOrUpdate(ctx,
		s.config.ResourceGroup, s.config.Environment, shareName,
		armappcontainers.ManagedEnvironmentStorage{
			Properties: &armappcontainers.ManagedEnvironmentStorageProperties{
				AzureFile: &armappcontainers.AzureFileProperties{
					AccountName: to.Ptr(accountName),
					ShareName:   to.Ptr(shareName),
					AccessMode:  to.Ptr(armappcontainers.AccessModeReadWrite),
				},
			},
		}, nil)
	if err != nil {
		return fmt.Errorf("create env storage %q: %w", shareName, err)
	}
	return nil
}

// deleteShareForVolume removes the env-storage entry (ACA-specific) + the
// underlying Azure Files share. Storage account stays in place.
// Operator-declared SharedVolumes are operator-owned — sockerless never
// deletes their shares.
func (s *Server) deleteShareForVolume(ctx context.Context, volName string) error {
	if s.config.LookupSharedVolumeByName(volName) != nil {
		return nil
	}
	shareName := azurecommon.ShareName(volName)

	// Env storage references the share, so delete it first.
	_, err := s.azure.EnvStorages.Delete(ctx, s.config.ResourceGroup, s.config.Environment, shareName, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Str("storage", shareName).Msg("delete env storage (non-fatal)")
	}
	return s.shares.DeleteShare(ctx, volName)
}

// listManagedShares is a thin wrapper so existing callers keep compiling.
func (s *Server) listManagedShares(ctx context.Context) ([]*armstorage.FileShareItem, error) {
	return s.shares.ListManaged(ctx)
}
