package main

// Property, metamorphic and differential tests for the parts of the action
// that compute a layout or a number from their input: text measuring and
// truncation, count formatting, the star graph's scale and label placement,
// and the README rewrite. Inputs are drawn with rapid, which shrinks a failure
// to a small input. `go test` draws 100 per test; the nightly Property
// workflow passes -rapid.checks=20000.

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"pgregory.net/rapid"
)

// textGen draws text mixing ASCII words, CJK and emoji, as repository
// descriptions do.
func textGen() *rapid.Generator[string] {
	word := rapid.OneOf(
		rapid.StringMatching(`[A-Za-z0-9.,;:'()-]{1,12}`),
		rapid.StringMatching(`[ぁ-んァ-ン一-龥]{1,6}`),
		rapid.SampledFrom([]string{"🚀", "✨", "Go", "CLI", "SQLite"}),
	)
	return rapid.Custom(func(t *rapid.T) string {
		return strings.Join(rapid.SliceOfN(word, 0, 30).Draw(t, "words"), " ")
	})
}

func TestProperty_TruncateFitsAndKeepsAPrefix(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := textGen().Draw(t, "s")
		size := rapid.Float64Range(8, 24).Draw(t, "size")
		width := rapid.Float64Range(1, 600).Draw(t, "width")

		got := truncate(s, size, width)
		if got == s {
			if textWidth(s, size, false) > width {
				t.Fatalf("truncate kept %q, %.2f wide, over %.2f", s, textWidth(s, size, false), width)
			}
			return
		}
		if !strings.HasSuffix(got, ellipsis) {
			t.Fatalf("truncate(%q) = %q, which does not end with an ellipsis", s, got)
		}
		if got != ellipsis && textWidth(got, size, false) > width {
			t.Fatalf("truncate(%q, %.2f) = %q, %.2f wide", s, width, got, textWidth(got, size, false))
		}
		if head := strings.TrimSuffix(got, ellipsis); !strings.HasPrefix(s, head) {
			t.Fatalf("truncate(%q) = %q, not a prefix of the text", s, got)
		}
	})
}

func TestMetamorphic_TruncateNeverShortensWhenGivenMoreRoom(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := textGen().Draw(t, "s")
		size := rapid.Float64Range(8, 24).Draw(t, "size")
		width := rapid.Float64Range(1, 600).Draw(t, "width")
		more := width + rapid.Float64Range(0, 400).Draw(t, "more")

		narrow, wide := truncate(s, size, width), truncate(s, size, more)
		if utf8.RuneCountInString(strings.TrimSuffix(wide, ellipsis)) < utf8.RuneCountInString(strings.TrimSuffix(narrow, ellipsis)) {
			t.Fatalf("width %.2f gave %q but the wider %.2f gave the shorter %q", width, narrow, more, wide)
		}
	})
}

func TestMetamorphic_TextWidthAddsUpOverConcatenation(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		a, b := textGen().Draw(t, "a"), textGen().Draw(t, "b")
		size := rapid.Float64Range(8, 24).Draw(t, "size")
		bold := rapid.Bool().Draw(t, "bold")
		sum := textWidth(a, size, bold) + textWidth(b, size, bold)
		if got := textWidth(a+b, size, bold); math.Abs(got-sum) > 1e-9*math.Max(1, sum) {
			t.Fatalf("width(%q+%q) = %v, widths add to %v", a, b, got, sum)
		}
	})
}

func TestProperty_WrapTextKeepsTheWordsInOrder(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := textGen().Draw(t, "s")
		width := rapid.Float64Range(40, 600).Draw(t, "width")
		maxLines := rapid.IntRange(1, 4).Draw(t, "maxLines")

		lines := wrapText(s, descSize, width, maxLines)
		if len(lines) > maxLines {
			t.Fatalf("%d lines, at most %d allowed", len(lines), maxLines)
		}
		words := strings.Fields(s)
		var got []string
		for i, l := range lines {
			if textWidth(l, descSize, false) > width && l != ellipsis {
				t.Fatalf("line %d %q is wider than %.2f", i, l, width)
			}
			got = append(got, strings.Fields(strings.TrimSuffix(l, ellipsis))...)
		}
		// Every word shown is the next word of the text; only the last one
		// shown may be cut short by the ellipsis.
		for i, w := range got {
			cutLast := i == len(got)-1 && strings.HasPrefix(words[min(i, len(words)-1)], w)
			if i >= len(words) || (w != words[i] && !cutLast) {
				t.Fatalf("wrapText(%q) shows %q, not the words in order", s, got)
			}
		}
	})
}

// compactValue reads back the number compact printed, in stars.
func compactValue(t *rapid.T, s string) float64 {
	if strings.HasSuffix(s, "k") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "k"), 64)
		if err != nil {
			t.Fatalf("compact printed %q", s)
		}
		return f * 1000
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("compact printed %q", s)
	}
	return float64(n)
}

