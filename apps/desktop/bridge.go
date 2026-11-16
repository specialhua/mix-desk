package main

import (
	"context"
	"time"
)

// readyTimeout is how long after the admin's page has loaded it is shown,
// ready or not: should a page lack the sidebar buttons, or they be renamed,
// it shows the page as it is rather than the spinner on and on.
const readyTimeout = 8 * time.Second

// Bridge is what the web admin's page calls in Go, through the preload
// script below. Its pages may call it because the window trusts the
// server's origin (PageOptions.TrustedOrigins); keep it to what the script
// needs.
type Bridge struct{ a *app }

// Route tells the app the web admin's hash route, as "/posts?page=2", after
// every navigation, so the native sidebar follows the page.
func (b *Bridge) Route(ctx context.Context, route string) {
	if len(route) > 2048 {
		return
	}
	b.a.win.Update(func() { b.a.routeChanged(route) })
}

// Ready tells the app whether the web admin's page has drawn something to
// show: false as a page of it loads, true once the admin has rendered past
// its loading screens.
func (b *Bridge) Ready(ctx context.Context, ready bool) {
	b.a.win.Update(func() { b.a.webReady = ready })
}

// WriteStats tells the app what a write page shows: its word count
// (writing.go).
func (b *Bridge) WriteStats(ctx context.Context, stats writeStats) {
	b.a.win.Update(func() {
		if isWriteRoute(b.a.route) {
			b.a.writing.stats = stats
		}
	})
}

// headerCount is the count beside the title of the page at route.
type headerCount struct {
	Route string `json:"route"`
	Count string `json:"count"`
}

// HeaderCount tells the app the count beside the title of a page the native
// header names, which the page hides with its title: the breadcrumbs show
// it. One for a page the user has since left is dropped.
func (b *Bridge) HeaderCount(ctx context.Context, h headerCount) {
	if len(h.Count) > 16 {
		return
	}
	b.a.win.Update(func() {
		if h.Route == b.a.route {
			b.a.headerCount = h.Count
		}
	})
}

// ThemeUnsupported tells the app that the page lacks the CSS variables the
// color theme overrides, and shows its own colors (themes.go).
func (b *Bridge) ThemeUnsupported(ctx context.Context) {
	b.a.win.Update(b.a.themeUnsupportedNotice)
}

// pageModes are the modes the preload script styles the page for.
type pageModes struct {
	Focus      bool        `json:"focus"`
	Typewriter bool        `json:"typewriter"`
	Header     float32     `json:"header"`
	Theme      themeChoice `json:"theme"`
}

// Modes tells a page that just loaded the modes it shows.
func (b *Bridge) Modes(ctx context.Context) pageModes {
	ch := make(chan pageModes, 1)
	b.a.win.Update(func() {
		ch <- pageModes{b.a.writing.focus, b.a.set.Typewriter, b.a.writing.header, b.a.themeChoice()}
	})
	select {
	case m := <-ch:
		return m
	case <-ctx.Done():
		return pageModes{}
	}
}
