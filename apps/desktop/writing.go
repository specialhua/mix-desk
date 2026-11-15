package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// The writing features around the web admin's write pages
// (docs/writing-plan.md): the status bar under the editor, focus mode and
// typewriter scrolling. The bridge's preload script reads the page and
// reports writeStats; the app keeps the writing time.

// writeStats is what the preload script reads from a write page.
type writeStats struct {
	// Words counts CJK characters one each, and other words.
	Words int `json:"words"`
}

// now is time.Now, which tests replace.
var now = time.Now

// writing is the app's writing state.
type writing struct {
	stats writeStats
	// doc is the document the write page shows (writeDoc).
	doc string
	// session is its writing time since it was opened, and last when it
	// was last counted (tickWriting).
	session time.Duration
	last    time.Time
	// unsaved is the writing time of a new document with no id yet, which
	// goes to its draft once saved.
	unsaved time.Duration
	// times are the documents' writing times in all, kept in
	// writing-time.json; dirty when they changed since saved.
	times map[string]time.Duration
	dirty bool

	// header is the height a write page keeps clear at its top, under the
	// shell's header.
	header float32

	// menuOpen is whether the settings ball has spread its items, which
	// sit menuDX from it when laid out.
	menuOpen bool
	menuDX   map[string]float32

	focus bool
	// wasLeftOpen is whether the sidebar was open before focus mode.
	wasLeftOpen bool
}

// writingTick is how often the writing time is counted and redrawn.
const writingTick = 20 * time.Second

// setupWriting loads the writing times and starts their clock.
func (a *app) setupWriting() {
	a.writing.times = loadWritingTimes()
	go func() {
		for range time.Tick(writingTick) {
			a.win.Update(func() {
				visible := a.win.IsVisible() && !a.win.IsMinimized()
				a.tickWriting(visible)
				a.saveWritingTimes()
				if a.writing.doc != "" {
					a.win.Invalidate()
				}
			})
		}
	}()
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.tickWriting(true)
		a.saveWritingTimes()
	})
}

// tickWriting counts the time since the last tick in the open document's
// writing time, while the window shows: a tick that comes late (the Mac
// asleep) counts no more than two.
func (a *app) tickWriting(visible bool) {
	t := now()
	d := t.Sub(a.writing.last)
	a.writing.last = t
	if a.writing.doc == "" || !visible || d <= 0 {
		return
	}
	d = min(d, 2*writingTick)
	a.writing.session += d
	if docHasID(a.writing.doc) {
		a.addWritingTime(a.writing.doc, d)
	} else {
		a.writing.unsaved += d
	}
}

func (a *app) addWritingTime(doc string, d time.Duration) {
	if a.writing.times == nil {
		a.writing.times = map[string]time.Duration{}
	}
	a.writing.times[doc] += d
	a.writing.dirty = true
}

// writingTotal is the open document's writing time in all.
func (a *app) writingTotal() time.Duration {
	if !docHasID(a.writing.doc) {
		return a.writing.unsaved
	}
	return a.writing.times[a.writing.doc]
}

func writingTimesPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "writing-time.json"), nil
}

// loadWritingTimes reads writing-time.json: seconds by document.
func loadWritingTimes() map[string]time.Duration {
	times := map[string]time.Duration{}
	path, err := writingTimesPath()
	if err != nil {
		return times
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return times
	}
	var secs map[string]int64
	if json.Unmarshal(data, &secs) == nil {
		for k, v := range secs {
			times[k] = time.Duration(v) * time.Second
		}
	}
	return times
}

func (a *app) saveWritingTimes() {
	if !a.writing.dirty {
		return
	}
	path, err := writingTimesPath()
	if err != nil {
		return
	}
	secs := make(map[string]int64, len(a.writing.times))
	for k, v := range a.writing.times {
		secs[k] = int64(v / time.Second)
	}
	data, _ := json.MarshalIndent(secs, "", "  ")
	if os.WriteFile(path, data, 0o600) == nil {
		a.writing.dirty = false
	}
}

