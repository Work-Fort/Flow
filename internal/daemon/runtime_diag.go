// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Flow/internal/domain"
)

// registerRuntimeDiagRoutes installs an internal-only diagnostic
// endpoint that drives the bound RuntimeDriver. Used by the e2e
// harness to exercise the RuntimeDriver interface end-to-end through
// the daemon. Returns 503 when no driver is bound (production today).
//
// Not part of the published API. Tagged "Runtime/_diag" in the OpenAPI
// surface and gated to localhost in a future plan once production
// drivers land.
func registerRuntimeDiagRoutes(api huma.API, rt domain.RuntimeDriver) {
	// startInput carries the canonical demo sequence inputs plus an
	// optional caller-supplied creds-volume ref. When CredsVolumeRef is
	// the zero value, the handler asks the driver to materialize one
	// via CloneWorkItemVolume from the project master — this lets the
	// stub driver and the production Nexus driver both work without a
	// kind check leaking into the handler.
	type startInput struct {
		Body struct {
			ProjectID      string           `json:"project_id"`
			WorkItemID     string           `json:"work_item_id"`
			AgentID        string           `json:"agent_id"`
			GitRef         string           `json:"git_ref"`
			CredsVolumeRef domain.VolumeRef `json:"creds_volume_ref,omitempty"`
		}
	}
	type startOutput struct {
		Body struct {
			Master domain.VolumeRef     `json:"master"`
			Work   domain.VolumeRef     `json:"work"`
			Handle domain.RuntimeHandle `json:"handle"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID:   "runtime-diag-start",
		Method:        http.MethodPost,
		Path:          "/v1/runtime/_diag/start",
		Summary:       "Internal: drive RuntimeDriver end-to-end (refresh → clone → start)",
		DefaultStatus: http.StatusOK,
		Tags:          []string{"Runtime/_diag"},
	}, func(ctx context.Context, input *startInput) (*startOutput, error) {
		if rt == nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "runtime driver not configured")
		}
		if err := rt.RefreshProjectMaster(ctx, input.Body.ProjectID, input.Body.GitRef); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		master := rt.GetProjectMasterRef(input.Body.ProjectID)
		work, err := rt.CloneWorkItemVolume(ctx, master, input.Body.WorkItemID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}

		// Resolve the creds volume:
		//   - when the caller supplied one (e.g. an integration test
		//     that pre-created its own ref), trust it verbatim
		//   - otherwise, ask the driver to materialize one by cloning
		//     the project master under a per-agent name. The driver's
		//     emit-kind is preserved so StartAgentRuntime's kind check
		//     passes for both stub.Driver and nexus.Driver.
		creds := input.Body.CredsVolumeRef
		if creds == (domain.VolumeRef{}) {
			creds, err = rt.CloneWorkItemVolume(ctx, master, "creds-"+input.Body.AgentID)
			if err != nil {
				return nil, huma.NewError(http.StatusInternalServerError, "materialize creds: "+err.Error())
			}
		}

		h, err := rt.StartAgentRuntime(ctx, input.Body.AgentID, creds, work)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		out := &startOutput{}
		out.Body.Master = master
		out.Body.Work = work
		out.Body.Handle = h
		return out, nil
	})

	type stopInput struct {
		Body struct {
			Handle domain.RuntimeHandle `json:"handle"`
			Volume domain.VolumeRef     `json:"volume"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID:   "runtime-diag-stop",
		Method:        http.MethodPost,
		Path:          "/v1/runtime/_diag/stop",
		Summary:       "Internal: drive RuntimeDriver stop + delete-volume",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Runtime/_diag"},
	}, func(ctx context.Context, input *stopInput) (*struct{}, error) {
		if rt == nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "runtime driver not configured")
		}
		if err := rt.StopAgentRuntime(ctx, input.Body.Handle); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		if err := rt.DeleteVolume(ctx, input.Body.Volume); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		return nil, nil
	})
}
