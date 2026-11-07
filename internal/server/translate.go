package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/abjelosevic88/pocket-homelab-translate/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/engine"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// ---- translating ----
//
// Three engines: the person's AI (Pocket AI through Core, or their own
// OpenAI-compatible key), and two offline ones on this box that speak the
// LibreTranslate API — Argos (LibreTranslate itself: quick, about fifty
// languages, everything through English) and NLLB-200 (slower, two hundred
// languages, Serbian among them). "Auto" uses the AI when there is one and
// the offline engines otherwise, or when the AI fails.

type trReq struct {
	Texts        []string
	From, To     string // app codes; From may be "auto"
	Engine       string // auto | ai | offline
	Tone         string // "" | formal | casual (AI only)
	Alternatives bool   // a couple of other ways to say it (AI only)
	Glossary     bool
	About        string // what the text is ("a restaurant menu"), for the AI
}

type trRes struct {
	Texts        []string `json:"texts"`
	Detected     string   `json:"detected"`
	Engine       string   `json:"engine"`   // ai | argos | nllb
	Provider     string   `json:"provider"` // pocket | openai (AI)
	Alternatives []string `json:"alternatives"`
	Terms        int      `json:"terms"` // glossary terms used
	Note         string   `json:"note"`  // why another engine was used
}

// errNoEngine means nothing can translate this pair for this person.
type errNoEngine struct{ msg string }

func (e *errNoEngine) Error() string { return e.msg }

// engines returns the box's offline engines (rebuilt when their addresses change).
func (s *Server) engines(ctx context.Context) (argos, nllb *engine.Libre) {
	s.engMu.Lock()
	defer s.engMu.Unlock()
	st, _ := s.store.GetSettings(ctx, 0)
	if s.argos == nil || s.argos.URL != strings.TrimRight(st.ArgosURL, "/") || s.argos.Key != st.ArgosKey {
		s.argos = engine.NewLibre("argos", st.ArgosURL, st.ArgosKey)
	}
	if s.nllb == nil || s.nllb.URL != strings.TrimRight(st.NLLBURL, "/") || s.nllb.Key != st.NLLBKey {
		s.nllb = engine.NewLibre("nllb", st.NLLBURL, st.NLLBKey)
	}
	return s.argos, s.nllb
}

func (s *Server) resetEngines() {
	s.engMu.Lock()
	s.argos, s.nllb = nil, nil
	s.engMu.Unlock()
}

// offlineFor picks the offline engine for a pair (app codes): Argos when it
// knows it (quicker), else NLLB.
func (s *Server) offlineFor(ctx context.Context, from, to string) *engine.Libre {
	argos, nllb := s.engines(ctx)
	f, t := engine.Base(from), engine.Base(to)
	if from == "auto" {
		f = "auto"
	}
	if f != "auto" && f == t {
		if argos.Has(ctx, t) {
			return argos
		}
		if nllb.Has(ctx, t) {
			return nllb
		}
	}
	if argos.Knows(ctx, f, t) {
		return argos
	}
	if nllb.Knows(ctx, f, t) {
		return nllb
	}
	return nil
}

// offlineKnows reports which offline engines can put text into a language.
func (s *Server) offlineKnows(ctx context.Context, to string) (argos, nllb bool) {
	a, n := s.engines(ctx)
	t := engine.Base(to)
	return a.Knows(ctx, "auto", t), n.Knows(ctx, "auto", t)
}

