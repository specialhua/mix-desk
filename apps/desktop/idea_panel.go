package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// 想法一瞬: a global shortcut shows a panel over whatever app the user is
// in, as Spotlight does, where they write or paste a thought and keep it
// with ⌘↵, on this computer (ideas.go): in the inbox, unless they choose a
// topic. Its source, origin and a note of their own are there to give, not
// asked for. The main window's 附加 › 想法一瞬 page sorts the entries into
// topics and pushes a topic to a draft (ideas_view.go).

// defaultIdeaShortcut summons the panel unless the settings say otherwise:
// ⌃⌥N, which macOS keeps for nothing (⌥⌘N may open its notifications).
const defaultIdeaShortcut = "Ctrl+Alt+N"

// ideaShortcutOff in settings.IdeaShortcut turns the shortcut off.
const ideaShortcutOff = "off"

// ideaShortcutCustom, chosen in the settings, opens the fields of a
// shortcut of the user's own (prefs.go).
const ideaShortcutCustom = "custom"

// ideaShortcuts are the shortcuts the settings offer, the default first.
var ideaShortcuts = func() []string {
	if runtime.GOOS != "darwin" {
		return []string{defaultIdeaShortcut, "Alt+Shift+Space"}
	}
	return []string{defaultIdeaShortcut, "Alt+Shift+Space", "Ctrl+Alt+Cmd+N", "CmdOrCtrl+Alt+N"}
}()

// The panel's choices of where a thought goes, besides a topic's ID.
const (
	inboxChoice    = ""
	newTopicChoice = "new"
)

const (
	ideaPanelWidth, ideaPanelHeight = 600, 420
	// ideaSavedDelay is how long the panel says where the thought went
	// before it hides.
	ideaSavedDelay = 900 * time.Millisecond
)

// ideaPanel is the panel's state, read and changed on the main thread.
type ideaPanel struct {
	win *mygo.Window
	// shortcut is the accelerator registered, "" for none.
	shortcut string

	// choice is where the thought goes: inboxChoice, a topic's ID or
	// newTopicChoice. Summoned afresh, the panel chooses the inbox; it
	// keeps the user's choice while they write one thought after another.
	choice   string
	newTitle string
	source   string
	content  string
	// more shows the fields of the origin and the user's note.
	more   bool
	origin string
	note   string
	// autoOrigin is the link the panel took from the content as its
	// origin, not to take again once the user cleared it.
	autoOrigin string
	// addingSource shows the field of a source of the user's own.
	addingSource bool
	newSource    string

	// saved tells where the last thought went; err why it did not.
	saved string
	err   string
	// draft is the content as last kept on disk (panel-draft.txt).
	draft string
}

// setupIdeas opens the store, creates the panel, hidden, and registers the
// shortcut. Called once the app is ready.
func (a *app) setupIdeas() {
	dir, err := ideasDir()
	if err == nil {
		a.store, err = openIdeaStore(dir)
	}
	if err != nil {
		log.Printf("ideas: %v", err)
		return
	}
	p := &a.panel
	p.content = loadPanelDraft(a.store)
	p.draft = p.content
	p.source = defaultSource
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:         a.tr("想法一瞬", "Idea Flash"),
		Panel:         true,
		Hidden:        true,
		Frameless:     true,
		Transparent:   true,
		Vibrancy:      mygo.VibrancyGlass,
		CornerRadius:  16,
		Width:         ideaPanelWidth,
		Height:        ideaPanelHeight,
		DisableResize: true,
		Content:       ui.View(a.ideaPanelView),
	})
	// Escape, Cmd+W: hide, keeping what is written, to show again at once.
	// A quit closes it (trackQuit); OnBeforeQuit saved the draft.
	win.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		a.hideIdeaPanel()
	})
	// The user went elsewhere.
	win.OnBlur(a.hideIdeaPanel)
	p.win = win
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.savePanelDraft() })
	a.applyIdeaShortcut(false)
	if a.store.fresh && a.ideaShortcut() != "" {
		_ = a.store.saveIndex()
		a.notify(components.ToastInfo, "idea-hint",
			a.tr("按 ", "Press ")+acceleratorText(a.ideaShortcut())+a.tr(" 随时记下想法", " to jot down an idea from anywhere"),
			a.tr("先记进收集箱，再到「附加 › 想法一瞬」归到主题、推送为草稿。", "They go to the inbox; sort them into topics and push them to drafts in Extras › Idea Flash."))
	}
}

