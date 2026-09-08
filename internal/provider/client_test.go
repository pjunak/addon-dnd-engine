package provider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pjunak/ttrpg-codex/sdk/go/workerrpc"
)

const testGeneration = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestRulesDataClientUsesV3ContentService(t *testing.T) {
	t.Parallel()
	caller := &rulesDataCaller{t: t}
	client, err := New(caller)
	if err != nil {
		t.Fatal(err)
	}
	meta := &workerrpc.Meta{Generation: testGeneration}
	current := client.Inspect(context.Background(), meta)
	if !current.Available || current.Status != "ready" ||
		current.Identity.ProviderAddonID != "synthetic-provider" ||
		current.Identity.RulesetID != "synthetic-dnd-2024" ||
		current.Identity.ContentRevision != "fixture-1" {
		t.Fatalf("context = %+v", current)
	}
	identity, record, err := client.Get(context.Background(), meta, "class", "wizard")
	if err != nil || identity.ContentRevision != "fixture-1" || record.ID != "wizard" {
		t.Fatalf("get = %+v %+v %v", identity, record, err)
	}
	page, err := client.Query(context.Background(), meta, Query{Kind: "class", Limit: 10})
	if err != nil || len(page.Records) != 1 || page.Records[0].ID != "wizard" {
		t.Fatalf("query = %+v %v", page, err)
	}
	if caller.calls < 3 {
		t.Fatalf("host service calls = %d", caller.calls)
	}
}

func TestRulesDataContextKeepsOptionalProviderFailureExplicit(t *testing.T) {
	t.Parallel()
	client, err := New(serviceCallerFunc(func(
		context.Context, string, any, *workerrpc.Meta,
	) (json.RawMessage, error) {
		return nil, workerrpc.NewRPCError(workerrpc.JSONRPCApplication,
			workerrpc.KindUnauthorized, "no bound provider", false, nil)
	}))
	if err != nil {
		t.Fatal(err)
	}
	current := client.Inspect(context.Background(), nil)
	if current.Available || current.Status != "missing" || len(current.Errors) != 1 {
		t.Fatalf("context = %+v", current)
	}
}

