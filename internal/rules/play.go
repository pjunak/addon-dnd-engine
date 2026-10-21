package rules

import (
	"fmt"

	"github.com/pjunak/addon-dnd-engine/character"
)

// ApplyCharacterPlay applies one play command. It is the only dispatcher of
// play operations.
func ApplyCharacterPlay(input character.Inputs, command character.PlayCommand, records Records, profile Ruleset) (character.Result, error) {
	// Play commands resolve records by ID only, like evaluation, and share one
	// decode cache across the evaluations of this command.
	snapshot := newCharacterRecordSnapshot(records)
	if verifyRecords != nil {
		defer verifyRecords(snapshot)
	}
	records = snapshot
	switch command.Operation {
	case character.PlayReplaceClassChoice:
		return replaceClassChoice(input, command, records, profile)
	case character.PlaySetHand:
		return applyHandCommand(input, records, profile, func(change *handChange) error { return change.setHand(command.Hand, command.ItemID) })
	case character.PlaySetGrip:
		return applyHandCommand(input, records, profile, func(change *handChange) error { return change.setGrip(command.Grip) })
	case character.PlayKeepConcentration:
		return applyConcentrationCommand(input, records, profile, func(saved *character.Inputs) { saved.Play.Concentration.SaveDC = 0 })
	case character.PlayEndConcentration:
		return applyConcentrationCommand(input, records, profile, func(saved *character.Inputs) { saved.Play.Concentration = nil })
	case character.PlayConsumeItem:
		return playInputs(input, records, profile, func(next *character.Inputs, _ character.Result) error {
			return consumeCharacterItem(next, command.ItemID)
		})
	case character.PlaySpendHitDie:
		return playInputs(input, records, profile, func(next *character.Inputs, current character.Result) error {
			return spendHitDie(next, current.Sheet.Calculation, command, profile)
		})
	case character.PlaySetHP:
		return playInputs(input, records, profile, func(next *character.Inputs, current character.Result) error {
			maximum := current.Sheet.Derived.MaxHP
			if command.Amount > maximum {
				return fmt.Errorf("Current HP cannot exceed the effective maximum (%d).", maximum)
			}
			return withAmount(command, func(amount int) { next.Play.HP = amount })
		})
	case character.PlayHeal:
		return playInputs(input, records, profile, func(next *character.Inputs, current character.Result) error {
			return withAmount(command, func(amount int) { next.Play.HP = min(current.Sheet.Derived.MaxHP, next.Play.HP+amount) })
		})
	case character.PlayDamage:
		return playInputs(input, records, profile, func(next *character.Inputs, _ character.Result) error {
			return withAmount(command, func(amount int) { takeDamage(next, amount, profile) })
		})
	case character.PlaySetTemporaryHP:
		return playInputs(input, records, profile, func(next *character.Inputs, _ character.Result) error {
			return withAmount(command, func(amount int) { next.Play.TemporaryHP = amount })
		})
	case character.PlayRest:
		return playDecisions(input, records, profile, nil, func(decisions Object, sheet *character.Calculation) error {
			applyRest(decisions, sheet, command.Rest)
			return nil
		})
	case character.PlayToggleFeature:
		return playDecisions(input, records, profile, func(next *character.Inputs, current character.Result) error {
			return spendActivationResource(next, current.Sheet.Calculation, command)
		}, func(decisions Object, sheet *character.Calculation) error {
			return toggleFeature(decisions, sheet, command)
		})
	case character.PlaySelectSpell:
		return playDecisions(input, records, profile, nil, func(decisions Object, sheet *character.Calculation) error {
			return selectPlaySpell(decisions, sheet, command.ClassID, command.Ref, command.Selection, command.Selected, records)
		})
	case character.PlayCastSpell:
		return playCast(input, records, profile, castRef(command.Ref), func(decisions Object, sheet *character.Calculation) error {
			return castPlaySpell(decisions, sheet, command, records)
		})
	case character.PlaySelectGrantSpell:
		return playDecisions(input, records, profile, nil, func(decisions Object, sheet *character.Calculation) error {
			return selectGrantSpell(decisions, sheet, command, records)
		})
	case character.PlaySelectCastingAbility:
		return playDecisions(input, records, profile, nil, func(decisions Object, sheet *character.Calculation) error {
			return selectCastingAbility(decisions, sheet, command)
		})
	case character.PlayCastGrantedSpell:
		return playCastGranted(input, records, profile, command)
	case character.PlayCastRitual:
		return playCast(input, records, profile, castRef(command.Ref), func(decisions Object, sheet *character.Calculation) error {
			if !canCastRitual(decisions, playCaster(sheet, command.ClassID), recordByID(records, "spell", command.Ref)) {
				return fmt.Errorf("This class cannot cast this spell as a ritual.")
			}
			return nil
		})
	case character.PlayCopySpell:
		return copySpell(input, records, profile, command)
	case character.PlaySwapSpell:
		return playDecisions(input, records, profile, func(next *character.Inputs, _ character.Result) error {
			if characterSpellReplacements(*next, records, command.ClassID) < 1 {
				return fmt.Errorf("No recorded level-up spell replacements remain for this class.")
			}
			return nil
		}, func(decisions Object, sheet *character.Calculation) error {
			return swapPlaySpell(decisions, sheet, command, records)
		})
	}
	return EvaluateCharacter(input, records, profile), fmt.Errorf("The play change has invalid fields.")
}