// ideaShortcut is the accelerator the settings ask for, "" for none.
func (a *app) ideaShortcut() string {
	switch a.set.IdeaShortcut {
	case "":
		return defaultIdeaShortcut
	case ideaShortcutOff:
		return ""
	}
	return a.set.IdeaShortcut
}

// applyIdeaShortcut registers the shortcut of the settings, then lets go
// of the one registered before, which stays when the new one cannot be
// had. report tells the user when it worked, after they chose it; a
// failure is always told.
func (a *app) applyIdeaShortcut(report bool) error {
	want := a.ideaShortcut()
	p := &a.panel
	if p.shortcut == want {
		return nil
	}
	if want != "" {
		if err := mygo.GlobalShortcut.Register(want, func() { a.toggleIdeaPanel("") }); err != nil {
			log.Printf("idea shortcut %s: %v", want, err)
			a.notify(components.ToastError, "idea-shortcut",
				a.tr("想法一瞬的快捷键不可用", "The Idea Flash shortcut is unavailable"),
				a.tr(acceleratorText(want)+" 可能已被其他应用占用，请换一个。", acceleratorText(want)+" may be taken by another app. Choose another."))
			return err
		}
	}
	if p.shortcut != "" {
		mygo.GlobalShortcut.Unregister(p.shortcut)
	}
	p.shortcut = want
	if report && want != "" {
		a.notify(components.ToastSuccess, "idea-shortcut", a.tr("想法一瞬快捷键：", "Idea Flash shortcut: ")+acceleratorText(want),
			a.tr("如果按下后没反应，可能被系统或其他应用占用了，可在设置里再换。", "If it does nothing, the system or another app has it: choose another in Settings."))
	}
	return nil
}

// setIdeaShortcut keeps the shortcut chosen, an accelerator or
// ideaShortcutOff, once it is registered; one that cannot be leaves the
// one before. Every place that shows it shows the new one.
func (a *app) setIdeaShortcut(v string) error {
	before := a.set.IdeaShortcut
	if v == defaultIdeaShortcut {
		v = ""
	}
	a.set.IdeaShortcut = v
	if err := a.applyIdeaShortcut(true); err != nil {
		a.set.IdeaShortcut = before
		a.invalidatePrefs()
		return err
	}
	_ = a.set.save()
	a.buildMenu()
	a.invalidatePrefs()
	if a.win != nil {
		a.win.Invalidate()
	}
	if w := a.panel.win; w != nil && !w.IsDestroyed() {
		w.Invalidate()
	}
	return nil
}

// toggleIdeaPanel shows the panel, or hides it when it shows. id, when
// given, is the topic to choose. Otherwise the panel chooses the inbox,
// unless it holds a thought not yet kept, whose choice it keeps.
func (a *app) toggleIdeaPanel(id string) {
	p := &a.panel
	if p.win == nil || p.win.IsDestroyed() || a.store == nil {
		return
	}
	if p.win.IsVisible() && id == "" {
		a.hideIdeaPanel()
		return
	}
	a.prepareIdeaPanel(id, p.win.IsVisible())
	p.win.Show()
	p.win.Focus()
	p.win.Invalidate()
}

// prepareIdeaPanel sets the panel's choice as it shows: the topic of id,
// or else the inbox, unless visible or holding a thought not yet kept.
func (a *app) prepareIdeaPanel(id string, visible bool) {
	p := &a.panel
	p.saved, p.err = "", ""
	switch {
	case id != "":
		p.choice = id
	case !visible && strings.TrimSpace(p.content) == "":
		p.choice = inboxChoice
	}
	if p.choice != inboxChoice && p.choice != newTopicChoice && a.store.topic(p.choice) == nil {
		p.choice = inboxChoice
	}
	if p.source == "" {
		p.source = defaultSource
	}
}

func (a *app) hideIdeaPanel() {
	if w := a.panel.win; w != nil && !w.IsDestroyed() {
		w.Hide()
	}
	a.savePanelDraft()
}

