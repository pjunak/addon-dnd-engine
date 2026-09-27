package rules

import (
	"encoding/hex"
	"fmt"

	"github.com/pjunak/addon-dnd-engine/character"
)

func handItem(input character.Inputs, id string) *character.Item {
	for index := range input.Play.Inventory {
		if input.Play.Inventory[index].ID == id {
			return &input.Play.Inventory[index]
		}
	}
	return nil
}

func characterItemSuspended(input character.Inputs, id string) bool {
	return input.Play.Hands != nil && input.Play.Hands.SuspendedOff != nil && input.Play.Hands.SuspendedOff.ItemID == id
}

func handGrips(item *character.Item, records Records) []string {
	if item == nil || item.Reference == nil {
		return []string{}
	}
	record := recordByID(records, item.Reference.Kind, item.Reference.ID)
	if text(record["armorType"]) == "shield" {
		return []string{"one"}
	}
	if text(record["damage"]) == "" {
		return []string{}
	}
	properties := stringsOf(record["properties"])
	if contains(properties, "two-handed") {
		return []string{"two"}
	}
	if contains(properties, "versatile") && text(record["versatileDamage"]) != "" {
		return []string{"one", "two"}
	}
	return []string{"one"}
}

func handEligibility(input character.Inputs, item *character.Item, records Records, off bool) string {
	if item == nil {
		return "missing"
	}
	if item.Quantity < 1 {
		return "empty"
	}
	grips := handGrips(item, records)
	if len(grips) == 0 || off && !contains(grips, "one") {
		return "ineligible"
	}
	proposed := input
	proposed.Play.Inventory = append([]character.Item(nil), input.Play.Inventory...)
	candidate := handItem(proposed, item.ID)
	candidate.Location = "equipped"
	if !characterEquipmentMechanics(*candidate, recordByID(records, item.Reference.Kind, item.Reference.ID), proposed) {
		return "ineligible"
	}
	return ""
}

func handRestoreReason(input character.Inputs, records Records) string {
	if input.Play.Hands == nil || input.Play.Hands.SuspendedOff == nil {
		return ""
	}
	suspended := input.Play.Hands.SuspendedOff
	item := handItem(input, suspended.ItemID)
	if item == nil {
		return "missing"
	}
	if item.Quantity < 1 {
		return "empty"
	}
	if item.Location != "carried" || character.HandItemFingerprint(*item) != suspended.ExpectedItemSHA256 {
		return "changed"
	}
	if reason := handEligibility(input, item, records, true); reason != "" {
		return reason
	}
	if item.ID == input.Play.Hands.Main {
		return "occupied"
	}
	record := recordByID(records, item.Reference.Kind, item.Reference.ID)
	slot := characterEquipmentSlot(record)
	if slot != "worn" {
		for _, other := range input.Play.Inventory {
			if other.ID != item.ID && other.Quantity > 0 && other.Location == "equipped" && other.Reference != nil && characterEquipmentSlot(recordByID(records, other.Reference.Kind, other.Reference.ID)) == slot {
				return "occupied"
			}
		}
	}
	if suspended.BodyPlacement != "" && !contains(characterBodyPlacements(*item, record, input), suspended.BodyPlacement) {
		return "ineligible"
	}
	return ""
}

