package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const stateVersion = 1

type UserRecord struct {
	Username  string     `json:"username"`
	ChannelID string     `json:"channel_id"`
	MessageID string     `json:"message_id"`
	Content   string     `json:"content"`
	SentAt    time.Time  `json:"sent_at"`
	AlertedAt *time.Time `json:"alerted_at,omitempty"`
}

type Restore struct {
	At     time.Time `json:"at"`
	Reason string    `json:"reason"`
}

type Meta struct {
	Version            int       `json:"version"`
	UpdatedAt          time.Time `json:"updated_at"`
	RestoredFromBackup *Restore  `json:"restored_from_backup"`
}

type State struct {
	Meta  Meta                   `json:"meta"`
	Users map[string]*UserRecord `json:"users"`
}

type Entry struct {
	UserID string
	UserRecord
}

type Store struct {
	mu    sync.Mutex
	dir   string
	state State
}

func (s *Store) path() string    { return filepath.Join(s.dir, "state.json") }
func (s *Store) bakPath() string { return s.path() + ".bak" }

func newState() State {
	return State{Meta: Meta{Version: stateVersion}, Users: map[string]*UserRecord{}}
}

// OpenStore loads state.json, falling back to state.json.bak when the main
// file is missing or corrupt. The returned bool reports whether a restore happened.
func OpenStore(dir string) (*Store, bool, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, false, err
	}
	s := &Store{dir: dir}

	st, err := readState(s.path())
	if err == nil {
		s.state = st
		return s, false, nil
	}
	mainMissing := errors.Is(err, fs.ErrNotExist)
	reason := "state.json: " + err.Error()
	if !mainMissing {
		quarantine(s.path())
	}

	bak, bakErr := readState(s.bakPath())
	switch {
	case bakErr == nil:
		s.state = bak
	case mainMissing && errors.Is(bakErr, fs.ErrNotExist):
		s.state = newState()
		return s, false, s.save()
	default:
		if !errors.Is(bakErr, fs.ErrNotExist) {
			quarantine(s.bakPath())
		}
		reason += "; state.json.bak: " + bakErr.Error()
		s.state = newState()
	}

	s.state.Meta.RestoredFromBackup = &Restore{At: time.Now().UTC(), Reason: reason}
	log.Printf("store: restored from backup (%s)", reason)
	return s, true, s.save()
}

func readState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, err
	}
	if st.Users == nil {
		st.Users = map[string]*UserRecord{}
	}
	return st, nil
}

func quarantine(path string) {
	dst := fmt.Sprintf("%s.corrupt-%s", path, time.Now().UTC().Format("20060102T150405Z"))
	if err := os.Rename(path, dst); err != nil {
		log.Printf("store: quarantine %s: %v", path, err)
	}
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// save writes state.json, re-reads it to validate and only then promotes it to
// state.json.bak. Caller must hold s.mu (or own s exclusively).
func (s *Store) save() error {
	s.state.Meta.Version = stateVersion
	s.state.Meta.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(s.path(), data); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	written, err := os.ReadFile(s.path())
	if err != nil {
		return fmt.Errorf("validate state: %w", err)
	}
	var check State
	if err := json.Unmarshal(written, &check); err != nil {
		return fmt.Errorf("validate state: %w", err)
	}
	if err := writeAtomic(s.bakPath(), written); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return nil
}

// Upsert keeps only the newest message per user and saves once if anything changed.
func (s *Store) Upsert(entries ...Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, e := range entries {
		cur, ok := s.state.Users[e.UserID]
		if ok && !e.SentAt.After(cur.SentAt) {
			continue
		}
		rec := e.UserRecord
		rec.AlertedAt = nil
		s.state.Users[e.UserID] = &rec
		changed = true
	}
	if !changed {
		return nil
	}
	return s.save()
}

func (s *Store) MarkAlerted(userID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.state.Users[userID]
	if !ok {
		return nil
	}
	rec.AlertedAt = &at
	return s.save()
}

func (s *Store) Get(userID string) (UserRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.state.Users[userID]
	if !ok {
		return UserRecord{}, false
	}
	return *rec, true
}

// All returns every record ordered from the longest silence to the most recent.
func (s *Store) All() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.state.Users))
	for id, rec := range s.state.Users {
		out = append(out, Entry{UserID: id, UserRecord: *rec})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SentAt.Before(out[j].SentAt) })
	return out
}

func (s *Store) Meta() Meta {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Meta
}

func (s *Store) Empty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.state.Users) == 0
}
