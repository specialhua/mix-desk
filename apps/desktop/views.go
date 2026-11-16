package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

// The pages the window shows in place of the sidebar and the web admin.
const (
	pageMain       = ""           // the shell's sidebar beside the web admin
	pageSetup      = "setup"      // ask for the server
	pageConnecting = "connecting" // the server answered, the web view is coming
	pageError      = "error"      // the web admin's page did not load
	pageLogin      = "login"      // the native sign-in
	pageWeb        = "web"        // a page of the web admin without its shell
)

// pageKind is the page the state asks for.
func (a *app) pageKind() string {
	switch {
	case a.srv.Root == "" && a.probing && a.set.Server != "":
		// Reconnecting to the saved server at launch: not the setup form.
		return pageConnecting
	case a.srv.Root == "":
		return pageSetup
	case a.web == nil:
		return pageConnecting
	case a.loadErr != "":
		return pageError
	case strings.HasPrefix(a.route, "/login") && !a.login.password:
		return pageLogin
	case a.route == "" && !a.webReady, isPublicRoute(a.route):
		// Loading, or a page of the web admin without its shell (the
		// password form, setup): the page alone. A page that never
		// reported its route (the preload script failing) gets the shell
		// once it shows, rather than none at all.
		return pageWeb
	}
	return pageMain
}

// beginFrame runs first in every frame of the main window, as the shell's
// Theme hook, before the shell reads its page: the page follows the state
// in the same frame that changed it.
func (a *app) beginFrame(c *ui.Context) *ui.Theme {
	if kind := a.pageKind(); kind != a.page {
		a.page = kind
		if kind == pageMain {
			a.shell.SetPage(nil)
		} else {
			a.shell.SetPage(a.pageView)
		}
	}
	return a.frameTheme(c)
}

// pageView builds the pages that replace the sidebar and the web admin.
func (a *app) pageView(c *ui.Context) {
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).AlignItems(ui.Stretch).Children(func() {
		switch a.pageKind() {
		case pageSetup:
			a.setupView(c)
		case pageConnecting:
			a.loadingView(c)
		case pageError:
			a.loadErrorView(c)
		case pageLogin:
			a.loginView(c)
		default:
			if a.login.password {
				ui.Row(c).Padding(0, 12, 8, 12).Children(func() {
					back := components.Button(c, "", components.ButtonGhost).Label(a.tr("返回", "Back"))
					back.Children(func() {
						ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
							components.Icon(c, lucide.ChevronLeft()).Size(16, 16)
							ui.Text(c, a.tr("返回", "Back"))
						})
					})
					if back.Clicked() {
						a.login.password = false
					}
				})
			}
			if a.web != nil {
				a.webView(c).Grow(1)
			}
		}
	})
	a.flushToasts(c)
}

// loadingView is the spinner shown while the web admin opens.
func (a *app) loadingView(c *ui.Context) {
	ui.Column(c).Fill().Center().Background(c.Theme().Background).Children(func() {
		components.Spinner(c, components.SpinnerRing).Label(a.tr("正在打开后台…", "Opening the admin…"))
	})
}

// webView is the web admin's page, under the loading spinner until it is
// ready to show: the same spinner as while connecting, so that one gives
// way to the other without a blank page between them.
func (a *app) webView(c *ui.Context) ui.Element {
	e := ui.WebView(c, a.web)
	if !a.webReady {
		e.Children(func() { a.loadingView(c) })
	}
	return e
}

// contentView is the web admin beside the shell's sidebar, below its
// header, with the command palette over it.
func (a *app) contentView(c *ui.Context) {
	header := sidebar.HeaderHeight(c)
	// A write page runs under the header and the status bar, which float
	// over it in Liquid Glass (writing.go); the page keeps the header's
	// height clear at its top itself. Other pages start below the header.
	// A page of the extras tab shows in place of the web admin, whose web
	// view hides, keeping its page, while no frame builds it.
	extra := a.extraPage != ""
	writePage := !extra && isWriteRoute(a.route)
	col := ui.Column(c).Fill().AlignItems(ui.Stretch)
	if !writePage {
		col.Padding(header, 0, 0, 0)
	}
	col.Children(func() {
		if extra {
			a.extraView(c)
		} else if a.web != nil {
			a.webView(c).Grow(1).MinWidth(0)
		} else {
			ui.Box(c).Grow(1) // tests draw the view without a web view
		}
	})
	if writePage {
		a.setPageHeader(header)
		a.sidebarToggleGlass(c, header)
		a.statusBarView(c)
	}

	// Not a menu accelerator: the admin's editor binds Cmd+K itself, and the
	// page keeps the keys it handles.
	if c.Shortcut(ui.Cmd, ui.KeyK) && !c.ShortcutRepeated() {
		a.openPalette()
	}
	a.paletteView(c)
	a.flushToasts(c)
}

