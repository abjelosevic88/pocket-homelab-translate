package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-translate/internal/docs"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/engine"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// ---- documents: queued, translated one at a time in the background ----

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListDocuments(ctx, actorID(ctx), queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "documents")
		return
	}
	formats := []map[string]string{}
	for _, f := range docs.Formats {
		formats = append(formats, map[string]string{"ext": f.Ext, "format": f.Format, "label": f.Label})
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": list, "formats": formats, "max_bytes": s.cfg.MaxDocumentBytes})
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, err := s.store.GetDocument(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "document")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// queueDocument checks and stores a document, then wakes the worker.
func (s *Server) queueDocument(ctx context.Context, uid int64, name, mime string, data []byte, from, to, eng, origin string) (*store.Document, error) {
	format := docs.FormatOf(name, mime)
	if format == "" {
		return nil, docs.ErrUnsupported
	}
	to = engine.Normalize(to)
	if to == "" || to == "auto" {
		return nil, errors.New("pick the language to translate into")
	}
	from = engine.Normalize(from)
	if from == "" {
		from = "auto"
	}
	if !oneOf(eng, []string{"auto", "ai", "offline"}) {
		eng = "auto"
	}
	if format != "pdf" {
		if _, err := docs.Open(format, data, ""); err != nil {
			return nil, err
		}
	}
	d := &store.Document{Name: clip(firstNonEmpty(name, "document"), 200), Format: format, Mime: mime, SourceLang: from, TargetLang: to, Engine: eng, Origin: origin}
	if err := s.store.AddDocument(ctx, uid, d, data); err != nil {
		return nil, err
	}
	select {
	case s.docWake <- struct{}{}:
	default:
	}
	return d, nil
}

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	data, name, mime, ok := s.formFile(w, r, s.cfg.MaxDocumentBytes)
	if !ok {
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	d, err := s.queueDocument(ctx, u.ID, name, mime, data, r.FormValue("from"), firstNonEmpty(r.FormValue("to"), st.TargetLang), firstNonEmpty(r.FormValue("engine"), st.Engine), "upload")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

// handleDocumentFrom takes a document from Pocket Files or Pocket Documents.
func (s *Server) handleDocumentFrom(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var in struct {
		App    string `json:"app"` // files | documents
		Ref    string `json:"ref"` // a path in Files, an id in Documents
		Name   string `json:"name"`
		From   string `json:"from"`
		To     string `json:"to"`
		Engine string `json:"engine"`
	}
	if err := decode(r, &in); err != nil || in.Ref == "" {
		writeErr(w, http.StatusBadRequest, "pick a document")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	c := s.clients(ctx, u.ID)
	var data []byte
	var mime, name, origin string
	switch in.App {
	case "files":
		sib, err := c.conn(ctx, appFiles, scopeFilesRead)
		if err != nil {
			s.appErr(w, appFiles, err)
			return
		}
		if data, mime, err = sib.client.Download(ctx, "/api/v1/fs/raw", url.Values{"space": {"home"}, "path": {in.Ref}}, s.cfg.MaxDocumentBytes); err != nil {
			s.appErr(w, appFiles, err)
			return
		}
		name, origin = path.Base(in.Ref), "files:"+in.Ref
	case "documents":
		sib, err := c.conn(ctx, appDocuments, scopeDocumentsRead)
		if err != nil {
			s.appErr(w, appDocuments, err)
			return
		}
		id, _ := strconv.ParseInt(in.Ref, 10, 64)
		if data, mime, err = sib.client.Download(ctx, fmt.Sprintf("/api/v1/documents/%d/download", id), url.Values{"original": {"true"}}, s.cfg.MaxDocumentBytes); err != nil {
			s.appErr(w, appDocuments, err)
			return
		}
		name, origin = firstNonEmpty(in.Name, "document-"+in.Ref), "documents:"+in.Ref
		if docs.FormatOf(name, "") == "" {
			if ext := extFor(mime); ext != "" {
				name += ext
			}
		}
	default:
		writeErr(w, http.StatusBadRequest, "the app is files or documents")
		return
	}
	d, err := s.queueDocument(ctx, u.ID, name, mime, data, in.From, firstNonEmpty(in.To, st.TargetLang), firstNonEmpty(in.Engine, st.Engine), origin)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func extFor(mime string) string {
	switch {
	case strings.Contains(mime, "pdf"):
		return ".pdf"
	case strings.Contains(mime, "wordprocessingml"):
		return ".docx"
	case strings.Contains(mime, "presentationml"):
		return ".pptx"
	case strings.HasPrefix(mime, "text/html"):
		return ".html"
	case strings.HasPrefix(mime, "text/"):
		return ".txt"
	}
	return ""
}

func (s *Server) appErr(w http.ResponseWriter, a pocketApp, err error) {
	if isNotConnected(err) {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeErr(w, http.StatusBadGateway, a.name+": "+err.Error())
}

func (s *Server) handleDocumentFile(which string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		uid := actorID(ctx)
		d, err := s.store.GetDocument(ctx, uid, idParam(r, "id"))
		if err != nil {
			s.fail(w, err, "document")
			return
		}
		b, err := s.store.DocumentFile(ctx, uid, d.ID, which)
		if err != nil {
			s.fail(w, err, "document")
			return
		}
		name, mime := d.Name, firstNonEmpty(d.Mime, "application/octet-stream")
		if which == "result" {
			name = d.ResultName
			mime = docs.Mime(path.Ext(name))
		}
		w.Header().Set("Content-Type", mime)
		disp := "attachment"
		if queryBool(r, "inline") {
			disp = "inline"
		}
		w.Header().Set("Content-Disposition", disp+`; filename*=UTF-8''`+url.PathEscape(name))
		w.Header().Set("Cache-Control", "private, no-cache")
		_, _ = w.Write(b)
	}
}

func (s *Server) handleRetryDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		To     string `json:"to"`
		Engine string `json:"engine"`
	}
	_ = decode(r, &in)
	to := engine.Normalize(in.To)
	if to == "auto" {
		to = ""
	}
	if in.Engine != "" && !oneOf(in.Engine, []string{"auto", "ai", "offline"}) {
		writeErr(w, http.StatusBadRequest, "the engine is auto, ai or offline")
		return
	}
	if err := s.store.RetryDocument(ctx, actorID(ctx), idParam(r, "id"), to, in.Engine); err != nil {
		s.fail(w, err, "retry")
		return
	}
	select {
	case s.docWake <- struct{}{}:
	default:
	}
	d, _ := s.store.GetDocument(ctx, actorID(ctx), idParam(r, "id"))
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteDocument(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete document")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSaveDocument puts the translated copy in Pocket Files or Documents.
func (s *Server) handleSaveDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	d, err := s.store.GetDocument(ctx, u.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "document")
		return
	}
	if d.Status != store.DocDone {
		writeErr(w, http.StatusConflict, "the translation is not done yet")
		return
	}
	var in struct {
		App    string `json:"app"`
		Folder string `json:"folder"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	data, err := s.store.DocumentFile(ctx, u.ID, d.ID, "result")
	if err != nil {
		s.fail(w, err, "document")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	c := s.clients(ctx, u.ID)
	var sv store.Saved
	switch in.App {
	case "files":
		sib, err := c.conn(ctx, appFiles, scopeFilesWrite)
		if err != nil {
			s.appErr(w, appFiles, err)
			return
		}
		dir := strings.Trim(firstNonEmpty(strings.TrimSpace(in.Folder), st.FilesFolder, "Translations"), "/")
		if strings.HasPrefix(d.Origin, "files:") && in.Folder == "" {
			dir = strings.Trim(path.Dir(strings.TrimPrefix(d.Origin, "files:")), "/.")
		}
		var e struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		q := url.Values{"space": {"home"}, "path": {dir}, "name": {d.ResultName}}
		if err := sib.client.PutRaw(ctx, "/api/v1/fs/upload", q, data, docs.Mime(path.Ext(d.ResultName)), &e); err != nil {
			s.appErr(w, appFiles, err)
			return
		}
		p := strings.TrimPrefix(firstNonEmpty(e.Path, dir+"/"+d.ResultName), "/")
		sv = store.Saved{App: "files", Title: p, URL: strings.TrimRight(sib.public, "/") + "/f/home/" + escapePath(path.Dir(p))}
	case "documents":
		sib, err := c.conn(ctx, appDocuments, scopeDocumentsWrite)
		if err != nil {
			s.appErr(w, appDocuments, err)
			return
		}
		var doc map[string]any
		fields := map[string]string{"title": strings.TrimSuffix(d.ResultName, path.Ext(d.ResultName))}
		if err := sib.client.Upload(ctx, http.MethodPost, "/api/v1/documents", nil, "document", d.ResultName, data, fields, &doc); err != nil {
			s.appErr(w, appDocuments, err)
			return
		}
		link := strings.TrimRight(sib.public, "/") + "/documents"
		if id := idOf(doc); id != "" {
			link = strings.TrimRight(sib.public, "/") + "/documents/" + id
		}
		sv = store.Saved{App: "documents", Title: fields["title"], URL: link}
	default:
		writeErr(w, http.StatusBadRequest, "the app is files or documents")
		return
	}
	_ = s.store.AddSaved(ctx, u.ID, d.ID, sv)
	writeJSON(w, http.StatusOK, sv)
}

func escapePath(p string) string {
	if p == "." || p == "" {
		return ""
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

// ---- the worker ----

func (s *Server) documentWorker(ctx context.Context) {
	for {
		d, err := s.store.NextDocument(ctx)
		if err == nil {
			s.runDocument(ctx, d)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-s.docWake:
		case <-time.After(time.Minute):
		}
	}
}

func (s *Server) runDocument(ctx context.Context, d *store.Document) {
	start := time.Now()
	err := s.translateDocument(ctx, d)
	if ctx.Err() != nil {
		return // stopping: it is queued again on the next start
	}
	if err != nil {
		d.Status, d.Error = store.DocFailed, err.Error()
		var ne *errNoEngine
		if errors.As(err, &ne) {
			d.Error = ne.msg
		}
		s.log.Warn("document translation failed", "id", d.ID, "err", err)
		_ = s.store.FinishDocument(context.WithoutCancel(ctx), d, nil)
		return
	}
	s.log.Info("document translated", "id", d.ID, "pieces", d.Total, "engine", d.Used, "took", time.Since(start).Round(time.Second).String())
	s.publish(ctx, d.UserID, "translate.document_done", d.ResultName+" is translated", map[string]any{"document_id": d.ID, "name": d.ResultName, "to": d.TargetLang, "url": s.cfg.BaseURL + "/documents"})
	if st, _ := s.store.GetSettings(ctx, d.UserID); st.NotifyDocs && st.NotifyURL != "" && time.Since(start) > 2*time.Minute {
		n := notify.New(st.NotifyURL, st.NotifyFormat)
		_ = n.Send(ctx, notify.Message{Title: "Translated: " + d.Name, Body: d.ResultName + " is ready to download.", Link: s.cfg.BaseURL + "/documents", Priority: "success", Tags: []string{"memo"}})
	}
}

func (s *Server) translateDocument(ctx context.Context, d *store.Document) error {
	data, err := s.store.DocumentFile(ctx, 0, d.ID, "original")
	if err != nil {
		return err
	}
	st, _ := s.store.GetSettings(ctx, d.UserID)
	lang, _ := engine.Get(d.TargetLang)
	var doc docs.Doc
	if d.Format == "pdf" {
		text, err := s.pdfText(ctx, data, st, d.SourceLang)
		if err != nil {
			return err
		}
		doc = docs.OpenPDFText(text, lang.Speech)
	} else if doc, err = docs.Open(d.Format, data, lang.Speech); err != nil {
		return err
	}
	pieces := doc.Pieces()
	if len(pieces) == 0 {
		return errors.New("there is no text to translate in this document")
	}
	d.Total = len(pieces)
	from := d.SourceLang
	if from == "auto" {
		sample := strings.Join(pieces[:min(len(pieces), 40)], "\n")
		if c, _, err := s.detect(ctx, d.UserID, st, sample); err == nil {
			from = c
		}
	}
	d.DetectedLang = from
	_ = s.store.DocumentProgress(ctx, d.ID, 0, d.Total, from)
	about := "part of a document called " + d.Name
	out := make([]string, 0, len(pieces))
	step := 30
	used := ""
	for i := 0; i < len(pieces); i += step {
		j := min(i+step, len(pieces))
		res, err := s.translate(ctx, d.UserID, st, trReq{Texts: pieces[i:j], From: from, To: d.TargetLang, Engine: d.Engine, Tone: st.Tone, Glossary: true, About: about})
		if err != nil {
			return err
		}
		if used == "" || res.Engine == "ai" {
			used = res.Engine
		}
		if res.Detected != "" && from == "auto" {
			from = res.Detected
			d.DetectedLang = from
		}
		out = append(out, res.Texts...)
		d.Done = len(out)
		_ = s.store.DocumentProgress(ctx, d.ID, d.Done, d.Total, "")
	}
	result, err := doc.Build(out)
	if err != nil {
		return err
	}
	d.Status, d.Used, d.Error = store.DocDone, used, ""
	d.ResultName = docs.ResultName(d.Name, d.TargetLang, doc.Ext())
	return s.store.FinishDocument(context.WithoutCancel(ctx), d, result)
}
