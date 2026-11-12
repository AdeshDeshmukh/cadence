package storage

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Memory struct {
	mu          sync.RWMutex
	transcripts []Transcript
	vocabulary  map[string]string
	recoveries  map[string]SessionRecovery
	snippets    map[string]Snippet
	profiles    map[string]PolishProfile
	meetings    map[string]Meeting
	segments    map[string][]MeetingSegment
	speakers    map[string]map[string]MeetingSpeaker
	summaries   map[string]MeetingSummary
	folders     map[string]Folder
	notes       map[string]Note
	noteTags    map[string]map[string]struct{}
	tags        map[string]time.Time
	embeddings  map[string]EmbeddingRecord
}

func NewMemory() *Memory {
	return &Memory{
		vocabulary: map[string]string{},
		recoveries: map[string]SessionRecovery{},
		snippets:   map[string]Snippet{},
		profiles:   map[string]PolishProfile{},
		meetings:   map[string]Meeting{},
		segments:   map[string][]MeetingSegment{},
		speakers:   map[string]map[string]MeetingSpeaker{},
		summaries:  map[string]MeetingSummary{},
		folders:    map[string]Folder{},
		notes:      map[string]Note{},
		noteTags:   map[string]map[string]struct{}{},
		tags:       map[string]time.Time{},
		embeddings: map[string]EmbeddingRecord{},
	}
}

func (m *Memory) SaveTranscript(t Transcript) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transcripts = append(m.transcripts, t)
	return nil
}

func (m *Memory) ListTranscripts(limit int) ([]Transcript, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := append([]Transcript(nil), m.transcripts...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})

	if limit <= 0 || limit > len(out) {
		limit = len(out)
	}
	return out[:limit], nil
}

func (m *Memory) GetTranscript(id string) (*Transcript, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, t := range m.transcripts {
		if t.ID == id {
			cp := t
			return &cp, nil
		}
	}
	return nil, errors.New("transcript not found")
}

func (m *Memory) SetVocabulary(e VocabularyEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vocabulary[e.Spoken] = e.Canonical
	return nil
}

func (m *Memory) Vocabulary() ([]VocabularyEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]VocabularyEntry, 0, len(m.vocabulary))
	for spoken, canonical := range m.vocabulary {
		out = append(out, VocabularyEntry{
			Spoken: spoken, Canonical: canonical,
		})
	}
	return out, nil
}

func (m *Memory) DeleteVocabulary(spoken string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.vocabulary, spoken)
	return nil
}

func (m *Memory) SaveRecovery(r SessionRecovery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.ID == "" {
		return errors.New("recovery id cannot be empty")
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = time.Now()
	}
	m.recoveries[r.ID] = r
	return nil
}

func (m *Memory) GetLastRecovery() (*SessionRecovery, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matches []SessionRecovery
	for _, r := range m.recoveries {
		if r.State != "completed" && (r.Text != "" || r.RawText != "" || r.PartialText != "" || r.Error != "") {
			matches = append(matches, r)
		}
	}
	if len(matches) == 0 {
		return nil, nil
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].UpdatedAt.After(matches[j].UpdatedAt)
	})

	res := matches[0]
	return &res, nil
}

func (m *Memory) ListRecoveries(limit int) ([]SessionRecovery, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matches []SessionRecovery
	for _, r := range m.recoveries {
		if r.State != "completed" && (r.Text != "" || r.RawText != "" || r.PartialText != "" || r.Error != "") {
			matches = append(matches, r)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].UpdatedAt.After(matches[j].UpdatedAt)
	})

	if limit <= 0 || limit > len(matches) {
		limit = len(matches)
	}
	return matches[:limit], nil
}

func (m *Memory) DeleteRecovery(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.recoveries, id)
	return nil
}

func (m *Memory) ClearRecoveries() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recoveries = make(map[string]SessionRecovery)
	return nil
}

