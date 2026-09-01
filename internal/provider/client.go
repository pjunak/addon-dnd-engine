// Package provider adapts the generic host service client to the
// dnd5e.rules-data v3 content contract.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/pjunak/addon-dnd-engine/internal/rules"
	"github.com/pjunak/ttrpg-codex/sdk/go/workerrpc"
)

const (
	RulesDataContract = "dnd5e.rules-data"
	RulesDataVersion  = "3.0.0"
	RulesSetID        = "rules"
)

type Client struct {
	services *workerrpc.ServiceClient
}

type Identity struct {
	ProviderAddonID         string `json:"providerAddonId"`
	ProviderContractVersion string `json:"providerContractVersion"`
	ProviderGeneration      string `json:"providerGeneration"`
	ContentRevision         string `json:"contentRevision"`
	RulesetID               string `json:"rulesetId,omitempty"`
	RulesetVersion          int    `json:"rulesetVersion,omitempty"`
	Edition                 string `json:"edition,omitempty"`
}

type Context struct {
	Available bool     `json:"available"`
	Status    string   `json:"status"`
	Identity  Identity `json:"identity"`
	Errors    []string `json:"errors"`
}

type Record struct {
	Kind  string          `json:"kind"`
	ID    string          `json:"id"`
	Value json.RawMessage `json:"value"`
}

type Query struct {
	Kind   string
	Cursor string
	Limit  int
}

type QueryResult struct {
	Identity   Identity
	Records    []Record
	NextCursor string
}

type RulesetResult struct {
	Identity Identity
	Ruleset  rules.Ruleset
}

func New(caller workerrpc.ServiceCaller) (*Client, error) {
	services, err := workerrpc.NewServiceClient(caller)
	if err != nil {
		return nil, err
	}
	return &Client{services: services}, nil
}

func (client *Client) Inspect(ctx context.Context, meta *workerrpc.Meta) Context {
	result, err := client.Ruleset(ctx, meta)
	if err == nil {
		return Context{
			Available: true, Status: "ready", Identity: result.Identity, Errors: []string{},
		}
	}
	status := "unavailable"
	var failure *workerrpc.RPCError
	if errors.As(err, &failure) && failure.Data != nil {
		switch failure.Data.Kind {
		case workerrpc.KindUnauthorized, workerrpc.KindNotFound:
			status = "missing"
		case workerrpc.KindValidationFailed:
			status = "incompatible"
		case workerrpc.KindStaleBinding:
			status = "stale"
		}
	}
	return Context{
		Available: false, Status: status, Errors: []string{boundedMessage(err)},
	}
}

func (client *Client) Catalog(
	ctx context.Context,
	meta *workerrpc.Meta,
) (Identity, error) {
	call, err := client.call(ctx, meta, "catalog", map[string]any{})
	if err != nil {
		return Identity{}, err
	}
	var response struct {
		ContractVersion string `json:"contractVersion"`
		Sets            []struct {
			ID          string         `json:"id"`
			Revision    string         `json:"revision"`
			RecordCount int            `json:"recordCount"`
			Kinds       map[string]int `json:"kinds"`
		} `json:"sets"`
	}
	if err := decodeExact(call.Result, &response); err != nil ||
		response.ContractVersion != "content-catalog.v1" {
		return Identity{}, incompatible("rules-data catalog is invalid")
	}
	for _, set := range response.Sets {
		if set.ID == RulesSetID && set.Revision != "" && set.RecordCount > 0 && len(set.Kinds) > 0 {
			identity := identityOf(call, set.Revision)
			return identity, nil
		}
	}
	return Identity{}, incompatible("rules-data catalog has no rules content set")
}

func (client *Client) Get(
	ctx context.Context,
	meta *workerrpc.Meta,
	kind string,
	id string,
) (Identity, Record, error) {
	if !validReference(kind, id) {
		return Identity{}, Record{}, invalid("rules-data record reference is invalid")
	}
	call, err := client.call(ctx, meta, "get", map[string]any{
		"setId": RulesSetID, "kind": kind, "id": id,
	})
	if err != nil {
		return Identity{}, Record{}, err
	}
	var response struct {
		ContractVersion string `json:"contractVersion"`
		SetID           string `json:"setId"`
		Revision        string `json:"revision"`
		Record          Record `json:"record"`
	}
	if err := decodeExact(call.Result, &response); err != nil ||
		response.ContractVersion != "content-record.v1" || response.SetID != RulesSetID ||
		response.Revision == "" || response.Record.Kind != kind || response.Record.ID != id ||
		!validRecord(response.Record) {
		return Identity{}, Record{}, incompatible("rules-data record response is invalid")
	}
	response.Record.Value = append(json.RawMessage(nil), response.Record.Value...)
	return identityOf(call, response.Revision), response.Record, nil
}

