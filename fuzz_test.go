package main

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"
)

// The descriptions, names, and release tags on a card come from GitHub and
// from the user's config, so any text must still give a well-formed SVG.
func FuzzRenderCard(f *testing.F) {
	f.Add("gup", "Update binaries installed with `go install`.", "Go", "#00ADD8", "v1.0.0", 622, true)
	f.Add(`a<b>&"c"`, "]]><script>x</script>", "", "", "", 0, false)
	f.Add("\x00\xff", "日本語の説明\u202e", "C++", "#f34b7d", "v1.0.0-rc.1+build", 1234567, true)
	f.Fuzz(func(t *testing.T, name, desc, lang, color, release string, stars int, featured bool) {
		for _, s := range []string{name, desc, lang, color, release} {
			if !utf8.ValidString(s) || strings.ContainsFunc(s, invalidXMLChar) {
				t.Skip("GitHub never returns text XML cannot carry")
			}
		}
		if stars < 0 {
			stars = -stars
		}
		d := CardData{Name: name, FullName: "o/" + name, Description: plainText(desc), Language: lang,
			LanguageColor: color, Stars: stars, Release: release, Featured: featured}
		for _, theme := range themes {
			mustBeXML(t, renderCard(d, theme))
		}
	})
}

func invalidXMLChar(r rune) bool {
	return r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r == 0xFFFE || r == 0xFFFF
}

func mustBeXML(t *testing.T, doc string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(doc))
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("not well-formed XML: %v\n%s", err, doc)
		}
	}
}

// Whatever the README holds, the text outside the markers must survive and a
// second replacement must change nothing.
func FuzzReplaceBetweenMarkers(f *testing.F) {
	f.Add("# Hi\n"+markerStart+"\nold\n"+markerEnd+"\nbye\n", "new\n")
	f.Add(markerEnd+markerStart, "x")
	f.Add(markerStart+markerStart+markerEnd, "")
	f.Fuzz(func(t *testing.T, doc, body string) {
		if strings.Contains(body, markerStart) || strings.Contains(body, markerEnd) {
			t.Skip("the rendered body never contains a marker")
		}
		got, err := replaceBetweenMarkers(doc, body)
		if err != nil {
			return
		}
		start := strings.Index(doc, markerStart)
		end := strings.Index(doc, markerEnd)
		if !strings.HasPrefix(got, doc[:start+len(markerStart)]) || !strings.HasSuffix(got, doc[end:]) {
			t.Fatalf("text outside the markers changed:\n doc %q\n got %q", doc, got)
		}
		if !strings.Contains(got, body) {
			t.Fatalf("body missing: %q", got)
		}
		again, err := replaceBetweenMarkers(got, body)
		if err != nil || again != got {
			t.Fatalf("second replacement changed the document: %q, %v", again, err)
		}
	})
}

// A config may hold anything; parsing must not panic, and an accepted config
// must name every repository validly.
func FuzzParseConfig(f *testing.F) {
	f.Add([]byte("sections:\n  - groups:\n      - title: T\n        repos:\n          - name: a\n            featured: true\n"))
	f.Add([]byte(`{"owner":"me","starHistory":{"minStars":-1},"sections":[]}`))
	f.Add([]byte("sections: [{groups: [{title: T, repos: [{name: a/b/c}]}]}]"))
	f.Add([]byte("*a"))
	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, err := parseConfig(data, "owner")
		if err != nil {
			return
		}
		if len(cfg.Repos()) == 0 || cfg.Owner == "" {
			t.Fatalf("accepted an empty config: %+v", cfg)
		}
		for _, r := range cfg.Repos() {
			owner, name := r.OwnerAndName(cfg.Owner)
			if owner == "" || name == "" || strings.Contains(name, "/") {
				t.Fatalf("accepted repository name %q", r.Name)
			}
		}
	})
}

// Wrapping never widens past the limit unless a single character is already
// wider, and never returns more lines than asked for.
func FuzzWrapText(f *testing.F) {
	f.Add("Load CSV, TSV, JSON, and Parquet files into SQLite.", 360.0, 2)
	f.Add("日本語の長い説明文が入ります", 50.0, 1)
	f.Add("", 10.0, 3)
	f.Fuzz(func(t *testing.T, s string, width float64, maxLines int) {
		if !utf8.ValidString(s) || width < 20 || width > 2000 || maxLines < 1 || maxLines > 5 {
			t.Skip()
		}
		lines := wrapText(s, descSize, width, maxLines)
		if len(lines) > maxLines {
			t.Fatalf("%d lines, max %d", len(lines), maxLines)
		}
		for _, l := range lines {
			if w := textWidth(l, descSize, false); w > width && utf8.RuneCountInString(l) > 1 {
				t.Fatalf("line %q is %.1fpx, wider than %.1fpx", l, w, width)
			}
		}
	})
}
