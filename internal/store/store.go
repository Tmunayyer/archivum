// Package store reads and writes the single global JSON store (ADR-0002):
// schemes, field keys, field values and their usage, all in one versioned
// document. Values are keyed by field key, never by scheme (ADR-0003), and
// arrive already normalized (ADR-0009).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"
)

type fieldKey struct {
	Type string `json:"type"`
}

type valueUse struct {
	Value    string    `json:"value"`
	LastUsed time.Time `json:"lastUsed"`
	Count    int       `json:"count"`
}

type document struct {
	Version   int                   `json:"version"`
	FieldKeys map[string]fieldKey   `json:"fieldKeys"`
	Values    map[string][]valueUse `json:"values"`
	Schemes   map[string][]string   `json:"schemes"`
}

// Store is the in-memory copy of one store file. It is not safe for
// concurrent use, matching ADR-0002's single-writer assumption.
type Store struct {
	path string
	doc  document

	// Now stamps recency on RecordValue; tests substitute a fixed clock.
	Now func() time.Time
}

// Load reads the store at path, creating an empty versioned store (and any
// missing parent directories) if the file does not exist. A file that
// cannot be parsed is fatal here rather than silently replaced — starting
// over would throw away every value the user has taught the tool.
func Load(path string) (*Store, error) {
	s := &Store{
		path: path,
		doc: document{
			Version:   1,
			FieldKeys: map[string]fieldKey{},
			Values:    map[string][]valueUse{},
			Schemes:   map[string][]string{},
		},
		Now: time.Now,
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := s.Save(); err != nil {
			return nil, fmt.Errorf("creating store %s: %w", path, err)
		}
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading store %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &s.doc); err != nil {
		return nil, fmt.Errorf("store %s is corrupt: %v — fix or remove it and re-run", path, err)
	}
	return s, nil
}

// FieldKeyType returns the declared type of a field key. Absence means the
// key was never created — the type lives on the key (ADR-0008), so the
// caller decides what an undeclared key means.
func (s *Store) FieldKeyType(key string) (string, bool) {
	fk, ok := s.doc.FieldKeys[key]
	return fk.Type, ok
}

// Scheme returns the ordered field keys of a named scheme.
func (s *Store) Scheme(name string) ([]string, bool) {
	keys, ok := s.doc.Schemes[name]
	return keys, ok
}

// RecordValue records one confirmed use of an already-normalized value
// under a field key, updating recency and count; a value seen before is
// re-stamped, never duplicated. The store file is untouched until Save.
func (s *Store) RecordValue(key, value string) {
	now := s.Now()
	for i, v := range s.doc.Values[key] {
		if v.Value == value {
			s.doc.Values[key][i].LastUsed = now
			s.doc.Values[key][i].Count++
			return
		}
	}
	s.doc.Values[key] = append(s.doc.Values[key], valueUse{Value: value, LastUsed: now, Count: 1})
}

// Recents returns up to n values for a field key, most recently used first.
func (s *Store) Recents(key string, n int) []string {
	uses := slices.Clone(s.doc.Values[key])
	sort.SliceStable(uses, func(i, j int) bool { return uses[i].LastUsed.After(uses[j].LastUsed) })
	if len(uses) > n {
		uses = uses[:n]
	}
	values := make([]string, len(uses))
	for i, u := range uses {
		values[i] = u.Value
	}
	return values
}

// Save writes the store atomically: a temp file in the same directory,
// then a rename, so an interrupted write can never leave a half-written
// store behind.
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.doc, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".store-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
