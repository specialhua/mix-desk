package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// The main window's 附加 › 想法一瞬 page: the inbox and the topics of the
// entries the panel kept (idea_panel.go), a search of them all and a
// timeline of them by day. There the user edits an entry, moves it to a
// topic or has another topic cite it, picks several to move or push at
// once, and pushes a topic, or what they picked, to a note or post draft
// (ideas_draft.go), seeing the draft before it is made. It shows in place
// of the web admin, whose page stays loaded underneath.

// extraIdeas is the extra page of the ideas (app.extraPage).
const extraIdeas = "ideas"

// The page's views besides a topic or the inbox (ideasPage.mode).
const (
	ideasTimeline = "timeline"
	ideasSearch   = "search"
)

// ideasPage is the page's state.
type ideasPage struct {
	// mode is "" for a topic, or the inbox, then ideasTimeline or
	// ideasSearch.
	mode string
	// selected is the topic shown, "" for the inbox.
	selected string
	query    string
	err      string
	// generating is set while a draft is being made.
	generating bool

	creating   bool
	createText string
	renaming   bool
	renameText string
	// editing is the ID of the entry being edited.
	editing                        string
	editSource, editText, editNote string
	editURL, editTitle, editAuthor string
	editPublished                  string
	// moving is the ID of the entry being moved, to moveTo; citing the ID
	// of the entry another topic is to cite, citeTo.
	moving, moveTo, moveTitle string
	citing, citeTo            string
	confirmDrop               bool
	// selecting is set while the user picks entries, picked, to move to
	// batchTo or push.
	selecting         bool
	picked            map[string]bool
	batchTo, batchNew string
	// preview is the push the user is about to make, shown first.
	preview *pushPlan
	// history shows every push of the topic.
	history bool
	// focus is the entry to bring into view, once; found the one to mark.
	focus, found string
}

// show shows a topic, "" for the inbox.
func (v *ideasPage) show(id string) {
	if v.selected != id || v.mode != "" {
		*v = ideasPage{selected: id, generating: v.generating, query: v.query}
	}
	v.mode = ""
}

// locate shows an entry in its topic, or the inbox.
func (v *ideasPage) locate(e *entry) {
	v.show(e.TopicID)
	v.focus, v.found = e.ID, e.ID
}

// showIdeasPage shows the page in the main window.
func (a *app) showIdeasPage() {
	a.extraPage = extraIdeas
	a.ideas.err = ""
	a.showMain()
	a.invalidateMain()
}

func (a *app) ideasView(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		if a.store == nil {
			ui.Text(c, a.tr("无法打开本机的想法存储", "Cannot open the ideas on this computer")).Padding(24).TextColor(t.Danger)
			return
		}
		v := &a.ideas
		if v.selected != "" && a.store.topic(v.selected) == nil {
			*v = ideasPage{generating: v.generating, mode: v.mode, query: v.query}
		}
		a.ideasList(c)
		ui.Box(c).Width(1).Background(t.Border)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			switch {
			case v.mode == ideasSearch:
				a.searchView(c)
			case v.mode == ideasTimeline:
				a.timelineView(c)
			case v.preview != nil:
				a.previewView(c, *v.preview)
			default:
				a.ideasDetail(c, a.store.topic(v.selected))
			}
		})
	})
}

