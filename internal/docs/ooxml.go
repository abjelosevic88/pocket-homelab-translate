package docs

import (
	"archive/zip"
	"bytes"
	"errors"
	"html"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
)

// ---- Word and PowerPoint ----
//
// A paragraph's text is spread over runs (each with its own formatting).
// The whole paragraph is translated as one piece (a sentence cut over
// runs cannot be translated run by run) and goes back into the paragraph's
// first run; the others are emptied. Paragraph and first-run formatting,
// tables, pictures, headers and footers all stay.

type ooxmlPart struct {
	name  string
	data  []byte
	paras []ooxmlPara
}

type ooxmlPara struct {
	runs [][2]int // byte ranges of the text inside <w:t>…</w:t>
	text string
}

type ooxmlDoc struct {
	files []*zip.File
	raw   map[string][]byte
	parts []*ooxmlPart
	kind  string
	lang  string
}

func openOOXML(data []byte, kind, lang string) (*ooxmlDoc, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("this file is not a valid " + strings.ToUpper(kind) + " (it does not open as one)")
	}
	d := &ooxmlDoc{files: zr.File, raw: map[string][]byte{}, kind: kind, lang: lang}
	para, text := "w:p", "w:t"
	match := regexp.MustCompile(`^word/(document|header\d*|footer\d*|footnotes|endnotes)\.xml$`)
	if kind == "pptx" {
		para, text = "a:p", "a:t"
		match = regexp.MustCompile(`^ppt/(slides/slide\d+|notesSlides/notesSlide\d+)\.xml$`)
	}
	for _, f := range zr.File {
		if f.UncompressedSize64 > 200<<20 {
			return nil, errors.New("a part of this file is too big")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 200<<20))
		rc.Close()
		if err != nil {
			return nil, err
		}
		d.raw[f.Name] = b
		if match.MatchString(f.Name) {
			d.parts = append(d.parts, &ooxmlPart{name: f.Name, data: b, paras: scanParas(b, para, text)})
		}
	}
	if len(d.parts) == 0 {
		return nil, errors.New("there is no text in this file")
	}
	sort.SliceStable(d.parts, func(i, j int) bool { return partOrder(d.parts[i].name) < partOrder(d.parts[j].name) })
	return d, nil
}

// partOrder keeps the body first and slides in their number order.
var digits = regexp.MustCompile(`\d+`)

func partOrder(name string) string {
	n := digits.FindString(path.Base(name))
	for len(n) < 5 {
		n = "0" + n
	}
	switch {
	case strings.Contains(name, "document.xml"), strings.Contains(name, "slides/"):
		return "0" + n
	case strings.Contains(name, "header"):
		return "1" + n
	case strings.Contains(name, "footer"):
		return "2" + n
	}
	return "3" + name + n
}

// scanParas finds each paragraph's text runs (paragraphs may nest, in
// text boxes: a run belongs to the innermost one).
func scanParas(b []byte, para, text string) []ooxmlPara {
	var paras []ooxmlPara
	stack := []int{}
	i := 0
	for i < len(b) {
		lt := bytes.IndexByte(b[i:], '<')
		if lt < 0 {
			break
		}
		i += lt
		gt := bytes.IndexByte(b[i:], '>')
		if gt < 0 {
			break
		}
		tag := b[i+1 : i+gt]
		end := i + gt + 1
		closing := len(tag) > 0 && tag[0] == '/'
		name := tag
		if closing {
			name = tag[1:]
		}
		if k := bytes.IndexAny(name, " \t\r\n/"); k >= 0 {
			name = name[:k]
		}
		self := len(tag) > 0 && tag[len(tag)-1] == '/'
		switch string(name) {
		case para:
			if closing {
				if n := len(stack); n > 0 {
					stack = stack[:n-1]
				}
			} else if !self {
				paras = append(paras, ooxmlPara{})
				stack = append(stack, len(paras)-1)
			}
		case text:
			if !closing && !self && len(stack) > 0 {
				closeTag := []byte("</" + text + ">")
				c := bytes.Index(b[end:], closeTag)
				if c < 0 {
					break
				}
				p := &paras[stack[len(stack)-1]]
				p.runs = append(p.runs, [2]int{end, end + c})
				p.text += html.UnescapeString(string(b[end : end+c]))
				i = end + c + len(closeTag)
				continue
			}
		}
		i = end
	}
	return paras
}

func (d *ooxmlDoc) Pieces() []string {
	out := []string{}
	for _, p := range d.parts {
		for _, para := range p.paras {
			if strings.TrimSpace(para.text) != "" && hasLetter(para.text) {
				out = append(out, para.text)
			}
		}
	}
	return out
}

var wLang = regexp.MustCompile(`(<w:lang\b[^>]*?\bw:val=")[^"]*(")`)
var aLang = regexp.MustCompile(`(<a:rPr\b[^>]*?\blang=")[^"]*(")`)

