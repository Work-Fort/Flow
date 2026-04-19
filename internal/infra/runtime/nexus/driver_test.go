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
	"time"

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

func TestStartAgentRuntime_HappyPath(t *testing.T) {
	var seq []string
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"POST /v1/vms": func(w http.ResponseWriter, r *http.Request) {
			seq = append(seq, "create-vm")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"vm_abc","name":"agent-a_007","state":"created"}`)
		},
		"POST /v1/drives/d_creds/attach": func(w http.ResponseWriter, r *http.Request) {
			seq = append(seq, "attach-creds")
			w.Write([]byte(`{"status":"ok"}`))
		},
		"POST /v1/drives/d_work/attach": func(w http.ResponseWriter, r *http.Request) {
			seq = append(seq, "attach-work")
			w.Write([]byte(`{"status":"ok"}`))
		},
		"POST /v1/vms/vm_abc/start": func(w http.ResponseWriter, r *http.Request) {
			seq = append(seq, "start-vm")
			w.WriteHeader(http.StatusNoContent)
		},
	})

	d := New(Config{BaseURL: url, VMImage: "alpine"})
	creds := domain.VolumeRef{Kind: VolumeKind, ID: "d_creds"}
	work := domain.VolumeRef{Kind: VolumeKind, ID: "d_work"}
	h, err := d.StartAgentRuntime(context.Background(), "a_007", creds, work)
	if err != nil {
		t.Fatalf("StartAgentRuntime: %v", err)
	}
	if h.Kind != RuntimeHandleKind {
		t.Errorf("handle Kind = %q, want %q", h.Kind, RuntimeHandleKind)
	}
	if h.ID != "vm_abc" {
		t.Errorf("handle ID = %q, want vm_abc", h.ID)
	}
	want := []string{"create-vm", "attach-creds", "attach-work", "start-vm"}
	if len(seq) != len(want) {
		t.Fatalf("call sequence = %v, want %v", seq, want)
	}
	for i, w := range want {
		if seq[i] != w {
			t.Errorf("seq[%d] = %q, want %q", i, seq[i], w)
		}
	}
}

func TestStartAgentRuntime_RejectsWrongKindCreds(t *testing.T) {
	d := New(Config{BaseURL: "http://nope.invalid"})
	_, err := d.StartAgentRuntime(context.Background(), "a_007",
		domain.VolumeRef{Kind: "stub", ID: "x"},
		domain.VolumeRef{Kind: VolumeKind, ID: "y"})
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Errorf("err = %v, want ErrUnsupportedKind", err)
	}
}

func TestStartAgentRuntime_AttachFailureCleansUpVM(t *testing.T) {
	var deletedVMs []string
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"POST /v1/vms": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"vm_xyz","state":"created"}`)
		},
		"POST /v1/drives/d_creds/attach": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "drive busy", http.StatusConflict)
		},
		"DELETE /v1/vms/vm_xyz": func(w http.ResponseWriter, r *http.Request) {
			deletedVMs = append(deletedVMs, "vm_xyz")
			w.WriteHeader(http.StatusNoContent)
		},
	})
	d := New(Config{BaseURL: url})
	_, err := d.StartAgentRuntime(context.Background(), "a",
		domain.VolumeRef{Kind: VolumeKind, ID: "d_creds"},
		domain.VolumeRef{Kind: VolumeKind, ID: "d_work"})
	if err == nil {
		t.Fatal("expected attach failure to bubble up")
	}
	if len(deletedVMs) != 1 || deletedVMs[0] != "vm_xyz" {
		t.Errorf("VM should be deleted on attach failure; deletedVMs=%v", deletedVMs)
	}
}

func TestStopAgentRuntime_HappyPath(t *testing.T) {
	var seq []string
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"POST /v1/vms/vm_abc/stop": func(w http.ResponseWriter, r *http.Request) {
			seq = append(seq, "stop")
			w.WriteHeader(http.StatusNoContent)
		},
		"DELETE /v1/vms/vm_abc": func(w http.ResponseWriter, r *http.Request) {
			seq = append(seq, "delete")
			w.WriteHeader(http.StatusNoContent)
		},
	})
	d := New(Config{BaseURL: url})
	err := d.StopAgentRuntime(context.Background(),
		domain.RuntimeHandle{Kind: RuntimeHandleKind, ID: "vm_abc"})
	if err != nil {
		t.Fatalf("StopAgentRuntime: %v", err)
	}
	want := []string{"stop", "delete"}
	if len(seq) != 2 || seq[0] != want[0] || seq[1] != want[1] {
		t.Errorf("seq = %v, want %v", seq, want)
	}
}

