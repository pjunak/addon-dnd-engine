// Package engine exposes the versioned worker service boundary around pure
// rules and the selected rules-data provider.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/pjunak/addon-dnd-engine/internal/provider"
	"github.com/pjunak/addon-dnd-engine/internal/rules"
	"github.com/pjunak/ttrpg-codex/sdk/go/workerrpc"
)

const (
	Contract        = "dnd5e.rules-engine"
	ContractVersion = "3.0.0"
	methodPrefix    = "service/" + Contract + "/"
)

type RulesData interface {
	Inspect(context.Context, *workerrpc.Meta) provider.Context
	Get(context.Context, *workerrpc.Meta, string, string) (provider.Identity, provider.Record, error)
	Query(context.Context, *workerrpc.Meta, provider.Query) (provider.QueryResult, error)
	Ruleset(context.Context, *workerrpc.Meta) (provider.RulesetResult, error)
	Evaluation(context.Context, *workerrpc.Meta) (provider.Identity, rules.Records, rules.Ruleset, error)
}

type Handler struct {
	provider RulesData
}

type contextResponse struct {
	ContractVersion       string            `json:"contractVersion"`
	EngineContract        string            `json:"engineContract"`
	EngineContractVersion string            `json:"engineContractVersion"`
	Available             bool              `json:"available"`
	Status                string            `json:"status"`
	Identity              provider.Identity `json:"identity"`
	Errors                []string          `json:"errors"`
}

type getRequest struct {
	ContractVersion string `json:"contractVersion"`
	Kind            string `json:"kind"`
	ID              string `json:"id"`
}

type getResponse struct {
	ContractVersion string            `json:"contractVersion"`
	Identity        provider.Identity `json:"identity"`
	Record          provider.Record   `json:"record"`
}

type queryRequest struct {
	ContractVersion string `json:"contractVersion"`
	Kind            string `json:"kind"`
	Cursor          string `json:"cursor,omitempty"`
	Limit           int    `json:"limit"`
}

type queryResponse struct {
	ContractVersion string            `json:"contractVersion"`
	Identity        provider.Identity `json:"identity"`
	Records         []provider.Record `json:"records"`
	NextCursor      string            `json:"nextCursor,omitempty"`
}

type deriveRequest struct {
	ContractVersion string          `json:"contractVersion"`
	Operation       string          `json:"operation"`
	Input           json.RawMessage `json:"input"`
}

type deriveResponse struct {
	ContractVersion string             `json:"contractVersion"`
	Value           any                `json:"value"`
	Identity        *provider.Identity `json:"identity,omitempty"`
}

type hydrateRequest struct {
	ContractVersion string          `json:"contractVersion"`
	Decisions       json.RawMessage `json:"decisions"`
}

type hydrateResponse struct {
	ContractVersion string             `json:"contractVersion"`
	Sheet           rules.Object       `json:"sheet"`
	Warnings        []string           `json:"warnings"`
	Identity        *provider.Identity `json:"identity,omitempty"`
}

type builderRequest struct {
	ContractVersion string          `json:"contractVersion"`
	Decisions       json.RawMessage `json:"decisions"`
	Change          json.RawMessage `json:"change,omitempty"`
}

type builderPlanResponse struct {
	ContractVersion string             `json:"contractVersion"`
	Available       bool               `json:"available"`
	Status          string             `json:"status"`
	Plan            rules.Object       `json:"plan,omitempty"`
	Identity        *provider.Identity `json:"identity,omitempty"`
	Errors          []string           `json:"errors"`
}

type builderDecisionsResponse struct {
	ContractVersion string             `json:"contractVersion"`
	Available       bool               `json:"available"`
	Status          string             `json:"status"`
	Decisions       rules.Object       `json:"decisions"`
	Identity        *provider.Identity `json:"identity,omitempty"`
	Errors          []string           `json:"errors"`
}

func New(data RulesData) (*Handler, error) {
	if data == nil {
		return nil, errors.New("rules-data provider client is required")
	}
	return &Handler{provider: data}, nil
}

