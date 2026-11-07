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
