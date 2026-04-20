package core

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// fakeImageLister implements only the CloudStateProvider methods that
// ImageList uses, so we can exercise the merge path without spinning
// up a cloud SDK or a simulator.
type fakeImageLister struct {
	containers []api.Container
	images     []*api.ImageSummary
	err        error
}

func (f *fakeImageLister) GetContainer(ctx context.Context, ref string) (api.Container, bool, error) {
	for _, c := range f.containers {
		if c.ID == ref {
			return c, true, nil
		}
	}
	return api.Container{}, false, nil
}
func (f *fakeImageLister) ListContainers(ctx context.Context, all bool, filters map[string][]string) ([]api.Container, error) {
	return f.containers, nil
}
func (f *fakeImageLister) CheckNameAvailable(ctx context.Context, name string) (bool, error) {
	return true, nil
}
func (f *fakeImageLister) WaitForExit(ctx context.Context, containerID string) (int, error) {
	return 0, nil
}
func (f *fakeImageLister) ListImages(ctx context.Context) ([]*api.ImageSummary, error) {
	return f.images, f.err
}

func newServerWithLister(t *testing.T, lister CloudStateProvider) *BaseServer {
	t.Helper()
	s := NewBaseServer(NewStore(), BackendDescriptor{ID: "t", Name: "t"}, zerolog.Nop())
	s.CloudState = lister
	return s
}

// TestImageList_MergesCacheAndCloud — cache has one image, cloud has
// two, result has three deduped by ID.
func TestImageList_MergesCacheAndCloud(t *testing.T) {
	s := newServerWithLister(t, &fakeImageLister{
		images: []*api.ImageSummary{
			{ID: "sha256:cloud-a", RepoTags: []string{"registry/a:v1"}},
			{ID: "sha256:cloud-b", RepoTags: []string{"registry/b:v2"}},
		},
	})
	s.Store.Images.Put("sha256:cache-x", api.Image{ID: "sha256:cache-x", RepoTags: []string{"cached:v1"}, Created: "2026-04-20T10:00:00Z"})

	got, err := s.ImageList(api.ImageListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 summaries (1 cache + 2 cloud), got %d", len(got))
	}
	ids := map[string]bool{}
	for _, s := range got {
		ids[s.ID] = true
	}
	for _, want := range []string{"sha256:cache-x", "sha256:cloud-a", "sha256:cloud-b"} {
		if !ids[want] {
			t.Errorf("missing image ID %q in merged result", want)
		}
	}
}

// TestImageList_DedupesByID — the same image ID in cache and cloud
// appears only once in the merged result.
func TestImageList_DedupesByID(t *testing.T) {
	s := newServerWithLister(t, &fakeImageLister{
		images: []*api.ImageSummary{
			{ID: "sha256:same", RepoTags: []string{"registry/same:v1"}},
		},
	})
	s.Store.Images.Put("sha256:same", api.Image{ID: "sha256:same", RepoTags: []string{"same:v1"}, Created: "2026-04-20T10:00:00Z"})

	got, err := s.ImageList(api.ImageListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 summary (dedupe by ID), got %d", len(got))
	}
}

// TestImageList_CloudErrorReturnsCacheOnly — cloud listing failure
// logs and returns whatever is in the cache (no propagation of the
// error to the caller; matches the semantics used by BaseServer).
func TestImageList_CloudErrorReturnsCacheOnly(t *testing.T) {
	s := newServerWithLister(t, &fakeImageLister{
		err: context.DeadlineExceeded,
	})
	s.Store.Images.Put("sha256:cache", api.Image{ID: "sha256:cache", RepoTags: []string{"cache:v1"}, Created: "2026-04-20T10:00:00Z"})

	got, err := s.ImageList(api.ImageListOptions{})
	if err != nil {
		t.Fatalf("list should swallow cloud error, got: %v", err)
	}
	if len(got) != 1 || got[0].ID != "sha256:cache" {
		t.Fatalf("expected cache-only result, got %+v", got)
	}
}

// fakePodLister — stub CloudStateProvider that also implements
// CloudPodLister for the PodList merge path.
type fakePodLister struct {
	containers []api.Container
	pods       []*api.PodListEntry
	err        error
}

func (f *fakePodLister) GetContainer(ctx context.Context, ref string) (api.Container, bool, error) {
	return api.Container{}, false, nil
}
func (f *fakePodLister) ListContainers(ctx context.Context, all bool, filters map[string][]string) ([]api.Container, error) {
	return f.containers, nil
}
func (f *fakePodLister) CheckNameAvailable(ctx context.Context, name string) (bool, error) {
	return true, nil
}
func (f *fakePodLister) WaitForExit(ctx context.Context, containerID string) (int, error) {
	return 0, nil
}
func (f *fakePodLister) ListPods(ctx context.Context) ([]*api.PodListEntry, error) {
	return f.pods, f.err
}

// TestPodList_MergesCacheAndCloud — local Store.Pods has one pod,
// cloud returns another, result has both.
func TestPodList_MergesCacheAndCloud(t *testing.T) {
	s := newServerWithLister(t, &fakePodLister{
		pods: []*api.PodListEntry{
			{ID: "pod-from-cloud", Name: "cloud-pod", Status: "Running"},
		},
	})
	_ = s.Store.Pods.CreatePodWithOpts("local-pod", nil, "", nil)

	got, err := s.PodList(api.PodListOptions{})
	if err != nil {
		t.Fatalf("pod list: %v", err)
	}
	names := make(map[string]bool)
	for _, p := range got {
		names[p.Name] = true
	}
	if !names["local-pod"] {
		t.Error("missing local-pod from result")
	}
	if !names["cloud-pod"] {
		t.Error("missing cloud-pod from result")
	}
}

// TestPodList_DedupesByID — when cloud returns a pod whose ID matches
// an in-memory pod, only the cache entry survives (first-wins).
func TestPodList_DedupesByID(t *testing.T) {
	s := NewBaseServer(NewStore(), BackendDescriptor{ID: "t", Name: "t"}, zerolog.Nop())
	local := s.Store.Pods.CreatePodWithOpts("dupe-pod", nil, "", nil)

	// Attach the fake lister AFTER the pod is created so the ID is known.
	s.CloudState = &fakePodLister{
		pods: []*api.PodListEntry{
			{ID: local.ID, Name: "dupe-pod-cloud-version", Status: "Running"},
		},
	}

	got, err := s.PodList(api.PodListOptions{})
	if err != nil {
		t.Fatalf("pod list: %v", err)
	}
	for _, p := range got {
		if p.ID == local.ID && p.Name != "dupe-pod" {
			t.Errorf("expected in-memory pod (Name=dupe-pod) to win, got %q", p.Name)
		}
	}
}