func (d *ooxmlDoc) Build(tr []string) ([]byte, error) {
	if err := check(d.Pieces(), tr); err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	k := 0
	for _, p := range d.parts {
		type edit struct {
			from, to int
			text     string
		}
		var edits []edit
		for _, para := range p.paras {
			if strings.TrimSpace(para.text) == "" || !hasLetter(para.text) {
				continue
			}
			t := strings.TrimSpace(strings.ReplaceAll(tr[k], "\n", " "))
			k++
			for j, r := range para.runs {
				if j == 0 {
					// keep the paragraph's own leading space (indents made with spaces)
					lead := para.text[:len(para.text)-len(strings.TrimLeft(para.text, " "))]
					edits = append(edits, edit{r[0], r[1], html.EscapeString(lead + t)})
				} else {
					edits = append(edits, edit{r[0], r[1], ""})
				}
			}
		}
		b := p.data
		var buf bytes.Buffer
		last := 0
		for _, e := range edits {
			buf.Write(b[last:e.from])
			buf.WriteString(e.text)
			last = e.to
		}
		buf.Write(b[last:])
		nb := buf.Bytes()
		if d.lang != "" {
			nb = d.setLang(nb)
		}
		// the first run's text may now start or end with a space
		nb = bytes.ReplaceAll(nb, []byte("<w:t>"), []byte(`<w:t xml:space="preserve">`))
		out[p.name] = nb
	}
	if d.kind == "docx" && d.lang != "" {
		if st, ok := d.raw["word/styles.xml"]; ok {
			out["word/styles.xml"] = d.setLang(st)
		}
	}
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	for _, f := range d.files {
		h := f.FileHeader
		h.Method = zip.Deflate
		if f.Name == "mimetype" {
			h.Method = zip.Store
		}
		w, err := zw.CreateHeader(&h)
		if err != nil {
			return nil, err
		}
		data, ok := out[f.Name]
		if !ok {
			data = d.raw[f.Name]
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return zb.Bytes(), nil
}

func (d *ooxmlDoc) setLang(b []byte) []byte {
	if d.kind == "docx" {
		return wLang.ReplaceAll(b, []byte("${1}"+d.lang+"${2}"))
	}
	return aLang.ReplaceAll(b, []byte("${1}"+d.lang+"${2}"))
}

func (d *ooxmlDoc) Ext() string { return "." + d.kind }

// ---- a new Word document from paragraphs (PDFs come back as one) ----

// NewDocx makes a plain Word document: one paragraph per item, a page
// break where an item is "\f".
func NewDocx(paras []string, lang string) ([]byte, error) {
	var body strings.Builder
	for _, p := range paras {
		if p == "\f" {
			body.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
			continue
		}
		body.WriteString(`<w:p>`)
		for i, line := range strings.Split(p, "\n") {
			body.WriteString(`<w:r>`)
			if i > 0 {
				body.WriteString(`<w:br/>`)
			}
			body.WriteString(`<w:t xml:space="preserve">` + html.EscapeString(line) + `</w:t></w:r>`)
		}
		body.WriteString(`</w:p>`)
	}
	if lang == "" {
		lang = "en-GB"
	}
	files := []struct{ name, data string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		{"word/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:cs="Calibri" w:eastAsia="Calibri"/><w:sz w:val="22"/><w:lang w:val="` + lang + `"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="160" w:line="276" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults></w:styles>`},
		{"word/document.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body.String() + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1418" w:right="1418" w:bottom="1418" w:left="1418" w:header="709" w:footer="709" w:gutter="0"/></w:sectPr></w:body></w:document>`},
	}
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.data)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return zb.Bytes(), nil
}

// PDFParas splits text taken out of a PDF (pages split by form feeds)
// into paragraphs, joining the lines of each and marking page breaks.
func PDFParas(text string) []string {
	out := []string{}
	pages := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\f")
	for pi, page := range pages {
		if strings.TrimSpace(page) == "" {
			continue
		}
		if pi > 0 && len(out) > 0 {
			out = append(out, "\f")
		}
		for _, blk := range regexp.MustCompile(`\n[ \t]*\n`).Split(page, -1) {
			lines := []string{}
			for _, l := range strings.Split(blk, "\n") {
				if l = strings.TrimSpace(l); l != "" {
					lines = append(lines, l)
				}
			}
			if len(lines) == 0 {
				continue
			}
			// a line broken with a hyphen goes back together
			joined := ""
			for i, l := range lines {
				switch {
				case i == 0:
					joined = l
				case strings.HasSuffix(joined, "-") && len(joined) > 1 && joined[len(joined)-2] != ' ':
					joined = joined[:len(joined)-1] + l
				default:
					joined += " " + l
				}
			}
			out = append(out, joined)
		}
	}
	return out
}

// pdfDoc is a PDF's text as a Word document.
type pdfDoc struct {
	paras  []string
	pieces []int
	lang   string
}

// OpenPDFText takes apart the text of a PDF.
func OpenPDFText(text, lang string) Doc {
	d := &pdfDoc{paras: PDFParas(text), lang: lang}
	for i, p := range d.paras {
		if p != "\f" && hasLetter(p) {
			d.pieces = append(d.pieces, i)
		}
	}
	return d
}

func (d *pdfDoc) Pieces() []string {
	out := make([]string, len(d.pieces))
	for i, p := range d.pieces {
		out[i] = d.paras[p]
	}
	return out
}

func (d *pdfDoc) Build(tr []string) ([]byte, error) {
	if err := check(d.Pieces(), tr); err != nil {
		return nil, err
	}
	paras := append([]string(nil), d.paras...)
	for i, p := range d.pieces {
		paras[p] = strings.TrimSpace(tr[i])
	}
	return NewDocx(paras, d.lang)
}

func (d *pdfDoc) Ext() string { return ".docx" }
