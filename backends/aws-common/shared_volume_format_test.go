package awscommon

import (
	"testing"

	core "github.com/sockerless/backend-core"
)

func TestECSSharedVolumeFormat(t *testing.T) {
	vols, err := core.ParseSharedVolumes("ws=/home/runner/_work=fsap-123, ext=/home/runner/externals=fsap-456=fs-789", ECSSharedVolumeFormat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vols) != 2 {
		t.Fatalf("got %d volumes, want 2", len(vols))
	}
	if vols[0].Name != "ws" || vols[0].ContainerPath != "/home/runner/_work" || vols[0].EFSAccessPointID != "fsap-123" || vols[0].EFSFileSystemID != "" {
		t.Errorf("vols[0] = %+v", vols[0])
	}
	if vols[1].Name != "ext" || vols[1].EFSAccessPointID != "fsap-456" || vols[1].EFSFileSystemID != "fs-789" {
		t.Errorf("vols[1] = %+v", vols[1])
	}
	for _, in := range []string{
		"ws=/home/runner/_work",        // too few fields
		"ws=/p=fsap-1=fs-1=extra",      // too many fields
		"=/home/runner/_work=fsap-1",   // empty name
		"ws==fsap-1",                   // empty containerPath
		"ws=/home/runner/_work=",       // empty access point
		"ok=/a=fsap-1,ws=/home/runner", // one valid + one malformed
	} {
		if _, err := core.ParseSharedVolumes(in, ECSSharedVolumeFormat); err == nil {
			t.Errorf("ParseSharedVolumes(%q) = nil error, want parse error", in)
		}
	}
}

func TestLambdaSharedVolumeFormat(t *testing.T) {
	vols, err := core.ParseSharedVolumes("ws=/home/runner/_work=fsap-1, ext=/home/runner/externals=fsap-1==externals, tool=/opt/tool=fsap-1=fs-9=/tooldir/", LambdaSharedVolumeFormat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vols) != 3 {
		t.Fatalf("got %d volumes, want 3", len(vols))
	}
	if vols[0].Name != "ws" || vols[0].EFSAccessPointID != "fsap-1" || vols[0].EFSSubpath != "" {
		t.Errorf("vols[0] = %+v", vols[0])
	}
	if vols[1].EFSFileSystemID != "" || vols[1].EFSSubpath != "externals" {
		t.Errorf("vols[1] = %+v (file system may be empty when only the sub-path is declared)", vols[1])
	}
	if vols[2].EFSFileSystemID != "fs-9" || vols[2].EFSSubpath != "tooldir" {
		t.Errorf("vols[2] = %+v (sub-path must be trimmed of slashes)", vols[2])
	}
	for _, in := range []string{
		"ws=/home/runner/_work",           // too few fields
		"ws=/p=fsap-1=fs-1=subpath=extra", // too many fields
		"=/home/runner/_work=fsap-1",      // empty name
		"ws=/home/runner/_work=",          // empty access point
		"ok=/a=fsap-1,ws=/home/runner",    // one valid + one malformed
	} {
		if _, err := core.ParseSharedVolumes(in, LambdaSharedVolumeFormat); err == nil {
			t.Errorf("ParseSharedVolumes(%q) = nil error, want parse error", in)
		}
	}
}