// ideasList is the search, then the inbox, the timeline and the topics.
func (a *app) ideasList(c *ui.Context) {
	t := c.Theme()
	v := &a.ideas
	ui.Column(c).Width(250).Children(func() {
		ui.Row(c).Padding(14, 14, 8, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("想法一瞬", "Idea Flash")).FontSize(15).Bold().Grow(1)
			tip := a.tr("记一条", "Jot down")
			if acc := a.ideaShortcut(); acc != "" {
				tip += " (" + acceleratorText(acc) + ")"
			}
			if components.IconButton(c, lucide.Zap(), tip, components.IconButtonToolbar).Clicked() {
				a.toggleIdeaPanel("")
			}
		})
		ui.Row(c).Padding(0, 14, 8, 14).Children(func() {
			search := ui.SearchField(c, &v.query).Placeholder(a.tr("搜索原文、出处、补充", "Search text, origins, notes")).
				Label(a.tr("搜索闪念", "Search entries")).FillWidth()
			if search.Changed() {
				if strings.TrimSpace(v.query) != "" {
					v.mode = ideasSearch
				} else if v.mode == ideasSearch {
					v.mode = ""
				}
			}
		})
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			components.Sidebar(c).Padding(0, t.Space(2), t.Space(3), t.Space(2)).Children(func() {
				inbox := fmt.Sprintf(a.tr("收集箱 · %d", "Inbox · %d"), a.store.count(""))
				if components.SidebarItem(c, inbox, lucide.Inbox(), v.mode == "" && v.selected == "").Key("inbox").Clicked() {
					v.show("")
				}
				if components.SidebarItem(c, a.tr("时间线", "Timeline"), lucide.CalendarDays(), v.mode == ideasTimeline).Key("timeline").Clicked() {
					v.show("")
					v.mode = ideasTimeline
				}
				components.SidebarSectionWithActions(c, a.tr("主题", "Topics"), func(c *ui.Context) {
					if components.IconButton(c, lucide.Plus(), a.tr("新建主题", "New topic"), components.IconButtonToolbar).Clicked() {
						v.creating, v.createText = true, ""
					}
				}, func() {
					if v.creating {
						in := components.TextBox(c, &v.createText, components.TextBoxOptions{}).
							Placeholder(a.tr("主题名，回车创建", "A topic, then Return")).Label(a.tr("新主题", "New topic")).FillWidth().AutoFocus()
						if in.Submitted() {
							if strings.TrimSpace(v.createText) == "" {
								v.creating = false
							} else if tp, err := a.store.createTopic(v.createText); err != nil {
								v.err = a.ideaErrorText(err)
							} else {
								v.show(tp.ID)
								a.ideasChanged()
							}
						}
					}
					topics := a.store.topicList()
					if len(topics) == 0 && !v.creating {
						ui.Text(c, a.tr("还没有主题", "No topics yet")).FontSize(12).TextColor(t.TextMuted).Padding(4, 8)
					}
					for _, tp := range topics {
						label := fmt.Sprintf("%s · %d", tp.Title, a.store.count(tp.ID))
						if tp.lastPush() != nil {
							if n := a.store.unpushed(tp); n > 0 {
								label += fmt.Sprintf(a.tr(" · 新增 %d", " · %d new"), n)
							} else {
								label += a.tr(" · 已推送", " · pushed")
							}
						}
						if components.SidebarItem(c, label, nil, v.mode == "" && v.selected == tp.ID).Key("topic:" + tp.ID).Clicked() {
							v.show(tp.ID)
						}
					}
				})
			})
		})
	})
}

// pageHeader is a view's name and what it is, then its buttons.
func (a *app) pageHeader(c *ui.Context, title, about string, buttons func()) {
	t := c.Theme()
	ui.Row(c).Padding(14, 20, 10, 20).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(20).Bold().SingleLine().MinWidth(0)
			ui.Text(c, about).FontSize(12).TextColor(t.TextMuted).Wrap()
		})
		if buttons != nil {
			buttons()
		}
	})
}

// pageError is the page's error, when there is one.
func (a *app) pageError(c *ui.Context) {
	if v := &a.ideas; v.err != "" {
		ui.Text(c, v.err).FontSize(12).TextColor(c.Theme().Danger).Padding(0, 20, 8, 20).Wrap()
	}
}

// entryList is a scrolling column of entries.
func entryList(c *ui.Context, fn func()) {
	ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(12, 20).Gap(12).Children(fn)
	})
}

// ideasDetail shows a topic, or the inbox for nil: its entries, and what
// to do with them.
func (a *app) ideasDetail(c *ui.Context, tp *topic) {
	t := c.Theme()
	v := &a.ideas
	id := ""
	var entries []*entry
	if tp != nil {
		id = tp.ID
		entries = a.store.material(tp)
	} else {
		entries = a.store.entriesOf("")
	}
	ui.Column(c).Fill().Children(func() {
		if tp != nil && v.renaming {
			ui.Row(c).Padding(14, 20, 10, 20).Gap(8).AlignItems(ui.Center).Children(func() {
				in := components.TextBox(c, &v.renameText, components.TextBoxOptions{}).Label(a.tr("主题名", "Topic")).Grow(1).AutoFocus()
				if in.Submitted() || components.Button(c, a.tr("保存", "Save"), components.ButtonPrimary).Clicked() {
					if _, err := a.store.renameTopic(id, v.renameText); err != nil {
						v.err = a.ideaErrorText(err)
					} else {
						v.renaming, v.err = false, ""
						a.ideasChanged()
					}
				}
				if components.Button(c, a.tr("取消", "Cancel"), components.ButtonGhost).Clicked() {
					v.renaming = false
				}
			})
		} else {
			title := a.tr("收集箱", "Inbox")
			about := a.tr("还没归到主题的闪念。移到主题后再推送，或勾选几条直接推送为草稿。", "Entries of no topic yet. Move them to a topic, or pick some and push them to a draft.")
			if tp != nil {
				title = tp.Title
				about = a.tr("主题是积累素材的容器，不是文章标题。", "A topic gathers material; it is not an article's title.")
				if n := len(a.store.refsOf(tp)); n > 0 {
					about += fmt.Sprintf(a.tr("其中 %d 条引自别处。", " %d of them cited from elsewhere."), n)
				}
			}
			a.pageHeader(c, title, about, func() {
				if tp != nil && components.IconButton(c, lucide.Pencil(), a.tr("改名", "Rename"), components.IconButtonToolbar).Clicked() {
					v.renaming, v.renameText = true, tp.Title
				}
				if len(entries) > 0 && !v.selecting {
					if components.Button(c, a.tr("多选", "Select"), components.ButtonGhost).Clicked() {
						v.selecting, v.picked, v.editing, v.moving, v.citing = true, map[string]bool{}, "", "", ""
						v.batchTo, v.batchNew = "", ""
					}
				}
				if components.Button(c, a.tr("记一条", "Jot Down"), components.ButtonOutline).Clicked() {
					a.toggleIdeaPanel(id)
				}
			})
		}
		if tp != nil {
			a.pushStatus(c, tp.Pushes, a.store.unpushed(tp))
		} else {
			a.pushStatus(c, a.store.inboxPushes(), 0)
		}
		a.pageError(c)
		ui.Divider(c)

		entryList(c, func() {
			if len(entries) == 0 {
				hint := a.tr("还没有素材", "Nothing here yet")
				if acc := a.ideaShortcut(); acc != "" {
					hint += a.tr("。按 ", ". Press ") + acceleratorText(acc) + a.tr(" 在任何应用里记一条。", " to jot one down from any app.")
				}
				ui.Text(c, hint).TextColor(t.TextMuted)
			}
			for k, e := range entries {
				a.entryView(c, entryContext{topic: tp, seq: k + 1}, e)
			}
		})
		ui.Divider(c)
		switch {
		case v.selecting:
			a.selectionBar(c, tp, entries)
		case tp != nil:
			a.topicActions(c, tp, len(entries))
		}
	})
}

