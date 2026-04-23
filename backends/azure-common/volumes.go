package azurecommon

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

// FileShareManager owns sockerless-managed Azure Files shares backing
// Docker named volumes for every Azure backend that can mount file
// shares (ACA today, AZF in Phase 94). One share per Docker volume,
// inside the operator-configured storage account, with this metadata:
//
//   - sockerless-managed=true  → identifies sockerless-owned shares
//   - sockerless-volume-name=<sanitised-docker-name> → round-trip mapping
//
// Each backend layers its own mount-attach semantics on top:
//   - ACA adds a ManagedEnvironmentsStorages entry linking the share to
//     the managed environment so Jobs/Apps can reference it by name.
//   - AZF (Phase 94) adds a `sites/<fn>/config/azurestorageaccounts/<mount>`
//     sub-resource linking the share to the function app.
//
// The share itself is identical across backends, so it's managed here.
type FileShareManager struct {
	Client         *armstorage.FileSharesClient
	ResourceGroup  string
	StorageAccount string

	mu    sync.Mutex
	cache map[string]string // docker-name → share-name
}

// NewFileShareManager wires a FileShareManager against a shares client.
func NewFileShareManager(client *armstorage.FileSharesClient, resourceGroup, storageAccount string) *FileShareManager {
	return &FileShareManager{
		Client:         client,
		ResourceGroup:  resourceGroup,
		StorageAccount: storageAccount,
		cache:          make(map[string]string),
	}
}

const (
	VolumeManagedTag  = "sockerless-managed"
	VolumeShareTagVal = "true"
	VolumeNameTag     = "sockerless-volume-name"
	ShareNamePrefix   = "sockerless-volume-"
)

// EnsureShare provisions the Azure Files share backing a Docker volume,
// idempotently. Returns the share name.
func (m *FileShareManager) EnsureShare(ctx context.Context, volName string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if name, ok := m.cache[volName]; ok {
		return name, nil
	}
	if m.StorageAccount == "" {
		return "", fmt.Errorf("sockerless storage account must be configured to provision Azure Files shares for volumes")
	}

	shareName := ShareName(volName)
	existing, err := m.Client.Get(ctx, m.ResourceGroup, m.StorageAccount, shareName, nil)
	if err == nil && existing.ID != nil {
		m.cache[volName] = shareName
		return shareName, nil
	}
	_, err = m.Client.Create(ctx, m.ResourceGroup, m.StorageAccount, shareName,
		armstorage.FileShare{
			FileShareProperties: &armstorage.FileShareProperties{
				Metadata: map[string]*string{
					VolumeManagedTag: to.Ptr(VolumeShareTagVal),
					VolumeNameTag:    to.Ptr(SanitiseMetaValue(volName)),
				},
			},
		}, nil)
	if err != nil {
		return "", fmt.Errorf("create file share %q: %w", shareName, err)
	}
	m.cache[volName] = shareName
	return shareName, nil
}

// DeleteShare removes the Azure Files share backing a Docker volume. The
// backend-specific mount-attach resource (env-storage / site-config)
// must be deleted first — FileShareManager does not know about either.
func (m *FileShareManager) DeleteShare(ctx context.Context, volName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	shareName := ShareName(volName)
	_, err := m.Client.Delete(ctx, m.ResourceGroup, m.StorageAccount, shareName, nil)
	if err != nil {
		return fmt.Errorf("delete file share %q: %w", shareName, err)
	}
	delete(m.cache, volName)
	return nil
}

// InvalidateCache forgets a cached docker-name → share-name binding so a
// subsequent EnsureShare call re-checks the backing share. Useful after a
// backend-specific attach step fails and the caller wants to retry.
func (m *FileShareManager) InvalidateCache(volName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cache, volName)
}

// ListManaged returns every file share in the configured storage account
// whose metadata marks it as a sockerless-owned Docker volume.
func (m *FileShareManager) ListManaged(ctx context.Context) ([]*armstorage.FileShareItem, error) {
	if m.StorageAccount == "" {
		return nil, nil
	}
	var out []*armstorage.FileShareItem
	pager := m.Client.NewListPager(m.ResourceGroup, m.StorageAccount, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list file shares: %w", err)
		}
		for _, it := range page.Value {
			if ShareIsManaged(it) {
				out = append(out, it)
			}
		}
	}
	return out, nil
}

// ShareIsManaged reports whether the file share metadata carries the
// sockerless-managed marker.
func ShareIsManaged(it *armstorage.FileShareItem) bool {
	if it == nil || it.Properties == nil || it.Properties.Metadata == nil {
		return false
	}
	v, ok := it.Properties.Metadata[VolumeManagedTag]
	if !ok || v == nil {
		return false
	}
	return *v == VolumeShareTagVal
}

// ShareVolumeName returns the Docker volume name encoded in the share
// metadata, or empty if unmanaged.
func ShareVolumeName(it *armstorage.FileShareItem) string {
	if it == nil || it.Properties == nil || it.Properties.Metadata == nil {
		return ""
	}
	v, ok := it.Properties.Metadata[VolumeNameTag]
	if !ok || v == nil {
		return ""
	}
	return *v
}

// ShareName returns an Azure-Files-safe share name bound to a Docker
// volume. Share names must be 3-63 chars, lowercase letters, digits,
// and hyphens only, and can't start or end with a hyphen.
func ShareName(volName string) string {
	safe := SanitiseShareName(volName)
	name := ShareNamePrefix + safe
	if len(name) > 63 {
		name = name[:63]
	}
	name = strings.Trim(name, "-")
	if name == "" {
		return "sockerless-volume"
	}
	return name
}

// SanitiseShareName folds a string down to the Azure-Files-share charset.
func SanitiseShareName(s string) string {
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

// SanitiseMetaValue returns an Azure-metadata-safe value (ASCII letters,
// digits, dashes, underscores; max 63 chars).
func SanitiseMetaValue(s string) string {
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