// isWriteRoute reports whether route is one of the web admin's write pages.
func isWriteRoute(route string) bool {
	p, _, _ := strings.Cut(route, "?")
	switch p {
	case "/posts/edit", "/notes/edit", "/pages/edit":
		return true
	}
	return false
}

// writeDoc names the document a write route shows, as the writing times
// keep it: "posts:a<id>" for an article, which keeps its id whatever the
// draft; "posts:d<draftId>" for a new one saved as a draft only; "posts:"
// for a new one not saved yet. Article and draft ids are snowflakes.
func writeDoc(route string) string {
	p, q, _ := strings.Cut(route, "?")
	kind := strings.TrimSuffix(strings.TrimPrefix(p, "/"), "/edit")
	v, _ := url.ParseQuery(q)
	if id := v.Get("id"); id != "" {
		return kind + ":a" + id
	}
	if id := v.Get("draftId"); id != "" {
		return kind + ":d" + id
	}
	return kind + ":"
}

// docHasID reports whether doc is saved, with an id to keep its time by.
func docHasID(doc string) bool {
	_, id, _ := strings.Cut(doc, ":")
	return id != ""
}

// writeRouteChanged follows the route into and out of the write pages.
func (a *app) writeRouteChanged(prev, route string) {
	if !isWriteRoute(route) {
		a.tickWriting(true)
		a.writing.stats = writeStats{}
		a.writing.doc = ""
		a.writing.session, a.writing.unsaved = 0, 0
		if a.writing.focus {
			a.setFocus(false)
		}
		return
	}
	doc := writeDoc(route)
	old := a.writing.doc
	if doc == old {
		return
	}
	a.tickWriting(true) // the time so far goes to the old document
	kind, id, _ := strings.Cut(doc, ":")
	oldKind, oldID, _ := strings.Cut(old, ":")
	switch {
	case old != "" && kind == oldKind && oldID == "" && id != "":
		// A new document saved for the first time: its time goes with it.
		a.addWritingTime(doc, a.writing.unsaved)
	case old != "" && kind == oldKind && strings.HasPrefix(oldID, "d") && strings.HasPrefix(id, "a"):
		// A new document published: its draft's time goes to the article.
		if d, ok := a.writing.times[old]; ok {
			a.addWritingTime(doc, d)
			delete(a.writing.times, old)
		}
	default:
		// Another document, or this one opened again: a new session.
		a.writing.session = 0
		a.writing.stats = writeStats{}
	}
	a.writing.unsaved = 0
	a.writing.doc = doc
}

// formatMinutes shows a duration in whole minutes: 4m, 20m, 1h 20m.
func formatMinutes(d time.Duration) string {
	m := int(d / time.Minute)
	if m < 60 {
		return strconv.Itoa(m) + "m"
	}
	if m%60 == 0 {
		return strconv.Itoa(m/60) + "h"
	}
	return fmt.Sprintf("%dh %dm", m/60, m%60)
}

