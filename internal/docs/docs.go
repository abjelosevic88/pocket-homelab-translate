// Package docs takes documents apart into the pieces of text to translate
// and puts them back together with the translations, keeping everything
// else as it was: Word and PowerPoint formatting, Markdown markup,
// subtitle timings, HTML tags. PDFs come in as text (see pdf.go) and go
// out as a Word document.
package docs

import (
	"errors"
	"path/filepath"
	"strings"
)

// Doc is a document taken apart.
type Doc interface {
	// Pieces is the text to translate, in reading order.
	Pieces() []string
	// Build puts the document back together with one translation per piece.
	Build(translated []string) ([]byte, error)
	// Ext is the extension of what Build makes (".docx", ".srt"…).
	Ext() string
}

// Formats are what can be translated, with a plain description.
var Formats = []struct {
	Ext, Format, Label string
}{
	{".docx", "docx", "Word document"},
	{".pptx", "pptx", "PowerPoint presentation"},
	{".pdf", "pdf", "PDF (comes back as a Word document)"},
	{".txt", "txt", "Text"},
	{".md", "md", "Markdown"},
	{".srt", "srt", "Subtitles (SRT)"},
	{".vtt", "vtt", "Subtitles (WebVTT)"},
	{".html", "html", "Web page"},
	{".htm", "html", "Web page"},
	{".markdown", "md", "Markdown"},
}

// ErrUnsupported is returned for a file the app cannot translate.
var ErrUnsupported = errors.New("this kind of file cannot be translated here; use Word, PowerPoint, PDF, text, Markdown, subtitles or a web page")

// FormatOf says which format a file is, by its name (and type).
func FormatOf(name, mime string) string {
	ext := strings.ToLower(filepath.Ext(name))
	for _, f := range Formats {
		if f.Ext == ext {
			return f.Format
		}
	}
	switch {
	case strings.Contains(mime, "wordprocessingml"):
		return "docx"
	case strings.Contains(mime, "presentationml"):
		return "pptx"
	case mime == "application/pdf":
		return "pdf"
	case mime == "text/html":
		return "html"
	case mime == "text/markdown":
		return "md"
	case strings.HasPrefix(mime, "text/plain"):
		return "txt"
	}
	return ""
}

// Mime is the type of a result file.
func Mime(ext string) string {
	switch ext {
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".html":
		return "text/html; charset=utf-8"
	case ".md":
		return "text/markdown; charset=utf-8"
	case ".srt":
		return "application/x-subrip; charset=utf-8"
	case ".vtt":
		return "text/vtt; charset=utf-8"
	}
	return "text/plain; charset=utf-8"
}

// Open takes a document apart. lang is the BCP 47 tag of the language it
// is translated into (Word files are marked with it, for spelling).
func Open(format string, data []byte, lang string) (Doc, error) {
	switch format {
	case "docx":
		return openOOXML(data, "docx", lang)
	case "pptx":
		return openOOXML(data, "pptx", lang)
	case "txt":
		return openText(string(data)), nil
	case "md":
		return openMarkdown(string(data)), nil
	case "srt":
		return openSubs(string(data), false), nil
	case "vtt":
		return openSubs(string(data), true), nil
	case "html":
		return openHTML(data)
	}
	return nil, ErrUnsupported
}

// ResultName is the translated file's name: "report.docx" in German is
// "report (de).docx".
func ResultName(name, code, ext string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return base + " (" + code + ")" + ext
}

func check(pieces, translated []string) error {
	if len(pieces) != len(translated) {
		return errors.New("the translation does not match the document")
	}
	return nil
}

// stripBOM drops a leading byte-order mark.
func stripBOM(s string) string { return strings.TrimPrefix(s, "\ufeff") }
