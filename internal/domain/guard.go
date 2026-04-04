// SPDX-License-Identifier: GPL-2.0-only
package domain

import (
	"encoding/json"
	"fmt"

	"github.com/google/cel-go/cel"
)

type GuardContext struct {
	Item     GuardItem     `json:"item"`
	Actor    GuardActor    `json:"actor"`
	Approval GuardApproval `json:"approval"`
}

type GuardItem struct {
	Title    string         `json:"title"`
	Priority string         `json:"priority"`
	Fields   map[string]any `json:"fields"`
	Step     string         `json:"step"`
}

type GuardActor struct {
	RoleID  string `json:"role_id"`
	AgentID string `json:"agent_id"`
}

type GuardApproval struct {
	Count      int `json:"count"`
	Rejections int `json:"rejections"`
}

func celEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("item", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("actor", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("approval", cel.MapType(cel.StringType, cel.DynType)),
	)
}

// EvaluateGuard returns nil if expression is empty or evaluates true.
// Returns ErrGuardDenied for false, ErrInvalidGuard for compile/eval errors.
func EvaluateGuard(expression string, ctx GuardContext) error {
	if expression == "" {
		return nil
	}
	env, err := celEnv()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGuard, err)
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGuard, issues.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGuard, err)
	}
	data, _ := json.Marshal(ctx)
	var vars map[string]any
	json.Unmarshal(data, &vars) //nolint:errcheck
	out, _, err := prg.Eval(vars)
	if err != nil {
		return fmt.Errorf("%w: eval: %v", ErrInvalidGuard, err)
	}
	if result, ok := out.Value().(bool); !ok || !result {
		return ErrGuardDenied
	}
	return nil
}

// ValidateGuard compiles the expression and returns ErrInvalidGuard if it fails.
func ValidateGuard(expression string) error {
	if expression == "" {
		return nil
	}
	env, err := celEnv()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGuard, err)
	}
	_, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("%w: %v", ErrInvalidGuard, issues.Err())
	}
	return nil
}
