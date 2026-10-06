package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func entry(id string, at time.Time) Entry {
	return Entry{UserID: id, UserRecord: UserRecord{Username: id, ChannelID: "c", MessageID: "m", Content: "oi", SentAt: at}}
}

func TestUpsertKeepsNewest(t *testing.T) {
	s, _, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := s.Upsert(entry("a", now), entry("a", now.Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}
	rec, _ := s.Get("a")
	if !rec.SentAt.Equal(now) {
		t.Fatalf("got %v, want %v", rec.SentAt, now)
	}
}

func TestRestoreFromBackup(t *testing.T) {
	dir := t.TempDir()
	s, _, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert(entry("a", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}

	s2, restored, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !restored || s2.Meta().RestoredFromBackup == nil {
		t.Fatal("expected restore flag")
	}
	if _, ok := s2.Get("a"); !ok {
		t.Fatal("expected user restored from backup")
	}
	if _, err := readState(filepath.Join(dir, "state.json")); err != nil {
		t.Fatalf("main file not repaired: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "state.json.corrupt-*"))
	if len(matches) != 1 {
		t.Fatalf("expected quarantined file, got %v", matches)
	}
}

func TestFirstRunNoRestoreFlag(t *testing.T) {
	s, restored, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if restored || s.Meta().RestoredFromBackup != nil {
		t.Fatal("fresh store should not be flagged as restored")
	}
}
