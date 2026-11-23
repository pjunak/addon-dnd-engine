package rules

import (
	"regexp"
	"slices"
	"sort"
	"strconv"

	"github.com/pjunak/addon-dnd-engine/character"
)

// TemporaryEffects lists the values that temporary play state changes in a
// calculated result: active authored conditions, features the player switched
// on and active DM grants that expire. Each cause is measured by calculating the
// same inputs again without it and comparing the marked values, so the marks
// follow every rule without bookkeeping where the rule applies. A character
// without such a cause costs nothing more. A cause whose removal leaves the
// character uncalculated, or whose effect another cause hides (Speed already
// 0), marks nothing for that value.
func TemporaryEffects(result character.Result, records Records, profile Ruleset) []character.TemporaryEffect {
	if result.Sheet.Calculation == nil {
		return nil
	}
	causes := temporaryCauses(result)
	if len(causes) == 0 {
		return nil
	}
	current := markedValues(result.Sheet)
	effects := map[valueKey]*character.TemporaryEffect{}
	for _, cause := range causes {
		without := evaluateCharacter(cause.without(result.Inputs), records, profile, true)
		if without.Sheet.Calculation == nil {
			continue
		}
		previous := markedValues(without.Sheet)
		d20 := result.Sheet.ConditionEffects.D20Adjustment - without.Sheet.ConditionEffects.D20Adjustment
		for _, key := range current.order {
			changes, tones := valueChanges(current.values[key], previous.values[key], d20)
			if len(changes) == 0 {
				continue
			}
			effect := effects[key]
			if effect == nil {
				effect = &character.TemporaryEffect{Path: key.path, Row: key.row, Causes: []character.EffectCause{}}
				effects[key] = effect
			}
			marked := cause.cause
			marked.Changes, marked.Tone = changes, combinedTone(tones)
			effect.Causes = append(effect.Causes, marked)
		}
	}
	list := []character.TemporaryEffect{}
	for _, key := range current.order {
		if effect := effects[key]; effect != nil {
			tones := []character.EffectTone{}
			for _, cause := range effect.Causes {
				tones = append(tones, cause.Tone)
			}
			effect.Tone = combinedTone(tones)
			list = append(list, *effect)
		}
	}
	return list
}

type temporaryCause struct {
	cause   character.EffectCause
	without func(character.Inputs) character.Inputs
}

// temporaryCauses lists the causes in a fixed order: authored conditions as
// entered, switched-on features by key, then timed grants as entered.
// Conditions an item grants have no automated effect, a condition the
// character is immune to is not active, and an expired grant no longer
// applies, so none of them is a cause.
func temporaryCauses(result character.Result) []temporaryCause {
	causes := []temporaryCause{}
	levelled := map[string]bool{}
	for _, option := range result.Guidance.Conditions.Options {
		levelled[option.ID] = option.MaximumLevel > 1
	}
	for _, row := range result.Sheet.Conditions {
		if row.Granted || row.Status != character.ConditionActive {
			continue
		}
		cause := character.EffectCause{Kind: character.CauseCondition, ID: row.ID, Name: row.Name, LabelKey: row.LabelKey}
		if levelled[row.ID] {
			cause.Level = row.Level
		}
		id := row.ID
		causes = append(causes, temporaryCause{cause, func(input character.Inputs) character.Inputs {
			input.Play.Conditions = slices.DeleteFunc(slices.Clone(input.Play.Conditions), func(condition character.Condition) bool { return condition.ID == id })
			return input
		}})
	}
	activations := slices.Clone(result.Sheet.Activations)
	sort.SliceStable(activations, func(i, j int) bool { return activations[i].Key < activations[j].Key })
	for _, activation := range activations {
		if !activation.Active {
			continue
		}
		key := activation.Key
		causes = append(causes, temporaryCause{character.EffectCause{Kind: character.CauseActiveFeature, ID: key, Name: firstText(activation.Name, activation.ID)}, func(input character.Inputs) character.Inputs {
			features := make(map[string]bool, len(input.Play.ActiveFeatures))
			for feature, on := range input.Play.ActiveFeatures {
				if feature != key {
					features[feature] = on
				}
			}
			input.Play.ActiveFeatures = features
			return input
		}})
	}
	for index, grant := range result.Inputs.Grants {
		if grant.ExpiresAt == "" || !characterGrantActive(grant, result.Inputs) {
			continue
		}
		causes = append(causes, temporaryCause{character.EffectCause{Kind: character.CauseTimedGrant, ID: grant.ID, Name: firstText(grant.Name, grant.ID)}, func(input character.Inputs) character.Inputs {
			input.Grants = slices.Clone(input.Grants)
			input.Grants[index].Active = false
			return input
		}})
	}
	return causes
}