// pushStatus tells whether and when the topic, or the inbox, was pushed,
// what came since, and every draft made, on demand.
func (a *app) pushStatus(c *ui.Context, pushes []ideaPush, unpushed int) {
	t := c.Theme()
	v := &a.ideas
	if len(pushes) == 0 {
		return
	}
	last := pushes[len(pushes)-1]
	ui.Row(c).Padding(0, 20, 6, 20).Gap(8).AlignItems(ui.Center).Children(func() {
		text := fmt.Sprintf(a.tr("已推送 %d 次，最近一次 %s 生成%s草稿", "Pushed %d times, last to a %[3]s draft on %[2]s"),
			len(pushes), last.At.Local().Format(ideaTimeLayout), a.refKind(last.RefType))
		if unpushed > 0 {
			text += fmt.Sprintf(a.tr(" · 之后新增 %d 条", " · %d new since"), unpushed)
		}
		ui.Text(c, text).FontSize(12).TextColor(t.TextMuted).SingleLine().MinWidth(0).Grow(1)
		icon, label := lucide.ChevronDown(), a.tr("推送记录", "Pushes")
		if v.history {
			icon = lucide.ChevronUp()
		}
		toggle := components.Button(c, "", components.ButtonGhost).Label(label)
		toggle.Children(func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				components.Text(c, label).SingleLine()
				components.Icon(c, icon).Size(14, 14)
			})
		})
		if toggle.Clicked() {
			v.history = !v.history
		}
		if components.Button(c, a.tr("打开上次草稿", "Open Last Draft"), components.ButtonGhost).Disabled(!a.signedIn()).Clicked() {
			a.openPushedDraft(last)
		}
	})
	if !v.history {
		return
	}
	ui.Column(c).Padding(0, 20, 10, 20).Gap(2).Children(func() {
		for k := len(pushes) - 1; k >= 0; k-- {
			p := pushes[k]
			ui.Row(c).Key(fmt.Sprintf("push:%d", k)).Gap(8).AlignItems(ui.Center).Children(func() {
				text := fmt.Sprintf(a.tr("第 %d 次 · %s · %s草稿 · %d 条", "#%d · %s · %s draft · %d entries"),
					k+1, p.At.Local().Format(ideaTimeLayout), a.refKind(p.RefType), len(p.Entries))
				if p.Title != "" {
					text += " · 「" + p.Title + "」"
				}
				ui.Text(c, text).FontSize(12).SingleLine().MinWidth(0).Grow(1)
				if components.Button(c, a.tr("打开", "Open"), components.ButtonGhost).Disabled(!a.signedIn()).Clicked() {
					a.openPushedDraft(p)
				}
			})
		}
	})
}