// formatCount groups digits: 12,345.
func formatCount(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// onWritePage reports whether the main window shows a write page.
func (a *app) onWritePage() bool {
	return a.web != nil && a.page == pageMain && isWriteRoute(a.route)
}

// toggleFocus enters or leaves focus mode, on a write page.
func (a *app) toggleFocus() {
	if !a.writing.focus && !a.onWritePage() {
		return
	}
	a.setFocus(!a.writing.focus)
}

// setFocus enters focus mode, within the window: the sidebar closed and
// the write page without its header; or leaves it, restoring the sidebar.
func (a *app) setFocus(on bool) {
	if on == a.writing.focus {
		return
	}
	a.writing.focus = on
	if on {
		a.writing.wasLeftOpen = a.shell.LeftExpanded()
		a.shell.SetLeftExpanded(false)
	} else if a.writing.wasLeftOpen {
		a.shell.SetLeftExpanded(true)
	}
	a.applyPageModes()
	a.buildMenu()
	a.win.Invalidate()
}

// setTypewriter turns typewriter scrolling on or off, and keeps it.
func (a *app) setTypewriter(on bool) {
	a.set.Typewriter = on
	_ = a.set.save()
	a.applyPageModes()
	a.buildMenu()
	a.win.Invalidate()
	a.invalidatePrefs()
}

// applyPageModes tells the page the modes the preload script styles.
func (a *app) applyPageModes() {
	if a.web == nil {
		return
	}
	page := a.web.Page()
	js := fmt.Sprintf(`window.__mxDesktopModes && window.__mxDesktopModes(%t, %t, %g)`, a.writing.focus, a.set.Typewriter, a.writing.header)
	go page.Eval(js)
}

// glassHeight is the height of the glass capsules, in the header and the
// status bar.
const glassHeight = 34

// glassCapsule is a capsule of Liquid Glass. Over a web view, macOS 26
// and later draw it as the system's glass, seeing the page under it;
// elsewhere the glass plugin's shader draws it.
func glassCapsule(c *ui.Context) ui.Element {
	return ui.Row(c).Height(glassHeight).Shrink(0).Radius(glassHeight / 2).AlignItems(ui.Center).
		Material(glass.Glass{Native: true})
}

// sidebarToggleGlass puts glass under the shell's sidebar toggle while the
// sidebar is closed, when the toggle is over the page: where the shell's
// header places it (sidebar.Shell.build).
func (a *app) sidebarToggleGlass(c *ui.Context, header float32) {
	if a.shell == nil || a.shell.LeftExpanded() {
		return
	}
	bar := c.TitleBar()
	left := max(8, bar.Left)
	if runtime.GOOS == "darwin" && bar.Left > 0 {
		left += 16
	}
	size := max(32, c.Theme().Rem(2))
	pad := (glassHeight - size) / 2
	ui.Box(c).Key("toggle-glass").Absolute().Left(left-pad).Top((header-glassHeight)/2).
		Size(glassHeight, glassHeight).Radius(glassHeight / 2).PassThrough().
		Material(glass.Glass{Native: true})
}

// setPageHeader tells the page the header's height, which a write page
// keeps clear at its top.
func (a *app) setPageHeader(h float32) {
	if a.writing.header == h {
		return
	}
	a.writing.header = h
	a.applyPageModes()
}

// statusBarView floats over the bottom right of a write page: a ball of
// glass with the settings icon, which spreads into the word count and
// writing time, and the focus and typewriter toggles, and gathers them
// back in when clicked again.
func (a *app) statusBarView(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Key("status-bar").Absolute().Right(12).Bottom(10).Height(glassHeight).
		AlignItems(ui.Center).Gap(8).Children(func() {
		items := map[string]ui.Element{}
		if a.writing.menuOpen {
			// Left to right; they come out of the ball nearest first.
			items["stats"] = a.menuItem(c, "stats", 2, ui.Row(c).Padding(0, 14).Gap(14).TextColor(t.TextMuted).
				Material(glass.Glass{Native: true}), func() { a.statsView(c) })
			items["focus"] = a.menuToggle(c, "focus", 1, lucide.Maximize2(), a.tr("专注", "Focus"),
				a.tr("专注模式", "Focus Mode")+" ("+focusShortcutText()+")", a.writing.focus, a.toggleFocus)
			items["typewriter"] = a.menuToggle(c, "typewriter", 0, lucide.TextCursorInput(), a.tr("打字机", "Typewriter"),
				a.tr("打字机滚动：光标行停在略高于中间的位置", "Typewriter scrolling: keep the line a little above the middle"),
				a.set.Typewriter, func() { a.setTypewriter(!a.set.Typewriter) })
		}
		label := a.tr("写作设置", "Writing Settings")
		ball := ui.ButtonBase(c).Key("ball").Size(glassHeight, glassHeight).Radius(glassHeight / 2).Label(label).
			Material(glass.Glass{Native: true, Interactive: true})
		ball.Children(func() {
			ui.Row(c).Fill().Center().TextColor(t.TextMuted).Children(func() {
				var turn float32
				if a.writing.menuOpen {
					turn = 90
				}
				components.Icon(c, lucide.Settings2()).Size(16, 16).AnimatePose(ui.Pose{Rotate: turn}, ui.Snappy)
			})
		})
		components.Tooltip(c, ball, label)
		if ball.Clicked() {
			a.writing.menuOpen = !a.writing.menuOpen
		}
		// Where each item is from the ball, as laid out last, for the next
		// time they spread or gather.
		bx := ball.LayoutBounds().X
		for k, e := range items {
			if b := e.LayoutBounds(); b.W > 0 && bx > 0 {
				if a.writing.menuDX == nil {
					a.writing.menuDX = map[string]float32{}
				}
				a.writing.menuDX[k] = bx - b.X
			}
		}
	})
}

// menuGuess is where an item is from the ball before it has been laid out.
var menuGuess = map[string]float32{"typewriter": 110, "focus": 200, "stats": 380}

// menuItem keys e and has it spread from the ball and gather back into it.
// order is how many items are nearer the ball.
func (a *app) menuItem(c *ui.Context, key string, order int, e ui.Element, children func()) ui.Element {
	dx, ok := a.writing.menuDX[key]
	if !ok {
		dx = menuGuess[key]
	}
	m := &ui.Motion{X: dx, Scale: 0.4}
	anim := ui.Snappy
	anim.Delay = time.Duration(order) * 30 * time.Millisecond
	return e.Key(key).Height(glassHeight).Shrink(0).Radius(glassHeight / 2).AlignItems(ui.Center).
		Transition(ui.ElementTransition{Animation: &anim, Enter: m, Exit: m}).Children(children)
}

// menuToggle is a toggle on glass, in the accent color while on.
func (a *app) menuToggle(c *ui.Context, key string, order int, icon *ui.SVG, text, tip string, on bool, toggle func()) ui.Element {
	t := c.Theme()
	g, fg := glass.Glass{Native: true, Interactive: true}, t.Text
	if on {
		g.Tint, fg = t.Accent, t.AccentText
	}
	b := ui.ButtonBase(c).Padding(0, 14).Material(g).Label(tip)
	b = a.menuItem(c, key, order, b, func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).TextColor(fg).Children(func() {
			components.Icon(c, icon).Size(14, 14)
			ui.Text(c, text).FontSize(12)
		})
	})
	components.Tooltip(c, b, tip)
	if b.Clicked() {
		toggle()
	}
	return b
}