// ideaPanelView is the panel.
func (a *app) ideaPanelView(c *ui.Context) {
	t := c.Theme()
	root := c.Root()
	if c.Vibrancy() {
		root.Background(ui.Transparent) // the glass shows through
	} else {
		root.Background(t.Background)
	}
	if a.store == nil {
		return
	}
	p := &a.panel
	chosen := a.store.topic(p.choice)
	if chosen == nil && p.choice != inboxChoice && p.choice != newTopicChoice {
		// Gone since: the inbox, which is always there.
		p.choice = inboxChoice
	}
	save := func() { a.saveIdeaEntry(false) }
	keep := func() { a.saveIdeaEntry(true) }
	clear := func() { p.saved, p.err = "", "" }

	ui.Column(c).Fill().OnShortcut(ui.Cmd, ui.KeyEnter, save).OnShortcut(ui.Cmd|ui.Shift, ui.KeyEnter, keep).Children(func() {
		// The panel's name and its shortcut.
		ui.Row(c).Padding(14, 18, 6, 18).Gap(8).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.Zap()).Size(16, 16).TextColor(t.Accent)
			ui.Text(c, a.tr("想法一瞬", "Idea Flash")).FontSize(13).Bold()
			ui.Spacer(c)
			if acc := a.ideaShortcut(); acc != "" {
				ui.Text(c, acceleratorText(acc)).FontSize(11).TextColor(t.TextMuted)
			}
		})

		// Where the thought goes, in large letters: the inbox, unless the
		// user chooses a topic.
		ui.Row(c).Padding(4, 18).Gap(10).AlignItems(ui.Center).Children(func() {
			topics := a.store.topicList()
			opts := make([]components.NativeSelectOption[string], 0, len(topics)+2)
			opts = append(opts, components.NativeSelectOption[string]{Value: inboxChoice, Label: a.tr("收集箱（未归类）", "Inbox (no topic)")})
			for _, tp := range topics {
				opts = append(opts, components.NativeSelectOption[string]{Value: tp.ID, Label: tp.Title})
			}
			opts = append(opts, components.NativeSelectOption[string]{Value: newTopicChoice, Label: a.tr("＋ 新建主题…", "＋ New topic…")})
			sel := components.NativeSelect(c, &p.choice, opts)
			sel.Trigger.Label(a.tr("记到", "Keep in")).Width(200)
			if sel.Changed() {
				clear()
			}
			if p.choice == newTopicChoice {
				components.TextBox(c, &p.newTitle, components.TextBoxOptions{}).
					Placeholder(a.tr("主题，例如：AI 自动化运维（不是文章标题）", "A topic, e.g. Automating ops with AI (not a title)")).
					Label(a.tr("新主题", "New topic")).Grow(1).AutoFocus()
				return
			}
			ui.Row(c).Grow(1).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.tr("记到：", "Keeping in: ")).TextColor(t.TextMuted)
				if chosen == nil {
					ui.Text(c, a.tr("收集箱", "Inbox")).FontSize(16).Bold()
					ui.Text(c, a.tr("· 以后再归到主题", "· sort it later")).TextColor(t.TextMuted).SingleLine().MinWidth(0)
					return
				}
				ui.Text(c, chosen.Title).FontSize(16).Bold().SingleLine().MinWidth(0)
				ui.Text(c, fmt.Sprintf(a.tr("· 已有 %d 条", "· %d so far"), a.store.count(chosen.ID))).TextColor(t.TextMuted)
			})
		})

		// The source: 闪念 unless the user says, the presets, their own and a
		// new one; and the fields of the origin and a note, on demand.
		ui.Row(c).Padding(4, 14).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Center).Wrap().Children(func() {
				for _, src := range a.store.sources() {
					v := components.ButtonGhost
					if src == p.source {
						v = components.ButtonSecondary
					}
					if components.Button(c, src, v).Key("src:" + src).Clicked() {
						p.source = src
					}
				}
				if p.addingSource {
					in := components.TextBox(c, &p.newSource, components.TextBoxOptions{}).
						Placeholder(a.tr("新来源", "New source")).Label(a.tr("新来源", "New source")).Width(110).AutoFocus()
					if in.Submitted() || components.Button(c, a.tr("添加", "Add"), components.ButtonOutline).Clicked() {
						if name := a.store.addSource(p.newSource); name != "" {
							p.source = name
						}
						p.addingSource, p.newSource = false, ""
					}
				} else if components.Button(c, "＋", components.ButtonGhost).Label(a.tr("新来源", "New source")).Clicked() {
					p.addingSource = true
				}
			})
			label := a.tr("出处 / 补充", "Origin / Note")
			if !p.more && (strings.TrimSpace(p.origin) != "" || strings.TrimSpace(p.note) != "") {
				label += a.tr(" · 已填", " · filled")
			}
			icon := lucide.ChevronDown()
			if p.more {
				icon = lucide.ChevronUp()
			}
			more := components.Button(c, "", components.ButtonGhost).Label(label)
			more.Children(func() {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					components.Text(c, label).SingleLine()
					components.Icon(c, icon).Size(14, 14)
				})
			})
			if more.Clicked() {
				p.more = !p.more
			}
		})

		ui.Divider(c)
		input := ui.TextAreaBase(c, &p.content).Grow(1).MinHeight(0).Padding(10, 18).FontSize(15).
			Placeholder(a.tr("输入或粘贴想法、摘录、书摘…一句话也行", "Type or paste a thought, a quote, a link… a line will do")).
			Label(a.tr("内容", "Content")).AutoFocus()
		if input.Changed() {
			clear()
			// A link in what was pasted is its origin, unless the user
			// gave one, or cleared the one taken.
			if u := firstURL(p.content); u != "" && strings.TrimSpace(p.origin) == "" && u != p.autoOrigin {
				p.origin, p.autoOrigin = u, u
			}
		}
		if p.more {
			ui.Divider(c)
			ui.Column(c).Padding(8, 18).Gap(6).Children(func() {
				components.TextBox(c, &p.origin, components.TextBoxOptions{}).
					Placeholder(a.tr("出处：链接，或书名、作者、帖子", "Origin: a link, or a book, author, thread")).
					Label(a.tr("出处", "Origin")).FillWidth()
				components.TextBox(c, &p.note, components.TextBoxOptions{}).
					Placeholder(a.tr("我的补充：为什么记下它（可不写）", "My note: why I kept it (optional)")).
					Label(a.tr("我的补充", "My note")).FillWidth()
			})
		}
		ui.Divider(c)

		ui.Row(c).Padding(8, 12, 8, 18).Gap(10).AlignItems(ui.Center).Children(func() {
			switch {
			case p.err != "":
				ui.Text(c, p.err).FontSize(12).TextColor(t.Danger).SingleLine().MinWidth(0)
			case p.saved != "":
				components.Icon(c, lucide.Check()).Size(14, 14).TextColor(t.Success)
				ui.Text(c, p.saved).FontSize(12).TextColor(t.Success).SingleLine().MinWidth(0)
			default:
				ui.Text(c, saveShortcutText()+a.tr(" 记下 · ", " to keep · ")+keepOnShortcutText()+a.tr(" 记下并继续 · esc 收起", " keep and go on · esc to close")).
					FontSize(11).TextColor(t.TextMuted).SingleLine().MinWidth(0)
			}
			ui.Spacer(c)
			empty := strings.TrimSpace(p.content) == "" || (p.choice == newTopicChoice && strings.TrimSpace(p.newTitle) == "")
			if components.Button(c, a.tr("记下", "Keep")+"  "+saveShortcutText(), components.ButtonPrimary).Disabled(empty).Clicked() {
				save()
			}
		})
	})
}

