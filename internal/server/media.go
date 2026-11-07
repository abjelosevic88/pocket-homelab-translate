package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-translate/internal/engine"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// ---- reading photos (tesseract) ----

var reOCR = regexp.MustCompile(`^[a-z]{3}(_[a-z]{3,4})?$`)

var ocrCache struct {
	sync.Mutex
	langs map[string]bool
	at    time.Time
}

// ocrLangs are the languages tesseract on this server reads (cached ten
// minutes; empty without tesseract).
func (s *Server) ocrLangs(ctx context.Context) map[string]bool {
	ocrCache.Lock()
	defer ocrCache.Unlock()
	if ocrCache.langs != nil && time.Since(ocrCache.at) < 10*time.Minute {
		return ocrCache.langs
	}
	out := map[string]bool{}
	if path, err := exec.LookPath(s.cfg.Tesseract); err == nil {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		b, _ := exec.CommandContext(cctx, path, "--list-langs").CombinedOutput()
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if reOCR.MatchString(l) && l != "osd" {
				out[l] = true
			}
		}
	}
	ocrCache.langs, ocrCache.at = out, time.Now()
	return out
}

// ocrStandIn reads a language when its own pack is missing: Serbian and
// Bosnian Latin share Croatian's letters.
var ocrStandIn = map[string]string{"srp_latn": "hrv", "bos": "hrv", "cnr": "hrv"}

// ocrHas is the tesseract language to use for c, "" when none is installed.
func ocrHas(have map[string]bool, c string) string {
	if have[c] {
		return c
	}
	if alt := ocrStandIn[c]; alt != "" && have[alt] {
		return alt
	}
	return ""
}

// ocrUse is the "+"-joined tesseract languages for a person: the one asked
// for first, then their own list, only those installed.
func (s *Server) ocrUse(ctx context.Context, st store.Settings, from string) (string, error) {
	have := s.ocrLangs(ctx)
	if len(have) == 0 {
		return "", &errNoEngine{"reading photos needs tesseract on this server (it is in the Docker image)"}
	}
	use := []string{}
	add := func(c string) {
		if c = ocrHas(have, c); c != "" && !oneOf(c, use) && len(use) < 6 {
			use = append(use, c)
		}
	}
	if l, ok := engine.Get(from); ok && l.OCR != "" {
		add(l.OCR)
	}
	for _, c := range st.OCRLangs {
		add(c)
	}
	if len(use) == 0 {
		add("eng")
	}
	if len(use) == 0 {
		langs := keysOf(have)
		sort.Strings(langs)
		use = langs[:1]
	}
	return strings.Join(use, "+"), nil
}

// ocrImage reads the text in an image.
func (s *Server) ocrImage(ctx context.Context, img []byte, langs string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, s.cfg.Tesseract, "stdin", "stdout", "-l", langs, "--psm", "3")
	cmd.Stdin = bytes.NewReader(img)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=2")
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if strings.Contains(msg, "Error in pixReadMem") || strings.Contains(msg, "Unsupported image") {
			return "", errors.New("this image could not be opened; use a JPEG or PNG photo")
		}
		return "", fmt.Errorf("reading the photo failed: %s", clip(firstNonEmpty(msg, err.Error()), 200))
	}
	return cleanOCR(out.String()), nil
}

var blankRuns = regexp.MustCompile(`\n{3,}`)

func cleanOCR(s string) string {
	s = strings.ReplaceAll(s, "\f", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimSpace(blankRuns.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

// handleOCR reads the text in a photo (to translate it next).
func (s *Server) handleOCR(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	data, _, _, ok := s.formFile(w, r, 25<<20)
	if !ok {
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	langs, err := s.ocrUse(ctx, st, engine.Normalize(r.FormValue("from")))
	if err != nil {
		s.writeTrErr(w, err)
		return
	}
	if l := strings.TrimSpace(r.FormValue("langs")); l != "" {
		have := s.ocrLangs(ctx)
		use := []string{}
		for _, c := range strings.Split(l, "+") {
			if c = ocrHas(have, c); c != "" && !oneOf(c, use) && len(use) < 6 {
				use = append(use, c)
			}
		}
		if len(use) > 0 {
			langs = strings.Join(use, "+")
		}
	}
	text, err := s.ocrImage(ctx, data, langs)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if text == "" {
		writeErr(w, http.StatusUnprocessableEntity, "no text was found in this photo; try again closer, straight on and in good light")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text, "langs": langs})
}

// formFile reads the multipart field "file", at most max bytes.
func (s *Server) formFile(w http.ResponseWriter, r *http.Request, max int64) ([]byte, string, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, max+1<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("send the file as the form field \"file\" (at most %d MB)", max>>20))
		return nil, "", "", false
	}
	f, fh, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no file in the form field \"file\"")
		return nil, "", "", false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "the file did not arrive whole")
		return nil, "", "", false
	}
	if int64(len(data)) > max {
		writeErr(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("the file is bigger than %d MB", max>>20))
		return nil, "", "", false
	}
	return data, filepath.Base(fh.Filename), fh.Header.Get("Content-Type"), true
}

// ---- PDFs: text, or for scans the pages read like photos ----