type valueKey struct{ path, row string }

// markedValue is one value a temporary effect can mark: its number, the net
// Advantage (+1) or Disadvantage (−1) on its roll, whether it is a D20 Test
// (the condition D20 adjustment applies to it) and, for a damage roll, its text.
type markedValue struct {
	number *int
	roll   int
	d20    bool
	speed  bool
	damage *string
}

type markedSet struct {
	order  []valueKey
	values map[valueKey]markedValue
}

// markedValues reads the values a temporary effect can mark, in display order.
func markedValues(sheet character.Sheet) markedSet {
	set := markedSet{values: map[valueKey]markedValue{}}
	add := func(path, row string, value markedValue) {
		key := valueKey{path, row}
		if _, exists := set.values[key]; exists {
			return
		}
		set.order = append(set.order, key)
		set.values[key] = value
	}
	number := func(value int) *int { return &value }
	roll := func(advantage, disadvantage bool) int {
		switch {
		case advantage && !disadvantage:
			return 1
		case disadvantage && !advantage:
			return -1
		}
		return 0
	}
	calculation := sheet.Calculation
	marks := calculation.RollMarks
	add("derived.armorClass", "", markedValue{number: number(calculation.Derived.ArmorClass)})
	add("derived.speed", "", markedValue{number: number(calculation.Derived.Speed), speed: true})
	for _, speed := range []struct {
		path  string
		value int
	}{{"flySpeed", calculation.FlySpeed}, {"swimSpeed", calculation.SwimSpeed}, {"climbSpeed", calculation.ClimbSpeed}, {"burrowSpeed", calculation.BurrowSpeed}} {
		add(speed.path, "", markedValue{number: number(speed.value), speed: true})
	}
	add("derived.initiative", "", markedValue{number: number(calculation.Derived.Initiative), roll: roll(marks.InitiativeAdvantage, marks.InitiativeDisadvantage), d20: true})
	add("derived.maxHp", "", markedValue{number: number(calculation.Derived.MaxHP)})
	add("derived.passivePerception", "", markedValue{number: number(calculation.Derived.PassivePerception)})
	for _, ability := range Abilities {
		row, exists := calculation.Abilities[ability]
		if !exists {
			continue
		}
		add("abilities."+ability+".score", "", markedValue{number: number(row.Score)})
		add("abilities."+ability+".mod", "", markedValue{number: number(row.Mod), roll: roll(row.CheckAdvantage, row.CheckDisadvantage), d20: true})
	}
	for _, ability := range Abilities {
		if row, exists := calculation.Saves[ability]; exists {
			add("saves."+ability+".total", "", markedValue{number: number(row.Total), roll: roll(row.Advantage, row.Disadvantage), d20: true})
		}
	}
	for _, id := range sortedKeys(calculation.Skills) {
		row := calculation.Skills[id]
		add("skills."+id+".total", "", markedValue{number: number(row.Total), roll: roll(row.Advantage, row.Disadvantage), d20: true})
	}
	for _, id := range sortedKeys(calculation.Senses) {
		add("senses."+id, "", markedValue{number: number(calculation.Senses[id])})
	}
	for _, attack := range calculation.Weapons {
		key := character.AttackKey(attack)
		add("weapons.attackBonus", key, markedValue{number: number(attack.AttackBonus), roll: roll(marks.AttackAdvantage, marks.AttackDisadvantage), d20: true})
		damage := attack.Damage
		add("weapons.damage", key, markedValue{damage: &damage})
	}
	for _, caster := range calculation.Spellcasting.PerClass {
		add("spellcasting.perClass.spellAttack", caster.ClassID, markedValue{number: number(caster.SpellAttack), roll: roll(marks.AttackAdvantage, marks.AttackDisadvantage), d20: true})
		add("spellcasting.perClass.saveDC", caster.ClassID, markedValue{number: number(caster.SaveDC)})
	}
	if sheet.Concentration != nil && sheet.Concentration.SaveBonus != nil {
		add("concentration.saveBonus", "", markedValue{number: number(*sheet.Concentration.SaveBonus), roll: roll(marks.ConcentrationAdvantage, marks.ConcentrationDisadvantage), d20: true})
	}
	return set
}