// topicActions are the topic's buttons: delete it, push it.
func (a *app) topicActions(c *ui.Context, tp *topic, n int) {
	t := c.Theme()
	v := &a.ideas
	ui.Row(c).Padding(10, 20).Gap(8).AlignItems(ui.Center).Children(func() {
		if v.confirmDrop {
			ui.Text(c, fmt.Sprintf(a.tr("删除这个主题？其中 %d 条闪念会移回收集箱，不会删除。", "Delete the topic? Its %d entries go back to the inbox."), a.store.count(tp.ID))).
				TextColor(t.Danger).Grow(1).MinWidth(0)
			if components.Button(c, a.tr("删除主题", "Delete Topic"), components.ButtonPrimary).Clicked() {
				if err := a.store.deleteTopic(tp.ID); err != nil {
					v.err = a.ideaErrorText(err)
				} else {
					v.show("")
				}
				a.ideasChanged()
			}
			if components.Button(c, a.tr("取消", "Cancel"), components.ButtonGhost).Clicked() {
				v.confirmDrop = false
			}
			return
		}
		if components.Button(c, a.tr("删除主题", "Delete Topic"), components.ButtonGhost).Clicked() {
			v.confirmDrop = true
		}
		ui.Spacer(c)
		a.pushButtons(c, pushPlan{TopicID: tp.ID}, n == 0)
	})
}

// pushButtons preview a push of the plan to a note or a post draft.
func (a *app) pushButtons(c *ui.Context, plan pushPlan, disabled bool) {
	v := &a.ideas
	disabled = disabled || v.generating
	for _, ref := range []string{refNote, refPost} {
		label, variant := a.tr("推送为手记草稿…", "Push to a Note Draft…"), components.ButtonOutline
		if ref == refPost {
			label, variant = a.tr("推送为博文草稿…", "Push to a Post Draft…"), components.ButtonPrimary
		}
		if components.Button(c, label, variant).Key("push:" + ref).Disabled(disabled).Clicked() {
			p := plan
			p.RefType, p.IDs = ref, slices.Clone(plan.IDs)
			v.preview, v.err = &p, ""
		}
	}
}

// selectionBar is what to do with the entries picked: move them, push
// them.
func (a *app) selectionBar(c *ui.Context, tp *topic, entries []*entry) {
	t := c.Theme()
	v := &a.ideas
	var ids []string
	for _, e := range entries {
		if v.picked[e.ID] {
			ids = append(ids, e.ID)
		}
	}
	ui.Column(c).Padding(10, 20).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf(a.tr("已选 %d 条", "%d picked"), len(ids))).Bold()
			all := len(ids) == len(entries)
			label := a.tr("全选", "All")
			if all {
				label = a.tr("全不选", "None")
			}
			if components.Button(c, label, components.ButtonGhost).Clicked() {
				v.picked = map[string]bool{}
				if !all {
					for _, e := range entries {
						v.picked[e.ID] = true
					}
				}
			}
			ui.Spacer(c)
			if components.Button(c, a.tr("完成", "Done"), components.ButtonGhost).Clicked() {
				v.selecting, v.picked = false, nil
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			own := ""
			if tp != nil {
				own = tp.ID
			}
			opts := a.topicOptions(own, tp != nil)
			if v.batchTo == "" && tp == nil && len(opts) > 0 {
				v.batchTo = opts[0].Value
			}
			sel := components.NativeSelect(c, &v.batchTo, opts)
			sel.Trigger.Label(a.tr("移到", "Move to")).Width(170)
			if v.batchTo == newTopicChoice {
				components.TextBox(c, &v.batchNew, components.TextBoxOptions{}).
					Placeholder(a.tr("新主题名", "New topic")).Label(a.tr("新主题名", "New topic")).Width(140)
			}
			ready := len(ids) > 0 && (v.batchTo != newTopicChoice || strings.TrimSpace(v.batchNew) != "")
			if components.Button(c, a.tr("移动", "Move"), components.ButtonOutline).Disabled(!ready).Clicked() {
				to, err := a.moveTarget(v.batchTo, v.batchNew)
				if err == nil {
					err = a.store.moveEntries(ids, to)
				}
				if err != nil {
					v.err = a.ideaErrorText(err)
				} else {
					v.selecting, v.picked, v.err = false, nil, ""
				}
				a.ideasChanged()
			}
			ui.Spacer(c)
			ui.Text(c, a.tr("或", "or")).TextColor(t.TextMuted)
			plan := pushPlan{IDs: ids}
			if tp != nil {
				plan.TopicID = tp.ID
			}
			a.pushButtons(c, plan, len(ids) == 0)
		})
	})
}

// topicOptions are where entries can go: the inbox, unless they are in it
// (inbox), the topics but own, and a new one.
func (a *app) topicOptions(own string, inbox bool) []components.NativeSelectOption[string] {
	var opts []components.NativeSelectOption[string]
	if inbox {
		opts = append(opts, components.NativeSelectOption[string]{Value: inboxChoice, Label: a.tr("收集箱", "Inbox")})
	}
	for _, o := range a.store.topicList() {
		if o.ID != own {
			opts = append(opts, components.NativeSelectOption[string]{Value: o.ID, Label: o.Title})
		}
	}
	return append(opts, components.NativeSelectOption[string]{Value: newTopicChoice, Label: a.tr("＋ 新建主题…", "＋ New topic…")})
}

