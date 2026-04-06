// SPDX-License-Identifier: GPL-2.0-only
package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"text/template"

	"github.com/Work-Fort/Flow/internal/domain"
)

// hookConfig is the JSON shape stored in IntegrationHook.Config for
// adapter_type=="chat" / action=="post_message".
type hookConfig struct {
	Channel  string `json:"channel"`
	Template string `json:"template"`
}

// fireTransitionHooks fires all integration hooks for the given transition.
// Hook execution errors are logged but do not fail the transition — the
// transition is already committed when this is called.
func (s *Service) fireTransitionHooks(ctx context.Context, tmpl *domain.WorkflowTemplate, item *domain.WorkItem, transitionID string) {
	if s.chat == nil {
		return
	}
	for _, h := range tmpl.IntegrationHooks {
		if h.TransitionID != transitionID {
			continue
		}
		if h.AdapterType != "chat" || h.Action != "post_message" {
			continue
		}
		s.fireChatPostMessage(ctx, h, item)
	}
}

// fireChatPostMessage executes a single chat post_message hook.
func (s *Service) fireChatPostMessage(ctx context.Context, h domain.IntegrationHook, item *domain.WorkItem) {
	var cfg hookConfig
	if err := json.Unmarshal(h.Config, &cfg); err != nil {
		return // malformed config — skip silently
	}
	if cfg.Channel == "" || cfg.Template == "" {
		return
	}

	content := renderTemplate(cfg.Template, item)

	// No metadata for plain post_message hooks.
	s.chat.PostMessage(ctx, cfg.Channel, content, nil) //nolint:errcheck
}

// renderTemplate replaces {{item.title}} and {{item.priority}} placeholders
// in the template string using Go's text/template package.
func renderTemplate(tmplStr string, item *domain.WorkItem) string {
	// Replace known {{item.field}} placeholders with their capitalized Go struct equivalents.
	tmplStr = strings.ReplaceAll(tmplStr, "{{item.title}}", "{{.Item.Title}}")
	tmplStr = strings.ReplaceAll(tmplStr, "{{item.priority}}", "{{.Item.Priority}}")

	t, err := template.New("hook").Parse(tmplStr)
	if err != nil {
		return tmplStr // return raw on parse error
	}

	data := struct {
		Item struct {
			Title    string
			Priority string
		}
	}{}
	data.Item.Title = item.Title
	data.Item.Priority = string(item.Priority)

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return tmplStr
	}
	return buf.String()
}