func TestRulesDataClientRejectsMismatchedRecordIdentity(t *testing.T) {
	t.Parallel()
	client, err := New(serviceCallerFunc(func(
		context.Context, string, any, *workerrpc.Meta,
	) (json.RawMessage, error) {
		return serviceEnvelope(`{
			"contractVersion":"content-record.v1","setId":"rules","revision":"fixture-1",
			"record":{"kind":"class","id":"wizard","value":{"kind":"class","id":"fighter","name":"Fighter"}}
		}`), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.Get(context.Background(), nil, "class", "wizard")
	var failure *workerrpc.RPCError
	if !errors.As(err, &failure) || failure.Data == nil || failure.Data.Kind != workerrpc.KindValidationFailed {
		t.Fatalf("error = %T %v", err, err)
	}
}

func TestRepositoryLoadsOneConsistentSnapshotAndReusesIt(t *testing.T) {
	t.Parallel()
	caller := &rulesDataCaller{t: t, methodCalls: make(map[string]int)}
	client, err := New(caller)
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.Repository(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Repository(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("unchanged provider content was loaded twice")
	}
	record, exists := first.GetByName("class", "WIZARD")
	if !exists || record.ID != "wizard" {
		t.Fatalf("record = %+v, exists = %v", record, exists)
	}
	record.Value[0] = '['
	again, _ := first.Get("class", "wizard")
	if again.Value[0] != '{' {
		t.Fatal("repository returned mutable record storage")
	}
	if caller.methodCalls["catalog"] != 2 || caller.methodCalls["query:ruleset"] != 1 ||
		caller.methodCalls["query:class"] != 1 {
		t.Fatalf("method calls = %v", caller.methodCalls)
	}
}

func TestRepositoryKeepsDuplicateNamesAddressableByID(t *testing.T) {
	t.Parallel()
	caller := &snapshotCaller{base: rulesDataCaller{t: t}, revision: "fixture-1", generation: testGeneration,
		records: []Record{
			{Kind: "class", ID: "first", Value: json.RawMessage(`{"id":"first","kind":"class","name":"Shared"}`)},
			{Kind: "class", ID: "second", Value: json.RawMessage(`{"id":"second","kind":"class","name":" SHARED "}`)},
			{Kind: "class", ID: "third", Value: json.RawMessage(`{"id":"third","kind":"class","name":"Shared"}`)},
			{Kind: "class", ID: "unique", Value: json.RawMessage(`{"id":"unique","kind":"class","name":"Unique"}`)},
		}}
	client, _ := New(caller)
	repository, err := client.Repository(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range caller.records {
		if loaded, ok := repository.Get("class", record.ID); !ok || loaded.ID != record.ID {
			t.Fatalf("ID lookup %s = %+v, %v", record.ID, loaded, ok)
		}
	}
	if _, ok := repository.GetByName("class", "shared"); ok {
		t.Fatal("ambiguous name selected an arbitrary record")
	}
	if record, ok := repository.GetByName("class", " UNIQUE "); !ok || record.ID != "unique" {
		t.Fatal("unique name no longer resolves")
	}
}

func TestRepositoryRefreshesChangedRevisionAndGenerationWithoutServingMissingProvider(t *testing.T) {
	t.Parallel()
	caller := &snapshotCaller{base: rulesDataCaller{t: t}, revision: "fixture-1", generation: testGeneration}
	client, _ := New(caller)
	first, err := client.Repository(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	caller.missing = true
	if stale, err := client.Repository(context.Background(), nil); err == nil || stale != nil {
		t.Fatal("cached snapshot hid provider loss")
	}
	caller.missing = false
	caller.revision = "fixture-2"
	caller.records = []Record{{Kind: "class", ID: "wizard", Value: json.RawMessage(`{"id":"wizard","kind":"class","name":"Revised Wizard"}`)}}
	second, err := client.Repository(context.Background(), nil)
	if err != nil || first == second || second.Identity.ContentRevision != "fixture-2" {
		t.Fatalf("changed revision = %+v, %v", second, err)
	}
	if record, ok := second.GetByName("class", "Revised Wizard"); !ok || record.ID != "wizard" {
		t.Fatal("changed content retained stale name index")
	}
	caller.generation = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	third, err := client.Repository(context.Background(), nil)
	if err != nil || second == third || third.Identity.ProviderGeneration != caller.generation {
		t.Fatalf("changed generation = %+v, %v", third, err)
	}
	if _, ok := first.GetByName("class", "Wizard"); !ok {
		t.Fatal("loading a replacement mutated the previous snapshot")
	}
}

type snapshotCaller struct {
	base                 rulesDataCaller
	records              []Record
	revision, generation string
	missing              bool
}

func (caller *snapshotCaller) Call(ctx context.Context, method string, params any, meta *workerrpc.Meta) (json.RawMessage, error) {
	if caller.missing {
		return nil, workerrpc.NewRPCError(workerrpc.JSONRPCApplication, workerrpc.KindUnauthorized, "no bound provider", false, nil)
	}
	raw, err := caller.base.Call(ctx, method, params, meta)
	if err != nil {
		return nil, err
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	envelope["providerGeneration"] = caller.generation
	result := envelope["result"].(map[string]any)
	if sets, ok := result["sets"].([]any); ok {
		catalog := sets[0].(map[string]any)
		catalog["revision"] = caller.revision
		if caller.records != nil {
			catalog["kinds"].(map[string]any)["class"] = len(caller.records)
			catalog["recordCount"] = len(caller.records) + 1
		}
	} else {
		result["revision"] = caller.revision
		if records, ok := result["records"].([]any); ok && caller.records != nil && records[0].(map[string]any)["kind"] == "class" {
			result["records"] = caller.records
		}
	}
	return json.Marshal(envelope)
}

type rulesDataCaller struct {
	t           *testing.T
	calls       int
	methodCalls map[string]int
}

func (caller *rulesDataCaller) Call(
	_ context.Context,
	method string,
	params any,
	_ *workerrpc.Meta,
) (json.RawMessage, error) {
	caller.calls++
	if method != "host/service.call" {
		caller.t.Fatalf("method = %s", method)
	}
	body, err := json.Marshal(params)
	if err != nil {
		caller.t.Fatal(err)
	}
	var request struct {
		Contract string `json:"contract"`
		Method   string `json:"method"`
		Params   struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &request); err != nil || request.Contract != RulesDataContract {
		caller.t.Fatalf("request = %s, %v", body, err)
	}
	if caller.methodCalls == nil {
		caller.methodCalls = make(map[string]int)
	}
	switch request.Method {
	case "catalog":
		caller.methodCalls["catalog"]++
		return serviceEnvelope(`{
			"contractVersion":"content-catalog.v1","sets":[{
				"id":"rules","revision":"fixture-1","schemaSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"recordCount":2,"kinds":{"class":1,"ruleset":1}
			}]
		}`), nil
	case "get":
		caller.methodCalls["get"]++
		return serviceEnvelope(`{
			"contractVersion":"content-record.v1","setId":"rules","revision":"fixture-1",
			"record":{"kind":"class","id":"wizard","value":{"kind":"class","id":"wizard","name":"Wizard"}}
		}`), nil
	case "query":
		caller.methodCalls["query:"+request.Params.Kind]++
		if request.Params.Kind == "ruleset" {
			ruleset, err := os.ReadFile(filepath.Join("..", "..", "testdata", "synthetic-ruleset.json"))
			if err != nil {
				caller.t.Fatal(err)
			}
			result, _ := json.Marshal(map[string]any{
				"contractVersion": "content-query-result.v1", "setId": "rules", "revision": "fixture-1",
				"records": []any{map[string]any{
					"kind": "ruleset", "id": "synthetic-dnd-2024", "value": json.RawMessage(ruleset),
				}},
			})
			return serviceEnvelope(string(result)), nil
		}
		return serviceEnvelope(`{
			"contractVersion":"content-query-result.v1","setId":"rules","revision":"fixture-1",
			"records":[{"kind":"class","id":"wizard","value":{"kind":"class","id":"wizard","name":"Wizard"}}]
		}`), nil
	default:
		caller.t.Fatalf("rules-data method = %s", request.Method)
		return nil, nil
	}
}

func serviceEnvelope(result string) json.RawMessage {
	body, _ := json.Marshal(map[string]any{
		"contractVersion": "host-service-result.v1",
		"contract":        RulesDataContract, "providerAddonId": "synthetic-provider",
		"providerContractVersion": RulesDataVersion, "providerGeneration": testGeneration,
		"result": json.RawMessage(result),
	})
	return body
}

type serviceCallerFunc func(context.Context, string, any, *workerrpc.Meta) (json.RawMessage, error)

func (caller serviceCallerFunc) Call(
	ctx context.Context,
	method string,
	params any,
	meta *workerrpc.Meta,
) (json.RawMessage, error) {
	return caller(ctx, method, params, meta)
}
