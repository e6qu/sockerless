package aca

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

// Phase 93 — Azure Files-backed named-volume + bind-mount provisioning
// for ACA (Jobs today; Apps when the UseApp path needs it).
//
// Docker volume semantics on ACA map to Azure Files shares:
//
//  - One Azure Files share per named volume, inside the operator-configured
//    storage account (`SOCKERLESS_ACA_STORAGE_ACCOUNT`).
//  - One ManagedEnvironmentsStorages resource per share, linking the
//    share to the managed environment so Jobs/Apps in that env can
//    reference it by storage name.
//  - Container spec Volume `{StorageType: AzureFile, StorageName}` +
//    VolumeMount translate at task-launch time into a mounted share.
//
// Host-path bind specs (`/h:/c`) stay rejected — ACA containers have
// no host filesystem to bind from.

const (
	azfVolumeManagedTag  = "sockerless-managed"
	azfVolumeShareTagVal = "true"
	azfVolumeNameTag     = "sockerless-volume-name"
	azfShareNamePrefix   = "sockerless-volume-"
)

// azureVolumeState caches volume-name → share-name + env-storage-name
// lookups. Lives on Server; initialised in NewServer.
type azureVolumeState struct {
	azVolMu    sync.Mutex
	azVolCache map[string]string // volume-name → share-name (same name used for env-storage)
}

// shareForVolume returns the share name bound to a Docker volume name,
// provisioning it + the matching managed-env storage entry on first
// call. Safe for concurrent callers.
func (s *Server) shareForVolume(ctx context.Context, volName string) (string, error) {
	s.azVolMu.Lock()
	defer s.azVolMu.Unlock()

	if name, ok := s.azVolCache[volName]; ok {
		return name, nil
	}
	if s.config.StorageAccount == "" {
		return "", fmt.Errorf("SOCKERLESS_ACA_STORAGE_ACCOUNT must be set to provision Azure Files shares for volumes")
	}

	shareName := azfShareName(volName)
	if _, err := s.aws_aca_ensureFileShare(ctx, shareName, volName); err != nil {
		return "", err
	}
	if err := s.aws_aca_ensureEnvStorage(ctx, shareName); err != nil {
		return "", err
	}
	s.azVolCache[volName] = shareName
	return shareName, nil
}

func (s *Server) aws_aca_ensureFileShare(ctx context.Context, shareName, volName string) (string, error) {
	existing, err := s.azure.FileShares.Get(ctx, s.config.ResourceGroup, s.config.StorageAccount, shareName, nil)
	if err == nil && existing.ID != nil {
		return shareName, nil
	}
	_, err = s.azure.FileShares.Create(ctx, s.config.ResourceGroup, s.config.StorageAccount, shareName,
		armstorage.FileShare{
			FileShareProperties: &armstorage.FileShareProperties{
				Metadata: map[string]*string{
					azfVolumeManagedTag: to.Ptr(azfVolumeShareTagVal),
					azfVolumeNameTag:    to.Ptr(sanitiseAzureMetaValue(volName)),
				},
			},
		}, nil)
	if err != nil {
		return "", fmt.Errorf("create file share %q: %w", shareName, err)
	}
	return shareName, nil
}

func (s *Server) aws_aca_ensureEnvStorage(ctx context.Context, shareName string) error {
	_, err := s.azure.EnvStorages.CreateOrUpdate(ctx,
		s.config.ResourceGroup, s.config.Environment, shareName,
		armappcontainers.ManagedEnvironmentStorage{
			Properties: &armappcontainers.ManagedEnvironmentStorageProperties{
				AzureFile: &armappcontainers.AzureFileProperties{
					AccountName: to.Ptr(s.config.StorageAccount),
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

// azfShareName returns an Azure-Files-safe share name bound to a
// Docker volume. Share names must be 3-63 chars, lowercase letters,
// digits, and hyphens only, and can't start or end with a hyphen.
func azfShareName(volName string) string {
	safe := sanitiseAzureShareName(volName)
	name := azfShareNamePrefix + safe
	if len(name) > 63 {
		name = name[:63]
	}
	name = strings.Trim(name, "-")
	if name == "" {
		return "sockerless-volume"
	}
	return name
}

func sanitiseAzureShareName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// sanitiseAzureMetaValue returns an Azure-metadata-safe value (ASCII
// letters, digits, dashes, underscores; max 63 chars).
func sanitiseAzureMetaValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > 63 {
		out = out[:63]
	}
	if out == "" {
		return "_"
	}
	return out
}

// deleteShareForVolume removes the env-storage + file-share backing a
// volume. The storage account is left in place so other volumes keep
// working.
func (s *Server) deleteShareForVolume(ctx context.Context, volName string) error {
	s.azVolMu.Lock()
	defer s.azVolMu.Unlock()

	shareName := azfShareName(volName)
	// Delete env storage first (it references the share).
	_, err := s.azure.EnvStorages.Delete(ctx, s.config.ResourceGroup, s.config.Environment, shareName, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Str("storage", shareName).Msg("delete env storage (non-fatal)")
	}
	_, err = s.azure.FileShares.Delete(ctx, s.config.ResourceGroup, s.config.StorageAccount, shareName, nil)
	if err != nil {
		return fmt.Errorf("delete file share %q: %w", shareName, err)
	}
	delete(s.azVolCache, volName)
	return nil
}

// listManagedShares returns every file share in the configured storage
// account whose metadata marks it as a sockerless-owned Docker volume.
func (s *Server) listManagedShares(ctx context.Context) ([]*armstorage.FileShareItem, error) {
	if s.config.StorageAccount == "" {
		return nil, nil
	}
	var out []*armstorage.FileShareItem
	pager := s.azure.FileShares.NewListPager(s.config.ResourceGroup, s.config.StorageAccount, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list file shares: %w", err)
		}
		for _, it := range page.Value {
			if shareIsManaged(it) {
				out = append(out, it)
			}
		}
	}
	return out, nil
}

func shareIsManaged(it *armstorage.FileShareItem) bool {
	if it == nil || it.Properties == nil || it.Properties.Metadata == nil {
		return false
	}
	v, ok := it.Properties.Metadata[azfVolumeManagedTag]
	if !ok || v == nil {
		return false
	}
	return *v == azfVolumeShareTagVal
}

func shareVolumeName(it *armstorage.FileShareItem) string {
	if it == nil || it.Properties == nil || it.Properties.Metadata == nil {
		return ""
	}
	v, ok := it.Properties.Metadata[azfVolumeNameTag]
	if !ok || v == nil {
		return ""
	}
	return *v
}
