package core

import (
	"fmt"
	"strings"
)

// SharedVolumes is the operator-declared set of workspace volumes a
// backend shares with the process that calls it. When sockerless runs
// inside a cloud workload that already mounts a volume at some path, and
// a docker client inside that workload binds the same path into a new
// container, the bind is rewritten to a named volume backed by the same
// cloud storage, so caller and container share one workspace.
//
// Every cloud backend carries one of these in its Config and reads it
// with the same three questions: which entry is mounted at this source
// path, which entry has this name, and is this path underneath any entry.
type SharedVolumes []SharedVolumeRef

// BySourcePath returns the entry whose ContainerPath equals path.
func (v SharedVolumes) BySourcePath(path string) *SharedVolumeRef {
	for i := range v {
		if v[i].ContainerPath == path {
			return &v[i]
		}
	}
	return nil
}

// ByName returns the entry whose Name equals name.
func (v SharedVolumes) ByName(name string) *SharedVolumeRef {
	for i := range v {
		if v[i].Name == name {
			return &v[i]
		}
	}
	return nil
}

// IsSubPath reports whether path is a strict descendant of any entry's
// ContainerPath. A sibling that merely shares a string prefix
// (`/work` and `/workspace`) is not a sub-path.
func (v SharedVolumes) IsSubPath(path string) bool {
	for i := range v {
		base := v[i].ContainerPath
		if base != "" && strings.HasPrefix(path, base+"/") {
			return true
		}
	}
	return false
}

// SharedVolumeField names one positional field of a shared-volume
// declaration. Each cloud family declares which fields its
// SOCKERLESS_*_SHARED_VOLUMES variable carries and in what order.
type SharedVolumeField int

const (
	SharedVolumeFieldName SharedVolumeField = iota
	SharedVolumeFieldContainerPath
	SharedVolumeFieldBacking
	SharedVolumeFieldEFSAccessPoint
	SharedVolumeFieldEFSFileSystem
	SharedVolumeFieldEFSSubpath
	SharedVolumeFieldGCSBucket
	SharedVolumeFieldAzureShare
	SharedVolumeFieldAzureAccount
)

// SharedVolumeFormat describes the `=`-separated tuple a cloud family's
// shared-volume variable accepts.
type SharedVolumeFormat struct {
	// Usage is the operator-facing grammar quoted in parse errors, e.g.
	// `name=containerPath=fsap-XXXX[=fs-YYYY]`.
	Usage string
	// Fields lists the tuple positions in order. Fields is the maximum
	// arity; a shorter tuple leaves the trailing fields empty.
	Fields []SharedVolumeField
	// Required is how many leading fields must be present and non-empty.
	Required int
}

// ParseSharedVolumes parses a comma-separated list of `=`-tuples in the
// given format. Empty input yields (nil, nil). A malformed entry is an
// error the caller surfaces at startup, so a misconfigured mapping fails
// the backend rather than silently dropping the volume.
func ParseSharedVolumes(s string, format SharedVolumeFormat) (SharedVolumes, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out SharedVolumes
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "=")
		if len(parts) < format.Required || len(parts) > len(format.Fields) {
			return nil, fmt.Errorf("entry %q malformed: want %s", entry, format.Usage)
		}
		var ref SharedVolumeRef
		for i, raw := range parts {
			value := strings.TrimSpace(raw)
			if i < format.Required && value == "" {
				return nil, fmt.Errorf("entry %q malformed: the first %d fields of %s must all be non-empty", entry, format.Required, format.Usage)
			}
			ref.set(format.Fields[i], value)
		}
		out = append(out, ref)
	}
	return out, nil
}

func (r *SharedVolumeRef) set(field SharedVolumeField, value string) {
	switch field {
	case SharedVolumeFieldName:
		r.Name = value
	case SharedVolumeFieldContainerPath:
		r.ContainerPath = value
	case SharedVolumeFieldBacking:
		r.Backing = StorageBacking(value)
	case SharedVolumeFieldEFSAccessPoint:
		r.EFSAccessPointID = value
	case SharedVolumeFieldEFSFileSystem:
		r.EFSFileSystemID = value
	case SharedVolumeFieldEFSSubpath:
		r.EFSSubpath = strings.Trim(value, "/")
	case SharedVolumeFieldGCSBucket:
		r.GCSBucket = value
	case SharedVolumeFieldAzureShare:
		r.AzureShareName = value
	case SharedVolumeFieldAzureAccount:
		r.AzureStorageAccount = value
	}
}

// AzureAccountOrDefault returns the storage account the volume pins, or
// def when the declaration left it to the backend's configured account.
func (r SharedVolumeRef) AzureAccountOrDefault(def string) string {
	if r.AzureStorageAccount != "" {
		return r.AzureStorageAccount
	}
	return def
}

// WithAzureAccountDefault returns a copy whose AzureStorageAccount is
// filled from def when the declaration left it empty.
func (r SharedVolumeRef) WithAzureAccountDefault(def string) SharedVolumeRef {
	r.AzureStorageAccount = r.AzureAccountOrDefault(def)
	return r
}
