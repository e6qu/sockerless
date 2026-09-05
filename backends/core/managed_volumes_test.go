package core

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/sockerless/api"
)

type fakeShare struct{ name, id string }

func TestInUseVolumeNames(t *testing.T) {
	pending := []api.Container{
		{HostConfig: api.HostConfig{Binds: []string{"cache:/cache:ro", "/host:/c", "./rel:/r", "junk"}}},
		{HostConfig: api.HostConfig{Binds: []string{"build:/b"}}},
	}
	got := InUseVolumeNames(pending)
	var names []string
	for n := range got {
		names = append(names, n)
	}
	sort.Strings(names)
	if want := []string{"build", "cache"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("InUseVolumeNames = %v, want %v", names, want)
	}
}

func TestManagedVolumeShaping(t *testing.T) {
	items := []fakeShare{{"ws", "id-1"}, {"cache", "id-2"}}
	toVolume := func(s fakeShare) *api.Volume { return &api.Volume{Name: s.name, Mountpoint: s.id} }

	list := ListManagedVolumes(items, toVolume)
	if len(list.Volumes) != 2 || list.Volumes[1].Name != "cache" {
		t.Fatalf("ListManagedVolumes = %+v", list.Volumes)
	}
	if empty := ListManagedVolumes(nil, toVolume); empty.Volumes == nil || len(empty.Volumes) != 0 {
		t.Fatalf("empty listing must be an empty, non-nil slice: %+v", empty.Volumes)
	}

	vol, err := InspectManagedVolume(items, "cache", func(s fakeShare) bool { return s.name == "cache" }, toVolume)
	if err != nil || vol.Mountpoint != "id-2" {
		t.Fatalf("InspectManagedVolume = %+v, %v", vol, err)
	}
	_, err = InspectManagedVolume(items, "nope", func(s fakeShare) bool { return s.name == "nope" }, toVolume)
	var nf *api.NotFoundError
	if !errors.As(err, &nf) || nf.Resource != "volume" || nf.ID != "nope" {
		t.Fatalf("InspectManagedVolume miss = %v, want volume not-found", err)
	}

	var deleted []string
	resp, err := PruneManagedVolumes(items, func(s fakeShare) string { return s.name }, map[string]struct{}{"ws": {}}, func(name string) error {
		deleted = append(deleted, name)
		return nil
	}, "Azure Files share")
	if err != nil || !reflect.DeepEqual(resp.VolumesDeleted, []string{"cache"}) || !reflect.DeepEqual(deleted, []string{"cache"}) {
		t.Fatalf("PruneManagedVolumes = %+v, %v (deleted %v)", resp, err, deleted)
	}
	_, err = PruneManagedVolumes(items, func(s fakeShare) string { return s.name }, nil, func(string) error { return errors.New("boom") }, "Azure Files share")
	var se *api.ServerError
	if !errors.As(err, &se) || se.Message != `delete Azure Files share for "ws": boom` {
		t.Fatalf("PruneManagedVolumes failure = %v", err)
	}
}

func TestContainerNameTaken(t *testing.T) {
	containers := []api.Container{{Name: "/web"}, {Name: "db"}}
	for name, want := range map[string]bool{"web": true, "/web": true, "db": true, "cache": false} {
		if got := ContainerNameTaken(containers, name); got != want {
			t.Errorf("ContainerNameTaken(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestProvisionCache(t *testing.T) {
	var c ProvisionCache
	finds, creates := 0, 0
	find := func() (string, bool, error) { finds++; return "", false, nil }
	create := func() (string, error) { creates++; return "res-1", nil }

	id, err := c.Ensure("ws", find, create)
	if err != nil || id != "res-1" || finds != 1 || creates != 1 {
		t.Fatalf("first Ensure = %q, %v (finds=%d creates=%d)", id, err, finds, creates)
	}
	id, err = c.Ensure("ws", find, create)
	if err != nil || id != "res-1" || finds != 1 || creates != 1 {
		t.Fatalf("cached Ensure must not touch the cloud: %q, %v (finds=%d creates=%d)", id, err, finds, creates)
	}

	existing := func() (string, bool, error) { return "res-existing", true, nil }
	if id, _ := c.Ensure("other", existing, create); id != "res-existing" || creates != 1 {
		t.Fatalf("an existing resource must be adopted, not re-created: %q creates=%d", id, creates)
	}

	if _, err := c.Ensure("bad", func() (string, bool, error) { return "", false, errors.New("list failed") }, create); err == nil {
		t.Fatal("a find error must surface")
	}

	if err := c.Forget("ws", func() error { return errors.New("delete failed") }); err == nil {
		t.Fatal("a delete error must surface and keep the cache entry")
	}
	if id, _ := c.Ensure("ws", find, create); id != "res-1" || creates != 1 {
		t.Fatalf("failed Forget must keep the cache: %q creates=%d", id, creates)
	}
	if err := c.Forget("ws", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Ensure("ws", find, create); err != nil || creates != 2 {
		t.Fatalf("after Forget the next Ensure must provision again: creates=%d", creates)
	}
}
