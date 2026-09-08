package story

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Budgets cover retrieved records, not the chapter, system prompt or output.
const settingsContextRunes = 8000
const memoryContextRunes = 8000

type knowledgeDocument struct {
	id, name, text string
	priority       float64
}

// Long fields are retrieved as whole sentences, with an explicit excerpt marker.
// The caller must never treat an excerpt as authority to automatically overwrite
// the complete stored entity.
func settingExcerpt(entity map[string]any, query string, budget int) (string, bool) {
	data, _ := json.Marshal(entity)
	if utf8.RuneCount(data) <= budget {
		return string(data), true
	}
	view := map[string]any{}
	for k, v := range entity {
		view[k] = v
	}
	fieldBudget := max(0, (budget-400)/max(1, len(entity)))
	for k, value := range view {
		text, ok := value.(string)
		if !ok || k == "id" || k == "name" || k == "source_id" || k == "target_id" || utf8.RuneCountInString(text) <= fieldBudget {
			continue
		}
		docs := []knowledgeDocument{}
		start := 0
		for i, r := range text {
			end := i + utf8.RuneLen(r)
			next, _ := utf8.DecodeRuneInString(text[end:])
			if r == '\n' || strings.ContainsRune("。！？；", r) || (strings.ContainsRune(".!?;", r) && (end == len(text) || unicode.IsSpace(next))) {
				docs = append(docs, knowledgeDocument{text: text[start:end]})
				start = end
			}
		}
		if start < len(text) {
			docs = append(docs, knowledgeDocument{text: text[start:]})
		}
		selected := packKnowledge(docs, rankKnowledge(docs, query), fieldBudget)
		sort.Ints(selected) // Preserve source order inside a field.
		parts := []string{"[EXCERPT / 节选；省略部分未提供]"}
		for _, i := range selected {
			parts = append(parts, docs[i].text)
		}
		view[k] = strings.Join(parts, "\n")
	}
	data, _ = json.Marshal(view)
	return string(data), false
}

// English words and CJK bigrams need no dictionary or external embedding API.
func knowledgeTerms(text string) map[string]int {
	terms := map[string]int{}
	var word []rune
	var previous rune
	flush := func() {
		if len(word) > 1 {
			terms[string(word)]++
		}
		word = word[:0]
	}
	for _, r := range strings.ToLower(text) {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			flush()
			if previous != 0 {
				terms[string([]rune{previous, r})]++
			}
			previous = r
		} else {
			previous = 0
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				word = append(word, r)
			} else {
				flush()
			}
		}
	}
	flush()
	return terms
}

func knowledgeNameMatch(query, name string) bool {
	name = strings.ToLower(strings.TrimSpace(StripNameMarks(name)))
	if name == "" {
		return false
	}
	query = strings.ToLower(query)
	for start := 0; start < len(query); {
		i := strings.Index(query[start:], name)
		if i < 0 {
			return false
		}
		i += start
		end := i + len(name)
		first, _ := utf8.DecodeRuneInString(name)
		last, _ := utf8.DecodeLastRuneInString(name)
		left, _ := utf8.DecodeLastRuneInString(query[:i])
		right, _ := utf8.DecodeRuneInString(query[end:])
		latin := func(r rune) bool { return unicode.Is(unicode.Latin, r) || unicode.IsDigit(r) }
		if !(latin(first) && latin(left)) && !(latin(last) && latin(right)) {
			return true
		}
		start = end
	}
	return false
}

