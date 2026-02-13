package app

import (
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	t.Parallel()

	in := ansiBlue + "INFO" + ansiReset + " plain " + ansiRed + "ERR" + ansiReset
	got := stripANSI(in)
	want := "INFO plain ERR"
	if got != want {
		t.Fatalf("stripANSI()=%q want=%q", got, want)
	}
}

func TestWrapSegments_WrapsForNarrowWidth(t *testing.T) {
	t.Parallel()

	s1 := strings.Repeat("a", 20)
	s2 := strings.Repeat("b", 20)
	s3 := strings.Repeat("c", 20)

	lines := wrapSegments(
		[]string{s1, s2, s3},
		" | ",
		60,
		"-> ",
	)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d (%v)", len(lines), lines)
	}
	if lines[0] != s1+" | "+s2 {
		t.Fatalf("line[0]=%q want %q", lines[0], s1+" | "+s2)
	}
	if lines[1] != "-> "+s3 {
		t.Fatalf("line[1]=%q want %q", lines[1], "-> "+s3)
	}
}

func TestWrapSegments_TruncatesLongSegment(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", 80)

	lines := wrapSegments(
		[]string{long},
		" | ",
		60,
		"-> ",
	)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if visualLen(lines[0]) > 60 {
		t.Fatalf("line too wide: %q (visualLen=%d)", lines[0], visualLen(lines[0]))
	}
	if !strings.Contains(lines[0], "…") {
		t.Fatalf("expected truncation marker in %q", lines[0])
	}
}

func TestTerminalWidth_PrefersExplicitOverride(t *testing.T) {
	h := &prettyHandler{}

	t.Setenv("ARC_LOG_WIDTH", "88")
	t.Setenv("COLUMNS", "132")
	if got := h.terminalWidth(); got != 88 {
		t.Fatalf("terminalWidth()=%d want 88", got)
	}
}

func TestTerminalWidth_UsesColumnsWhenOverrideMissing(t *testing.T) {
	h := &prettyHandler{}

	t.Setenv("ARC_LOG_WIDTH", "")
	t.Setenv("COLUMNS", "72")
	if got := h.terminalWidth(); got != 72 {
		t.Fatalf("terminalWidth()=%d want 72", got)
	}
}

func TestTerminalWidth_FallbackDefault(t *testing.T) {
	h := &prettyHandler{}

	t.Setenv("ARC_LOG_WIDTH", "10")
	t.Setenv("COLUMNS", "20")
	if got := h.terminalWidth(); got != 100 {
		t.Fatalf("terminalWidth()=%d want 100", got)
	}
}