func TestProperty_CompactIsCloseToTheCountAndNeverGoesDown(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 10_000_000).Draw(t, "n")
		m := n + rapid.IntRange(0, 5000).Draw(t, "step")

		got := compactValue(t, compact(n))
		if n < 1000 && got != float64(n) {
			t.Fatalf("compact(%d) = %q, want the exact count", n, compact(n))
		}
		// 1.2k is within 50 of the count; 12k is the count's thousands.
		if n >= 1000 && n < 10000 && math.Abs(got-float64(n)) > 50 {
			t.Fatalf("compact(%d) = %q, more than 50 off", n, compact(n))
		}
		if n >= 10000 && (got > float64(n) || float64(n)-got >= 1000) {
			t.Fatalf("compact(%d) = %q, not the count's thousands", n, compact(n))
		}
		if compactValue(t, compact(m)) < got {
			t.Fatalf("compact(%d) = %q is less than compact(%d) = %q", m, compact(m), n, compact(n))
		}
	})
}

func TestDifferential_GroupedMatchesTheDigitsOfItoa(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, math.MaxInt).Draw(t, "n")
		got := grouped(n)
		if strings.ReplaceAll(got, ",", "") != strconv.Itoa(n) {
			t.Fatalf("grouped(%d) = %q, digits differ from %q", n, got, strconv.Itoa(n))
		}
		groups := strings.Split(got, ",")
		for i, g := range groups {
			if len(g) > 3 || len(g) == 0 || (i > 0 && len(g) != 3) {
				t.Fatalf("grouped(%d) = %q, bad group %q", n, got, g)
			}
		}
	})
}

func TestProperty_NiceScaleCoversTheMaximumWithARoundStep(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		maxStars := rapid.IntRange(0, 50_000_000).Draw(t, "max")
		step, top := niceScale(maxStars)
		if step <= 0 || top <= maxStars || top%step != 0 {
			t.Fatalf("niceScale(%d) = step %d, top %d", maxStars, step, top)
		}
		lead := step
		for lead >= 10 && lead%10 == 0 {
			lead /= 10
		}
		if lead != 1 && lead != 2 && lead != 5 {
			t.Fatalf("niceScale(%d) step %d is not 1, 2 or 5 times a power of ten", maxStars, step)
		}
		if intervals := top / step; intervals < 1 || intervals > 9 {
			t.Fatalf("niceScale(%d) gives %d intervals", maxStars, intervals)
		}
	})
}

func TestProperty_SpreadLabelsKeepsTheGapInsideTheArea(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		top := rapid.Float64Range(0, 100).Draw(t, "top")
		bottom := top + rapid.Float64Range(50, 400).Draw(t, "height")
		gap := rapid.Float64Range(1, 20).Draw(t, "gap")
		maxN := int((bottom-top)/gap) + 1
		ys := rapid.SliceOfN(rapid.Float64Range(top, bottom), 0, min(maxN, 12)).Draw(t, "ys")

		pos := spreadLabels(ys, top, bottom, gap)
		if len(pos) != len(ys) {
			t.Fatalf("%d positions for %d labels", len(pos), len(ys))
		}
		for i, p := range pos {
			if p < top-1e-9 || p > bottom+1e-9 {
				t.Fatalf("label %d at %v, outside [%v, %v]", i, p, top, bottom)
			}
		}
		sorted := slices.Clone(pos)
		slices.Sort(sorted)
		for i := 1; i < len(sorted); i++ {
			if sorted[i]-sorted[i-1] < gap-1e-9 {
				t.Fatalf("labels %v (from %v) closer than %v", pos, ys, gap)
			}
		}
	})
}

func TestMetamorphic_ReplacingBetweenMarkersTwiceIsReplacingOnce(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		before := rapid.StringMatching(`[^<]{0,40}`).Draw(t, "before")
		old := rapid.StringMatching(`[^<]{0,40}`).Draw(t, "old")
		after := rapid.StringMatching(`[^<]{0,40}`).Draw(t, "after")
		body := rapid.StringMatching(`[^<]{0,80}`).Draw(t, "body")
		doc := before + markerStart + old + markerEnd + after

		once, err := replaceBetweenMarkers(doc, body)
		if err != nil {
			t.Fatalf("replaceBetweenMarkers(%q): %v", doc, err)
		}
		twice, err := replaceBetweenMarkers(once, body)
		if err != nil || twice != once {
			t.Fatalf("second replace changed %q into %q (%v)", once, twice, err)
		}
		if !strings.HasPrefix(once, before+markerStart) || !strings.HasSuffix(once, markerEnd+after) {
			t.Fatalf("text outside the markers changed: %q", once)
		}
	})
}