// contentTitle shows where the web admin is, in the shell's header: the
// sidebar items down to the chosen one, whose ancestors go back up.
func (a *app) contentTitle(c *ui.Context) {
	ui.Row(c).Fill().AlignItems(ui.Center).Children(func() {
		if it := a.extraItem(); it != nil {
			glassCapsule(c).Padding(0, 14).Children(func() {
				titles := []string{a.tr("附加", "Extras"), a.tr(it.Title, it.TitleEN)}
				chosen := 1
				components.Breadcrumbs(c, titles, &chosen)
			})
			return
		}
		if len(navTrail(a.nav)) > 0 {
			glassCapsule(c).Padding(0, 14).Children(func() { a.breadcrumbs(c) })
		}
	})
}

// breadcrumbs are the sidebar items down to the chosen one.
func (a *app) breadcrumbs(c *ui.Context) {
	trail := navTrail(a.nav)
	if len(trail) == 0 {
		return
	}
	titles := make([]string, len(trail))
	for i, it := range trail {
		titles[i] = a.tr(it.Title, it.TitleEN)
	}
	chosen := len(trail) - 1
	if components.Breadcrumbs(c, titles, &chosen).Changed() && chosen < len(trail)-1 {
		a.nav = trail[chosen].Path
		a.navigate(trail[chosen].Path)
	}
	if a.headerCount != "" {
		ui.Text(c, a.headerCount).Key("header-count").Padding(0, 0, 0, 8).
			FontSize(c.Theme().FontSize * 0.85).TextColor(c.Theme().TextMuted)
	}
}

// sidebarView is the shell's sidebar: the web admin's pages.
func (a *app) sidebarView(c *ui.Context) {
	t := c.Theme()
	// The scroll starts below the shell's header, which holds the window's
	// buttons and the sidebar toggle: items scroll out of sight at its edge
	// rather than under them.
	// A global theme tints the sidebar's material (themes.go).
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Background(a.sidebarTint(c)).Children(func() {
		tab := a.set.SidebarTab
		if tab != sideTabExtras {
			tab = sideTabBlog
		}
		ui.Row(c).FillWidth().Padding(0, t.Space(3), t.Space(2), t.Space(3)).Children(func() {
			tabs := components.Tabs(c, &tab, components.TabsSegmented, a.tr("博客", "Blog"), a.tr("附加", "Extras"))
			if tabs.List.Changed() {
				a.set.SidebarTab = tab
				_ = a.set.save()
			}
		})
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			components.Sidebar(c).Padding(0, t.Space(3), t.Space(3), t.Space(3)).Children(func() {
				if tab == sideTabExtras {
					a.extrasView(c)
					return
				}
				for _, s := range navSections {
					items := func() {
						for _, it := range s.Items {
							a.sidebarItem(c, it)
						}
					}
					if s.Title == "" {
						ui.Column(c).FillWidth().MinWidth(0).Gap(t.Space(1)).Children(items)
						continue
					}
					components.SidebarSection(c, a.tr(s.Title, s.TitleEN), items)
				}
			})
		})
	})
}

// extrasView is the sidebar's 附加 (extras) tab: the app's own features,
// which run when clicked rather than show a page of the web admin.
func (a *app) extrasView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().MinWidth(0).Gap(t.Space(1)).Children(func() {
		for _, it := range extraItems {
			if components.SidebarItem(c, a.tr(it.Title, it.TitleEN), it.Icon(), a.extraPage == it.ID).Key("extra:" + it.ID).Clicked() {
				it.Run(a)
			}
			if it.ID == extraIdeas {
				// The shortcut that adds to it, from any app.
				if acc := a.ideaShortcut(); acc != "" {
					ui.Text(c, a.tr("随时记下：", "Jot down anytime: ")+acceleratorText(acc)).
						FontSize(11).TextColor(t.TextMuted).Padding(0, 0, t.Space(1), 34)
				}
			}
		}
	})
}