// detect guesses a text's language: offline (NLLB's detector knows the
// most), else the AI; close languages go to the one the person uses.
func (s *Server) detect(ctx context.Context, uid int64, st store.Settings, text string) (string, string, error) {
	text = clip(text, 2000)
	mine := append([]string{st.TargetLang}, st.RecentLangs...)
	argos, nllb := s.engines(ctx)
	for _, l := range []*engine.Libre{nllb, argos} {
		if ok, _ := l.Health(ctx); !ok {
			continue
		}
		ds, err := l.Detect(ctx, text)
		if err == nil && len(ds) > 0 {
			if c := engine.Normalize(ds[0].Language); c != "" {
				return engine.Prefer(c, text, mine), l.Name, nil
			}
		}
	}
	cl, _, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		return "", "", &errNoEngine{firstNonEmpty(why, "no engine to tell the language: set up an offline engine or AI")}
	}
	var out struct {
		Language string `json:"language"`
	}
	if err := cl.JSON(ctx, `Say which language the user's text is written in. Answer JSON {"language": "<ISO 639-1 code>"}; for Serbian answer "sr-Latn" or "sr-Cyrl" by the alphabet.`, text, &out); err != nil {
		return "", "", err
	}
	c := engine.Normalize(out.Language)
	if c == "" || c == "auto" {
		return "", "", errors.New("the language could not be told")
	}
	return engine.Prefer(c, text, mine), "ai", nil
}

// translate runs a translation for one person.
func (s *Server) translate(ctx context.Context, uid int64, st store.Settings, req trReq) (*trRes, error) {
	req.To = engine.Normalize(req.To)
	if req.To == "" || req.To == "auto" {
		return nil, &errNoEngine{"pick the language to translate into"}
	}
	if req.From = engine.Normalize(req.From); req.From == "" {
		req.From = "auto"
	}
	if req.Engine == "" {
		req.Engine = st.Engine
	}
	empty := true
	for _, t := range req.Texts {
		if strings.TrimSpace(t) != "" {
			empty = false
		}
	}
	if empty {
		return &trRes{Texts: req.Texts, Detected: req.From, Alternatives: []string{}}, nil
	}
	var terms []*store.Term
	if req.Glossary {
		terms, _ = s.store.ListTerms(ctx, uid, "", req.To, "")
	}
	var cl *ai.Client
	provider, why := "", ""
	if req.Engine != "offline" {
		cl, provider, why = s.aiFor(ctx, uid, st)
	}
	note := ""
	if cl != nil {
		res, err := s.translateAI(ctx, cl, req, termsFor(terms, req.From))
		if err == nil {
			res.Provider = provider
			return res, nil
		}
		if req.Engine == "ai" || ctx.Err() != nil {
			return nil, err
		}
		note = "The AI did not answer (" + clip(err.Error(), 160) + "), so the offline engine translated this."
		s.log.Warn("ai translation failed, using offline", "err", err)
	} else if req.Engine == "ai" {
		return nil, &errNoEngine{firstNonEmpty(why, "no AI is set up (Settings → AI)")}
	}
	res, err := s.translateOffline(ctx, uid, st, req, terms)
	if err != nil {
		var ne *errNoEngine
		if errors.As(err, &ne) && req.Engine == "auto" && cl == nil && why != "" {
			return nil, &errNoEngine{ne.msg + "; and no AI either: " + why}
		}
		return nil, err
	}
	res.Note = note
	return res, nil
}

// termsFor keeps the glossary terms that apply to texts in a language.
func termsFor(terms []*store.Term, from string) []*store.Term {
	out := []*store.Term{}
	for _, t := range terms {
		if from == "auto" || t.SourceLang == "*" || t.SourceLang == from || engine.Base(t.SourceLang) == engine.Base(from) {
			out = append(out, t)
		}
	}
	return out
}

// ---- AI ----

func langLine(code string) string {
	switch code {
	case "sr-Latn":
		return "Serbian, written in the Latin alphabet (latinica)"
	case "sr-Cyrl":
		return "Serbian, written in the Cyrillic alphabet (ćirilica)"
	case "cnr":
		return "Montenegrin, in the Latin alphabet"
	case "zt":
		return "Chinese in traditional characters"
	case "zh":
		return "Chinese in simplified characters"
	case "pb":
		return "Brazilian Portuguese"
	}
	return engine.Name(code)
}

