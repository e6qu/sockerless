package ecs

import (
	"reflect"
	"testing"
)

func TestDedupeNonEmpty(t *testing.T) {
	got := dedupeNonEmpty([]string{"a", "", "a", "b", "", "c", "b"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupeNonEmpty = %v, want %v", got, want)
	}
	if got := dedupeNonEmpty(nil); len(got) != 0 {
		t.Fatalf("dedupeNonEmpty(nil) = %v, want empty", got)
	}
}

func TestCloudMapNamesFor(t *testing.T) {
	// gitlab-runner sends the service alias duplicated (["redis","redis"]);
	// the hostname leads, aliases follow, dupes + empties dropped.
	got := cloudMapNamesFor("runner-x-redis-0", []string{"redis", "redis", "", "runner-x-redis-0"})
	want := []string{"runner-x-redis-0", "redis"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cloudMapNamesFor = %v, want %v", got, want)
	}
	// No aliases → just the hostname.
	if got := cloudMapNamesFor("solo", nil); !reflect.DeepEqual(got, []string{"solo"}) {
		t.Fatalf("cloudMapNamesFor(solo, nil) = %v", got)
	}
}
