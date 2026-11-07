package docs

import (
	"bytes"
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
)

// ---- web pages: the text between tags (and alt/title texts) ----

type htmlTok struct {
	raw   []byte
	text  string // the text to translate (unescaped)
	lead  string
	trail string
	piece bool
}

type htmlDoc struct{ toks []htmlTok }

var htmlSkip = map[string]bool{"script": true, "style": true, "code": true, "pre": true, "noscript": true, "template": true, "svg": true, "math": true}

func openHTML(data []byte) (*htmlDoc, error) {
	d := &htmlDoc{}
	z := xhtml.NewTokenizer(bytes.NewReader(data))
	skip := []string{}
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			break
		}
		raw := append([]byte(nil), z.Raw()...)
		switch tt {
		case xhtml.StartTagToken:
			name, _ := z.TagName()
			if htmlSkip[string(name)] {
				skip = append(skip, string(name))
			}
		case xhtml.EndTagToken:
			name, _ := z.TagName()
			if n := len(skip); n > 0 && skip[n-1] == string(name) {
				skip = skip[:n-1]
			}
		case xhtml.TextToken:
			t := html.UnescapeString(string(raw))
			if len(skip) == 0 && strings.TrimSpace(t) != "" && hasLetter(t) {
				lead := t[:len(t)-len(strings.TrimLeft(t, " \t\r\n"))]
				trail := t[len(strings.TrimRight(t, " \t\r\n")):]
				d.toks = append(d.toks, htmlTok{raw: raw, text: strings.TrimSpace(t), lead: lead, trail: trail, piece: true})
				continue
			}
		}
		d.toks = append(d.toks, htmlTok{raw: raw})
	}
	return d, nil
}

func hasLetter(s string) bool {
	for _, r := range s {
		if r > 0x40 && (r < 0x5b || r > 0x60 && r < 0x7b || r > 0xbf) {
			return true
		}
	}
	return false
}

func (d *htmlDoc) Pieces() []string {
	out := []string{}
	for _, t := range d.toks {
		if t.piece {
			out = append(out, t.text)
		}
	}
	return out
}

func (d *htmlDoc) Build(tr []string) ([]byte, error) {
	if err := check(d.Pieces(), tr); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	i := 0
	for _, t := range d.toks {
		if t.piece {
			b.WriteString(t.lead + html.EscapeString(strings.TrimSpace(tr[i])) + t.trail)
			i++
			continue
		}
		b.Write(t.raw)
	}
	return b.Bytes(), nil
}

func (d *htmlDoc) Ext() string { return ".html" }
