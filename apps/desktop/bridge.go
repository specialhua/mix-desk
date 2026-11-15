package main

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
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

// pageModes are the modes the preload script styles the page for.
type pageModes struct {
	Focus      bool    `json:"focus"`
	Typewriter bool    `json:"typewriter"`
	Header     float32 `json:"header"`
}

// Modes tells a page that just loaded the modes it shows.
func (b *Bridge) Modes(ctx context.Context) pageModes {
	ch := make(chan pageModes, 1)
	b.a.win.Update(func() { ch <- pageModes{b.a.writing.focus, b.a.set.Typewriter, b.a.writing.header} })
	select {
	case m := <-ch:
		return m
	case <-ctx.Done():
		return pageModes{}
	}
}

// preloadScript runs before the web admin's own scripts, on its page. It
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
//   - on the write pages, reports the word count (Bridge.WriteStats), styles
//     focus mode
//     and keeps the caret's line in the middle for typewriter scrolling
//     (window.__mxDesktopModes, which applyPageModes calls). It reads the
//     selectors docs/writing-plan.md lists.
//
// It reads the web admin's storage keys (apps/admin: SIDEBAR_COLLAPSED_STORAGE_KEY,
// the theme store's "theme-mode", UI_LOCALE_STORAGE_KEY), the ids #app and
// #initial-loader of its index.html and the aria-label of its two sidebar
// buttons, which an upstream change may rename.
func preloadScript(origin, adminPath, lang string) string {
	o, _ := json.Marshal(origin)
	p, _ := json.Marshal(adminPath)
	l, _ := json.Marshal(lang)
	timeout := strconv.FormatInt(readyTimeout.Milliseconds(), 10)
	return strings.NewReplacer("__ORIGIN__", string(o), "__PATH__", string(p), "__LANG__", string(l), "__READY_TIMEOUT__", timeout).Replace(`(() => {
  // The web admin's page only: not an error page or the API's pages.
  if (location.origin !== __ORIGIN__ || location.pathname.replace(/\/$/, "") !== __PATH__) return;
  try {
    localStorage.setItem("admin:sidebar-collapsed", "true");
    localStorage.removeItem("theme-mode");
    localStorage.setItem("mx-admin-ui-locale", __LANG__);
  } catch {}
  const sidebarButtons = [
    'button[aria-label="展开侧边栏"]', 'button[aria-label="Expand sidebar"]',
    'button[aria-label="打开导航"]', 'button[aria-label="Open navigation"]',
  ].join(",");
  // The script runs as the document is created: WebView2 (Windows) runs it
  // before there is an <html> element to add to, which WKWebView has.
  const addStyle = (css) => {
    const style = document.createElement("style");
    style.textContent = css;
    const put = () => (document.head || document.documentElement).appendChild(style);
    if (document.documentElement) return put();
    const wait = new MutationObserver(() => {
      if (document.documentElement) { wait.disconnect(); put(); }
    });
    wait.observe(document, {childList: true});
  };
  addStyle(sidebarButtons + "{display:none!important}");
  const call = (name, arg) => {
    try { window.mygo && window.mygo.call(name, arg); } catch {}
  };

  let last = null;
  const report = () => {
    const route = location.hash.replace(/^#/, "") || "/";
    if (route === last) return;
    last = route;
    call("Bridge.Route", route);
  };
  for (const name of ["pushState", "replaceState"]) {
    const original = history[name];
    history[name] = function (...args) {
      const result = original.apply(this, args);
      queueMicrotask(report);
      return result;
    };
  }
  addEventListener("hashchange", report);
  addEventListener("popstate", report);
  addEventListener("DOMContentLoaded", report);

  // As isPublicRoute in views.go: the pages shown without a session.
  const isPublic = (route) => /^\/(login|setup|collab)(?=$|[/?])/.test(route);
  let ready = false, observer = null;
  const markReady = () => {
    if (ready) return;
    ready = true;
    if (observer) observer.disconnect();
    // Once the frame with it has painted.
    requestAnimationFrame(() => requestAnimationFrame(() => call("Bridge.Ready", true)));
  };
  const checkReady = () => {
    const app = document.getElementById("app");
    if (!app || !app.firstElementChild || document.getElementById("initial-loader")) return;
    const route = location.hash.replace(/^#/, "") || "/";
    if (isPublic(route) || document.querySelector(sidebarButtons)) markReady();
  };
  addEventListener("DOMContentLoaded", () => {
    call("Bridge.Ready", false);
    observer = new MutationObserver(checkReady);
    observer.observe(document.documentElement, {childList: true, subtree: true, attributes: true, attributeFilter: ["aria-label"]});
    checkReady();
    setTimeout(markReady, __READY_TIMEOUT__);
  });

  // The write pages (writing.go).
  const isWrite = () => /^\/(posts|notes|pages)\/edit(?=$|[?])/.test(location.hash.replace(/^#/, ""));
  // The root editor, not one nested in a block such as a callout.
  const EDITOR = '[data-lexical-editor="true"]:not([data-lexical-editor] [data-lexical-editor]), .cm-content';
  addStyle([
    // The app's header floats over the top of a write page: keep it clear.
    "form:has([data-lexical-editor], .cm-content) > section{padding-top:var(--mx-desktop-header,0px)!important}",
    "html.mx-desktop-focus form:has([data-lexical-editor]) > section > div:first-child{display:none!important}",
    // The root editor only: callouts and other blocks nest editors of their own.
    "html.mx-desktop-typewriter [data-lexical-editor]:not([data-lexical-editor] [data-lexical-editor]){padding-bottom:50vh!important}",
  ].join("\n"));

  const CJK = /[\u3040-\u30ff\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\uac00-\ud7af]/g;
  const countWords = (text) => {
    const cjk = (text.match(CJK) || []).length;
    const words = (text.replace(CJK, " ").match(/[\p{L}\p{N}]+(?:['’.\-][\p{L}\p{N}]+)*/gu) || []).length;
    return cjk + words;
  };
  let lastStats = "", statsTimer = 0;
  const reportStats = () => {
    statsTimer = 0;
    if (!isWrite()) { lastStats = ""; return; }
    const editor = document.querySelector(EDITOR);
    const stats = {words: editor ? countWords(editor.innerText || "") : 0};
    const key = JSON.stringify(stats);
    if (key === lastStats) return;
    lastStats = key;
    call("Bridge.WriteStats", stats);
  };
  const scheduleStats = () => {
    if (!isWrite()) { lastStats = ""; return; }
    if (!statsTimer) statsTimer = setTimeout(reportStats, 400);
  };

  const typed = (e) => {
    if (typewriter && isWrite() && e.target instanceof Element && e.target.closest(EDITOR)) requestAnimationFrame(centerCaret);
  };

  // Typewriter scrolling: the caret's line a little above the middle of
  // the editor's scroll area, leaving room below it for the editor's menus
  // (the slash menu flips above the caret, under the toolbar, without it).
  const TYPEWRITER_LINE = 0.4;
  let typewriter = false;
  // The scroll area around the editor; never the document, which the
  // admin's layout keeps still.
  const scroller = (el) => {
    for (let n = el.parentElement; n && n !== document.body; n = n.parentElement) {
      if (/(auto|scroll)/.test(getComputedStyle(n).overflowY) && n.scrollHeight > n.clientHeight) return n;
    }
    return null;
  };
  const caretRect = (range) => {
    const rects = range.getClientRects();
    if (rects.length) return rects[rects.length - 1];
    const r = range.getBoundingClientRect();
    if (r.height) return r;
    const node = range.startContainer.nodeType === 1 ? range.startContainer : range.startContainer.parentElement;
    return node ? node.getBoundingClientRect() : null;
  };
  const centerCaret = () => {
    if (!typewriter || !isWrite()) return;
    const sel = getSelection();
    if (!sel || !sel.rangeCount || !sel.isCollapsed) return;
    const range = sel.getRangeAt(0);
    const node = range.startContainer.nodeType === 1 ? range.startContainer : range.startContainer.parentElement;
    const editor = node && node.closest('[data-lexical-editor="true"]');
    if (!editor) return;
    const rect = caretRect(range);
    const box = scroller(editor);
    if (!rect || !box) return;
    const delta = rect.top + rect.height / 2 - (box.getBoundingClientRect().top + box.clientHeight * TYPEWRITER_LINE);
    if (Math.abs(delta) > 2) box.scrollBy({top: delta});
  };

  window.__mxDesktopModes = (focus, tw, header) => {
    if (header) document.documentElement.style.setProperty("--mx-desktop-header", header + "px");
    const c = document.documentElement.classList;
    c.toggle("mx-desktop-focus", !!focus);
    c.toggle("mx-desktop-typewriter", !!tw);
    typewriter = !!tw;
    if (typewriter) requestAnimationFrame(centerCaret);
  };

  addEventListener("DOMContentLoaded", () => {
    try {
      window.mygo && window.mygo.call("Bridge.Modes").then((m) => m && window.__mxDesktopModes(m.focus, m.typewriter, m.header), () => {});
    } catch {}
    // The admin's layout fills the window and scrolls inside: a write page
    // scrolled as a whole (the editor bringing the caret into view through
    // every ancestor) shows a strip of nothing at the bottom. Keep it still.
    addEventListener("scroll", (e) => {
      const doc = document.scrollingElement;
      if (e.target === document && isWrite() && doc && doc.scrollTop) doc.scrollTop = 0;
    }, true);
    new MutationObserver(scheduleStats).observe(document.documentElement, {childList: true, subtree: true, characterData: true});
    addEventListener("input", typed, true);
    addEventListener("keyup", (e) => { if (typewriter && /^(Arrow|Page|Home|End|Enter)/.test(e.key)) requestAnimationFrame(centerCaret); }, true);
    document.addEventListener("selectionchange", () => { if (typewriter) requestAnimationFrame(centerCaret); });
    scheduleStats();
  });
})();`)
}