func aiSystem(req trReq, terms []*store.Term) string {
	var b strings.Builder
	b.WriteString("You are a careful professional translator. Translate each item of the user's JSON list ")
	if req.From != "auto" {
		b.WriteString("from " + langLine(req.From) + " ")
	}
	b.WriteString("into " + langLine(req.To) + ". ")
	b.WriteString("Translate the meaning naturally, as a native speaker would write it, not word for word. Keep line breaks, Markdown, numbers, names, e-mail and web addresses, placeholders like {name}, %s or {{x}} exactly. Do not add explanations or quotes. An item that is already in the target language comes back as it is (corrected only if it has obvious errors). ")
	switch req.Tone {
	case "formal":
		b.WriteString("Use a formal, polite register (formal you, e.g. Sie, vous, Vi). ")
	case "casual":
		b.WriteString("Use a casual, friendly register (informal you, e.g. du, tu, ti). ")
	}
	if req.About != "" {
		b.WriteString("The text is " + clip(req.About, 200) + ". ")
	}
	if len(terms) > 0 {
		b.WriteString("Always translate these terms exactly as given (adjusting only grammatical endings): ")
		for i, t := range terms {
			if i > 0 {
				b.WriteString("; ")
			}
			b.WriteString(fmt.Sprintf("%q → %q", t.Term, t.Translation))
			if t.Note != "" {
				b.WriteString(" (" + clip(t.Note, 80) + ")")
			}
		}
		b.WriteString(". ")
	}
	b.WriteString(`Answer JSON only: {"detected": "<ISO 639-1 code of the language the items are written in; for Serbian sr-Latn or sr-Cyrl by alphabet>", "items": ["<translation of item 1>", ...]`)
	if req.Alternatives {
		b.WriteString(`, "alternatives": ["<another good way to translate item 1>", "<a third>"]`)
	}
	b.WriteString(`}. "items" has exactly as many entries as the list, in the same order.`)
	return b.String()
}

// aiChunk is how much text goes to the AI at once.
const aiChunk = 6000

func (s *Server) translateAI(ctx context.Context, cl *ai.Client, req trReq, terms []*store.Term) (*trRes, error) {
	res := &trRes{Engine: "ai", Texts: make([]string, len(req.Texts)), Alternatives: []string{}}
	// batches by size, keeping empty items out of the AI's way
	type batch struct{ idx []int }
	var batches []batch
	cur, size := batch{}, 0
	for i, t := range req.Texts {
		if strings.TrimSpace(t) == "" {
			res.Texts[i] = t
			continue
		}
		n := utf8.RuneCountInString(t)
		if size > 0 && size+n > aiChunk {
			batches = append(batches, cur)
			cur, size = batch{}, 0
		}
		cur.idx = append(cur.idx, i)
		size += n
	}
	if len(cur.idx) > 0 {
		batches = append(batches, cur)
	}
	detected := map[string]int{}
	for _, bt := range batches {
		items := make([]string, len(bt.idx))
		for j, i := range bt.idx {
			items[j] = req.Texts[i]
		}
		out, det, alts, err := s.aiItems(ctx, cl, req, terms, items)
		if err != nil && len(items) > 1 {
			// one by one when the list came back wrong
			out = make([]string, len(items))
			for j, it := range items {
				o, d, _, e := s.aiItems(ctx, cl, req, terms, []string{it})
				if e != nil {
					return nil, e
				}
				out[j], det = o[0], d
			}
			err = nil
		}
		if err != nil {
			return nil, err
		}
		for j, i := range bt.idx {
			res.Texts[i] = engine.Script(req.To, keepEdges(req.Texts[i], out[j]))
		}
		if det != "" {
			detected[det]++
		}
		if req.Alternatives && len(req.Texts) == 1 {
			for _, a := range alts {
				if a = engine.Script(req.To, strings.TrimSpace(a)); a != "" && a != res.Texts[0] && !oneOf(a, res.Alternatives) && len(res.Alternatives) < 3 {
					res.Alternatives = append(res.Alternatives, a)
				}
			}
		}
	}
	res.Detected = req.From
	if req.From == "auto" {
		best, n := "", 0
		for k, v := range detected {
			if v > n {
				best, n = k, v
			}
		}
		res.Detected = best
	}
	for _, t := range terms {
		for _, x := range res.Texts {
			if strings.Contains(strings.ToLower(x), strings.ToLower(t.Translation)) {
				res.Terms++
				break
			}
		}
	}
	return res, nil
}

