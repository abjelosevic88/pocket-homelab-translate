package store

import (
	"encoding/json"
	"strings"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Translation is one translation in a person's history; starred ones are
// their phrasebook.
type Translation struct {
	ID           int64  `json:"id"`
	UserID       int64  `json:"-"`
	Kind         string `json:"kind"`   // text | photo | speech | app | import
	Source       string `json:"source"` // "" = this app, else the linked app's slug
	SourceLang   string `json:"source_lang"`
	DetectedLang string `json:"detected_lang"`
	TargetLang   string `json:"target_lang"`
	SourceText   string `json:"source_text"`
	ResultText   string `json:"result_text"`
	Engine       string `json:"engine"` // ai | argos | nllb | import
	Model        string `json:"model"`
	Tone         string `json:"tone"`
	Starred      bool   `json:"starred"`
	Collection   string `json:"collection"`
	Note         string `json:"note"`
	LearnRef     string `json:"learn_ref"`
	ExternalID   string `json:"external_id,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// From is the language the text was in: what was detected, else what was asked.
func (t *Translation) From() string {
	if t.DetectedLang != "" {
		return t.DetectedLang
	}
	return t.SourceLang
}

// Kinds of translation.
var Kinds = []string{"text", "photo", "speech", "app", "import"}

// Term is one glossary entry: this term is always translated as that.
type Term struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"-"`
	SourceLang  string `json:"source_lang"`
	TargetLang  string `json:"target_lang"`
	Term        string `json:"term"`
	Translation string `json:"translation"`
	Note        string `json:"note"`
	MatchCase   bool   `json:"match_case"`
	CreatedAt   string `json:"created_at"`
}

// Document statuses.
const (
	DocQueued  = "queued"
	DocWorking = "working"
	DocDone    = "done"
	DocFailed  = "failed"
)

// Saved is a place a translated document was put in another app.
type Saved struct {
	App   string `json:"app"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Document is a file being translated, or translated.
type Document struct {
	ID           int64   `json:"id"`
	UserID       int64   `json:"-"`
	Name         string  `json:"name"`
	Format       string  `json:"format"`
	Mime         string  `json:"mime"`
	Size         int64   `json:"size"`
	SourceLang   string  `json:"source_lang"`
	DetectedLang string  `json:"detected_lang"`
	TargetLang   string  `json:"target_lang"`
	Engine       string  `json:"engine"`
	Used         string  `json:"used"`
	Status       string  `json:"status"`
	Done         int     `json:"done"`
	Total        int     `json:"total"`
	Error        string  `json:"error"`
	ResultName   string  `json:"result_name"`
	Origin       string  `json:"origin"`
	Saved        []Saved `json:"saved"`
	CreatedAt    string  `json:"created_at"`
	FinishedAt   string  `json:"finished_at"`
}

// Conversation is a talk between two languages.
type Conversation struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Title     string `json:"title"`
	LangA     string `json:"lang_a"`
	LangB     string `json:"lang_b"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Turns     int    `json:"turns"`
	Last      string `json:"last"` // the last thing said, for lists
}

// Turn is one thing said in a conversation, and its translation.
type Turn struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"-"`
	ConversationID int64  `json:"conversation_id"`
	Side           string `json:"side"` // a | b
	Spoken         bool   `json:"spoken"`
	SourceLang     string `json:"source_lang"`
	TargetLang     string `json:"target_lang"`
	SourceText     string `json:"source_text"`
	ResultText     string `json:"result_text"`
	Engine         string `json:"engine"`
	CreatedAt      string `json:"created_at"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	// The offline engines (box-wide, the admin's): servers speaking the
	// LibreTranslate API. Argos (LibreTranslate itself) is quick; NLLB-200
	// is slower and knows many more languages (Serbian among them).
	ArgosURL string `json:"argos_url"`
	ArgosKey string `json:"argos_key"`
	NLLBURL  string `json:"nllb_url"`
	NLLBKey  string `json:"nllb_key"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// Translating.
	SourceLang     string   `json:"source_lang"` // auto or a code
	TargetLang     string   `json:"target_lang"`
	RecentLangs    []string `json:"recent_langs"`
	Engine         string   `json:"engine"` // auto | ai | offline
	Tone           string   `json:"tone"`   // "" | formal | casual
	LiveTranslate  bool     `json:"live_translate"`
	KeepHistory    bool     `json:"keep_history"`
	HistoryDays    int      `json:"history_days"`     // 0 = keep everything
	KeepAppHistory bool     `json:"keep_app_history"` // what other apps have translated
	OCRLangs       []string `json:"ocr_langs"`        // languages photos are read in

	// Speech: written out by Pocket Voice or the person's own OpenAI key.
	Speech      string `json:"speech"` // voice | openai
	SpeechModel string `json:"speech_model"`
	AutoSpeak   bool   `json:"auto_speak"` // read conversation translations aloud

	// The other apps.
	LearnDeck   string `json:"learn_deck"`
	NotesFolder string `json:"notes_folder"`
	FilesFolder string `json:"files_folder"`

	// Notifications.
	NotifyDocs   bool   `json:"notify_docs"` // a long document is done
	NotifyURL    string `json:"notify_url"`
	NotifyFormat string `json:"notify_format"` // apprise | ntfy | json

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func cleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		k := strings.ToLower(t)
		if t == "" || seen[k] || len(out) >= 20 {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out
}
