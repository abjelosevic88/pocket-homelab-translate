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
// read the rows the Phrasebook and History pages list, as the request's
// person. Nothing is translated, detected or changed: no engine and no
// Pocket AI is asked.

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

// pocketSearch answers Pocket Search with what the Phrasebook and History
// search boxes find (GET /translations?q=: the text, its translation, the
// note and the collection), saved phrases first, then the rest of the
// history, each newest first.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	if !hasWord(q) {
		return nil, nil
	}
	ctx := r.Context()
	list, _, err := s.store.ListTranslations(ctx, actorID(ctx), store.TranslationFilter{Q: q, PhrasesFirst: true, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]core.SearchItem, 0, len(list))
	for _, t := range list {
		kind := "translation"
		if t.Starred {
			kind = "phrase"
		}
		out = append(out, core.SearchItem{ID: strconv.FormatInt(t.ID, 10), Title: clip(oneLine(t.SourceText), 100), Kind: kind, Date: t.CreatedAt,
			Snippet: joinNonEmpty(" · ", languagePair(t), clip(oneLine(t.ResultText), 160)), URL: translationPath(t)})
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
