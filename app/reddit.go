package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type RedditClient struct {
	ID, Secret, Agent, Salt string
	HTTP                    *http.Client
	AuthURL, APIURL         string
}

func newRedditClient(s Settings, secret string) *RedditClient {
	return &RedditClient{ID: s.ClientID, Secret: secret, Agent: s.UserAgent, Salt: s.Salt,
		HTTP: &http.Client{Timeout: 25 * time.Second}, AuthURL: "https://www.reddit.com/api/v1/access_token", APIURL: "https://oauth.reddit.com"}
}
func (c *RedditClient) token(ctx context.Context) (string, error) {
	if c.ID == "" || c.Secret == "" {
		return "", errors.New("Reddit client ID ve secret girilmeli")
	}
	data := url.Values{"grant_type": {"client_credentials"}}
	req, e := http.NewRequestWithContext(ctx, "POST", c.AuthURL, strings.NewReader(data.Encode()))
	if e != nil {
		return "", e
	}
	req.SetBasicAuth(c.ID, c.Secret)
	req.Header.Set("User-Agent", c.Agent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Reddit yetkilendirme HTTP %d: %s", resp.StatusCode, safeError(string(body)))
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if e = json.Unmarshal(body, &parsed); e != nil {
		return "", e
	}
	if parsed.AccessToken == "" {
		return "", errors.New("Reddit access_token dönmedi; yetki ve API başvurusunu kontrol edin")
	}
	return parsed.AccessToken, nil
}
func safeError(s string) string {
	if len(s) > 240 {
		s = s[:240]
	}
	return s
}
func (c *RedditClient) getJSON(ctx context.Context, token, path string) (any, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", c.APIURL+path, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", c.Agent)
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 429 {
			return nil, errors.New("Reddit rate limit (429). Daha sonra yeniden deneyin")
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("Reddit HTTP %d: %s", resp.StatusCode, safeError(string(b)))
	}
	var value any
	e = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&value)
	return value, e
}
func children(v any) []map[string]any {
	o, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	data, ok := o["data"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := data["children"].([]any)
	if !ok {
		return nil
	}
	out := []map[string]any{}
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		d, ok := m["data"].(map[string]any)
		if ok {
			out = append(out, d)
		}
	}
	return out
}
func stringField(m map[string]any, name string) string { v, _ := m[name].(string); return v }
func numberField(m map[string]any, name string) int64  { f, _ := m[name].(float64); return int64(f) }
func (c *RedditClient) doc(m map[string]any, kind, sub string) Document {
	author := hashAuthor(stringField(m, "author"), c.Salt)
	body := stringField(m, "body")
	if kind == "t3" {
		body = strings.TrimSpace(stringField(m, "title") + " " + stringField(m, "selftext"))
	}
	p := stringField(m, "permalink")
	if p != "" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return Document{ID: kind + "_" + stringField(m, "id"), Subreddit: sub, AuthorHash: author,
		Created: numberField(m, "created_utc"), Body: body, URL: "https://www.reddit.com" + p, Source: "reddit"}
}
func (c *RedditClient) fetch(ctx context.Context, subreddits string, perSub, perPost int) ([]Document, []string, error) {
	tok, e := c.token(ctx)
	if e != nil {
		return nil, nil, e
	}
	result := []Document{}
	warnings := []string{}
	names := strings.Split(subreddits, ",")
	if len(names) > 7 {
		names = names[:7]
	}
	for _, raw := range names {
		sub := strings.TrimSpace(raw)
		if sub == "" {
			continue
		}
		valid := true
		for _, r := range sub {
			if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
				valid = false
				break
			}
		}
		if !valid || len(sub) > 40 {
			warnings = append(warnings, "Geçersiz subreddit: "+sub)
			continue
		}
		listing, e := c.getJSON(ctx, tok, "/r/"+url.PathEscape(sub)+"/new?limit="+strconv.Itoa(perSub)+"&raw_json=1")
		if e != nil {
			warnings = append(warnings, sub+": "+e.Error())
			continue
		}
		for i, m := range children(listing) {
			id := stringField(m, "id")
			if id == "" {
				continue
			}
			result = append(result, c.doc(m, "t3", sub))
			if perPost == 0 || i >= 8 {
				continue
			} // limit number of calls to avoid rate limits
			thread, e := c.getJSON(ctx, tok, "/comments/"+url.PathEscape(id)+".json?limit="+strconv.Itoa(perPost)+"&depth=1&raw_json=1")
			if e != nil {
				warnings = append(warnings, "Yorum isteği başarısız: "+sub)
				continue
			}
			arr, ok := thread.([]any)
			if !ok || len(arr) < 2 {
				continue
			}
			kids := children(arr[1])
			if len(kids) > perPost {
				kids = kids[:perPost]
			}
			for _, comm := range kids {
				if stringField(comm, "id") != "" {
					result = append(result, c.doc(comm, "t1", sub))
				}
			}
		}
	}
	return result, warnings, nil
}
