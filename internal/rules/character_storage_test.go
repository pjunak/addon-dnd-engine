package rules

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestStorageRetainsOwnedInstancesWithoutChangingMechanics(t *testing.T) {
	input, records, profile := characterFixture(t)
	input.Play.Inventory = quickUseItems()
	input.Play.QuickUse = []string{"one", "empty"}
	before := EvaluateCharacter(input, records, profile)
	input.Play.Containers = []character.Container{{ID: "pack", Name: "Pack"}, {ID: "pouch", Name: "Pack"}}
	input.Play.Inventory[0].ContainerID = "pack"
	input.Play.Inventory[1].ContainerID = "pouch"
	input.Play.Inventory[2].ContainerID = "pack"
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || !reflect.DeepEqual(result.Inputs, input) || object(result.Guidance["authoredPlay"])["storage"] != true {
		t.Fatal("valid storage rejected or rewritten", result.Issues)
	}
	for _, key := range []string{"derived", "attunement", "weapons", "quickUse"} {
		if !reflect.DeepEqual(before.Sheet[key], result.Sheet[key]) {
			t.Fatal("organization changed mechanics", key)
		}
	}
	containers := objects(object(result.Sheet["storage"])["containers"])
	if !reflect.DeepEqual(stringsOf(containers[0]["itemIds"]), []string{"one", "empty"}) || !reflect.DeepEqual(stringsOf(containers[1]["itemIds"]), []string{"duplicate-copy"}) {
		t.Fatal("same-name or depleted membership lost", containers)
	}
	for _, change := range []Object{{"operation": "consume-item", "itemId": "one"}, {"operation": "rest", "rest": "long"}, {"operation": "rest", "rest": "short"}} {
		next, err := ApplyCharacterPlay(input, change, records, profile)
		if err != nil || !reflect.DeepEqual(next.Inputs.Play.Containers, input.Play.Containers) {
			t.Fatal("play changed container identity", err)
		}
		for i, item := range input.Play.Inventory {
			if next.Inputs.Play.Inventory[i].ContainerID != item.ContainerID {
				t.Fatal("play changed membership", change)
			}
		}
	}
	result.Inputs.Play.Containers[0].Name = "Changed"
	result.Inputs.Play.Inventory[0].ContainerID = "Changed"
	if input.Play.Containers[0].Name != "Pack" || input.Play.Inventory[0].ContainerID != "pack" {
		t.Fatal("output aliases authored storage")
	}
}

func TestStorageRejectsInvalidAssignmentsWithoutNormalizing(t *testing.T) {
	input, records, profile := characterFixture(t)
	input.Play.Inventory = quickUseItems()
	input.Play.Containers = []character.Container{{ID: "pack", Name: "Pack"}}
	input.Play.Inventory[0].ContainerID = "pack"
	cases := map[string]func(*character.Inputs){
		"missing":           func(v *character.Inputs) { v.Play.Inventory[0].ContainerID = "missing" },
		"deleted-container": func(v *character.Inputs) { v.Play.Containers = nil },
		"blank-id":          func(v *character.Inputs) { v.Play.Containers[0].ID = " " },
		"duplicate-id":      func(v *character.Inputs) { v.Play.Containers = append(v.Play.Containers, v.Play.Containers[0]) },
		"blank-name":        func(v *character.Inputs) { v.Play.Containers[0].Name = " " },
		"long-name":         func(v *character.Inputs) { v.Play.Containers[0].Name = strings.Repeat("x", 121) },
		"equipped":          func(v *character.Inputs) { v.Play.Inventory[0].Location = "equipped" },
		"unknown-location":  func(v *character.Inputs) { v.Play.Inventory[0].Location = "unknown" },
		"limit": func(v *character.Inputs) {
			for i := 0; i < maximumContainers; i++ {
				v.Play.Containers = append(v.Play.Containers, character.Container{ID: jsonNumber(i), Name: "Pack"})
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := cloneCharacter(input)
			mutate(&candidate)
			result := EvaluateCharacter(candidate, records, profile)
			if result.Ready || truth(result.Guidance["canSave"]) || !reflect.DeepEqual(result.Inputs, candidate) {
				t.Fatal("invalid storage accepted or rewritten", result.Issues)
			}
		})
	}
}

func TestStorageSupportsOldIncompleteAndExplicitRemoval(t *testing.T) {
	_, records, profile := characterFixture(t)
	input := character.Blank()
	old := EvaluateCharacter(input, records, profile)
	body, _ := json.Marshal(old.Inputs.Play)
	if strings.Contains(string(body), "containers") {
		t.Fatal("default containers were added to an older character")
	}
	input.Play.Inventory = quickUseItems()
	input.Play.Containers = []character.Container{{ID: "pack", Name: "Pouch"}}
	input.Play.Inventory[0].ContainerID = "pack"
	result := EvaluateCharacter(input, records, profile)
	if !truth(result.Guidance["canSave"]) || result.Ready {
		t.Fatal("incomplete character cannot organize inventory", result.Issues)
	}
	input.Play.Containers = nil
	input.Play.Inventory[0].ContainerID = ""
	removed := EvaluateCharacter(input, records, profile)
	if !truth(removed.Guidance["canSave"]) || !reflect.DeepEqual(removed.Inputs.Play.Inventory, quickUseItems()) {
		t.Fatal("explicit container removal changed contents", removed.Issues)
	}
}
