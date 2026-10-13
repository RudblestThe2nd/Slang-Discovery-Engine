package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeRareAndPreserveReview(t *testing.T) {
	docs := []Document{}
	for i := 0; i < 8; i++ {
		docs = append(docs, Document{ID: fmt.Sprint(i), Created: int64(1700000000 + i*86400), Collected: time.Now().Unix(), Subreddit: "GenZ", Source: "demo", AuthorHash: fmt.Sprintf("author%d", i%4), Body: fmt.Sprintf("Today my friend uses this slangified expression and we're really hyped about it %d", i)})
	}
	results := analyze(docs, nil)
	found := false
	for _, c := range results {
		if c.Term == "slangified" && c.Kind == "unusual_token" {
			found = true
			if c.Authors != 4 || c.Mentions != 8 {
				t.Errorf("wrong stats: %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("rare token candidate missing")
	}
	prev := []Candidate{{Term: "slangified", Kind: "unusual_token", Status: "accepted", Note: "human review"}}
	next := analyze(docs, prev)
	found = false
	for _, c := range next {
		if c.Term == "slangified" && c.Kind == "unusual_token" {
			found = true
			if c.Status != "accepted" {
				t.Error("review status lost")
			}
		}
	}
	if !found {
		t.Fatal("missing token")
	}
}
func TestPersistAndEvidence(t *testing.T) {
	fn := filepath.Join(t.TempDir(), "state.json")
	r, e := newRuntime(fn)
	if e != nil {
		t.Fatal(e)
	}
	n, e := r.addDocuments([]Document{{ID: "a", Created: 1700000000, Body: "Today my friend uses new slangified words in comments", Source: "demo", AuthorHash: "u1"}, {ID: "a", Created: 1700000000, Body: "dup", Source: "demo"}})
	if e != nil || n != 1 {
		t.Fatalf("ingest: %d, %v", n, e)
	}
	rr, e := newRuntime(fn)
	if e != nil || len(rr.db.Documents) != 1 {
		t.Fatalf("reload failed: %v", e)
	}
	if _, e = rr.addDocuments([]Document{{ID: "b", Created: 1700000010, Body: "This is another synthetic sentence for testing", Source: "demo", AuthorHash: "u2"}}); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(fn + ".bak"); e != nil {
		t.Fatal("backup not created", e)
	}
}
func TestHTTPStateAndCSRF(t *testing.T) {
	r, _ := newRuntime(filepath.Join(t.TempDir(), "state.json"))
	s := &Server{run: r, token: "token123", origin: "http://127.0.0.1:4567"}
	handler := s.serve()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1:4567/api/state", nil))
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret_encrypted") {
		t.Error("password field leaked")
	}
	req := httptest.NewRequest("POST", "http://127.0.0.1:4567/api/settings", bytes.NewBufferString(`{"client_id":"new","user_agent":"windows:SlangDiscoveryEngine:1.0","subreddits":"GenZ","posts_per_sub":5,"comments_per_post":0,"retention_days":90}`))
	req.Header.Set("Origin", s.origin)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("missing token should be forbidden, got %d", w.Code)
	}
	req = httptest.NewRequest("POST", "http://127.0.0.1:4567/api/settings", bytes.NewBufferString(`{"client_id":"new","user_agent":"windows:SlangDiscoveryEngine:1.0","subreddits":"GenZ","posts_per_sub":5,"comments_per_post":0,"retention_days":90}`))
	req.Header.Set("Origin", s.origin)
	req.Header.Set("X-App-Token", s.token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if r.db.Settings.ClientID != "new" {
		t.Fatal("settings not saved")
	}
}
func TestRedditOAuthAndCollectionMock(t *testing.T) {
	var auth bool
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/access_token":
			u, p, ok := r.BasicAuth()
			auth = ok && u == "testid" && p == "testsecret"
			json.NewEncoder(w).Encode(map[string]string{"access_token": "fake"})
		case strings.HasPrefix(r.URL.Path, "/r/"):
			if r.Header.Get("Authorization") != "Bearer fake" {
				t.Errorf("missing bearer")
			}
			fmt.Fprint(w, `{"data":{"children":[{"data":{"id":"abc","author":"someone","created_utc":1700000000,"title":"Wild new slangified things","selftext":"Today we are excited","permalink":"/r/GenZ/comments/abc/test/"}}]}}`)
		case strings.HasPrefix(r.URL.Path, "/comments/"):
			fmt.Fprint(w, `[{"data":{"children":[]}},{"data":{"children":[{"data":{"id":"def","author":"reader","created_utc":1700001000,"body":"That slangified thing is pretty funny","permalink":"/r/GenZ/comments/abc/comment/def/"}}]}}]`)
		default:
			t.Errorf("unexpected URL: %s", r.URL)
			w.WriteHeader(404)
		}
	}))
	defer mock.Close()
	c := newRedditClient(initialSettings(), "testsecret")
	c.ID = "testid"
	c.AuthURL = mock.URL + "/api/v1/access_token"
	c.APIURL = mock.URL
	docs, warnings, e := c.fetch(context.Background(), "GenZ", 5, 3)
	if e != nil || !auth || len(warnings) != 0 || len(docs) != 2 {
		t.Fatalf("fetch failed auth=%v docs=%d warnings=%v err=%v", auth, len(docs), warnings, e)
	}
	if docs[0].AuthorHash == "someone" || docs[0].AuthorHash == "" {
		t.Fatal("author not hashed")
	}
}
func TestInfoUIAndFileLinks(t *testing.T) {
	b, e := assets.ReadFile("web/index.html")
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"open-info", "client-secret", "reddit.com/prefs/apps", "Responsible-Builder-Policy", "data-view=\"settings\""} {
		if !bytes.Contains(b, []byte(s)) {
			t.Errorf("missing UI: %s", s)
		}
	}
}

func TestDemoNoOrdinaryWordFalsePositives(t *testing.T) {
	b, e := assets.ReadFile("demo.jsonl")
	if e != nil {
		t.Fatal(e)
	}
	docs := []Document{}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var row struct {
			ID         string `json:"id"`
			Body       string `json:"body"`
			AuthorHash string `json:"author_hash"`
			Subreddit  string `json:"subreddit"`
			Created    int64  `json:"created_utc"`
		}
		if e = json.Unmarshal([]byte(l), &row); e != nil {
			t.Fatal(e)
		}
		docs = append(docs, Document{ID: row.ID, Body: row.Body, AuthorHash: row.AuthorHash, Subreddit: row.Subreddit, Created: row.Created, Source: "demo"})
	}
	found := map[string]bool{}
	for _, c := range analyze(docs, nil) {
		found[c.Kind+"/"+c.Term] = true
	}
	for _, w := range []string{"meal", "delicious", "everyone", "impossible", "lobby", "clip"} {
		if found["unusual_token/"+w] {
			t.Errorf("ordinary word wrongly flagged: %s", w)
		}
	}
	if !found["unusual_token/glorpmaxxing"] {
		t.Fatal("invented term missing")
	}
	if !found["context_drift/cooked"] {
		t.Fatal("cooked context shift hypothesis missing")
	}
}
