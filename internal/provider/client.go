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
	"sort"
	"strings"
	"sync"

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
	cacheMu  sync.Mutex
	cached   *Repository
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

type CatalogResult struct {
	Identity    Identity
	RecordCount int
	Kinds       map[string]int
}

type Repository struct {
	Identity Identity
	Ruleset  rules.Ruleset
	records  map[string]map[string]Record
	names    map[string]map[string]string
}

var engineKinds = [...]string{
	"armor", "background", "class", "feat", "feature", "skill", "species",
	"spell", "subclass", "tool", "weapon",
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
) (CatalogResult, error) {
	call, err := client.call(ctx, meta, "catalog", map[string]any{})
	if err != nil {
		return CatalogResult{}, err
	}
	var response struct {
		ContractVersion string `json:"contractVersion"`
		Sets            []struct {
			ID           string          `json:"id"`
			Revision     string          `json:"revision"`
			SchemaSHA256 string          `json:"schemaSha256"`
			RecordCount  int             `json:"recordCount"`
			Kinds        map[string]int  `json:"kinds"`
			Groups       json.RawMessage `json:"groups,omitempty"`
		} `json:"sets"`
	}
	if err := decodeExact(call.Result, &response); err != nil ||
		response.ContractVersion != "content-catalog.v1" {
		return CatalogResult{}, incompatible("rules-data catalog is invalid")
	}
	for _, set := range response.Sets {
		if set.ID == RulesSetID && set.Revision != "" && validDigest(set.SchemaSHA256) &&
			validCatalogCounts(set.RecordCount, set.Kinds) {
			identity := identityOf(call, set.Revision)
			return CatalogResult{
				Identity: identity, RecordCount: set.RecordCount, Kinds: cloneCounts(set.Kinds),
			}, nil
		}
	}
	return CatalogResult{}, incompatible("rules-data catalog has no rules content set")
}

func (client *Client) Repository(
	ctx context.Context,
	meta *workerrpc.Meta,
) (*Repository, error) {
	if client == nil {
		return nil, errors.New("rules-data client is unavailable")
	}
	client.cacheMu.Lock()
	defer client.cacheMu.Unlock()

	catalog, err := client.Catalog(ctx, meta)
	if err != nil {
		return nil, err
	}
	if client.cached != nil && sameProviderContent(client.cached.Identity, catalog.Identity) {
		return client.cached, nil
	}
	profile, err := client.Ruleset(ctx, meta)
	if err != nil {
		return nil, err
	}
	if !sameProviderContent(profile.Identity, catalog.Identity) {
		return nil, incompatible("rules-data identity changed while loading engine content")
	}

	repository := &Repository{
		Identity: profile.Identity,
		Ruleset:  profile.Ruleset,
		records:  make(map[string]map[string]Record),
		names:    make(map[string]map[string]string),
	}
	for _, kind := range engineKinds {
		expected := catalog.Kinds[kind]
		if expected == 0 {
			continue
		}
		loaded, err := client.loadKind(ctx, meta, catalog.Identity, kind, expected)
		if err != nil {
			return nil, err
		}
		repository.records[kind] = make(map[string]Record, len(loaded))
		repository.names[kind] = make(map[string]string, len(loaded))
		for _, record := range loaded {
			repository.records[kind][record.ID] = record
			name, err := recordName(record.Value)
			if err != nil {
				return nil, incompatible("rules-data record name is invalid")
			}
			normalized := strings.ToLower(name)
			if previous, duplicate := repository.names[kind][normalized]; duplicate && previous != record.ID {
				return nil, incompatible("rules-data record names are ambiguous")
			}
			repository.names[kind][normalized] = record.ID
		}
	}
	client.cached = repository
	return repository, nil
}

func (client *Client) loadKind(
	ctx context.Context,
	meta *workerrpc.Meta,
	identity Identity,
	kind string,
	expected int,
) ([]Record, error) {
	cursor := ""
	seenCursors := make(map[string]struct{})
	seenRecords := make(map[string]struct{}, expected)
	records := make([]Record, 0, expected)
	for {
		page, err := client.Query(ctx, meta, Query{Kind: kind, Cursor: cursor, Limit: 200})
		if err != nil {
			return nil, err
		}
		if !sameProviderContent(page.Identity, identity) {
			return nil, incompatible("rules-data identity changed while loading records")
		}
		for _, record := range page.Records {
			if _, duplicate := seenRecords[record.ID]; duplicate {
				return nil, incompatible("rules-data query returned a duplicate record")
			}
			seenRecords[record.ID] = struct{}{}
			records = append(records, record)
		}
		if page.NextCursor == "" {
			break
		}
		if _, duplicate := seenCursors[page.NextCursor]; duplicate {
			return nil, incompatible("rules-data query repeated a cursor")
		}
		seenCursors[page.NextCursor] = struct{}{}
		cursor = page.NextCursor
	}
	if len(records) != expected {
		return nil, incompatible("rules-data catalog count changed while loading records")
	}
	sort.Slice(records, func(left, right int) bool { return records[left].ID < records[right].ID })
	return records, nil
}

func (repository *Repository) Get(kind, id string) (Record, bool) {
	if repository == nil {
		return Record{}, false
	}
	record, exists := repository.records[kind][id]
	if !exists {
		return Record{}, false
	}
	return cloneRecord(record), true
}

func (repository *Repository) GetByName(kind, name string) (Record, bool) {
	if repository == nil {
		return Record{}, false
	}
	id, exists := repository.names[kind][strings.ToLower(strings.TrimSpace(name))]
	if !exists {
		return Record{}, false
	}
	return repository.Get(kind, id)
}

func (repository *Repository) List(kind string) []Record {
	if repository == nil {
		return []Record{}
	}
	records := repository.records[kind]
	ids := make([]string, 0, len(records))
	for id := range records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]Record, 0, len(ids))
	for _, id := range ids {
		result = append(result, cloneRecord(records[id]))
	}
	return result
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

func sameProviderContent(left, right Identity) bool {
	return left.ProviderAddonID == right.ProviderAddonID &&
		left.ProviderContractVersion == right.ProviderContractVersion &&
		left.ProviderGeneration == right.ProviderGeneration &&
		left.ContentRevision == right.ContentRevision
}

func cloneCounts(source map[string]int) map[string]int {
	result := make(map[string]int, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func validCatalogCounts(total int, kinds map[string]int) bool {
	if total < 1 || len(kinds) == 0 {
		return false
	}
	sum := 0
	for kind, count := range kinds {
		if !validKind(kind) || count < 1 {
			return false
		}
		sum += count
	}
	return sum == total
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func cloneRecord(record Record) Record {
	record.Value = append(json.RawMessage(nil), record.Value...)
	return record
}

func recordName(value json.RawMessage) (string, error) {
	var identity struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(value, &identity); err != nil || strings.TrimSpace(identity.Name) == "" {
		return "", errors.New("record name is missing")
	}
	return identity.Name, nil
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
