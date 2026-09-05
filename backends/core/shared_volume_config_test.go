package core

import (
	"strings"
	"testing"
)

// testFormat is a four-field tuple whose first two fields are required,
// exercising every rule the per-cloud formats rely on: arity bounds,
// required non-empty leading fields, optional trailing fields that may be
// empty, and per-field normalisation.
var testFormat = SharedVolumeFormat{
	Usage:    "name=containerPath[=bucket[=subpath]]",
	Fields:   []SharedVolumeField{SharedVolumeFieldName, SharedVolumeFieldContainerPath, SharedVolumeFieldGCSBucket, SharedVolumeFieldEFSSubpath},
	Required: 2,
}

func TestParseSharedVolumes(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		for _, in := range []string{"", "   ", " , "} {
			vols, err := ParseSharedVolumes(in, testFormat)
			if err != nil || len(vols) != 0 {
				t.Fatalf("ParseSharedVolumes(%q) = %v, %v; want empty, nil", in, vols, err)
			}
		}
	})

	t.Run("fields land on the declared members", func(t *testing.T) {
		vols, err := ParseSharedVolumes(" ws = /home/runner/_work = ws-bucket , ext=/ext==/sub/dir/ , min=/m", testFormat)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vols) != 3 {
			t.Fatalf("got %d volumes, want 3", len(vols))
		}
		if vols[0].Name != "ws" || vols[0].ContainerPath != "/home/runner/_work" || vols[0].GCSBucket != "ws-bucket" || vols[0].EFSSubpath != "" {
			t.Errorf("vols[0] = %+v", vols[0])
		}
		if vols[1].GCSBucket != "" || vols[1].EFSSubpath != "sub/dir" {
			t.Errorf("vols[1] = %+v (subpath must be trimmed of slashes, empty optional field allowed)", vols[1])
		}
		if vols[2].Name != "min" || vols[2].GCSBucket != "" {
			t.Errorf("vols[2] = %+v", vols[2])
		}
	})

	t.Run("malformed entries error and name the usage", func(t *testing.T) {
		for _, in := range []string{
			"ws",                  // too few fields
			"ws=/p=b=s=extra",     // too many fields
			"=/home/runner/_work", // empty required name
			"ws=",                 // empty required path
			"ok=/a,ws",            // one valid + one malformed
			"ws=/w=b=s, =/x",      // second entry empty name
		} {
			_, err := ParseSharedVolumes(in, testFormat)
			if err == nil {
				t.Errorf("ParseSharedVolumes(%q) = nil error, want parse error", in)
				continue
			}
			if !strings.Contains(err.Error(), testFormat.Usage) {
				t.Errorf("error %q does not quote the usage %q", err, testFormat.Usage)
			}
		}
	})
}

func TestSharedVolumesLookups(t *testing.T) {
	vols := SharedVolumes{
		{Name: "ws", ContainerPath: "/home/runner/_work"},
		{Name: "ext", ContainerPath: "/home/runner/externals"},
		{Name: "unmounted"},
	}
	if sv := vols.BySourcePath("/home/runner/externals"); sv == nil || sv.Name != "ext" {
		t.Fatalf("BySourcePath = %+v", sv)
	}
	if vols.BySourcePath("/home/runner/_work/_temp") != nil {
		t.Fatal("BySourcePath must match exactly, not by prefix")
	}
	if sv := vols.ByName("ws"); sv == nil || sv.ContainerPath != "/home/runner/_work" {
		t.Fatalf("ByName = %+v", sv)
	}
	if vols.ByName("nope") != nil {
		t.Fatal("ByName of an unknown name must be nil")
	}
	if !vols.IsSubPath("/home/runner/_work/_temp/_github_home") {
		t.Fatal("descendant must be a sub-path")
	}
	if vols.IsSubPath("/home/runner/_workspace") {
		t.Fatal("a sibling sharing a string prefix is not a sub-path")
	}
	if vols.IsSubPath("/home/runner/_work") {
		t.Fatal("the path itself is not a strict sub-path")
	}
	if vols.IsSubPath("/anything") {
		t.Fatal("an entry with no ContainerPath must not match everything")
	}
}

func TestSharedVolumeRefAzureAccountDefault(t *testing.T) {
	sv := SharedVolumeRef{Name: "ws", ContainerPath: "/w", AzureShareName: "share", Backing: BackingAzureFilesEphemeral}
	if got := sv.AzureAccountOrDefault("defacct"); got != "defacct" {
		t.Errorf("AzureAccountOrDefault = %q, want default", got)
	}
	if got := sv.WithAzureAccountDefault("defacct").AzureStorageAccount; got != "defacct" {
		t.Errorf("WithAzureAccountDefault = %q, want default", got)
	}
	sv.AzureStorageAccount = "pinned"
	if got := sv.AzureAccountOrDefault("defacct"); got != "pinned" {
		t.Errorf("AzureAccountOrDefault = %q, want pinned", got)
	}
	if got := sv.WithAzureAccountDefault("defacct"); got.AzureStorageAccount != "pinned" || got.AzureShareName != "share" {
		t.Errorf("WithAzureAccountDefault = %+v", got)
	}
}
