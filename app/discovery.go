package main

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var urlRe = regexp.MustCompile(`(?i)https?://\S+|www\.\S+`)
var wordRe = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9']{1,}`)
var stop = wordSet(`the a an and or but if of for in on at to from by is it its i you your we our they their he she them was were am are be been being do did does can could would should must have has had with not as so this that these those who what when where why how there here then than into like about just some any all more most only also very really even out up down over under off too much many other each every own my me him her his hers ours yours im ive dont didnt isnt wasnt cant wont youre theyre were thats whats lol lmao omg idk tbh imo btw yeah yep nah no yes okay ok got get did say said one two three four day days did at after before which because while its was today tomorrow yesterday bro dude guys girls but our`)
var lexicon = loadLexicon()

func loadLexicon() map[string]bool {
	b, e := assets.ReadFile("lexicon.txt")
	if e != nil {
		return map[string]bool{}
	}
	return wordSet(string(b))
}

var common = wordSet(`actually almost always already another answer anything anyone around away back bad basic beautiful best better big blue book bored boring both boy boys bring business call called came car case change chat check child children city class clear cold college come comes coming comment comments community cool correct country crazy data date deep difference different difficult done door dream easy eat eating end enough ever everybody everything example exam exhausted experience face fact failed family far fast feel feeling few find fine first fit food forever found friend friends fun funny game games gaming getting girl give given going good great group guess guy guys hair hand hands happy hard hate help high history home hope hot hour house idea important information inside internet job joke keep kind know known last later laugh left less let life light little living long look looking lot love made make makes making man men mean means meet might mind minute miss month morning move movie music name names need never new news next nice night nothing now old once open order original outside own page parent parents part past pay person people phone place play playing please point post posts pretty problem project question questions quick quite read reading real reason red right room run running sad same school see seen seems sentence serious set several short show sick side similar simple since sleep small social something sometimes soon sorry sound special still stop story strange strong student students stuff summer sure take taking talk talking tell ten test text thank thanks thing things think thinking thought time times tired together told tomorrow took top totally trouble try trying turn tweet under understand until use used user usual usually video view want wanted was watching water way week weeks well went were whats whole why will win wish without word words work working world wrong year years young late early yesterday today study learned learn learning homework cooked chicken kitchen baking recipe cooked cooking meat pan dish soup dinner lunch after before tired exams score tonight tomorrow test math physics really super crazy dude really funny awkward hype meme viral trend account thread nothing terrible awesome amazing insane okay`)

func wordSet(s string) map[string]bool {
	m := make(map[string]bool)
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}
func words(s string) []string {
	s = strings.ToLower(urlRe.ReplaceAllString(s, " "))
	matches := wordRe.FindAllString(s, -1)
	res := make([]string, 0, len(matches))
	for _, w := range matches {
		w = strings.Trim(w, "'")
		if w != "" {
			res = append(res, w)
		}
	}
	return res
}

// English heuristic is intentionally conservative; imported datasets can be marked manually.
func likelyEnglish(body string) bool {
	seq := words(body)
	if len(seq) < 4 {
		return false
	}
	count := 0
	for _, w := range seq {
		if stop[w] || common[w] || lexicon[w] {
			count++
		}
	}
	return float64(count)/float64(len(seq)) >= 0.14
}

type accumulator struct {
	ids         map[string]bool
	authors     map[string]bool
	subs        map[string]bool
	first, last int64
	docs        []Document
}