func (s *Server) pdfText(ctx context.Context, data []byte, st store.Settings, from string) (string, error) {
	path, err := exec.LookPath(s.cfg.PDFToText)
	if err != nil {
		return "", errors.New("PDFs need pdftotext on this server (poppler; it is in the Docker image)")
	}
	dir, err := os.MkdirTemp("", "ptl-pdf-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return "", err
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(cctx, path, "-enc", "UTF-8", in, "-").Output()
	if err != nil {
		return "", errors.New("this PDF could not be read (it may be locked with a password)")
	}
	text := string(out)
	if len(strings.Fields(text)) > 20 {
		return text, nil
	}
	// a scan: each page as a picture, read like a photo
	ppm, err := exec.LookPath(s.cfg.PDFToPPM)
	if err != nil {
		return text, nil
	}
	langs, err := s.ocrUse(ctx, st, from)
	if err != nil {
		if strings.TrimSpace(text) == "" {
			return "", errors.New("this PDF is a scan and reading scans needs tesseract on this server")
		}
		return text, nil
	}
	if err := exec.CommandContext(cctx, ppm, "-r", "200", "-png", "-l", "60", in, filepath.Join(dir, "p")).Run(); err != nil {
		return text, nil
	}
	pages, _ := filepath.Glob(filepath.Join(dir, "p*.png"))
	sort.Strings(pages)
	var b strings.Builder
	for i, p := range pages {
		img, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		t, err := s.ocrImage(ctx, img, langs)
		if err != nil {
			return "", err
		}
		if i > 0 {
			b.WriteString("\f")
		}
		b.WriteString(t)
	}
	return b.String(), nil
}

// ---- speech ----

// speechReady says whether what a person says can be written out.
func (s *Server) speechReady(ctx context.Context, u *store.User, st store.Settings) (bool, string) {
	if st.Speech == "openai" {
		if st.OpenAIKey == "" && (st.OpenAIBaseURL == "" || strings.Contains(st.OpenAIBaseURL, "api.openai.com")) {
			return false, "add your OpenAI API key under Settings → AI, or write out speech with Pocket Voice"
		}
		return true, ""
	}
	as := s.appStatus(ctx, u, st, appVoice)
	if as.Can(scopeTranscribe) {
		return true, ""
	}
	return false, firstNonEmpty(as.Detail, "Pocket Voice is not connected") + ". " + as.Fix
}

// transcribe writes out a recording in a language (app code; "" = any).
func (s *Server) transcribe(ctx context.Context, u *store.User, st store.Settings, data []byte, name, lang string) (string, string, error) {
	code := ""
	if lang != "" && lang != "auto" {
		code = engine.Base(lang)
		if i := strings.IndexByte(code, '-'); i > 0 {
			code = code[:i]
		}
		if code == "zt" {
			code = "zh"
		}
		if code == "pb" {
			code = "pt"
		}
	}
	if name == "" {
		name = "speech.webm"
	}
	var out struct {
		Text     string `json:"text"`
		Language string `json:"language"`
	}
	if st.Speech == "openai" {
		if err := s.openAISpeech(ctx, st, data, name, code, &out); err != nil {
			return "", "", err
		}
	} else {
		sib, err := s.clients(ctx, u.ID).conn(ctx, appVoice, scopeTranscribe)
		if err != nil {
			return "", "", &errNoEngine{err.Error()}
		}
		fields := map[string]string{}
		if code != "" {
			fields["language"] = code
		}
		if err := sib.client.Upload(ctx, http.MethodPost, "/api/v1/transcribe", nil, "file", name, data, fields, &out); err != nil {
			return "", "", fmt.Errorf("Pocket Voice: %w", err)
		}
	}
	text := strings.TrimSpace(out.Text)
	got := engine.Normalize(out.Language)
	if lang != "" && lang != "auto" {
		got = lang
		text = engine.Script(lang, text)
	}
	return text, got, nil
}

func (s *Server) openAISpeech(ctx context.Context, st store.Settings, data []byte, name, lang string, out any) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("model", firstNonEmpty(st.SpeechModel, "whisper-1"))
	if lang != "" {
		_ = mw.WriteField("language", lang)
	}
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		return err
	}
	_, _ = fw.Write(data)
	_ = mw.Close()
	base := strings.TrimRight(firstNonEmpty(st.OpenAIBaseURL, "https://api.openai.com/v1"), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/audio/transcriptions", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if st.OpenAIKey != "" {
		req.Header.Set("Authorization", "Bearer "+st.OpenAIKey)
	}
	res, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return fmt.Errorf("speech to text: %s", firstNonEmpty(e.Error.Message, res.Status))
	}
	return json.Unmarshal(raw, out)
}

// handleSpeech writes out a recording (dictation into the translator).
func (s *Server) handleSpeech(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	data, name, _, ok := s.formFile(w, r, 25<<20)
	if !ok {
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	text, lang, err := s.transcribe(ctx, u, st, data, name, engine.Normalize(r.FormValue("language")))
	if err != nil {
		s.writeTrErr(w, err)
		return
	}
	if text == "" {
		writeErr(w, http.StatusUnprocessableEntity, "nothing was heard; try again closer to the microphone")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text, "language": lang})
}