// playReady evaluates the character and refuses play while choices block it.
// It returns the evaluation and a detached copy of its inputs to change.
func readyForPlay(input character.Inputs, records Records, profile Ruleset) (character.Result, character.Inputs, error) {
	current := EvaluateCharacter(input, records, profile)
	if !current.Ready {
		return current, input, fmt.Errorf("Resolve the character's blocking choices before applying play changes.")
	}
	return current, cloneCharacter(current.Inputs), nil
}

// playInputs changes the saved inputs directly, then evaluates them.
func playInputs(input character.Inputs, records Records, profile Ruleset, change func(*character.Inputs, character.Result) error) (character.Result, error) {
	current, next, err := readyForPlay(input, records, profile)
	if err != nil {
		return current, err
	}
	if err := change(&next, current); err != nil {
		return current, err
	}
	return EvaluateCharacter(next, records, profile), nil
}

// playDecisions runs a Builder-decision change: before (optional) checks or
// changes the inputs, change edits the decisions with the current sheet, and
// the changed play state is written back to the inputs.
func playDecisions(
	input character.Inputs, records Records, profile Ruleset,
	before func(*character.Inputs, character.Result) error,
	change func(Object, *character.Calculation) error,
) (character.Result, error) {
	current, next, err := readyForPlay(input, records, profile)
	if err != nil {
		return current, err
	}
	if err := decisionsPlay(&next, current, records, profile, before, change); err != nil {
		return current, err
	}
	return EvaluateCharacter(next, records, profile), nil
}

func decisionsPlay(
	input *character.Inputs, current character.Result, records Records, profile Ruleset,
	before func(*character.Inputs, character.Result) error,
	change func(Object, *character.Calculation) error,
) error {
	if before != nil {
		if err := before(input, current); err != nil {
			return err
		}
	}
	sheet := current.Sheet.Calculation
	decisions := characterDecisions(*input, records, profile, &current)
	decisions["resourceUses"] = characterRemainingResources(*input, sheet)
	next := NormalizeBuilderDecisions(decisions, records, profile)
	if err := change(next, sheet); err != nil {
		return err
	}
	writePlayDecisions(input, next, sheet)
	return nil
}

// writePlayDecisions copies the play state a decision change edits back to
// the inputs. Resolved feats are computation output and are not written.
func writePlayDecisions(input *character.Inputs, next Object, sheet *character.Calculation) {
	input.Play.HP = integer(next["hp"], input.Play.HP)
	input.Play.TemporaryHP = integer(next["tempHp"], input.Play.TemporaryHP)
	decodeInto(next["currency"], &input.Play.Currency)
	for _, resource := range sheet.Resources {
		if input.Play.ResourceUses == nil {
			input.Play.ResourceUses = map[string]int{}
		}
		input.Play.ResourceUses[resource.Key] = resource.Max - integer(object(next["resourceUses"])[resource.Key], resource.Max)
	}
	decodeInto(next["activeFeatures"], &input.Play.ActiveFeatures)
	decodeInto(next["preparedSpells"], &input.Play.PreparedSpells)
	decodeInto(next["cantrips"], &input.Build.Spells.Cantrips)
	decodeInto(next["spellbook"], &input.Build.Spells.Spellbook)
	decodeInto(next["grantChoices"], &input.Build.Spells.GrantChoices)
	decodeInto(next["grantCastingAbilities"], &input.Build.Spells.CastingAbilities)
	previousSwaps := len(input.Build.Spells.Swaps)
	decodeInto(next["spellSwaps"], &input.Build.Spells.Swaps)
	for index := previousSwaps; index < len(input.Build.Spells.Swaps); index++ {
		input.Build.Spells.Swaps[index].Origin = "recorded"
	}
	items := map[string]Object{}
	for _, item := range objects(next["inventory"]) {
		items[text(item["id"])] = item
	}
	for index := range input.Play.Inventory {
		item := &input.Play.Inventory[index]
		before := item.Quantity
		if next, ok := items[item.ID]; ok {
			item.Quantity = integer(next["qty"], item.Quantity)
		} else {
			item.Quantity = 0
		}
		if before > 0 && item.Quantity == 0 {
			clearCharacterItemState(input, item.ID)
		}
	}
}