// moveTarget is the topic chosen, made when it is a new one.
func (a *app) moveTarget(choice, title string) (string, error) {
	if choice != newTopicChoice {
		return choice, nil
	}
	tp, err := a.store.createTopic(title)
	if err != nil {
		return "", err
	}
	return tp.ID, nil
}

// entryContext is where an entry shows: in a topic, numbered seq, or the
// inbox for a nil topic; or, where, in a search or the timeline, with the
// topic it is in.
type entryContext struct {
	topic *topic
	seq   int
	where bool
}

// entryView is an entry, to edit, move, cite, pick or delete.
func (a *app) entryView(c *ui.Context, ec entryContext, e *entry) {
	t := c.Theme()
	v := &a.ideas
	cited := ec.topic != nil && e.TopicID != ec.topic.ID
	card := ui.Column(c).Key("entry:"+e.ID).FillWidth().Gap(6).Padding(10, 12).Radius(t.Radius * 1.5).Background(t.Surface)
	if v.found == e.ID && !ec.where {
		card.Border(2, t.Accent)
	} else {
		card.Border(1, t.Border)
	}
	if v.focus == e.ID && !ec.where {
		card.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollCenter})
		v.focus = ""
	}
	card.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if v.selecting && !ec.where {
				on := v.picked[e.ID]
				if components.Checkbox(c, &on, "").Label(a.tr("选中", "Pick")).Changed() {
					v.picked[e.ID] = on
				}
			}
			if ec.seq > 0 {
				ui.Text(c, fmt.Sprint(ec.seq)).Bold()
			}
			ui.Text(c, e.Created.Local().Format(ideaTimeLayout)).FontSize(12).TextColor(t.TextMuted)
			components.Badge(c, e.Source)
			if ec.where {
				components.Badge(c, orDefault(a.store.topicTitle(e.TopicID), a.tr("收集箱", "Inbox")))
			}
			if cited {
				components.Badge(c, a.tr("引自 ", "From ")+orDefault(a.store.topicTitle(e.TopicID), a.tr("收集箱", "Inbox")))
			}
			if ec.topic != nil && ec.topic.lastPush() != nil && !ec.topic.pushed(e.ID) {
				components.Badge(c, a.tr("新增", "New"))
			}
			if e.Edited != nil {
				ui.Text(c, a.tr("已编辑 · 原文已保留", "Edited · original kept")).FontSize(11).TextColor(t.TextMuted).SingleLine().MinWidth(0)
			}
			ui.Spacer(c)
			if v.editing == e.ID || v.moving == e.ID || v.citing == e.ID || v.selecting && !ec.where {
				return
			}
			a.entryActions(c, ec, e, cited)
		})
		if v.editing == e.ID {
			a.entryEditor(c, e)
			return
		}
		ui.Text(c, e.Content).Wrap().Selectable()
		if o := e.Origin.String(); o != "" {
			ui.Text(c, a.tr("出处：", "Origin: ")+o).FontSize(12).TextColor(t.TextMuted).Wrap().Selectable()
		}
		if e.Note != "" {
			ui.Column(c).FillWidth().Gap(2).Padding(6, 10).Radius(t.Radius).Background(t.Background).Children(func() {
				ui.Text(c, a.tr("我的补充", "My note")).FontSize(11).Bold().TextColor(t.TextMuted)
				ui.Text(c, e.Note).Wrap().Selectable()
			})
		}
		if by := a.store.citedBy(e.ID); len(by) > 0 && !cited {
			names := make([]string, len(by))
			for k, o := range by {
				names[k] = "「" + o.Title + "」"
			}
			ui.Text(c, a.tr("也被引用于：", "Also cited by: ")+strings.Join(names, a.tr("、", ", "))).FontSize(12).TextColor(t.TextMuted).Wrap()
		}
		switch e.ID {
		case v.moving:
			a.entryMover(c, e)
		case v.citing:
			a.entryCiter(c, e)
		}
	})
}

