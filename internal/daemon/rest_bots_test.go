// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/Work-Fort/Flow/internal/domain"
)

type fakePassport struct {
	mintPlaintext string
	mintKeyID     string
	mintErr       error
	revokedKeyIDs []string
}

func (f *fakePassport) MintAPIKey(_ context.Context, _ string) (string, string, error) {
	if f.mintErr != nil {
		return "", "", f.mintErr
	}
	return f.mintPlaintext, f.mintKeyID, nil
}
func (f *fakePassport) RevokeAPIKey(_ context.Context, keyID string) error {
	f.revokedKeyIDs = append(f.revokedKeyIDs, keyID)
	return nil
}

// minimalBotStore extends minimalFakeStore with bot operations.
type minimalBotStore struct {
	minimalFakeStore
	bots []*domain.Bot
}

func (s *minimalBotStore) CreateBot(_ context.Context, b *domain.Bot) error {
	s.bots = append(s.bots, b)
	return nil
}
func (s *minimalBotStore) GetBotByID(_ context.Context, id string) (*domain.Bot, error) {
	for _, b := range s.bots {
		if b.ID == id {
			return b, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (s *minimalBotStore) GetBotByProject(_ context.Context, projectID string) (*domain.Bot, error) {
	for _, b := range s.bots {
		if b.ProjectID == projectID {
			return b, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (s *minimalBotStore) DeleteBotByProject(_ context.Context, projectID string) error {
	for i, b := range s.bots {
		if b.ProjectID == projectID {
			s.bots = append(s.bots[:i], s.bots[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}
func (s *minimalBotStore) UpdateBot(_ context.Context, b *domain.Bot) error {
	for i, existing := range s.bots {
		if existing.ID == b.ID {
			s.bots[i] = b
			return nil
		}
	}
	return domain.ErrNotFound
}

func newBotTestMux(t *testing.T, store domain.Store, passport domain.PassportProvider) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("test", "1.0.0"))
	registerProjectRoutes(api, store, "/tmp/bot-keys-test", nil)
	registerBotKeyRoutes(api, store, "/tmp/bot-keys-test", passport)
	return mux
}

func postJSON2(mux http.Handler, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func TestCreateBot_AutoMint_ReturnsPlaintextOnce(t *testing.T) {
	store := &minimalBotStore{
		minimalFakeStore: minimalFakeStore{
			projects: []*domain.Project{{ID: "prj_1", Name: "flow", ChannelName: "#flow", VocabularyID: "voc_1"}},
			vocabs:   []*domain.Vocabulary{{ID: "voc_sdlc", Name: "sdlc"}},
		},
	}
	fp := &fakePassport{mintPlaintext: "wf-svc_abc123", mintKeyID: "kid_1"}
	mux := newBotTestMux(t, store, fp)

	rr := postJSON2(mux, "/v1/projects/prj_1/bot", map[string]any{
		"hive_role_assignments": []string{"developer"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create bot status=%d body=%s", rr.Code, rr.Body.String())
	}

	var body struct {
		Bot             map[string]any `json:"bot"`
		PlaintextAPIKey string         `json:"plaintext_api_key"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.PlaintextAPIKey != "wf-svc_abc123" {
		t.Errorf("plaintext = %q, want wf-svc_abc123", body.PlaintextAPIKey)
	}
	if len(store.bots) == 0 {
		t.Fatal("no bot created in store")
	}
}

func TestCreateBot_BYOKey_SkipsMint(t *testing.T) {
	store := &minimalBotStore{
		minimalFakeStore: minimalFakeStore{
			projects: []*domain.Project{{ID: "prj_2", Name: "hive", ChannelName: "#hive", VocabularyID: "voc_1"}},
			vocabs:   []*domain.Vocabulary{{ID: "voc_sdlc", Name: "sdlc"}},
		},
	}
	fp := &fakePassport{mintPlaintext: "should-not-appear", mintKeyID: "should-not"}
	mux := newBotTestMux(t, store, fp)

	rr := postJSON2(mux, "/v1/projects/prj_2/bot", map[string]any{
		"bring_your_own_key":  true,
		"passport_api_key":    "wf-svc_manual",
		"passport_api_key_id": "kid_manual",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(store.bots) == 0 || store.bots[0].PassportAPIKeyID != "kid_manual" {
		t.Errorf("expected manual key id, got %v", store.bots)
	}
}

func TestGetBot_NeverReturnsPlaintext(t *testing.T) {
	store := &minimalBotStore{
		minimalFakeStore: minimalFakeStore{
			projects: []*domain.Project{{ID: "prj_3", Name: "test", ChannelName: "#test", VocabularyID: "voc_1"}},
			vocabs:   []*domain.Vocabulary{{ID: "voc_sdlc", Name: "sdlc"}},
		},
		bots: []*domain.Bot{{ID: "bot_1", ProjectID: "prj_3", PassportAPIKeyID: "kid_1", HiveRoleAssignments: []string{}}},
	}
	fp := &fakePassport{}
	mux := newBotTestMux(t, store, fp)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/prj_3/bot", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "wf-svc_") {
		t.Errorf("GET bot leaked plaintext: %s", rr.Body.String())
	}
}
