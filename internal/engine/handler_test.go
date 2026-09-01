package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

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
	if !current.Available || current.Identity.ProviderAddonID != "synthetic-provider" {
		t.Fatalf("context = %+v", current)
	}

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
}

func TestUniversalDeriveDoesNotRequireRulesData(t *testing.T) {
	t.Parallel()
	data := engineProvider{rulesetError: errors.New("provider must not be called")}
	handler, _ := New(&data)

	value, err := handler.HandleRPC(context.Background(), rpcRequest("derive", `{
		"contractVersion":"rules-engine-derive.v1","operation":"ability-modifier","input":{"score":16}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	derived := value.(deriveResponse)
	if derived.Value != 3 || derived.Identity != nil || data.rulesetCalls != 0 {
		t.Fatalf("derived = %+v, ruleset calls = %d", derived, data.rulesetCalls)
	}
}

func TestRulesetDeriveCarriesProviderIdentity(t *testing.T) {
	t.Parallel()
	data := engineProvider{ruleset: engineRuleset(t)}
	handler, _ := New(&data)

	value, err := handler.HandleRPC(context.Background(), rpcRequest("derive", `{
		"contractVersion":"rules-engine-derive.v1","operation":"point-buy-cost","input":{"score":15}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	derived := value.(deriveResponse)
	if derived.Value != 9 || derived.Identity == nil ||
		derived.Identity.ContentRevision != "fixture-1" || data.rulesetCalls != 1 {
		t.Fatalf("derived = %+v, ruleset calls = %d", derived, data.rulesetCalls)
	}
}

func TestHandlerRejectsMissingInputsAndUnknownMethods(t *testing.T) {
	t.Parallel()
	data := engineProvider{ruleset: engineRuleset(t)}
	handler, _ := New(&data)

	_, err := handler.HandleRPC(context.Background(), rpcRequest("derive", `{
		"contractVersion":"rules-engine-derive.v1","operation":"ability-modifier","input":{}
	}`))
	assertRPCError(t, err, workerrpc.KindInvalidRequest)
	_, err = handler.HandleRPC(context.Background(), rpcRequest("unknown", `{}`))
	assertRPCError(t, err, workerrpc.KindNotFound)
	if data.rulesetCalls != 0 {
		t.Fatalf("invalid local requests called rules data %d times", data.rulesetCalls)
	}
}

func TestHandlerHydratesWithExactProviderIdentity(t *testing.T) {
	t.Parallel()
	data := engineProvider{ruleset: engineRuleset(t)}
	handler, _ := New(&data)
	value, err := handler.HandleRPC(context.Background(), rpcRequest("hydrate", `{
		"contractVersion":"rules-engine-hydrate.v1",
		"decisions":{"abilities":{"STR":16,"DEX":14},"level":5}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	hydrated := value.(hydrateResponse)
	if hydrated.Identity == nil || hydrated.Identity.ContentRevision != "fixture-1" ||
		len(hydrated.Warnings) != 0 || hydrated.Sheet["totalLevel"] != 5 {
		t.Fatalf("hydrated = %+v", hydrated)
	}
}

func TestHandlerHydrationDegradesWhenOptionalProviderIsMissing(t *testing.T) {
	t.Parallel()
	data := engineProvider{rulesetError: workerrpc.NewRPCError(
		workerrpc.JSONRPCApplication, workerrpc.KindUnauthorized, "no provider", false, nil,
	)}
	handler, _ := New(&data)
	value, err := handler.HandleRPC(context.Background(), rpcRequest("hydrate", `{
		"contractVersion":"rules-engine-hydrate.v1","decisions":{"abilities":{"DEX":14},"level":5}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	hydrated := value.(hydrateResponse)
	if hydrated.Identity != nil || len(hydrated.Warnings) != 1 ||
		objectNumber(hydrated.Sheet, "derived", "initiative") != 2 {
		t.Fatalf("hydrated = %+v", hydrated)
	}
}

func TestHandlerExposesBuilderLifecycle(t *testing.T) {
	t.Parallel()
	data := engineProvider{ruleset: engineRuleset(t)}
	handler, _ := New(&data)
	value, err := handler.HandleRPC(context.Background(), rpcRequest("builder-plan", `{
		"contractVersion":"rules-engine-builder-plan.v1","decisions":{}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	plan := value.(builderPlanResponse)
	if !plan.Available || plan.Status != "ready" || plan.Identity == nil || plan.Plan == nil {
		t.Fatalf("plan = %+v", plan)
	}

	value, err = handler.HandleRPC(context.Background(), rpcRequest("reconcile-builder-decisions", `{
		"contractVersion":"rules-engine-builder-reconcile.v1","decisions":{"featureChoices":{"stale":"x"}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	reconciled := value.(builderDecisionsResponse)
	if !reconciled.Available || len(reconciled.Decisions) == 0 {
		t.Fatalf("reconciled = %+v", reconciled)
	}
}

func TestBuilderMutationIsNoOpWhenOptionalProviderIsMissing(t *testing.T) {
	t.Parallel()
	data := engineProvider{rulesetError: workerrpc.NewRPCError(
		workerrpc.JSONRPCApplication, workerrpc.KindUnauthorized, "no provider", false, nil,
	)}
	handler, _ := New(&data)
	value, err := handler.HandleRPC(context.Background(), rpcRequest("apply-builder-choice", `{
		"contractVersion":"rules-engine-builder-change.v1",
		"decisions":{"marker":"preserved"},"change":{"choiceId":"unknown","value":"x"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(builderDecisionsResponse)
	if result.Available || result.Status != "missing" || result.Decisions["marker"] != "preserved" ||
		result.Identity != nil {
		t.Fatalf("result = %+v", result)
	}
}

type engineProvider struct {
	ruleset      rules.Ruleset
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
	return engineIdentity(), nil, data.ruleset, nil
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

func objectNumber(source rules.Object, path ...string) int {
	var current any = source
	for _, key := range path {
		value, _ := current.(rules.Object)
		if value == nil {
			if raw, ok := current.(map[string]any); ok {
				value = rules.Object(raw)
			}
		}
		current = value[key]
	}
	switch value := current.(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}
