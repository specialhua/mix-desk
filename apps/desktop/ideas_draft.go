package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/oneo-me/onevra/source/components"
)

// A topic is pushed to a draft of a note or a post, on the server, in the
// Lexical format: the write page's AI assistant works on Lexical drafts
// only. The draft holds the topic's entries as raw material, said to be
// so, not an article; its title is the topic's, for the user to change.
// The app opens the draft's write page, opens the assistant and fills in a
// prompt there that asks it to discuss the material first, which it does
// not send (inject/preload.js, __mxDesktopAiPrompt).
//
// The entries become materials first, then the draft's text, its Lexical
// content and the prompt: what an AI of the app reads one day should be
// the same materials.
//
// A topic pushed again makes another draft, of all its entries, the new
// ones marked: the drafts made before, which the user may have changed,
// are not touched. The user may push some of a topic's entries alone, or
// some of the inbox's, under a title they give: no topic is made for them.

const (
	refNote = "note"
	refPost = "post"
)

// ideaTimeLayout is how an entry's time is written.
const ideaTimeLayout = "2006-01-02 15:04"

// aiPromptTimeout is how long the page has to show the assistant's input.
const aiPromptTimeout = 20 * time.Second

// material is an entry as a draft shows it.
type material struct {
	Seq     int
	ID      string
	At      time.Time
	Source  string
	Origin  string
	Content string
	Note    string
	// New is set for an entry not in the drafts made of the topic before.
	New bool
	// Cited is set for an entry of elsewhere the topic cites, From the
	// title of the topic it belongs to, "" for the inbox.
	Cited bool
	From  string
}

// materials are entries to push, in the order they were written: a
// topic's, its own and those it cites, or some of them, or some of the
// inbox's.
type materials struct {
	// Topic is the draft's title: the topic's, or the user's for the inbox.
	Topic   string
	Entries []material
	// Inbox is set for entries of the inbox; Some for some of a topic's.
	Inbox, Some bool
	// Pushed is when the topic was last pushed, zero for never; New is the
	// number of entries since.
	Pushed time.Time
	New    int
}

// pushPlan is what the user is about to push: a topic, "" for the inbox;
// the entries chosen, all of the topic's for nil; and the draft's kind and,
// for the inbox, its title.
type pushPlan struct {
	TopicID, Title, RefType string
	IDs                     []string
}

func (m materials) entryIDs() []string {
	ids := make([]string, len(m.Entries))
	for k, e := range m.Entries {
		ids[k] = e.ID
	}
	return ids
}

// materialsOf are the topic's entries as they are now, its own and those
// it cites.
func (s *ideaStore) materialsOf(t *topic) materials {
	return s.materialsFrom(t.Title, t.ID, s.material(t), t.Pushes)
}

// materialsFrom numbers entries, from 1, marking those of no push before
// and those of a topic other than own.
func (s *ideaStore) materialsFrom(title, own string, es []*entry, pushes []ideaPush) materials {
	m := materials{Topic: title}
	if len(pushes) > 0 {
		m.Pushed = pushes[len(pushes)-1].At
	}
	for k, e := range es {
		mat := material{Seq: k + 1, ID: e.ID, At: e.Created, Source: e.Source, Origin: e.Origin.String(), Content: e.Content, Note: e.Note}
		if own != "" && e.TopicID != own {
			mat.Cited, mat.From = true, s.topicTitle(e.TopicID)
		}
		if !m.Pushed.IsZero() && !inPushes(pushes, e.ID) {
			mat.New = true
			m.New++
		}
		m.Entries = append(m.Entries, mat)
	}
	return m
}

// planMaterials are the materials of a plan, as the store holds them now.
func (s *ideaStore) planMaterials(p pushPlan) (materials, error) {
	pick := func(es []*entry) []*entry {
		if p.IDs == nil {
			return es
		}
		return slices.DeleteFunc(es, func(e *entry) bool { return !slices.Contains(p.IDs, e.ID) })
	}
	if p.TopicID == "" {
		var es []*entry
		for _, id := range p.IDs {
			if e := s.entry(id); e != nil {
				es = append(es, e)
			}
		}
		sortEntries(es)
		m := s.materialsFrom(strings.TrimSpace(p.Title), "", es, s.inboxPushes())
		m.Inbox = true
		return m, nil
	}
	t, err := s.loadTopic(p.TopicID)
	if err != nil {
		return materials{}, err
	}
	all := s.material(t)
	m := s.materialsFrom(t.Title, t.ID, pick(slices.Clone(all)), t.Pushes)
	m.Some = p.IDs != nil && len(m.Entries) < len(all)
	return m, nil
}

