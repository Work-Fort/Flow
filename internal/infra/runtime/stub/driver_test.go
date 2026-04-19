// SPDX-License-Identifier: GPL-2.0-only
package stub_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/runtime/stub"
)

func TestStubDriver_RecordsCalls(t *testing.T) {
	d := stub.New()
	ctx := context.Background()

	if err := d.RefreshProjectMaster(ctx, "flow", "main"); err != nil {
		t.Fatalf("RefreshProjectMaster: %v", err)
	}
	master := d.GetProjectMasterRef("flow")
	if master.Kind != "stub" || master.ID == "" {
		t.Fatalf("master ref: got %+v", master)
	}

	vol, err := d.CloneWorkItemVolume(ctx, master, "wi-1")
	if err != nil {
		t.Fatalf("CloneWorkItemVolume: %v", err)
	}

	creds := domain.VolumeRef{Kind: "stub", ID: "creds-a3"}
	h, err := d.StartAgentRuntime(ctx, "a_003", creds, vol)
	if err != nil {
		t.Fatalf("StartAgentRuntime: %v", err)
	}

	alive, err := d.IsRuntimeAlive(ctx, h)
	if err != nil || !alive {
		t.Errorf("alive: got (%v, %v), want (true, nil)", alive, err)
	}

	if err := d.StopAgentRuntime(ctx, h); err != nil {
		t.Fatalf("StopAgentRuntime: %v", err)
	}
	alive, _ = d.IsRuntimeAlive(ctx, h)
	if alive {
		t.Error("runtime should be dead after StopAgentRuntime")
	}

	if err := d.DeleteVolume(ctx, vol); err != nil {
		t.Fatalf("DeleteVolume: %v", err)
	}

	want := []string{
		"RefreshProjectMaster:flow:main",
		"CloneWorkItemVolume:flow:wi-1",
		"StartAgentRuntime:a_003",
		"IsRuntimeAlive",
		"StopAgentRuntime",
		"IsRuntimeAlive",
		"DeleteVolume",
	}
	got := d.Calls()
	if len(got) != len(want) {
		t.Fatalf("call log length: got %d, want %d (%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("call[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestStubDriver_SatisfiesRuntimeDriverInterface(t *testing.T) {
	var _ domain.RuntimeDriver = stub.New()
}