func makeAccumulator() *accumulator {
	return &accumulator{ids: map[string]bool{}, authors: map[string]bool{}, subs: map[string]bool{}}
}
func (a *accumulator) add(d Document) {
	if a.ids[d.ID] {
		return
	}
	a.ids[d.ID] = true
	if d.AuthorHash != "unknown" && d.AuthorHash != "" {
		a.authors[d.AuthorHash] = true
	}
	a.subs[d.Subreddit] = true
	if a.first == 0 || d.Created < a.first {
		a.first = d.Created
	}
	if d.Created > a.last {
		a.last = d.Created
	}
	if len(a.docs) < 500 {
		a.docs = append(a.docs, d)
	}
}
func candidateFrom(t, k string, a *accumulator, prev map[string]Candidate, bonus float64) Candidate {
	ids := make([]string, 0, len(a.ids))
	for id := range a.ids {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) > 25 {
		ids = ids[:25]
	}
	n, auth, subs := len(a.ids), len(a.authors), len(a.subs)
	score := math.Min(98, 28+11*math.Log2(float64(n)+1)+11*math.Log2(float64(auth)+1)+5*float64(min(subs, 4))+bonus)
	key := k + "|" + t
	p, ok := prev[key]
	status := "pending"
	note := ""
	if ok {
		status = p.Status
		note = p.Note
	}
	return Candidate{Term: t, Kind: k, Score: math.Round(score*10) / 10, Mentions: n, Authors: auth, Subreddits: subs, FirstSeen: a.first, LastSeen: a.last, EvidenceIDs: ids, Status: status, Note: note}
}
func validRare(w string) bool {
	if len(w) < 4 || len(w) > 22 || stop[w] || common[w] || lexicon[w] {
		return false
	}
	for _, c := range w {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '\'' {
			return false
		}
	}
	if strings.ContainsAny(w, "0123456789") && len(w) > 12 {
		return false
	}
	// Frequent duplicated keyboard sequences are often spam rather than slang.
	if strings.Contains(w, "aaaa") || strings.Contains(w, "xxxx") {
		return false
	}
	return true
}
func phrasePossiblyInteresting(seq []string) bool {
	if len(seq) < 2 {
		return false
	}
	if stop[seq[0]] || stop[seq[len(seq)-1]] {
		return false
	}
	uncommon := 0
	for _, w := range seq {
		if validRare(w) {
			uncommon++
		}
	}
	return uncommon > 0
}
func analyze(docs []Document, prior []Candidate) []Candidate {
	prev := map[string]Candidate{}
	for _, c := range prior {
		prev[c.Kind+"|"+c.Term] = c
	}
	single := map[string]*accumulator{}
	phrases := map[string]*accumulator{}
	byWord := map[string][]Document{}
	for _, d := range docs {
		if d.Source != "demo" && !likelyEnglish(d.Body) {
			continue
		}
		seq := words(d.Body)
		if len(seq) == 0 {
			continue
		}
		seen := map[string]bool{}
		for _, w := range seq {
			if seen[w] {
				continue
			}
			seen[w] = true
			if validRare(w) {
				if single[w] == nil {
					single[w] = makeAccumulator()
				}
				single[w].add(d)
			}
			if common[w] || stop[w] {
				byWord[w] = append(byWord[w], d)
			}
		}
		// Multiword candidates are sentence-bound and constrained to 2-3 grams.
		for _, sentence := range regexp.MustCompile(`[.!?;\n]+`).Split(d.Body, -1) {
			parts := words(sentence)
			for size := 2; size <= 3; size++ {
				for i := 0; i+size <= len(parts); i++ {
					span := parts[i : i+size]
					if !phrasePossiblyInteresting(span) {
						continue
					}
					text := strings.Join(span, " ")
					if phrases[text] == nil {
						phrases[text] = makeAccumulator()
					}
					phrases[text].add(d)
				}
			}
		}
	}
	result := []Candidate{}
	for t, a := range single {
		if len(a.ids) >= 3 && len(a.authors) >= 2 {
			result = append(result, candidateFrom(t, "unusual_token", a, prev, 0))
		}
	}
	for t, a := range phrases {
		if len(a.ids) >= 3 && len(a.authors) >= 2 {
			result = append(result, candidateFrom(t, "phrase_candidate", a, prev, -8))
		}
	}
	// Hypothesis of context shift: require separated time periods and distinct authors.
	for t, uses := range byWord {
		if len(uses) < 8 || len(t) < 4 {
			continue
		}
		// Deduplicate documents per token, then sort by date.
		sort.Slice(uses, func(i, j int) bool { return uses[i].Created < uses[j].Created })
		mid := len(uses) / 2
		if mid < 4 || len(uses)-mid < 4 || uses[mid-1].Created >= uses[mid].Created {
			continue
		}
		early, late := uses[:mid], uses[mid:]
		vecA, vecB := make(map[string]float64), make(map[string]float64)
		a := makeAccumulator()
		for _, d := range uses {
			a.add(d)
		}
		if len(a.authors) < 3 {
			continue
		}
		for _, d := range early {
			countContext(vecA, d.Body, t)
		}
		for _, d := range late {
			countContext(vecB, d.Body, t)
		}
		sim := cosine(vecA, vecB)
		if sim >= 0.30 {
			continue
		}
		c := candidateFrom(t, "context_drift", a, prev, 0)
		c.Score = math.Round((40+55*(1-sim))*10) / 10
		c.Interpretation = "Bağlam farklılaştı; yeni anlam henüz doğrulanmadı."
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score == result[j].Score {
			return result[i].Term < result[j].Term
		}
		return result[i].Score > result[j].Score
	})
	if len(result) > 1500 {
		result = result[:1500]
	}
	return result
}
func countContext(v map[string]float64, body, term string) {
	seq := words(body)
	for i, w := range seq {
		if w != term {
			continue
		}
		start := max(0, i-7)
		end := min(len(seq), i+8)
		for j := start; j < end; j++ {
			if j == i || stop[seq[j]] || len(seq[j]) < 3 {
				continue
			}
			v[seq[j]]++
		}
	}
}
func cosine(a, b map[string]float64) float64 {
	var dot, na, nb float64
	for k, v := range a {
		na += v * v
		dot += v * b[k]
	}
	for _, v := range b {
		nb += v * v
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}
func (r *Runtime) runAnalysis() (int, error) {
	r.mu.RLock()
	docs := append([]Document(nil), r.db.Documents...)
	previous := append([]Candidate(nil), r.db.Candidates...)
	r.mu.RUnlock()
	candidates := analyze(docs, previous)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.db.Candidates = candidates
	return len(candidates), r.persistLocked()
}