// statsView is the word count and the writing time.
func (a *app) statsView(c *ui.Context) {
	item := func(icon *ui.SVG, text, tip string) {
		row := ui.Row(c).Gap(5).AlignItems(ui.Center)
		row.Children(func() {
			components.Icon(c, icon).Size(13, 13)
			ui.Text(c, text).FontSize(12)
		})
		components.Tooltip(c, row, tip)
	}
	words := formatCount(a.writing.stats.Words)
	item(lucide.Type(), a.tr(words+" 字", words+" words"),
		a.tr("正文字数：中日韩文字每字计一，其他按词计", "Words in the body: CJK characters count one each"))
	session, total := formatMinutes(a.writing.session), formatMinutes(a.writingTotal())
	item(lucide.Timer(), a.tr("本次 "+session+" · 累计 "+total, session+" · "+total+" in all"),
		a.tr("写作时长：本次从打开这篇文章算起，累计包括以前每次打开的时间，只计窗口显示的时候", "Writing time: this session since the document was opened, and in all, counted while the window shows"))
}

// focusAccelerator is focus mode's shortcut, which neither the web admin
// nor its editor binds.
const focusAccelerator = "Shift+CmdOrCtrl+F"

func focusShortcutText() string {
	if runtime.GOOS == "darwin" {
		return "⇧⌘F"
	}
	return "Ctrl+Shift+F"
}