// draftText are the words of a draft, in the app's language.
type draftText struct {
	// Intro is the paragraph that says what the draft is, after Lead in
	// bold.
	Lead, Intro                      string
	Seq, Time, Source, Origin, State string
	New, Note, Cited, Inbox          string
}

func (a *app) draftText(m materials) draftText {
	from, to := "", ""
	if n := len(m.Entries); n > 0 {
		from, to = m.Entries[0].At.Local().Format("2006-01-02"), m.Entries[n-1].At.Local().Format("2006-01-02")
	}
	what := fmt.Sprintf(a.tr("围绕主题「%s」积累的 %d 条闪念素材", "%[2]d raw notes gathered under the topic \"%[1]s\""), m.Topic, len(m.Entries))
	switch {
	case m.Inbox:
		what = fmt.Sprintf(a.tr("从收集箱里挑出的 %d 条闪念素材", "%d raw notes picked from my inbox"), len(m.Entries))
	case m.Some:
		what = fmt.Sprintf(a.tr("从主题「%s」里挑出的 %d 条闪念素材", "%[2]d raw notes picked from the topic \"%[1]s\""), m.Topic, len(m.Entries))
	}
	d := draftText{
		Lead: a.tr("素材说明：", "About this draft: "),
		Intro: fmt.Sprintf(a.tr(
			"以下是%s（%s 至 %s），按记录时间排列，未经整理，不是正式文章。表格里的「来源」只是素材的出处类型；「我的补充」是我当时写下的看法，其余是原文。",
			"These are %s (%s to %s), in the order they were written: material, not an article. In the tables, the source is where a note comes from, nothing more; \"My note\" is what I thought of it then, the rest is the original text.",
		), what, from, to),
		Seq: a.tr("序号", "No."), Time: a.tr("记录时间", "Written"), Source: a.tr("来源", "Source"),
		Origin: a.tr("出处", "Origin"), State: a.tr("状态", "State"), New: a.tr("新增", "New"),
		Note: a.tr("我的补充：", "My note: "), Cited: a.tr("引自", "From"), Inbox: a.tr("收集箱", "Inbox"),
	}
	if slices.ContainsFunc(m.Entries, func(e material) bool { return e.Cited }) {
		d.Intro += a.tr("标「引自」的是这个主题引用的、属于别处的素材。", " Those marked \"From\" belong elsewhere, cited by this topic.")
	}
	if m.Inbox {
		d.Intro += a.tr("标题是暂定的，讨论后再定。", " The title is a working one, to settle once discussed.")
	} else {
		d.Intro += a.tr("标题暂用主题名，讨论后再定。", " The title is the topic's for now, to settle once discussed.")
	}
	if m.New > 0 {
		d.Intro += fmt.Sprintf(a.tr(
			"其中标「新增」的 %d 条是上次推送（%s）之后新记的。",
			" The %d marked \"New\" came after the last push (%s).",
		), m.New, m.Pushed.Local().Format(ideaTimeLayout))
	}
	return d
}

// meta are the headings of a material's table and its values under them.
func (d draftText) meta(e material) (heads, values []string) {
	heads = []string{d.Seq, d.Time, d.Source}
	values = []string{fmt.Sprint(e.Seq), e.At.Local().Format(ideaTimeLayout), e.Source}
	if e.Origin != "" {
		heads, values = append(heads, d.Origin), append(values, e.Origin)
	}
	if e.Cited {
		heads, values = append(heads, d.Cited), append(values, orDefault(e.From, d.Inbox))
	}
	if e.New {
		heads, values = append(heads, d.State), append(values, d.New)
	}
	return heads, values
}