// extraItem is the item of the extras page shown, or nil.
func (a *app) extraItem() *extraItem {
	for k := range extraItems {
		if extraItems[k].ID == a.extraPage {
			return &extraItems[k]
		}
	}
	return nil
}

// extraView is the page of the extras tab shown.
func (a *app) extraView(c *ui.Context) {
	switch a.extraPage {
	case extraIdeas:
		a.ideasView(c)
	}
}

// sidebarItem shows an item and, while it or one of them is chosen, its
// children, as the web sidebar expands the active branch.
func (a *app) sidebarItem(c *ui.Context, it navItem) {
	e := components.SidebarItem(c, a.tr(it.Title, it.TitleEN), navIcon(it.Icon), a.extraPage == "" && a.nav == it.Path).Key(it.Path)
	if e.Clicked() && (a.nav != it.Path || a.extraPage != "") {
		a.nav = it.Path
		a.navigate(it.Path)
	}
	if len(it.Children) == 0 || !(a.nav == it.Path || strings.HasPrefix(a.nav, it.Path+"/")) {
		return
	}
	// Indent the children with the padding of their column, not a margin of
	// their own, which would push them past the sidebar's edge.
	t := c.Theme()
	ui.Column(c).Key(it.Path+"/children").FillWidth().MinWidth(0).Gap(t.Space(1)).Padding(0, 0, 0, 14).Children(func() {
		for _, child := range it.Children {
			a.sidebarItem(c, child)
		}
	})
}

// sidebarToolbar is the bar under the sidebar.
func (a *app) sidebarToolbar(c *ui.Context) {
	// The sidebar's tint. An absolute element is placed in its parent's
	// padding box, which holds the toolbar's padding, and is above its
	// siblings: under the buttons instead, and letting the pointer through.
	if tint := a.sidebarTint(c); tint != ui.Transparent {
		ui.Box(c).Key("tint").Absolute().Left(0).Top(0).Right(0).Bottom(0).ZIndex(-1).PassThrough().Background(tint)
	}
	if sidebar.IconButton(c, lucide.Search(), a.tr("前往页面或操作", "Go to a page or action")+" ("+shortcutText("K")+")").Clicked() {
		a.openPalette()
	}
	ui.Spacer(c)
	if sidebar.IconButton(c, lucide.ExternalLink(), a.tr("打开主站", "Open Main Site")).Clicked() {
		a.openMainSite()
	}
	if sidebar.IconButton(c, lucide.Settings(), a.tr("设置", "Settings")+" ("+shortcutText(",")+")").Clicked() {
		a.openPrefs()
	}
}

// loadErrorView shows why the web admin did not load, with a retry.
func (a *app) loadErrorView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Children(func() {
		ui.Column(c).Width(400).Gap(12).Children(func() {
			ui.Text(c, a.tr("无法打开后台", "Cannot open the admin")).FontSize(20).Bold()
			ui.Text(c, a.loadErr).TextColor(t.TextMuted).Selectable()
			ui.Row(c).Gap(8).Children(func() {
				if components.Button(c, a.tr("重试", "Retry"), components.ButtonPrimary).Clicked() {
					a.loadErr = ""
					a.web.Page().Reload()
				}
				if components.Button(c, a.tr("更换服务器…", "Change Server…"), components.ButtonOutline).Clicked() {
					a.changeServer()
				}
			})
		})
	})
}

