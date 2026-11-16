package main

import (
	"embed"
	"encoding/json"
	"strconv"
	"strings"
)

// inject/ holds everything the app adds to the web admin's page, which is
// the server's and which the app does not change otherwise:
//
//   - preload.js, the preload script below;
//   - page.css, its changes to the admin's UI: the sidebar buttons hidden,
//     the page titles the native header shows, the write pages;
//   - themes/<id>.css, the color themes (themes.go).
//
// They read the admin's DOM, storage keys and CSS variables, which README.md
// lists; an upstream change is adapted to here.
//
//go:embed inject/preload.js inject/page.css inject/themes/*.css
var injectFiles embed.FS

func injectFile(name string) string {
	b, err := injectFiles.ReadFile("inject/" + name)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// preloadScript runs before the web admin's own scripts, on its page
// (inject/preload.js). It
//   - collapses the web admin's sidebar, which the native one replaces, and
//     hides the buttons that would bring it back;
//   - lets the theme follow the app (prefers-color-scheme follows
//     mygo.Theme) and sets the locale the app uses;
//   - reports the hash route: React Router's HashRouter changes it with
//     history.pushState, which fires no hashchange;
//   - reports when the page is ready to show (Bridge.Ready): React has
//     replaced the #initial-loader in #app, and on a page that needs a
//     session, one of the two sidebar buttons is there, which only the
//     admin's page layout draws (past its session check and the page's
//     lazy loading); or after readyTimeout, should neither come;
//   - on the pages the native header names (navPaths), hides the page's own
//     title and keeps the rest of its header, its actions at the right (the
//     class mx-desktop-titled on <html>), and reports the count beside the
//     title (Bridge.HeaderCount);
//   - on the write pages, reports the word count (Bridge.WriteStats), styles
//     focus mode and keeps the caret's line in the middle for typewriter
//     scrolling (window.__mxDesktopModes, which applyPageModes calls). It
//     reads the selectors docs/writing-plan.md lists;
//   - recolors the page with the color theme, where its scope says
//     (themes.go): data-mx-theme on <html>, which the app changes through
//     window.__mxDesktopTheme.
func preloadScript(origin, adminPath, lang string, theme themeChoice) string {
	o, _ := json.Marshal(origin)
	p, _ := json.Marshal(adminPath)
	l, _ := json.Marshal(lang)
	th, _ := json.Marshal(theme)
	themes, _ := json.Marshal(themeCSS())
	nav, _ := json.Marshal(navPaths())
	css, _ := json.Marshal(injectFile("page.css"))
	timeout := strconv.FormatInt(readyTimeout.Milliseconds(), 10)
	return strings.NewReplacer("__ORIGIN__", string(o), "__PATH__", string(p), "__LANG__", string(l),
		"__READY_TIMEOUT__", timeout, "__THEME__", string(th), "__THEMES__", string(themes),
		"__NAV__", string(nav), "__CSS__", string(css)).Replace(injectFile("preload.js"))
}
