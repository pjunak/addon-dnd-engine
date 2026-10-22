package rules

import (
	"encoding/hex"
	"fmt"
	"slices"

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

func handGrips(item *character.Item, records Records) []character.Grip {
	if item == nil || item.Reference == nil {
		return []character.Grip{}
	}
	record := equipmentRecord(*item, records)
	if text(record["armorType"]) == "shield" {
		return []character.Grip{character.GripOne}
	}
	if text(record["damage"]) == "" {
		return []character.Grip{}
	}
	properties := stringsOf(record["properties"])
	if contains(properties, "two-handed") {
		return []character.Grip{character.GripTwo}
	}
	if contains(properties, "versatile") && text(record["versatileDamage"]) != "" {
		return []character.Grip{character.GripOne, character.GripTwo}
	}
	return []character.Grip{character.GripOne}
}

func handEligibility(input character.Inputs, item *character.Item, records Records, off bool) character.HandReason {
	if item == nil {
		return character.HandMissing
	}
	if item.Quantity < 1 {
		return character.HandEmpty
	}
	grips := handGrips(item, records)
	if len(grips) == 0 || off && !slices.Contains(grips, character.GripOne) {
		return character.HandIneligible
	}
	proposed := input
	proposed.Play.Inventory = append([]character.Item(nil), input.Play.Inventory...)
	candidate := handItem(proposed, item.ID)
	candidate.Location = "equipped"
	if !characterEquipmentMechanics(*candidate, recordByID(records, item.Reference.Kind, item.Reference.ID), proposed, records) {
		return character.HandIneligible
	}
	return character.HandUsable
}

func handRestoreReason(input character.Inputs, records Records) character.HandReason {
	if input.Play.Hands == nil || input.Play.Hands.SuspendedOff == nil {
		return character.HandUsable
	}
	suspended := input.Play.Hands.SuspendedOff
	item := handItem(input, suspended.ItemID)
	if item == nil {
		return character.HandMissing
	}
	if item.Quantity < 1 {
		return character.HandEmpty
	}
	if item.Location != "carried" || character.HandItemFingerprint(*item) != suspended.ExpectedItemSHA256 {
		return character.HandChanged
	}
	if reason := handEligibility(input, item, records, true); reason != character.HandUsable {
		return reason
	}
	if item.ID == input.Play.Hands.Main {
		return character.HandOccupied
	}
	record := recordByID(records, item.Reference.Kind, item.Reference.ID)
	slot := characterItemSlot(*item, records)
	if slot != "worn" {
		for _, other := range input.Play.Inventory {
			if other.ID != item.ID && other.Quantity > 0 && other.Location == "equipped" && other.Reference != nil && characterItemSlot(other, records) == slot {
				return character.HandOccupied
			}
		}
	}
	if suspended.BodyPlacement != "" && !contains(characterBodyPlacements(*item, record, input, records), suspended.BodyPlacement) {
		return character.HandIneligible
	}
	return character.HandUsable
}

func validateCharacterHands(input character.Inputs, records Records, result *character.Result) {
	hands := input.Play.Hands
	if hands == nil {
		return
	}
	block := func(message string) {
		addCharacterIssue(result, "hands", "hands", message, character.SeverityBlocker, nil)
	}
	if !hands.Grip.Valid() || hands.Main != "" && hands.Main == hands.Off || hands.Grip == character.GripTwo && hands.Off != "" {
		block("Choose distinct hand instances and a supported grip.")
	}
	if suspended := hands.SuspendedOff; suspended != nil {
		digest, err := hex.DecodeString(suspended.ExpectedItemSHA256)
		if suspended.ItemID == "" || suspended.ItemID == hands.Main || suspended.ItemID == hands.Off || hands.Grip != character.GripTwo || err != nil || len(digest) != 32 {
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
		if item.ID == hands.Main && hands.Grip == character.GripTwo || !slices.Contains(grips, character.GripOne) {
			used += 2
		} else {
			used++
		}
	}
	if used > 2 {
		block("The equipped hand items require more than two hands. Stow an item or suspend the selected off hand.")
	}
	for _, selection := range []struct {
		id   string
		grip character.Grip
		off  bool
	}{{hands.Main, hands.Grip, false}, {hands.Off, character.GripOne, true}} {
		if selection.id == "" {
			continue
		}
		item := handItem(input, selection.id)
		if reason := handEligibility(input, item, records, selection.off); reason != character.HandUsable || item.Location != "equipped" {
			// Authored inventory edits may remove or stow a selected instance. Keep
			// its identity for explicit repair without granting an active hand.
			addCharacterIssue(result, "hand-inactive:"+selection.id, "hands", "The selected hand item is unavailable; choose another item or leave the hand free.", character.SeverityWarning, nil)
		} else if !slices.Contains(handGrips(item, records), selection.grip) {
			block("The selected weapon does not support this grip.")
		}
	}
}

func characterHands(input character.Inputs, records Records, result *character.Result) {
	options := []character.HandOption{}
	restorableOff := ""
	if hands := input.Play.Hands; hands != nil && hands.SuspendedOff != nil && handRestoreReason(input, records) == character.HandUsable {
		restorableOff = hands.SuspendedOff.ItemID
	}
	for _, item := range input.Play.Inventory {
		grips := handGrips(&item, records)
		if len(grips) == 0 {
			continue
		}
		options = append(options, character.HandOption{ItemID: item.ID, Name: item.Name, Grips: grips, CanMain: item.ID != restorableOff && handEligibility(input, &item, records, false) == character.HandUsable, CanOff: handEligibility(input, &item, records, true) == character.HandUsable})
	}
	hands := input.Play.Hands
	if hands == nil {
		hands = &character.Hands{Grip: character.GripOne}
	}
	slot := func(id string, off bool) character.HandSlot {
		entry := character.HandSlot{ItemID: id, Name: id}
		if item := handItem(input, id); item != nil {
			entry.Name, entry.Reference = item.Name, item.Reference
			entry.Active = item.Location == "equipped" && handEligibility(input, item, records, off) == character.HandUsable
		}
		return entry
	}
	saved := character.SheetHands{Main: slot(hands.Main, false), Off: slot(hands.Off, true), Grip: hands.Grip}
	if hands.SuspendedOff != nil {
		suspended := slot(hands.SuspendedOff.ItemID, true)
		suspended.Active = false
		saved.SuspendedOff = &suspended
		saved.RestoreReason = handRestoreReason(input, records)
	}
	result.Sheet.Hands = saved
	main := handItem(input, hands.Main)
	grips := handGrips(main, records)
	canRelease := main == nil || main.Quantity < 1 || main.Location != "equipped" || slices.Contains(grips, character.GripOne)
	canTwo := handEligibility(input, main, records, false) == character.HandUsable && main.Location == "equipped" && slices.Contains(grips, character.GripTwo)
	result.Guidance.Hands = character.HandsGuidance{Options: options, Grips: grips, CanRelease: canRelease, CanTwo: canTwo, RestoreReason: handRestoreReason(input, records)}
	result.Explanations["hands"] = character.Explanation{Label: "Hands and grip", Formula: "Owned instance selection and source-declared grip. A suspended off-hand item is carried and contributes no active equipment effects; its attunement allocation is retained. Restoration checks the exact unchanged instance. Turn and action costs are not tracked.", Value: saved, Terms: []character.Term{}, Sources: []character.Reference{}}
}

// handChange edits the hand state of a detached copy of the inputs, and
// records why a suspended off-hand item could not be restored.
type handChange struct {
	input         *character.Inputs
	records       Records
	restoreReason character.HandReason
	unrestored    string
}

func (change *handChange) hands() *character.Hands { return change.input.Play.Hands }

func (change *handChange) release() {
	hands := change.hands()
	if suspended := hands.SuspendedOff; suspended != nil {
		change.restoreReason = handRestoreReason(*change.input, change.records)
		if change.restoreReason == character.HandUsable {
			item := handItem(*change.input, suspended.ItemID)
			item.Location, item.BodyPlacement = "equipped", suspended.BodyPlacement
			hands.Off = item.ID
		} else {
			change.unrestored = suspended.ItemID
		}
		hands.SuspendedOff = nil
	}
	hands.Grip = character.GripOne
}

func (change *handChange) suspend() {
	hands := change.hands()
	if item := handItem(*change.input, hands.Off); item != nil && item.Quantity > 0 && item.Location == "equipped" {
		placement := item.BodyPlacement
		item.Location, item.BodyPlacement = "carried", ""
		hands.SuspendedOff = &character.SuspendedHand{ItemID: item.ID, ExpectedItemSHA256: character.HandItemFingerprint(*item), BodyPlacement: placement}
	} else if hands.Off != "" {
		change.restoreReason, change.unrestored = character.HandChanged, hands.Off
	}
	hands.Off, hands.Grip = "", character.GripTwo
}

func (change *handChange) stow(id string) {
	if item := handItem(*change.input, id); item != nil && item.Location == "equipped" {
		item.Location, item.BodyPlacement = "carried", ""
	}
}

func (change *handChange) setHand(hand character.Hand, id string) error {
	hands, input := change.hands(), *change.input
	if hand == character.HandMain && id == hands.Off && id != "" || hand == character.HandOff && (id == hands.Main && id != "" || hands.Grip == character.GripTwo) {
		return fmt.Errorf("Choose a distinct available hand; release the two-handed grip first.")
	}
	item := handItem(input, id)
	if id != "" && handEligibility(input, item, change.records, hand == character.HandOff) != character.HandUsable {
		return fmt.Errorf("The selected item cannot be used in that hand.")
	}
	if hand == character.HandMain {
		change.release()
		if hands.Off == id && id != "" {
			return fmt.Errorf("Choose a distinct item for each hand.")
		}
		if hands.Main != id {
			change.stow(hands.Main)
		}
		hands.Main = id
	} else {
		if hands.Off != id {
			change.stow(hands.Off)
		}
		hands.Off = id
	}
	if item != nil {
		item.Location, item.ContainerID = "equipped", ""
		if hand == character.HandMain && !slices.Contains(handGrips(item, change.records), character.GripOne) {
			change.suspend()
		}
	}
	return nil
}

func (change *handChange) setGrip(grip character.Grip) error {
	hands, input := change.hands(), *change.input
	item := handItem(input, hands.Main)
	if grip == character.GripTwo && (handEligibility(input, item, change.records, false) != character.HandUsable || item.Location != "equipped" || !slices.Contains(handGrips(item, change.records), character.GripTwo)) || grip == character.GripOne && item != nil && item.Quantity > 0 && item.Location == "equipped" && !slices.Contains(handGrips(item, change.records), character.GripOne) {
		return fmt.Errorf("The selected weapon does not support this grip.")
	}
	if grip == character.GripOne {
		change.release()
	} else if hands.Grip != character.GripTwo {
		change.suspend()
	}
	return nil
}

// applyHandCommand applies a set-hand or set-grip change to a detached copy
// of the inputs; legal unfinished builds may change hands.
func applyHandCommand(input character.Inputs, records Records, profile Ruleset, apply func(*handChange) error) (character.Result, error) {
	current := EvaluateCharacter(input, records, profile)
	input = cloneCharacter(current.Inputs)
	if input.Play.Hands == nil {
		input.Play.Hands = &character.Hands{Grip: character.GripOne}
	}
	change := &handChange{input: &input, records: records}
	if err := apply(change); err != nil {
		return current, err
	}
	result := EvaluateCharacter(input, records, profile)
	if !result.Guidance.CanSave {
		return current, fmt.Errorf("Resolve the character's equipment conflicts before changing hands.")
	}
	if change.restoreReason != character.HandUsable {
		result.Sheet.Hands.RestoreReason = change.restoreReason
		result.Sheet.Hands.UnrestoredItem = change.unrestored
	}
	return result, nil
}
