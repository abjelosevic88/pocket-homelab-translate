package engine

import "testing"

func TestSerbianScripts(t *testing.T) {
	cases := []struct{ cyr, lat string }{
		{"Где је најближа апотека?", "Gde je najbliža apoteka?"},
		{"Љубав и њива, џем", "Ljubav i njiva, džem"},
		{"ЉУБАВ", "LJUBAV"},
		{"Ђорђе Ћирић", "Đorđe Ćirić"},
	}
	for _, c := range cases {
		if got := ToLatin(c.cyr); got != c.lat {
			t.Errorf("ToLatin(%q) = %q, want %q", c.cyr, got, c.lat)
		}
		if got := ToCyrillic(c.lat); got != c.cyr {
			t.Errorf("ToCyrillic(%q) = %q, want %q", c.lat, got, c.cyr)
		}
	}
	if got := ToCyrillic("Pogledaj www.example.com i WiFi."); got != "Погледај www.example.com и WiFi." {
		t.Errorf("foreign words: %q", got)
	}
	if !IsCyrillic("Здраво свете") || IsCyrillic("Zdravo svete") {
		t.Error("IsCyrillic")
	}
}

func TestNormalizeAndPrefer(t *testing.T) {
	for in, want := range map[string]string{"sr": "sr-Latn", "SR_Cyrl": "sr-Cyrl", "German": "de", "zh-TW": "zt", "pt-BR": "pb", "en-US": "en", "xx": "", "auto": "auto"} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Prefer("hr", "Treba mi nešto", []string{"en", "sr-Latn"}); got != "sr-Latn" {
		t.Errorf("Prefer hr → %q", got)
	}
	if got := Prefer("bs", "Треба ми нешто", nil); got != "sr-Cyrl" {
		t.Errorf("Prefer Cyrillic → %q", got)
	}
	if got := Prefer("hr", "Trebam nešto", []string{"en", "de"}); got != "hr" {
		t.Errorf("Prefer keeps → %q", got)
	}
}
