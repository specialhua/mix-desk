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
//     lazy loading); or after readyTimeout, should neither come.
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
  const style = document.createElement("style");
  style.textContent = sidebarButtons + "{display:none!important}";
  (document.head || document.documentElement).appendChild(style);
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
})();`)
}
