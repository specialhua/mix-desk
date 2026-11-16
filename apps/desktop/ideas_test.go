package main

import (
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// testStore is a store in a temporary directory, whose clock ticks a
// minute a call from 2026-11-16 09:00.
func testStore(t *testing.T) *ideaStore {
	t.Helper()
	s, err := openIdeaStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.Local)
	s.now = func() time.Time { now = now.Add(time.Minute); return now }
	return s
}

func TestIdeaInboxAndTopics(t *testing.T) {
	s := testStore(t)
	if !s.fresh {
		t.Error("a new store is not fresh")
	}
	// A line alone, no topic: the inbox.
	q, err := s.addEntry("", entryInput{Content: " AI 该不该有改生产配置的权限？ "})
	if err != nil {
		t.Fatal(err)
	}
	if q.TopicID != "" || q.Source != "闪念" || q.Content != "AI 该不该有改生产配置的权限？" {
		t.Errorf("inbox entry = %+v", q)
	}
	ops, _ := s.createTopic("AI 自动化运维")
	other, _ := s.createTopic("城市与步行")
	if same, _ := s.createTopic("  ai 自动化运维 "); same.ID != ops.ID {
		t.Error("a topic made twice")
	}
	s.addEntry(ops.ID, entryInput{Source: "论坛", Content: "其实就是 AI 幻觉", Origin: "https://v2ex.com/t/1", Note: "我配过负载均衡，想想后怕"})
	s.addEntry(other.ID, entryInput{Content: "街道是给人走的"})

	// Sorted later: the inbox entry joins the topic, in the order written.
	if err := s.moveEntry(q.ID, ops.ID); err != nil {
		t.Fatal(err)
	}
	// Reopened from disk, as at the next launch.
	s2, err := openIdeaStore(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.entriesOf(ops.ID)
	if len(got) != 2 || got[0].ID != q.ID || got[1].Source != "论坛" || s2.count("") != 0 || s2.count(other.ID) != 1 {
		t.Fatalf("after the move: %+v", got)
	}
	if e := got[1]; e.Origin.URL != "https://v2ex.com/t/1" || e.Note != "我配过负载均衡，想想后怕" {
		t.Errorf("origin and note: %+v", e)
	}
	if s2.seqOf(got[0]) != 1 || s2.seqOf(got[1]) != 2 || s2.fresh {
		t.Errorf("seq %d, fresh %v", s2.seqOf(got[0]), s2.fresh)
	}
	// A topic deleted gives its entries back to the inbox.
	if err := s2.deleteTopic(ops.ID); err != nil {
		t.Fatal(err)
	}
	s3, _ := openIdeaStore(s.dir)
	if s3.count("") != 2 || s3.topic(ops.ID) != nil || len(s3.topicList()) != 1 {
		t.Errorf("after deleting the topic: inbox %d, topics %d", s3.count(""), len(s3.topicList()))
	}
	if _, err := s3.renameTopic(other.ID, " 步行城市 "); err != nil || s3.topic(other.ID).Title != "步行城市" {
		t.Errorf("rename: %v", err)
	}
}

func TestIdeaAddNeverFallsBack(t *testing.T) {
	s := testStore(t)
	a, _ := s.createTopic("A")
	b, _ := s.createTopic("B")
	// B is gone from disk, behind the store's back.
	os.Remove(s.topicPath(b.ID))
	if _, err := s.addEntry(b.ID, entryInput{Content: "x"}); !errors.Is(err, errTopicGone) {
		t.Errorf("add to a removed topic: %v", err)
	}
	for _, id := range []string{"index", "../x", "A", "new"} {
		if _, err := s.addEntry(id, entryInput{Content: "x"}); err == nil {
			t.Errorf("add to %q succeeded", id)
		}
	}
	if _, err := s.addEntry(a.ID, entryInput{Content: "  "}); err == nil {
		t.Error("an empty entry")
	}
	if n := len(s.entries); n != 0 {
		t.Errorf("%d entries went somewhere", n)
	}
}

func TestIdeaEditKeepsTheOriginal(t *testing.T) {
	s := testStore(t)
	e, _ := s.addEntry("", entryInput{Source: "网页", Content: "原文，有错别子"})
	s.editEntry(e.ID, entryInput{Source: "网页", Content: "原文，有错别字", Note: "补一句"})
	s.editEntry(e.ID, entryInput{Source: "书籍", Content: "第三版"})
	s2, _ := openIdeaStore(s.dir)
	got := s2.entries[e.ID]
	if got.Content != "第三版" || got.Source != "书籍" || got.Edited == nil || got.Original == nil ||
		got.Original.Content != "原文，有错别子" || got.Original.Source != "网页" || got.Original.Note != "" {
		t.Errorf("edited = %+v, original %+v", got, got.Original)
	}
	if !got.Created.Equal(e.Created) {
		t.Error("an edit changed when it was written")
	}
	// Unchanged: no edit.
	f, _ := s.addEntry("", entryInput{Content: "x", Origin: "《置身事内》"})
	if g, _ := s.editEntry(f.ID, entryInput{Content: "x ", Origin: "《置身事内》"}); g.Edited != nil {
		t.Error("an edit of nothing")
	}
}

func TestIdeaOrigin(t *testing.T) {
	if o := parseOrigin("https://example.com/a?b=1"); o.URL == "" || o.Title != "" {
		t.Errorf("link: %+v", o)
	}
	if o := parseOrigin(" 《置身事内》兰小欢 "); o.Title != "《置身事内》兰小欢" || o.URL != "" {
		t.Errorf("name: %+v", o)
	}
	if parseOrigin(" ") != nil {
		t.Error("an empty origin")
	}
	for in, want := range map[string]string{
		"看这个（https://v2ex.com/t/12）。": "https://v2ex.com/t/12",
		"链接:https://a.example/x, 好":   "https://a.example/x",
		"没有链接":                        "",
	} {
		if got := firstURL(in); got != want {
			t.Errorf("firstURL(%q) = %q; want %q", in, got, want)
		}
	}
	if s := s(&entryOrigin{Title: "帖子", URL: "https://x"}); s != "帖子 · https://x" {
		t.Errorf("origin text = %q", s)
	}
}

func s(o *entryOrigin) string { return o.String() }

func TestIdeaOriginParts(t *testing.T) {
	st := testStore(t)
	e, _ := st.addEntry("", entryInput{Content: "x", Origin: "https://a.example/p"})
	parts := &entryOrigin{URL: " https://a.example/p ", Title: "一篇文章", Author: " 某人 ", Published: "2026-11-01"}
	g, err := st.editEntry(e.ID, entryInput{Content: "x", OriginParts: parts})
	if err != nil || g.Edited == nil || *g.Origin != (entryOrigin{URL: "https://a.example/p", Title: "一篇文章", Author: "某人", Published: "2026-11-01"}) {
		t.Fatalf("parts: %+v, %v", g.Origin, err)
	}
	if g.Original.Origin.URL != "https://a.example/p" || g.Original.Origin.Title != "" {
		t.Errorf("original origin: %+v", g.Original.Origin)
	}
	if h, _ := st.editEntry(e.ID, entryInput{Content: "x", OriginParts: parts}); h.Edited != g.Edited {
		t.Error("the same parts again made an edit")
	}
	if h, _ := st.editEntry(e.ID, entryInput{Content: "x", OriginParts: &entryOrigin{Author: "  "}}); h.Origin != nil {
		t.Errorf("parts cleared: %+v", h.Origin)
	}
}

func TestIdeaCite(t *testing.T) {
	st := testStore(t)
	ops, _ := st.createTopic("运维")
	city, _ := st.createTopic("城市")
	own, _ := st.addEntry(ops.ID, entryInput{Content: "运维的一条"})
	in, _ := st.addEntry("", entryInput{Content: "收集箱的一条"})
	other, _ := st.addEntry(city.ID, entryInput{Content: "城市的一条"})
	for _, id := range []string{other.ID, in.ID, other.ID} {
		if err := st.cite(ops.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.cite(ops.ID, own.ID); err != nil || len(st.topic(ops.ID).Refs) != 2 {
		t.Errorf("refs = %v, %v", st.topic(ops.ID).Refs, err)
	}
	st2, _ := openIdeaStore(st.dir)
	tp := st2.topic(ops.ID)
	mat := st2.material(tp)
	if len(mat) != 3 || mat[0].ID != own.ID || mat[2].ID != other.ID || st2.count(ops.ID) != 1 || st2.entry(other.ID).TopicID != city.ID {
		t.Fatalf("material = %v", mat)
	}
	if by := st2.citedBy(other.ID); len(by) != 1 || by[0].ID != ops.ID {
		t.Errorf("cited by %v", by)
	}
	m := st2.materialsOf(tp)
	if !m.Entries[1].Cited || m.Entries[1].From != "" || m.Entries[2].From != "城市" || m.Entries[0].Cited {
		t.Errorf("materials = %+v", m.Entries)
	}
	a := &app{set: settings{Lang: "zh-CN"}}
	d := a.draftText(m)
	md := ideaMarkdown(m, d)
	if !strings.Contains(md, "| 引自 |") || !strings.Contains(md, "| 3 | 2026-11-16 09:06 | 闪念 | 城市 |") || !strings.Contains(md, "| 收集箱 |") || !strings.Contains(d.Intro, "标「引自」") {
		t.Errorf("markdown =\n%s", md)
	}
	// Moved into the topic, it is its own; cited no more, it stays.
	st2.moveEntry(other.ID, ops.ID)
	if len(st2.refsOf(tp)) != 1 || len(st2.citedBy(other.ID)) != 0 {
		t.Error("an entry of the topic cited by it")
	}
	if err := st2.uncite(ops.ID, in.ID); err != nil || len(st2.refsOf(st2.topic(ops.ID))) != 0 || st2.entry(in.ID) == nil {
		t.Errorf("uncite: %v", err)
	}
	// A cited entry deleted is gone from the topic too.
	st2.cite(city.ID, own.ID)
	st2.deleteEntry(own.ID)
	if len(st2.material(st2.topic(city.ID))) != 0 {
		t.Error("a deleted entry still cited")
	}
}

func TestIdeaSearchAndTimeline(t *testing.T) {
	st := testStore(t)
	tp, _ := st.createTopic("运维")
	st.addEntry("", entryInput{Content: "AI 改生产配置", Note: "后怕"})
	st.addEntry(tp.ID, entryInput{Content: "负载均衡", Origin: "https://v2ex.com/t/1"})
	st.addEntry(tp.ID, entryInput{Source: "书籍", Content: "置身事内"})
	for q, want := range map[string]int{"ai": 1, "后怕": 1, "V2EX": 1, "书籍": 1, "生产 后怕": 1, "生产 均衡": 0, "  ": 0, "": 0} {
		if got := len(st.search(q)); got != want {
			t.Errorf("search %q: %d; want %d", q, got, want)
		}
	}
	all := st.all()
	if len(all) != 3 || all[0].Content != "置身事内" {
		t.Errorf("all = %v", all)
	}
	// Two days: the latest first, each day's in the order written.
	all[0].Created = all[0].Created.AddDate(0, 0, 1)
	days := timelineDays(all)
	if len(days) != 2 || len(days[0].entries) != 1 || days[1].entries[0].Content != "AI 改生产配置" {
		t.Errorf("days = %+v", days)
	}
	a := &app{set: settings{Lang: "zh-CN"}}
	if got := a.dayLabel(time.Date(2026, 11, 17, 8, 0, 0, 0, time.Local)); got != "2026年11月17日 周二" {
		t.Errorf("day = %q", got)
	}
}

func TestIdeaPushSome(t *testing.T) {
	st := testStore(t)
	tp, _ := st.createTopic("运维")
	x, _ := st.addEntry(tp.ID, entryInput{Content: "一"})
	y, _ := st.addEntry(tp.ID, entryInput{Content: "二"})
	i1, _ := st.addEntry("", entryInput{Content: "甲"})
	i2, _ := st.addEntry("", entryInput{Content: "乙"})
	if err := st.moveEntries([]string{y.ID}, ""); err != nil || st.count("") != 3 {
		t.Fatalf("move: %v", err)
	}
	st.moveEntries([]string{y.ID}, tp.ID)

	m, err := st.planMaterials(pushPlan{TopicID: tp.ID, IDs: []string{y.ID}})
	if err != nil || !m.Some || len(m.Entries) != 1 || m.Entries[0].Seq != 1 || m.Topic != "运维" {
		t.Fatalf("some = %+v, %v", m, err)
	}
	if all, _ := st.planMaterials(pushPlan{TopicID: tp.ID}); all.Some || len(all.Entries) != 2 {
		t.Errorf("all = %+v", all)
	}
	st.recordPush(tp.ID, refNote, "d1", m.entryIDs())
	if st.unpushed(st.topic(tp.ID)) != 1 || st.topic(tp.ID).pushed(x.ID) {
		t.Error("the entry not pushed counted pushed")
	}

	// Some of the inbox, under a title of the user's, kept in the index.
	m, _ = st.planMaterials(pushPlan{Title: " 收集箱里的两条 ", IDs: []string{i2.ID, i1.ID, "gone"}})
	if !m.Inbox || m.Topic != "收集箱里的两条" || len(m.Entries) != 2 || m.Entries[0].ID != i1.ID || m.Entries[0].Cited {
		t.Fatalf("inbox = %+v", m)
	}
	a := &app{set: settings{Lang: "zh-CN"}}
	if d := a.draftText(m); !strings.Contains(d.Intro, "从收集箱里挑出的 2 条") || !strings.Contains(d.Intro, "标题是暂定的") {
		t.Errorf("intro = %q", d.Intro)
	}
	if p := a.ideaPrompt(m); !strings.Contains(p, "我从收集箱里挑出的 2 条") {
		t.Errorf("prompt = %q", p)
	}
	if err := st.recordInboxPush(refPost, "d2", m.Topic, m.entryIDs()); err != nil {
		t.Fatal(err)
	}
	st2, _ := openIdeaStore(st.dir)
	if p := st2.inboxPushes(); len(p) != 1 || p[0].Title != "收集箱里的两条" || len(p[0].Entries) != 2 {
		t.Errorf("inbox pushes = %+v", p)
	}
	m, _ = st2.planMaterials(pushPlan{Title: "再推", IDs: []string{i1.ID, i2.ID}})
	if m.New != 0 || m.Pushed.IsZero() {
		t.Errorf("pushed again = %+v", m)
	}
}

func TestIdeaPushes(t *testing.T) {
	st := testStore(t)
	tp, _ := st.createTopic("城市与步行")
	var ids []string
	for _, text := range []string{"一", "二"} {
		e, _ := st.addEntry(tp.ID, entryInput{Content: text})
		ids = append(ids, e.ID)
	}
	tp = st.topic(tp.ID)
	if m := st.materialsOf(tp); m.New != 0 || m.Entries[0].New || !m.Pushed.IsZero() {
		t.Errorf("never pushed: %+v", m)
	}
	st.recordPush(tp.ID, refNote, "d1", ids)
	e3, _ := st.addEntry(tp.ID, entryInput{Content: "三"})
	st2, _ := openIdeaStore(st.dir)
	tp = st2.topic(tp.ID)
	if st2.unpushed(tp) != 1 || tp.lastPush().DraftID != "d1" {
		t.Fatalf("pushes = %+v", tp.Pushes)
	}
	m := st2.materialsOf(tp)
	if m.New != 1 || !m.Entries[2].New || m.Entries[2].ID != e3.ID || m.Entries[0].New || len(m.entryIDs()) != 3 {
		t.Errorf("materials = %+v", m)
	}
	a := &app{set: settings{Lang: "zh-CN"}}
	d := a.draftText(m)
	if !strings.Contains(d.Intro, "其中标「新增」的 1 条") {
		t.Errorf("intro = %q", d.Intro)
	}
	if md := ideaMarkdown(m, d); !strings.Contains(md, "| 3 | 2026-11-16 09:07 | 闪念 | 新增 |") || strings.Count(md, "| 新增 |") != 1 {
		t.Errorf("markdown =\n%s", md)
	}
	if p := a.ideaPrompt(m); !strings.Contains(p, "1 条是上次推送后新记的") {
		t.Errorf("prompt = %q", p)
	}
	st2.recordPush(tp.ID, refPost, "d2", m.entryIDs())
	if st2.unpushed(st2.topic(tp.ID)) != 0 {
		t.Error("all pushed")
	}
}

// testMaterials are two entries, the second of two lines and a note.
func testMaterials() materials {
	at := time.Date(2026, 11, 16, 9, 5, 0, 0, time.Local)
	return materials{Topic: "城市与步行", Entries: []material{
		{Seq: 1, ID: "a", At: at, Source: "网页", Origin: "https://x.example", Content: "街道是给人走的"},
		{Seq: 2, ID: "b", At: at.Add(time.Hour), Source: "闪念", Content: "第一行\n第二行\n\n\n另起一段 a|b", Note: "我的看法"},
	}}
}

func TestIdeaDraftContent(t *testing.T) {
	m := testMaterials()
	a := &app{set: settings{Lang: "zh-CN"}}
	d := a.draftText(m)
	if !strings.Contains(d.Intro, "「城市与步行」积累的 2 条闪念素材（2026-11-16 至 2026-11-16）") || !strings.Contains(d.Intro, "不是正式文章") {
		t.Errorf("intro = %q", d.Intro)
	}
	md := ideaMarkdown(m, d)
	want := "**素材说明：**" + d.Intro +
		"\n\n---\n\n" +
		"| 序号 | 记录时间 | 来源 | 出处 |\n| --- | --- | --- | --- |\n| 1 | 2026-11-16 09:05 | 网页 | https://x.example |\n\n街道是给人走的" +
		"\n\n---\n\n" +
		"| 序号 | 记录时间 | 来源 |\n| --- | --- | --- |\n| 2 | 2026-11-16 10:05 | 闪念 |\n\n第一行  \n第二行\n\n另起一段 a|b\n\n**我的补充：**\n\n我的看法"
	if md != want {
		t.Errorf("markdown =\n%s\nwant\n%s", md, want)
	}
	lex := ideaLexical(m, d)
	type node struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Format   any    `json:"format"`
		Children []node `json:"children"`
	}
	var doc struct{ Root node }
	if err := json.Unmarshal([]byte(lex), &doc); err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, n := range doc.Root.Children {
		types = append(types, n.Type)
	}
	// intro | table, content | table, two paragraphs, the note's label and
	// paragraph | the last paragraph.
	if got := strings.Join(types, " "); got != "paragraph horizontalrule table paragraph horizontalrule table paragraph paragraph paragraph paragraph paragraph" {
		t.Fatalf("root: %s", got)
	}
	if lead := doc.Root.Children[0].Children[0]; lead.Text != "素材说明：" || lead.Format != float64(1) {
		t.Errorf("lead: %+v", lead)
	}
	if row := doc.Root.Children[2].Children[1]; len(row.Children) != 4 || row.Children[3].Children[0].Children[0].Text != "https://x.example" {
		t.Errorf("table: %+v", row)
	}
	if p := doc.Root.Children[6]; len(p.Children) != 3 || p.Children[1].Type != "linebreak" {
		t.Errorf("lines: %+v", p)
	}
	if os.Getenv("LEXICAL_SAMPLE") != "" {
		os.WriteFile(os.Getenv("LEXICAL_SAMPLE"), []byte(lex), 0o644)
	}

	body := draftBody(m, d, refPost)
	data := body["data"].(map[string]any)
	if body["refType"] != "post" || data["contentFormat"] != "lexical" || data["title"] != "城市与步行" {
		t.Errorf("body = %+v", body)
	}
	if got := draftEditRoute(refPost, "42"); got != "/posts/edit?draftId=42" {
		t.Errorf("route = %q", got)
	}
	if got := draftEditRoute(refNote, "42"); got != "/notes/edit?draftId=42" {
		t.Errorf("route = %q", got)
	}
	if got := draftExpect(m); got != "街道是给人走的" {
		t.Errorf("expect = %q", got)
	}
	p := a.ideaPrompt(m)
	for _, want := range []string{"「城市与步行」", "2 条闪念素材", "不是文章", "请先不要写正文", "2–4 个不同的写作方向", "不要只凭来源下结论"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q: %q", want, p)
		}
	}
	if strings.Contains(p, "新增") {
		t.Errorf("a first push has nothing new: %q", p)
	}
}

func TestIdeaPanelChoice(t *testing.T) {
	st := testStore(t)
	a := &app{set: settings{Lang: "zh-CN"}, store: st}
	tp, _ := st.createTopic("AI 自动化运维")

	// Summoned afresh: the inbox.
	a.panel.choice = tp.ID
	a.prepareIdeaPanel("", false)
	if a.panel.choice != inboxChoice || a.panel.source != "闪念" {
		t.Errorf("fresh: %+v", a.panel)
	}
	// The user picks a topic and keeps one thought after another.
	a.panel.choice, a.panel.source, a.panel.content = tp.ID, "论坛", "一"
	a.saveIdeaEntry(true)
	if a.panel.choice != tp.ID || a.panel.source != "闪念" || a.panel.content != "" || !strings.Contains(a.panel.saved, "「AI 自动化运维」第 1 条") {
		t.Errorf("after keeping: %+v", a.panel)
	}
	a.panel.content = "二"
	a.saveIdeaEntry(true)
	if st.count(tp.ID) != 2 {
		t.Errorf("topic has %d", st.count(tp.ID))
	}
	// Hidden halfway through a thought: its choice stays.
	a.panel.content = "写了一半"
	a.prepareIdeaPanel("", false)
	if a.panel.choice != tp.ID {
		t.Error("lost the choice of a thought not yet kept")
	}
	// Asked for a topic: that one.
	a.panel.content = ""
	a.prepareIdeaPanel(tp.ID, false)
	if a.panel.choice != tp.ID {
		t.Error("ignored the topic asked for")
	}
	// A topic gone: the inbox.
	st.deleteTopic(tp.ID)
	a.prepareIdeaPanel("", true)
	if a.panel.choice != inboxChoice {
		t.Error("kept a topic gone")
	}
	a.panel.content = "收集箱里的一条"
	a.saveIdeaEntry(true)
	if !strings.Contains(a.panel.saved, "收集箱") || st.count("") != 3 {
		t.Errorf("inbox: %q, %d", a.panel.saved, st.count(""))
	}
	// A new topic needs its name.
	a.panel.choice, a.panel.content = newTopicChoice, "x"
	a.saveIdeaEntry(true)
	if a.panel.err == "" {
		t.Error("a new topic without a name")
	}
}

func TestPageResult(t *testing.T) {
	r := pageResult{Status: 201, Body: `{"data":{"id":"7"}}`}
	var d struct{ ID string }
	if err := r.err(); err != nil {
		t.Fatal(err)
	}
	if err := r.data(&d); err != nil || d.ID != "7" {
		t.Errorf("data = %+v, %v", d, err)
	}
	err := pageResult{Status: 401, Body: `{"error":{"code":"AUTH_NOT_LOGGED_IN","message":"未登录"}}`}.err()
	if !isUnauthorized(err) {
		t.Errorf("401: %v", err)
	}
	var ae *apiError
	err = pageResult{Status: 400, Body: `{"error":{"code":"VALIDATION_FAILED","message":"bad"}}`}.err()
	if !errors.As(err, &ae) || ae.Message != "bad" {
		t.Errorf("400: %#v", err)
	}
	if err := (pageResult{Error: "Failed to fetch"}).err(); err == nil || isUnauthorized(err) {
		t.Errorf("network: %v", err)
	}
	s, _ := pageFetchScript("POST", "https://s.example/api/v3/drafts", nil, map[string]string{"a": "</script>"})
	for _, want := range []string{`"https://s.example/api/v3/drafts"`, `"credentials":"include"`, `"content-type":"application/json"`} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %s:\n%s", want, s)
		}
	}
}