func (handler *Handler) HandleRPC(ctx context.Context, request workerrpc.Request) (any, error) {
	if handler == nil || handler.provider == nil {
		return nil, errors.New("rules engine is unavailable")
	}
	switch request.Method {
	case methodPrefix + "context":
		if err := decodeEmpty(request.Params); err != nil {
			return nil, invalidRequest("rules engine context request is invalid")
		}
		current := handler.provider.Inspect(ctx, request.Meta)
		return contextResponse{
			ContractVersion: "rules-engine-context.v1",
			EngineContract:  Contract, EngineContractVersion: ContractVersion,
			Available: current.Available, Status: current.Status,
			Identity: current.Identity, Errors: append([]string(nil), current.Errors...),
		}, nil
	case methodPrefix + "get-record":
		var input getRequest
		if decodeExact(request.Params, &input) != nil || input.ContractVersion != "rules-engine-get.v1" {
			return nil, invalidRequest("rules engine get request is invalid")
		}
		identity, record, err := handler.provider.Get(ctx, request.Meta, input.Kind, input.ID)
		if err != nil {
			return nil, err
		}
		return getResponse{
			ContractVersion: "rules-engine-record.v1", Identity: identity, Record: record,
		}, nil
	case methodPrefix + "query-records":
		var input queryRequest
		if decodeExact(request.Params, &input) != nil || input.ContractVersion != "rules-engine-query.v1" {
			return nil, invalidRequest("rules engine query request is invalid")
		}
		result, err := handler.provider.Query(ctx, request.Meta, provider.Query{
			Kind: input.Kind, Cursor: input.Cursor, Limit: input.Limit,
		})
		if err != nil {
			return nil, err
		}
		return queryResponse{
			ContractVersion: "rules-engine-query-result.v1", Identity: result.Identity,
			Records: result.Records, NextCursor: result.NextCursor,
		}, nil
	case methodPrefix + "derive":
		var input deriveRequest
		if decodeExact(request.Params, &input) != nil || input.ContractVersion != "rules-engine-derive.v1" ||
			input.Operation == "" || !objectPayload(input.Input) {
			return nil, invalidRequest("rules engine derive request is invalid")
		}
		value, identity, err := handler.derive(ctx, request.Meta, input)
		if err != nil {
			return nil, err
		}
		return deriveResponse{
			ContractVersion: "rules-engine-derived.v1", Value: value, Identity: identity,
		}, nil
	case methodPrefix + "hydrate":
		var input hydrateRequest
		if decodeExact(request.Params, &input) != nil || input.ContractVersion != "rules-engine-hydrate.v1" ||
			!objectPayload(input.Decisions) {
			return nil, invalidRequest("rules engine hydrate request is invalid")
		}
		decisions, valid := rules.DecodeObject(input.Decisions)
		if !valid {
			return nil, invalidRequest("rules engine hydrate decisions are invalid")
		}
		identity, records, profile, err := handler.provider.Evaluation(ctx, request.Meta)
		if err != nil {
			current := provider.ContextForError(err)
			result := rules.HydrateWithoutRulesData(decisions, current.Status)
			return hydrateResponse{
				ContractVersion: "rules-engine-hydrated.v1", Sheet: result.Sheet, Warnings: result.Warnings,
			}, nil
		}
		normalized := rules.NormalizeBuilderDecisions(decisions, records, profile)
		result := rules.Hydrate(normalized, records, &profile)
		return hydrateResponse{
			ContractVersion: "rules-engine-hydrated.v1", Sheet: result.Sheet,
			Warnings: result.Warnings, Identity: &identity,
		}, nil
	case methodPrefix + "builder-plan":
		input, decisions, err := decodeBuilderRequest(request.Params, "rules-engine-builder-plan.v1", false)
		if err != nil {
			return nil, err
		}
		_ = input
		identity, records, profile, evaluationErr := handler.provider.Evaluation(ctx, request.Meta)
		if evaluationErr != nil {
			current := provider.ContextForError(evaluationErr)
			return builderPlanResponse{
				ContractVersion: "rules-engine-builder-plan-result.v1", Available: false,
				Status: current.Status, Errors: current.Errors,
			}, nil
		}
		return builderPlanResponse{
			ContractVersion: "rules-engine-builder-plan-result.v1", Available: true, Status: "ready",
			Plan: rules.BuilderPlan(decisions, records, profile), Identity: &identity, Errors: []string{},
		}, nil
	case methodPrefix + "apply-builder-choice":
		input, decisions, err := decodeBuilderRequest(request.Params, "rules-engine-builder-change.v1", true)
		if err != nil {
			return nil, err
		}
		change, valid := rules.DecodeObject(input.Change)
		if !valid {
			return nil, invalidRequest("rules engine builder change is invalid")
		}
		identity, records, profile, evaluationErr := handler.provider.Evaluation(ctx, request.Meta)
		if evaluationErr != nil {
			current := provider.ContextForError(evaluationErr)
			return builderDecisionsResponse{
				ContractVersion: "rules-engine-builder-decisions.v1", Available: false,
				Status: current.Status, Decisions: decisions, Errors: current.Errors,
			}, nil
		}
		return builderDecisionsResponse{
			ContractVersion: "rules-engine-builder-decisions.v1", Available: true, Status: "ready",
			Decisions: rules.ApplyBuilderChoice(decisions, change, records, profile),
			Identity:  &identity, Errors: []string{},
		}, nil
	case methodPrefix + "reconcile-builder-decisions":
		_, decisions, err := decodeBuilderRequest(request.Params, "rules-engine-builder-reconcile.v1", false)
		if err != nil {
			return nil, err
		}
		identity, records, profile, evaluationErr := handler.provider.Evaluation(ctx, request.Meta)
		if evaluationErr != nil {
			current := provider.ContextForError(evaluationErr)
			return builderDecisionsResponse{
				ContractVersion: "rules-engine-builder-decisions.v1", Available: false,
				Status: current.Status, Decisions: decisions, Errors: current.Errors,
			}, nil
		}
		return builderDecisionsResponse{
			ContractVersion: "rules-engine-builder-decisions.v1", Available: true, Status: "ready",
			Decisions: rules.ReconcileBuilderDecisions(decisions, records, profile),
			Identity:  &identity, Errors: []string{},
		}, nil
	default:
		return nil, workerrpc.NewRPCError(workerrpc.JSONRPCMethodNotFound, workerrpc.KindNotFound,
			"The rules engine method was not found.", false, nil)
	}
}