func (m *Memory) SetSnippet(s Snippet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(s.Trigger) == "" {
		return errors.New("snippet trigger cannot be empty")
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	m.snippets[s.Trigger] = s
	return nil
}

func (m *Memory) GetSnippet(trigger string) (*Snippet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snip, ok := m.snippets[trigger]
	if !ok {
		return nil, nil
	}
	res := snip
	return &res, nil
}

func (m *Memory) ListSnippets() ([]Snippet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Snippet, 0, len(m.snippets))
	for _, snip := range m.snippets {
		out = append(out, snip)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Trigger < out[j].Trigger
	})
	return out, nil
}

func (m *Memory) DeleteSnippet(trigger string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.snippets, trigger)
	return nil
}

func (m *Memory) SetPolishProfile(p PolishProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return errors.New("profile name cannot be empty")
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	p.Name = name
	m.profiles[name] = p
	return nil
}

func (m *Memory) GetPolishProfile(name string) (*PolishProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.profiles[name]
	if !ok {
		return nil, nil
	}
	res := p
	return &res, nil
}

func (m *Memory) ListPolishProfiles() ([]PolishProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]PolishProfile, 0, len(m.profiles))
	for _, p := range m.profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (m *Memory) DeletePolishProfile(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.profiles, name)
	return nil
}

func (m *Memory) SaveMeeting(mtg Meeting) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.meetings[mtg.ID] = mtg
	return nil
}

func (m *Memory) GetMeeting(id string) (*Meeting, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mtg, ok := m.meetings[id]
	if !ok {
		return nil, nil
	}
	res := mtg
	return &res, nil
}

func (m *Memory) ListMeetings(limit int) ([]Meeting, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Meeting, 0, len(m.meetings))
	for _, mtg := range m.meetings {
		out = append(out, mtg)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) DeleteMeeting(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.meetings, id)
	delete(m.segments, id)
	delete(m.speakers, id)
	delete(m.summaries, id)
	return nil
}

func (m *Memory) SaveMeetingSegment(seg MeetingSegment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.segments[seg.MeetingID]
	for i, existing := range list {
		if existing.ID == seg.ID {
			list[i] = seg
			m.segments[seg.MeetingID] = list
			return nil
		}
	}
	m.segments[seg.MeetingID] = append(list, seg)
	return nil
}

func (m *Memory) ListMeetingSegments(meetingID string) ([]MeetingSegment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	segs := m.segments[meetingID]
	out := make([]MeetingSegment, len(segs))
	copy(out, segs)
	sort.Slice(out, func(i, j int) bool {
		return out[i].SegmentIndex < out[j].SegmentIndex
	})
	return out, nil
}

func (m *Memory) SetMeetingSpeaker(spk MeetingSpeaker) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if spk.MeetingID == "" || spk.SpeakerID == "" {
		return errors.New("meeting_id and speaker_id are required")
	}
	if spk.UpdatedAt.IsZero() {
		spk.UpdatedAt = time.Now()
	}
	if m.speakers[spk.MeetingID] == nil {
		m.speakers[spk.MeetingID] = make(map[string]MeetingSpeaker)
	}
	m.speakers[spk.MeetingID][spk.SpeakerID] = spk
	return nil
}

func (m *Memory) ListMeetingSpeakers(meetingID string) ([]MeetingSpeaker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	spkMap := m.speakers[meetingID]
	out := make([]MeetingSpeaker, 0, len(spkMap))
	for _, spk := range spkMap {
		out = append(out, spk)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SpeakerID < out[j].SpeakerID
	})
	return out, nil
}

func (m *Memory) RenameMeetingSpeaker(meetingID string, fromSpeaker string, toName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if meetingID == "" || fromSpeaker == "" || toName == "" {
		return errors.New("meeting_id, fromSpeaker, and toName are required")
	}

	if m.speakers[meetingID] == nil {
		m.speakers[meetingID] = make(map[string]MeetingSpeaker)
	}
	m.speakers[meetingID][fromSpeaker] = MeetingSpeaker{
		MeetingID: meetingID,
		SpeakerID: fromSpeaker,
		Name:      toName,
		UpdatedAt: time.Now(),
	}

	list := m.segments[meetingID]
	for i := range list {
		if list[i].Speaker == fromSpeaker {
			list[i].Speaker = toName
		}
	}
	m.segments[meetingID] = list
	return nil
}

