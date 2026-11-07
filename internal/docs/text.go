package docs

import (
	"regexp"
	"strings"
)

// ---- plain text: paragraphs, the blank lines between them kept ----

type textDoc struct {
	parts  []string // paragraphs and the whitespace between them, in order
	pieces []int    // indexes into parts that are text
}

func openText(s string) *textDoc {
	s = strings.ReplaceAll(stripBOM(s), "\r\n", "\n")
	d := &textDoc{}
	sep := regexp.MustCompile(`\n[ \t]*\n\s*`)
	last := 0
	for _, m := range sep.FindAllStringIndex(s, -1) {
		d.add(s[last:m[0]])
		d.parts = append(d.parts, s[m[0]:m[1]])
		last = m[1]
	}
	d.add(s[last:])
	return d
}

func (d *textDoc) add(p string) {
	if strings.TrimSpace(p) != "" {
		d.pieces = append(d.pieces, len(d.parts))
	}
	d.parts = append(d.parts, p)
}

func (d *textDoc) Pieces() []string {
	out := make([]string, len(d.pieces))
	for i, p := range d.pieces {
		out[i] = d.parts[p]
	}
	return out
}

func (d *textDoc) Build(tr []string) ([]byte, error) {
	if err := check(d.Pieces(), tr); err != nil {
		return nil, err
	}
	parts := append([]string(nil), d.parts...)
	for i, p := range d.pieces {
		parts[p] = tr[i]
	}
	return []byte(strings.Join(parts, "")), nil
}

func (d *textDoc) Ext() string { return ".txt" }

// ---- Markdown: the markup stays, code is not translated ----

type mdLine struct {
	prefix, text, suffix string
	piece                bool
}

type mdDoc struct{ lines []mdLine }

var (
	mdPrefix = regexp.MustCompile(`^(\s*(?:#{1,6}\s+|>\s*|[-*+]\s+(?:\[[ xX]\]\s+)?|\d+[.)]\s+)*)`)
	mdFence  = regexp.MustCompile("^\\s*(```|~~~)")
	mdRule   = regexp.MustCompile(`^\s*([-*_]\s*){3,}$`)
	mdTable  = regexp.MustCompile(`^\s*\|`)
	mdSepRow = regexp.MustCompile(`^\s*\|?\s*:?-{2,}`)
)

func openMarkdown(s string) *mdDoc {
	s = strings.ReplaceAll(stripBOM(s), "\r\n", "\n")
	d := &mdDoc{}
	fenced := false
	frontMatter := strings.HasPrefix(s, "---\n")
	for i, ln := range strings.Split(s, "\n") {
		switch {
		case frontMatter:
			d.lines = append(d.lines, mdLine{prefix: ln})
			if i > 0 && strings.TrimSpace(ln) == "---" {
				frontMatter = false
			}
		case mdFence.MatchString(ln):
			fenced = !fenced
			d.lines = append(d.lines, mdLine{prefix: ln})
		case fenced, strings.TrimSpace(ln) == "", mdRule.MatchString(ln), strings.HasPrefix(ln, "    "), strings.HasPrefix(ln, "\t"):
			d.lines = append(d.lines, mdLine{prefix: ln})
		case mdTable.MatchString(ln):
			if mdSepRow.MatchString(ln) {
				d.lines = append(d.lines, mdLine{prefix: ln}, mdLine{prefix: "\n"})
				continue
			}
			// each cell on its own
			cells := strings.Split(ln, "|")
			for j, c := range cells {
				if strings.TrimSpace(c) == "" {
					d.lines = append(d.lines, mdLine{prefix: c + bar(j, len(cells))})
					continue
				}
				lead := c[:len(c)-len(strings.TrimLeft(c, " "))]
				trail := c[len(strings.TrimRight(c, " ")):]
				d.lines = append(d.lines, mdLine{prefix: lead, text: strings.TrimSpace(c), suffix: trail + bar(j, len(cells)), piece: true})
			}
			d.lines = append(d.lines, mdLine{prefix: "\n"})
			continue
		default:
			p := mdPrefix.FindString(ln)
			d.lines = append(d.lines, mdLine{prefix: p, text: ln[len(p):], piece: true})
		}
		d.lines = append(d.lines, mdLine{prefix: "\n"})
	}
	if n := len(d.lines); n > 0 && d.lines[n-1].prefix == "\n" && !d.lines[n-1].piece {
		d.lines = d.lines[:n-1]
	}
	return d
}

func bar(i, n int) string {
	if i < n-1 {
		return "|"
	}
	return ""
}

func (d *mdDoc) Pieces() []string {
	out := []string{}
	for _, l := range d.lines {
		if l.piece {
			out = append(out, l.text)
		}
	}
	return out
}

func (d *mdDoc) Build(tr []string) ([]byte, error) {
	if err := check(d.Pieces(), tr); err != nil {
		return nil, err
	}
	var b strings.Builder
	i := 0
	for _, l := range d.lines {
		b.WriteString(l.prefix)
		if l.piece {
			b.WriteString(strings.ReplaceAll(strings.TrimSpace(tr[i]), "\n", " "))
			i++
		}
		b.WriteString(l.suffix)
	}
	return []byte(b.String()), nil
}

func (d *mdDoc) Ext() string { return ".md" }

// ---- subtitles: numbers and timings stay, lines are translated per cue ----

type subsDoc struct {
	blocks []subBlock
	vtt    bool
}

type subBlock struct {
	head string // number / id and timing line(s), or anything not a cue
	text string
	cue  bool
}

func openSubs(s string, vtt bool) *subsDoc {
	s = strings.ReplaceAll(stripBOM(s), "\r\n", "\n")
	d := &subsDoc{vtt: vtt}
	for _, blk := range regexp.MustCompile(`\n{2,}`).Split(strings.Trim(s, "\n"), -1) {
		lines := strings.Split(blk, "\n")
		t := -1
		for i, l := range lines {
			if strings.Contains(l, "-->") {
				t = i
				break
			}
		}
		if t < 0 || strings.HasPrefix(blk, "NOTE") || strings.HasPrefix(blk, "STYLE") {
			d.blocks = append(d.blocks, subBlock{head: blk})
			continue
		}
		text := strings.Join(lines[t+1:], "\n")
		d.blocks = append(d.blocks, subBlock{head: strings.Join(lines[:t+1], "\n"), text: text, cue: strings.TrimSpace(text) != ""})
	}
	return d
}

func (d *subsDoc) Pieces() []string {
	out := []string{}
	for _, b := range d.blocks {
		if b.cue {
			out = append(out, b.text)
		}
	}
	return out
}

func (d *subsDoc) Build(tr []string) ([]byte, error) {
	if err := check(d.Pieces(), tr); err != nil {
		return nil, err
	}
	parts := []string{}
	i := 0
	for _, b := range d.blocks {
		if !b.cue {
			if b.text != "" {
				parts = append(parts, b.head+"\n"+b.text)
			} else {
				parts = append(parts, b.head)
			}
			continue
		}
		parts = append(parts, b.head+"\n"+strings.TrimSpace(tr[i]))
		i++
	}
	return []byte(strings.Join(parts, "\n\n") + "\n"), nil
}

func (d *subsDoc) Ext() string {
	if d.vtt {
		return ".vtt"
	}
	return ".srt"
}
