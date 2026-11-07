// Package engine holds what Pocket Translate needs to know about languages
// and the offline engines: the language list, Serbian's two alphabets and
// a client for servers that speak the LibreTranslate API (LibreTranslate
// itself with its Argos models, and Pocket Translate's NLLB-200 engine).
package engine

import (
	"sort"
	"strings"
)

// Lang is a language a person can pick.
type Lang struct {
	Code   string `json:"code"`   // what the app uses: ISO 639-1, sr-Latn / sr-Cyrl, zt
	Name   string `json:"name"`   // in English
	Native string `json:"native"` // in itself
	RTL    bool   `json:"rtl,omitempty"`
	Speech string `json:"speech"` // BCP 47 tag for reading aloud in the browser
	OCR    string `json:"ocr"`    // tesseract's language, "" = none
}

// Langs is every language the app offers, by English name.
var Langs = []Lang{
	{"af", "Afrikaans", "Afrikaans", false, "af-ZA", "afr"},
	{"sq", "Albanian", "Shqip", false, "sq-AL", "sqi"},
	{"am", "Amharic", "አማርኛ", false, "am-ET", "amh"},
	{"ar", "Arabic", "العربية", true, "ar-SA", "ara"},
	{"hy", "Armenian", "Հայերեն", false, "hy-AM", "hye"},
	{"az", "Azerbaijani", "Azərbaycanca", false, "az-AZ", "aze"},
	{"eu", "Basque", "Euskara", false, "eu-ES", "eus"},
	{"be", "Belarusian", "Беларуская", false, "be-BY", "bel"},
	{"bn", "Bengali", "বাংলা", false, "bn-IN", "ben"},
	{"bs", "Bosnian", "Bosanski", false, "bs-BA", "bos"},
	{"bg", "Bulgarian", "Български", false, "bg-BG", "bul"},
	{"my", "Burmese", "မြန်မာ", false, "my-MM", "mya"},
	{"ca", "Catalan", "Català", false, "ca-ES", "cat"},
	{"zh", "Chinese (simplified)", "简体中文", false, "zh-CN", "chi_sim"},
	{"zt", "Chinese (traditional)", "繁體中文", false, "zh-TW", "chi_tra"},
	{"hr", "Croatian", "Hrvatski", false, "hr-HR", "hrv"},
	{"cs", "Czech", "Čeština", false, "cs-CZ", "ces"},
	{"da", "Danish", "Dansk", false, "da-DK", "dan"},
	{"nl", "Dutch", "Nederlands", false, "nl-NL", "nld"},
	{"en", "English", "English", false, "en-GB", "eng"},
	{"eo", "Esperanto", "Esperanto", false, "eo", "epo"},
	{"et", "Estonian", "Eesti", false, "et-EE", "est"},
	{"fi", "Finnish", "Suomi", false, "fi-FI", "fin"},
	{"fr", "French", "Français", false, "fr-FR", "fra"},
	{"gl", "Galician", "Galego", false, "gl-ES", "glg"},
	{"ka", "Georgian", "ქართული", false, "ka-GE", "kat"},
	{"de", "German", "Deutsch", false, "de-DE", "deu"},
	{"el", "Greek", "Ελληνικά", false, "el-GR", "ell"},
	{"gu", "Gujarati", "ગુજરાતી", false, "gu-IN", "guj"},
	{"ha", "Hausa", "Hausa", false, "ha-NG", ""},
	{"he", "Hebrew", "עברית", true, "he-IL", "heb"},
	{"hi", "Hindi", "हिन्दी", false, "hi-IN", "hin"},
	{"hu", "Hungarian", "Magyar", false, "hu-HU", "hun"},
	{"is", "Icelandic", "Íslenska", false, "is-IS", "isl"},
	{"ig", "Igbo", "Igbo", false, "ig-NG", ""},
	{"id", "Indonesian", "Bahasa Indonesia", false, "id-ID", "ind"},
	{"ga", "Irish", "Gaeilge", false, "ga-IE", "gle"},
	{"it", "Italian", "Italiano", false, "it-IT", "ita"},
	{"ja", "Japanese", "日本語", false, "ja-JP", "jpn"},
	{"kn", "Kannada", "ಕನ್ನಡ", false, "kn-IN", "kan"},
	{"kk", "Kazakh", "Қазақ", false, "kk-KZ", "kaz"},
	{"km", "Khmer", "ខ្មែរ", false, "km-KH", "khm"},
	{"ko", "Korean", "한국어", false, "ko-KR", "kor"},
	{"ky", "Kyrgyz", "Кыргызча", false, "ky-KG", "kir"},
	{"lo", "Lao", "ລາວ", false, "lo-LA", "lao"},
	{"lv", "Latvian", "Latviešu", false, "lv-LV", "lav"},
	{"lt", "Lithuanian", "Lietuvių", false, "lt-LT", "lit"},
	{"lb", "Luxembourgish", "Lëtzebuergesch", false, "lb-LU", "ltz"},
	{"mk", "Macedonian", "Македонски", false, "mk-MK", "mkd"},
	{"ms", "Malay", "Bahasa Melayu", false, "ms-MY", "msa"},
	{"ml", "Malayalam", "മലയാളം", false, "ml-IN", "mal"},
	{"mt", "Maltese", "Malti", false, "mt-MT", "mlt"},
	{"mr", "Marathi", "मराठी", false, "mr-IN", "mar"},
	{"mn", "Mongolian", "Монгол", false, "mn-MN", "mon"},
	{"cnr", "Montenegrin", "Crnogorski", false, "sr-ME", "srp_latn"},
	{"ne", "Nepali", "नेपाली", false, "ne-NP", "nep"},
	{"nb", "Norwegian", "Norsk", false, "nb-NO", "nor"},
	{"ps", "Pashto", "پښتو", true, "ps-AF", "pus"},
	{"fa", "Persian", "فارسی", true, "fa-IR", "fas"},
	{"pl", "Polish", "Polski", false, "pl-PL", "pol"},
	{"pt", "Portuguese", "Português", false, "pt-PT", "por"},
	{"pb", "Portuguese (Brazil)", "Português (Brasil)", false, "pt-BR", "por"},
	{"pa", "Punjabi", "ਪੰਜਾਬੀ", false, "pa-IN", "pan"},
	{"ro", "Romanian", "Română", false, "ro-RO", "ron"},
	{"ru", "Russian", "Русский", false, "ru-RU", "rus"},
	{"sr-Latn", "Serbian (Latin)", "Srpski (latinica)", false, "sr-RS", "srp_latn"},
	{"sr-Cyrl", "Serbian (Cyrillic)", "Српски (ћирилица)", false, "sr-RS", "srp"},
	{"si", "Sinhala", "සිංහල", false, "si-LK", "sin"},
	{"sk", "Slovak", "Slovenčina", false, "sk-SK", "slk"},
	{"sl", "Slovenian", "Slovenščina", false, "sl-SI", "slv"},
	{"so", "Somali", "Soomaali", false, "so-SO", ""},
	{"es", "Spanish", "Español", false, "es-ES", "spa"},
	{"sw", "Swahili", "Kiswahili", false, "sw-KE", "swa"},
	{"sv", "Swedish", "Svenska", false, "sv-SE", "swe"},
	{"tl", "Tagalog", "Tagalog", false, "fil-PH", "tgl"},
	{"tg", "Tajik", "Тоҷикӣ", false, "tg-TJ", "tgk"},
	{"ta", "Tamil", "தமிழ்", false, "ta-IN", "tam"},
	{"te", "Telugu", "తెలుగు", false, "te-IN", "tel"},
	{"th", "Thai", "ไทย", false, "th-TH", "tha"},
	{"tr", "Turkish", "Türkçe", false, "tr-TR", "tur"},
	{"uk", "Ukrainian", "Українська", false, "uk-UA", "ukr"},
	{"ur", "Urdu", "اردو", true, "ur-PK", "urd"},
	{"uz", "Uzbek", "Oʻzbekcha", false, "uz-UZ", "uzb"},
	{"vi", "Vietnamese", "Tiếng Việt", false, "vi-VN", "vie"},
	{"cy", "Welsh", "Cymraeg", false, "cy-GB", "cym"},
	{"xh", "Xhosa", "isiXhosa", false, "xh-ZA", ""},
	{"yo", "Yoruba", "Yorùbá", false, "yo-NG", "yor"},
	{"zu", "Zulu", "isiZulu", false, "zu-ZA", ""},
}

