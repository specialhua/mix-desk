package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// 想法一瞬 (ideas): thoughts the user jots down from anywhere (idea_panel.go),
// kept on this computer, gathered under topics, then pushed to the blog as
// drafts of raw material (ideas_draft.go).
//
// Three things, not to be confused:
//   - an entry (闪念) is one thought as the user wrote or pasted it, kept
//     unchanged; it may belong to no topic, which puts it in the inbox
//     (收集箱);
//   - a topic (主题) gathers entries over weeks; its name is not the title
//     of an article, and it stays open after it is pushed;
//   - an article is written on the blog, from what was pushed.
//
// A topic may also cite entries of other topics, or of the inbox, among its
// material (Refs): the entry stays where it is, and its file unchanged.
//
// On disk, under the user data directory:
//
//	ideas/index.json          what the store shares: the user's sources,
//	                          and the drafts made of entries of the inbox
//	ideas/topics/<id>.json    a topic, and the drafts it was pushed to
//	ideas/entries/<id>.json   an entry, one file each, so that moving it
//	                          to another topic changes that file alone
//	ideas/ai/                 (not yet) what an AI makes of the entries,
//	                          apart from them: suggestions of tags, topics
//	                          and links, each naming the entries it rests
//	                          on and whether the user took it
//
// Each file is written whole, through a temporary file and a rename. An
// entry goes to the topic of the ID the user chose, read from disk then:
// never to another one, should that one be gone.

// ideaSchema is the version of the files this app writes.
const ideaSchema = 2

// ideaSources are the sources offered before the user's own: where an
// entry comes from, and nothing more. What it is (a fact, an opinion…)
// and what it may do in an article are not read into it.
var ideaSources = []string{"闪念", "网页", "书籍", "新闻", "论坛"}

// defaultSource is the source of an entry the user says nothing about.
var defaultSource = ideaSources[0]

type topic struct {
	V       int       `json:"v"`
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	// Refs are the entries of elsewhere the topic cites, by ID, the first
	// first: material of the topic, which stays the entry's own topic's.
	Refs []string `json:"refs,omitempty"`
	// Pushes are the drafts made of the topic, the first first.
	Pushes []ideaPush `json:"pushes,omitempty"`
}

// ideaPush is a draft a topic, or entries of the inbox, were pushed to,
// and the entries it held.
type ideaPush struct {
	RefType string    `json:"refType"` // "note" or "post"
	DraftID string    `json:"draftId"`
	At      time.Time `json:"at"`
	Entries []string  `json:"entries"`
	// Title is the draft's title when it is not the topic's: the inbox's.
	Title string `json:"title,omitempty"`
}

func (t *topic) lastPush() *ideaPush {
	if len(t.Pushes) == 0 {
		return nil
	}
	return &t.Pushes[len(t.Pushes)-1]
}

// pushed reports whether an entry went to a draft of the topic.
func (t *topic) pushed(entryID string) bool { return inPushes(t.Pushes, entryID) }

func inPushes(pushes []ideaPush, entryID string) bool {
	for _, p := range pushes {
		if slices.Contains(p.Entries, entryID) {
			return true
		}
	}
	return false
}

// entry is one thought. Content and Note are the user's words, kept as
// they were written; an edit keeps the first version in Original.
type entry struct {
	V       int       `json:"v"`
	ID      string    `json:"id"`
	Created time.Time `json:"created"`
	// TopicID is the topic of the entry, "" for the inbox.
	TopicID string `json:"topicId,omitempty"`
	Source  string `json:"source"`
	Content string `json:"content"`
	// Origin is where an outside text comes from, when the user says.
	Origin *entryOrigin `json:"origin,omitempty"`
	// Note is what the user adds of their own: why they kept it.
	Note     string        `json:"note,omitempty"`
	Edited   *time.Time    `json:"edited,omitempty"`
	Original *entryVersion `json:"original,omitempty"`
}

// entryOrigin is where an entry comes from. The panel fills URL or Title;
// the main window all four.
type entryOrigin struct {
	URL       string `json:"url,omitempty"`
	Title     string `json:"title,omitempty"`
	Author    string `json:"author,omitempty"`
	Published string `json:"published,omitempty"`
}

func (o *entryOrigin) empty() bool {
	return o == nil || *o == entryOrigin{}
}

