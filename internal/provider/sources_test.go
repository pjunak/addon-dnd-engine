package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pjunak/ttrpg-codex/sdk/go/workerrpc"
)

type sourceFixture struct {
	revision string
	records  []Record
}
type sourcesCaller map[string]*sourceFixture

func (sources sourcesCaller) Call(_ context.Context, method string, params any, _ *workerrpc.Meta) (json.RawMessage, error) {
	encoded, _ := json.Marshal(params)
	var request struct {
		Provider string `json:"providerAddonId"`
		Method   string `json:"method"`
		Params   struct {
			Kind string `json:"kind"`
		} `json:"params"`
	}
	if err := json.Unmarshal(encoded, &request); err != nil {
		return nil, err
	}
	source := sources[request.Provider]
	if source == nil || method != "host/service.call" {
		return nil, incompatible("requested provider is not bound")
	}
	var result any
	if request.Method == "catalog" {
		kinds := map[string]int{}
		for _, record := range source.records {
			kinds[record.Kind]++
		}
		result = map[string]any{"contractVersion": "content-catalog.v1", "sets": []any{map[string]any{"id": "rules", "revision": source.revision, "recordCount": len(source.records), "kinds": kinds, "schemaSha256": strings.Repeat("a", 64)}}}
	} else {
		records := []Record{}
		for _, record := range source.records {
			if record.Kind == request.Params.Kind {
				records = append(records, record)
			}
		}
		result = map[string]any{"contractVersion": "content-query-result.v1", "setId": "rules", "revision": source.revision, "records": records}
	}
	return json.Marshal(map[string]any{"contractVersion": "host-service-result.v1", "contract": RulesDataContract, "providerAddonId": request.Provider, "providerContractVersion": RulesDataVersion, "providerGeneration": testGeneration, "result": result})
}

func sourceFixtures(t *testing.T) sourcesCaller {
	t.Helper()
	profile, err := os.ReadFile(filepath.Join("..", "..", "testdata", "synthetic-ruleset.json"))
	if err != nil {
		t.Fatal(err)
	}
	return sourcesCaller{
		"foundation":  {revision: "base-1", records: []Record{{Kind: "ruleset", ID: "synthetic-dnd-2024", Value: profile}, {Kind: "class", ID: "wizard", Value: json.RawMessage(`{"kind":"class","id":"wizard","name":"Wizard"}`)}}},
		"extra-books": {revision: "extra-1", records: []Record{{Kind: "class", ID: "fighter", Value: json.RawMessage(`{"kind":"class","id":"fighter","name":"Fighter"}`)}}},
	}
}

func TestCombinedSourcesPreserveProvenanceAndInvalidateSnapshotsAndCursors(t *testing.T) {
	ctx := context.Background()
	sources := sourceFixtures(t)
	client, err := New(sources, "foundation", "extra-books")
	if err != nil {
		t.Fatal(err)
	}
	current := client.Inspect(ctx, nil)
	if !current.Available || current.Identity.ProviderAddonID != "foundation" {
		t.Fatalf("context=%+v", current)
	}
	first, err := client.Query(ctx, nil, Query{Kind: "class", Limit: 1})
	if err != nil || len(first.Records) != 1 || first.Records[0].ID != "fighter" || first.Records[0].ProviderAddonID != "extra-books" || len(first.NextCursor) > 32 {
		t.Fatalf("first=%+v,%v", first, err)
	}
	second, err := client.Query(ctx, nil, Query{Kind: "class", Limit: 1, Cursor: first.NextCursor})
	if err != nil || second.Records[0].ID != "wizard" || second.NextCursor != "" {
		t.Fatalf("second=%+v,%v", second, err)
	}
	repository, err := client.Repository(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := repository.Get("class", "fighter"); !found {
		t.Fatal("additional source missing from computation snapshot")
	}
	sources["extra-books"].records = []Record{}
	sources["extra-books"].revision = "extra-2"
	if _, err := client.Query(ctx, nil, Query{Kind: "class", Limit: 1, Cursor: first.NextCursor}); err == nil {
		t.Fatal("source change accepted an old cursor")
	}
	updated, err := client.Repository(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Identity.ContentRevision == repository.Identity.ContentRevision {
		t.Fatal("source change retained old identity")
	}
	if _, found := updated.Get("class", "fighter"); found {
		t.Fatal("disabled record survived in current snapshot")
	}
	if _, found := repository.Get("class", "fighter"); !found {
		t.Fatal("an already returned snapshot was mutated")
	}
	delete(sources, "extra-books")
	if _, err := client.Repository(ctx, nil); err == nil {
		t.Fatal("cached snapshot concealed unavailable provider")
	}
}

func TestCombinedSourcesRejectDuplicateRecordsAndProfiles(t *testing.T) {
	for _, kind := range []string{"class", "ruleset"} {
		t.Run(kind, func(t *testing.T) {
			sources := sourceFixtures(t)
			for _, record := range sources["foundation"].records {
				if record.Kind == kind {
					sources["extra-books"].records = []Record{record}
				}
			}
			client, err := New(sources, "extra-books", "foundation")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Repository(context.Background(), nil); err == nil {
				t.Fatal("conflicting sources were combined")
			}
		})
	}
}
