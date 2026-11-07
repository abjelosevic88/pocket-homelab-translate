package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-translate/internal/engine"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// ---- history and the phrasebook (starred translations) ----

func (s *Server) handleListTranslations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	f := store.TranslationFilter{Q: q.Get("q"), Starred: queryBool(r, "starred"), Kind: q.Get("kind"), Lang: engine.Normalize(q.Get("lang")),
		Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset")}
	if q.Has("collection") {
		c := q.Get("collection")
		f.Collection = &c
	}
	if f.Lang == "auto" {
		f.Lang = ""
	}
	list, total, err := s.store.ListTranslations(ctx, actorID(ctx), f)
	if err != nil {
		s.fail(w, err, "translations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"translations": list, "total": total})
}

// ownTranslation loads one of the caller's translations; a linked app sees
// every one (phrasebook:read) but may change only those it added.
func (s *Server) ownTranslation(w http.ResponseWriter, r *http.Request, write bool) (*store.Translation, bool) {
	ctx := r.Context()
	t, err := s.store.GetTranslation(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "translation")
		return nil, false
	}
	if app := linkApp(ctx); write && app != "" && t.Source != app {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	return t, true
}

func (s *Server) handleGetTranslation(w http.ResponseWriter, r *http.Request) {
	if t, ok := s.ownTranslation(w, r, false); ok {
		writeJSON(w, http.StatusOK, t)
	}
}

// handleAddTranslation adds a phrase by hand (or from another app) to the
// phrasebook. Without a translation it is translated first.
func (s *Server) handleAddTranslation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		SourceText string `json:"source_text"`
		ResultText string `json:"result_text"`
		SourceLang string `json:"source_lang"`
		TargetLang string `json:"target_lang"`
		Collection string `json:"collection"`
		Note       string `json:"note"`
		Starred    *bool  `json:"starred"`
		ExternalID string `json:"external_id"`
		// from this app's own translator (a live translation kept once it settles)
		Engine       string `json:"engine"`
		Provider     string `json:"provider"`
		DetectedLang string `json:"detected_lang"`
		Kind         string `json:"kind"`
		Tone         string `json:"tone"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.SourceText = strings.TrimSpace(in.SourceText)
	if in.SourceText == "" || len([]rune(in.SourceText)) > maxText {
		writeErr(w, http.StatusBadRequest, "write the phrase (at most 20,000 characters)")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	to := engine.Normalize(firstNonEmpty(in.TargetLang, st.TargetLang))
	from := engine.Normalize(firstNonEmpty(in.SourceLang, "auto"))
	if to == "" || to == "auto" || from == "" {
		writeErr(w, http.StatusBadRequest, "unknown language")
		return
	}
	t := &store.Translation{Kind: "text", Source: linkApp(ctx), SourceLang: from, TargetLang: to, SourceText: in.SourceText, ResultText: strings.TrimSpace(in.ResultText),
		Starred: in.Starred == nil || *in.Starred, Collection: clip(strings.TrimSpace(in.Collection), 80), Note: clip(in.Note, 2000), ExternalID: clip(in.ExternalID, 200), Engine: "manual"}
	if t.Source != "" {
		t.Kind = "app"
	} else if t.ResultText != "" && in.Engine != "" {
		t.Engine, t.Model, t.Tone = clip(in.Engine, 20), clip(in.Provider, 20), clip(in.Tone, 20)
		t.DetectedLang = engine.Normalize(in.DetectedLang)
		if oneOf(in.Kind, []string{"text", "photo", "speech"}) {
			t.Kind = in.Kind
		}
	}
	if !t.Starred && !st.KeepHistory {
		writeJSON(w, http.StatusOK, map[string]any{"kept": false})
		return
	}
	if t.ResultText == "" {
		res, err := s.translate(ctx, u.ID, st, trReq{Texts: []string{t.SourceText}, From: from, To: to, Glossary: true, Tone: st.Tone})
		if err != nil {
			s.writeTrErr(w, err)
			return
		}
		t.ResultText, t.DetectedLang, t.Engine, t.Model = res.Texts[0], res.Detected, res.Engine, res.Provider
	}
	dup, err := s.store.AddTranslation(ctx, u.ID, t)
	if err != nil {
		s.fail(w, err, "add phrase")
		return
	}
	if !dup && t.Starred {
		s.publish(ctx, u.ID, "translate.phrase_saved", "Saved: "+clip(t.SourceText, 60), map[string]any{"translation_id": t.ID, "source_text": t.SourceText, "result_text": t.ResultText, "to": t.TargetLang, "collection": t.Collection})
	}
	status := http.StatusCreated
	if dup {
		status = http.StatusOK
	}
	writeJSON(w, status, t)
}

func (s *Server) handleUpdateTranslation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, ok := s.ownTranslation(w, r, true)
	if !ok {
		return
	}
	var in struct {
		ResultText *string `json:"result_text"`
		Starred    *bool   `json:"starred"`
		Collection *string `json:"collection"`
		Note       *string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	wasStarred := t.Starred
	if in.ResultText != nil {
		t.ResultText = strings.TrimSpace(*in.ResultText)
	}
	if in.Starred != nil {
		t.Starred = *in.Starred
	}
	if in.Collection != nil {
		t.Collection = clip(strings.TrimSpace(*in.Collection), 80)
	}
	if in.Note != nil {
		t.Note = clip(*in.Note, 2000)
	}
	if err := s.store.UpdateTranslation(ctx, actorID(ctx), t); err != nil {
		s.fail(w, err, "update translation")
		return
	}
	if t.Starred && !wasStarred {
		s.publish(ctx, actorID(ctx), "translate.phrase_saved", "Saved: "+clip(t.SourceText, 60), map[string]any{"translation_id": t.ID, "source_text": t.SourceText, "result_text": t.ResultText, "to": t.TargetLang, "collection": t.Collection})
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTranslation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, ok := s.ownTranslation(w, r, true)
	if !ok {
		return
	}
	if err := s.store.DeleteTranslation(ctx, actorID(ctx), t.ID); err != nil {
		s.fail(w, err, "delete translation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleClearHistory removes the caller's history (the phrasebook stays
// unless all=true).
func (s *Server) handleClearHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, err := s.store.ClearHistory(ctx, actorID(ctx), queryBool(r, "all"))
	if err != nil {
		s.fail(w, err, "clear history")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": n})
}

func (s *Server) handleCollections(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.Collections(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "collections")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"collections": list})
}

func (s *Server) handleRenameCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	n, err := s.store.RenameCollection(ctx, actorID(ctx), in.From, clip(in.To, 80))
	if err != nil {
		s.fail(w, err, "rename collection")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"moved": n})
}

// ---- to the other apps ----

// handleToLearn makes a flashcard in Pocket Learn from a phrase.
func (s *Server) handleToLearn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	t, ok := s.ownTranslation(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Deck    string `json:"deck"`
		Reverse bool   `json:"reverse"` // a card each way
	}
	_ = decode(r, &in)
	st, _ := s.store.GetSettings(ctx, u.ID)
	sib, err := s.clients(ctx, u.ID).conn(ctx, appLearn, scopeLearnWrite)
	if err != nil {
		s.appErr(w, appLearn, err)
		return
	}
	deck := firstNonEmpty(strings.TrimSpace(in.Deck), st.LearnDeck, "Phrasebook")
	kind := "basic"
	if in.Reverse {
		kind = "reverse"
	}
	extra := engine.Name(t.From()) + " → " + engine.Name(t.TargetLang)
	if t.Note != "" {
		extra += "\n" + t.Note
	}
	tags := "translate"
	if t.Collection != "" {
		tags += " " + strings.ReplaceAll(strings.ToLower(t.Collection), " ", "-")
	}
	body := map[string]any{"deck": deck, "kind": kind, "front": t.SourceText, "back": t.ResultText, "extra": extra, "tags": tags,
		"external_id": fmt.Sprintf("translation-%d", t.ID), "source_url": s.cfg.BaseURL + "/phrasebook"}
	res, err := postJSONTo(ctx, sib, "/api/v1/notes", body)
	if err != nil {
		s.appErr(w, appLearn, err)
		return
	}
	link := strings.TrimRight(sib.public, "/")
	if n := obj(res, "note"); n != nil {
		if id := idOf(n); id != "" {
			t.LearnRef = id
		}
	}
	if t.LearnRef == "" {
		t.LearnRef = "added"
	}
	t.Starred = true
	_ = s.store.UpdateTranslation(ctx, u.ID, t)
	writeJSON(w, http.StatusOK, map[string]any{"translation": t, "deck": deck, "url": link, "duplicate": res["duplicate"] == true})
}

// handleToNotes saves a translation as a note in Pocket Notes.
func (s *Server) handleToNotes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	t, ok := s.ownTranslation(w, r, false)
	if !ok {
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	title := clip(firstLine(t.SourceText), 80) + " (" + engine.Name(t.TargetLang) + ")"
	content := fmt.Sprintf("**%s**\n\n%s\n\n**%s**\n\n%s", engine.Name(t.From()), t.SourceText, engine.Name(t.TargetLang), t.ResultText)
	if t.Note != "" {
		content += "\n\n" + t.Note
	}
	content += "\n\n— Pocket Translate, " + time.Now().Format("2 Jan 2006")
	link, err := s.noteTo(r, u, st, title, content)
	if err != nil {
		s.appErr(w, appNotes, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": link})
}

// noteTo writes a note in Pocket Notes (the person's folder) and answers its link.
func (s *Server) noteTo(r *http.Request, u *store.User, st store.Settings, title, content string) (string, error) {
	ctx := r.Context()
	sib, err := s.clients(ctx, u.ID).conn(ctx, appNotes, scopeNotesWrite)
	if err != nil {
		return "", err
	}
	body := map[string]any{"title": title, "content": content}
	if st.NotesFolder != "" {
		body["category"] = st.NotesFolder
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/notes", body)
	if err != nil {
		return "", err
	}
	link := strings.TrimRight(sib.public, "/")
	if id := idOf(res); id != "" {
		link += "/note/" + id
	}
	return link, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// ---- overviews for the rest of the box (Search, Pocket Day, AI) ----

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	now := time.Now()
	ws := now.AddDate(0, 0, -((int(now.Weekday()) - st.WeekStart%7 + 7) % 7))
	stats, err := s.store.StatsFor(ctx, actorID(ctx), now.Format("2006-01-02"), ws.Format("2006-01-02"))
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}})
		return
	}
	list, _, err := s.store.ListTranslations(ctx, actorID(ctx), store.TranslationFilter{Q: q, Limit: 25})
	if err != nil {
		s.fail(w, err, "search")
		return
	}
	out := []map[string]any{}
	for _, t := range list {
		where := "/history"
		if t.Starred {
			where = "/phrasebook"
		}
		out = append(out, map[string]any{"id": t.ID, "title": clip(firstLine(t.SourceText), 100), "snippet": clip(t.ResultText, 200),
			"from": t.From(), "to": t.TargetLang, "starred": t.Starred, "at": t.CreatedAt, "url": s.cfg.BaseURL + where + "?q=" + urlQuery(q)})
	}
	terms, _ := s.store.ListTerms(ctx, actorID(ctx), "", "", q)
	for _, t := range terms {
		out = append(out, map[string]any{"id": t.ID, "title": t.Term + " → " + t.Translation, "snippet": "Glossary: " + engine.Name(t.SourceLang) + " → " + engine.Name(t.TargetLang),
			"kind": "glossary", "url": s.cfg.BaseURL + "/glossary"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

// handleDay is what was translated on a day (Pocket Day).
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day := chiParam(r, "date")
	if !validDate(day) {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	list, total, err := s.store.ListTranslations(ctx, actorID(ctx), store.TranslationFilter{Day: day, Limit: 50})
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	items := []map[string]any{}
	pairs := map[string]int{}
	for _, t := range list {
		items = append(items, map[string]any{"id": t.ID, "text": clip(t.SourceText, 160), "translation": clip(t.ResultText, 160), "from": t.From(), "to": t.TargetLang, "kind": t.Kind, "starred": t.Starred, "at": t.CreatedAt})
		pairs[engine.Name(t.From())+" → "+engine.Name(t.TargetLang)]++
	}
	summary := ""
	if total > 0 {
		summary = fmt.Sprintf("%d %s", total, plural(total, "translation", "translations"))
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": day, "count": total, "summary": summary, "pairs": pairs, "items": items, "url": s.cfg.BaseURL + "/history"})
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func urlQuery(s string) string {
	r := strings.NewReplacer(" ", "+", "&", "%26", "#", "%23", "?", "%3F")
	return r.Replace(s)
}