// valueChanges compares a value with and without one cause. d20 is the
// cause's share of the condition D20 adjustment, which no value includes.
func valueChanges(current, previous markedValue, d20 int) ([]character.EffectChange, []character.EffectTone) {
	changes, tones := []character.EffectChange{}, []character.EffectTone{}
	change := func(kind character.ChangeKind, tone character.EffectTone, value *int, included *bool) {
		changes = append(changes, character.EffectChange{Kind: kind, Value: value, Included: included})
		tones = append(tones, tone)
	}
	included, separate := true, false
	if current.number != nil && previous.number != nil && *current.number != *previous.number {
		delta := *current.number - *previous.number
		tone := character.ToneBonus
		if delta < 0 {
			tone = character.TonePenalty
		}
		if current.speed && *current.number == 0 {
			zero := 0
			change(character.ChangeSet, tone, &zero, &included)
		} else {
			change(character.ChangeAdd, tone, &delta, &included)
		}
	}
	if current.roll > previous.roll {
		change(character.ChangeAdvantage, character.ToneBonus, nil, nil)
	} else if current.roll < previous.roll {
		change(character.ChangeDisadvantage, character.TonePenalty, nil, nil)
	}
	if current.d20 && d20 != 0 {
		tone := character.ToneBonus
		if d20 < 0 {
			tone = character.TonePenalty
		}
		value := d20
		change(character.ChangeAdd, tone, &value, &separate)
	}
	if current.damage != nil && previous.damage != nil && *current.damage != *previous.damage {
		now, nowOK := parseDamage(*current.damage)
		before, beforeOK := parseDamage(*previous.damage)
		switch {
		case nowOK && beforeOK && now.dice == before.dice:
			delta := now.flat - before.flat
			tone := character.ToneBonus
			if delta < 0 {
				tone = character.TonePenalty
			}
			change(character.ChangeAdd, tone, &delta, &included)
		case nowOK && beforeOK && now.average() > before.average():
			change(character.ChangeOther, character.ToneBonus, nil, nil)
		case nowOK && beforeOK && now.average() < before.average():
			change(character.ChangeOther, character.TonePenalty, nil, nil)
		default:
			change(character.ChangeOther, character.ToneMixed, nil, nil)
		}
	}
	return changes, tones
}

// combinedTone is the tone of several changes or causes: their shared tone,
// or mixed when they differ.
func combinedTone(tones []character.EffectTone) character.EffectTone {
	if len(tones) == 0 {
		return character.ToneMixed
	}
	for _, tone := range tones[1:] {
		if tone != tones[0] {
			return character.ToneMixed
		}
	}
	return tones[0]
}

// damageRoll is a damage text the attack rows write: a die such as "1d8"
// with an optional signed flat part ("1d8 +4"), or a flat number.
type damageRoll struct {
	dice        string
	count, side int
	flat        int
}

func (roll damageRoll) average() float64 {
	return float64(roll.count)*float64(roll.side+1)/2 + float64(roll.flat)
}

var damagePattern = regexp.MustCompile(`^(?:(\d+)d(\d+))?(?:\s*([+-]?\d+))?$`)

func parseDamage(text string) (damageRoll, bool) {
	match := damagePattern.FindStringSubmatch(text)
	if match == nil || match[1] == "" && match[3] == "" {
		return damageRoll{}, false
	}
	roll := damageRoll{}
	if match[1] != "" {
		roll.count, _ = strconv.Atoi(match[1])
		roll.side, _ = strconv.Atoi(match[2])
		roll.dice = match[1] + "d" + match[2]
	}
	if match[3] != "" {
		roll.flat, _ = strconv.Atoi(match[3])
	}
	return roll, true
}
