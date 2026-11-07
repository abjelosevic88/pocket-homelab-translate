package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeLibre is an offline engine: it knows a few words and puts the rest
// in brackets. As "nllb" it answers Serbian in Cyrillic, like NLLB does.
func fakeLibre(t *testing.T, name string) *httptest.Server {
	t.Helper()
	words := map[string]string{"the": "das", "car": "Auto", "is": "ist", "red": "rot", "hello": "hallo"}
	sr := map[string]string{"the": "", "car": "аутомобил", "is": "је", "red": "црвен", "hello": "здраво"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/languages":
			langs := []map[string]any{{"code": "en", "targets": []string{"de"}}, {"code": "de", "targets": []string{"en"}}}
			if name == "nllb" {
				langs = []map[string]any{{"code": "en", "targets": []string{"sr", "de"}}, {"code": "sr", "targets": []string{"en"}}}
			}
			json.NewEncoder(w).Encode(langs)
		case "/detect":
			json.NewEncoder(w).Encode([]map[string]any{{"language": "en", "confidence": 90}})
		case "/translate":
			var in struct {
				Q      []string `json:"q"`
				Source string   `json:"source"`
				Target string   `json:"target"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			out := []string{}
			for _, q := range in.Q {
				var parts []string
				for _, wd := range strings.Fields(strings.ToLower(q)) {
					d := words
					if in.Target == "sr" {
						d = sr
					}
					if strings.HasPrefix(wd, "[") && strings.HasSuffix(wd, "]") {
						parts = append(parts, wd) // placeholders pass, as with Argos and NLLB
					} else if v, ok := d[wd]; ok {
						if v != "" {
							parts = append(parts, v)
						}
					} else {
						parts = append(parts, "["+wd+"]")
					}
				}
				out = append(out, strings.Join(parts, " "))
			}
			json.NewEncoder(w).Encode(map[string]any{"translatedText": out})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

// fakeAI answers chat completions: every item comes back as "AI(<item>)".
func fakeAI(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Messages []struct {
				Role, Content string
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if seen != nil {
			*seen = append(*seen, in.Messages[0].Content)
		}
		var items []string
		_ = json.Unmarshal([]byte(in.Messages[1].Content), &items)
		out := []string{}
		for _, it := range items {
			out = append(out, "AI("+it+")")
		}
		content, _ := json.Marshal(map[string]any{"detected": "en", "items": out, "alternatives": []string{"other way"}})
		json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": string(content)}}}})
	}))
	t.Cleanup(ts.Close)
	return ts
}

func withEngines(t *testing.T, a *app) {
	t.Helper()
	argos, nllb := fakeLibre(t, "argos"), fakeLibre(t, "nllb")
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"argos_url": argos.URL, "nllb_url": nllb.URL})
}

func TestOfflineTranslateGlossaryAndSerbian(t *testing.T) {
	a := newApp(t, nil)
	withEngines(t, a)
	res := a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "The car is red", "from": "en", "to": "de", "engine": "offline"})
	if res["text"] != "das Auto ist rot" || res["engine"] != "argos" {
		t.Fatalf("argos: %v", res)
	}
	// a glossary term replaces the engine's own word
	a.ok(a.alice, "POST", "/api/v1/glossary", map[string]any{"source_lang": "en", "target_lang": "de", "term": "car", "translation": "Wagen"})
	res = a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "The car is red", "from": "auto", "to": "de", "engine": "offline"})
	if res["text"] != "das Wagen ist rot" || res["terms"] != float64(1) || res["detected"] != "en" {
		t.Fatalf("glossary: %v", res)
	}
	// Bob's translations do not use Alice's glossary
	res = a.ok(a.bob, "POST", "/api/v1/translate", map[string]any{"text": "The car is red", "from": "en", "to": "de", "engine": "offline"})
	if res["text"] != "das Auto ist rot" {
		t.Fatalf("bob: %v", res)
	}
	// Serbian only NLLB knows; it answers Cyrillic, Latin is asked for
	res = a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "The car is red", "from": "en", "to": "sr-Latn", "engine": "offline"})
	if res["text"] != "automobil je crven" || res["engine"] != "nllb" {
		t.Fatalf("serbian latin: %v", res)
	}
	res = a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "hello", "from": "en", "to": "sr-Cyrl", "engine": "offline"})
	if res["text"] != "здраво" {
		t.Fatalf("serbian cyrillic: %v", res)
	}
	// same language, other alphabet: no engine needed
	res = a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "Zdravo svete", "from": "sr-Latn", "to": "sr-Cyrl", "engine": "offline"})
	if res["text"] != "Здраво свете" {
		t.Fatalf("script: %v", res)
	}
	// a pair nobody knows says so plainly
	code, m, _ := a.call(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "hallo", "from": "de", "to": "ja", "engine": "offline"})
	if code != http.StatusServiceUnavailable || !strings.Contains(fmt.Sprint(m["error"]), "no offline engine translates German into Japanese") {
		t.Fatalf("unknown pair: %d %v", code, m)
	}
	// languages say which engine knows them
	langs := a.ok(a.alice, "GET", "/api/v1/languages", nil)["languages"].([]any)
	for _, l := range langs {
		m := l.(map[string]any)
		if m["code"] == "sr-Latn" && (m["nllb"] != true || m["argos"] != false) {
			t.Fatalf("sr-Latn: %v", m)
		}
	}
}

func TestAITranslateToneAndFallback(t *testing.T) {
	a := newApp(t, nil)
	var prompts []string
	ai := fakeAI(t, &prompts)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"ai_provider": "openai", "openai_api_key": "sk-test", "openai_base_url": ai.URL})
	res := a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "Hello", "to": "de", "tone": "formal", "alternatives": true})
	if res["text"] != "AI(Hello)" || res["engine"] != "ai" || res["provider"] != "openai" || res["detected"] != "en" {
		t.Fatalf("ai: %v", res)
	}
	if alts := res["alternatives"].([]any); len(alts) != 1 || alts[0] != "other way" {
		t.Fatalf("alternatives: %v", res["alternatives"])
	}
	if !strings.Contains(prompts[0], "formal") || !strings.Contains(prompts[0], "German") {
		t.Fatalf("prompt: %s", prompts[0])
	}
	// kept in the history
	tr := res["translation"].(map[string]any)
	if tr["source_text"] != "Hello" || tr["result_text"] != "AI(Hello)" {
		t.Fatalf("history: %v", tr)
	}
	// Serbian Latin is asked for by name and alphabet
	a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "Hello", "to": "sr-Latn"})
	if !strings.Contains(prompts[len(prompts)-1], "Latin alphabet") {
		t.Fatalf("serbian prompt: %s", prompts[len(prompts)-1])
	}
	// the AI failing in auto falls back to the offline engine, saying so
	ai.Close()
	withEngines(t, a)
	res = a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "The car", "from": "en", "to": "de", "engine": "auto"})
	if res["text"] != "das Auto" || res["note"] == "" {
		t.Fatalf("fallback: %v", res)
	}
	// "ai" does not fall back
	a.expect(http.StatusBadGateway, a.alice, "POST", "/api/v1/translate", map[string]any{"text": "The car", "from": "en", "to": "de", "engine": "ai"})
	// without any engine Bob is told what to set up
	code, m, _ := a.call(a.bob, "POST", "/api/v1/translate", map[string]any{"text": "x", "to": "de", "engine": "ai"})
	if code != http.StatusServiceUnavailable || m["error"] == "" {
		t.Fatalf("no ai: %d %v", code, m)
	}
}

func TestPerPersonIsolation(t *testing.T) {
	a := newApp(t, nil)
	withEngines(t, a)
	tr := a.ok(a.alice, "POST", "/api/v1/translations", map[string]any{"source_text": "hello", "result_text": "hallo", "source_lang": "en", "target_lang": "de", "collection": "Travel"})
	tid := id(tr)
	term := a.ok(a.alice, "POST", "/api/v1/glossary", map[string]any{"source_lang": "en", "target_lang": "de", "term": "car", "translation": "Wagen"})
	conv := a.ok(a.alice, "POST", "/api/v1/conversations", map[string]any{"lang_a": "en", "lang_b": "de"})
	turn := a.ok(a.alice, "POST", "/api/v1/conversations/"+id(conv)+"/turns", map[string]any{"side": "a", "text": "hello"})
	if tu := turn["turn"].(map[string]any); tu["result_text"] != "hallo" {
		t.Fatalf("turn: %v", turn)
	}
	doc := a.upload(a.alice, "/api/v1/documents", "note.txt", []byte("Hello\n\nThe car is red\n"), map[string]string{"from": "en", "to": "de", "engine": "offline"})
	for _, p := range []string{"/api/v1/translations/" + tid, "/api/v1/conversations/" + id(conv), "/api/v1/documents/" + id(doc), "/api/v1/documents/" + id(doc) + "/original"} {
		a.expect(http.StatusNotFound, a.bob, "GET", p, nil)
	}
	a.expect(http.StatusNotFound, a.bob, "PUT", "/api/v1/translations/"+tid, map[string]any{"starred": false})
	a.expect(http.StatusNotFound, a.bob, "DELETE", "/api/v1/translations/"+tid, nil)
	a.expect(http.StatusNotFound, a.bob, "PUT", "/api/v1/glossary/"+id(term), map[string]any{"source_lang": "en", "target_lang": "de", "term": "x", "translation": "y"})
	a.expect(http.StatusNotFound, a.bob, "DELETE", "/api/v1/glossary/"+id(term), nil)
	a.expect(http.StatusNotFound, a.bob, "DELETE", "/api/v1/conversations/"+id(conv), nil)
	a.expect(http.StatusNotFound, a.bob, "POST", "/api/v1/conversations/"+id(conv)+"/turns", map[string]any{"side": "b", "text": "x"})
	a.expect(http.StatusNotFound, a.bob, "DELETE", "/api/v1/turns/"+id(turn["turn"].(map[string]any)), nil)
	a.expect(http.StatusNotFound, a.bob, "POST", "/api/v1/documents/"+id(doc)+"/retry", nil)
	if l := a.ok(a.bob, "GET", "/api/v1/translations", nil)["translations"].([]any); len(l) != 0 {
		t.Fatalf("bob sees %d translations", len(l))
	}
	if l := a.ok(a.bob, "GET", "/api/v1/glossary", nil)["terms"].([]any); len(l) != 0 {
		t.Fatal("bob sees the glossary")
	}
	if l := a.ok(a.bob, "GET", "/api/v1/documents", nil)["documents"].([]any); len(l) != 0 {
		t.Fatal("bob sees documents")
	}
	if l := a.ok(a.bob, "GET", "/api/v1/conversations", nil)["conversations"].([]any); len(l) != 0 {
		t.Fatal("bob sees conversations")
	}
	if r := a.ok(a.bob, "GET", "/api/v1/search?q=hello", nil)["results"].([]any); len(r) != 0 {
		t.Fatal("bob finds alice's phrases")
	}
	// the document gets translated, layout kept
	a.waitDoc(a.alice, id(doc))
	b := a.raw(a.alice, "/api/v1/documents/"+id(doc)+"/result")
	if string(b) != "hallo\n\ndas Wagen ist rot\n" { // with her glossary
		t.Fatalf("result %q", b)
	}
	a.expect(http.StatusNotFound, a.bob, "GET", "/api/v1/documents/"+id(doc)+"/result", nil)
}

func TestSettingsPerPerson(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"target_lang": "de", "tone": "formal", "argos_url": "http://argos.test"})
	a.ok(a.bob, "PUT", "/api/v1/settings", map[string]any{"target_lang": "sr", "argos_url": "http://evil.test", "nllb_url": "http://evil.test"})
	al := a.ok(a.alice, "GET", "/api/v1/settings", nil)
	bo := a.ok(a.bob, "GET", "/api/v1/settings", nil)
	if al["target_lang"] != "de" || al["tone"] != "formal" || bo["target_lang"] != "sr-Latn" || bo["tone"] != "" {
		t.Fatalf("alice %v / bob %v", al["target_lang"], bo["target_lang"])
	}
	if al["argos_url"] != "http://argos.test" || al["nllb_url"] != "" {
		t.Fatalf("a member changed the box engines: %v %v", al["argos_url"], al["nllb_url"])
	}
	if _, ok := bo["argos_url"]; ok {
		t.Fatal("a member sees the engine addresses")
	}
	a.expect(http.StatusBadRequest, a.alice, "PUT", "/api/v1/settings", map[string]any{"target_lang": "klingon"})
	a.expect(http.StatusBadRequest, a.alice, "PUT", "/api/v1/settings", map[string]any{"target_lang": "auto"})
}

func TestExportImportIdempotent(t *testing.T) {
	a := newApp(t, nil)
	withEngines(t, a)
	a.ok(a.alice, "POST", "/api/v1/translations", map[string]any{"source_text": "hello", "result_text": "hallo", "source_lang": "en", "target_lang": "de"})
	a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "The car", "from": "en", "to": "de", "engine": "offline"})
	a.ok(a.alice, "POST", "/api/v1/glossary", map[string]any{"source_lang": "en", "target_lang": "de", "term": "car", "translation": "Wagen"})
	c := a.ok(a.alice, "POST", "/api/v1/conversations", map[string]any{"lang_a": "en", "lang_b": "de"})
	a.ok(a.alice, "POST", "/api/v1/conversations/"+id(c)+"/turns", map[string]any{"side": "a", "text": "hello"})
	exp := a.raw(a.alice, "/api/v1/export.json")
	// into Bob's account, twice
	r1 := a.post(a.bob, "/api/v1/import", exp)
	r2 := a.post(a.bob, "/api/v1/import", exp)
	if r1["added"] != float64(2) || r1["terms"] != float64(1) || r1["conversations"] != float64(1) {
		t.Fatalf("first import %v", r1)
	}
	if r2["added"] != float64(0) || r2["unchanged"] != float64(2) || r2["terms"] != float64(0) || r2["conversations"] != float64(0) {
		t.Fatalf("second import %v", r2)
	}
	// Google Translate's saved phrases
	g := []byte("English,German,Good morning,Guten Morgen\nEnglish,Serbian,Thank you,Hvala\n")
	g1 := a.post(a.bob, "/api/v1/import?collection=Google", g)
	g2 := a.post(a.bob, "/api/v1/import?collection=Google", g)
	if g1["added"] != float64(2) || g2["added"] != float64(0) {
		t.Fatalf("google %v %v", g1, g2)
	}
	ph := a.ok(a.bob, "GET", "/api/v1/translations?starred=1&collection=Google", nil)["translations"].([]any)
	if len(ph) != 2 || ph[0].(map[string]any)["target_lang"] == "" {
		t.Fatalf("phrasebook %v", ph)
	}
	// glossary CSV (DeepL's two columns) for a pair
	gl := a.post(a.bob, "/api/v1/glossary/import?from=en&to=de", []byte("invoice,Rechnung\nboard,Vorstand\n"))
	if gl["added"] != float64(2) {
		t.Fatalf("glossary import %v", gl)
	}
}

func TestLinkScopes(t *testing.T) {
	req := func(method, path string) *http.Request { return httptest.NewRequest(method, path, nil) }
	cases := []struct {
		scopes []string
		method string
		path   string
		want   bool
	}{
		{[]string{"translate:run"}, "POST", "/api/v1/translate", true},
		{[]string{"translate:run"}, "GET", "/api/v1/translations", false},
		{[]string{"phrasebook:read"}, "GET", "/api/v1/translations", true},
		{[]string{"phrasebook:read"}, "POST", "/api/v1/translations", false},
		{[]string{"phrasebook:write"}, "PUT", "/api/v1/translations/4", true},
		{[]string{"phrasebook:read"}, "GET", "/api/v1/day/2026-11-07", true},
		{[]string{"translate:run"}, "GET", "/api/v1/languages", true},
		{[]string{"translate:run", "phrasebook:read", "phrasebook:write"}, "GET", "/api/v1/settings", false},
		{[]string{"translate:run", "phrasebook:read", "phrasebook:write"}, "GET", "/api/v1/documents", false},
		{[]string{"translate:run", "phrasebook:read", "phrasebook:write"}, "GET", "/api/v1/export.json", false},
	}
	for _, c := range cases {
		if got := linkAllows(c.scopes, req(c.method, c.path)); got != c.want {
			t.Errorf("%v %s %s = %v, want %v", c.scopes, c.method, c.path, got, c.want)
		}
	}
}

func TestLibreTranslateCompatibleAPI(t *testing.T) {
	a := newApp(t, nil)
	withEngines(t, a)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"engine": "offline"})
	form := url.Values{"q": {"hello"}, "source": {"auto"}, "target": {"de"}, "api_key": {a.alice}}
	res, err := http.PostForm(a.url+"/translate", form)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	if out["translatedText"] != "hallo" || out["detectedLanguage"].(map[string]any)["language"] != "en" {
		t.Fatalf("compat: %v", out)
	}
	res, _ = http.PostForm(a.url+"/translate", url.Values{"q": {"hello"}, "target": {"de"}, "api_key": {"ptl_wrong"}})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad key: %d", res.StatusCode)
	}
	// not kept in the history
	if l := a.ok(a.alice, "GET", "/api/v1/translations", nil)["translations"].([]any); len(l) != 0 {
		t.Fatalf("compat kept %d", len(l))
	}
	// HTML keeps its tags
	body, _ := json.Marshal(map[string]any{"q": "<p>hello <b>red</b></p>", "source": "en", "target": "de", "format": "html", "api_key": a.alice})
	res, _ = http.Post(a.url+"/translate", "application/json", bytes.NewReader(body))
	_ = json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	if out["translatedText"] != "<p>hallo <b>rot</b></p>" {
		t.Fatalf("html: %v", out)
	}
}

func TestHistoryAndPhrasebook(t *testing.T) {
	a := newApp(t, nil)
	withEngines(t, a)
	r := a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "hello", "from": "en", "to": "de", "engine": "offline"})
	tid := id(r["translation"].(map[string]any))
	a.ok(a.alice, "PUT", "/api/v1/translations/"+tid, map[string]any{"starred": true, "collection": "Travel", "result_text": "Hallo!"})
	a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "red", "from": "en", "to": "de", "engine": "offline"})
	cols := a.ok(a.alice, "GET", "/api/v1/collections", nil)["collections"].([]any)
	if len(cols) != 1 || cols[0].(map[string]any)["name"] != "Travel" {
		t.Fatalf("collections %v", cols)
	}
	// clearing history keeps the phrasebook
	if n := a.ok(a.alice, "DELETE", "/api/v1/translations", nil)["removed"]; n != float64(1) {
		t.Fatalf("cleared %v", n)
	}
	l := a.ok(a.alice, "GET", "/api/v1/translations", nil)["translations"].([]any)
	if len(l) != 1 || l[0].(map[string]any)["result_text"] != "Hallo!" {
		t.Fatalf("left %v", l)
	}
	// keep_history off: nothing new is kept
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"keep_history": false})
	if r := a.ok(a.alice, "POST", "/api/v1/translate", map[string]any{"text": "red", "from": "en", "to": "de", "engine": "offline"}); r["translation"] != nil {
		t.Fatal("kept with history off")
	}
	// the day and search, for the rest of the box
	day := a.ok(a.alice, "GET", "/api/v1/day/"+time.Now().Format("2006-01-02"), nil)
	if day["count"] != float64(1) {
		t.Fatalf("day %v", day)
	}
	if r := a.ok(a.alice, "GET", "/api/v1/search?q=hallo", nil)["results"].([]any); len(r) != 1 {
		t.Fatalf("search %v", r)
	}
	// Learn and Notes are not connected: said plainly
	code, m, _ := a.call(a.alice, "POST", "/api/v1/translations/"+tid+"/learn", nil)
	if code != http.StatusServiceUnavailable || !strings.Contains(fmt.Sprint(m["error"]), "Pocket Learn") {
		t.Fatalf("learn: %d %v", code, m)
	}
}

// ---- helpers ----

func (a *app) upload(auth, path, name string, data []byte, fields map[string]string) map[string]any {
	a.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", a.url+path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	if res.StatusCode >= 300 {
		a.t.Fatalf("upload → %d %v", res.StatusCode, m)
	}
	return m
}

func (a *app) raw(auth, path string) []byte {
	a.t.Helper()
	req, _ := http.NewRequest("GET", a.url+path, nil)
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		a.t.Fatalf("GET %s → %d %s", path, res.StatusCode, b)
	}
	return b
}

func (a *app) post(auth, path string, body []byte) map[string]any {
	a.t.Helper()
	req, _ := http.NewRequest("POST", a.url+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	if res.StatusCode >= 300 {
		a.t.Fatalf("POST %s → %d %v", path, res.StatusCode, m)
	}
	return m
}

func (a *app) waitDoc(auth, docID string) map[string]any {
	a.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		d := a.ok(auth, "GET", "/api/v1/documents/"+docID, nil)
		switch d["status"] {
		case "done":
			return d
		case "failed":
			a.t.Fatalf("document failed: %v", d["error"])
		}
		time.Sleep(50 * time.Millisecond)
	}
	a.t.Fatal("document not translated in time")
	return nil
}