// BM25 ranks original records. Stable ties preserve deterministic prompts.
// ponytail: rebuild the in-memory term index per request; cache by revision only
// if profiling shows indexing dominates model latency on very large projects.
func rankKnowledge(docs []knowledgeDocument, query string) []int {
	queryTerms := knowledgeTerms(query)
	terms := make([]string, 0, len(queryTerms))
	for term := range queryTerms {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	frequencies := make([]map[string]int, len(docs))
	lengths := make([]int, len(docs))
	df := map[string]int{}
	total := 0
	for i, doc := range docs {
		frequencies[i] = knowledgeTerms(doc.name + " " + doc.text)
		for term, count := range frequencies[i] {
			lengths[i] += count
			if queryTerms[term] > 0 {
				df[term]++
			}
		}
		total += lengths[i]
	}
	average := float64(max(total, 1)) / float64(max(len(docs), 1))
	scores := make([]float64, len(docs))
	order := []int{}
	for i, doc := range docs {
		score := doc.priority
		if knowledgeNameMatch(query, doc.name) {
			score += 1000
		}
		for _, term := range terms {
			count := frequencies[i][term]
			if count == 0 {
				continue
			}
			idf := math.Log(1 + (float64(len(docs)-df[term])+0.5)/(float64(df[term])+0.5))
			tf := float64(count)
			score += idf * tf * 2.2 / (tf + 1.2*(0.25+0.75*float64(lengths[i])/average))
		}
		if score > 0 {
			scores[i] = score
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return scores[order[i]] > scores[order[j]] })
	return order
}

// Keep whole records: clipping a setting/fact can remove a negation or condition.
func packKnowledge(docs []knowledgeDocument, order []int, budget int) []int {
	selected := []int{}
	for _, i := range order {
		cost := utf8.RuneCountInString(docs[i].text) + 1
		if cost > budget {
			continue
		}
		selected = append(selected, i)
		budget -= cost
	}
	return selected
}

func chapterKnowledgeQuery(ch ChapterState) string {
	var out strings.Builder
	out.WriteString(ch.Title + "\n" + ch.Outline + "\n" + ch.Content)
	for _, c := range ch.Characters {
		out.WriteString("\n" + c.Name)
	}
	return out.String()
}

// Retrieve a chapter-time snapshot, then expand direct entity links once. The
// expansion is not recursive, so a large guild cannot pull in the whole book.
func retrieveSettings(settings *ProjectSettings, query string, budget int) (*ProjectSettings, map[string]bool) {
	out := &ProjectSettings{}
	visible := map[string]bool{}
	if settings == nil {
		return out, visible
	}
	docs := []knowledgeDocument{}
	full := []bool{}
	kinds := []string{}
	entities := settingsEntities(settings)
	direct := map[string]bool{}
	for _, kind := range []string{"characters", "worldview", "organizations", "relations"} {
		for _, entity := range entities[kind] {
			id, _ := entity["id"].(string)
			name, _ := entity["name"].(string)
			data, complete := settingExcerpt(entity, query, min(2400, max(0, budget-100)))
			docs = append(docs, knowledgeDocument{id: id, name: name, text: data})
			full = append(full, complete)
			kinds = append(kinds, kind)
			if knowledgeNameMatch(query, name) {
				direct[id] = true
			}
		}
	}
	neighbors := map[string]bool{}
	for _, r := range settings.Relations {
		if direct[r.SourceID] || direct[r.TargetID] {
			neighbors[r.ID], neighbors[r.SourceID], neighbors[r.TargetID] = true, true, true
		}
	}
	for _, o := range settings.Organizations {
		for _, id := range o.Members {
			if direct[id] {
				neighbors[o.ID] = true
			}
			if direct[o.ID] {
				neighbors[id] = true
			}
		}
	}
	for i := range docs {
		if neighbors[docs[i].id] {
			docs[i].priority = 100
		}
	}
	for _, i := range packKnowledge(docs, rankKnowledge(docs, query), max(0, budget-100)) {
		// Decode only selected original records, without provenance history.
		switch kinds[i] {
		case "characters":
			var v Character
			_ = json.Unmarshal([]byte(docs[i].text), &v)
			out.Characters = append(out.Characters, v)
		case "worldview":
			var v WorldviewEntry
			_ = json.Unmarshal([]byte(docs[i].text), &v)
			out.Worldview = append(out.Worldview, v)
		case "organizations":
			var v Organization
			_ = json.Unmarshal([]byte(docs[i].text), &v)
			out.Organizations = append(out.Organizations, v)
		case "relations":
			var v Relation
			_ = json.Unmarshal([]byte(docs[i].text), &v)
			out.Relations = append(out.Relations, v)
		}
		visible[docs[i].id] = full[i]
	}
	return out, visible
}

func retrieveMemories(state *Progress, ch ChapterState, budget int, evidence bool) []MemoryEntry {
	docs := []knowledgeDocument{}
	entries := []MemoryEntry{}
	for _, m := range state.MemoryEntries {
		if m.Chapter > ch.Num {
			continue
		}
		live, linked := len(m.References) == 0, false
		for _, r := range m.References {
			if r.Chapter <= ch.Num && ReferenceLive(state, r) {
				live = true
				linked = linked || r.Chapter == ch.Num
			}
		}
		if !live {
			continue
		}
		priority := 0.0
		if linked {
			priority = 1000
		} else if m.Chapter >= ch.Num-3 {
			priority = 1
		}
		copy := m
		copy.References = nil
		copy.Snippet = ""
		if evidence {
			for _, r := range m.References {
				if r.Chapter <= ch.Num && ReferenceLive(state, r) {
					// One bounded original evidence passage, never future evidence.
					copy.Snippet = string([]rune(r.Quote)[:min(300, utf8.RuneCountInString(r.Quote))])
					break
				}
			}
		}
		data, _ := json.Marshal(copy)
		docs = append(docs, knowledgeDocument{text: string(data), priority: priority})
		entries = append(entries, copy)
	}
	out := []MemoryEntry{}
	for _, i := range packKnowledge(docs, rankKnowledge(docs, chapterKnowledgeQuery(ch)), budget) {
		out = append(out, entries[i])
	}
	return out
}