// saveIdeaEntry keeps the thought where the panel says now, then hides the
// panel, unless stay: then it keeps the topic for the next thought.
func (a *app) saveIdeaEntry(stay bool) {
	p := &a.panel
	p.saved, p.err = "", ""
	if strings.TrimSpace(p.content) == "" {
		return
	}
	id := p.choice
	if id == newTopicChoice {
		if strings.TrimSpace(p.newTitle) == "" {
			p.err = a.tr("先写主题名，或改记到收集箱", "Name the topic, or keep it in the inbox")
			return
		}
		tp, err := a.store.createTopic(p.newTitle)
		if err != nil {
			p.err = a.ideaErrorText(err)
			return
		}
		id = tp.ID
	}
	e, err := a.store.addEntry(id, entryInput{Source: p.source, Content: p.content, Origin: p.origin, Note: p.note})
	if err != nil {
		p.err = a.ideaErrorText(err)
		if err == errTopicGone {
			p.choice = inboxChoice
		}
		return
	}
	p.choice, p.newTitle = id, ""
	p.content, p.origin, p.note, p.autoOrigin, p.more = "", "", "", "", false
	p.source = defaultSource
	if tp := a.store.topic(id); tp != nil {
		p.saved = fmt.Sprintf(a.tr("已存到本机 · 「%s」第 %d 条", "Kept on this computer · #%[2]d of \"%[1]s\""), tp.Title, a.store.seqOf(e))
	} else {
		p.saved = a.tr("已存到本机 · 收集箱", "Kept on this computer · Inbox")
	}
	a.savePanelDraft()
	a.ideasChanged()
	if stay {
		return
	}
	win := p.win
	time.AfterFunc(ideaSavedDelay, func() {
		if win == nil {
			return
		}
		win.Update(func() {
			if a.panel.saved != "" && win.IsVisible() {
				a.hideIdeaPanel()
			}
		})
	})
}