// entryBlocks splits content into its paragraphs, at blank lines, and each
// paragraph into its lines.
func entryBlocks(content string) [][]string {
	var out [][]string
	var cur []string
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if len(cur) > 0 {
				out = append(out, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// ideaMarkdown is the draft's text, as ideaLexical lays it out.
func ideaMarkdown(m materials, d draftText) string {
	cell := func(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
	paras := func(b *strings.Builder, text string) {
		for _, lines := range entryBlocks(text) {
			b.WriteString("\n\n" + strings.Join(lines, "  \n"))
		}
	}
	parts := []string{"**" + strings.TrimSpace(d.Lead) + "**" + d.Intro}
	for _, e := range m.Entries {
		heads, values := d.meta(e)
		for k := range values {
			values[k] = cell(values[k])
		}
		var b strings.Builder
		b.WriteString("| " + strings.Join(heads, " | ") + " |\n")
		b.WriteString(strings.Repeat("| --- ", len(heads)) + "|\n")
		b.WriteString("| " + strings.Join(values, " | ") + " |")
		paras(&b, e.Content)
		if e.Note != "" {
			b.WriteString("\n\n**" + strings.TrimSpace(d.Note) + "**")
			paras(&b, e.Note)
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// Lexical's serialized nodes: the plain ones, and the table and horizontal
// rule the admin's editor (@haklex/rich-headless) knows.
type lexText struct {
	Detail  int    `json:"detail"`
	Format  int    `json:"format"`
	Mode    string `json:"mode"`
	Style   string `json:"style"`
	Text    string `json:"text,omitempty"`
	Type    string `json:"type"`
	Version int    `json:"version"`
}

// lexLeaf is a node of a type alone: a line break, a horizontal rule.
type lexLeaf struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
}

type lexElement struct {
	Children   []any   `json:"children"`
	Direction  *string `json:"direction"`
	Format     string  `json:"format"`
	Indent     int     `json:"indent"`
	Type       string  `json:"type"`
	Version    int     `json:"version"`
	TextFormat *int    `json:"textFormat,omitempty"`
	TextStyle  *string `json:"textStyle,omitempty"`
}

// lexCell is a table cell: an element with its span, header state and
// background, which Lexical writes null for none.
type lexCell struct {
	lexElement
	BackgroundColor *string `json:"backgroundColor"`
	ColSpan         int     `json:"colSpan"`
	RowSpan         int     `json:"rowSpan"`
	HeaderState     int     `json:"headerState"`
}

func lexParagraph(children []any) lexElement {
	// A paragraph's text format is its first text's, as Lexical keeps it.
	format, empty := 0, ""
	if children == nil {
		children = []any{}
	}
	if len(children) > 0 {
		if t, ok := children[0].(lexText); ok {
			format = t.Format
		}
	}
	return lexElement{Children: children, Type: "paragraph", Version: 1, TextFormat: &format, TextStyle: &empty}
}

func lexTextNode(s string) lexText {
	return lexText{Mode: "normal", Text: s, Type: "text", Version: 1}
}

// lexBold is text in bold.
func lexBold(s string) lexText {
	t := lexTextNode(s)
	t.Format = 1
	return t
}

// lexLines is a paragraph of lines, kept with line breaks.
func lexLines(lines []string) lexElement {
	var kids []any
	for k, line := range lines {
		if k > 0 {
			kids = append(kids, lexLeaf{Type: "linebreak", Version: 1})
		}
		if line != "" {
			kids = append(kids, lexTextNode(line))
		}
	}
	return lexParagraph(kids)
}

// lexTable is a table of a header row and rows of values.
func lexTable(heads []string, rows ...[]string) lexElement {
	row := func(cells []string, header int) lexElement {
		r := lexElement{Type: "tablerow", Version: 1, Children: []any{}}
		for _, v := range cells {
			r.Children = append(r.Children, lexCell{
				lexElement: lexElement{Type: "tablecell", Version: 1, Children: []any{lexParagraph([]any{lexTextNode(v)})}},
				ColSpan:    1, RowSpan: 1, HeaderState: header,
			})
		}
		return r
	}
	t := lexElement{Type: "table", Version: 1, Children: []any{row(heads, 1)}}
	for _, r := range rows {
		t.Children = append(t.Children, row(r, 0))
	}
	return t
}

// ideaLexical is the draft's content: what it is, then for each entry,
// the table of its number, time, source and origin, its paragraphs and the
// user's note; a horizontal rule between them.
func ideaLexical(m materials, d draftText) string {
	hr := lexLeaf{Type: "horizontalrule", Version: 1}
	nodes := []any{lexParagraph([]any{lexBold(strings.TrimSpace(d.Lead)), lexTextNode(d.Intro)})}
	for _, e := range m.Entries {
		nodes = append(nodes, hr)
		heads, values := d.meta(e)
		nodes = append(nodes, lexTable(heads, values))
		for _, lines := range entryBlocks(e.Content) {
			nodes = append(nodes, lexLines(lines))
		}
		if e.Note != "" {
			nodes = append(nodes, lexParagraph([]any{lexBold(strings.TrimSpace(d.Note))}))
			for _, lines := range entryBlocks(e.Note) {
				nodes = append(nodes, lexLines(lines))
			}
		}
	}
	// Lexical wants a paragraph to end on, where the caret can go.
	nodes = append(nodes, lexParagraph(nil))
	root := lexElement{Children: nodes, Type: "root", Version: 1}
	data, _ := json.Marshal(map[string]any{"root": root})
	return string(data)
}

// ideaPrompt is what the app fills in the AI assistant: to discuss the
// material before anything is written.
func (a *app) ideaPrompt(m materials) string {
	what := fmt.Sprintf(a.tr("围绕「%s」积累的", "gathered on \"%s\""), m.Topic)
	if m.Inbox {
		what = a.tr("从收集箱里挑出的", "picked from my inbox")
	} else if m.Some {
		what = fmt.Sprintf(a.tr("从「%s」里挑出的", "picked from \"%s\""), m.Topic)
	}
	p := fmt.Sprintf(a.tr(
		"这篇草稿是我%[1]s %[2]d 条闪念素材，未经整理，不是文章。每条上方的表格写着序号、记录时间和来源（只是出处类型），「我的补充」是我当时的看法，其余是原文。",
		"This draft holds %[2]d raw notes %[1]s: material, not an article. The table above each gives its number, when I wrote it and its source (where it comes from, nothing more); \"My note\" is what I thought then, the rest is the original text.",
	), what, len(m.Entries))
	if m.New > 0 {
		p += fmt.Sprintf(a.tr("其中标「新增」的 %d 条是上次推送后新记的，请留意它们带来的变化。", " The %d marked \"New\" came since I last pushed this topic: mind what they change."), m.New)
	}
	return p + a.tr(`
请先不要写正文，而是：
1. 概括这些碎片主要在思考什么；
2. 找出它们之间的共同点、矛盾和关联，引用序号；
3. 区分其中的事实、我的判断和未经证实的推测，不要只凭来源下结论；
4. 给出 2–4 个不同的写作方向，每个附暂定标题和核心论点；
5. 指出还缺哪些事实、经历或论证。
然后等我选择或反驳。`, `
Do not write the article yet. First:
1. Say what these fragments are mainly thinking about.
2. Find what they share, where they conflict and how they connect, citing their numbers.
3. Tell the facts from my judgments and from unproven guesses; do not go by the source alone.
4. Offer 2–4 different directions to write in, each with a working title and its main argument.
5. Say what facts, experience or reasoning are missing.
Then wait for me to choose or push back.`)
}

// draftBody is the request that creates the topic's draft.
func draftBody(m materials, d draftText, refType string) map[string]any {
	return map[string]any{
		"refType": refType,
		"data": map[string]any{
			"title":         m.Topic,
			"text":          ideaMarkdown(m, d),
			"content":       ideaLexical(m, d),
			"contentFormat": "lexical",
		},
	}
}

// draftExpect is a text of the draft the editor shows once it has loaded
// it: the start of the first entry's content.
func draftExpect(m materials) string {
	if len(m.Entries) == 0 {
		return ""
	}
	blocks := entryBlocks(m.Entries[0].Content)
	if len(blocks) == 0 {
		return ""
	}
	r := []rune(blocks[0][0])
	return string(r[:min(len(r), 20)])
}

// draftEditRoute is the write page of a new draft.
func draftEditRoute(refType, draftID string) string {
	page := "/notes/edit"
	if refType == refPost {
		page = "/posts/edit"
	}
	return page + "?draftId=" + draftID
}

// generateDraft pushes what the plan says to a new draft, and opens it.
func (a *app) generateDraft(plan pushPlan) {
	v := &a.ideas
	if v.generating {
		return
	}
	v.err = ""
	fail := func(msg string) {
		v.err = msg
		a.invalidateMain()
	}
	if !a.signedIn() {
		fail(a.tr("请先登录后台，再生成草稿", "Sign in to the admin before making a draft"))
		return
	}
	if plan.TopicID == "" && strings.TrimSpace(plan.Title) == "" {
		fail(a.tr("先给草稿起个暂定标题", "Give the draft a working title first"))
		return
	}
	m, err := a.store.planMaterials(plan)
	if err != nil {
		fail(a.ideaErrorText(err))
		return
	}
	if len(m.Entries) == 0 {
		fail(a.tr("没有可推送的素材", "There is nothing to push"))
		return
	}
	v.generating = true
	a.invalidateMain()
	page, api, prompt := a.web.Page(), a.srv.API, a.ideaPrompt(m)
	body := draftBody(m, a.draftText(m), plan.RefType)
	go func() {
		res, err := pageFetch(page, http.MethodPost, api+"/drafts", nil, body)
		var draft struct {
			ID string `json:"id"`
		}
		if err == nil {
			err = res.data(&draft)
		}
		if err == nil && draft.ID == "" {
			err = fmt.Errorf("the server sent no draft ID")
		}
		a.win.Update(func() {
			v.generating = false
			if err != nil {
				v.err = a.tr("生成草稿失败：", "Cannot make the draft: ") + a.apiErrorText(err)
				return
			}
			if plan.TopicID == "" {
				err = a.store.recordInboxPush(plan.RefType, draft.ID, m.Topic, m.entryIDs())
			} else {
				_, err = a.store.recordPush(plan.TopicID, plan.RefType, draft.ID, m.entryIDs())
			}
			if err != nil {
				log.Printf("ideas: record the push of %q: %v", plan.TopicID, err)
			}
			v.preview, v.selecting, v.picked = nil, false, nil
			a.ideasChanged()
			a.notify(components.ToastSuccess, "idea-draft", a.tr("已生成", "Made a ")+a.refKind(plan.RefType)+a.tr("草稿", " draft"), m.Topic)
			a.openDraft(plan.RefType, draft.ID, prompt, draftExpect(m))
		})
	}()
}

// refKind names a kind of draft.
func (a *app) refKind(refType string) string {
	if refType == refPost {
		return a.tr("博文", "post")
	}
	return a.tr("手记", "note")
}

// openPushedDraft opens a draft pushed before, unless it is gone:
// published or deleted on the blog.
func (a *app) openPushedDraft(p ideaPush) {
	v := &a.ideas
	if !a.signedIn() {
		v.err = a.tr("请先登录后台", "Sign in to the admin first")
		a.invalidateMain()
		return
	}
	page, api := a.web.Page(), a.srv.API
	go func() {
		res, err := pageFetch(page, http.MethodGet, api+"/drafts/"+p.DraftID, nil, nil)
		if err == nil {
			err = res.err()
		}
		a.win.Update(func() {
			var ae *apiError
			switch {
			case errors.As(err, &ae) && ae.Status == http.StatusNotFound:
				v.err = a.tr("这份草稿已不在了，可能已发布或删除；可以再生成一份。", "That draft is gone, published or deleted. Make another.")
			case err != nil:
				v.err = a.apiErrorText(err)
			default:
				a.openDraft(p.RefType, p.DraftID, "", "")
			}
		})
	}()
}

// openDraft shows the draft's write page, then fills in the AI prompt,
// when given, once the editor shows expect, a text of the draft.
func (a *app) openDraft(refType, draftID, prompt, expect string) {
	a.extraPage = ""
	route := draftEditRoute(refType, draftID)
	a.nav = activeNavPath(route)
	a.navigate(route)
	a.showMain()
	if prompt == "" || a.web == nil {
		return
	}
	page := a.web.Page()
	p, _ := json.Marshal(prompt)
	e, _ := json.Marshal(expect)
	go func() {
		got, err := mygo.EvalAs[string](page, fmt.Sprintf(
			`return await window.__mxDesktopAiPrompt(%s, %d, %s)`, p, aiPromptTimeout.Milliseconds(), e))
		if err == nil && got == "ok" {
			return
		}
		if err != nil {
			log.Printf("ai prompt: %v", err)
		}
		mygo.Clipboard.WriteText(prompt)
		a.win.Update(func() {
			a.notify(components.ToastInfo, "idea-ai", a.tr("未能自动打开 AI 助手", "Could not open the AI assistant"),
				a.tr("提示词已复制，可粘贴到 AI 助手里。", "The prompt is on the clipboard: paste it into the assistant."))
		})
	}()
}