func (s *Server) aiItems(ctx context.Context, cl *ai.Client, req trReq, terms []*store.Term, items []string) ([]string, string, []string, error) {
	select {
	case s.aiSem <- struct{}{}:
		defer func() { <-s.aiSem }()
	case <-ctx.Done():
		return nil, "", nil, ctx.Err()
	}
	in, _ := json.Marshal(items)
	var out struct {
		Detected     string   `json:"detected"`
		Items        []string `json:"items"`
		Alternatives []string `json:"alternatives"`
	}
	if err := cl.JSON(ctx, aiSystem(req, terms), string(in), &out); err != nil {
		return nil, "", nil, err
	}
	if len(out.Items) != len(items) {
		return nil, "", nil, fmt.Errorf("the AI answered %d items for %d", len(out.Items), len(items))
	}
	det := engine.Normalize(out.Detected)
	if det == "auto" {
		det = ""
	}
	if det != "" {
		det = engine.Prefer(det, strings.Join(items, " "), nil)
	}
	return out.Items, det, out.Alternatives, nil
}

// keepEdges gives a translation the leading and trailing whitespace of
// its original (paragraph spacing in documents).
func keepEdges(orig, tr string) string {
	lead := orig[:len(orig)-len(strings.TrimLeft(orig, " \t\n"))]
	trail := orig[len(strings.TrimRight(orig, " \t\n")):]
	return lead + strings.TrimSpace(tr) + trail
}

// ---- offline ----

const offlineBatch = 24

func (s *Server) translateOffline(ctx context.Context, uid int64, st store.Settings, req trReq, terms []*store.Term) (*trRes, error) {
	argos, nllb := s.engines(ctx)
	if !argos.Configured() && !nllb.Configured() {
		return nil, &errNoEngine{"there is no offline engine on this box (an admin sets one up under Settings → Engines)"}
	}
	from := req.From
	if from == "auto" {
		sample := strings.Join(req.Texts, "\n")
		d, _, err := s.detect(ctx, uid, st, sample)
		if err != nil {
			return nil, err
		}
		from = d
	}
	res := &trRes{Detected: from, Alternatives: []string{}}
	if engine.Base(from) == engine.Base(req.To) {
		// same language: only the alphabet may change
		res.Engine = "none"
		res.Texts = make([]string, len(req.Texts))
		for i, t := range req.Texts {
			res.Texts[i] = engine.Script(req.To, t)
		}
		return res, nil
	}
	l := s.offlineFor(ctx, from, req.To)
	if l == nil {
		ok1, _ := argos.Health(ctx)
		ok2, _ := nllb.Health(ctx)
		if !ok1 && !ok2 {
			return nil, &errNoEngine{"the offline engines are not ready yet (they may still be downloading their language models)"}
		}
		return nil, &errNoEngine{"no offline engine translates " + engine.Name(from) + " into " + engine.Name(req.To)}
	}
	res.Engine = l.Name
	terms = termsFor(terms, from)
	// glossary terms go through the engine as [n] placeholders (both
	// engines keep them where they belong) and come back as the person's words
	sorted := append([]*store.Term(nil), terms...)
	sort.SliceStable(sorted, func(i, j int) bool { return len([]rune(sorted[i].Term)) > len([]rune(sorted[j].Term)) })
	marks := make([]map[string]*store.Term, len(req.Texts))
	in := make([]string, len(req.Texts))
	for i, t := range req.Texts {
		if len(sorted) > 0 && !strings.Contains(t, "[") {
			for n, term := range sorted {
				if !hasTerm(t, term.Term, term.MatchCase) {
					continue
				}
				ph := fmt.Sprintf("[%d]", n+1)
				t = wordPattern(term.Term, term.MatchCase).ReplaceAllString(t, "${1}"+ph+"${3}")
				if marks[i] == nil {
					marks[i] = map[string]*store.Term{}
				}
				marks[i][ph] = term
			}
		}
		if engine.Base(from) == "sr" && l.Name == "nllb" {
			t = engine.ToCyrillic(t) // NLLB reads Serbian in Cyrillic
		}
		in[i] = t
	}
	out, err := s.offlineRun(ctx, l, from, req.To, req.Texts, in)
	if err != nil {
		return nil, err
	}
	used := map[int64]bool{}
	var lost []int
	for k, m := range marks {
		ok := true
		for ph := range m {
			if !strings.Contains(out[k], ph) {
				ok = false
			}
		}
		if !ok {
			lost = append(lost, k)
			continue
		}
		for ph, term := range m {
			out[k] = strings.ReplaceAll(out[k], ph, engine.Script(req.To, term.Translation))
			used[term.ID] = true
		}
	}
	if len(lost) > 0 {
		// a placeholder did not survive: those texts go again as they are,
		// with the engine's own word for each term swapped afterwards
		src, plain := make([]string, len(lost)), make([]string, len(lost))
		for n, k := range lost {
			src[n] = req.Texts[k]
			plain[n] = req.Texts[k]
			if engine.Base(from) == "sr" && l.Name == "nllb" {
				plain[n] = engine.ToCyrillic(plain[n])
			}
		}
		again, err := s.offlineRun(ctx, l, from, req.To, src, plain)
		if err != nil {
			return nil, err
		}
		s.applyGlossary(ctx, l, from, req.To, src, again, terms)
		for n, k := range lost {
			out[k] = again[n]
			for _, term := range terms {
				if hasTerm(src[n], term.Term, term.MatchCase) && hasTerm(again[n], engine.Script(req.To, term.Translation), false) {
					used[term.ID] = true
				}
			}
		}
	}
	res.Terms = len(used)
	res.Texts = out
	return res, nil
}

