package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/pjunak/addon-dnd-engine/character"
	"github.com/pjunak/addon-dnd-engine/internal/provider"
	"github.com/pjunak/addon-dnd-engine/internal/rules"
	"github.com/pjunak/ttrpg-codex/sdk/go/workerrpc"
)

func TestHandlerExposesContextAndRecords(t *testing.T) {
	t.Parallel()
	data := engineProvider{ruleset: engineRuleset(t)}
	handler, err := New(&data)
	if err != nil {
		t.Fatal(err)
	}

	contextValue, err := handler.HandleRPC(context.Background(), rpcRequest("context", `{}`))
	if err != nil {
		t.Fatal(err)
	}
	current := contextValue.(contextResponse)
	if !current.Available || current.Identity.ProviderAddonID != "synthetic-provider" || current.Errors == nil {
		t.Fatalf("context = %+v", current)
	}
	matchesSchema(t, "context.response.schema.json", current)

	recordValue, err := handler.HandleRPC(context.Background(), rpcRequest("get-record", `{
		"contractVersion":"rules-engine-get.v1","kind":"class","id":"wizard"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	record := recordValue.(getResponse)
	if record.Record.Kind != "class" || record.Record.ID != "wizard" || data.getCalls != 1 {
		t.Fatalf("record = %+v, calls = %d", record, data.getCalls)
	}
	matchesSchema(t, "get-record.response.schema.json", record)

	queryValue, err := handler.HandleRPC(context.Background(), rpcRequest("query-records", `{
		"contractVersion":"rules-engine-query.v1","kind":"class","limit":20
	}`))
	if err != nil {
		t.Fatal(err)
	}
	page := queryValue.(queryResponse)
	if len(page.Records) != 1 || page.Records[0].ID != "wizard" || data.queryCalls != 1 {
		t.Fatalf("query = %+v, calls = %d", page, data.queryCalls)
	}
	matchesSchema(t, "query-records.response.schema.json", page)
}

// matchesSchema validates a response against its packaged schema, as the host
// does before a consumer sees it.
func matchesSchema(t *testing.T, name string, response any) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	files, err := filepath.Glob(filepath.Join("..", "..", "contracts", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("contracts: %v", err)
	}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if err := compiler.AddResource("file:///addon/contracts/"+filepath.Base(file), document); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("file:///addon/contracts/" + name)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Errorf("response does not match contracts/%s: %v", name, err)
	}
}

func TestHandlerRejectsMissingInputsAndUnknownMethods(t *testing.T) {
	t.Parallel()
	data := engineProvider{ruleset: engineRuleset(t)}
	handler, _ := New(&data)

	_, err := handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", `{"contractVersion":"rules-character.v2","inputs":{}}`))
	assertRPCError(t, err, workerrpc.KindInvalidRequest)
	_, err = handler.HandleRPC(context.Background(), rpcRequest("character-play", `{"contractVersion":"rules-character.v3","inputs":`+blankInputs(t)+`,"change":{"operation":"rest","rest":"week"}}`))
	var failure *workerrpc.RPCError
	if !errors.As(err, &failure) || failure.Message != "Choose a short or long rest." {
		t.Fatalf("malformed play command = %v", err)
	}
	_, err = handler.HandleRPC(context.Background(), rpcRequest("derive", `{}`))
	assertRPCError(t, err, workerrpc.KindNotFound)
	_, err = handler.HandleRPC(context.Background(), rpcRequest("unknown", `{}`))
	assertRPCError(t, err, workerrpc.KindNotFound)
	if data.rulesetCalls != 0 {
		t.Fatalf("invalid local requests called rules data %d times", data.rulesetCalls)
	}
}

func blankInputs(t *testing.T) string {
	t.Helper()
	body, err := json.Marshal(character.Blank())
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

type engineRecords []rules.Object

func (records engineRecords) lookup(kind, field, value string) (json.RawMessage, bool) {
	for _, record := range records {
		if record["kind"] == kind && record[field] == value {
			body, _ := json.Marshal(record)
			return body, true
		}
	}
	return nil, false
}

func (records engineRecords) Value(kind, id string) (json.RawMessage, bool) {
	return records.lookup(kind, "id", id)
}

func (records engineRecords) Values(kind string) []json.RawMessage {
	result := []json.RawMessage{}
	for _, record := range records {
		if record["kind"] == kind {
			body, _ := json.Marshal(record)
			result = append(result, body)
		}
	}
	return result
}

type engineProvider struct {
	ruleset      rules.Ruleset
	records      rules.Records
	rulesetError error
	getCalls     int
	queryCalls   int
	rulesetCalls int
}

func (data *engineProvider) Inspect(context.Context, *workerrpc.Meta) provider.Context {
	return provider.Context{Available: true, Status: "ready", Identity: engineIdentity(), Errors: []string{}}
}

func (data *engineProvider) Get(
	context.Context, *workerrpc.Meta, string, string,
) (provider.Identity, provider.Record, error) {
	data.getCalls++
	return engineIdentity(), provider.Record{
		Kind: "class", ID: "wizard",
		Value: json.RawMessage(`{"kind":"class","id":"wizard","name":"Wizard"}`),
	}, nil
}

func (data *engineProvider) Query(
	context.Context, *workerrpc.Meta, provider.Query,
) (provider.QueryResult, error) {
	data.queryCalls++
	return provider.QueryResult{Identity: engineIdentity(), Records: []provider.Record{{
		Kind: "class", ID: "wizard",
		Value: json.RawMessage(`{"kind":"class","id":"wizard","name":"Wizard"}`),
	}}}, nil
}

func (data *engineProvider) Ruleset(
	context.Context, *workerrpc.Meta,
) (provider.RulesetResult, error) {
	data.rulesetCalls++
	if data.rulesetError != nil {
		return provider.RulesetResult{}, data.rulesetError
	}
	return provider.RulesetResult{Identity: engineIdentity(), Ruleset: data.ruleset}, nil
}

func (data *engineProvider) Evaluation(
	context.Context, *workerrpc.Meta,
) (provider.Identity, rules.Records, rules.Ruleset, error) {
	data.rulesetCalls++
	if data.rulesetError != nil {
		return provider.Identity{}, nil, rules.Ruleset{}, data.rulesetError
	}
	return engineIdentity(), data.records, data.ruleset, nil
}

func rpcRequest(method, params string) workerrpc.Request {
	return workerrpc.Request{Method: methodPrefix + method, Params: json.RawMessage(params)}
}

func engineIdentity() provider.Identity {
	return provider.Identity{
		ProviderAddonID: "synthetic-provider", ProviderContractVersion: provider.RulesDataVersion,
		ProviderGeneration: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ContentRevision:    "fixture-1", RulesetID: "synthetic-dnd-2024", RulesetVersion: 2, Edition: "2024",
	}
}

func engineRuleset(t *testing.T) rules.Ruleset {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "synthetic-ruleset.json"))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := rules.DecodeRuleset(body)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func assertRPCError(t *testing.T, err error, kind string) {
	t.Helper()
	var failure *workerrpc.RPCError
	if !errors.As(err, &failure) || failure.Data == nil || failure.Data.Kind != kind {
		t.Fatalf("error = %T %v", err, err)
	}
}
