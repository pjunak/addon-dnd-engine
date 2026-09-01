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