func TestStopAgentRuntime_StopAlreadyStopped_StillDeletes(t *testing.T) {
	var seq []string
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"POST /v1/vms/vm_abc/stop": func(w http.ResponseWriter, r *http.Request) {
			seq = append(seq, "stop")
			http.Error(w, "already stopped", http.StatusConflict)
		},
		"DELETE /v1/vms/vm_abc": func(w http.ResponseWriter, r *http.Request) {
			seq = append(seq, "delete")
			w.WriteHeader(http.StatusNoContent)
		},
	})
	d := New(Config{BaseURL: url})
	err := d.StopAgentRuntime(context.Background(),
		domain.RuntimeHandle{Kind: RuntimeHandleKind, ID: "vm_abc"})
	if err != nil {
		t.Errorf("idempotent stop should not error on already-stopped, got %v", err)
	}
	if len(seq) != 2 || seq[1] != "delete" {
		t.Errorf("delete must run even when stop returns 409; seq=%v", seq)
	}
}

func TestStopAgentRuntime_404IsIdempotent(t *testing.T) {
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"POST /v1/vms/vm_gone/stop": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		},
		"DELETE /v1/vms/vm_gone": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		},
	})
	d := New(Config{BaseURL: url})
	err := d.StopAgentRuntime(context.Background(),
		domain.RuntimeHandle{Kind: RuntimeHandleKind, ID: "vm_gone"})
	if err != nil {
		t.Errorf("404 stop+delete should be no-op, got %v", err)
	}
}

func TestIsRuntimeAlive_RunningTrue(t *testing.T) {
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"GET /v1/vms/vm_abc": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, `{"id":"vm_abc","state":"running"}`)
		},
	})
	d := New(Config{BaseURL: url})
	alive, err := d.IsRuntimeAlive(context.Background(),
		domain.RuntimeHandle{Kind: RuntimeHandleKind, ID: "vm_abc"})
	if err != nil || !alive {
		t.Errorf("alive=%v err=%v, want (true, nil)", alive, err)
	}
}

func TestIsRuntimeAlive_StoppedFalse(t *testing.T) {
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"GET /v1/vms/vm_abc": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, `{"id":"vm_abc","state":"stopped"}`)
		},
	})
	d := New(Config{BaseURL: url})
	alive, err := d.IsRuntimeAlive(context.Background(),
		domain.RuntimeHandle{Kind: RuntimeHandleKind, ID: "vm_abc"})
	if err != nil || alive {
		t.Errorf("alive=%v err=%v, want (false, nil)", alive, err)
	}
}

func TestIsRuntimeAlive_404False(t *testing.T) {
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"GET /v1/vms/vm_gone": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		},
	})
	d := New(Config{BaseURL: url})
	alive, err := d.IsRuntimeAlive(context.Background(),
		domain.RuntimeHandle{Kind: RuntimeHandleKind, ID: "vm_gone"})
	if err != nil || alive {
		t.Errorf("alive=%v err=%v, want (false, nil) on 404", alive, err)
	}
}

func TestIsRuntimeAlive_RespectsContextDeadline(t *testing.T) {
	// Per the port comment: "MUST NOT block indefinitely — drivers
	// should cap internal timeouts at ctx's deadline or ~2s,
	// whichever is smaller." A hung Nexus must not stall the
	// scheduler's liveness sweep.
	url := fakeNexus(t, map[string]http.HandlerFunc{
		"GET /v1/vms/vm_slow": func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done() // mirror the client cancel
		},
	})
	d := New(Config{BaseURL: url})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _ = d.IsRuntimeAlive(ctx,
		domain.RuntimeHandle{Kind: RuntimeHandleKind, ID: "vm_slow"})
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Errorf("IsRuntimeAlive blocked %v, want < 1s when ctx deadline is 50ms", elapsed)
	}
}
