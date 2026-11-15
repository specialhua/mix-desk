package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestWriteRoutes(t *testing.T) {
	for route, want := range map[string]bool{
		"/posts/edit": true, "/posts/edit?id=1": true, "/notes/edit?draftId=d": true, "/pages/edit": true,
		"/posts": false, "/posts/editor": false, "/dashboard": false, "": false,
	} {
		if got := isWriteRoute(route); got != want {
			t.Errorf("isWriteRoute(%q) = %v", route, got)
		}
	}
	for route, want := range map[string]string{
		"/posts/edit?id=42&draftId=9": "posts:a42", // the article, whatever its draft
		"/notes/edit?draftId=9":       "notes:d9",
		"/pages/edit":                 "pages:",
	} {
		if got := writeDoc(route); got != want {
			t.Errorf("writeDoc(%q) = %q, want %q", route, got, want)
		}
	}
}

func TestFormat(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0: "0m", 59 * time.Second: "0m", 4 * time.Minute: "4m", 20*time.Minute + 50*time.Second: "20m",
		60 * time.Minute: "1h", 80 * time.Minute: "1h 20m",
	} {
		if got := formatMinutes(d); got != want {
			t.Errorf("formatMinutes(%v) = %q, want %q", d, got, want)
		}
	}
	if got := formatCount(1234567); got != "1,234,567" {
		t.Errorf("formatCount = %q", got)
	}
}

// clock replaces now for a test.
func clock(t *testing.T) *time.Time {
	at := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	now = func() time.Time { return at }
	t.Cleanup(func() { now = time.Now })
	return &at
}

func TestWritingTime(t *testing.T) {
	at := clock(t)
	a := &app{set: settings{Lang: "zh-CN"}}
	minutes := func(n int, visible bool) {
		for range n * 3 { // ticks of 20s
			*at = at.Add(writingTick)
			a.tickWriting(visible)
		}
	}
	a.tickWriting(true)
	a.routeChanged("/posts/edit")
	minutes(2, true)
	a.routeChanged("/posts/edit?draftId=d1") // saved: the time goes with it
	minutes(1, true)
	minutes(5, false) // the window hidden: not counted
	if a.writing.session != 3*time.Minute || a.writingTotal() != 3*time.Minute {
		t.Errorf("draft: session %v, total %v", a.writing.session, a.writingTotal())
	}
	a.routeChanged("/posts/edit?id=7&draftId=d1") // published: to the article
	minutes(1, true)
	if a.writing.session != 4*time.Minute || a.writingTotal() != 4*time.Minute {
		t.Errorf("published: session %v, total %v", a.writing.session, a.writingTotal())
	}
	if _, ok := a.writing.times["posts:dd1"]; ok {
		t.Error("the draft's time stayed behind")
	}
	a.routeChanged("/notes/edit?id=3")
	if a.writing.session != 0 || a.writingTotal() != 0 {
		t.Errorf("another document: %v, %v", a.writing.session, a.writingTotal())
	}
	// Opened again: the session from zero, the total goes on.
	a.routeChanged("/dashboard")
	a.routeChanged("/posts/edit?id=7")
	minutes(2, true)
	if a.writing.session != 2*time.Minute || a.writingTotal() != 6*time.Minute {
		t.Errorf("opened again: session %v, total %v", a.writing.session, a.writingTotal())
	}
	// A tick long late, as after sleep, counts two at most.
	*at = at.Add(time.Hour)
	a.tickWriting(true)
	if a.writingTotal() != 6*time.Minute+2*writingTick {
		t.Errorf("after sleep: %v", a.writingTotal())
	}
}

func TestStatusBar(t *testing.T) {
	at := clock(t)
	a := &app{set: settings{Lang: "zh-CN"}}
	a.routeChanged("/posts/edit?id=7")
	a.writing.stats = writeStats{Words: 1234}
	a.writing.session = 4*time.Minute + 20*time.Second
	a.writing.times = map[string]time.Duration{"posts:a7": 80 * time.Minute}
	_ = at
	tt := ui.NewTester(a.contentView, 900, 600)
	if tt.HasText("1,234 字") {
		t.Error("the stats show before the ball spreads")
	}
	a.writing.menuOpen = true
	tt = ui.NewTester(a.contentView, 900, 600)
	for _, want := range []string{"1,234 字", "本次 4m · 累计 1h 20m", "专注", "打字机"} {
		if !tt.HasText(want) {
			t.Errorf("status bar lacks %q: %q", want, tt.Texts())
		}
	}
	a.routeChanged("/posts")
	tt = ui.NewTester(a.contentView, 900, 600)
	if tt.HasText("1,234 字") {
		t.Errorf("status bar off the write pages: %q", tt.Texts())
	}
}

func TestPreloadWriting(t *testing.T) {
	s := preloadScript("https://a.example", "/proxy/qaqdmin", "zh-CN")
	for _, want := range []string{`"Bridge.WriteStats"`, `"Bridge.Modes"`, "__mxDesktopModes", "mx-desktop-focus", "mx-desktop-typewriter"} {
		if !strings.Contains(s, want) {
			t.Errorf("the script lacks %s", want)
		}
	}
}
