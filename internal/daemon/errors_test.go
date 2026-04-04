// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Work-Fort/Flow/internal/domain"
)

func TestMapDomainErr(t *testing.T) {
	errOther := errors.New("some unknown error")

	cases := []struct {
		name       string
		input      error
		wantStatus int
		wantNil    bool
	}{
		{"nil", nil, 0, true},
		{"ErrNotFound", domain.ErrNotFound, http.StatusNotFound, false},
		{"ErrAlreadyExists", domain.ErrAlreadyExists, http.StatusConflict, false},
		{"ErrHasDependencies", domain.ErrHasDependencies, http.StatusConflict, false},
		{"ErrGuardDenied", domain.ErrGuardDenied, http.StatusUnprocessableEntity, false},
		{"ErrInvalidTransition", domain.ErrInvalidTransition, http.StatusUnprocessableEntity, false},
		{"ErrNotAtGateStep", domain.ErrNotAtGateStep, http.StatusUnprocessableEntity, false},
		{"ErrGateRequiresApproval", domain.ErrGateRequiresApproval, http.StatusUnprocessableEntity, false},
		{"ErrInvalidGuard", domain.ErrInvalidGuard, http.StatusUnprocessableEntity, false},
		{"ErrPermissionDenied", domain.ErrPermissionDenied, http.StatusForbidden, false},
		{"unknown", errOther, http.StatusInternalServerError, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapDomainErr(tc.input)
			if tc.wantNil {
				if got != nil {
					t.Errorf("want nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want error, got nil")
			}
			humaErr, ok := got.(huma.StatusError)
			if !ok {
				t.Fatalf("want huma.StatusError, got %T: %v", got, got)
			}
			if humaErr.GetStatus() != tc.wantStatus {
				t.Errorf("status: got %d, want %d", humaErr.GetStatus(), tc.wantStatus)
			}
		})
	}
}