func (m *Memory) SaveMeetingSummary(s MeetingSummary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.MeetingID == "" {
		return errors.New("meeting_id is required")
	}
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	m.summaries[s.MeetingID] = s
	return nil
}

func (m *Memory) GetMeetingSummary(meetingID string) (*MeetingSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.summaries[meetingID]
	if !ok {
		return nil, nil
	}
	res := s
	return &res, nil
}

func (m *Memory) DeleteMeetingSummary(meetingID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.summaries, meetingID)
	return nil
}

// SaveFolder creates or updates a folder in memory.
func (m *Memory) SaveFolder(f Folder) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(f.ID) == "" {
		return errors.New("folder id is required")
	}
	if strings.TrimSpace(f.Name) == "" {
		return errors.New("folder name is required")
	}
	now := time.Now()
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	f.UpdatedAt = now
	m.folders[f.ID] = f
	return nil
}

// GetFolder retrieves a folder by id.
func (m *Memory) GetFolder(id string) (*Folder, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.folders[id]
	if !ok {
		return nil, nil
	}
	res := f
	return &res, nil
}

// ListFolders lists all folders sorted alphabetically by name.
func (m *Memory) ListFolders() ([]Folder, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Folder, 0, len(m.folders))
	for _, f := range m.folders {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// DeleteFolder deletes a folder and detaches associated notes.
func (m *Memory) DeleteFolder(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.folders, id)
	for k, n := range m.notes {
		if n.FolderID == id {
			n.FolderID = ""
			m.notes[k] = n
		}
	}
	return nil
}

// SaveNote creates or updates a note in memory.
func (m *Memory) SaveNote(n Note) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(n.ID) == "" {
		return errors.New("note id is required")
	}
	now := time.Now()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	n.UpdatedAt = now

	// If tags were provided in struct, sync them
	if n.Tags != nil {
		tagSet := make(map[string]struct{})
		for _, t := range n.Tags {
			clean := strings.ToLower(strings.TrimSpace(t))
			if clean != "" {
				tagSet[clean] = struct{}{}
				if _, exists := m.tags[clean]; !exists {
					m.tags[clean] = now
				}
			}
		}
		m.noteTags[n.ID] = tagSet
	} else if existingTags, ok := m.noteTags[n.ID]; ok {
		// Populate tags from existing map
		tags := make([]string, 0, len(existingTags))
		for t := range existingTags {
			tags = append(tags, t)
		}
		sort.Strings(tags)
		n.Tags = tags
	}

	m.notes[n.ID] = n
	return nil
}

// GetNote retrieves a single note by id with its tags.
func (m *Memory) GetNote(id string) (*Note, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, ok := m.notes[id]
	if !ok {
		return nil, nil
	}
	res := n
	if tagSet, ok := m.noteTags[id]; ok {
		tags := make([]string, 0, len(tagSet))
		for t := range tagSet {
			tags = append(tags, t)
		}
		sort.Strings(tags)
		res.Tags = tags
	} else {
		res.Tags = []string{}
	}
	return &res, nil
}

// ListNotes retrieves notes matching the given filter.
func (m *Memory) ListNotes(filter NoteFilter) ([]Note, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	targetTag := strings.ToLower(strings.TrimSpace(filter.Tag))
	out := make([]Note, 0, len(m.notes))

	for id, n := range m.notes {
		if filter.FolderID != "" && n.FolderID != filter.FolderID {
			continue
		}
		if filter.MeetingID != "" && n.MeetingID != filter.MeetingID {
			continue
		}
		if targetTag != "" {
			tagSet := m.noteTags[id]
			if tagSet == nil {
				continue
			}
			if _, ok := tagSet[targetTag]; !ok {
				continue
			}
		}

		item := n
		if tagSet, ok := m.noteTags[id]; ok {
			tags := make([]string, 0, len(tagSet))
			for t := range tagSet {
				tags = append(tags, t)
			}
			sort.Strings(tags)
			item.Tags = tags
		} else {
			item.Tags = []string{}
		}
		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})

	if filter.Limit > 0 && filter.Limit < len(out) {
		out = out[:filter.Limit]
	}

	return out, nil
}