// offlineRun translates prepared texts in batches; empty ones stay as they were.
func (s *Server) offlineRun(ctx context.Context, l *engine.Libre, from, to string, orig, in []string) ([]string, error) {
	out := make([]string, len(in))
	for i := 0; i < len(in); i += offlineBatch {
		j := min(i+offlineBatch, len(in))
		part, idx := []string{}, []int{}
		for k := i; k < j; k++ {
			if strings.TrimSpace(in[k]) == "" {
				out[k] = orig[k]
				continue
			}
			part = append(part, in[k])
			idx = append(idx, k)
		}
		if len(part) == 0 {
			continue
		}
		tr, _, err := l.Translate(ctx, part, engine.Base(from), engine.Base(to))
		if err != nil {
			return nil, offlineErr(l, err)
		}
		for n, k := range idx {
			out[k] = engine.Script(to, keepEdges(orig[k], tr[n]))
		}
	}
	return out, nil
}

func offlineErr(l *engine.Libre, err error) error {
	name := "Argos (LibreTranslate)"
	if l.Name == "nllb" {
		name = "NLLB"
	}
	if errors.Is(err, engine.ErrNotReady) {
		return &errNoEngine{"the offline engine " + name + " is not ready: " + strings.TrimPrefix(err.Error(), engine.ErrNotReady.Error()+": ")}
	}
	return fmt.Errorf("%s: %w", name, err)
}

