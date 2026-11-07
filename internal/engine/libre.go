package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Libre is a server that speaks the LibreTranslate API: LibreTranslate
// (Argos models) or Pocket Translate's NLLB engine.
type Libre struct {
	Name string // argos | nllb
	URL  string
	Key  string
	HTTP *http.Client

	mu    sync.Mutex
	pairs map[string]map[string]bool // source → targets (engine codes)
	at    time.Time
	err   error
}

// NewLibre makes a client; an empty URL means the engine is not set up.
func NewLibre(name, url, key string) *Libre {
	return &Libre{Name: name, URL: strings.TrimRight(url, "/"), Key: key, HTTP: &http.Client{Timeout: 10 * time.Minute}}
}

// Configured reports whether there is an address.
func (l *Libre) Configured() bool { return l != nil && l.URL != "" }

// ErrNotReady is returned while the engine cannot answer (not set up,
// starting, downloading its models).
var ErrNotReady = errors.New("the offline engine is not ready")

func (l *Libre) post(ctx context.Context, path string, body map[string]any, v any) error {
	if !l.Configured() {
		return ErrNotReady
	}
	if l.Key != "" {
		body["api_key"] = l.Key
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.URL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := l.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotReady, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := strings.TrimSpace(e.Error)
		if msg == "" {
			msg = res.Status
		}
		if res.StatusCode == http.StatusServiceUnavailable || res.StatusCode == http.StatusBadGateway {
			return fmt.Errorf("%w: %s", ErrNotReady, msg)
		}
		return errors.New(msg)
	}
	return json.Unmarshal(raw, v)
}

// Pairs returns which languages translate into which (engine codes),
// asked once every ten minutes (a minute after a failure).
func (l *Libre) Pairs(ctx context.Context) (map[string]map[string]bool, error) {
	if !l.Configured() {
		return nil, ErrNotReady
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fresh := 10 * time.Minute
	if l.err != nil {
		fresh = time.Minute
	}
	if time.Since(l.at) < fresh {
		return l.pairs, l.err
	}
	l.pairs, l.err = l.fetchPairs(ctx)
	l.at = time.Now()
	return l.pairs, l.err
}

func (l *Libre) fetchPairs(ctx context.Context) (map[string]map[string]bool, error) {
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, l.URL+"/languages", nil)
	if err != nil {
		return nil, err
	}
	res, err := l.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotReady, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s", ErrNotReady, res.Status)
	}
	var langs []struct {
		Code    string   `json:"code"`
		Targets []string `json:"targets"`
	}
	if err := json.NewDecoder(res.Body).Decode(&langs); err != nil {
		return nil, err
	}
	out := map[string]map[string]bool{}
	for _, lg := range langs {
		m := map[string]bool{}
		for _, t := range lg.Targets {
			m[t] = true
		}
		out[lg.Code] = m
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: it has no languages yet", ErrNotReady)
	}
	return out, nil
}

// Forget drops the cached language list (the address changed).
func (l *Libre) Forget() {
	l.mu.Lock()
	l.at = time.Time{}
	l.mu.Unlock()
}

// variants are the other codes servers know a language by (LibreTranslate
// writes Chinese as zh-Hans / zh-Hant, older versions zh / zt).
var variants = map[string][]string{"zh": {"zh-Hans", "zh-CN"}, "zt": {"zh-Hant", "zh-TW"}, "pb": {"pt-BR"}, "nb": {"no"}, "tl": {"fil"}}

// code is how this server writes a language (Base code in, its code out).
func (l *Libre) code(ctx context.Context, c string) string {
	vs, ok := variants[c]
	if !ok || c == "auto" {
		return c
	}
	p, err := l.Pairs(ctx)
	if err != nil {
		return c
	}
	if _, ok := p[c]; ok {
		return c
	}
	for _, v := range vs {
		if _, ok := p[v]; ok {
			return v
		}
	}
	return c
}

// Knows reports whether the engine translates from → to (engine codes;
// from "auto" = it knows the target).
func (l *Libre) Knows(ctx context.Context, from, to string) bool {
	from, to = l.code(ctx, from), l.code(ctx, to)
	p, err := l.Pairs(ctx)
	if err != nil {
		return false
	}
	if from == "auto" || from == "" {
		for _, ts := range p {
			if ts[to] {
				return true
			}
		}
		return false
	}
	return p[from][to]
}

// Has reports whether the engine knows a language at all (engine code).
func (l *Libre) Has(ctx context.Context, code string) bool {
	code = l.code(ctx, code)
	p, err := l.Pairs(ctx)
	if err != nil {
		return false
	}
	if _, ok := p[code]; ok {
		return true
	}
	for _, ts := range p {
		if ts[code] {
			return true
		}
	}
	return false
}

// Translate translates texts (engine codes; from may be "auto"). It
// answers the translations and, for auto, the language detected.
func (l *Libre) Translate(ctx context.Context, texts []string, from, to string) ([]string, string, error) {
	if len(texts) == 0 {
		return []string{}, "", nil
	}
	if from == "" {
		from = "auto"
	}
	from, to = l.code(ctx, from), l.code(ctx, to)
	var out struct {
		TranslatedText   json.RawMessage `json:"translatedText"`
		DetectedLanguage json.RawMessage `json:"detectedLanguage"`
	}
	if err := l.post(ctx, "/translate", map[string]any{"q": texts, "source": from, "target": to, "format": "text"}, &out); err != nil {
		return nil, "", err
	}
	var res []string
	if err := json.Unmarshal(out.TranslatedText, &res); err != nil {
		var one string
		if err := json.Unmarshal(out.TranslatedText, &one); err != nil {
			return nil, "", fmt.Errorf("unexpected answer from %s", l.Name)
		}
		res = []string{one}
	}
	if len(res) != len(texts) {
		return nil, "", fmt.Errorf("%s answered %d texts for %d", l.Name, len(res), len(texts))
	}
	detected := ""
	if len(out.DetectedLanguage) > 0 {
		var d struct {
			Language string `json:"language"`
		}
		if json.Unmarshal(out.DetectedLanguage, &d) != nil {
			var ds []struct {
				Language string `json:"language"`
			}
			if json.Unmarshal(out.DetectedLanguage, &ds) == nil && len(ds) > 0 {
				d.Language = ds[0].Language
			}
		}
		detected = d.Language
	}
	return res, detected, nil
}

// Detection is one guess of a text's language.
type Detection struct {
	Language   string  `json:"language"`
	Confidence float64 `json:"confidence"`
}

// Detect guesses the language of a text (engine codes, best first).
func (l *Libre) Detect(ctx context.Context, text string) ([]Detection, error) {
	var out []Detection
	if err := l.post(ctx, "/detect", map[string]any{"q": text}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Health says whether the engine answers, and if not why.
func (l *Libre) Health(ctx context.Context) (bool, string) {
	if !l.Configured() {
		return false, "not set up"
	}
	if _, err := l.Pairs(ctx); err != nil {
		return false, strings.TrimPrefix(err.Error(), ErrNotReady.Error()+": ")
	}
	return true, ""
}