func validateCharacterHands(input character.Inputs, records Records, result *character.Result) {
	hands := input.Play.Hands
	if hands == nil {
		return
	}
	block := func(message string) { addCharacterIssue(result, "hands", "hands", message, "blocker", nil) }
	if !contains([]string{"one", "two"}, hands.Grip) || hands.Main != "" && hands.Main == hands.Off || hands.Grip == "two" && hands.Off != "" {
		block("Choose distinct hand instances and a supported grip.")
	}
	if suspended := hands.SuspendedOff; suspended != nil {
		digest, err := hex.DecodeString(suspended.ExpectedItemSHA256)
		if suspended.ItemID == "" || suspended.ItemID == hands.Main || suspended.ItemID == hands.Off || hands.Grip != "two" || err != nil || len(digest) != 32 {
			block("The suspended off-hand identity is invalid.")
		}
	}
	used := 0
	for _, item := range input.Play.Inventory {
		if item.Quantity < 1 || item.Location != "equipped" || characterItemSuspended(input, item.ID) {
			continue
		}
		grips := handGrips(&item, records)
		if len(grips) == 0 {
			continue
		}
		if item.ID == hands.Main && hands.Grip == "two" || !contains(grips, "one") {
			used += 2
		} else {
			used++
		}
	}
	if used > 2 {
		block("The equipped hand items require more than two hands. Stow an item or suspend the selected off hand.")
	}
	for _, selection := range []struct {
		id, grip string
		off      bool
	}{{hands.Main, hands.Grip, false}, {hands.Off, "one", true}} {
		if selection.id == "" {
			continue
		}
		item := handItem(input, selection.id)
		if reason := handEligibility(input, item, records, selection.off); reason != "" || item.Location != "equipped" {
			// Authored inventory edits may remove or stow a selected instance. Keep
			// its identity for explicit repair without granting an active hand.
			addCharacterIssue(result, "hand-inactive:"+selection.id, "hands", "The selected hand item is unavailable; choose another item or leave the hand free.", "warning", nil)
		} else if !contains(handGrips(item, records), selection.grip) {
			block("The selected weapon does not support this grip.")
		}
	}
}

func characterHands(input character.Inputs, records Records, result *character.Result) {
	options := []any{}
	restorableOff := ""
	if hands := input.Play.Hands; hands != nil && hands.SuspendedOff != nil && handRestoreReason(input, records) == "" {
		restorableOff = hands.SuspendedOff.ItemID
	}
	for _, item := range input.Play.Inventory {
		grips := handGrips(&item, records)
		if len(grips) == 0 {
			continue
		}
		options = append(options, Object{"itemId": item.ID, "name": item.Name, "grips": anyStrings(grips), "canMain": item.ID != restorableOff && handEligibility(input, &item, records, false) == "", "canOff": handEligibility(input, &item, records, true) == ""})
	}
	hands := input.Play.Hands
	if hands == nil {
		hands = &character.Hands{Grip: "one"}
	}
	row := func(id string, off bool) Object {
		entry := Object{"itemId": id, "name": id, "active": false}
		if item := handItem(input, id); item != nil {
			entry["name"], entry["reference"] = item.Name, item.Reference
			entry["active"] = item.Location == "equipped" && handEligibility(input, item, records, off) == ""
		}
		return entry
	}
	saved := Object{"main": row(hands.Main, false), "off": row(hands.Off, true), "grip": hands.Grip}
	if hands.SuspendedOff != nil {
		saved["suspendedOff"] = row(hands.SuspendedOff.ItemID, true)
		object(saved["suspendedOff"])["active"] = false
		saved["restoreReason"] = handRestoreReason(input, records)
	}
	result.Sheet["hands"] = saved
	main := handItem(input, hands.Main)
	grips := handGrips(main, records)
	canRelease := main == nil || main.Quantity < 1 || main.Location != "equipped" || contains(grips, "one")
	canTwo := handEligibility(input, main, records, false) == "" && main.Location == "equipped" && contains(grips, "two")
	result.Guidance["hands"] = Object{"options": options, "grips": anyStrings(grips), "canRelease": canRelease, "canTwo": canTwo, "restoreReason": handRestoreReason(input, records)}
	result.Explanations["hands"] = character.Explanation{Label: "Hands and grip", Formula: "Owned instance selection and source-declared grip. A suspended off-hand item is carried and contributes no active equipment effects; its attunement allocation is retained. Restoration checks the exact unchanged instance. Turn and action costs are not tracked.", Value: saved, Terms: []character.Term{}, Sources: []character.Reference{}}
}

