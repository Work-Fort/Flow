// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
	"time"
)

type Vocabulary struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	ReleaseEvent string            `json:"release_event,omitempty"`
	Events       []VocabularyEvent `json:"events"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type VocabularyEvent struct {
	ID              string   `json:"id"`
	VocabularyID    string   `json:"vocabulary_id"`
	EventType       string   `json:"event_type"`
	MessageTemplate string   `json:"message_template"`
	MetadataKeys    []string `json:"metadata_keys,omitempty"`
}

type VocabularyContext struct {
	Project   *Project
	WorkItem  *WorkItem
	AgentName string
	Role      string
	Payload   map[string]any
}

// safeVocabCtx wraps VocabularyContext to expose nil-safe struct accessors
// so text/template can navigate to nested fields without panicking on nil
// pointer dereferences (e.g. .WorkItem.Title when WorkItem is nil).
type safeVocabCtx struct {
	Project   *Project
	WorkItem  safeWorkItem
	AgentName string
	Role      string
	Payload   map[string]any
}

type safeWorkItem struct{ w *WorkItem }

func (s safeWorkItem) Title() string {
	if s.w == nil {
		return "<no value>"
	}
	return s.w.Title
}

func (s safeWorkItem) Priority() Priority {
	if s.w == nil {
		return ""
	}
	return s.w.Priority
}

func (v *Vocabulary) RenderEvent(eventType string, ctx VocabularyContext) (string, json.RawMessage, error) {
	var ev *VocabularyEvent
	for i := range v.Events {
		if v.Events[i].EventType == eventType {
			ev = &v.Events[i]
			break
		}
	}
	if ev == nil {
		return "", nil, fmt.Errorf("%w: %q in vocabulary %q", ErrEventNotInVocabulary, eventType, v.Name)
	}
	t, err := template.New("event").Option("missingkey=default").Parse(ev.MessageTemplate)
	if err != nil {
		return "", nil, fmt.Errorf("parse vocabulary template %q: %w", eventType, err)
	}
	safe := safeVocabCtx{
		Project:   ctx.Project,
		WorkItem:  safeWorkItem{ctx.WorkItem},
		AgentName: ctx.AgentName,
		Role:      ctx.Role,
		Payload:   ctx.Payload,
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, safe); err != nil {
		return "", nil, fmt.Errorf("render vocabulary template %q: %w", eventType, err)
	}

	meta := map[string]any{"event_type": eventType}
	for _, k := range ev.MetadataKeys {
		if val, ok := ctx.Payload[k]; ok {
			meta[k] = val
		}
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return "", nil, fmt.Errorf("marshal vocabulary metadata: %w", err)
	}
	return buf.String(), metaJSON, nil
}
