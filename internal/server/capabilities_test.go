package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/config"
)

// capLinks starts Translate behind a fake Core that vouches for link tokens
// pcx_<alice|bob>_<search|day|widget|capture|read|write|run> from Pocket Hub
// and remembers the manifest Translate registers with.
func capLinks(t *testing.T) (*app, func() map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var manifest map[string]any
	scopes := map[string]string{"search": core.ScopeSearch, "day": core.ScopeDay, "widget": core.ScopeWidget, "capture": core.ScopeCapture,
		"read": "phrasebook:read", "write": "phrasebook:write", "run": "translate:run"}
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/register":
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			mu.Lock()
			manifest = m
			mu.Unlock()
			io.WriteString(w, `{}`)
		case "/api/v1/auth/introspect":
			var in struct {
				Token string `json:"token"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			parts := strings.Split(in.Token, "_")
			if len(parts) != 3 || parts[0] != "pcx" || scopes[parts[2]] == "" {
				io.WriteString(w, `{"active":false}`)
				return
			}
			email := parts[1] + "@example.com"
			fmt.Fprintf(w, `{"active":true,"kind":"delegated","scope":%q,"audience":"translate","user":{"id":1,"sub":%q,"email":%q,"name":%q,"role":"member"},"app":{"id":9,"slug":"hub","name":"Pocket Hub"}}`, scopes[parts[2]], email, email, parts[1])
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(coreSrv.Close)
	a := newApp(t, func(c *config.Config) { c.CoreURL, c.CoreRegistrationKey = coreSrv.URL, "key" })
	return a, func() map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return manifest
	}
}

// phrase saves a translation as the person's own translator does, with its
// translation already there (nothing is translated); starred = in the phrasebook.
func (a *app) phrase(auth, text, result, from, to string, starred bool) string {
	a.t.Helper()
	return id(a.ok(auth, "POST", "/api/v1/translations", map[string]any{"source_text": text, "result_text": result, "source_lang": from, "target_lang": to, "starred": starred}))
}

// capItems are the rows of a capability answer.
func capItems(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	l, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("no items list in %v", m)
	}
	out := []map[string]any{}
	for _, v := range l {
		out = append(out, v.(map[string]any))
	}
	return out
}

// rows counts every translation kept, to show that reading added nothing.
func (a *app) rows() int {
	a.t.Helper()
	var n int
	if err := a.st.DB.QueryRow(`SELECT COUNT(1) FROM translations`).Scan(&n); err != nil {
		a.t.Fatal(err)
	}
	return n
}

func TestPocketSearch(t *testing.T) {
	a := newApp(t, nil)
	evening := a.phrase(a.alice, "Good evening", "Guten Abend", "en", "de", true)
	seen := a.phrase(a.alice, "Have you seen the evening news?\nIt was long.", "Hast du die Abendnachrichten gesehen?", "en", "de", false)
	a.phrase(a.alice, "Where is the station?", "Gdje je stanica?", "auto", "hr", true)
	a.phrase(a.bob, "Good evening, Bob", "Guten Abend, Bob", "en", "de", true)
	before := a.rows()

	got := capItems(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=evening", nil))
	if len(got) != 2 {
		t.Fatalf("evening: %v", got)
	}
	// the phrasebook first, then the history, each with where it opens
	if p := got[0]; p["id"] != evening || p["kind"] != "phrase" || p["title"] != "Good evening" || p["snippet"] != "English → German · Guten Abend" ||
		p["url"] != "/phrasebook?q=Good+evening" || p["date"] == "" {
		t.Fatalf("phrase: %v", p)
	}
	if h := got[1]; h["id"] != seen || h["kind"] != "translation" || h["title"] != "Have you seen the evening news? It was long." ||
		h["snippet"] != "English → German · Hast du die Abendnachrichten gesehen?" || h["url"] != "/history?q="+url.QueryEscape("Have you seen the evening news?") {
		t.Fatalf("history: %v", h)
	}
	// the translation is searched too; a language never detected is left out of the line
	if got := capItems(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=stanica", nil)); len(got) != 1 || got[0]["snippet"] != "Croatian · Gdje je stanica?" {
		t.Fatalf("by translation: %v", got)
	}
	if got := capItems(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=evening&limit=1", nil)); len(got) != 1 || got[0]["id"] != evening {
		t.Fatalf("limit 1: %v", got)
	}
	// the URL finds the row on its page, as the page's own search does
	if l := a.ok(a.alice, "GET", "/api/v1/translations?starred=1&q=Good+evening", nil)["translations"].([]any); len(l) != 1 || id(l[0].(map[string]any)) != evening {
		t.Fatalf("the phrasebook's search: %v", l)
	}
	// no words: nothing, not every phrase
	for _, q := range []string{"", "%3F%3F", "*", "%25", "_"} {
		if got := capItems(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q="+q, nil)); len(got) != 0 {
			t.Fatalf("q=%q: %v", q, got)
		}
	}
	// another person's phrases never appear
	if got := capItems(t, a.ok(a.bob, "GET", "/api/v1/pocket/search?q=evening", nil)); len(got) != 1 || got[0]["title"] != "Good evening, Bob" {
		t.Fatalf("bob: %v", got)
	}
	if got := capItems(t, a.ok(a.bob, "GET", "/api/v1/pocket/search?q=station", nil)); len(got) != 0 {
		t.Fatalf("bob finds alice's: %v", got)
	}
	a.expect(http.StatusUnauthorized, "", "GET", "/api/v1/pocket/search?q=evening", nil)
	if a.rows() != before {
		t.Fatal("searching kept a translation")
	}
}

func TestPocketWidget(t *testing.T) {
	a := newApp(t, nil)
	// nothing saved: nothing to show, even with history
	a.phrase(a.alice, "red", "rot", "en", "de", false)
	if w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || len(capItems(t, w)) != 0 {
		t.Fatalf("empty phrasebook: %v", w)
	}
	first := a.phrase(a.alice, "Good morning", "Guten Morgen", "en", "de", true)
	w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	got := capItems(t, w)
	if w["line"] != "1 phrase" || w["url"] != "/phrasebook" || len(got) != 1 {
		t.Fatalf("one phrase: %v", w)
	}
	if it := got[0]; it["id"] != first || it["title"] != "Good morning" || it["detail"] != "Guten Morgen" || it["url"] != "/phrasebook?q=Good+morning" {
		t.Fatalf("row: %v", it)
	}
	// the newest few, whatever their collection; the line counts them all
	for i := range 6 {
		a.ok(a.alice, "POST", "/api/v1/translations", map[string]any{"source_text": fmt.Sprintf("Phrase %d", i), "result_text": fmt.Sprintf("Satz %d", i),
			"target_lang": "de", "collection": []string{"Work", "A travel"}[i%2]})
	}
	w = a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	if got := capItems(t, w); w["line"] != "7 phrases" || len(got) != widgetRows || got[0]["title"] != "Phrase 5" || got[4]["title"] != "Phrase 1" {
		t.Fatalf("seven phrases: %v", w)
	}
	// another person's phrasebook never shows
	if w := a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || len(capItems(t, w)) != 0 {
		t.Fatalf("bob: %v", w)
	}
	a.expect(http.StatusUnauthorized, "", "GET", "/api/v1/pocket/widget", nil)
}

// A link token with one pocket:* scope reaches its one route and nothing
// else; the app's own scopes do not open the new routes.
func TestPocketLinkTokens(t *testing.T) {
	a, _ := capLinks(t)
	evening := a.phrase(a.alice, "Good evening", "Guten Abend", "en", "de", true)
	line := map[string]any{"request_id": "req-0000000000000001", "kind": "phrase", "text": "Good night"}
	for _, c := range []struct {
		tok, method, path string
		body              any
		want              int
	}{
		{"pcx_alice_search", "GET", "/api/v1/pocket/search?q=evening", nil, 200},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/widget", nil, 200},
		{"pcx_alice_search", "GET", "/api/v1/auth/me", nil, 200},
		{"pcx_alice_widget", "GET", "/api/v1/auth/me", nil, 200},
		// another capability's route
		{"pcx_alice_search", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/search?q=evening", nil, 403},
		{"pcx_alice_search", "POST", "/api/v1/pocket/search?q=evening", nil, 405},
		// Translate has no day and keeps no quick line: no such routes
		{"pcx_alice_day", "GET", "/api/v1/pocket/day/2026-11-27", nil, 404},
		{"pcx_alice_capture", "POST", "/api/v1/pocket/capture", line, 404},
		{a.alice, "GET", "/api/v1/pocket/day/2026-11-27", nil, 404},
		{a.alice, "POST", "/api/v1/pocket/capture", line, 404},
		{"pcx_alice_day", "GET", "/api/v1/pocket/search?q=evening", nil, 403},
		{"pcx_alice_capture", "GET", "/api/v1/pocket/widget", nil, 403},
		// the rest of the app: no reading, no change, no translating
		{"pcx_alice_search", "GET", "/api/v1/translations?q=evening", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/translations/" + evening, nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/search?q=evening", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/summary", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/glossary", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/settings", nil, 403},
		{"pcx_alice_search", "POST", "/api/v1/translate", map[string]any{"text": "hello", "to": "de"}, 403},
		{"pcx_alice_widget", "POST", "/api/v1/translations", map[string]any{"source_text": "hello", "result_text": "hallo"}, 403},
		{"pcx_alice_search", "DELETE", "/api/v1/translations/" + evening, nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/export.json", nil, 403},
		// the app's own scopes do not open the new routes
		{"pcx_alice_read", "GET", "/api/v1/pocket/search?q=evening", nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_write", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_run", "GET", "/api/v1/pocket/search?q=evening", nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/translations?q=evening", nil, 200},
		{"pcx_nobody", "GET", "/api/v1/pocket/widget", nil, 401},
	} {
		if code, m, _ := a.call(c.tok, c.method, c.path, c.body); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%v)", c.tok, c.method, c.path, code, c.want, m)
		}
	}
	// the token acts for its person only
	if got := capItems(t, a.ok("pcx_alice_search", "GET", "/api/v1/pocket/search?q=evening", nil)); len(got) != 1 || got[0]["id"] != evening {
		t.Fatalf("alice through the link: %v", got)
	}
	if got := capItems(t, a.ok("pcx_bob_search", "GET", "/api/v1/pocket/search?q=evening", nil)); len(got) != 0 {
		t.Fatalf("bob through the link finds alice's: %v", got)
	}
	if w := a.ok("pcx_alice_widget", "GET", "/api/v1/pocket/widget", nil); w["line"] != "1 phrase" {
		t.Fatalf("alice's widget through the link: %v", w)
	}
	if w := a.ok("pcx_bob_widget", "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || len(capItems(t, w)) != 0 {
		t.Fatalf("bob's widget through the link: %v", w)
	}
}

func TestPocketLinkScopes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	for _, c := range []struct {
		scope string
		r     *http.Request
		want  bool
	}{
		{core.ScopeSearch, req("GET", "/api/v1/pocket/search?q=x"), true},
		{core.ScopeWidget, req("GET", "/api/v1/pocket/widget"), true},
		{core.ScopeWidget, req("GET", "/api/v1/auth/me"), true},
		{core.ScopeSearch, req("POST", "/api/v1/pocket/search"), false},
		{core.ScopeSearch, req("GET", "/api/v1/search"), false},
		{core.ScopeSearch, req("GET", "/api/v1/translations"), false},
		{core.ScopeWidget, req("GET", "/api/v1/summary"), false},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/2026-01-02"), false},
		{core.ScopeDay, req("GET", "/api/v1/day/2026-01-02"), false},
		{core.ScopeCapture, req("POST", "/api/v1/pocket/capture"), false},
		{core.ScopeCapture, req("POST", "/api/v1/translations"), false},
		{"phrasebook:read", req("GET", "/api/v1/pocket/search?q=x"), false},
		{"phrasebook:read", req("GET", "/api/v1/pocket/widget"), false},
		{"phrasebook:write", req("POST", "/api/v1/pocket/capture"), false},
		{"translate:run", req("GET", "/api/v1/pocket/search?q=x"), false},
	} {
		if got := linkAllows([]string{c.scope}, c.r); got != c.want {
			t.Errorf("%s %s %s = %v, want %v", c.scope, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}

// Translate tells Core what it can do, so Search and Hub find it on their own.
func TestManifestCapabilities(t *testing.T) {
	a, manifest := capLinks(t)
	if err := a.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := manifest()
	if m["slug"] != "translate" || m["category"] != "Tools" {
		t.Fatalf("manifest: %v %v", m["slug"], m["category"])
	}
	search, _ := m["search"].(map[string]any)
	widget, _ := m["widget"].(map[string]any)
	if search == nil || fmt.Sprint(search["bangs"]) != "[translate phrase]" || fmt.Sprint(search["kinds"]) != "[phrase translation]" ||
		widget == nil || widget["title"] != "Phrasebook" {
		t.Fatalf("capabilities: %v %v", search, widget)
	}
	if m["day"] != nil || m["capture"] != nil {
		t.Fatalf("no day, no quick line: %v %v", m["day"], m["capture"])
	}
	offered := map[string]bool{}
	for _, o := range m["offers"].([]any) {
		offered[o.(map[string]any)["scope"].(string)] = true
	}
	for _, sc := range []string{"translate:run", "phrasebook:read", "phrasebook:write", core.ScopeSearch, core.ScopeWidget} {
		if !offered[sc] {
			t.Errorf("%s is not offered", sc)
		}
	}
	if offered[core.ScopeDay] || offered[core.ScopeCapture] {
		t.Error("pocket:day or pocket:capture is offered")
	}
}