var byCode = func() map[string]Lang {
	m := map[string]Lang{}
	for _, l := range Langs {
		m[strings.ToLower(l.Code)] = l
	}
	return m
}()

// aliases are other ways people and engines write a language.
var aliases = map[string]string{
	"sr": "sr-Latn", "sr-latn": "sr-Latn", "sr-cyrl": "sr-Cyrl", "srp": "sr-Latn", "sr-rs": "sr-Latn", "sr-me": "cnr", "me": "cnr",
	"zh-cn": "zh", "zh-hans": "zh", "zh-tw": "zt", "zh-hant": "zt", "pt-br": "pb", "pt-pt": "pt", "no": "nb", "nn": "nb",
	"en-us": "en", "en-gb": "en", "iw": "he", "fil": "tl", "in": "id", "jw": "jv",
}

// Normalize turns what a person, an import or an engine wrote into the
// app's code ("" when it is not a language the app knows). "auto" stays.
func Normalize(code string) string {
	c := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(code, "_", "-")))
	if c == "" {
		return ""
	}
	if c == "auto" {
		return "auto"
	}
	if a, ok := aliases[c]; ok {
		return a
	}
	if l, ok := byCode[c]; ok {
		return l.Code
	}
	if i := strings.IndexByte(c, '-'); i > 0 {
		if l, ok := byCode[c[:i]]; ok {
			return l.Code
		}
	}
	// a language written out by name ("German", "Deutsch")
	for _, l := range Langs {
		if strings.EqualFold(l.Name, code) || strings.EqualFold(l.Native, code) {
			return l.Code
		}
	}
	if c == "serbian" || c == "srpski" {
		return "sr-Latn"
	}
	if c == "chinese" {
		return "zh"
	}
	return ""
}

