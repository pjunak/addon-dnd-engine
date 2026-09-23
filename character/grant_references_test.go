package character

import (
	"encoding/json"
	"net/url"
	"reflect"
	"testing"
)

func TestRemapGrantReferencesPreservesNestedOwnersAndAuthoredValues(t *testing.T) {
	input := Blank()
	old := "feat:practice@grant%3Aexternal:skill"
	current := "feat:practice@grant%3Aapproved:skill"
	nested := "feat:gift@" + url.QueryEscape(old) + ":spell"
	nestedNext := "feat:gift@" + url.QueryEscape(current) + ":spell"
	input.Build.Choices = []Choice{{ID: old, Slot: 0, Value: json.RawMessage(`"grant:external"`)}, {ID: old, Slot: 1, Value: json.RawMessage(`"other"`)}}
	input.Build.Spells.GrantChoices[nested] = []string{"spark"}
	input.Build.Spells.CastingAbilities[nested] = "INT"
	input.Play.ResourceUses["charge:"+nested] = 1
	input.Play.ResourceUses["charge:feat:gift@grant%3Aexternal-long:spell"] = 2
	input.Play.ActiveFeatures[nested] = true
	input.Play.Rolls = []PlayRoll{{ID: "roll", Resource: nested, Result: 3}}
	input.Play.Inventory = []Item{{ID: "item", GrantID: "external", Notes: old}, {ID: "unresolved", GrantID: "missing"}}
	input.Grants = []Grant{{ID: "external", Name: old, ActorID: "original actor", Effects: []Effect{{Target: "resourceMax", Key: "charge:" + nested, Value: 2}}, Waivers: []string{"required-choice:" + old}}}
	input.Notes = nested
	before, _ := json.Marshal(input)
	result, err := RemapGrantReferences(input, map[string]string{"external": "approved"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Build.Choices[0].ID != current || result.Build.Choices[1].ID != current || string(result.Build.Choices[0].Value) != `"grant:external"` {
		t.Fatal(result.Build.Choices)
	}
	if !reflect.DeepEqual(result.Build.Spells.GrantChoices[nestedNext], []string{"spark"}) || result.Build.Spells.CastingAbilities[nestedNext] != "INT" {
		t.Fatal(result.Build.Spells)
	}
	if result.Play.ResourceUses["charge:"+nestedNext] != 1 || result.Play.ResourceUses["charge:feat:gift@grant%3Aexternal-long:spell"] != 2 || !result.Play.ActiveFeatures[nestedNext] || result.Play.Rolls[0].Resource != nestedNext {
		t.Fatal(result.Play)
	}
	if result.Play.Inventory[0].GrantID != "approved" || result.Play.Inventory[1].GrantID != "missing" || result.Play.Inventory[0].Notes != old {
		t.Fatal(result.Play.Inventory)
	}
	grant := result.Grants[0]
	if grant.ID != "external" || grant.ActorID != "original actor" || grant.Name != old || grant.Effects[0].Key != "charge:"+nestedNext || grant.Waivers[0] != "required-choice:"+current {
		t.Fatal(grant)
	}
	if result.Notes != nested {
		t.Fatal("changed authored notes")
	}
	result.Build.Spells.GrantChoices[nestedNext][0] = "changed"
	result.Build.Choices[0].Value[1] = 'X'
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("mutated caller-owned inputs")
	}
}

func TestRemapGrantReferencesRejectsCollisionsAndInvalidMappings(t *testing.T) {
	const old = "feat:practice@grant%3Aold:skill"
	const current = "feat:practice@grant%3Anew:skill"
	for _, scenario := range []struct {
		name  string
		setup func(*Inputs)
		ids   map[string]string
	}{
		{"resource collision", func(in *Inputs) { in.Play.ResourceUses[old] = 1; in.Play.ResourceUses[current] = 0 }, map[string]string{"old": "new"}},
		{"choice collision", func(in *Inputs) {
			in.Build.Choices = []Choice{{ID: old, Value: json.RawMessage(`"a"`)}, {ID: current, Value: json.RawMessage(`"b"`)}}
		}, map[string]string{"old": "new"}},
		{"empty source", func(*Inputs) {}, map[string]string{"": "new"}},
		{"empty target", func(*Inputs) {}, map[string]string{"old": ""}},
		{"shared target", func(*Inputs) {}, map[string]string{"old": "new", "other": "new"}},
		{"invalid encoding", func(in *Inputs) { in.Play.ResourceUses["feat:a@%zz:resource"] = 1 }, map[string]string{"old": "new"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			input := Blank()
			scenario.setup(&input)
			before, _ := json.Marshal(input)
			if _, err := RemapGrantReferences(input, scenario.ids); err == nil {
				t.Fatal("accepted ambiguous imported ownership")
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("changed input on rejection")
			}
		})
	}
}

func TestRemapGrantReferencesRekeysSimultaneously(t *testing.T) {
	input := Blank()
	input.Play.ResourceUses["feat:a@grant%3Afirst:pool"] = 1
	input.Play.ResourceUses["feat:a@grant%3Asecond:pool"] = 2
	input.Play.ResourceUses["feat:a:legacy"] = 3
	result, err := RemapGrantReferences(input, map[string]string{"first": "second", "second": "first"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"feat:a@grant%3Afirst:pool": 2, "feat:a@grant%3Asecond:pool": 1, "feat:a:legacy": 3}
	if !reflect.DeepEqual(result.Play.ResourceUses, want) {
		t.Fatal(result.Play.ResourceUses)
	}
}
