package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-translate/internal/docs"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/engine"
)

// ---- the LibreTranslate API ----
//
// /translate, /detect and /languages answer like LibreTranslate, so the
// browser extensions, apps and scripts made for it work with Pocket
// Translate: the "API key" is a Pocket Translate API token (Settings → API
// & shortcuts). Translations go through the person's own engine choice and
// glossary, and are not kept in the history.

type compatKey struct{}

// compatBody reads a LibreTranslate request (JSON or a form) and moves its
// api_key into the Authorization header for requireAuth.
func (s *Server) compat(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		in := map[string]any{}
		if r.Method == http.MethodPost {
			raw, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
			ct := r.Header.Get("Content-Type")
			switch {
			case strings.Contains(ct, "json"):
				_ = json.Unmarshal(raw, &in)
			case strings.HasPrefix(ct, "multipart/"):
				r.Body = io.NopCloser(bytes.NewReader(raw))
				if r.ParseMultipartForm(8<<20) == nil {
					for k, v := range r.MultipartForm.Value {
						in[k] = one(v)
					}
				}
			default:
				q, _ := url.ParseQuery(string(raw))
				for k, v := range q {
					in[k] = one(v)
				}
			}
		}
		for k, v := range r.URL.Query() {
			if _, ok := in[k]; !ok {
				in[k] = one(v)
			}
		}
		if key, _ := in["api_key"].(string); key != "" && r.Header.Get("Authorization") == "" {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		ctx := context.WithValue(r.Context(), compatKey{}, in)
		s.requireAuth(next).ServeHTTP(w, r.WithContext(ctx))
	})
}

func one(v []string) any {
	if len(v) == 1 {
		return v[0]
	}
	out := make([]any, len(v))
	for i, x := range v {
		out[i] = x
	}
	return out
}

func compatIn(r *http.Request) map[string]any {
	in, _ := r.Context().Value(compatKey{}).(map[string]any)
	if in == nil {
		in = map[string]any{}
	}
	return in
}

func compatErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// compatCode is the code LibreTranslate clients expect for one of ours.
func compatCode(c string) string {
	switch c {
	case "sr-Latn", "sr-Cyrl":
		return "sr"
	}
	return c
}

func (s *Server) handleCompatTranslate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	in := compatIn(r)
	var texts []string
	single := true
	switch q := in["q"].(type) {
	case string:
		texts = []string{q}
	case []any:
		single = false
		for _, x := range q {
			v, _ := x.(string)
			texts = append(texts, v)
		}
	default:
		compatErr(w, http.StatusBadRequest, "Invalid request: missing q parameter")
		return
	}
	src, _ := in["source"].(string)
	tgt, _ := in["target"].(string)
	to := engine.Normalize(tgt)
	if to == "" || to == "auto" {
		compatErr(w, http.StatusBadRequest, "Invalid request: unknown target "+tgt)
		return
	}
	from := engine.Normalize(firstNonEmpty(src, "auto"))
	if from == "" {
		compatErr(w, http.StatusBadRequest, "Invalid request: unknown source "+src)
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	format, _ := in["format"].(string)
	alts := 0
	switch a := in["alternatives"].(type) {
	case float64:
		alts = int(a)
	case string:
		if a != "" && a != "0" {
			alts = 2
		}
	}
	var htmlDocs []docs.Doc
	if format == "html" {
		var pieces []string
		for _, t := range texts {
			d, err := docs.Open("html", []byte(t), "")
			if err != nil {
				compatErr(w, http.StatusBadRequest, err.Error())
				return
			}
			htmlDocs = append(htmlDocs, d)
			pieces = append(pieces, d.Pieces()...)
		}
		texts = pieces
	}
	res, err := s.translate(ctx, u.ID, st, trReq{Texts: texts, From: from, To: to, Tone: st.Tone, Glossary: true, Alternatives: alts > 0 && single && format != "html"})
	if err != nil {
		var ne *errNoEngine
		if asNoEngine(err, &ne) {
			compatErr(w, http.StatusServiceUnavailable, ne.msg)
			return
		}
		compatErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := res.Texts
	if format == "html" {
		out = []string{}
		k := 0
		for _, d := range htmlDocs {
			n := len(d.Pieces())
			b, _ := d.Build(res.Texts[k : k+n])
			k += n
			out = append(out, string(b))
		}
	}
	ans := map[string]any{}
	if single {
		ans["translatedText"] = out[0]
		if alts > 0 {
			ans["alternatives"] = res.Alternatives
		}
	} else {
		ans["translatedText"] = out
	}
	if from == "auto" {
		ans["detectedLanguage"] = map[string]any{"language": compatCode(res.Detected), "confidence": 90}
	}
	writeJSON(w, http.StatusOK, ans)
}

func (s *Server) handleCompatDetect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	q, _ := compatIn(r)["q"].(string)
	if strings.TrimSpace(q) == "" {
		compatErr(w, http.StatusBadRequest, "Invalid request: missing q parameter")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	code, _, err := s.detect(ctx, u.ID, st, q)
	if err != nil {
		compatErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, []map[string]any{{"language": compatCode(code), "confidence": 90}})
}

func (s *Server) handleCompatLanguages(w http.ResponseWriter, r *http.Request) {
	codes := []string{}
	seen := map[string]bool{}
	for _, l := range engine.Langs {
		c := compatCode(l.Code)
		if !seen[c] {
			seen[c] = true
			codes = append(codes, c)
		}
	}
	out := []map[string]any{}
	for _, c := range codes {
		targets := []string{}
		for _, t := range codes {
			if t != c {
				targets = append(targets, t)
			}
		}
		name := engine.Name(c)
		if c == "sr" {
			name = "Serbian"
		}
		out = append(out, map[string]any{"code": c, "name": name, "targets": targets})
	}
	writeJSON(w, http.StatusOK, out)
}
