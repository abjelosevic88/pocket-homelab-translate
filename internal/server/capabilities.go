package server

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/engine"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// What Pocket Search and Pocket Hub ask Translate (the capabilities value in
// links.go): one route each, in the shape every Pocket app answers in. They
// read the rows the Phrasebook, Glossary and History pages list, as the
// request's person. Nothing is translated, detected or changed: no engine
// and no Pocket AI is asked.

// widgetRows is how many saved phrases the widget names.
const widgetRows = 5

// oneLine is a text on one line: its runs of spaces and line breaks as one space.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// cutRunes keeps the first n characters of a line, without an ellipsis.
func cutRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n]))
	}
	return s
}

// hasWord reports whether a query holds a letter or a digit; "??" or "*"
// would match every row, which is no answer to a search.
func hasWord(q string) bool {
	return strings.IndexFunc(q, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// languagePair names the languages of a translation as the lists do:
// "English → German", or only the language it went into when the language
// it came from was never known.
func languagePair(t *store.Translation) string {
	if from := t.From(); from != "" && from != "auto" {
		return engine.Name(from) + " → " + engine.Name(t.TargetLang)
	}
	return engine.Name(t.TargetLang)
}

// translationPath opens a translation in the app: the Phrasebook or History
// page searching for how it starts, the only way those pages open one row.
func translationPath(t *store.Translation) string {
	page := "/history"
	if t.Starred {
		page = "/phrasebook"
	}
	if q := cutRunes(firstLine(t.SourceText), 60); q != "" {
		return page + "?q=" + url.QueryEscape(q)
	}
	return page
}

// ---------- search ----------

// termLanguages names a glossary term's languages as the Glossary page
// does, "*" being any language: "Any language → Serbian".
func termLanguages(t *store.Term) string {
	name := func(code string) string {
		if code == "*" {
			return "Any language"
		}
		return engine.Name(code)
	}
	return name(t.SourceLang) + " → " + name(t.TargetLang)
}

// pocketSearch answers Pocket Search with what the app's own searches find:
// the Phrasebook and History search boxes (GET /translations?q=: the text,
// its translation, the note and the collection) and the glossary's (GET
// /glossary?q=: the term, its translation and the note), the matches GET
// /search gives. Saved phrases come first, then glossary terms, then the
// rest of the history; phrases and history newest first, terms as the
// Glossary page lists them. The answer mixes two tables, so ids are
// "phrase:12", "term:4", "translation:13".
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	if !hasWord(q) {
		return nil, nil
	}
	ctx := r.Context()
	uid := actorID(ctx)
	list, _, err := s.store.ListTranslations(ctx, uid, store.TranslationFilter{Q: q, PhrasesFirst: true, Limit: limit})
	if err != nil {
		return nil, err
	}
	terms, err := s.store.ListTerms(ctx, uid, "", "", q)
	if err != nil {
		return nil, err
	}
	var phrases, history []core.SearchItem
	for _, t := range list {
		it := core.SearchItem{Title: clip(oneLine(t.SourceText), 100), Date: t.CreatedAt,
			Snippet: joinNonEmpty(" · ", languagePair(t), clip(oneLine(t.ResultText), 160)), URL: translationPath(t)}
		if t.Starred {
			it.ID, it.Kind = "phrase:"+strconv.FormatInt(t.ID, 10), "phrase"
			phrases = append(phrases, it)
		} else {
			it.ID, it.Kind = "translation:"+strconv.FormatInt(t.ID, 10), "translation"
			history = append(history, it)
		}
	}
	out := make([]core.SearchItem, 0, len(list)+len(terms))
	out = append(out, phrases...)
	for _, t := range terms {
		out = append(out, core.SearchItem{ID: "term:" + strconv.FormatInt(t.ID, 10), Title: clip(oneLine(t.Term), 100), Kind: "term", Date: t.CreatedAt,
			Snippet: joinNonEmpty(" · ", termLanguages(t), clip(oneLine(t.Translation), 100), clip(oneLine(t.Note), 100)), URL: "/glossary"})
	}
	out = append(out, history...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---------- widget ----------

// pocketWidget answers Pocket Hub: how many phrases the phrasebook holds and
// the newest few of them with their translation. An empty phrasebook has
// nothing to show.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx := r.Context()
	list, total, err := s.store.ListTranslations(ctx, actorID(ctx), store.TranslationFilter{Starred: true, PhrasesFirst: true, Limit: widgetRows})
	if err != nil || total == 0 {
		return nil, err
	}
	w := &core.WidgetData{URL: "/phrasebook", Line: strconv.Itoa(total) + " " + plural(total, "phrase", "phrases")}
	for _, t := range list {
		w.Items = append(w.Items, core.WidgetItem{ID: strconv.FormatInt(t.ID, 10), Title: clip(oneLine(t.SourceText), 80), Detail: clip(oneLine(t.ResultText), 80), URL: translationPath(t)})
	}
	return w, nil
}
