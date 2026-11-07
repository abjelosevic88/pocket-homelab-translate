package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-translate/internal/engine"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// ---- conversations: two people, two languages, turn by turn ----

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListConversations(ctx, actorID(ctx), queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "conversations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": list})
}

type convIn struct {
	Title *string `json:"title"`
	LangA string  `json:"lang_a"`
	LangB string  `json:"lang_b"`
}

func (s *Server) handleAddConversation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in convIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	a, b := engine.Normalize(in.LangA), engine.Normalize(in.LangB)
	if a == "" || a == "auto" {
		a = firstNonEmpty(st.TargetLang, "en")
	}
	if b == "" || b == "auto" || b == a {
		writeErr(w, http.StatusBadRequest, "pick the other person's language")
		return
	}
	c := &store.Conversation{LangA: a, LangB: b}
	if in.Title != nil {
		c.Title = clip(strings.TrimSpace(*in.Title), 120)
	}
	if c.Title == "" {
		c.Title = engine.Name(a) + " ↔ " + engine.Name(b) + ", " + time.Now().Format("2 Jan 15:04")
	}
	if err := s.store.AddConversation(ctx, u.ID, c); err != nil {
		s.fail(w, err, "add conversation")
		return
	}
	s.rememberLangs(ctx, u.ID, st, a, b)
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := s.store.GetConversation(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "conversation")
		return
	}
	turns, err := s.store.Turns(ctx, actorID(ctx), c.ID)
	if err != nil {
		s.fail(w, err, "conversation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation": c, "turns": turns})
}

func (s *Server) handleUpdateConversation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := s.store.GetConversation(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "conversation")
		return
	}
	var in convIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Title != nil {
		c.Title = clip(strings.TrimSpace(*in.Title), 120)
	}
	if v := engine.Normalize(in.LangA); v != "" && v != "auto" {
		c.LangA = v
	}
	if v := engine.Normalize(in.LangB); v != "" && v != "auto" {
		c.LangB = v
	}
	if c.LangA == c.LangB {
		writeErr(w, http.StatusBadRequest, "the two sides need different languages")
		return
	}
	if err := s.store.UpdateConversation(ctx, actorID(ctx), c); err != nil {
		s.fail(w, err, "update conversation")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteConversation(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete conversation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAddTurn takes what one side said — a recording (multipart "file")
// or typed text — writes it out, translates it for the other side and
// keeps both.
func (s *Server) handleAddTurn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	c, err := s.store.GetConversation(ctx, u.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "conversation")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	side, text, spoken := "", "", false
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		data, name, _, ok := s.formFile(w, r, 25<<20)
		if !ok {
			return
		}
		side = r.FormValue("side")
		lang := c.LangA
		if side == "b" {
			lang = c.LangB
		}
		text, _, err = s.transcribe(ctx, u, st, data, name, lang)
		if err != nil {
			s.writeTrErr(w, err)
			return
		}
		if text == "" {
			writeErr(w, http.StatusUnprocessableEntity, "nothing was heard; try again closer to the microphone")
			return
		}
		spoken = true
	} else {
		var in struct {
			Side string `json:"side"`
			Text string `json:"text"`
		}
		if err := decode(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		side, text = in.Side, strings.TrimSpace(in.Text)
	}
	if side != "a" && side != "b" {
		writeErr(w, http.StatusBadRequest, "the side is a (you) or b (the other person)")
		return
	}
	if text == "" || len([]rune(text)) > 4000 {
		writeErr(w, http.StatusBadRequest, "say or write something (at most 4,000 characters)")
		return
	}
	from, to := c.LangA, c.LangB
	if side == "b" {
		from, to = c.LangB, c.LangA
	}
	t := &store.Turn{ConversationID: c.ID, Side: side, Spoken: spoken, SourceLang: from, TargetLang: to, SourceText: engine.Script(from, text)}
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	res, err := s.translate(cctx, u.ID, st, trReq{Texts: []string{t.SourceText}, From: from, To: to, Tone: st.Tone, Glossary: true, About: "something said aloud in a conversation"})
	if err != nil {
		// keep what was said, so it is not lost; the translation can be retried
		t.Engine = ""
		_ = s.store.AddTurn(ctx, u.ID, t)
		var ne *errNoEngine
		msg := err.Error()
		if ok := asNoEngine(err, &ne); ok {
			msg = ne.msg
		}
		writeJSON(w, http.StatusOK, map[string]any{"turn": t, "error": msg})
		return
	}
	t.ResultText, t.Engine = res.Texts[0], res.Engine
	if err := s.store.AddTurn(ctx, u.ID, t); err != nil {
		s.fail(w, err, "add turn")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"turn": t, "note": res.Note})
}

// handleRetryTurn translates a turn again (it failed, or another engine).
func (s *Server) handleRetryTurn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	t, err := s.store.GetTurn(ctx, u.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "turn")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	res, err := s.translate(ctx, u.ID, st, trReq{Texts: []string{t.SourceText}, From: t.SourceLang, To: t.TargetLang, Tone: st.Tone, Glossary: true})
	if err != nil {
		s.writeTrErr(w, err)
		return
	}
	t.ResultText, t.Engine = res.Texts[0], res.Engine
	if _, err := s.store.DB.ExecContext(ctx, `UPDATE turns SET result_text = ?, engine = ? WHERE id = ? AND user_id = ?`, t.ResultText, t.Engine, t.ID, u.ID); err != nil {
		s.fail(w, err, "turn")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"turn": t})
}

func (s *Server) handleDeleteTurn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteTurn(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete turn")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) transcript(ctx context.Context, uid int64, c *store.Conversation) string {
	turns, _ := s.store.Turns(ctx, uid, c.ID)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s ↔ %s\n\n", c.Title, engine.Name(c.LangA), engine.Name(c.LangB))
	for _, t := range turns {
		who := "Me"
		if t.Side == "b" {
			who = "Them"
		}
		at := t.CreatedAt
		if tm, err := time.Parse(time.RFC3339Nano, t.CreatedAt); err == nil {
			at = tm.Local().Format("15:04")
		}
		fmt.Fprintf(&b, "%s, %s: %s\n", who, at, t.SourceText)
		if t.ResultText != "" {
			fmt.Fprintf(&b, "    → %s\n", t.ResultText)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (s *Server) handleTranscript(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := s.store.GetConversation(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "conversation")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="conversation.txt"`)
	_, _ = w.Write([]byte(s.transcript(ctx, actorID(ctx), c)))
}

// handleConversationToNotes saves the whole conversation as a note.
func (s *Server) handleConversationToNotes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	c, err := s.store.GetConversation(ctx, u.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "conversation")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	text := s.transcript(ctx, u.ID, c)
	_, body, _ := strings.Cut(text, "\n\n")
	link, err := s.noteTo(r, u, st, c.Title, body+"\n— Pocket Translate")
	if err != nil {
		s.appErr(w, appNotes, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": link})
}

func asNoEngine(err error, ne **errNoEngine) bool { return errors.As(err, ne) }
