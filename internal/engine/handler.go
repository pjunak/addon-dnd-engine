// Package engine exposes the versioned worker service boundary around pure
// rules and the selected rules-data provider.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pjunak/addon-dnd-engine/character"
	"github.com/pjunak/addon-dnd-engine/internal/provider"
	"github.com/pjunak/addon-dnd-engine/internal/rules"
	"github.com/pjunak/ttrpg-codex/sdk/go/workerrpc"

	"github.com/pjunak/addon-dnd-engine/internal/jsonexact"
)

const (
	Contract        = "dnd5e.rules-engine"
	ContractVersion = "6.0.0"
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

type characterResponse struct {
	ContractVersion string                 `json:"contractVersion"`
	Identity        provider.Identity      `json:"identity"`
	Evaluation      character.Result       `json:"evaluation"`
	Policy          *rules.CharacterPolicy `json:"policy"`
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
	case methodPrefix + "evaluate-character", methodPrefix + "character-play":
		if len(request.Params) > character.MaxRequestBytes {
			return nil, invalidRequest(fmt.Sprintf("character request is %d bytes; the limit is %d bytes", len(request.Params), character.MaxRequestBytes))
		}
		var input struct {
			ContractVersion string           `json:"contractVersion"`
			Inputs          character.Inputs `json:"inputs"`
			Change          json.RawMessage  `json:"change,omitempty"`
		}
		if jsonexact.Decode(request.Params, &input) != nil || input.ContractVersion != character.ContractVersion {
			return nil, invalidRequest("character request is invalid")
		}
		play := request.Method == methodPrefix+"character-play"
		if !play && input.Change != nil {
			return nil, invalidRequest("evaluation does not accept a play command")
		}
		var command character.PlayCommand
		if play {
			decoded, err := character.DecodePlayCommand(input.Change)
			if err != nil {
				return nil, invalidRequest(err.Error())
			}
			command = decoded
		}
		identity, records, profile, err := handler.provider.Evaluation(ctx, request.Meta)
		if err != nil {
			return nil, err
		}
		var result character.Result
		if play {
			result, err = rules.ApplyCharacterPlay(input.Inputs, command, records, profile)
			if err != nil {
				return nil, invalidRequest(err.Error())
			}
		} else {
			result = rules.EvaluateCharacter(input.Inputs, records, profile)
		}
		character.FillEmpty(&result)
		return characterResponse{ContractVersion: character.ResponseVersion, Identity: identity, Evaluation: result, Policy: profile.Constants.Character}, nil
	case methodPrefix + "context":
		if err := decodeEmpty(request.Params); err != nil {
			return nil, invalidRequest("rules engine context request is invalid")
		}
		current := handler.provider.Inspect(ctx, request.Meta)
		return contextResponse{
			ContractVersion: "rules-engine-context.v1",
			EngineContract:  Contract, EngineContractVersion: ContractVersion,
			Available: current.Available, Status: current.Status,
			Identity: current.Identity, Errors: append([]string{}, current.Errors...),
		}, nil
	case methodPrefix + "get-record":
		var input getRequest
		if jsonexact.Decode(request.Params, &input) != nil || input.ContractVersion != "rules-engine-get.v1" {
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
		if jsonexact.Decode(request.Params, &input) != nil || input.ContractVersion != "rules-engine-query.v1" {
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

	default:
		return nil, workerrpc.NewRPCError(workerrpc.JSONRPCMethodNotFound, workerrpc.KindNotFound,
			"The rules engine method was not found.", false, nil)
	}
}

func invalidRequest(message string) error {
	return workerrpc.NewRPCError(workerrpc.JSONRPCInvalidParams, workerrpc.KindInvalidRequest,
		message, false, nil)
}

func decodeEmpty(body json.RawMessage) error {
	var value map[string]json.RawMessage
	if err := jsonexact.Decode(body, &value); err != nil {
		return err
	}
	if value == nil || len(value) != 0 {
		return errors.New("expected an empty object")
	}
	return nil
}

var _ workerrpc.RequestHandler = (*Handler)(nil)
