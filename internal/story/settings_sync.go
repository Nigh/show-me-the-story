package story

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"showmethestory/internal/llm"
	"showmethestory/internal/sse"
	"strings"
)

type SettingChange struct {
	ID       int             `json:"id"`
	Kind     string          `json:"kind"`
	EntityID string          `json:"entity_id"`
	Before   map[string]any  `json:"before,omitempty"`
	After    map[string]any  `json:"after,omitempty"`
	Source   MemoryReference `json:"source"`
	Status   string          `json:"status"` // applied, pending, rejected, reverted
	Reason   string          `json:"reason,omitempty"`
}
type settingDelta struct {
	Kind      string         `json:"kind"`
	Entity    map[string]any `json:"entity"`
	BlockID   int            `json:"block_id"`
	Evolution bool           `json:"evolution"`
	Conflict  bool           `json:"conflict"`
	Reason    string         `json:"reason"`
}

func cloneSettings(s *ProjectSettings) *ProjectSettings {
	data, _ := json.Marshal(s)
	var out ProjectSettings
	_ = json.Unmarshal(data, &out)
	return &out
}
func settingsEntities(s *ProjectSettings) map[string][]map[string]any {
	data, _ := json.Marshal(map[string]any{"characters": s.Characters, "worldview": s.Worldview, "organizations": s.Organizations, "relations": s.Relations})
	var all map[string]json.RawMessage
	_ = json.Unmarshal(data, &all)
	out := map[string][]map[string]any{}
	for _, k := range []string{"characters", "worldview", "organizations", "relations"} {
		var items []map[string]any
		_ = json.Unmarshal(all[k], &items)
		out[k] = items
	}
	return out
}
func entityAt(s *ProjectSettings, kind, id string) map[string]any {
	var value any
	switch kind {
	case "characters":
		for _, e := range s.Characters {
			if e.ID == id {
				value = e
				break
			}
		}
	case "worldview":
		for _, e := range s.Worldview {
			if e.ID == id {
				value = e
				break
			}
		}
	case "organizations":
		for _, e := range s.Organizations {
			if e.ID == id {
				value = e
				break
			}
		}
	case "relations":
		for _, e := range s.Relations {
			if e.ID == id {
				value = e
				break
			}
		}
	}
	if value == nil {
		return nil
	}
	data, _ := json.Marshal(value)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}
func putEntity(s *ProjectSettings, kind, id string, value map[string]any) error {
	all := settingsEntities(s)
	items := []map[string]any{}
	found := false
	for _, e := range all[kind] {
		if e["id"] == id {
			found = true
			if value != nil {
				items = append(items, value)
			}
		} else {
			items = append(items, e)
		}
	}
	if !found && value != nil {
		items = append(items, value)
	}
	data, err := json.Marshal(map[string]any{kind: items})
	if err != nil {
		return err
	}
	var decoded ProjectSettings
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	switch kind {
	case "characters":
		s.Characters = decoded.Characters
	case "worldview":
		s.Worldview = decoded.Worldview
	case "organizations":
		s.Organizations = decoded.Organizations
	case "relations":
		s.Relations = decoded.Relations
	default:
		return fmt.Errorf("invalid entity kind")
	}
	return nil
}
func validateEntity(s *ProjectSettings, kind string, e map[string]any) error {
	fields := map[string]string{
		"characters":    "id name age appearance personality background motivation abilities notes",
		"worldview":     "id category name description tags",
		"organizations": "id name type description members",
		"relations":     "id source_id source_type target_id target_type label",
	}
	allowed, ok := fields[kind]
	if !ok {
		return fmt.Errorf("invalid entity kind")
	}
	for k, v := range e {
		if !strings.Contains(" "+allowed+" ", " "+k+" ") {
			return fmt.Errorf("invalid entity field")
		}
		if k == "members" {
			a, ok := v.([]any)
			if !ok {
				return fmt.Errorf("invalid members")
			}
			for _, id := range a {
				str, ok := id.(string)
				if !ok || entityAt(s, "characters", str) == nil {
					return fmt.Errorf("unknown member")
				}
			}
		} else if _, ok := v.(string); !ok {
			return fmt.Errorf("invalid entity value")
		}
	}
	required := []string{"name"}
	if kind == "worldview" {
		required = append(required, "category", "description")
	}
	if kind == "relations" {
		required = []string{"source_id", "source_type", "target_id", "target_type", "label"}
		for _, end := range []string{"source", "target"} {
			typ, _ := e[end+"_type"].(string)
			id, _ := e[end+"_id"].(string)
			kinds := map[string]string{"character": "characters", "worldview": "worldview", "organization": "organizations"}
			if kinds[typ] == "" || entityAt(s, kinds[typ], id) == nil {
				return fmt.Errorf("unknown relation endpoint")
			}
		}
	}
	for _, k := range required {
		v, _ := e[k].(string)
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("missing entity field")
		}
	}
	return nil
}

