// SPDX-License-Identifier: GPL-2.0-only
package domain_test

import (
	"errors"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
)

func TestEvaluateGuard_Empty(t *testing.T) {
	if err := domain.EvaluateGuard("", domain.GuardContext{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestEvaluateGuard_True(t *testing.T) {
	ctx := domain.GuardContext{
		Item: domain.GuardItem{Priority: "high"},
	}
	if err := domain.EvaluateGuard(`item.priority == "high"`, ctx); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestEvaluateGuard_False(t *testing.T) {
	ctx := domain.GuardContext{
		Item: domain.GuardItem{Priority: "low"},
	}
	err := domain.EvaluateGuard(`item.priority == "high"`, ctx)
	if !errors.Is(err, domain.ErrGuardDenied) {
		t.Fatalf("expected ErrGuardDenied, got %v", err)
	}
}

func TestEvaluateGuard_InvalidSyntax(t *testing.T) {
	err := domain.EvaluateGuard(`item.priority ==`, domain.GuardContext{})
	if !errors.Is(err, domain.ErrInvalidGuard) {
		t.Fatalf("expected ErrInvalidGuard, got %v", err)
	}
}

func TestValidateGuard_Valid(t *testing.T) {
	if err := domain.ValidateGuard(`item.priority == "high"`); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateGuard_Invalid(t *testing.T) {
	err := domain.ValidateGuard(`item.priority ==`)
	if !errors.Is(err, domain.ErrInvalidGuard) {
		t.Fatalf("expected ErrInvalidGuard, got %v", err)
	}
}