func TestAcceleratorText(t *testing.T) {
	got, want := acceleratorText("CmdOrCtrl+Alt+N"), "Ctrl+Alt+N"
	if runtime.GOOS == "darwin" {
		want = "⌥⌘N"
	}
	if got != want {
		t.Errorf("acceleratorText = %q; want %q", got, want)
	}
	if runtime.GOOS == "darwin" {
		for acc, want := range map[string]string{defaultIdeaShortcut: "⌃⌥N", "Ctrl+Alt+Cmd+N": "⌃⌥⌘N", "Alt+Shift+Space": "⌥⇧Space"} {
			if got := acceleratorText(acc); got != want {
				t.Errorf("acceleratorText(%q) = %q; want %q", acc, got, want)
			}
		}
	}
	a := &app{}
	if a.ideaShortcut() != defaultIdeaShortcut {
		t.Errorf("default = %q", a.ideaShortcut())
	}
	a.set.IdeaShortcut = ideaShortcutOff
	if a.ideaShortcut() != "" {
		t.Errorf("off = %q", a.ideaShortcut())
	}
}

func TestShortcutDraft(t *testing.T) {
	a := &app{set: settings{Lang: "zh-CN"}}
	d := draftOf("Ctrl+Alt+Shift+Cmd+K")
	if !d.ctrl || !d.alt || !d.shift || !d.cmd || d.key != "K" || d.accelerator() != "Ctrl+Alt+Shift+Cmd+K" {
		t.Errorf("draft = %+v", d)
	}
	if d := draftOf("Alt+Shift+Space"); d.accelerator() != "Alt+Shift+Space" || a.shortcutProblem(d) != "" {
		t.Errorf("space = %+v", d)
	}
	for acc, bad := range map[string]bool{"Shift+K": true, "Cmd+K": true, "Cmd+Space": false, "F5": false, "Ctrl+Alt+N": false, "Alt+N": false} {
		if got := a.shortcutProblem(draftOf(acc)) != ""; got != bad {
			t.Errorf("%s: problem %v; want %v", acc, got, bad)
		}
	}
	if (shortcutDraft{ctrl: true}).accelerator() != "" || a.shortcutProblem(shortcutDraft{ctrl: true}) == "" {
		t.Error("no key")
	}
	// The settings show the shortcut in force, and the user's own.
	a.set.IdeaShortcut = "Ctrl+Alt+Shift+J"
	tt := ui.NewTester(a.ideaShortcutSetting, 420, 120)
	if runtime.GOOS == "darwin" && !tt.HasText("⌃⌥⇧J") {
		t.Errorf("settings: %q", tt.Texts())
	}
	a.shortcutDraft = draftOf("Ctrl+Alt+Shift+J")
	a.shortcutDraft.open = true
	tt = ui.NewTester(a.ideaShortcutSetting, 420, 160)
	if !tt.HasText("将使用 "+acceleratorText("Ctrl+Alt+Shift+J")) || !tt.HasText("使用") {
		t.Errorf("custom: %q", tt.Texts())
	}
	saveShot(t, "idea-shortcut-custom", tt)
}

