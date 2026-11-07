package server

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-translate/internal/engine"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// mergeWithin is how long a history row stays open to the same text being
// typed further (one row per sentence, not one per pause).
const mergeWithin = 5 * time.Minute

// maxText caps one text to translate.
const maxText = 20000

type translateIn struct {
	Text         string   `json:"text"`
	Texts        []string `json:"texts"` // several at once (other apps); not kept in history
	From         string   `json:"from"`
	To           string   `json:"to"`
	Engine       string   `json:"engine"`
	Tone         *string  `json:"tone"`
	Alternatives bool     `json:"alternatives"`
	Glossary     *bool    `json:"glossary"`
	Kind         string   `json:"kind"` // text | photo | speech
	About        string   `json:"about"`
	Save         *bool    `json:"save"` // keep it in history (default: the person's setting)
}

// writeTrErr answers a failed translation in words a person can act on.
func (s *Server) writeTrErr(w http.ResponseWriter, err error) {
	var ne *errNoEngine
	switch {
	case errors.As(err, &ne):
		writeErr(w, http.StatusServiceUnavailable, ne.msg)
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		writeErr(w, http.StatusGatewayTimeout, "the translation took too long; try a shorter text")
	default:
		s.log.Warn("translate", "err", err)
		writeErr(w, http.StatusBadGateway, err.Error())
	}
}

func (s *Server) handleTranslate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in translateIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	batch := in.Texts != nil
	texts := in.Texts
	if !batch {
		texts = []string{in.Text}
	}
	total := 0
	for _, t := range texts {
		total += len([]rune(t))
	}
	if total > maxText*5 || (!batch && total > maxText) {
		writeErr(w, http.StatusRequestEntityTooLarge, "that is too much text at once; translate it as a document (Documents)")
		return
	}
	if len(texts) > 200 {
		writeErr(w, http.StatusRequestEntityTooLarge, "at most 200 texts at once")
		return
	}
	if in.Engine != "" && !oneOf(in.Engine, []string{"auto", "ai", "offline"}) {
		writeErr(w, http.StatusBadRequest, "the engine is auto, ai or offline")
		return
	}
	tone := st.Tone
	if in.Tone != nil {
		tone = *in.Tone
	}
	if !oneOf(tone, []string{"", "formal", "casual"}) {
		writeErr(w, http.StatusBadRequest, "the tone is formal, casual or none")
		return
	}
	to := firstNonEmpty(in.To, st.TargetLang)
	from := firstNonEmpty(in.From, "auto")
	req := trReq{Texts: texts, From: from, To: to, Engine: in.Engine, Tone: tone, Alternatives: in.Alternatives && !batch,
		Glossary: in.Glossary == nil || *in.Glossary, About: in.About}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	res, err := s.translate(cctx, u.ID, st, req)
	if err != nil {
		s.writeTrErr(w, err)
		return
	}
	out := map[string]any{"detected": res.Detected, "engine": res.Engine, "provider": res.Provider, "alternatives": res.Alternatives,
		"terms": res.Terms, "note": res.Note, "to": engine.Normalize(to)}
	if batch {
		out["texts"] = res.Texts
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["text"] = res.Texts[0]
	app := linkApp(ctx)
	save := st.KeepHistory
	if app != "" {
		save = st.KeepAppHistory
	}
	if in.Save != nil {
		save = *in.Save && (app == "" || st.KeepAppHistory)
	}
	if save && strings.TrimSpace(in.Text) != "" {
		kind := "text"
		if oneOf(in.Kind, []string{"photo", "speech"}) {
			kind = in.Kind
		}
		if app != "" {
			kind = "app"
		}
		t := &store.Translation{Kind: kind, Source: app, SourceLang: engine.Normalize(from), DetectedLang: res.Detected, TargetLang: engine.Normalize(to),
			SourceText: in.Text, ResultText: res.Texts[0], Engine: res.Engine, Model: res.Provider, Tone: tone}
		if t.SourceLang == "" {
			t.SourceLang = "auto"
		}
		merged := false
		if app == "" {
			merged, _ = s.store.MergeRecent(ctx, u.ID, t, mergeWithin)
		}
		if merged {
			out["translation"] = t
		} else if _, err := s.store.AddTranslation(ctx, u.ID, t); err == nil {
			out["translation"] = t
		}
	}
	if app == "" {
		s.rememberLangs(ctx, u.ID, st, res.Detected, engine.Normalize(to))
	}
	writeJSON(w, http.StatusOK, out)
}

