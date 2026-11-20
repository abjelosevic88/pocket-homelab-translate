package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/config"
)

// moveLinks starts Translate behind a fake Core whose pcx_<who>_<export|import|read>
// tokens are Pocket Move links acting for alice@ or bob@.
func moveLinks(t *testing.T) *app {
	t.Helper()
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/introspect" {
			io.WriteString(w, `{}`)
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.SplitN(strings.TrimPrefix(in.Token, "pcx_"), "_", 2)
		if len(parts) != 2 {
			io.WriteString(w, `{"active":false}`)
			return
		}
		scope := map[string]string{"export": core.ScopeMoveExport, "import": core.ScopeMoveImport, "read": "phrasebook:read"}[parts[1]]
		email := parts[0] + "@example.com"
		fmt.Fprintf(w, `{"active":true,"kind":"delegated","scope":%q,"audience":"translate","user":{"id":1,"sub":%q,"email":%q,"name":%q,"role":"member"},"app":{"id":7,"slug":"move","name":"Pocket Move"}}`, scope, email, email, parts[0])
	}))
	t.Cleanup(coreSrv.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL = coreSrv.URL })
}

func (a *app) mreq(tok, method, path, ctype string, body []byte) (int, []byte) {
	a.t.Helper()
	req, _ := http.NewRequest(method, a.url+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func (a *app) moveImport(tok, path, ctype string, data []byte) map[string]any {
	a.t.Helper()
	code, out := a.mreq(tok, "POST", path, ctype, data)
	if code != 200 {
		a.t.Fatalf("POST %s → %d %s", path, code, out)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	return m
}

func TestMoveLinkScopes(t *testing.T) {
	a := moveLinks(t)
	exp, imp, read := "pcx_bob_export", "pcx_bob_import", "pcx_bob_read"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{exp, "GET", core.PortablePath, 200},
		{exp, "GET", "/api/v1/export.json", 200},
		{exp, "GET", "/api/v1/export.csv", 200},
		{exp, "POST", "/api/v1/import", 403},
		{exp, "GET", "/api/v1/settings", 403},
		{exp, "GET", "/api/v1/translations", 403},
		{exp, "GET", "/api/v1/backup", 403},
		{imp, "GET", core.PortablePath, 200},
		{imp, "GET", "/api/v1/export.json", 403},
		{imp, "PUT", "/api/v1/settings", 403},
		{imp, "POST", "/api/v1/translations", 403},
		{imp, "POST", "/api/v1/glossary/import", 403},
		{read, "GET", core.PortablePath, 403},
		{read, "GET", "/api/v1/export.json", 403},
	} {
		if code, body := a.mreq(c.tok, c.method, c.path, "application/json", []byte("{}")); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	_, body := a.mreq(exp, "GET", core.PortablePath, "", nil)
	var p core.Portable
	if err := json.Unmarshal(body, &p); err != nil || len(p.Exports) != 2 || len(p.Imports) != 2 || p.Exports[0].Kind != core.KindPocket {
		t.Fatalf("portable: %s", body)
	}
}

// Move brings Alice's translations to Bob: each import adds once, her own
// file back adds nothing, and nothing of one person shows up for the other.
func TestMoveImportsAddOnce(t *testing.T) {
	a := moveLinks(t)
	a.ok(a.alice, "POST", "/api/v1/translations", map[string]any{"source_text": "good evening", "result_text": "guten Abend", "source_lang": "en", "target_lang": "de"})
	a.ok(a.alice, "POST", "/api/v1/glossary", map[string]any{"source_lang": "en", "target_lang": "de", "term": "car", "translation": "Wagen"})

	code, export := a.mreq("pcx_alice_export", "GET", "/api/v1/export.json", "", nil)
	if code != 200 || !strings.Contains(string(export), "good evening") {
		t.Fatalf("alice's export: %d %s", code, export)
	}
	if _, bob := a.mreq("pcx_bob_export", "GET", "/api/v1/export.json", "", nil); strings.Contains(string(bob), "good evening") {
		t.Fatalf("bob's export holds alice's translations: %s", bob)
	}
	if own := a.moveImport("pcx_alice_import", "/api/v1/import", "application/json", export); num(own, "added")+num(own, "terms") != 0 {
		t.Fatalf("alice's own file back: %v", own)
	}

	first := a.moveImport("pcx_bob_import", "/api/v1/import", "application/json", export)
	second := a.moveImport("pcx_bob_import", "/api/v1/import", "application/json", export)
	if num(first, "added") != 1 || num(first, "terms") != 1 || num(second, "added")+num(second, "terms")+num(second, "conversations") != 0 {
		t.Fatalf("imports: %v then %v", first, second)
	}

	csv := []byte("English,German,Good morning,Guten Morgen\n")
	c1 := a.moveImport("pcx_bob_import", "/api/v1/import", "text/csv", csv)
	c2 := a.moveImport("pcx_bob_import", "/api/v1/import", "text/csv", csv)
	if num(c1, "added") != 1 || num(c2, "added") != 0 {
		t.Fatalf("csv imports: %v then %v", c1, c2)
	}

	if l := a.ok(a.bob, "GET", "/api/v1/translations", nil)["translations"].([]any); len(l) != 2 {
		t.Fatalf("bob's translations: %v", l)
	}
	if l := a.ok(a.alice, "GET", "/api/v1/translations", nil)["translations"].([]any); len(l) != 1 {
		t.Fatalf("alice's translations changed: %v", l)
	}
}