// clean trims the fields, nil for none.
func (o *entryOrigin) clean() *entryOrigin {
	if o == nil {
		return nil
	}
	c := entryOrigin{
		URL: strings.TrimSpace(o.URL), Title: strings.TrimSpace(o.Title),
		Author: strings.TrimSpace(o.Author), Published: strings.TrimSpace(o.Published),
	}
	if c.empty() {
		return nil
	}
	return &c
}

func (o *entryOrigin) same(p *entryOrigin) bool {
	if o.empty() || p.empty() {
		return o.empty() == p.empty()
	}
	return *o == *p
}

// String is the origin in a line: its title, author and link.
func (o *entryOrigin) String() string {
	if o.empty() {
		return ""
	}
	var parts []string
	for _, s := range []string{o.Title, o.Author, o.Published, o.URL} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " · ")
}

// parseOrigin reads what the user wrote as an origin: a link, or a name.
func parseOrigin(s string) *entryOrigin {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return nil
	case isURL(s):
		return &entryOrigin{URL: s}
	}
	return &entryOrigin{Title: s}
}

func isURL(s string) bool {
	return (strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")) && !strings.ContainsAny(s, " \t\n")
}

// firstURL is the first link in a text, "" for none.
func firstURL(s string) string {
	for _, f := range strings.Fields(s) {
		if i := strings.Index(f, "http"); i >= 0 && isURL(f[i:]) {
			return strings.TrimRight(f[i:], ".,;:!?)）」』。，；：！？")
		}
	}
	return ""
}

// entryVersion is an entry as it was first written.
type entryVersion struct {
	Source  string       `json:"source"`
	Content string       `json:"content"`
	Origin  *entryOrigin `json:"origin,omitempty"`
	Note    string       `json:"note,omitempty"`
}

// ideaIndex is what the store shares: the user's own sources, and the
// drafts made of entries of the inbox.
type ideaIndex struct {
	V       int        `json:"v"`
	Sources []string   `json:"sources,omitempty"`
	Pushes  []ideaPush `json:"pushes,omitempty"`
}

var (
	errTopicGone = errors.New("topic not found")
	errEntryGone = errors.New("entry not found")
	errEmpty     = errors.New("empty")
	errTopicName = errors.New("topic exists")
)

// ideaStore keeps the topics and entries in a directory, and in memory,
// as only this app writes them. Its methods run on the main thread, one
// after another.
type ideaStore struct {
	dir string
	// fresh is set when the store had no index yet: the first launch.
	fresh   bool
	index   ideaIndex
	topics  map[string]*topic
	entries map[string]*entry
	now     func() time.Time
}

func ideasDir() (string, error) {
	dir, err := appUserData()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ideas"), nil
}

// openIdeaStore opens (and creates) the store in dir.
func openIdeaStore(dir string) (*ideaStore, error) {
	for _, d := range []string{dir, filepath.Join(dir, "topics"), filepath.Join(dir, "entries")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	s := &ideaStore{dir: dir, now: time.Now, topics: map[string]*topic{}, entries: map[string]*entry{}}
	data, err := os.ReadFile(s.indexPath())
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.fresh = true
	case err != nil:
		return nil, err
	default:
		_ = json.Unmarshal(data, &s.index)
	}
	s.readAll()
	// The index marks the store made: fresh, the first launch, is the
	// one without it.
	if s.index.V != ideaSchema {
		s.index.V = ideaSchema
		_ = s.saveIndex()
	}
	return s, nil
}

func (s *ideaStore) indexPath() string { return filepath.Join(s.dir, "index.json") }

func (s *ideaStore) topicPath(id string) string {
	return filepath.Join(s.dir, "topics", id+".json")
}

func (s *ideaStore) entryPath(id string) string {
	return filepath.Join(s.dir, "entries", id+".json")
}

func (s *ideaStore) saveIndex() error { return writeJSONFile(s.indexPath(), s.index) }

func (s *ideaStore) saveTopic(t *topic) error {
	t.V = ideaSchema
	if err := writeJSONFile(s.topicPath(t.ID), t); err != nil {
		return err
	}
	s.topics[t.ID] = t
	return nil
}

func (s *ideaStore) saveEntry(e *entry) error {
	e.V = ideaSchema
	if err := writeJSONFile(s.entryPath(e.ID), e); err != nil {
		return err
	}
	s.entries[e.ID] = e
	return nil
}

// readAll reads the topics and entries into memory.
func (s *ideaStore) readAll() {
	read := func(sub string, fn func(id string, data []byte) error) {
		names, _ := filepath.Glob(filepath.Join(s.dir, sub, "*.json"))
		for _, name := range names {
			id := strings.TrimSuffix(filepath.Base(name), ".json")
			if !validIdeaID(id) {
				continue
			}
			data, err := os.ReadFile(name)
			if err == nil {
				err = fn(id, data)
			}
			if err != nil {
				log.Printf("ideas: %s: %v", name, err)
			}
		}
	}
	read("topics", func(id string, data []byte) error {
		var t topic
		if err := json.Unmarshal(data, &t); err != nil {
			return err
		}
		if t.ID == id {
			s.topics[id] = &t
		}
		return nil
	})
	read("entries", func(id string, data []byte) error {
		var e entry
		if err := json.Unmarshal(data, &e); err != nil {
			return err
		}
		if e.ID == id {
			if e.Source == "" {
				e.Source = defaultSource
			}
			s.entries[id] = &e
		}
		return nil
	})
	// An entry of a topic gone is in the inbox.
	for _, e := range s.entries {
		if e.TopicID != "" && s.topics[e.TopicID] == nil {
			e.TopicID = ""
		}
	}
}

// entry returns the entry of an ID, or nil.
func (s *ideaStore) entry(id string) *entry { return s.entries[id] }

// topicTitle names the topic of an ID, the inbox for "".
func (s *ideaStore) topicTitle(id string) string {
	if t := s.topic(id); t != nil {
		return t.Title
	}
	return ""
}

// topicList returns the topics, the latest first.
func (s *ideaStore) topicList() []*topic {
	out := make([]*topic, 0, len(s.topics))
	for _, t := range s.topics {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b *topic) int {
		if c := b.Updated.Compare(a.Updated); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// topic returns the topic of an ID, or nil.
func (s *ideaStore) topic(id string) *topic {
	if id == "" {
		return nil
	}
	return s.topics[id]
}

// loadTopic reads a topic from disk, as it is now.
func (s *ideaStore) loadTopic(id string) (*topic, error) {
	if !validIdeaID(id) {
		return nil, errTopicGone
	}
	data, err := os.ReadFile(s.topicPath(id))
	if errors.Is(err, os.ErrNotExist) {
		delete(s.topics, id)
		return nil, errTopicGone
	}
	if err != nil {
		return nil, err
	}
	var t topic
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	if t.ID != id {
		return nil, errTopicGone
	}
	s.topics[id] = &t
	return &t, nil
}

// entriesOf returns the entries of a topic, "" for the inbox, in the
// order they were written.
func (s *ideaStore) entriesOf(topicID string) []*entry {
	var out []*entry
	for _, e := range s.entries {
		if e.TopicID == topicID {
			out = append(out, e)
		}
	}
	sortEntries(out)
	return out
}

// refsOf returns the entries of elsewhere a topic cites, in the order they
// were written; those gone, or since moved into the topic, are left out.
func (s *ideaStore) refsOf(t *topic) []*entry {
	var out []*entry
	for _, id := range t.Refs {
		if e := s.entries[id]; e != nil && e.TopicID != t.ID {
			out = append(out, e)
		}
	}
	sortEntries(out)
	return out
}

// citedBy returns the topics citing an entry, by title.
func (s *ideaStore) citedBy(entryID string) []*topic {
	var out []*topic
	for _, t := range s.topicList() {
		if slices.Contains(t.Refs, entryID) && s.entries[entryID] != nil && s.entries[entryID].TopicID != t.ID {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b *topic) int { return strings.Compare(a.Title, b.Title) })
	return out
}

// material returns a topic's own entries and those it cites, in the
// order they were written.
func (s *ideaStore) material(t *topic) []*entry {
	out := append(s.entriesOf(t.ID), s.refsOf(t)...)
	sortEntries(out)
	return out
}

func sortEntries(es []*entry) {
	slices.SortFunc(es, func(a, b *entry) int {
		if c := a.Created.Compare(b.Created); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

// all returns every entry, the latest first.
func (s *ideaStore) all() []*entry {
	out := make([]*entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	sortEntries(out)
	slices.Reverse(out)
	return out
}

// search returns the entries holding every word of the query, ignoring
// case, in their content, note, origin or source, the latest first.
func (s *ideaStore) search(query string) []*entry {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	var out []*entry
	for _, e := range s.all() {
		hay := strings.ToLower(strings.Join([]string{e.Content, e.Note, e.Origin.String(), e.Source}, "\n"))
		if !slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(hay, w) }) {
			out = append(out, e)
		}
	}
	return out
}

// count is the number of entries of a topic, "" for the inbox.
func (s *ideaStore) count(topicID string) int {
	n := 0
	for _, e := range s.entries {
		if e.TopicID == topicID {
			n++
		}
	}
	return n
}

// unpushed is the number of entries of a topic, its own and those it
// cites, that went to no draft of it.
func (s *ideaStore) unpushed(t *topic) int {
	n := 0
	for _, e := range s.material(t) {
		if !t.pushed(e.ID) {
			n++
		}
	}
	return n
}

// seqOf numbers an entry within its topic, from 1, for show: an entry is
// known by its ID, not by this number.
func (s *ideaStore) seqOf(e *entry) int {
	for k, o := range s.entriesOf(e.TopicID) {
		if o.ID == e.ID {
			return k + 1
		}
	}
	return 0
}

// findTopic returns the topic of a title, ignoring case and the spaces
// around it, or nil.
func (s *ideaStore) findTopic(title string) *topic {
	title = strings.TrimSpace(title)
	for _, t := range s.topicList() {
		if strings.EqualFold(strings.TrimSpace(t.Title), title) {
			return t
		}
	}
	return nil
}

// createTopic returns the topic of the title, made when there is none.
func (s *ideaStore) createTopic(title string) (*topic, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errEmpty
	}
	if t := s.findTopic(title); t != nil {
		return t, nil
	}
	now := s.now()
	t := &topic{ID: newIdeaID(), Title: title, Created: now, Updated: now}
	return t, s.saveTopic(t)
}

// touch marks a topic changed now, as it is on disk.
func (s *ideaStore) touch(id string) {
	if t, err := s.loadTopic(id); err == nil {
		t.Updated = s.now()
		_ = s.saveTopic(t)
	}
}

// entryInput is what the user writes of an entry. The origin is Origin, a
// line read by parseOrigin, unless its fields are given in OriginParts.
type entryInput struct {
	Source, Content, Origin, Note string
	OriginParts                   *entryOrigin
}

func (in entryInput) clean() entryInput {
	in.Source = orDefault(strings.TrimSpace(in.Source), defaultSource)
	in.Content = strings.TrimSpace(in.Content)
	in.Origin = strings.TrimSpace(in.Origin)
	in.Note = strings.TrimSpace(in.Note)
	return in
}

func (in entryInput) origin() *entryOrigin {
	if in.OriginParts != nil {
		return in.OriginParts.clean()
	}
	return parseOrigin(in.Origin)
}

// addEntry keeps a thought in the topic of the ID, "" for the inbox. The
// topic is read from disk now; one gone is an error, and the entry goes
// to no other.
func (s *ideaStore) addEntry(topicID string, in entryInput) (*entry, error) {
	in = in.clean()
	if in.Content == "" {
		return nil, errEmpty
	}
	if topicID != "" {
		if _, err := s.loadTopic(topicID); err != nil {
			return nil, err
		}
	}
	e := &entry{
		ID: newIdeaID(), Created: s.now(), TopicID: topicID,
		Source: in.Source, Content: in.Content, Origin: in.origin(), Note: in.Note,
	}
	if err := s.saveEntry(e); err != nil {
		return nil, err
	}
	if topicID != "" {
		s.touch(topicID)
	}
	return e, nil
}

// editEntry changes what the user wrote of an entry, keeping its first
// version.
func (s *ideaStore) editEntry(id string, in entryInput) (*entry, error) {
	e, ok := s.entries[id]
	if !ok {
		return nil, errEntryGone
	}
	in = in.clean()
	if in.Content == "" {
		return nil, errEmpty
	}
	origin := in.origin()
	if in.Source == e.Source && in.Content == e.Content && in.Note == e.Note && origin.same(e.Origin) {
		return e, nil
	}
	next := *e
	if next.Original == nil {
		next.Original = &entryVersion{Source: e.Source, Content: e.Content, Origin: e.Origin, Note: e.Note}
	}
	now := s.now()
	next.Source, next.Content, next.Origin, next.Note, next.Edited = in.Source, in.Content, origin, in.Note, &now
	return &next, s.saveEntry(&next)
}

// moveEntry puts an entry in the topic of the ID, "" for the inbox.
func (s *ideaStore) moveEntry(id, topicID string) error {
	e, ok := s.entries[id]
	if !ok {
		return errEntryGone
	}
	if topicID != "" {
		if _, err := s.loadTopic(topicID); err != nil {
			return err
		}
	}
	if e.TopicID == topicID {
		return nil
	}
	next := *e
	next.TopicID = topicID
	if err := s.saveEntry(&next); err != nil {
		return err
	}
	for _, t := range []string{e.TopicID, topicID} {
		if t != "" {
			s.touch(t)
		}
	}
	return nil
}

// moveEntries puts entries in the topic of the ID, "" for the inbox; it
// stops at the first that cannot be moved.
func (s *ideaStore) moveEntries(ids []string, topicID string) error {
	for _, id := range ids {
		if err := s.moveEntry(id, topicID); err != nil {
			return err
		}
	}
	return nil
}

// cite has a topic cite an entry of elsewhere.
func (s *ideaStore) cite(topicID, entryID string) error {
	e := s.entries[entryID]
	if e == nil {
		return errEntryGone
	}
	t, err := s.loadTopic(topicID)
	if err != nil {
		return err
	}
	if e.TopicID == topicID || slices.Contains(t.Refs, entryID) {
		return nil
	}
	t.Refs, t.Updated = append(t.Refs, entryID), s.now()
	return s.saveTopic(t)
}

// uncite has a topic cite an entry no more; the entry stays as it is.
func (s *ideaStore) uncite(topicID, entryID string) error {
	t, err := s.loadTopic(topicID)
	if err != nil {
		return err
	}
	if !slices.Contains(t.Refs, entryID) {
		return nil
	}
	t.Refs = slices.DeleteFunc(t.Refs, func(id string) bool { return id == entryID })
	t.Updated = s.now()
	return s.saveTopic(t)
}

func (s *ideaStore) deleteEntry(id string) error {
	e, ok := s.entries[id]
	if !ok {
		return errEntryGone
	}
	if err := os.Remove(s.entryPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(s.entries, id)
	if e.TopicID != "" {
		s.touch(e.TopicID)
	}
	return nil
}

func (s *ideaStore) renameTopic(id, title string) (*topic, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errEmpty
	}
	if other := s.findTopic(title); other != nil && other.ID != id {
		return nil, errTopicName
	}
	t, err := s.loadTopic(id)
	if err != nil {
		return nil, err
	}
	t.Title, t.Updated = title, s.now()
	return t, s.saveTopic(t)
}

// deleteTopic removes a topic; its entries go back to the inbox.
func (s *ideaStore) deleteTopic(id string) error {
	if !validIdeaID(id) {
		return errTopicGone
	}
	for _, e := range s.entriesOf(id) {
		next := *e
		next.TopicID = ""
		if err := s.saveEntry(&next); err != nil {
			return err
		}
	}
	err := os.Remove(s.topicPath(id))
	delete(s.topics, id)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// recordPush keeps the draft a topic was pushed to, with its entries.
func (s *ideaStore) recordPush(id, refType, draftID string, entryIDs []string) (*topic, error) {
	if id == "" {
		return nil, errTopicGone
	}
	t, err := s.loadTopic(id)
	if err != nil {
		return nil, err
	}
	t.Pushes = append(t.Pushes, ideaPush{RefType: refType, DraftID: draftID, At: s.now(), Entries: entryIDs})
	return t, s.saveTopic(t)
}

// recordInboxPush keeps a draft made of entries of the inbox, titled.
func (s *ideaStore) recordInboxPush(refType, draftID, title string, entryIDs []string) error {
	s.index.Pushes = append(s.index.Pushes, ideaPush{RefType: refType, DraftID: draftID, At: s.now(), Entries: entryIDs, Title: title})
	return s.saveIndex()
}

// inboxPushes are the drafts made of entries of the inbox, the first first.
func (s *ideaStore) inboxPushes() []ideaPush { return s.index.Pushes }

// sources are the preset sources, then the user's own.
func (s *ideaStore) sources() []string {
	out := slices.Clone(ideaSources)
	for _, c := range s.index.Sources {
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// addSource keeps a source of the user's; the name is returned trimmed,
// "" for an empty one.
func (s *ideaStore) addSource(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 12 {
		return ""
	}
	if !slices.Contains(s.sources(), name) {
		s.index.Sources = append(s.index.Sources, name)
		_ = s.saveIndex()
	}
	return name
}

func newIdeaID() string { return time.Now().Format("20060102-150405") + "-" + randomHex(4) }

// validIdeaID keeps an ID to a file name of the store.
func validIdeaID(id string) bool {
	if id == "" || id == "index" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// writeJSONFile writes v whole, through a temporary file and a rename, so
// that a crash leaves the old file or the new one, never half of one.
func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// orDefault is s, or def when s is empty.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
