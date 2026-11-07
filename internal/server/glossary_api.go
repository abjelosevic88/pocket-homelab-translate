package server

import (
	"bytes"
	"encoding/csv"
	"io"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-translate/internal/engine"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// ---- the glossary: a person's own words ----

type termIn struct {
	SourceLang  string `json:"source_lang"`
	TargetLang  string `json:"target_lang"`
	Term        string `json:"term"`
	Translation string `json:"translation"`
	Note        string `json:"note"`
	MatchCase   bool   `json:"match_case"`
}

// termLang is a glossary language: a code, or "*" for any.
func termLang(v string) string {
	if strings.TrimSpace(v) == "*" || strings.EqualFold(strings.TrimSpace(v), "any") {
		return "*"
	}
	c := engine.Normalize(v)
	if c == "auto" {
		return "*"
	}
	return c
}

func (in termIn) term() (*store.Term, string) {
	t := &store.Term{SourceLang: termLang(in.SourceLang), TargetLang: termLang(in.TargetLang), Term: strings.TrimSpace(in.Term),
		Translation: strings.TrimSpace(in.Translation), Note: clip(strings.TrimSpace(in.Note), 300), MatchCase: in.MatchCase}
	switch {
	case t.SourceLang == "" || t.TargetLang == "":
		return nil, "unknown language"
	case t.TargetLang == "*":
		return nil, "pick the language the term is translated into"
	case t.Term == "" || t.Translation == "":
		return nil, "write the term and how it is always translated"
	case len([]rune(t.Term)) > 200 || len([]rune(t.Translation)) > 200:
		return nil, "a term is up to 200 characters"
	}
	return t, ""
}

func (s *Server) handleListTerms(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	from, to := termLang(q.Get("from")), termLang(q.Get("to"))
	if from == "*" {
		from = ""
	}
	if to == "*" {
		to = ""
	}
	list, err := s.store.ListTerms(ctx, actorID(ctx), from, to, q.Get("q"))
	if err != nil {
		s.fail(w, err, "glossary")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"terms": list})
}

func (s *Server) handleAddTerm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in termIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	t, msg := in.term()
	if t == nil {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.AddTerm(ctx, actorID(ctx), t); err != nil {
		s.fail(w, err, "add term")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleUpdateTerm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	old, err := s.store.GetTerm(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "term")
		return
	}
	var in termIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	t, msg := in.term()
	if t == nil {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	t.ID, t.CreatedAt = old.ID, old.CreatedAt
	if err := s.store.UpdateTerm(ctx, actorID(ctx), t); err != nil {
		s.fail(w, err, "update term")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTerm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteTerm(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete term")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleImportTerms takes a CSV or TSV: "term,translation" (DeepL's
// glossary format) for the pair asked, or with a header
// source_lang,target_lang,term,translation[,note] (this app's export).
func (s *Server) handleImportTerms(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "the file did not arrive whole")
		return
	}
	from, to := termLang(r.URL.Query().Get("from")), termLang(r.URL.Query().Get("to"))
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	sep := ','
	if first, _, _ := bytes.Cut(body, []byte("\n")); bytes.Count(first, []byte("\t")) > bytes.Count(first, []byte(",")) {
		sep = '\t'
	} else if bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		sep = ';'
	}
	cr := csv.NewReader(bytes.NewReader(body))
	cr.Comma, cr.FieldsPerRecord, cr.LazyQuotes = sep, -1, true
	recs, err := cr.ReadAll()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "the file is not a CSV or TSV: "+err.Error())
		return
	}
	col := map[string]int{}
	start := 0
	if len(recs) > 0 {
		for i, h := range recs[0] {
			col[strings.ToLower(strings.TrimSpace(h))] = i
		}
		if _, ok := col["term"]; ok {
			start = 1
		} else {
			col = nil
		}
	}
	get := func(rec []string, k string, def int) string {
		i := def
		if col != nil {
			v, ok := col[k]
			if !ok {
				return ""
			}
			i = v
		}
		if i < 0 || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	added, updated, skipped := 0, 0, 0
	for _, rec := range recs[start:] {
		in := termIn{SourceLang: from, TargetLang: to, Term: get(rec, "term", 0), Translation: get(rec, "translation", 1), Note: get(rec, "note", 2)}
		if col != nil {
			in.SourceLang = firstNonEmpty(get(rec, "source_lang", -1), from)
			in.TargetLang = firstNonEmpty(get(rec, "target_lang", -1), to)
			in.MatchCase = get(rec, "match_case", -1) == "true"
		}
		t, msg := in.term()
		if t == nil {
			if msg == "unknown language" || msg == "pick the language the term is translated into" {
				writeErr(w, http.StatusBadRequest, "say which languages the terms are in (from and to)")
				return
			}
			skipped++
			continue
		}
		isNew, err := s.store.UpsertTerm(ctx, uid, t)
		if err != nil {
			skipped++
			continue
		}
		if isNew {
			added++
		} else {
			updated++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "updated": updated, "skipped": skipped})
}

func (s *Server) handleExportTerms(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListTerms(ctx, actorID(ctx), "", "", "")
	if err != nil {
		s.fail(w, err, "glossary")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-translate-glossary.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"source_lang", "target_lang", "term", "translation", "note", "match_case"})
	for _, t := range list {
		mc := "false"
		if t.MatchCase {
			mc = "true"
		}
		_ = cw.Write([]string{t.SourceLang, t.TargetLang, t.Term, t.Translation, t.Note, mc})
	}
	cw.Flush()
}