// Get returns a language by its app code.
func Get(code string) (Lang, bool) {
	l, ok := byCode[strings.ToLower(code)]
	return l, ok
}

// Name is a language's English name (the code when unknown, "Detect" for auto).
func Name(code string) string {
	if code == "auto" || code == "" {
		return "Detect language"
	}
	if l, ok := Get(code); ok {
		return l.Name
	}
	return code
}

// Base is the code an engine knows a language by: Serbian in either
// alphabet is "sr", Montenegrin goes as Serbian.
func Base(code string) string {
	switch code {
	case "sr-Latn", "sr-Cyrl", "cnr":
		return "sr"
	}
	return code
}

// Close are languages a detector easily mixes up; the one a person uses
// wins over the one detected.
var close = [][]string{{"sr-Latn", "sr-Cyrl", "hr", "bs", "cnr"}, {"nb", "da"}, {"ms", "id"}, {"pt", "pb"}, {"zh", "zt"}}

// Prefer picks, for a detected language, the one among its close languages
// that a person uses (recent languages first), keeping the detected one
// otherwise. Serbian text in Cyrillic is always Serbian (Cyrillic).
func Prefer(detected, text string, mine []string) string {
	for _, group := range close {
		if !contains(group, detected) {
			continue
		}
		if group[0] == "sr-Latn" && IsCyrillic(text) {
			if contains([]string{"sr-Latn", "sr-Cyrl", "bs", "cnr"}, detected) || contains(mine, "sr-Cyrl") || contains(mine, "sr-Latn") {
				return "sr-Cyrl"
			}
		}
		for _, m := range mine {
			if contains(group, m) {
				if group[0] == "sr-Latn" && (m == "sr-Cyrl" || m == "sr-Latn") {
					if IsCyrillic(text) {
						return "sr-Cyrl"
					}
					return "sr-Latn"
				}
				return m
			}
		}
	}
	if detected == "sr-Latn" && IsCyrillic(text) {
		return "sr-Cyrl"
	}
	return detected
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Sorted returns the codes sorted by language name.
func Sorted(codes []string) []string {
	out := append([]string(nil), codes...)
	sort.Slice(out, func(i, j int) bool { return Name(out[i]) < Name(out[j]) })
	return out
}