// entryActions are an entry's buttons: in a search or the timeline, to
// show it in its topic; cited, to cite it no more.
func (a *app) entryActions(c *ui.Context, ec entryContext, e *entry, cited bool) {
	v := &a.ideas
	if ec.where {
		label := a.tr("在主题中查看", "Show in Topic")
		if e.TopicID == "" {
			label = a.tr("在收集箱中查看", "Show in Inbox")
		}
		if components.Button(c, label, components.ButtonGhost).Clicked() {
			v.locate(e)
		}
		return
	}
	if components.IconButton(c, lucide.Pencil(), a.tr("编辑", "Edit"), components.IconButtonToolbar).Clicked() {
		v.editing, v.moving, v.citing = e.ID, "", ""
		v.editSource, v.editText, v.editNote = e.Source, e.Content, e.Note
		o := entryOrigin{}
		if e.Origin != nil {
			o = *e.Origin
		}
		v.editURL, v.editTitle, v.editAuthor, v.editPublished = o.URL, o.Title, o.Author, o.Published
	}
	if cited {
		if components.IconButton(c, lucide.Unlink(), a.tr("取消引用（原条目不受影响）", "Cite no more (the entry stays)"), components.IconButtonToolbar).Clicked() {
			if err := a.store.uncite(ec.topic.ID, e.ID); err != nil {
				v.err = a.ideaErrorText(err)
			}
			a.ideasChanged()
		}
		return
	}
	if len(a.store.topicList()) > 0 && components.IconButton(c, lucide.Link2(), a.tr("引用到其他主题", "Cite in another topic"), components.IconButtonToolbar).Clicked() {
		v.citing, v.editing, v.moving, v.citeTo = e.ID, "", "", ""
		for _, o := range a.store.topicList() {
			if o.ID != e.TopicID {
				v.citeTo = o.ID
				break
			}
		}
	}
	if components.IconButton(c, lucide.FolderInput(), a.tr("移到主题", "Move to a topic"), components.IconButtonToolbar).Clicked() {
		v.moving, v.editing, v.citing, v.moveTitle = e.ID, "", "", ""
		opts := a.topicOptions(e.TopicID, e.TopicID != "")
		v.moveTo = opts[0].Value
	}
	if components.IconButton(c, lucide.Trash(), a.tr("删除", "Delete"), components.IconButtonToolbar).Clicked() {
		if err := a.store.deleteEntry(e.ID); err != nil {
			v.err = a.ideaErrorText(err)
		}
		a.ideasChanged()
	}
}

// entryEditor edits what the user wrote of an entry, its origin field by
// field; the store keeps the first version.
func (a *app) entryEditor(c *ui.Context, e *entry) {
	v := &a.ideas
	srcs := make([]components.NativeSelectOption[string], 0)
	for _, s := range a.store.sources() {
		srcs = append(srcs, components.NativeSelectOption[string]{Value: s, Label: s})
	}
	sel := components.NativeSelect(c, &v.editSource, srcs)
	sel.Trigger.Label(a.tr("来源", "Source")).Width(140)
	components.TextBox(c, &v.editText, components.TextBoxOptions{Multiline: true}).Label(a.tr("内容", "Content")).FillWidth().MinHeight(80)
	field := func(text *string, label, placeholder string) {
		components.TextBox(c, text, components.TextBoxOptions{}).Placeholder(placeholder).Label(label).Grow(1).MinWidth(0)
	}
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		field(&v.editURL, a.tr("出处链接", "Origin link"), a.tr("出处链接 https://…", "Link https://…"))
		field(&v.editTitle, a.tr("出处标题", "Origin title"), a.tr("标题：书名、文章、帖子", "Title: a book, article, thread"))
	})
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		field(&v.editAuthor, a.tr("作者", "Author"), a.tr("作者", "Author"))
		field(&v.editPublished, a.tr("原文发布时间", "Published"), a.tr("原文发布时间，如 2026-11-01", "Published, e.g. 2026-11-01"))
	})
	components.TextBox(c, &v.editNote, components.TextBoxOptions{Multiline: true}).
		Placeholder(a.tr("我的补充（可不写）", "My note (optional)")).Label(a.tr("我的补充", "My note")).FillWidth().MinHeight(48)
	ui.Row(c).Gap(8).Children(func() {
		if components.Button(c, a.tr("保存", "Save"), components.ButtonPrimary).Disabled(strings.TrimSpace(v.editText) == "").Clicked() {
			in := entryInput{Source: v.editSource, Content: v.editText, Note: v.editNote,
				OriginParts: &entryOrigin{URL: v.editURL, Title: v.editTitle, Author: v.editAuthor, Published: v.editPublished}}
			if _, err := a.store.editEntry(e.ID, in); err != nil {
				v.err = a.ideaErrorText(err)
			} else {
				v.editing = ""
			}
			a.ideasChanged()
		}
		if components.Button(c, a.tr("取消", "Cancel"), components.ButtonGhost).Clicked() {
			v.editing = ""
		}
	})
}

// entryMover picks where to move an entry: the inbox, another topic, or a
// new one.
func (a *app) entryMover(c *ui.Context, e *entry) {
	v := &a.ideas
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		sel := components.NativeSelect(c, &v.moveTo, a.topicOptions(e.TopicID, e.TopicID != ""))
		sel.Trigger.Label(a.tr("移到", "Move to")).Width(180)
		if v.moveTo == newTopicChoice {
			components.TextBox(c, &v.moveTitle, components.TextBoxOptions{}).
				Placeholder(a.tr("新主题名", "New topic")).Label(a.tr("新主题名", "New topic")).Grow(1).AutoFocus()
		} else {
			ui.Spacer(c)
		}
		ready := v.moveTo != newTopicChoice || strings.TrimSpace(v.moveTitle) != ""
		if components.Button(c, a.tr("移动", "Move"), components.ButtonPrimary).Disabled(!ready).Clicked() {
			to, err := a.moveTarget(v.moveTo, v.moveTitle)
			if err == nil {
				err = a.store.moveEntry(e.ID, to)
			}
			if err != nil {
				v.err = a.ideaErrorText(err)
			} else {
				v.moving = ""
			}
			a.ideasChanged()
		}
		if components.Button(c, a.tr("取消", "Cancel"), components.ButtonGhost).Clicked() {
			v.moving = ""
		}
	})
}

