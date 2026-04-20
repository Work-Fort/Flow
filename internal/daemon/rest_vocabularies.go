// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Flow/internal/domain"
)

func registerVocabularyRoutes(api huma.API, store domain.VocabularyStore) {
	huma.Register(api, huma.Operation{
		OperationID: "list-vocabularies",
		Method:      http.MethodGet,
		Path:        "/v1/vocabularies",
		Summary:     "List vocabularies",
		Tags:        []string{"Vocabularies"},
	}, func(ctx context.Context, input *struct{}) (*VocabularyListOutput, error) {
		vocs, err := store.ListVocabularies(ctx)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		resp := make([]vocabularyResponse, len(vocs))
		for i, v := range vocs {
			resp[i] = vocToResponse(v)
		}
		return &VocabularyListOutput{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-vocabulary",
		Method:      http.MethodGet,
		Path:        "/v1/vocabularies/{id}",
		Summary:     "Get a vocabulary by ID",
		Tags:        []string{"Vocabularies"},
	}, func(ctx context.Context, input *IDPathInput) (*VocabularyOutput, error) {
		v, err := store.GetVocabulary(ctx, input.ID)
		if err != nil {
			return nil, mapDomainErr(err)
		}
		return &VocabularyOutput{Body: vocToResponse(v)}, nil
	})
}

func vocToResponse(v *domain.Vocabulary) vocabularyResponse {
	events := make([]vocabularyEventResponse, len(v.Events))
	for i, e := range v.Events {
		events[i] = vocabularyEventResponse{
			ID:              e.ID,
			VocabularyID:    e.VocabularyID,
			EventType:       e.EventType,
			MessageTemplate: e.MessageTemplate,
			MetadataKeys:    e.MetadataKeys,
		}
	}
	return vocabularyResponse{
		ID:           v.ID,
		Name:         v.Name,
		Description:  v.Description,
		ReleaseEvent: v.ReleaseEvent,
		Events:       events,
		CreatedAt:    v.CreatedAt,
		UpdatedAt:    v.UpdatedAt,
	}
}