func (client *Client) Query(
	ctx context.Context,
	meta *workerrpc.Meta,
	query Query,
) (QueryResult, error) {
	if !validKind(query.Kind) || query.Limit < 1 || query.Limit > 200 || len(query.Cursor) > 32 {
		return QueryResult{}, invalid("rules-data query is invalid")
	}
	params := map[string]any{"setId": RulesSetID, "kind": query.Kind, "limit": query.Limit}
	if query.Cursor != "" {
		params["cursor"] = query.Cursor
	}
	call, err := client.call(ctx, meta, "query", params)
	if err != nil {
		return QueryResult{}, err
	}
	var response struct {
		ContractVersion string   `json:"contractVersion"`
		SetID           string   `json:"setId"`
		Revision        string   `json:"revision"`
		Records         []Record `json:"records"`
		NextCursor      string   `json:"nextCursor,omitempty"`
	}
	if err := decodeExact(call.Result, &response); err != nil ||
		response.ContractVersion != "content-query-result.v1" || response.SetID != RulesSetID ||
		response.Revision == "" || len(response.Records) > query.Limit || len(response.NextCursor) > 32 {
		return QueryResult{}, incompatible("rules-data query response is invalid")
	}
	for index := range response.Records {
		if response.Records[index].Kind != query.Kind || !validRecord(response.Records[index]) {
			return QueryResult{}, incompatible("rules-data query record is invalid")
		}
		response.Records[index].Value = append(json.RawMessage(nil), response.Records[index].Value...)
	}
	return QueryResult{
		Identity: identityOf(call, response.Revision),
		Records:  append([]Record(nil), response.Records...), NextCursor: response.NextCursor,
	}, nil
}

func (client *Client) Ruleset(
	ctx context.Context,
	meta *workerrpc.Meta,
) (RulesetResult, error) {
	cursor := ""
	seen := make(map[string]struct{})
	var selected *Record
	var identity Identity
	for {
		page, err := client.Query(ctx, meta, Query{Kind: "ruleset", Cursor: cursor, Limit: 20})
		if err != nil {
			return RulesetResult{}, err
		}
		if identity.ProviderGeneration != "" && identity != page.Identity {
			return RulesetResult{}, incompatible("rules-data identity changed during ruleset query")
		}
		identity = page.Identity
		for index := range page.Records {
			if selected != nil {
				return RulesetResult{}, incompatible("rules-data provider publishes more than one ruleset")
			}
			record := page.Records[index]
			selected = &record
		}
		if page.NextCursor == "" {
			break
		}
		if _, duplicate := seen[page.NextCursor]; duplicate {
			return RulesetResult{}, incompatible("rules-data query repeated a cursor")
		}
		seen[page.NextCursor] = struct{}{}
		cursor = page.NextCursor
	}
	if selected == nil {
		return RulesetResult{}, incompatible("rules-data provider publishes no ruleset")
	}
	ruleset, err := rules.DecodeRuleset(selected.Value)
	if err != nil {
		return RulesetResult{}, incompatible(err.Error())
	}
	identity.RulesetID = ruleset.StableID()
	identity.RulesetVersion = ruleset.RulesetVersion
	identity.Edition = ruleset.Edition
	return RulesetResult{Identity: identity, Ruleset: ruleset}, nil
}

func (client *Client) call(
	ctx context.Context,
	meta *workerrpc.Meta,
	method string,
	params map[string]any,
) (workerrpc.ServiceResult, error) {
	if client == nil || client.services == nil {
		return workerrpc.ServiceResult{}, errors.New("rules-data client is unavailable")
	}
	return client.services.Call(ctx, meta, workerrpc.ServiceCall{
		Contract: RulesDataContract, Method: method, Params: params,
	})
}

func identityOf(call workerrpc.ServiceResult, revision string) Identity {
	return Identity{
		ProviderAddonID: call.ProviderAddonID, ProviderContractVersion: call.ProviderContractVersion,
		ProviderGeneration: call.ProviderGeneration, ContentRevision: revision,
	}
}

func validRecord(record Record) bool {
	if !validReference(record.Kind, record.ID) || len(record.Value) == 0 || !json.Valid(record.Value) {
		return false
	}
	var identity struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	return json.Unmarshal(record.Value, &identity) == nil && identity.Kind == record.Kind &&
		identity.ID == record.ID && strings.TrimSpace(identity.Name) != ""
}

func validReference(kind, id string) bool {
	if !validKind(kind) || len(id) == 0 || len(id) > 200 || strings.TrimSpace(id) != id {
		return false
	}
	for _, character := range id {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validKind(kind string) bool {
	if len(kind) < 1 || len(kind) > 100 {
		return false
	}
	for index, character := range kind {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			index > 0 && (character == '.' || character == '_' || character == '-') {
			continue
		}
		return false
	}
	return true
}

func invalid(message string) error {
	return workerrpc.NewRPCError(workerrpc.JSONRPCInvalidParams, workerrpc.KindInvalidRequest,
		message, false, nil)
}

func incompatible(message string) error {
	return workerrpc.NewRPCError(workerrpc.JSONRPCApplication, workerrpc.KindValidationFailed,
		message, false, nil)
}

func boundedMessage(err error) string {
	message := "Rules data is unavailable."
	var failure *workerrpc.RPCError
	if errors.As(err, &failure) && failure.Data != nil && failure.Data.Message != "" {
		message = failure.Data.Message
	}
	if len(message) > 300 {
		message = message[:300]
	}
	return message
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