func discardUnknownEntityFields(kind string, e map[string]any) {
	allowed := map[string]string{
		"characters":    " id name age appearance personality background motivation abilities notes ",
		"worldview":     " id category name description tags ",
		"organizations": " id name type description members ",
		"relations":     " id source_id source_type target_id target_type label ",
	}[kind]
	for k := range e {
		if !strings.Contains(allowed, " "+k+" ") {
			delete(e, k)
		}
	}
}
func newEntityID(s *ProjectSettings, kind string) string {
	switch kind {
	case "characters":
		return s.NextCharacterID()
	case "worldview":
		return s.NextWorldviewID()
	case "organizations":
		return s.NextOrganizationID()
	default:
		return s.NextRelationID()
	}
}
func settingsAtChapter(s *ProjectSettings, num int) *ProjectSettings {
	if s == nil {
		return nil
	}
	// Entity reads decode independent maps; putEntity replaces a slice, so the
	// read snapshot need not clone the entire provenance history.
	out := &ProjectSettings{Characters: s.Characters, Worldview: s.Worldview, Organizations: s.Organizations, Relations: s.Relations}
	for i := len(s.StoryChanges) - 1; i >= 0; i-- {
		c := s.StoryChanges[i]
		if c.Status != "applied" || c.Source.Chapter <= num {
			continue
		}
		current := entityAt(out, c.Kind, c.EntityID)
		if current == nil {
			continue
		}
		if c.Before == nil {
			if reflect.DeepEqual(current, c.After) {
				_ = putEntity(out, c.Kind, c.EntityID, nil)
			}
			continue
		}
		for key, value := range c.After {
			if !reflect.DeepEqual(current[key], value) {
				continue
			}
			if old, ok := c.Before[key]; ok {
				current[key] = old
			} else {
				delete(current, key)
			}
		}
		_ = putEntity(out, c.Kind, c.EntityID, current)
	}
	return out
}
func invalidateSettingSources(s *ProjectSettings, state *Progress) {
	for i := len(s.StoryChanges) - 1; i >= 0; i-- {
		c := &s.StoryChanges[i]
		ci := FindChapterIdx(state, c.Source.Chapter)
		valid := ci >= 0 && state.Chapters[ci].Status == StatusAccepted && ReferenceLive(state, c.Source)
		if c.Status == "pending" && c.Reason == "source_changed" {
			continue
		}
		if valid || (c.Status != "applied" && c.Status != "pending") {
			continue
		}
		if c.Status == "pending" {
			c.Status = "rejected"
			continue
		}
		dependent := false
		for j := i + 1; j < len(s.StoryChanges); j++ {
			d := s.StoryChanges[j]
			if d.Status == "applied" && d.EntityID == c.EntityID {
				dependent = true
			}
		}
		// A created node cannot be removed while another entity refers to it.
		if c.Before == nil {
			dependent = dependent || entityHasDependents(s, c.EntityID)
		}
		if !dependent && reflect.DeepEqual(entityAt(s, c.Kind, c.EntityID), c.After) {
			_ = putEntity(s, c.Kind, c.EntityID, c.Before)
			c.Status = "reverted"
		} else {
			// Keep subsequent or author changes; propose withdrawal without overwriting them.
			s.StoryChanges = append(s.StoryChanges, SettingChange{ID: len(s.StoryChanges) + 1, Kind: c.Kind, EntityID: c.EntityID, Before: entityAt(s, c.Kind, c.EntityID), After: c.Before, Source: c.Source, Status: "pending", Reason: "source_changed"})
			s.StoryChanges[i].Status = "superseded"
		}
	}
}
func entityHasDependents(s *ProjectSettings, id string) bool {
	for _, r := range s.Relations {
		if r.SourceID == id || r.TargetID == id {
			return true
		}
	}
	for _, o := range s.Organizations {
		for _, member := range o.Members {
			if member == id {
				return true
			}
		}
	}
	return false
}
func applySettingDeltas(s *ProjectSettings, ch ChapterState, deltas []settingDelta, visible ...map[string]bool) (resultErr error) {
	detail := &knowledgeValidationError{chapter: ch.Num, rev: ChapterRevision(ch), index: -1}
	defer func() {
		if resultErr != nil {
			detail.reason = resultErr.Error()
			resultErr = detail
		}
	}()
	aliases := map[string]string{}
	for index, d := range deltas {
		id, _ := d.Entity["id"].(string)
		detail.index, detail.block = index, d.BlockID
		detail.kind, detail.id = safeSettingKind(d.Kind), safeSettingID(id)
		detail.inCurrentKind = id != "" && entityAt(s, d.Kind, id) != nil
		detail.inContext = false
		if len(visible) > 0 {
			_, detail.inContext = visible[0][id]
		}
		discardUnknownEntityFields(d.Kind, d.Entity)
		for _, k := range []string{"source_id", "target_id"} {
			if id, ok := d.Entity[k].(string); ok && aliases[id] != "" {
				d.Entity[k] = aliases[id]
			}
		}
		if members, ok := d.Entity["members"].([]any); ok {
			for i, v := range members {
				if id, ok := v.(string); ok && aliases[id] != "" {
					members[i] = aliases[id]
				}
			}
		}
		bi := FindBlockIdx(&ch, d.BlockID)
		if bi < 0 {
			return fmt.Errorf("invalid setting evidence")
		}
		id, _ = d.Entity["id"].(string)
		alias := ""
		if strings.HasPrefix(id, "$") {
			alias = id
			id = ""
		}
		before := entityAt(s, d.Kind, id)
		if id != "" && before == nil {
			return fmt.Errorf("unknown entity identity")
		}
		if id == "" {
			// Match exact names/edges before allocating IDs; fuzzy identity needs author review.
			for _, e := range settingsEntities(s)[d.Kind] {
				same := d.Kind != "relations" && e["name"] == d.Entity["name"]
				if d.Kind == "relations" {
					same = e["source_id"] == d.Entity["source_id"] && e["target_id"] == d.Entity["target_id"] && e["label"] == d.Entity["label"]
				}
				if same {
					id, _ = e["id"].(string)
					before = e
					break
				}
			}
		}
		if id == "" {
			id = newEntityID(s, d.Kind)
		}
		if alias != "" {
			aliases[alias] = id
		}
		after := map[string]any{}
		for k, v := range before {
			after[k] = v
		}
		for k, v := range d.Entity {
			after[k] = v
		}
		after["id"] = id
		if err := validateEntity(s, d.Kind, after); err != nil {
			return err
		}
		if reflect.DeepEqual(before, after) {
			continue
		}
		conflict := d.Conflict
		// Author knowledge describes rules, not automatic plot evolution.
		if d.Kind == "worldview" && before != nil && before["category"] == "knowledge" {
			conflict = true
		}
		if before != nil && len(visible) > 0 && !visible[0][id] {
			// Includes name/edge collisions resolved after local $ID aliases.
			conflict = true
		}
		for k, v := range before {
			if k == "id" || v == "" || reflect.DeepEqual(v, after[k]) {
				continue
			}
			owned := false
			for i := len(s.StoryChanges) - 1; i >= 0; i-- {
				c := s.StoryChanges[i]
				if c.EntityID == id && c.Status == "applied" {
					owned = reflect.DeepEqual(c.After, before)
					break
				}
			}
			if !(owned && d.Evolution) && !(d.Kind == "relations" && k == "label" && d.Evolution) {
				conflict = true
			}
		}
		status := "applied"
		if conflict {
			status = "pending"
		}
		duplicate := false
		for _, old := range s.StoryChanges {
			if old.Status == "pending" && old.Kind == d.Kind && old.EntityID == id && old.Source.Chapter == ch.Num && old.Source.BlockID == d.BlockID && old.Source.ContentRev == ChapterRevision(ch) && reflect.DeepEqual(old.Before, before) && reflect.DeepEqual(old.After, after) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		c := SettingChange{ID: len(s.StoryChanges) + 1, Kind: d.Kind, EntityID: id, Before: before, After: after, Source: MemoryReference{Chapter: ch.Num, BlockID: d.BlockID, Quote: ch.Blocks[bi].Text, ContentRev: ChapterRevision(ch)}, Status: status, Reason: d.Reason}
		s.StoryChanges = append(s.StoryChanges, c)
		if !conflict {
			if err := putEntity(s, d.Kind, id, after); err != nil {
				return fmt.Errorf("invalid setting entity encoding")
			}
		}
	}
	return nil
}

func safeSettingKind(kind string) string {
	switch kind {
	case "characters", "worldview", "organizations", "relations":
		return kind
	default:
		return "invalid"
	}
}

func safeSettingID(id string) string {
	if id == "" {
		return "empty"
	}
	// Local aliases may contain names. Only log server-shaped numeric IDs.
	if strings.HasPrefix(id, "$") {
		return "$alias"
	}
	if len(id) < 3 || len(id) > 64 || !strings.ContainsRune("cwor", rune(id[0])) || id[1] != '_' {
		return "invalid"
	}
	for _, digit := range id[2:] {
		if digit < '0' || digit > '9' {
			return "invalid"
		}
	}
	return id
}

func settingRetryPrompt(lang string, err error) string {
	if i18n.NormalizeLanguage(lang) == i18n.LangEN {
		return "\nThe previous response failed validation: " + err.Error() + ". Return the complete corrected JSON with a changes array. Reuse only supplied existing IDs in their supplied kind; never invent a server ID. For new entities omit id, use an empty id, or use a local $ID. List new entities before references to them. Use supplied block IDs. Do not guess identities.\n"
	}
	return "\n上次输出校验失败：" + err.Error() + "。请返回含 changes 数组的完整修正后 JSON。已有 ID 只能按所提供的分类复用，禁止编造正式 ID。新实体省略 id、使用空 id 或局部 $ID，先输出新实体再引用。使用所提供的真实段落 ID，不得猜测身份。\n"
}

func ResolveSettingChange(s *ProjectSettings, state *Progress, id int, accept bool, path string) error {
	next := cloneSettings(s)
	for i := range next.StoryChanges {
		c := &next.StoryChanges[i]
		if c.ID != id || c.Status != "pending" {
			continue
		}
		if accept {
			if c.After == nil && entityHasDependents(next, c.EntityID) {
				return fmt.Errorf("setting_has_dependents")
			}
			if !reflect.DeepEqual(entityAt(next, c.Kind, c.EntityID), c.Before) {
				return fmt.Errorf("content_version_conflict")
			}
			if c.Reason != "source_changed" && !ReferenceLive(state, c.Source) {
				return fmt.Errorf("content_version_conflict")
			}
			if c.After != nil {
				if err := validateEntity(next, c.Kind, c.After); err != nil {
					return err
				}
			}
			if err := putEntity(next, c.Kind, c.EntityID, c.After); err != nil {
				return err
			}
			c.Status = "applied"
			if c.Reason == "source_changed" {
				c.Status = "resolved"
			}
		} else {
			c.Status = "rejected"
		}
		if err := SaveProjectSettings(path, next); err != nil {
			return err
		}
		*s = *next
		return nil
	}
	return fmt.Errorf("unknown pending setting change")
}
func SyncPendingKnowledge(ctx context.Context, api *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, path string, logger *sse.LogBroadcaster) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if settings == nil {
		return nil
	}
	activeChapter := ChapterState{}
	stage, currentAttempt := "invalidate_sources", 0
	defer func() {
		if resultErr != nil {
			knowledgeSyncEvent(ctx, "error", "knowledge_sync_failed", "settings", stage, activeChapter, currentAttempt, resultErr)
			resultErr = fmt.Errorf("%w: phase=settings stage=%s chapter=%d rev=%s attempt=%d", resultErr, stage, activeChapter.Num, ChapterRevision(activeChapter), currentAttempt)
		}
	}()
	settingsPath := filepath.Join(filepath.Dir(path), "settings.json")
	next := cloneSettings(settings)
	invalidateSettingSources(next, state)
	if !reflect.DeepEqual(next, settings) {
		if err := SaveProjectSettings(settingsPath, next); err != nil {
			return err
		}
		*settings = *next
		logger.SettingsUpdated()
	}
	for i := range state.Chapters {
		ch := state.Chapters[i]
		activeChapter, stage, currentAttempt = ch, "facts", 0
		if !ch.KnowledgeTracked || ch.Content == "" || (ch.Status != StatusReview && ch.Status != StatusAccepted) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := SyncChapterMemory(ctx, api, cfg, state, i, path, logger); err != nil {
			return err
		}
		ch = state.Chapters[i]
		if ch.Status != StatusAccepted || settings.StorySynced[ch.Num] == ChapterRevision(ch) {
			continue
		}
		activeChapter, stage, currentAttempt = ch, "config", 0
		if err := llm.ValidateConfig(api); err != nil {
			return err
		}
		selected, visible := retrieveSettings(settingsAtChapter(settings, ch.Num), chapterKnowledgeQuery(ch), settingsContextRunes)
		entities, _ := json.Marshal(settingsEntities(selected))
		blocks, _ := json.Marshal(ch.Blocks)
		prompt := settingUpdatePrompt(cfg.Language) + knowledgeSelectionNotice(cfg.Language) + "\n" + string(entities) + "\n" + string(blocks)
		var syncErr error
		for attempt := 0; attempt < 2; attempt++ {
			stage, currentAttempt = "request", attempt+1
			raw := llm.CallAPIWithRetryLog(ctx, api, i18n.SystemPromptFor(cfg.Language, "memory_manager"), prompt, logger)
			if err := ctx.Err(); err != nil {
				return err
			}
			var result struct {
				Changes *[]settingDelta `json:"changes"`
			}
			stage = "validate"
			if err := json.Unmarshal([]byte(cleanJSONResponse(raw)), &result); err != nil {
				syncErr = &knowledgeValidationError{reason: "invalid setting JSON", chapter: ch.Num, rev: ChapterRevision(ch), index: -1}
			} else if result.Changes == nil {
				syncErr = &knowledgeValidationError{reason: "missing changes array", chapter: ch.Num, rev: ChapterRevision(ch), index: -1}
			} else {
				next = cloneSettings(settings)
				syncErr = applySettingDeltas(next, ch, *result.Changes, visible)
			}
			if syncErr == nil {
				break
			}
			knowledgeSyncEvent(ctx, "warn", "knowledge_validation_failed", "settings", stage, ch, currentAttempt, syncErr)
			prompt += settingRetryPrompt(cfg.Language, syncErr)
		}
		if syncErr != nil {
			return syncErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		stage = "save"
		if next.StorySynced == nil {
			next.StorySynced = map[int]string{}
		}
		next.StorySynced[ch.Num] = ChapterRevision(ch)
		if err := SaveProjectSettings(settingsPath, next); err != nil {
			return err
		}
		*settings = *next
		logger.SettingsUpdated()
		knowledgeSyncEvent(ctx, "info", "knowledge_sync_succeeded", "settings", "complete", ch, currentAttempt, nil)
	}
	return nil
}
