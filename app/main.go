package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed web/* demo.jsonl lexicon.txt
var assets embed.FS

type Server struct {
	run           *Runtime
	token, origin string
}

func jsonReply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func jsonError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	jsonReply(w, map[string]any{"error": msg})
}
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonError(w, 405, "POST gerekli")
			return
		}
		if r.Header.Get("X-App-Token") != s.token || r.Header.Get("Origin") != s.origin {
			jsonError(w, 403, "Güvenlik doğrulaması başarısız")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next(w, r)
	}
}
func decode(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }
func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	s.run.mu.RLock()
	defer s.run.mu.RUnlock()
	settings := s.run.db.Settings
	settings.SecretEncrypted = ""
	jsonReply(w, map[string]any{"settings": settings, "has_secret": s.run.db.Settings.SecretEncrypted != "", "documents": len(s.run.db.Documents), "candidates": s.run.db.Candidates, "last_updated": s.run.db.Updated, "running": s.run.running, "job": s.run.job, "job_state": s.run.jobState, "last_error": s.run.lastErr, "last_result": s.run.lastResult})
}
func (s *Server) shutdown(w http.ResponseWriter, r *http.Request) {
	jsonReply(w, map[string]bool{"ok": true})
	go func() { time.Sleep(300 * time.Millisecond); os.Exit(0) }()
}
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID        string `json:"client_id"`
		ClientSecret    string `json:"client_secret"`
		ClearSecret     bool   `json:"clear_secret"`
		UserAgent       string `json:"user_agent"`
		Subreddits      string `json:"subreddits"`
		PostsPerSub     int    `json:"posts_per_sub"`
		CommentsPerPost int    `json:"comments_per_post"`
		RetentionDays   int    `json:"retention_days"`
	}
	if e := decode(r, &req); e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	if len(req.ClientID) > 200 || len(req.UserAgent) > 240 || len(req.Subreddits) > 400 || len(req.ClientSecret) > 400 {
		jsonError(w, 400, "Alan uzunluğu sınırını aştı")
		return
	}
	if req.PostsPerSub < 1 || req.PostsPerSub > 50 || req.CommentsPerPost < 0 || req.CommentsPerPost > 50 || req.RetentionDays < 1 || req.RetentionDays > 365 {
		jsonError(w, 400, "Limitler geçersiz")
		return
	}
	if !strings.Contains(req.UserAgent, "SlangDiscovery") {
		jsonError(w, 400, "User-Agent içinde SlangDiscovery bulunmalı")
		return
	}
	s.run.mu.Lock()
	defer s.run.mu.Unlock()
	if s.run.running {
		jsonError(w, 409, "Çalışan işlem bitmeden ayarlar değiştirilemez")
		return
	}
	p := &s.run.db.Settings
	p.ClientID = strings.TrimSpace(req.ClientID)
	p.UserAgent = strings.TrimSpace(req.UserAgent)
	p.Subreddits = strings.TrimSpace(req.Subreddits)
	p.PostsPerSub = req.PostsPerSub
	p.CommentsPerPost = req.CommentsPerPost
	p.RetentionDays = req.RetentionDays
	if req.ClearSecret {
		p.SecretEncrypted = ""
	}
	if req.ClientSecret != "" {
		e, sErr := seal(req.ClientSecret)
		if sErr != nil {
			jsonError(w, 500, sErr.Error())
			return
		}
		p.SecretEncrypted = e
	}
	if e := s.run.persistLocked(); e != nil {
		jsonError(w, 500, e.Error())
		return
	}
	jsonReply(w, map[string]any{"ok": true, "has_secret": p.SecretEncrypted != ""})
}
func (s *Server) job(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
	}
	if e := decode(r, &req); e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	if req.Action != "demo" && req.Action != "analyze" && req.Action != "reddit" && req.Action != "test_reddit" {
		jsonError(w, 400, "Geçersiz işlem")
		return
	}
	s.run.mu.Lock()
	if s.run.running {
		s.run.mu.Unlock()
		jsonError(w, 409, "Başka bir işlem devam ediyor")
		return
	}
	s.run.running = true
	s.run.job = req.Action
	s.run.jobState = "running"
	s.run.lastErr = ""
	s.run.lastResult = ""
	s.run.mu.Unlock()
	go func() {
		var result string
		var e error
		defer func() {
			s.run.mu.Lock()
			s.run.running = false
			s.run.jobState = "finished"
			if e != nil {
				s.run.jobState = "error"
				s.run.lastErr = e.Error()
			} else {
				s.run.lastResult = result
			}
			s.run.mu.Unlock()
		}()
		switch req.Action {
		case "demo":
			buf, _ := assets.ReadFile("demo.jsonl")
			var docs []Document
			for _, line := range strings.Split(string(buf), "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				var rec struct {
					ID        string `json:"id"`
					Subreddit string `json:"subreddit"`
					Author    string `json:"author_hash"`
					Created   int64  `json:"created_utc"`
					Body      string `json:"body"`
					URL       string `json:"url"`
				}
				if json.Unmarshal([]byte(line), &rec) != nil {
					continue
				}
				docs = append(docs, Document{ID: "demo_" + rec.ID, Subreddit: rec.Subreddit, AuthorHash: rec.Author, Created: rec.Created, Collected: time.Now().Unix(), Body: rec.Body, URL: rec.URL, Source: "demo"})
			}
			n, addErr := s.run.addDocuments(docs)
			if addErr != nil {
				e = addErr
				return
			}
			c, analysisErr := s.run.runAnalysis()
			e = analysisErr
			result = fmt.Sprintf("%d demo kayıt eklendi. %d aday hesaplandı.", n, c)
		case "analyze":
			var n int
			n, e = s.run.runAnalysis()
			result = fmt.Sprintf("%d aday yeniden hesaplandı.", n)
		case "reddit", "test_reddit":
			s.run.mu.RLock()
			cfg := s.run.db.Settings
			s.run.mu.RUnlock()
			secret, err := reveal(cfg.SecretEncrypted)
			if err != nil {
				e = err
				return
			}
			client := newRedditClient(cfg, secret)
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			if req.Action == "test_reddit" {
				_, e = client.token(ctx)
				if e == nil {
					result = "Reddit OAuth token alındı. Bu, veri erişimi izninin kesin doğrulaması değildir."
				}
				return
			}
			docs, warnings, fetchErr := client.fetch(ctx, cfg.Subreddits, cfg.PostsPerSub, cfg.CommentsPerPost)
			if fetchErr != nil {
				e = fetchErr
				return
			}
			n, addErr := s.run.addDocuments(docs)
			if addErr != nil {
				e = addErr
				return
			}
			num, analysisErr := s.run.runAnalysis()
			e = analysisErr
			result = fmt.Sprintf("%d yeni belge, %d aday. Uyarı sayısı: %d.", n, num, len(warnings))
			if len(warnings) > 0 {
				result += " " + strings.Join(warnings, " | ")
			}
		}
	}()
	jsonReply(w, map[string]any{"ok": true, "started": req.Action})
}

// importJSONL accepts rights-cleared exported text, so the standalone app is
// usable without Reddit credentials. Raw usernames never reach local storage.
func (s *Server) importJSONL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lines string `json:"lines"`
	}
	if e := decode(r, &req); e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	s.run.mu.RLock()
	salt := s.run.db.Settings.Salt
	busy := s.run.running
	s.run.mu.RUnlock()
	if busy {
		jsonError(w, 409, "Çalışan işlem bitmeden içe aktarma yapılamaz")
		return
	}
	docs := []Document{}
	invalid := 0
	for i, line := range strings.Split(req.Lines, "\n") {
		if len(docs) >= 2500 {
			break
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		var v struct {
			ID         string `json:"id"`
			Body       string `json:"body"`
			Subreddit  string `json:"subreddit"`
			Author     string `json:"author"`
			AuthorHash string `json:"author_hash"`
			Created    int64  `json:"created_utc"`
			URL        string `json:"url"`
			Source     string `json:"source"`
		}
		if e := json.Unmarshal([]byte(line), &v); e != nil || v.ID == "" || v.Body == "" {
			invalid++
			continue
		}
		if v.Created == 0 {
			v.Created = time.Now().Unix()
		}
		if v.Subreddit == "" {
			v.Subreddit = "import"
		}
		// Each imported record is namespaced to avoid accidental collisions with Reddit IDs.
		hash := v.AuthorHash
		if v.Author != "" {
			hash = hashAuthor(v.Author, salt)
		}
		if hash == "" {
			hash = "unknown"
		}
		source := "import" // do not label user-supplied content as verified Reddit data.
		docs = append(docs, Document{ID: fmt.Sprintf("import_%d_%s", i, v.ID), Body: v.Body, Subreddit: v.Subreddit, AuthorHash: hash, Created: v.Created, Collected: time.Now().Unix(), URL: v.URL, Source: source})
	}
	if len(docs) == 0 {
		jsonError(w, 400, "Geçerli JSONL satırı bulunamadı")
		return
	}
	n, e := s.run.addDocuments(docs)
	if e != nil {
		jsonError(w, 500, e.Error())
		return
	}
	num, e := s.run.runAnalysis()
	if e != nil {
		jsonError(w, 500, e.Error())
		return
	}
	jsonReply(w, map[string]any{"ok": true, "imported": n, "invalid": invalid, "candidates": num})
}

func (s *Server) review(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Term   string `json:"term"`
		Kind   string `json:"kind"`
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if e := decode(r, &req); e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	if e := s.run.markReview(req.Term, req.Kind, req.Status, req.Note); e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	jsonReply(w, map[string]any{"ok": true})
}
func (s *Server) evidence(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	kind := r.URL.Query().Get("kind")
	s.run.mu.RLock()
	defer s.run.mu.RUnlock()
	ids := map[string]bool{}
	for _, c := range s.run.db.Candidates {
		if c.Term == term && c.Kind == kind {
			for _, id := range c.EvidenceIDs {
				ids[id] = true
			}
		}
	}
	res := []Document{}
	for _, d := range s.run.db.Documents {
		if ids[d.ID] {
			res = append(res, d)
		}
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Created > res[j].Created })
	if len(res) > 20 {
		res = res[:20]
	}
	jsonReply(w, res)
}
func (s *Server) trend(w http.ResponseWriter, r *http.Request) {
	term, kind := r.URL.Query().Get("term"), r.URL.Query().Get("kind")
	s.run.mu.RLock()
	defer s.run.mu.RUnlock()
	ids := map[string]bool{}
	for _, c := range s.run.db.Candidates {
		if c.Term == term && c.Kind == kind {
			for _, id := range c.EvidenceIDs {
				ids[id] = true
			}
		}
	}
	weekAll := map[string]int{}
	weekFound := map[string]int{}
	for _, d := range s.run.db.Documents {
		t := time.Unix(d.Created, 0).UTC()
		year, wk := t.ISOWeek()
		key := fmt.Sprintf("%04d-W%02d", year, wk)
		weekAll[key]++
		if ids[d.ID] {
			weekFound[key]++
		}
	}
	keys := []string{}
	for k := range weekAll {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := []map[string]any{}
	for _, k := range keys {
		total := weekAll[k]
		hits := weekFound[k]
		rows = append(rows, map[string]any{"week": k, "documents": total, "mentions": hits, "per_1000_docs": float64(hits) * 1000 / float64(total)})
	}
	jsonReply(w, rows)
}
func (s *Server) exportCSV(w http.ResponseWriter, r *http.Request) {
	s.run.mu.RLock()
	cands := append([]Candidate(nil), s.run.db.Candidates...)
	s.run.mu.RUnlock()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="slang-candidates.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"term", "kind", "score", "mentions", "authors", "subreddits", "status", "note"})
	for _, c := range cands {
		_ = cw.Write([]string{c.Term, c.Kind, strconv.FormatFloat(c.Score, 'f', 1, 64), strconv.Itoa(c.Mentions), strconv.Itoa(c.Authors), strconv.Itoa(c.Subreddits), c.Status, c.Note})
	}
	cw.Flush()
}
func (s *Server) serve() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		bytes, e := assets.ReadFile("web/index.html")
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		html := strings.ReplaceAll(string(bytes), "%%APP_TOKEN%%", s.token)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, html)
	})
	mux.HandleFunc("/styles.css", func(w http.ResponseWriter, r *http.Request) {
		b, _ := assets.ReadFile("web/styles.css")
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("/app.js", func(w http.ResponseWriter, r *http.Request) {
		b, _ := assets.ReadFile("web/app.js")
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("/api/state", s.state)
	mux.HandleFunc("/api/evidence", s.evidence)
	mux.HandleFunc("/api/trend", s.trend)
	mux.HandleFunc("/api/export.csv", s.exportCSV)
	mux.HandleFunc("/api/settings", s.guard(s.settings))
	mux.HandleFunc("/api/job", s.guard(s.job))
	mux.HandleFunc("/api/review", s.guard(s.review))
	mux.HandleFunc("/api/shutdown", s.guard(s.shutdown))
	mux.HandleFunc("/api/import", s.guard(s.importJSONL))
	return mux
}
func browser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
func main() {
	path := dataPath()
	r, e := newRuntime(path)
	if e != nil {
		log.Println("Config:", e)
		os.Exit(1)
	}
	tokenBytes := make([]byte, 24)
	_, _ = rand.Read(tokenBytes)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		log.Fatal(e)
	}
	addr := "http://" + listener.Addr().String()
	s := &Server{run: r, token: hex.EncodeToString(tokenBytes), origin: addr}
	server := &http.Server{Handler: s.serve(), ReadHeaderTimeout: 8 * time.Second}
	fmt.Println("Application running at", addr)
	go func() { time.Sleep(250 * time.Millisecond); browser(addr) }()
	if e = server.Serve(listener); e != nil && !errors.Is(e, http.ErrServerClosed) {
		log.Fatal(e)
	}
}
