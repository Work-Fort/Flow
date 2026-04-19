// SPDX-License-Identifier: GPL-2.0-only
package nexus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
)

func TestDriver_SatisfiesRuntimeDriverInterface(t *testing.T) {
	var _ domain.RuntimeDriver = New(Config{
		BaseURL: "http://example.invalid",
	})
}

func TestCloneWorkItemVolume_HappyPath(t *testing.T) {
	var gotBody map[string]any
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"POST /v1/drives/clone": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{
				"id":"d_clone1","name":"work-item-w1","size_bytes":104857600,
				"mount_path":"/work","created_at":"2026-04-19T00:00:00.000Z",
				"source_volume_ref":"project-master"
			}`)
		},
	})
	d := New(Config{BaseURL: url})
	master := domain.VolumeRef{Kind: VolumeKind, ID: "project-master"}
	work, err := d.CloneWorkItemVolume(context.Background(), master, "w1")
	if err != nil {
		t.Fatalf("CloneWorkItemVolume: %v", err)
	}
	if work.Kind != VolumeKind {
		t.Errorf("Kind = %q, want %q", work.Kind, VolumeKind)
	}
	if work.ID != "d_clone1" {
		t.Errorf("ID = %q, want d_clone1", work.ID)
	}
	if gotBody["source_volume_ref"] != "project-master" {
		t.Errorf("request source_volume_ref = %v", gotBody["source_volume_ref"])
	}
	if gotBody["name"] != "work-item-w1" {
		t.Errorf("request name = %v", gotBody["name"])
	}
	if _, ok := gotBody["mount_path"]; ok {
		t.Errorf("mount_path must be omitted from request (inherit from source); got %v",
			gotBody["mount_path"])
	}
}

func TestCloneWorkItemVolume_RejectsWrongKind(t *testing.T) {
	d := New(Config{BaseURL: "http://nope.invalid"})
	_, err := d.CloneWorkItemVolume(context.Background(),
		domain.VolumeRef{Kind: "k8s-pvc", ID: "x"}, "w1")
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Errorf("err = %v, want ErrUnsupportedKind", err)
	}
}

func TestCloneWorkItemVolume_404FromSourceMissing(t *testing.T) {
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"POST /v1/drives/clone": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		},
	})
	d := New(Config{BaseURL: url})
	_, err := d.CloneWorkItemVolume(context.Background(),
		domain.VolumeRef{Kind: VolumeKind, ID: "ghost"}, "w1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want wrap of ErrNotFound", err)
	}
}

func TestDeleteVolume_HappyPath(t *testing.T) {
	var deletedID string
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"DELETE /v1/drives/d_x1": func(w http.ResponseWriter, r *http.Request) {
			deletedID = strings.TrimPrefix(r.URL.Path, "/v1/drives/")
			w.WriteHeader(http.StatusNoContent)
		},
	})
	d := New(Config{BaseURL: url})
	err := d.DeleteVolume(context.Background(),
		domain.VolumeRef{Kind: VolumeKind, ID: "d_x1"})
	if err != nil {
		t.Errorf("DeleteVolume: %v", err)
	}
	if deletedID != "d_x1" {
		t.Errorf("deleted = %q, want d_x1", deletedID)
	}
}

func TestDeleteVolume_404IsIdempotent(t *testing.T) {
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"DELETE /v1/drives/d_gone": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		},
	})
	d := New(Config{BaseURL: url})
	err := d.DeleteVolume(context.Background(),
		domain.VolumeRef{Kind: VolumeKind, ID: "d_gone"})
	if err != nil {
		t.Errorf("404 DELETE should be treated as idempotent success, got %v", err)
	}
}

func TestDeleteVolume_RejectsWrongKind(t *testing.T) {
	d := New(Config{BaseURL: "http://nope.invalid"})
	err := d.DeleteVolume(context.Background(),
		domain.VolumeRef{Kind: "stub", ID: "x"})
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Errorf("err = %v, want ErrUnsupportedKind", err)
	}
}

func TestDeleteVolume_ZeroValueIsNoop(t *testing.T) {
	// A zero-value VolumeRef means the orchestrator never produced
	// a clone (e.g., aborted before clone). DeleteVolume should
	// silently succeed in that case so cleanup paths can be
	// unconditional.
	d := New(Config{BaseURL: "http://nope.invalid"})
	if err := d.DeleteVolume(context.Background(), domain.VolumeRef{}); err != nil {
		t.Errorf("zero-value delete should be no-op, got %v", err)
	}
}