// playCast casts a spell through the decisions and, when the spell needs
// Concentration, starts concentrating on it. spell names the cast spell.
func playCast(input character.Inputs, records Records, profile Ruleset, spell func(*character.Calculation) string, cast func(Object, *character.Calculation) error) (character.Result, error) {
	current, next, err := readyForPlay(input, records, profile)
	if err != nil {
		return current, err
	}
	if err := decisionsPlay(&next, current, records, profile, nil, cast); err != nil {
		return current, err
	}
	if ref := spell(current.Sheet.Calculation); truth(recordByID(records, "spell", ref)["concentration"]) {
		next.Play.Concentration = &character.Concentration{Ref: ref}
	}
	return EvaluateCharacter(next, records, profile), nil
}

// castRef names the spell a command casts by its reference.
func castRef(ref string) func(*character.Calculation) string {
	return func(*character.Calculation) string { return ref }
}

func playCastGranted(input character.Inputs, records Records, profile Ruleset, command character.PlayCommand) (character.Result, error) {
	granted := func(sheet *character.Calculation) string {
		for _, grant := range sheet.Spellcasting.Granted {
			if spellGrantKey(grant) == command.Key {
				return grant.Ref
			}
		}
		return ""
	}
	return playCast(input, records, profile, granted, func(decisions Object, sheet *character.Calculation) error {
		return castGrantedSpell(decisions, sheet, command, records)
	})
}

// copySpell copies a spell into a spellbook, paying for it and spending the
// scroll when one is named, and records the acquisition.
func copySpell(input character.Inputs, records Records, profile Ruleset, command character.PlayCommand) (character.Result, error) {
	current, next, err := readyForPlay(input, records, profile)
	if err != nil {
		return current, err
	}
	if command.AcquisitionID == "" {
		return current, fmt.Errorf("Spell copying requires a recorded acquisition ID.")
	}
	for _, acquisition := range next.Build.Spells.Acquisitions {
		if acquisition.ID == command.AcquisitionID {
			return current, fmt.Errorf("This spell acquisition is already recorded.")
		}
	}
	if command.ScrollID != "" {
		found := false
		for _, item := range next.Play.Inventory {
			found = found || item.ID == command.ScrollID && item.Quantity > 0 && item.SpellID == command.Ref
		}
		if !found {
			return current, fmt.Errorf("Choose an owned scroll explicitly associated with this spell.")
		}
	}
	err = decisionsPlay(&next, current, records, profile, nil, func(decisions Object, sheet *character.Calculation) error {
		return copyPlaySpell(decisions, sheet, command, records, profile)
	})
	if err != nil {
		return current, err
	}
	spell := recordByID(records, "spell", command.Ref)
	next.Build.Spells.Acquisitions = append(next.Build.Spells.Acquisitions, character.SpellAcquisition{ID: command.AcquisitionID, ClassID: command.ClassID, SpellID: command.Ref, Level: len(next.Build.Levels), At: next.Play.AsOf, CostGP: ScrollCopyCost(number(spell["level"], 0), profile), ScrollID: command.ScrollID, Origin: "recorded"})
	return EvaluateCharacter(next, records, profile), nil
}

func spendHitDie(input *character.Inputs, sheet *character.Calculation, command character.PlayCommand, profile Ruleset) error {
	resource := findPlayResource(sheet, command.Key)
	if resource == nil || resource.Kind != character.ResourceHitDice {
		return fmt.Errorf("Record a valid result for an available hit die.")
	}
	size := hitDieSize(resource.Die)
	if command.Result < 1 || command.Result > size || command.RollID == "" {
		return fmt.Errorf("Record a valid result for an available hit die.")
	}
	for _, old := range input.Play.Rolls {
		if old.ID == command.RollID {
			return fmt.Errorf("This roll has already been recorded.")
		}
	}
	if input.Play.ResourceUses[resource.Key] >= resource.Max {
		return fmt.Errorf("No hit dice remain in this pool.")
	}
	if input.Play.ResourceUses == nil {
		input.Play.ResourceUses = map[string]int{}
	}
	input.Play.ResourceUses[resource.Key]++
	healing := max(profile.Constants.Character.MinimumHPGain, command.Result+sheet.Abilities["CON"].Mod)
	input.Play.HP = min(sheet.Derived.MaxHP, input.Play.HP+healing)
	input.Play.Rolls = append(input.Play.Rolls, character.PlayRoll{ID: command.RollID, Resource: resource.Key, Die: size, Result: command.Result, At: input.Play.AsOf, Origin: "recorded"})
	return nil
}

