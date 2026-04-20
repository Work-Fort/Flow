// SPDX-License-Identifier: GPL-2.0-only
package domain

import "time"

type Project struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	TemplateID   string    `json:"template_id,omitempty"`
	ChannelName  string    `json:"channel_name"`
	VocabularyID string    `json:"vocabulary_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
