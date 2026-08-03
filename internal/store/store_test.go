package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func storePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config", "archivum", "store.json")
}

const seeded = `{
  "version": 1,
  "fieldKeys": { "movement": { "type": "label" }, "location": { "type": "label" } },
  "values": {
    "movement": [
      { "value": "bench-press", "lastUsed": "2026-07-26T18:04:11Z", "count": 12 }
    ]
  },
  "schemes": { "lifting": ["movement", "location"] }
}`

func TestLoad(t *testing.T) {
	t.Run("missing store is created empty", func(t *testing.T) {
		path := storePath(t)
		s, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("store file not created: %v", err)
		}
		if _, ok := s.Scheme("lifting"); ok {
			t.Fatal("fresh store should have no schemes")
		}
	})

	t.Run("corrupt store errors naming the file", func(t *testing.T) {
		path := storePath(t)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if err == nil {
			t.Fatal("want error for corrupt store")
		}
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("error %q does not name the file %q", err, path)
		}
	})

	t.Run("seeded store resolves schemes in order", func(t *testing.T) {
		path := storePath(t)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(seeded), 0o644); err != nil {
			t.Fatal(err)
		}
		s, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		keys, ok := s.Scheme("lifting")
		if !ok {
			t.Fatal("scheme lifting not found")
		}
		if len(keys) != 2 || keys[0] != "movement" || keys[1] != "location" {
			t.Fatalf("Scheme(lifting) = %v, want [movement location]", keys)
		}
		if _, ok := s.Scheme("unknown"); ok {
			t.Fatal("unknown scheme should not resolve")
		}
	})
}

func TestRecordValue(t *testing.T) {
	t.Run("recorded values come back most-recent-first after reload", func(t *testing.T) {
		path := storePath(t)
		s, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		tick := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
		s.Now = func() time.Time { tick = tick.Add(time.Minute); return tick }
		s.RecordValue("movement", "bench-press")
		s.RecordValue("movement", "squat")
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}

		s2, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		got := s2.Recents("movement", 3)
		if len(got) != 2 || got[0] != "squat" || got[1] != "bench-press" {
			t.Fatalf("Recents = %v, want [squat bench-press]", got)
		}
	})

	t.Run("re-recording a value reorders it rather than duplicating", func(t *testing.T) {
		path := storePath(t)
		s, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		tick := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
		s.Now = func() time.Time { tick = tick.Add(time.Minute); return tick }
		s.RecordValue("movement", "bench-press")
		s.RecordValue("movement", "squat")
		s.RecordValue("movement", "bench-press")
		got := s.Recents("movement", 3)
		if len(got) != 2 || got[0] != "bench-press" || got[1] != "squat" {
			t.Fatalf("Recents = %v, want [bench-press squat]", got)
		}
	})

	t.Run("recents are capped at n", func(t *testing.T) {
		s, err := Load(storePath(t))
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range []string{"a", "b", "c", "d"} {
			s.RecordValue("movement", v)
		}
		if got := s.Recents("movement", 3); len(got) != 3 {
			t.Fatalf("Recents = %v, want 3 entries", got)
		}
	})
}

func TestSave(t *testing.T) {
	t.Run("writes the versioned schema from the ticket", func(t *testing.T) {
		path := storePath(t)
		s, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		s.RecordValue("movement", "bench-press")
		s.RecordValue("movement", "bench-press")
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Version int `json:"version"`
			Values  map[string][]struct {
				Value    string    `json:"value"`
				LastUsed time.Time `json:"lastUsed"`
				Count    int       `json:"count"`
			} `json:"values"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		if doc.Version != 1 {
			t.Fatalf("version = %d, want 1", doc.Version)
		}
		vs := doc.Values["movement"]
		if len(vs) != 1 || vs[0].Value != "bench-press" || vs[0].Count != 2 || vs[0].LastUsed.IsZero() {
			t.Fatalf("values = %+v, want one bench-press with count 2 and a lastUsed", vs)
		}
	})

	t.Run("round-trips a hand-seeded store without losing field keys", func(t *testing.T) {
		path := storePath(t)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(seeded), 0o644); err != nil {
			t.Fatal(err)
		}
		s, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			FieldKeys map[string]struct {
				Type string `json:"type"`
			} `json:"fieldKeys"`
			Schemes map[string][]string `json:"schemes"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		if doc.FieldKeys["movement"].Type != "label" {
			t.Fatalf("fieldKeys lost on round-trip: %+v", doc.FieldKeys)
		}
		if len(doc.Schemes["lifting"]) != 2 {
			t.Fatalf("schemes lost on round-trip: %+v", doc.Schemes)
		}
	})
}