// withAmount applies a hit-point command's amount once it is in range.
func withAmount(command character.PlayCommand, apply func(int)) error {
	if command.Amount < 0 || command.Amount > 1_000_000 {
		return fmt.Errorf("Enter a non-negative whole amount.")
	}
	apply(command.Amount)
	return nil
}

// takeDamage spends temporary HP first, then current HP, and records the
// concentration save the damage calls for.
func takeDamage(input *character.Inputs, amount int, profile Ruleset) {
	absorbed := min(input.Play.TemporaryHP, amount)
	input.Play.TemporaryHP -= absorbed
	input.Play.HP = max(0, input.Play.HP-(amount-absorbed))
	concentrationAfterDamage(input, amount, profile)
}

// spendActivationResource spends a feature's uses when it is switched on.
func spendActivationResource(input *character.Inputs, sheet *character.Calculation, command character.PlayCommand) error {
	if !command.Enabled || input.Play.ActiveFeatures[command.Key] {
		return nil
	}
	for _, activation := range sheet.Activations {
		if activation.Key != command.Key || activation.Resource == nil || activation.Resource.Key == "" {
			continue
		}
		cost := max(1, activation.Resource.Cost)
		resource := findPlayResource(sheet, activation.Resource.Key)
		if resource == nil || input.Play.ResourceUses[resource.Key]+cost > resource.Max {
			return fmt.Errorf("No uses of this feature remain.")
		}
		if input.Play.ResourceUses == nil {
			input.Play.ResourceUses = map[string]int{}
		}
		input.Play.ResourceUses[resource.Key] += cost
	}
	return nil
}

func toggleFeature(decisions Object, sheet *character.Calculation, command character.PlayCommand) error {
	var selected *character.Activation
	for index := range sheet.Activations {
		if sheet.Activations[index].Key == command.Key {
			selected = &sheet.Activations[index]
			break
		}
	}
	if selected == nil || command.Enabled && !selected.Available {
		return fmt.Errorf("This feature is unavailable with the current equipment.")
	}
	active := playMap(decisions, "activeFeatures")
	delete(active, command.Key)
	if command.Enabled {
		if group := selected.ExclusiveGroup; group != nil && *group != "" {
			for _, activation := range sheet.Activations {
				if activation.ExclusiveGroup != nil && *activation.ExclusiveGroup == *group {
					delete(active, activation.Key)
				}
			}
		}
		active[command.Key] = true
	}
	return nil
}

func playMap(decisions Object, key string) Object {
	value := object(decisions[key])
	if value == nil {
		value = Object{}
		decisions[key] = value
	}
	return value
}

func findPlayResource(sheet *character.Calculation, key string) *character.Resource {
	for index := range sheet.Resources {
		if sheet.Resources[index].Key == key {
			return &sheet.Resources[index]
		}
	}
	return nil
}

func remainingResource(decisions Object, resource *character.Resource) int {
	maximum := max(0, resource.Max)
	return min(maximum, max(0, integer(object(decisions["resourceUses"])[resource.Key], maximum)))
}

func spendResource(decisions Object, resource *character.Resource) error {
	return spendResourceUses(decisions, resource, 1)
}

func spendResourceUses(decisions Object, resource *character.Resource, cost int) error {
	if resource == nil || remainingResource(decisions, resource) < cost {
		return fmt.Errorf("No uses remain for this resource.")
	}
	playMap(decisions, "resourceUses")[resource.Key] = remainingResource(decisions, resource) - cost
	return nil
}

