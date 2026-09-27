package provider

import (
	"context"
	"encoding/json"
	"testing"
)

func TestConditionReferenceLoadsOnlyItsExactRecordAndRetainsSourceIdentity(t *testing.T) {
	sources := sourceFixtures(t)
	var profile map[string]any
	if err := json.Unmarshal(sources["foundation"].records[0].Value, &profile); err != nil {
		t.Fatal(err)
	}
	profile["constants"].(map[string]any)["character"] = map[string]any{"conditionRules": "table-states", "uniqueAttunement": true, "maximumLevel": 20, "minimumHpGain": 1, "standardArray": []int{15, 14, 13, 12, 10, 8}, "rollDice": 4, "rollSides": 6, "rollKeep": 3, "distanceUnit": "ft"}
	sources["foundation"].records[0].Value, _ = json.Marshal(profile)
	reference := Record{Kind: "rule", ID: "table-states", Value: json.RawMessage(`{"kind":"rule","id":"table-states","name":"States","conditionDefinitions":[]}`)}
	sources["extra-books"].records = append(sources["extra-books"].records, reference, Record{Kind: "rule", ID: "unrelated", Value: json.RawMessage(`{"kind":"rule","id":"unrelated","name":"Reference prose"}`)})
	client, _ := New(sources, "foundation", "extra-books")
	repository, err := client.Repository(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	stored, found := repository.Get("rule", "table-states")
	if !found || stored.SourceIdentity.ProviderAddonID != "extra-books" || stored.SourceIdentity.ContentRevision != "extra-1" {
		t.Fatal(stored)
	}
	if _, found := repository.Get("rule", "unrelated"); found {
		t.Fatal("unrelated rule was loaded")
	}
	sources["extra-books"].records = sources["extra-books"].records[:1]
	sources["extra-books"].revision = "withdrawn"
	repository, err = client.Repository(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := repository.Get("rule", "table-states"); found {
		t.Fatal("withdrawn source survived snapshot replacement")
	}
	sources["extra-books"].records = append(sources["extra-books"].records, reference)
	sources["extra-books"].revision = "restored"
	sources["foundation"].records = append(sources["foundation"].records, reference)
	sources["foundation"].revision = "duplicate"
	if _, err = client.Repository(context.Background(), nil); err == nil {
		t.Fatal("ambiguous condition source accepted")
	}
}
