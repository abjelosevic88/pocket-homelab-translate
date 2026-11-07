package server

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-translate/internal/engine"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// ---- export and import ----
//
// The export is one person's everything (documents as a list, without the
// files). Importing an export again changes nothing: rows are matched by
// their export id. Google Translate's saved phrases (Takeout, "Saved
// translations.csv") come in as starred phrases.

type exportConv struct {
	store.Conversation
	TurnsList []*store.Turn `json:"turns_list"`
}

type exportDoc struct {
	App           string               `json:"app"`
	Version       string               `json:"version"`
	ExportedAt    string               `json:"exported_at"`
	Translations  []*store.Translation `json:"translations"`
	Glossary      []*store.Term        `json:"glossary"`
	Conversations []exportConv         `json:"conversations"`
	Documents     []*store.Document    `json:"documents"`
}

func (s *Server) export(ctx context.Context, uid int64) exportDoc {
	d := exportDoc{App: "pocket-translate", Version: Version, ExportedAt: store.Now()}
	d.Translations, _ = s.store.AllTranslations(ctx, uid)
	d.Glossary, _ = s.store.ListTerms(ctx, uid, "", "", "")
	convs, _ := s.store.ListConversations(ctx, uid, 500)
	for _, c := range convs {
		turns, _ := s.store.Turns(ctx, uid, c.ID)
		d.Conversations = append(d.Conversations, exportConv{Conversation: *c, TurnsList: turns})
	}
	d.Documents, _ = s.store.ListDocuments(ctx, uid, 500)
	if d.Conversations == nil {
		d.Conversations = []exportConv{}
	}
	return d
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	doc := s.export(r.Context(), actorID(r.Context()))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="pocket-translate-%s.json"`, time.Now().Format("2006-01-02")))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(doc)
}

// handleExportCSV is the history and phrasebook as a spreadsheet.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.AllTranslations(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	starredOnly := queryBool(r, "starred")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	name := "pocket-translate-history"
	if starredOnly {
		name = "pocket-translate-phrasebook"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.csv"`, name, time.Now().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "from", "to", "text", "translation", "starred", "collection", "note", "kind", "engine"})
	for _, t := range list {
		if starredOnly && !t.Starred {
			continue
		}
		st := ""
		if t.Starred {
			st = "yes"
		}
		_ = cw.Write([]string{t.CreatedAt, t.From(), t.TargetLang, t.SourceText, t.ResultText, st, t.Collection, t.Note, t.Kind, t.Engine})
	}
	cw.Flush()
}

func exportID(parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return "x" + hex.EncodeToString(h[:10])
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	body, err := io.ReadAll(io.LimitReader(r.Body, s.cfg.MaxBodyBytes))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "the file did not arrive whole")
		return
	}
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var doc exportDoc
		if err := json.Unmarshal(trimmed, &doc); err != nil || doc.App != "pocket-translate" {
			writeErr(w, http.StatusBadRequest, "this is not a Pocket Translate export")
			return
		}
		writeJSON(w, http.StatusOK, s.importPocket(ctx, uid, doc))
		return
	}
	res, err := s.importCSV(ctx, uid, body, r.URL.Query().Get("collection"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) importPocket(ctx context.Context, uid int64, doc exportDoc) map[string]any {
	added, same := 0, 0
	for _, t := range doc.Translations {
		if t == nil || strings.TrimSpace(t.SourceText) == "" || engine.Normalize(t.TargetLang) == "" {
			continue
		}
		n := *t
		n.ID = 0
		n.Source = ""
		if n.ExternalID == "" {
			n.ExternalID = exportID(t.CreatedAt, t.SourceText, t.TargetLang)
		}
		dup, err := s.store.AddTranslation(ctx, uid, &n)
		if err != nil {
			continue
		}
		if dup {
			same++
		} else {
			added++
		}
	}
	terms := 0
	for _, t := range doc.Glossary {
		if t == nil || t.Term == "" || t.Translation == "" {
			continue
		}
		n := *t
		if isNew, err := s.store.UpsertTerm(ctx, uid, &n); err == nil && isNew {
			terms++
		}
	}
	convs := 0
	have, _ := s.store.ListConversations(ctx, uid, 500)
	seen := map[string]bool{}
	for _, c := range have {
		seen[c.CreatedAt+c.LangA+c.LangB] = true
	}
	for _, c := range doc.Conversations {
		if c.LangA == "" || c.LangB == "" || seen[c.CreatedAt+c.LangA+c.LangB] {
			continue
		}
		n := c.Conversation
		if err := s.store.AddConversation(ctx, uid, &n); err != nil {
			continue
		}
		for _, t := range c.TurnsList {
			if t == nil {
				continue
			}
			nt := *t
			nt.ConversationID = n.ID
			_ = s.store.AddTurn(ctx, uid, &nt)
		}
		convs++
	}
	return map[string]any{"added": added, "unchanged": same, "terms": terms, "conversations": convs}
}

// importCSV takes Google Translate's saved phrases (four columns: from
// language, to language, text, translation — the languages by name) or
// this app's own CSV (with a header).
func (s *Server) importCSV(ctx context.Context, uid int64, data []byte, collection string) (map[string]any, error) {
	sep := ','
	if first, _, _ := bytes.Cut(data, []byte("\n")); bytes.Count(first, []byte("\t")) > bytes.Count(first, []byte(",")) {
		sep = '\t'
	}
	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma, cr.FieldsPerRecord, cr.LazyQuotes = sep, -1, true
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("this is not a CSV file: %v", err)
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("the file is empty")
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	_, own := col["translation"]
	start := 0
	if own {
		start = 1
	}
	at := func(rec []string, i int) string {
		if i < 0 || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	idx := func(k string, def int) int {
		if !own {
			return def
		}
		if v, ok := col[k]; ok {
			return v
		}
		return -1
	}
	added, same, skipped := 0, 0, 0
	for _, rec := range recs[start:] {
		from, to := engine.Normalize(at(rec, idx("from", 0))), engine.Normalize(at(rec, idx("to", 1)))
		text, tr := at(rec, idx("text", 2)), at(rec, idx("translation", 3))
		if text == "" || tr == "" || to == "" || to == "auto" {
			skipped++
			continue
		}
		if from == "" {
			from = "auto"
		}
		t := &store.Translation{Kind: "import", SourceLang: from, TargetLang: to, SourceText: text, ResultText: tr, Engine: "import",
			Starred: true, Collection: clip(collection, 80), ExternalID: exportID(from, to, text)}
		if from != "auto" {
			t.DetectedLang = from
		}
		if own {
			if _, ok := col["starred"]; ok {
				t.Starred = at(rec, idx("starred", -1)) == "yes"
			}
			t.Collection = firstNonEmpty(at(rec, idx("collection", -1)), t.Collection)
			t.Note = at(rec, idx("note", -1))
			if d := at(rec, idx("date", -1)); d != "" {
				t.CreatedAt = d
			}
			t.ExternalID = exportID(t.CreatedAt, text, to)
		}
		dup, err := s.store.AddTranslation(ctx, uid, t)
		switch {
		case err != nil:
			skipped++
		case dup:
			same++
		default:
			added++
		}
	}
	return map[string]any{"added": added, "unchanged": same, "skipped": skipped}, nil
}
