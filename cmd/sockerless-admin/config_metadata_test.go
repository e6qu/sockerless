package main

import (
	"reflect"
	"testing"
)

func TestAnnotationKnownHotKey(t *testing.T) {
	got := Annotation("SIM_LOG_LEVEL")
	if !got.HotReloadable {
		t.Errorf("SIM_LOG_LEVEL should be hot-reloadable, got %+v", got)
	}
	if got.Doc == "" {
		t.Errorf("SIM_LOG_LEVEL should carry a doc string")
	}
}

func TestAnnotationKnownRestartKey(t *testing.T) {
	got := Annotation("SIM_DATA_DIR")
	if got.HotReloadable {
		t.Errorf("SIM_DATA_DIR must NOT be hot-reloadable; toggling mid-run is unsafe")
	}
	if got.Doc == "" {
		t.Errorf("SIM_DATA_DIR should carry a doc string")
	}
}

func TestAnnotationUnknownKeyDefaultsRestart(t *testing.T) {
	got := Annotation("WHATEVER_NEW_KEY_X42")
	if got.HotReloadable {
		t.Errorf("unknown key must default to restart-required (safe default)")
	}
	if got.Name != "WHATEVER_NEW_KEY_X42" {
		t.Errorf("Name should reflect the queried key, got %q", got.Name)
	}
}

func TestAllAnnotationsSortedNoDupes(t *testing.T) {
	all := AllAnnotations()
	names := make(map[string]bool)
	for i, m := range all {
		if names[m.Name] {
			t.Errorf("duplicate annotation %q in AllAnnotations", m.Name)
		}
		names[m.Name] = true
		if i > 0 && all[i-1].Name > m.Name {
			t.Errorf("AllAnnotations not sorted: %q before %q", all[i-1].Name, m.Name)
		}
	}
	if len(all) == 0 {
		t.Errorf("AllAnnotations should have curated entries, got 0")
	}
}

func TestClassifyChangesHotKey(t *testing.T) {
	prev := map[string]string{"SIM_LOG_LEVEL": "info"}
	next := map[string]string{"SIM_LOG_LEVEL": "debug"}
	hot, restart := ClassifyChanges(prev, next)
	if !reflect.DeepEqual(hot, []string{"SIM_LOG_LEVEL"}) {
		t.Errorf("hot = %v, want [SIM_LOG_LEVEL]", hot)
	}
	if len(restart) != 0 {
		t.Errorf("restart should be empty, got %v", restart)
	}
}

func TestClassifyChangesRestartKey(t *testing.T) {
	prev := map[string]string{"SIM_DATA_DIR": "/tmp/a"}
	next := map[string]string{"SIM_DATA_DIR": "/tmp/b"}
	hot, restart := ClassifyChanges(prev, next)
	if len(hot) != 0 {
		t.Errorf("hot should be empty, got %v", hot)
	}
	if !reflect.DeepEqual(restart, []string{"SIM_DATA_DIR"}) {
		t.Errorf("restart = %v, want [SIM_DATA_DIR]", restart)
	}
}

func TestClassifyChangesMixed(t *testing.T) {
	prev := map[string]string{
		"SIM_LOG_LEVEL": "info",
		"SIM_DATA_DIR":  "/tmp/a",
		"UNCHANGED":     "x",
	}
	next := map[string]string{
		"SIM_LOG_LEVEL": "debug",  // hot change
		"SIM_DATA_DIR":  "/tmp/b", // restart change
		"UNCHANGED":     "x",      // skip
		"SIM_NEW_KEY":   "added",  // unknown → restart
	}
	hot, restart := ClassifyChanges(prev, next)
	if !reflect.DeepEqual(hot, []string{"SIM_LOG_LEVEL"}) {
		t.Errorf("hot = %v", hot)
	}
	wantRestart := []string{"SIM_DATA_DIR", "SIM_NEW_KEY"}
	if !reflect.DeepEqual(restart, wantRestart) {
		t.Errorf("restart = %v, want %v", restart, wantRestart)
	}
}

func TestClassifyChangesRemovedKey(t *testing.T) {
	// Removing a hot key counts as a hot change; removing a restart
	// key counts as a restart change. Either way, the classification
	// matches the key's annotation.
	prev := map[string]string{
		"SIM_LOG_LEVEL": "info",
		"SIM_DATA_DIR":  "/tmp/a",
	}
	next := map[string]string{}
	hot, restart := ClassifyChanges(prev, next)
	if !reflect.DeepEqual(hot, []string{"SIM_LOG_LEVEL"}) {
		t.Errorf("hot = %v", hot)
	}
	if !reflect.DeepEqual(restart, []string{"SIM_DATA_DIR"}) {
		t.Errorf("restart = %v", restart)
	}
}

func TestClassifyChangesNoChange(t *testing.T) {
	prev := map[string]string{"K": "v"}
	next := map[string]string{"K": "v"}
	hot, restart := ClassifyChanges(prev, next)
	if len(hot) != 0 || len(restart) != 0 {
		t.Errorf("identity diff should produce no changes, got hot=%v restart=%v", hot, restart)
	}
}
