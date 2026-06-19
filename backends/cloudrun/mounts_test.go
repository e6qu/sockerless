package cloudrun

import (
	"testing"

	runpb "cloud.google.com/go/run/apiv2/runpb"
)

func TestCloudRunMounts(t *testing.T) {
	got := cloudRunMounts([]*runpb.VolumeMount{
		{Name: "data", MountPath: "/var/data"},
		nil,
		{Name: "cache", MountPath: "/cache"},
	})
	if len(got) != 2 {
		t.Fatalf("want 2 mounts (nil skipped), got %d", len(got))
	}
	m := got[0]
	if m.Type != "volume" || m.Name != "data" || m.Source != "data" || m.Destination != "/var/data" || !m.RW || m.Mode != "rw" {
		t.Fatalf("mount[0] = %+v", m)
	}
	if got[1].Destination != "/cache" || got[1].Name != "cache" {
		t.Fatalf("mount[1] = %+v", got[1])
	}
}
