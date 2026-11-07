package docs

import (
	"strings"
	"testing"
)

func upper(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToUpper(s)
	}
	return out
}

func roundTrip(t *testing.T, format, in string) string {
	t.Helper()
	d, err := Open(format, []byte(in), "de-DE")
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.Build(upper(d.Pieces()))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestText(t *testing.T) {
	got := roundTrip(t, "txt", "Hello there.\nSecond line.\n\n\nNext paragraph\n")
	if got != "HELLO THERE.\nSECOND LINE.\n\n\nNEXT PARAGRAPH\n" {
		t.Fatalf("%q", got)
	}
}

func TestMarkdown(t *testing.T) {
	in := "# Title\n\n- item one\n- [x] done\n\n```go\nfmt.Println(\"hi\")\n```\n\n| a | b |\n|---|---|\n| one | two |\n> quote"
	got := roundTrip(t, "md", in)
	want := "# TITLE\n\n- ITEM ONE\n- [x] DONE\n\n```go\nfmt.Println(\"hi\")\n```\n\n| A | B |\n|---|---|\n| ONE | TWO |\n> QUOTE"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSubtitles(t *testing.T) {
	in := "1\n00:00:01,000 --> 00:00:02,000\nHello\nthere\n\n2\n00:00:03,000 --> 00:00:04,000\nBye\n"
	got := roundTrip(t, "srt", in)
	if got != "1\n00:00:01,000 --> 00:00:02,000\nHELLO\nTHERE\n\n2\n00:00:03,000 --> 00:00:04,000\nBYE\n" {
		t.Fatalf("%q", got)
	}
	vtt := roundTrip(t, "vtt", "WEBVTT\n\n00:01.000 --> 00:02.000\nHi\n")
	if vtt != "WEBVTT\n\n00:01.000 --> 00:02.000\nHI\n" {
		t.Fatalf("%q", vtt)
	}
}

func TestHTML(t *testing.T) {
	got := roundTrip(t, "html", `<p class="x">Hello <b>big</b> world &amp; more</p><script>var a = "no";</script>`)
	if got != `<p class="x">HELLO <b>BIG</b> WORLD &amp; MORE</p><script>var a = "no";</script>` {
		t.Fatalf("%q", got)
	}
}

func TestDocx(t *testing.T) {
	src, err := NewDocx([]string{"First paragraph.", "\f", "Second & last"}, "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Open("docx", src, "sr-RS")
	if err != nil {
		t.Fatal(err)
	}
	if p := d.Pieces(); len(p) != 2 || p[1] != "Second & last" {
		t.Fatalf("pieces %q", p)
	}
	out, err := d.Build([]string{"Prvi pasus.", "Drugi & poslednji"})
	if err != nil {
		t.Fatal(err)
	}
	d2, err := Open("docx", out, "")
	if err != nil {
		t.Fatal(err)
	}
	if p := d2.Pieces(); len(p) != 2 || p[0] != "Prvi pasus." || p[1] != "Drugi & poslednji" {
		t.Fatalf("after %q", p)
	}
	if !strings.Contains(string(d2.(*ooxmlDoc).raw["word/styles.xml"]), `w:val="sr-RS"`) {
		t.Fatal("language not set")
	}
}

func TestRunsJoined(t *testing.T) {
	xml := []byte(`<w:body><w:p><w:r><w:t>Hel</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">lo world</w:t></w:r></w:p><w:p/></w:body>`)
	ps := scanParas(xml, "w:p", "w:t")
	if len(ps) != 1 || ps[0].text != "Hello world" || len(ps[0].runs) != 2 {
		t.Fatalf("%+v", ps)
	}
}

func TestPDFParas(t *testing.T) {
	got := PDFParas("Line one of a para-\ngraph goes on\n\nNext one\fPage two")
	want := []string{"Line one of a paragraph goes on", "Next one", "\f", "Page two"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%q", got)
	}
}

func TestFormatOf(t *testing.T) {
	if FormatOf("a.DOCX", "") != "docx" || FormatOf("x", "application/pdf") != "pdf" || FormatOf("x.zip", "") != "" {
		t.Fatal("FormatOf")
	}
	if ResultName("report.docx", "de", ".docx") != "report (de).docx" {
		t.Fatal("ResultName")
	}
}