func TestPreloadAiPrompt(t *testing.T) {
	s := preloadScript("https://a.example", "/proxy/qaqdmin", "zh-CN", themeChoice{})
	for _, want := range []string{"__mxDesktopAiPrompt", `aria-label="AI 助手"`, `placeholder="输入修改要求"`} {
		if !strings.Contains(s, want) {
			t.Errorf("the script lacks %s", want)
		}
	}
}

func TestIdeaScreens(t *testing.T) {
	st := testStore(t)
	a := &app{set: settings{Lang: "zh-CN"}, store: st}
	a.prepareIdeaPanel("", false)
	tt := ui.NewTester(a.ideaPanelView, ideaPanelWidth, ideaPanelHeight)
	for _, want := range []string{"想法一瞬", "记到：", "收集箱", "闪念", "论坛", "出处 / 补充"} {
		if !tt.HasText(want) {
			t.Errorf("empty panel lacks %q: %q", want, tt.Texts())
		}
	}
	saveShot(t, "idea-panel-inbox", tt)

	tp, _ := st.createTopic("AI 自动化运维")
	a.panel.choice, a.panel.more = tp.ID, true
	a.panel.content, a.panel.origin = "AI 执行命令是否应该拥有修改生产环境配置的权限？", "https://v2ex.com/t/1"
	tt = ui.NewTester(a.ideaPanelView, ideaPanelWidth, ideaPanelHeight)
	if !tt.HasText("AI 自动化运维") || !tt.HasText("· 已有 0 条") {
		t.Errorf("panel of a topic: %q", tt.Texts())
	}
	saveShot(t, "idea-panel-more", tt)
	a.panel.source, a.panel.note = "论坛", "我配过负载均衡，想想后怕"
	a.saveIdeaEntry(true)
	st.addEntry("", entryInput{Content: "某个软件的权限设计不合理"})
	e, _ := st.addEntry(tp.ID, entryInput{Content: "自动化系统出事，责任归谁？"})
	st.recordPush(tp.ID, refNote, "d1", []string{st.entriesOf(tp.ID)[0].ID})
	st.editEntry(e.ID, entryInput{Content: "自动化系统出了事，责任该归谁？"})

	city, _ := st.createTopic("城市与步行")
	walk, _ := st.addEntry(city.ID, entryInput{Source: "书籍", Content: "街道是给人走的", OriginParts: &entryOrigin{Title: "《美国大城市的死与生》", Author: "简·雅各布斯"}})
	st.cite(tp.ID, walk.ID)
	st.touch(tp.ID)
	a.ideas.selected = tp.ID
	tt = ui.NewTester(a.ideasView, 900, 640)
	for _, want := range []string{"收集箱 · 1", "时间线", "AI 自动化运维 · 2 · 新增 2", "出处：https://v2ex.com/t/1", "我的补充", "已推送 1 次", "新增", "已编辑 · 原文已保留", "引自 城市与步行", "推送为手记草稿…", "推送为博文草稿…", "多选"} {
		if !tt.HasText(want) {
			t.Errorf("topic page lacks %q: %q", want, tt.Texts())
		}
	}
	saveShot(t, "ideas-topic", tt)

	a.ideas.history = true
	tt = ui.NewTester(a.ideasView, 900, 640)
	if !tt.HasText("第 1 次 · 2026-11-16 09:") || !tt.HasText("推送记录") {
		t.Errorf("history: %q", tt.Texts())
	}

	a.ideas.preview = &pushPlan{TopicID: tp.ID, RefType: refPost}
	tt = ui.NewTester(a.ideasView, 900, 640)
	for _, want := range []string{"预览博文草稿", "共 3 条，上次推送后新增 2 条，引自别处 1 条", "会另生成一份新草稿", "素材说明：", "引自", "生成博文草稿"} {
		if !tt.HasText(want) {
			t.Errorf("preview lacks %q: %q", want, tt.Texts())
		}
	}
	saveShot(t, "ideas-preview", tt)

	a.ideas.show(city.ID)
	tt = ui.NewTester(a.ideasView, 900, 640)
	if !tt.HasText("也被引用于：「AI 自动化运维」") || !tt.HasText("出处：《美国大城市的死与生》 · 简·雅各布斯") {
		t.Errorf("cited entry: %q", tt.Texts())
	}

	a.ideas.show("")
	a.ideas.selecting, a.ideas.picked = true, map[string]bool{}
	for _, e := range st.entriesOf("") {
		a.ideas.picked[e.ID] = true
	}
	tt = ui.NewTester(a.ideasView, 900, 640)
	for _, want := range []string{"已选 1 条", "全不选", "移动", "推送为博文草稿…"} {
		if !tt.HasText(want) {
			t.Errorf("selection lacks %q: %q", want, tt.Texts())
		}
	}
	saveShot(t, "ideas-select", tt)
	a.ideas.selecting = false

	a.ideas.query = "权限"
	a.ideas.mode = ideasSearch
	tt = ui.NewTester(a.ideasView, 900, 640)
	if !tt.HasText("找到 2 条") || !tt.HasText("在主题中查看") {
		t.Errorf("search: %q", tt.Texts())
	}
	saveShot(t, "ideas-search", tt)
	a.ideas.locate(st.entry(e.ID))
	if a.ideas.mode != "" || a.ideas.selected != tp.ID || a.ideas.focus != e.ID {
		t.Errorf("locate: %+v", a.ideas)
	}

	a.ideas.mode = ideasTimeline
	tt = ui.NewTester(a.ideasView, 900, 640)
	if !tt.HasText("2026年11月16日 周一") || !tt.HasText("城市与步行") {
		t.Errorf("timeline: %q", tt.Texts())
	}
	saveShot(t, "ideas-timeline", tt)
	a.ideas.mode = ""

	a.ideas.show("")
	tt = ui.NewTester(a.ideasView, 900, 640)
	if !tt.HasText("某个软件的权限设计不合理") || tt.HasText("推送为博文草稿") {
		t.Errorf("inbox page: %q", tt.Texts())
	}
	saveShot(t, "ideas-inbox", tt)

	// Not signed in: no draft, and say why.
	a.generateDraft(pushPlan{TopicID: tp.ID, RefType: refNote})
	if a.ideas.err == "" || a.ideas.generating {
		t.Errorf("made a draft signed out: %+v", a.ideas)
	}

	a.set.SidebarTab = sideTabExtras
	a.extraPage = extraIdeas
	tt = ui.NewTester(a.sidebarView, 260, 400)
	if !tt.HasText("想法一瞬") || tt.HasText("仪表盘") {
		t.Errorf("extras tab: %q", tt.Texts())
	}
	saveShot(t, "sidebar-extras", tt)
	tt = ui.NewTester(a.contentTitle, 600, 48)
	if !tt.HasText("附加") || !tt.HasText("想法一瞬") {
		t.Errorf("title: %q", tt.Texts())
	}
}

// saveShot saves what tt drew to $SCREENSHOTS/name.png, as TestScreens does.
func saveShot(t *testing.T, name string, tt *ui.Tester) {
	dir := os.Getenv("SCREENSHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}