// entryCiter picks a topic to cite an entry, which stays where it is.
func (a *app) entryCiter(c *ui.Context, e *entry) {
	t := c.Theme()
	v := &a.ideas
	var opts []components.NativeSelectOption[string]
	for _, o := range a.store.topicList() {
		if o.ID != e.TopicID {
			label := o.Title
			if slices.Contains(o.Refs, e.ID) {
				label += a.tr("（已引用）", " (cited)")
			}
			opts = append(opts, components.NativeSelectOption[string]{Value: o.ID, Label: label})
		}
	}
	if len(opts) == 0 {
		v.citing = ""
		return
	}
	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			sel := components.NativeSelect(c, &v.citeTo, opts)
			sel.Trigger.Label(a.tr("引用到", "Cite in")).Width(180)
			ui.Spacer(c)
			if components.Button(c, a.tr("引用", "Cite"), components.ButtonPrimary).Disabled(v.citeTo == "").Clicked() {
				if err := a.store.cite(v.citeTo, e.ID); err != nil {
					v.err = a.ideaErrorText(err)
				} else {
					v.citing = ""
				}
				a.ideasChanged()
			}
			if components.Button(c, a.tr("取消", "Cancel"), components.ButtonGhost).Clicked() {
				v.citing = ""
			}
		})
		ui.Text(c, a.tr("引用后，这条也算那个主题的素材，推送时一并带上；它仍属于现在的位置。", "Cited, it is material of that topic too, pushed with it; it stays where it is.")).
			FontSize(12).TextColor(t.TextMuted).Wrap()
	})
}

// searchView lists the entries the query finds, the latest first.
func (a *app) searchView(c *ui.Context) {
	t := c.Theme()
	v := &a.ideas
	found := a.store.search(v.query)
	ui.Column(c).Fill().Children(func() {
		a.pageHeader(c, a.tr("搜索", "Search"),
			fmt.Sprintf(a.tr("「%s」· 找到 %d 条，在原文、出处、补充和来源里", "\"%s\" · %d found in texts, origins, notes and sources"), strings.TrimSpace(v.query), len(found)),
			func() {
				if components.Button(c, a.tr("清除", "Clear"), components.ButtonGhost).Clicked() {
					v.query, v.mode = "", ""
				}
			})
		a.pageError(c)
		ui.Divider(c)
		entryList(c, func() {
			if len(found) == 0 {
				ui.Text(c, a.tr("没有找到。可以换个词，或用空格分开几个词。", "Nothing found. Try another word, or several apart.")).TextColor(t.TextMuted)
			}
			for _, e := range found {
				a.entryView(c, entryContext{where: true}, e)
			}
		})
	})
}

// timelineView lists every entry by the day it was written, the latest
// day first, the day's entries in the order they were written.
func (a *app) timelineView(c *ui.Context) {
	t := c.Theme()
	days := timelineDays(a.store.all())
	ui.Column(c).Fill().Children(func() {
		a.pageHeader(c, a.tr("时间线", "Timeline"),
			fmt.Sprintf(a.tr("所有闪念按记录的日子排列，不分主题 · 共 %d 天", "Every entry by the day it was written, whatever its topic · %d days"), len(days)), nil)
		a.pageError(c)
		ui.Divider(c)
		entryList(c, func() {
			if len(days) == 0 {
				ui.Text(c, a.tr("还没有闪念", "No entries yet")).TextColor(t.TextMuted)
			}
			for _, d := range days {
				ui.Row(c).Key("day:"+d.day).Gap(8).AlignItems(ui.Center).Padding(4, 0, 0, 0).Children(func() {
					ui.Text(c, a.dayLabel(d.at)).FontSize(15).Bold()
					ui.Text(c, fmt.Sprintf(a.tr("%d 条", "%d"), len(d.entries))).FontSize(12).TextColor(t.TextMuted)
				})
				for _, e := range d.entries {
					a.entryView(c, entryContext{where: true}, e)
				}
			}
		})
	})
}

// timelineDay is a day's entries.
type timelineDay struct {
	day     string
	at      time.Time
	entries []*entry
}

