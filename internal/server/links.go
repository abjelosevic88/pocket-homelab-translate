package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Translate
// as the target: another Pocket app the person allowed in Core may act here
// for them with a pcx_ token, only within these scopes. Every route not
// listed in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "translate:run", Name: "Translate for you", Description: "Translate text for you with your own engine choice, tone and glossary (a chat message, an email, a note, a blog post), tell which language a text is in, and explain a phrase. What other apps translate is kept in your history only if you allow it in Settings."},
	{Scope: "phrasebook:read", Name: "See your phrasebook", Description: "Read your phrasebook, your translation history and your glossary — for search, your journal, flashcards and Pocket AI."},
	{Scope: "phrasebook:write", Name: "Add to your phrasebook", Description: "Save a phrase and its translation to your phrasebook (\"save this sentence\" in Chat or Mail). It may change or remove only the phrases it added itself."},
}

// portable is what Pocket Move may copy out and bring in for a person
// (docs/move.md in pocket-homelab-core); Register adds move:export/import.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "translate.json", Kind: core.KindPocket, Title: "Your translations and phrasebook, glossary, conversations and the list of documents you translated"},
		{Path: "/api/v1/export.csv", File: "translations.csv", Kind: core.KindCSV, Title: "Your translations and phrasebook as a table"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, ContentType: "application/json", Title: "Adds the translations, glossary and conversations from another box; what is already here is skipped"},
		{Path: "/api/v1/import", Kind: core.KindCSV, ContentType: "text/csv", Title: "Adds Google Translate's saved phrases (Takeout \"Saved translations.csv\") or this app's own CSV to the phrasebook; phrases already here are skipped"},
	},
}

// capabilities are what Pocket Search and Pocket Hub may ask Translate
// (docs/capabilities.md in pocket-homelab-core); Register adds the pocket:*
// offers. The routes are in capabilities.go. Both only read what is kept
// here: nothing is translated. There is no day (Pocket Day already reads
// the history through phrasebook:read) and no quick line: a phrase added
// without its translation is translated on the spot, by Pocket AI or an
// engine, which is too slow for a line typed in Pocket Hub.
var capabilities = core.Capabilities{
	Search: &core.Searchable{What: "Your phrasebook, glossary and translation history, by the text and its translation", Kinds: []string{"phrase", "term", "translation"}, Bangs: []string{"translate", "phrase"}},
	Widget: &core.Widget{Title: "Phrasebook", What: "The phrases you saved last"},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/languages$`), ""},
	{"POST", regexp.MustCompile(`^/api/v1/(translate|detect|explain)$`), "translate:run"},
	{"GET", regexp.MustCompile(`^/api/v1/(translations|collections|glossary|summary|search|day/\d{4}-\d{2}-\d{2})$`), "phrasebook:read"},
	{"GET", regexp.MustCompile(`^/api/v1/translations/\d+$`), "phrasebook:read"},
	{"POST", regexp.MustCompile(`^/api/v1/translations$`), "phrasebook:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/translations/\d+$`), "phrasebook:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) || capabilities.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "translate" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