// applyGlossary puts a person's own words into an offline translation: the
// engine's own translation of each term found in the text is swapped for
// the person's. It returns how many terms were used.
func (s *Server) applyGlossary(ctx context.Context, l *engine.Libre, from, to string, src, out []string, terms []*store.Term) int {
	found := []*store.Term{}
	for _, t := range terms {
		for _, x := range src {
			if hasTerm(x, t.Term, t.MatchCase) {
				found = append(found, t)
				break
			}
		}
	}
	if len(found) == 0 {
		return 0
	}
	words := make([]string, len(found))
	for i, t := range found {
		words[i] = t.Term
		if engine.Base(from) == "sr" && l.Name == "nllb" {
			words[i] = engine.ToCyrillic(t.Term)
		}
	}
	theirs, _, err := l.Translate(ctx, words, engine.Base(from), engine.Base(to))
	if err != nil {
		return 0
	}
	used := 0
	for i, t := range found {
		mine := engine.Script(to, t.Translation)
		w := strings.Trim(engine.Script(to, theirs[i]), " .,!?;:")
		hit := false
		for k := range out {
			if !hasTerm(src[k], t.Term, t.MatchCase) {
				continue
			}
			if w != "" && hasTerm(out[k], w, false) {
				out[k] = replaceWord(out[k], w, mine)
				hit = true
			} else if hasTerm(out[k], t.Term, t.MatchCase) {
				// the engine left the term as it was
				out[k] = replaceWord(out[k], t.Term, mine)
				hit = true
			}
		}
		if hit {
			used++
		}
	}
	return used
}

var wordRe sync.Map // pattern → *regexp.Regexp

func wordPattern(w string, matchCase bool) *regexp.Regexp {
	key := fmt.Sprint(matchCase, w)
	if r, ok := wordRe.Load(key); ok {
		return r.(*regexp.Regexp)
	}
	p := `(^|[^\p{L}\p{N}])(` + regexp.QuoteMeta(w) + `)($|[^\p{L}\p{N}])`
	if !matchCase {
		p = "(?i)" + p
	}
	r := regexp.MustCompile(p)
	wordRe.Store(key, r)
	return r
}

func hasTerm(text, term string, matchCase bool) bool {
	if strings.TrimSpace(term) == "" {
		return false
	}
	return wordPattern(term, matchCase).MatchString(text)
}

func replaceWord(text, w, with string) string {
	return wordPattern(w, false).ReplaceAllStringFunc(text, func(m string) string {
		sub := wordPattern(w, false).FindStringSubmatch(m)
		rep := with
		// "Word" at the start of a sentence stays capitalised
		if r, _ := utf8.DecodeRuneInString(sub[2]); r != utf8.RuneError && strings.ToUpper(string(r)) == string(r) && strings.ToLower(string(r)) != string(r) {
			if f, n := utf8.DecodeRuneInString(rep); f != utf8.RuneError {
				rep = strings.ToUpper(string(f)) + rep[n:]
			}
		}
		return sub[1] + rep + sub[3]
	})
}

// ---- explaining (AI only) ----

type explanation struct {
	Meaning  string `json:"meaning"`
	Literal  string `json:"literal"`
	Usage    string `json:"usage"`
	Register string `json:"register"`
	Examples []struct {
		Text        string `json:"text"`
		Translation string `json:"translation"`
	} `json:"examples"`
}

func (s *Server) explain(ctx context.Context, uid int64, st store.Settings, text, from, to string) (*explanation, error) {
	cl, _, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		return nil, &errNoEngine{firstNonEmpty(why, "explaining needs AI (Settings → AI)")}
	}
	sys := "You explain a word or phrase to someone learning the language. Write the explanation in " + langLine(to) + ", plainly and briefly. "
	if from != "" && from != "auto" {
		sys += "The phrase is in " + langLine(from) + ". "
	}
	sys += `Answer JSON {"meaning": "what it means and when it is said", "literal": "a word-for-word rendering, if it differs", "usage": "how and where it is used, pitfalls", "register": "formal, casual, rude, old-fashioned… in a few words", "examples": [{"text": "a short example in the phrase's language", "translation": "its translation"}]} with two or three examples.`
	select {
	case s.aiSem <- struct{}{}:
		defer func() { <-s.aiSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	var out explanation
	if err := cl.JSON(ctx, sys, clip(text, 1000), &out); err != nil {
		return nil, err
	}
	for i := range out.Examples {
		out.Examples[i].Translation = engine.Script(to, out.Examples[i].Translation)
	}
	out.Meaning, out.Usage, out.Literal, out.Register = engine.Script(to, out.Meaning), engine.Script(to, out.Usage), engine.Script(to, out.Literal), engine.Script(to, out.Register)
	return &out, nil
}