// DeleteNote removes a note and its tag associations.
func (m *Memory) DeleteNote(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.notes, id)
	delete(m.noteTags, id)
	return nil
}

// ListTags lists all tags with note count.
func (m *Memory) ListTags() ([]Tag, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	counts := make(map[string]int)
	for _, tagSet := range m.noteTags {
		for t := range tagSet {
			counts[t]++
		}
	}

	out := make([]Tag, 0, len(m.tags))
	for name, createdAt := range m.tags {
		out = append(out, Tag{
			Name:      name,
			NoteCount: counts[name],
			CreatedAt: createdAt,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out, nil
}

// SetNoteTags associates a set of tags with a note.
func (m *Memory) SetNoteTags(noteID string, tags []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if strings.TrimSpace(noteID) == "" {
		return errors.New("note id is required")
	}

	now := time.Now()
	tagSet := make(map[string]struct{})
	cleanTags := make([]string, 0, len(tags))

	for _, t := range tags {
		clean := strings.ToLower(strings.TrimSpace(t))
		if clean != "" {
			tagSet[clean] = struct{}{}
			cleanTags = append(cleanTags, clean)
			if _, exists := m.tags[clean]; !exists {
				m.tags[clean] = now
			}
		}
	}

	m.noteTags[noteID] = tagSet
	if n, ok := m.notes[noteID]; ok {
		sort.Strings(cleanTags)
		n.Tags = cleanTags
		n.UpdatedAt = now
		m.notes[noteID] = n
	}

	return nil
}

// GetNoteTags returns tags associated with a note.
func (m *Memory) GetNoteTags(noteID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tagSet, ok := m.noteTags[noteID]
	if !ok {
		return []string{}, nil
	}
	out := make([]string, 0, len(tagSet))
	for t := range tagSet {
		out = append(out, t)
	}
	sort.Strings(out)
	return out, nil
}

// DeleteTag removes a tag from the system and all notes.
func (m *Memory) DeleteTag(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clean := strings.ToLower(strings.TrimSpace(name))
	delete(m.tags, clean)

	for noteID, tagSet := range m.noteTags {
		if _, ok := tagSet[clean]; ok {
			delete(tagSet, clean)
			if n, ok := m.notes[noteID]; ok {
				newTags := make([]string, 0, len(tagSet))
				for t := range tagSet {
					newTags = append(newTags, t)
				}
				sort.Strings(newTags)
				n.Tags = newTags
				m.notes[noteID] = n
			}
		}
	}

	return nil
}

// Search performs an in-memory search across notes, transcripts, and meetings.
func (m *Memory) Search(query string, filter SearchFilter) ([]SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleanQuery := strings.TrimSpace(query)
	if cleanQuery == "" {
		return []SearchResult{}, nil
	}

	lowerQuery := strings.ToLower(cleanQuery)
	searchAll := len(filter.Types) == 0
	typesSet := make(map[SearchResultType]bool)
	for _, t := range filter.Types {
		typesSet[t] = true
	}

	var results []SearchResult

	// 1. Notes
	if searchAll || typesSet[SearchResultNote] {
		for _, n := range m.notes {
			titleMatch := strings.Contains(strings.ToLower(n.Title), lowerQuery)
			contentMatch := strings.Contains(strings.ToLower(n.Content), lowerQuery)
			if titleMatch || contentMatch {
				snippet := extractSnippet(n.Content, cleanQuery, 120)
				if snippet == "" {
					snippet = n.Title
				}
				results = append(results, SearchResult{
					ID:        n.ID,
					Type:      SearchResultNote,
					Title:     n.Title,
					Snippet:   snippet,
					CreatedAt: n.CreatedAt,
				})
			}
		}
	}

	// 2. Transcripts
	if searchAll || typesSet[SearchResultTranscript] {
		for _, t := range m.transcripts {
			textMatch := strings.Contains(strings.ToLower(t.Text), lowerQuery)
			rawMatch := strings.Contains(strings.ToLower(t.RawText), lowerQuery)
			if textMatch || rawMatch {
				snippet := extractSnippet(t.Text, cleanQuery, 120)
				if snippet == "" {
					snippet = extractSnippet(t.RawText, cleanQuery, 120)
				}
				title := fmt.Sprintf("Dictation %s", t.CreatedAt.Format("2006-01-02 15:04"))
				results = append(results, SearchResult{
					ID:        t.ID,
					Type:      SearchResultTranscript,
					Title:     title,
					Snippet:   snippet,
					CreatedAt: t.CreatedAt,
				})
			}
		}
	}

	// 3. Meetings
	if searchAll || typesSet[SearchResultMeeting] {
		for _, mtg := range m.meetings {
			mtgTitleMatch := strings.Contains(strings.ToLower(mtg.Title), lowerQuery)
			var matchedText string
			if mtgTitleMatch {
				matchedText = mtg.Title
			} else {
				// Search segments
				for _, seg := range m.segments[mtg.ID] {
					if strings.Contains(strings.ToLower(seg.Text), lowerQuery) {
						matchedText = seg.Text
						break
					}
				}
				// Search summary overview if not found yet
				if matchedText == "" {
					if sum, ok := m.summaries[mtg.ID]; ok {
						if strings.Contains(strings.ToLower(sum.Overview), lowerQuery) {
							matchedText = sum.Overview
						}
					}
				}
			}

			if matchedText != "" {
				snippet := extractSnippet(matchedText, cleanQuery, 120)
				results = append(results, SearchResult{
					ID:        mtg.ID,
					Type:      SearchResultMeeting,
					Title:     mtg.Title,
					Snippet:   snippet,
					CreatedAt: mtg.CreatedAt,
				})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if filter.Limit > 0 && filter.Limit < len(results) {
		results = results[:filter.Limit]
	}

	return results, nil
}

func extractSnippet(text string, query string, maxLen int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if maxLen <= 0 {
		maxLen = 120
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	idx := strings.Index(lowerText, lowerQuery)
	if idx == -1 {
		words := strings.Fields(lowerQuery)
		for _, w := range words {
			if pos := strings.Index(lowerText, w); pos != -1 {
				idx = pos
				break
			}
		}
	}
	if idx == -1 {
		if len(text) > maxLen {
			return text[:maxLen] + "..."
		}
		return text
	}

	start := idx - maxLen/4
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
	}

	snippet := strings.TrimSpace(text[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet = snippet + "..."
	}
	return snippet
}

func embeddingKey(targetType, targetID, model string) string {
	return fmt.Sprintf("%s:%s:%s", targetType, targetID, model)
}

func (m *Memory) SaveEmbedding(rec EmbeddingRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.embeddings == nil {
		m.embeddings = make(map[string]EmbeddingRecord)
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = time.Now()
	}
	key := embeddingKey(rec.TargetType, rec.TargetID, rec.Model)
	cp := append([]float32(nil), rec.Embedding...)
	rec.Embedding = cp
	m.embeddings[key] = rec
	return nil
}

func (m *Memory) GetEmbedding(targetType, targetID, model string) (*EmbeddingRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := embeddingKey(targetType, targetID, model)
	rec, ok := m.embeddings[key]
	if !ok {
		return nil, errors.New("embedding not found")
	}
	cp := rec
	cp.Embedding = append([]float32(nil), rec.Embedding...)
	return &cp, nil
}

func (m *Memory) ListEmbeddings(targetType, model string) ([]EmbeddingRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []EmbeddingRecord
	for _, rec := range m.embeddings {
		if targetType != "" && rec.TargetType != targetType {
			continue
		}
		if model != "" && rec.Model != model {
			continue
		}
		cp := rec
		cp.Embedding = append([]float32(nil), rec.Embedding...)
		results = append(results, cp)
	}
	return results, nil
}

func (m *Memory) DeleteEmbedding(targetType, targetID, model string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if model != "" {
		delete(m.embeddings, embeddingKey(targetType, targetID, model))
		return nil
	}
	for k, rec := range m.embeddings {
		if rec.TargetType == targetType && rec.TargetID == targetID {
			delete(m.embeddings, k)
		}
	}
	return nil
}