// timelineDays groups entries, the latest first, by their local day, the
// latest day first, each day's in the order they were written.
func timelineDays(latestFirst []*entry) []timelineDay {
	var days []timelineDay
	for _, e := range latestFirst {
		at := e.Created.Local()
		day := at.Format("2006-01-02")
		if n := len(days); n == 0 || days[n-1].day != day {
			days = append(days, timelineDay{day: day, at: at})
		}
		d := &days[len(days)-1]
		d.entries = append(d.entries, e)
	}
	for _, d := range days {
		slices.Reverse(d.entries)
	}
	return days
}

// dayLabel names a day: its date and weekday.
func (a *app) dayLabel(at time.Time) string {
	if a.set.Lang == "en-US" {
		return at.Format("Mon, Jan 2, 2006")
	}
	week := []string{"日", "一", "二", "三", "四", "五", "六"}
	return at.Format("2006年1月2日") + " 周" + week[at.Weekday()]
}

// previewView shows the draft a push is to make, before it is made.
func (a *app) previewView(c *ui.Context, plan pushPlan) {
	t := c.Theme()
	v := &a.ideas
	m, err := a.store.planMaterials(plan)
	if err != nil {
		v.preview, v.err = nil, a.ideaErrorText(err)
		return
	}
	d := a.draftText(m)
	ui.Column(c).Fill().Children(func() {
		about := fmt.Sprintf(a.tr("共 %d 条", "%d entries"), len(m.Entries))
		if m.New > 0 {
			about += fmt.Sprintf(a.tr("，上次推送后新增 %d 条", ", %d new since the last push"), m.New)
		}
		if k := len(slices.DeleteFunc(slices.Clone(m.Entries), func(e material) bool { return !e.Cited })); k > 0 {
			about += fmt.Sprintf(a.tr("，引自别处 %d 条", ", %d cited from elsewhere"), k)
		}
		if !m.Pushed.IsZero() {
			about += a.tr("。会另生成一份新草稿，之前的草稿不会改动。", ". This makes a new draft; those made before stay as they are.")
		} else {
			about += a.tr("。生成后打开写作页，并在 AI 助手里填好讨论提示词（不会自动发送）。", ". The write page opens, the AI assistant's prompt filled in, not sent.")
		}
		a.pageHeader(c, fmt.Sprintf(a.tr("预览%s草稿", "Preview the %s draft"), a.refKind(plan.RefType)), about, nil)
		ui.Row(c).Padding(0, 20, 10, 20).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("标题", "Title")).TextColor(t.TextMuted)
			if plan.TopicID == "" {
				components.TextBox(c, &v.preview.Title, components.TextBoxOptions{}).
					Placeholder(a.tr("暂定标题，讨论后再改", "A working title, to change later")).Label(a.tr("草稿标题", "Draft title")).Grow(1).AutoFocus()
				return
			}
			ui.Text(c, m.Topic).FontSize(16).Bold().SingleLine().MinWidth(0)
		})
		a.pageError(c)
		ui.Divider(c)
		entryList(c, func() {
			ui.Text(c, strings.TrimSpace(d.Lead)+d.Intro).Wrap().Selectable()
			for _, e := range m.Entries {
				ui.Column(c).Key("mat:"+e.ID).FillWidth().Gap(6).Padding(10, 12).Radius(t.Radius*1.5).Background(t.Surface).Border(1, t.Border).Children(func() {
					heads, values := d.meta(e)
					ui.Row(c).Gap(12).Wrap().Children(func() {
						for k := range heads {
							ui.Column(c).Gap(1).Children(func() {
								ui.Text(c, heads[k]).FontSize(11).TextColor(t.TextMuted)
								ui.Text(c, values[k]).FontSize(12).SingleLine().MaxWidth(320)
							})
						}
					})
					ui.Text(c, e.Content).Wrap()
					if e.Note != "" {
						ui.Text(c, strings.TrimSpace(d.Note)).FontSize(12).Bold()
						ui.Text(c, e.Note).Wrap()
					}
				})
			}
		})
		ui.Divider(c)
		ui.Row(c).Padding(10, 20).Gap(8).AlignItems(ui.Center).Children(func() {
			if components.Button(c, a.tr("返回", "Back"), components.ButtonGhost).Clicked() {
				v.preview, v.err = nil, ""
			}
			ui.Spacer(c)
			if v.generating {
				components.Spinner(c, components.SpinnerRing).Size(16, 16)
				ui.Text(c, a.tr("正在生成草稿…", "Making the draft…")).TextColor(t.TextMuted)
			}
			ready := !v.generating && len(m.Entries) > 0 && (plan.TopicID != "" || strings.TrimSpace(plan.Title) != "")
			if components.Button(c, fmt.Sprintf(a.tr("生成%s草稿", "Make the %s Draft"), a.refKind(plan.RefType)), components.ButtonPrimary).Disabled(!ready).Clicked() {
				a.generateDraft(*v.preview)
			}
		})
	})
}