func applyRest(decisions Object, sheet *character.Calculation, rest character.RestKind) {
	uses := playMap(decisions, "resourceUses")
	// Older profiles recover half the total level across all hit-die pools,
	// rather than granting that allowance separately to every multiclass die.
	hitDiceBudget := max(1, sheet.TotalLevel/2)
	for index := range sheet.Resources {
		resource := &sheet.Resources[index]
		maximum := max(0, resource.Max)
		current := remainingResource(decisions, resource)
		for _, recharge := range resource.Recharge {
			on := recharge.On
			if string(on) != string(rest) && on != character.RechargeShortOrLong && !(rest == character.RestLong && on == character.RechargeShort) {
				continue
			}
			switch amount := recharge.Amount; {
			case amount.Full:
				current = maximum
			case amount.HalfLevel:
				recovered := min(maximum-current, max(1, sheet.TotalLevel/2))
				if resource.Kind == character.ResourceHitDice {
					recovered = min(recovered, hitDiceBudget)
					hitDiceBudget -= recovered
				}
				current += recovered
			default:
				recovered := amount.Uses
				if amount.Ability != "" {
					recovered = max(1, sheet.Abilities[amount.Ability].Mod)
				}
				current = min(maximum, current+recovered)
			}
		}
		if current >= maximum {
			delete(uses, resource.Key)
		} else {
			uses[resource.Key] = current
		}
	}
	if rest == character.RestLong {
		decisions["hp"] = max(0, sheet.Derived.MaxHP)
		decisions["tempHp"] = 0
		decisions["activeFeatures"] = Object{}
	}
}

func playCaster(sheet *character.Calculation, classID string) *character.Caster {
	for index := range sheet.Spellcasting.PerClass {
		if sheet.Spellcasting.PerClass[index].ClassID == classID {
			return &sheet.Spellcasting.PerClass[index]
		}
	}
	return nil
}

func selectPlaySpell(decisions Object, sheet *character.Calculation, classID, ref string, selection character.SpellList, selected bool, records Records) error {
	lists := playMap(decisions, string(selection))
	current := unique(stringsOf(lists[classID]))
	// Removing an obsolete reference must remain possible after a provider update.
	if !selected {
		filtered := make([]string, 0)
		for _, item := range current {
			if item != ref {
				filtered = append(filtered, item)
			}
		}
		lists[classID] = anyStrings(filtered)
		if selection == character.ListSpellbook {
			return selectPlaySpell(decisions, sheet, classID, ref, character.ListPrepared, false, records)
		}
		return nil
	}
	spell, caster := recordByID(records, "spell", ref), playCaster(sheet, classID)
	if spell == nil || caster == nil {
		return fmt.Errorf("Choose a spell and a spellcasting class.")
	}
	level := integer(spell["level"], 0)
	if !contains(stringsOf(spell["classes"]), caster.SpellListClassID) && !contains(caster.ExpandedSpellIDs, ref) {
		return fmt.Errorf("This spell is not on the class spell list.")
	}
	if level > caster.MaxSpellLevel || (selection == character.ListCantrips) != (level == 0) {
		return fmt.Errorf("This spell is not eligible for this selection.")
	}
	limit := caster.PreparedLimit
	if selection == character.ListCantrips {
		limit = caster.CantripsKnown
	}
	if selection == character.ListSpellbook {
		if caster.Prepares != "spellbook" {
			return fmt.Errorf("This class does not use a spellbook.")
		}
		// Books can contain copied spells beyond level-up additions.
		limit = 5000
	}
	if selection == character.ListPrepared && caster.Prepares == "spellbook" && !contains(stringsOf(object(decisions["spellbook"])[classID]), ref) {
		return fmt.Errorf("Learn this spell in the spellbook before preparing it.")
	}
	if contains(current, ref) {
		return nil
	}
	if len(current) >= limit {
		return fmt.Errorf("The class selection limit has been reached (%d).", limit)
	}
	lists[classID] = anyStrings(append(current, ref))
	return nil
}

func castPlaySpell(decisions Object, sheet *character.Calculation, command character.PlayCommand, records Records) error {
	spell, caster := recordByID(records, "spell", command.Ref), playCaster(sheet, command.ClassID)
	if spell == nil || caster == nil {
		return fmt.Errorf("Choose a spellcasting class and a known spell.")
	}
	level := integer(spell["level"], 0)
	selection := character.ListPrepared
	if level == 0 {
		selection = character.ListCantrips
	}
	if !contains(stringsOf(object(decisions[string(selection)])[command.ClassID]), command.Ref) {
		return fmt.Errorf("This spell is not prepared or known by this class.")
	}
	if level == 0 {
		if command.Slot != "" {
			return fmt.Errorf("Cantrips do not spend spell slots.")
		}
		return nil
	}
	return spendSpellSlot(decisions, sheet, records, command.Ref, level, command.Slot)
}