func (a *app) ideaErrorText(err error) string {
	switch err {
	case errTopicGone:
		return a.tr("这个主题已不存在，请重新选择", "That topic is gone. Choose another")
	case errEntryGone:
		return a.tr("这条闪念已不存在", "That entry is gone")
	case errEmpty:
		return a.tr("内容不能为空", "It cannot be empty")
	case errTopicName:
		return a.tr("已有同名主题", "A topic of that name exists")
	}
	return err.Error()
}

// invalidateMain redraws the main window.
func (a *app) invalidateMain() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

// ideasChanged redraws what shows the ideas.
func (a *app) ideasChanged() {
	a.invalidateMain()
	if w := a.panel.win; w != nil {
		w.Invalidate()
	}
}

func panelDraftPath(s *ideaStore) string { return filepath.Join(s.dir, "panel-draft.txt") }

func loadPanelDraft(s *ideaStore) string {
	data, err := os.ReadFile(panelDraftPath(s))
	if err != nil {
		return ""
	}
	return string(data)
}

// savePanelDraft keeps what the panel holds unsaved on disk, should the app
// quit before it is kept; an empty one removes the file.
func (a *app) savePanelDraft() {
	p := &a.panel
	if a.store == nil || p.content == p.draft {
		return
	}
	path := panelDraftPath(a.store)
	if strings.TrimSpace(p.content) == "" {
		_ = os.Remove(path)
	} else if err := os.WriteFile(path, []byte(p.content), 0o600); err != nil {
		log.Printf("ideas: panel draft: %v", err)
		return
	}
	p.draft = p.content
}

// ideaMenuLabel is the View menu's item, with the global shortcut, which
// is not its accelerator.
func ideaMenuLabel(a *app) string {
	label := a.tr("记一个想法…", "Jot Down an Idea…")
	if acc := a.ideaShortcut(); acc != "" {
		label += "  (" + acceleratorText(acc) + ")"
	}
	return label
}

// keepOnShortcutText is ⇧⌘↵ as the platform writes it.
func keepOnShortcutText() string {
	if runtime.GOOS == "darwin" {
		return "⇧⌘↵"
	}
	return "Ctrl+Shift+Enter"
}

// saveShortcutText is ⌘↵ as the platform writes it.
func saveShortcutText() string {
	if runtime.GOOS == "darwin" {
		return "⌘↵"
	}
	return "Ctrl+Enter"
}

// acceleratorText shows an accelerator as the platform writes it: ⌥⌘N on
// macOS, Ctrl+Alt+N elsewhere.
func acceleratorText(acc string) string {
	if acc == "" {
		return ""
	}
	if runtime.GOOS != "darwin" {
		return strings.ReplaceAll(acc, "CmdOrCtrl", "Ctrl")
	}
	// macOS writes the modifiers in this order.
	order := []struct{ name, sym string }{{"Ctrl", "⌃"}, {"Alt", "⌥"}, {"Shift", "⇧"}, {"CmdOrCtrl", "⌘"}, {"Cmd", "⌘"}}
	parts := strings.Split(acc, "+")
	var b strings.Builder
	for _, o := range order {
		for _, p := range parts {
			if p == o.name {
				b.WriteString(o.sym)
			}
		}
	}
	b.WriteString(parts[len(parts)-1])
	return b.String()
}
