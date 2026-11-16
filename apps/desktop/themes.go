package main

import (
	"encoding/json"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// Color themes recolor the app and the web admin it shows, as an editor's
// themes do: Light and Dark stay the appearance, the theme picks the colors
// of each.
//
// The web admin is the server's, the official build on most servers, so a
// theme cannot change its code: inject/themes/<id>.css overrides the CSS variables
// it and its editor read, under html[data-mx-theme="<id>"], which the
// preload script sets (inject.go). Its rules for the write pages are tuned
// to them; elsewhere they recolor what follows the variables, which an
// upstream change may rename. ThemeScope chooses where the theme applies:
//
//   - "write", the default: the write pages only, and the native UI keeps
//     its colors, beside the admin's other pages that keep theirs;
//   - "global": every page of the admin, and the native UI too.
//
// The preload script checks that the page defines the variables the theme
// overrides; on a page that does not, it leaves the page as it is and says
// so (Bridge.ThemeUnsupported), and the native UI goes back to its colors.
//
// A theme is an entry of colorThemes and its CSS file.

// Theme scopes.
const (
	scopeWrite  = "write"
	scopeGlobal = "global"
)

type colorTheme struct {
	ID string
	// Title and TitleEN name it in the settings and the command palette.
	Title, TitleEN string
	Light, Dark    nativeColors
}

// nativeColors are a theme's colors for the native UI, in one appearance.
type nativeColors struct {
	Background, Surface, SurfaceHover, SurfacePressed, Border ui.Color
	Text, TextMuted                                           ui.Color
	Accent, AccentHover, AccentPressed, AccentText            ui.Color
	Danger, Warning, Success                                  ui.Color
	Selection, Focus                                          ui.Color
	// Sidebar tints the sidebar's translucent material (macOS, Windows
	// 11), which shows the desktop through it otherwise; without a material
	// the sidebar is the Surface.
	Sidebar ui.Color
}

var colorThemes = []colorTheme{{
	// Atom's One Dark and One Light, after kognise/obsidian-atom.
	ID: "atom", Title: "Atom", TitleEN: "Atom",
	Light: nativeColors{
		Background:     ui.Hex("#fafafa"),
		Surface:        ui.Hex("#eaeaeb"),
		SurfaceHover:   ui.Hex("#dbdbdc"),
		SurfacePressed: ui.Hex("#cfd0d2"),
		Border:         ui.Hex("#dbdbdc"),
		Text:           ui.Hex("#383a42"),
		TextMuted:      ui.Hex("#696c77"),
		Accent:         ui.Hex("#4078f2"),
		AccentHover:    ui.Hex("#2f66e0"),
		AccentPressed:  ui.Hex("#2556c4"),
		AccentText:     ui.Hex("#ffffff"),
		Danger:         ui.Hex("#e45649"),
		Warning:        ui.Hex("#986801"),
		Success:        ui.Hex("#50a14f"),
		Selection:      ui.RGBA(64, 120, 242, 0.2),
		Focus:          ui.RGBA(64, 120, 242, 0.5),
		Sidebar:        ui.RGBA(234, 234, 235, 0.55),
	},
	Dark: nativeColors{
		Background:     ui.Hex("#282c34"),
		Surface:        ui.Hex("#21252b"),
		SurfaceHover:   ui.Hex("#2c313a"),
		SurfacePressed: ui.Hex("#353b45"),
		Border:         ui.Hex("#3e4451"),
		Text:           ui.Hex("#dcdfe4"),
		TextMuted:      ui.Hex("#9da5b4"),
		Accent:         ui.Hex("#4d78cc"),
		AccentHover:    ui.Hex("#5a86da"),
		AccentPressed:  ui.Hex("#3f69bd"),
		AccentText:     ui.Hex("#ffffff"),
		Danger:         ui.Hex("#e06c75"),
		Warning:        ui.Hex("#e5c07b"),
		Success:        ui.Hex("#98c379"),
		Selection:      ui.RGBA(97, 175, 239, 0.25),
		Focus:          ui.RGBA(97, 175, 239, 0.55),
		Sidebar:        ui.RGBA(33, 37, 43, 0.72),
	},
}}

func findColorTheme(id string) *colorTheme {
	for i := range colorThemes {
		if colorThemes[i].ID == id {
			return &colorThemes[i]
		}
	}
	return nil
}

// themeCSS returns the CSS of every theme, by ID, for the preload script.
func themeCSS() map[string]string {
	css := make(map[string]string, len(colorThemes))
	for _, th := range colorThemes {
		b, err := injectFiles.ReadFile("inject/themes/" + th.ID + ".css")
		if err != nil {
			panic(fmt.Sprintf("theme %s: %v", th.ID, err))
		}
		css[th.ID] = string(b)
	}
	return css
}

// themeChoice is the theme the page shows, as the preload script reads it.
type themeChoice struct {
	ID    string `json:"id"`
	Scope string `json:"scope"`
}

func (a *app) themeChoice() themeChoice {
	if findColorTheme(a.set.Theme) == nil {
		return themeChoice{}
	}
	return themeChoice{a.set.Theme, a.themeScope()}
}

func (a *app) themeScope() string {
	if a.set.ThemeScope == scopeGlobal {
		return scopeGlobal
	}
	return scopeWrite
}

// nativeTheme is the theme the native UI takes its colors from, nil for
// its own: only a global theme the page supports.
func (a *app) nativeTheme() *colorTheme {
	if a.themeScope() != scopeGlobal || a.themeUnsupported {
		return nil
	}
	return findColorTheme(a.set.Theme)
}

// nativeColors are the colors of the native UI's theme in the frame's
// appearance, nil for its own.
func (a *app) nativeColors(c *ui.Context) *nativeColors {
	th := a.nativeTheme()
	if th == nil {
		return nil
	}
	if c.Theme().Dark {
		return &th.Dark
	}
	return &th.Light
}

// frameTheme returns the frame's theme with the native UI's theme's colors:
// the windows' Theme hook.
func (a *app) frameTheme(c *ui.Context) *ui.Theme {
	n := a.nativeColors(c)
	if n == nil {
		return c.Theme()
	}
	// A copy: the frame's theme is the runtime's, and keeps the appearance
	// and the system's preferences (font size, contrast) it was made with.
	t := *c.Theme()
	t.Background, t.Surface, t.SurfaceHover, t.SurfacePressed, t.Border = n.Background, n.Surface, n.SurfaceHover, n.SurfacePressed, n.Border
	t.Text, t.TextMuted = n.Text, n.TextMuted
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = n.Accent, n.AccentHover, n.AccentPressed, n.AccentText
	t.Danger, t.Warning, t.Success = n.Danger, n.Warning, n.Success
	t.Selection, t.Focus = n.Selection, n.Focus
	return &t
}

// sidebarTint is the color over the sidebar's material, transparent for
// none.
func (a *app) sidebarTint(c *ui.Context) ui.Color {
	n := a.nativeColors(c)
	if n == nil || !c.Vibrancy() {
		return ui.Transparent
	}
	return n.Sidebar
}

// setTheme chooses the color theme, "" for the app's own colors.
func (a *app) setTheme(id string) {
	if id != "" && findColorTheme(id) == nil {
		id = ""
	}
	a.set.Theme = id
	a.themeChanged()
}

// setThemeScope chooses where the theme applies: scopeWrite or scopeGlobal.
func (a *app) setThemeScope(scope string) {
	if scope != scopeGlobal {
		scope = scopeWrite
	}
	a.set.ThemeScope = scope
	a.themeChanged()
}

func (a *app) themeChanged() {
	_ = a.set.save()
	// The page checks again whether it supports the theme.
	a.themeUnsupported = false
	a.applyTheme()
	if a.win != nil {
		a.win.Invalidate()
	}
	a.invalidatePrefs()
}

// applyTheme tells the page the theme it shows.
func (a *app) applyTheme() {
	if a.web == nil {
		return
	}
	choice, _ := json.Marshal(a.themeChoice())
	go a.web.Page().Eval("window.__mxDesktopTheme && window.__mxDesktopTheme(" + string(choice) + ")")
}

// themeUnsupportedNotice says the page did not take the theme.
func (a *app) themeUnsupportedNotice() {
	if a.themeUnsupported {
		return
	}
	a.themeUnsupported = true
	th := findColorTheme(a.set.Theme)
	if th == nil {
		return
	}
	a.notify(components.ToastWarning, "theme",
		a.tr("后台未适配「"+th.Title+"」主题", "The admin does not support the "+th.TitleEN+" theme"),
		a.tr("这个版本的后台缺少主题需要的样式变量，已保持默认配色。", "This version of the admin lacks the style variables the theme overrides; it keeps its own colors."))
	if a.win != nil {
		a.win.Invalidate()
	}
	a.invalidatePrefs()
}

// themeOptions are the choices of the theme setting.
func (a *app) themeOptions() []components.NativeSelectOption[string] {
	opts := []components.NativeSelectOption[string]{{Value: "", Label: a.tr("默认", "Default")}}
	for _, th := range colorThemes {
		opts = append(opts, components.NativeSelectOption[string]{Value: th.ID, Label: a.tr(th.Title, th.TitleEN)})
	}
	return opts
}

// themeKeywords find the theme settings and actions in a search.
var themeKeywords = []string{"theme", "color", "palette", "主题", "配色", "颜色"}

// themeScopeDescription explains the scope chosen, and what may go wrong.
func (a *app) themeScopeDescription() string {
	d := a.tr("只给写作页（写博文、手记、页面和编辑草稿）换色，已逐项适配；应用保持原有配色。",
		"Recolors the write pages only (posts, notes, pages and drafts), tuned to them; the app keeps its colors.")
	if a.themeScope() == scopeGlobal {
		d = a.tr("后台所有页面和应用一起换色。后台由服务器提供，部分页面可能显示不一致。",
			"Recolors every page of the admin, and the app. The admin is the server's: some pages may not match.")
	}
	if a.themeUnsupported {
		d += a.tr(" 当前后台未适配这个主题，已保持默认配色。", " This admin does not support the theme; it keeps its own colors.")
	}
	return d
}