// setupView asks for the server.
func (a *app) setupView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Children(func() {
		ui.Column(c).Width(400).Gap(14).Children(func() {
			ui.Text(c, a.tr("连接你的空间", "Connect your space")).FontSize(24).Bold()
			ui.Text(c, a.tr("填入 Mix Space 服务地址，例如 https://server.example.com。", "Enter your Mix Space server, e.g. https://server.example.com.")).TextColor(t.TextMuted)
			input := components.TextBox(c, &a.serverInput, components.TextBoxOptions{LeadingIcon: lucide.Server()}).
				Placeholder("https://").Label(a.tr("服务地址", "Server")).Disabled(a.probing).AutoFocus()
			empty := strings.TrimSpace(a.serverInput) == ""
			connect := components.Button(c, a.tr("连接", "Connect"), components.ButtonPrimary).Disabled(a.probing || empty)
			if (connect.Clicked() || input.Submitted()) && !a.probing && !empty {
				a.connect(a.serverInput, true)
			}
			if a.probing {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					components.Spinner(c, components.SpinnerRing).Size(16, 16)
					ui.Text(c, a.tr("正在连接…", "Connecting…")).TextColor(t.TextMuted)
				})
			}
		})
	})
}

// loginView offers the two ways to sign in: a passkey in the system
// browser, or the web admin's password form.
func (a *app) loginView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Children(func() {
		ui.Column(c).Width(380).Gap(14).Children(func() {
			ui.Text(c, a.tr("登录", "Sign in")).FontSize(24).Bold()
			ui.Text(c, a.srv.Root).TextColor(t.TextMuted).Selectable()

			switch {
			case a.login.code != nil:
				ui.Text(c, a.tr("已在浏览器中打开授权页面。请用 Passkey 登录并确认下面的验证码：", "The approval page is open in your browser. Sign in with your passkey and confirm this code:")).TextColor(t.TextMuted)
				ui.Text(c, a.login.code.UserCode).FontSize(28).Bold().Font("monospace").Selectable().AlignSelf(ui.Center).
					Padding(8, 20, 8, 20).Radius(t.Radius*2).Background(t.Surface).Border(1, t.Border)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					components.Spinner(c, components.SpinnerRing).Size(16, 16)
					ui.Text(c, a.tr("等待确认…", "Waiting for approval…")).TextColor(t.TextMuted)
				})
				ui.Row(c).Gap(8).Children(func() {
					if components.Button(c, a.tr("重新打开授权页", "Open the approval page again"), components.ButtonOutline).Clicked() {
						url := a.login.url
						go mygo.Shell.OpenExternal(url)
					}
					if components.Button(c, a.tr("取消", "Cancel"), components.ButtonGhost).Clicked() {
						a.cancelLogin()
					}
				})
				// The approval page sends a browser that is not signed in to
				// /proxy/qaqdmin/#/login, a 404, and the admin's login does not
				// come back to it: sign in first, then reopen the approval page.
				ui.Text(c, a.tr(
					"浏览器显示 404 或要求登录？先在浏览器中登录后台，再点「重新打开授权页」。",
					"Does the browser show a 404 or ask you to sign in? Sign in to the admin there first, then open the approval page again.",
				)).FontSize(12).TextColor(t.TextMuted)
				if components.Button(c, a.tr("在浏览器中登录后台", "Sign in to the admin in the browser"), components.ButtonLink).Clicked() {
					url := a.srv.AdminRoute("/login")
					go mygo.Shell.OpenExternal(url)
				}
			case a.login.busy:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					components.Spinner(c, components.SpinnerRing).Size(16, 16)
					ui.Text(c, a.tr("正在登录…", "Signing in…")).TextColor(t.TextMuted)
				})
			default:
				if components.Button(c, a.tr("使用 Passkey 登录", "Sign in with a passkey"), components.ButtonPrimary).FillWidth().Clicked() {
					a.startPasskeyLogin()
				}
				ui.Text(c, a.tr("将在系统浏览器中完成验证。", "You approve it in your web browser.")).FontSize(12).TextColor(t.TextMuted)
				if components.Button(c, a.tr("使用密码登录", "Sign in with a password"), components.ButtonOutline).FillWidth().Clicked() {
					a.login.password = true
				}
			}
		})
	})
}

// notify queues a toast for the main window, which shows it in its next
// frame. Errors stay longer, and a toast with the same ID replaces it.
func (a *app) notify(kind, id, title, desc string) {
	t := ui.Toast{ID: id, Type: kind, Title: title, Description: desc}
	if kind == components.ToastError {
		t.Timeout = 8 * time.Second
	}
	a.toasts = append(a.toasts, t)
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) flushToasts(c *ui.Context) {
	for _, t := range a.toasts {
		c.AddToast(t)
	}
	a.toasts = nil
}
