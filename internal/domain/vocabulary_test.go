// SPDX-License-Identifier: GPL-2.0-only
package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
)

func TestVocabulary_RenderEvent_SDLCTaskAssigned(t *testing.T) {
	v := &domain.Vocabulary{
		ID:   "voc_sdlc",
		Name: "sdlc",
		Events: []domain.VocabularyEvent{{
			ID:              "ve_task_assigned",
			VocabularyID:    "voc_sdlc",
			EventType:       "task_assigned",
			MessageTemplate: "Task assigned: {{.WorkItem.Title}} → {{.AgentName}}",
			MetadataKeys:    []string{"agent_id", "role"},
		}},
	}
	text, meta, err := v.RenderEvent("task_assigned", domain.VocabularyContext{
		WorkItem:  &domain.WorkItem{Title: "Refactor parser"},
		AgentName: "agent-7",
		Payload:   map[string]any{"agent_id": "a_7", "role": "developer", "ignored": "x"},
	})
	if err != nil {
		t.Fatalf("RenderEvent: %v", err)
	}
	want := "Task assigned: Refactor parser → agent-7"
	if text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
	var m map[string]any
	if err := json.Unmarshal(meta, &m); err != nil {
		t.Fatalf("metadata not JSON: %v", err)
	}
	if m["event_type"] != "task_assigned" || m["agent_id"] != "a_7" || m["role"] != "developer" {
		t.Errorf("metadata = %v", m)
	}
	if _, leaked := m["ignored"]; leaked {
		t.Errorf("metadata leaked unauthorised payload key 'ignored': %v", m)
	}
}

func TestVocabulary_RenderEvent_UnknownEventReturnsErr(t *testing.T) {
	v := &domain.Vocabulary{ID: "voc_x", Events: nil}
	_, _, err := v.RenderEvent("nope", domain.VocabularyContext{})
	if !errors.Is(err, domain.ErrEventNotInVocabulary) {
		t.Errorf("err = %v, want ErrEventNotInVocabulary", err)
	}
}

func TestVocabulary_RenderEvent_NoCrashOnNilFields(t *testing.T) {
	v := &domain.Vocabulary{
		ID: "voc_x",
		Events: []domain.VocabularyEvent{{
			EventType:       "x",
			MessageTemplate: "title={{.WorkItem.Title}} agent={{.AgentName}}",
		}},
	}
	text, _, err := v.RenderEvent("x", domain.VocabularyContext{})
	if err != nil {
		t.Fatalf("RenderEvent: %v", err)
	}
	if !strings.Contains(text, "<no value>") {
		t.Errorf("expected text/template <no value> sentinel, got %q", text)
	}
}