func applyCharacterHandCommand(input character.Inputs, change Object, records Records, profile Ruleset) (character.Result, error) {
	current := EvaluateCharacter(input, records, profile)
	input = cloneCharacter(current.Inputs)
	if input.Play.Hands == nil {
		input.Play.Hands = &character.Hands{Grip: "one"}
	}
	hands := input.Play.Hands
	restoreReason, unrestored := "", ""
	release := func() {
		if suspended := hands.SuspendedOff; suspended != nil {
			restoreReason = handRestoreReason(input, records)
			if restoreReason == "" {
				item := handItem(input, suspended.ItemID)
				item.Location, item.BodyPlacement = "equipped", suspended.BodyPlacement
				hands.Off = item.ID
			} else {
				unrestored = suspended.ItemID
			}
			hands.SuspendedOff = nil
		}
		hands.Grip = "one"
	}
	suspend := func() {
		if item := handItem(input, hands.Off); item != nil && item.Quantity > 0 && item.Location == "equipped" {
			placement := item.BodyPlacement
			item.Location, item.BodyPlacement = "carried", ""
			hands.SuspendedOff = &character.SuspendedHand{ItemID: item.ID, ExpectedItemSHA256: character.HandItemFingerprint(*item), BodyPlacement: placement}
		} else if hands.Off != "" {
			restoreReason, unrestored = "changed", hands.Off
		}
		hands.Off, hands.Grip = "", "two"
	}
	stow := func(id string) {
		if item := handItem(input, id); item != nil && item.Location == "equipped" {
			item.Location, item.BodyPlacement = "carried", ""
		}
	}
	switch text(change["operation"]) {
	case "set-hand":
		hand, ok := change["hand"].(string)
		id, idOK := change["itemId"].(string)
		if len(change) != 3 || !ok || !idOK || !contains([]string{"main", "off"}, hand) {
			return current, fmt.Errorf("Choose a hand and one owned item, or leave it free.")
		}
		if hand == "main" && id == hands.Off && id != "" || hand == "off" && (id == hands.Main && id != "" || hands.Grip == "two") {
			return current, fmt.Errorf("Choose a distinct available hand; release the two-handed grip first.")
		}
		item := handItem(input, id)
		if id != "" && handEligibility(input, item, records, hand == "off") != "" {
			return current, fmt.Errorf("The selected item cannot be used in that hand.")
		}
		if hand == "main" {
			release()
			if hands.Off == id && id != "" {
				return current, fmt.Errorf("Choose a distinct item for each hand.")
			}
			if hands.Main != id {
				stow(hands.Main)
			}
			hands.Main = id
		} else {
			if hands.Off != id {
				stow(hands.Off)
			}
			hands.Off = id
		}
		if item != nil {
			item.Location, item.ContainerID = "equipped", ""
			if hand == "main" && !contains(handGrips(item, records), "one") {
				suspend()
			}
		}
	case "set-grip":
		grip, ok := change["grip"].(string)
		if len(change) != 2 || !ok || !contains([]string{"one", "two"}, grip) {
			return current, fmt.Errorf("Choose a supported hand grip.")
		}
		item := handItem(input, hands.Main)
		if grip == "two" && (handEligibility(input, item, records, false) != "" || item.Location != "equipped" || !contains(handGrips(item, records), "two")) || grip == "one" && item != nil && item.Quantity > 0 && item.Location == "equipped" && !contains(handGrips(item, records), "one") {
			return current, fmt.Errorf("The selected weapon does not support this grip.")
		}
		if grip == "one" {
			release()
		} else if hands.Grip != "two" {
			suspend()
		}
	default:
		return current, fmt.Errorf("Choose a supported hand operation.")
	}
	result := EvaluateCharacter(input, records, profile)
	if !truth(result.Guidance["canSave"]) {
		return current, fmt.Errorf("Resolve the character's equipment conflicts before changing hands.")
	}
	if restoreReason != "" {
		object(result.Sheet["hands"])["restoreReason"] = restoreReason
		object(result.Sheet["hands"])["unrestoredItem"] = unrestored
	}
	return result, nil
}