func decodeBuilderRequest(
	body json.RawMessage,
	contractVersion string,
	requireChange bool,
) (builderRequest, rules.Object, error) {
	var input builderRequest
	if decodeExact(body, &input) != nil || input.ContractVersion != contractVersion ||
		!objectPayload(input.Decisions) || requireChange && !objectPayload(input.Change) ||
		!requireChange && len(input.Change) != 0 {
		return builderRequest{}, nil, invalidRequest("rules engine builder request is invalid")
	}
	decisions, valid := rules.DecodeObject(input.Decisions)
	if !valid {
		return builderRequest{}, nil, invalidRequest("rules engine builder decisions are invalid")
	}
	return input, decisions, nil
}

func (handler *Handler) derive(
	ctx context.Context,
	meta *workerrpc.Meta,
	request deriveRequest,
) (any, *provider.Identity, error) {
	switch request.Operation {
	case "ability-modifier":
		var input struct {
			Score *float64 `json:"score"`
		}
		if decodeExact(request.Input, &input) != nil || input.Score == nil {
			return nil, nil, invalidDerive()
		}
		return rules.AbilityModifier(*input.Score), nil, nil
	case "proficiency-bonus":
		var input struct {
			TotalLevel *float64 `json:"totalLevel"`
		}
		if decodeExact(request.Input, &input) != nil || input.TotalLevel == nil {
			return nil, nil, invalidDerive()
		}
		return rules.ProficiencyBonus(*input.TotalLevel), nil, nil
	case "hit-die-average":
		var input struct {
			HitDie string `json:"hitDie"`
		}
		if decodeExact(request.Input, &input) != nil || input.HitDie == "" {
			return nil, nil, invalidDerive()
		}
		return rules.HitDieAverage(input.HitDie), nil, nil
	case "clamp-hp":
		var input struct {
			HitPoints *float64 `json:"hitPoints"`
			Maximum   *float64 `json:"maximum"`
		}
		if decodeExact(request.Input, &input) != nil || input.HitPoints == nil || input.Maximum == nil {
			return nil, nil, invalidDerive()
		}
		return rules.ClampHP(*input.HitPoints, *input.Maximum), nil, nil
	case "save-dc":
		var input struct {
			AbilityScore *float64 `json:"abilityScore"`
			TotalLevel   *float64 `json:"totalLevel"`
		}
		if decodeExact(request.Input, &input) != nil || input.AbilityScore == nil || input.TotalLevel == nil {
			return nil, nil, invalidDerive()
		}
		return rules.SaveDC(*input.AbilityScore, *input.TotalLevel), nil, nil
	case "feat-asi-from":
		var input struct {
			Grant map[string]any `json:"grant"`
		}
		if decodeExact(request.Input, &input) != nil || input.Grant == nil {
			return nil, nil, invalidDerive()
		}
		return rules.FeatASIFrom(input.Grant), nil, nil
	}
	if !rulesetOperation(request.Operation) {
		return nil, nil, invalidRequest("rules engine derive operation is unknown")
	}

	profile, err := handler.provider.Ruleset(ctx, meta)
	if err != nil {
		return nil, nil, err
	}
	identity := profile.Identity
	switch request.Operation {
	case "scroll-copy-cost":
		var input struct {
			Level *float64 `json:"level"`
		}
		if decodeExact(request.Input, &input) != nil || input.Level == nil {
			return nil, nil, invalidDerive()
		}
		return rules.ScrollCopyCost(*input.Level, profile.Ruleset), &identity, nil
	case "point-buy-cost":
		var input struct {
			Score *int `json:"score"`
		}
		if decodeExact(request.Input, &input) != nil || input.Score == nil {
			return nil, nil, invalidDerive()
		}
		return rules.PointBuyCost(*input.Score, profile.Ruleset), &identity, nil
	case "points-spent":
		var input struct {
			Scores map[string]int `json:"scores"`
		}
		if decodeExact(request.Input, &input) != nil || input.Scores == nil {
			return nil, nil, invalidDerive()
		}
		return rules.PointsSpent(input.Scores, profile.Ruleset), &identity, nil
	case "multiclass-slots":
		var input struct {
			CasterLevel *int `json:"casterLevel"`
		}
		if decodeExact(request.Input, &input) != nil || input.CasterLevel == nil {
			return nil, nil, invalidDerive()
		}
		return rules.MulticlassSlots(*input.CasterLevel, profile.Ruleset), &identity, nil
	case "pact-magic":
		var input struct {
			Level *int `json:"level"`
		}
		if decodeExact(request.Input, &input) != nil || input.Level == nil {
			return nil, nil, invalidDerive()
		}
		return rules.PactMagic(*input.Level, profile.Ruleset), &identity, nil
	case "feat-ability-cap":
		var input struct {
			Feat map[string]any `json:"feat"`
		}
		if decodeExact(request.Input, &input) != nil || input.Feat == nil {
			return nil, nil, invalidDerive()
		}
		return rules.FeatAbilityCap(input.Feat, profile.Ruleset), &identity, nil
	}
	panic("ruleset operation was accepted but not handled")
}

func rulesetOperation(operation string) bool {
	switch operation {
	case "scroll-copy-cost", "point-buy-cost", "points-spent", "multiclass-slots", "pact-magic", "feat-ability-cap":
		return true
	default:
		return false
	}
}

func invalidDerive() error {
	return invalidRequest("rules engine derive input is invalid")
}

func invalidRequest(message string) error {
	return workerrpc.NewRPCError(workerrpc.JSONRPCInvalidParams, workerrpc.KindInvalidRequest,
		message, false, nil)
}

func objectPayload(body json.RawMessage) bool {
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 1 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' && json.Valid(trimmed)
}

func decodeEmpty(body json.RawMessage) error {
	var value map[string]json.RawMessage
	if err := decodeExact(body, &value); err != nil {
		return err
	}
	if value == nil || len(value) != 0 {
		return errors.New("expected an empty object")
	}
	return nil
}

func decodeExact(body json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON contains more than one value")
	}
	return nil
}

var _ workerrpc.RequestHandler = (*Handler)(nil)
