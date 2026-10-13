package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Settings struct {
	ClientID        string `json:"client_id"`
	SecretEncrypted string `json:"secret_encrypted,omitempty"`
	UserAgent       string `json:"user_agent"`
	Subreddits      string `json:"subreddits"`
	PostsPerSub     int    `json:"posts_per_sub"`
	CommentsPerPost int    `json:"comments_per_post"`
	RetentionDays   int    `json:"retention_days"`
	Salt            string `json:"salt"`
}
type Document struct {
	ID         string `json:"id"`
	Subreddit  string `json:"subreddit"`
	AuthorHash string `json:"author_hash"`
	Created    int64  `json:"created"`
	Collected  int64  `json:"collected"`
	Body       string `json:"body"`
	URL        string `json:"url"`
	Source     string `json:"source"`
}
type Candidate struct {
	Term           string   `json:"term"`
	Kind           string   `json:"kind"`
	Score          float64  `json:"score"`
	Mentions       int      `json:"mentions"`
	Authors        int      `json:"authors"`
	Subreddits     int      `json:"subreddits"`
	FirstSeen      int64    `json:"first_seen"`
	LastSeen       int64    `json:"last_seen"`
	EvidenceIDs    []string `json:"evidence_ids"`
	Status         string   `json:"status"`
	Note           string   `json:"note"`
	Interpretation string   `json:"interpretation,omitempty"`
}
type Review struct {
	Term   string `json:"term"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Note   string `json:"note"`
	At     int64  `json:"at"`
}
type Store struct {
	Settings   Settings    `json:"settings"`
	Documents  []Document  `json:"documents"`
	Candidates []Candidate `json:"candidates"`
	Reviews    []Review    `json:"reviews"`
	Updated    int64       `json:"updated"`
}
type Runtime struct {
	mu         sync.RWMutex
	db         Store
	filename   string
	running    bool
	job        string
	jobState   string
	lastErr    string
	lastResult string
}

func initialSettings() Settings {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return Settings{Subreddits: "GenZ,OutOfTheLoop,teenagers", PostsPerSub: 12, CommentsPerPost: 15, RetentionDays: 90, Salt: hex.EncodeToString(b), UserAgent: "windows:SlangDiscoveryEngine:1.0 (research; contact: your-reddit-username)"}
}
func dataPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		dir, e := os.UserConfigDir()
		if e == nil {
			base = dir
		} else {
			base = "."
		}
	}
	return filepath.Join(base, "SlangDiscoveryEngine", "state.json")
}
func newRuntime(path string) (*Runtime, error) {
	r := &Runtime{filename: path, db: Store{Settings: initialSettings(), Documents: []Document{}, Candidates: []Candidate{}, Reviews: []Review{}}, jobState: "idle"}
	buf, e := os.ReadFile(path)
	if e == nil {
		if e = json.Unmarshal(buf, &r.db); e != nil {
			return nil, errors.New("veri dosyasi okunamadi; yedek dosyanizi kontrol edin: " + e.Error())
		}
		if r.db.Settings.Salt == "" {
			r.db.Settings.Salt = initialSettings().Salt
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if r.db.Documents == nil {
		r.db.Documents = []Document{}
	}
	if r.db.Candidates == nil {
		r.db.Candidates = []Candidate{}
	}
	if r.db.Reviews == nil {
		r.db.Reviews = []Review{}
	}
	return r, nil
}
func (r *Runtime) persistLocked() error {
	dir := filepath.Dir(r.filename)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	r.db.Updated = time.Now().Unix()
	b, e := json.MarshalIndent(r.db, "", "  ")
	if e != nil {
		return e
	}
	tmp := r.filename + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	// Windows rename does not atomically replace an existing destination.
	if _, e = os.Stat(r.filename); e == nil {
		old := r.filename + ".bak"
		_ = os.Remove(old)
		if e = os.Rename(r.filename, old); e != nil {
			return e
		}
	}
	if e = os.Rename(tmp, r.filename); e != nil {
		if _, se := os.Stat(r.filename + ".bak"); se == nil {
			_ = os.Rename(r.filename+".bak", r.filename)
		}
		return e
	}
	return nil
}
func hashAuthor(author, salt string) string {
	if author == "" || author == "[deleted]" || author == "unknown" {
		return "unknown"
	}
	s := sha256.Sum256([]byte(salt + ":" + author))
	return hex.EncodeToString(s[:])[:20]
}
func (r *Runtime) addDocuments(docs []Document) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing := make(map[string]bool, len(r.db.Documents))
	for _, d := range r.db.Documents {
		existing[d.ID] = true
	}
	added := 0
	for _, d := range docs {
		if d.ID == "" || len(strings.TrimSpace(d.Body)) < 6 || d.Body == "[deleted]" || d.Body == "[removed]" || existing[d.ID] {
			continue
		}
		if d.Collected == 0 {
			d.Collected = time.Now().Unix()
		}
		if d.Created == 0 {
			d.Created = d.Collected
		}
		if len(d.Body) > 6000 {
			d.Body = d.Body[:6000]
		}
		r.db.Documents = append(r.db.Documents, d)
		existing[d.ID] = true
		added++
	}
	cutoff := time.Now().AddDate(0, 0, -r.db.Settings.RetentionDays).Unix()
	kept := r.db.Documents[:0]
	for _, d := range r.db.Documents {
		if d.Collected >= cutoff {
			kept = append(kept, d)
		}
	}
	r.db.Documents = kept
	if len(r.db.Documents) > 15000 {
		sort.Slice(r.db.Documents, func(i, j int) bool { return r.db.Documents[i].Collected > r.db.Documents[j].Collected })
		r.db.Documents = r.db.Documents[:15000]
	}
	return added, r.persistLocked()
}
func (r *Runtime) markReview(term, kind, status, note string) error {
	if status != "accepted" && status != "rejected" && status != "needs_review" && status != "pending" {
		return errors.New("gecersiz durum")
	}
	if len(note) > 500 {
		note = note[:500]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	found := false
	for i := range r.db.Candidates {
		c := &r.db.Candidates[i]
		if c.Term == term && c.Kind == kind {
			c.Status = status
			c.Note = note
			found = true
			break
		}
	}
	if !found {
		return errors.New("aday bulunamadi")
	}
	r.db.Reviews = append(r.db.Reviews, Review{Term: term, Kind: kind, Status: status, Note: note, At: time.Now().Unix()})
	if len(r.db.Reviews) > 10000 {
		r.db.Reviews = r.db.Reviews[len(r.db.Reviews)-10000:]
	}
	return r.persistLocked()
}
