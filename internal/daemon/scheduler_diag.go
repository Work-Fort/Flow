// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/scheduler"
)

// In-memory map keyed by (agent_id, workflow_id) so the release
// endpoint can find the live *AgentClaim the scheduler returned.
// Local to this file because nothing else in production needs it —
// a real workflow engine will hold AgentClaim values directly.
var diagClaims = struct {
	m map[string]*domain.AgentClaim
}{m: make(map[string]*domain.AgentClaim)}

// registerSchedulerAndAuditDiagRoutes registers the scheduler diag
// claim/release endpoints and the audit list-by-workflow endpoint.
// sch may be nil (returns 503 from the scheduler endpoints); audit
// must NOT be nil — the daemon always has a Store.
func registerSchedulerAndAuditDiagRoutes(api huma.API, sch *scheduler.Scheduler, audit domain.AuditEventStore) {
	type claimInput struct {
		Body struct {
			Role            string `json:"role"`
			Project         string `json:"project"`
			WorkflowID      string `json:"workflow_id"`
			LeaseTTLSeconds int    `json:"lease_ttl_seconds"`
		}
	}
	type claimOutput struct {
		Body struct {
			AgentID    string `json:"agent_id"`
			WorkflowID string `json:"workflow_id"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID:   "scheduler-diag-claim",
		Method:        http.MethodPost,
		Path:          "/v1/scheduler/_diag/claim",
		Summary:       "Internal: drive Scheduler.AcquireAgent",
		DefaultStatus: http.StatusOK,
		Tags:          []string{"Scheduler/_diag"},
	}, func(ctx context.Context, in *claimInput) (*claimOutput, error) {
		if sch == nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "scheduler not configured")
		}
		ttl := time.Duration(in.Body.LeaseTTLSeconds) * time.Second
		claim, err := sch.AcquireAgent(ctx, in.Body.Role, in.Body.Project, in.Body.WorkflowID, ttl)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		diagClaims.m[claim.AgentID+"|"+claim.WorkflowID] = claim
		out := &claimOutput{}
		out.Body.AgentID = claim.AgentID
		out.Body.WorkflowID = claim.WorkflowID
		return out, nil
	})

	type releaseInput struct {
		Body struct {
			AgentID    string `json:"agent_id"`
			WorkflowID string `json:"workflow_id"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID:   "scheduler-diag-release",
		Method:        http.MethodPost,
		Path:          "/v1/scheduler/_diag/release",
		Summary:       "Internal: drive Scheduler.ReleaseAgent",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Scheduler/_diag"},
	}, func(ctx context.Context, in *releaseInput) (*struct{}, error) {
		if sch == nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "scheduler not configured")
		}
		claim, ok := diagClaims.m[in.Body.AgentID+"|"+in.Body.WorkflowID]
		if !ok {
			return nil, huma.NewError(http.StatusNotFound, "no live diag claim")
		}
		if err := sch.ReleaseAgent(ctx, claim); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		delete(diagClaims.m, in.Body.AgentID+"|"+in.Body.WorkflowID)
		return nil, nil
	})

	// --- audit list endpoint ---

	type eventResp struct {
		Type     string `json:"type"`
		AgentID  string `json:"agent_id"`
		Workflow string `json:"workflow_id"`
	}
	type listInput struct {
		ID string `path:"id"`
	}
	type listOutput struct {
		Body struct {
			Events []eventResp `json:"events"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID: "audit-diag-by-workflow",
		Method:      http.MethodGet,
		Path:        "/v1/audit/_diag/by-workflow/{id}",
		Summary:     "Internal: list audit events by workflow ID",
		Tags:        []string{"Audit/_diag"},
	}, func(ctx context.Context, in *listInput) (*listOutput, error) {
		events, err := audit.ListAuditEventsByWorkflow(ctx, in.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, err.Error())
		}
		out := &listOutput{}
		out.Body.Events = make([]eventResp, 0, len(events))
		for _, e := range events {
			out.Body.Events = append(out.Body.Events, eventResp{
				Type:     string(e.Type),
				AgentID:  e.AgentID,
				Workflow: e.WorkflowID,
			})
		}
		return out, nil
	})
}