// rememberLangs keeps the languages a person uses at the front of their list.
func (s *Server) rememberLangs(ctx context.Context, uid int64, st store.Settings, langs ...string) {
	out := []string{}
	for _, l := range langs {
		if l != "" && l != "auto" && !oneOf(l, out) {
			out = append(out, l)
		}
	}
	for _, l := range st.RecentLangs {
		if !oneOf(l, out) && len(out) < 8 {
			out = append(out, l)
		}
	}
	if strings.Join(out, ",") == strings.Join(st.RecentLangs, ",") {
		return
	}
	st.RecentLangs = out
	_ = s.store.SaveUserSettings(ctx, uid, st)
}

func (s *Server) handleDetect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Text) == "" {
		writeErr(w, http.StatusBadRequest, "send the text")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	code, by, err := s.detect(ctx, u.ID, st, in.Text)
	if err != nil {
		s.writeTrErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"language": code, "name": engine.Name(code), "by": by})
}

func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		Text string `json:"text"`
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Text) == "" {
		writeErr(w, http.StatusBadRequest, "send the phrase to explain")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	to := engine.Normalize(firstNonEmpty(in.To, st.TargetLang))
	if to == "" || to == "auto" {
		to = "en"
	}
	ex, err := s.explain(ctx, u.ID, st, in.Text, engine.Normalize(in.From), to)
	if err != nil {
		s.writeTrErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ex)
}

// handleLanguages lists every language with what can be done in it here:
// which offline engine knows it, whether photos can be read in it.
func (s *Server) handleLanguages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ocr := s.ocrLangs(ctx)
	type langOut struct {
		engine.Lang
		Argos bool `json:"argos"`
		NLLB  bool `json:"nllb"`
		Read  bool `json:"read"` // photos can be read in it
	}
	out := make([]langOut, 0, len(engine.Langs))
	for _, l := range engine.Langs {
		a, n := s.offlineKnows(ctx, l.Code)
		out = append(out, langOut{Lang: l, Argos: a, NLLB: n, Read: l.OCR != "" && ocrHas(ocr, l.OCR) != ""})
	}
	writeJSON(w, http.StatusOK, map[string]any{"languages": out})
}

// handleEngines says which engines work for this person, and if not why.
func (s *Server) handleEngines(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	st, _ := s.store.GetSettings(ctx, u.ID)
	argos, nllb := s.engines(ctx)
	cl, provider, why := s.aiFor(ctx, u.ID, st)
	eng := func(l *engine.Libre) map[string]any {
		ok, detail := l.Health(ctx)
		n := 0
		if p, err := l.Pairs(ctx); err == nil {
			n = len(p)
		}
		return map[string]any{"configured": l.Configured(), "ready": ok, "detail": detail, "languages": n}
	}
	ocr := s.ocrLangs(ctx)
	_, pdfErr := exec.LookPath(s.cfg.PDFToText)
	speechOK, speechWhy := s.speechReady(ctx, u, st)
	writeJSON(w, http.StatusOK, map[string]any{
		"ai":     map[string]any{"ready": cl != nil, "provider": provider, "detail": why},
		"argos":  eng(argos),
		"nllb":   eng(nllb),
		"ocr":    map[string]any{"ready": len(ocr) > 0, "languages": ocrList(ocr)},
		"pdf":    map[string]any{"ready": pdfErr == nil},
		"speech": map[string]any{"ready": speechOK, "detail": speechWhy, "via": st.Speech},
	})
}

func keysOf(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ocrList is the installed photo languages, with the ones read through a
// stand-in (Serbian Latin through Croatian) counted as there.
func ocrList(have map[string]bool) []string {
	out := keysOf(have)
	for c := range ocrStandIn {
		if !have[c] && ocrHas(have, c) != "" {
			out = append(out, c)
		}
	}
	return out
}
