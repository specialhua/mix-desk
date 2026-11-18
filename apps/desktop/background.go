package main

import (
	"runtime"

	"github.com/egoist/mygo"
)

// keepInBackground makes the main window's close button (and Cmd+W) hide
// the window instead of closing it, as macOS apps do: the app and its web
// view keep running, so the admin's WebSocket notifications still arrive,
// and clicking the Dock icon brings the window back as it was. Quit
// (Cmd+Q, the Dock's Quit, logging out) still closes it.
//
// Only on macOS, where the Dock icon stays to reopen the app; elsewhere the
// app has no tray icon, so closing the window quits as before.
func (a *app) keepInBackground() {
	if runtime.GOOS != "darwin" {
		return
	}
	win := a.win
	win.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		if win.IsFullScreen() {
			// Hiding a full-screen window leaves its empty Space behind:
			// leave full screen first, then hide.
			var off func()
			off = win.OnLeaveFullScreen(func() {
				off()
				win.Hide()
			})
			win.SetFullScreen(false)
			return
		}
		win.Hide()
	})
	// Clicking the Dock icon (or opening the app again) while the main
	// window is hidden, even with the settings window open: bring it back.
	mygo.App.OnActivate(func(bool) {
		if !win.IsVisible() || win.IsMinimized() {
			a.showMain()
		}
	})
	// The settings window closed while the main one is hidden: stay.
	mygo.App.OnWindowAllClosed(func() {})
}

// trackQuit keeps a.quitting. A quit closes the windows one by one and is
// canceled by the first that refuses, after those before it are gone: a
// window that hides instead of closing must let a quit close it, or the
// main window, closed first, is gone while the app stays, with nothing to
// bring back from the Dock.
func (a *app) trackQuit() {
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnQuitCanceled(func() { a.quitting = false })
}

// showMain shows and focuses the main window, hidden or minimized.
func (a *app) showMain() {
	if a.win == nil || a.win.IsDestroyed() {
		return
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	a.win.Focus()
}
