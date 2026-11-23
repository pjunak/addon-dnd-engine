package rules

import (
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// TemporaryEffects lists the values that temporary play state changes in a
// calculated result: active authored conditions, features the player switched
// on and active DM grants that expire. It calculates the character once
// without any of them (the baseline) and once with each cause alone on top of
// it, so a mark follows every rule that applies a cause, with no bookkeeping.
// A value is marked when it differs from the baseline, in number or in net
// Advantage and Disadvantage, or when a cause alone changes it (an Advantage
// and a Disadvantage that cancel). Its causes are those that change it alone;
// an effect that only appears in combination is attributed by taking each
// cause away from the current state instead. A cause's number is included in
// the shown value unless another cause overrides it (a Speed penalty under
// Speed 0). One cause costs one more calculation, n causes n+1, plus one per
// cause when overlapping numbers need the removal check. A permanent effect
// such as armor's Stealth Disadvantage is in the baseline and marks nothing.
func TemporaryEffects(result character.Result, records Records, profile Ruleset) []character.TemporaryEffect {
	if result.Sheet.Calculation == nil {
		return nil
	}
	causes := temporaryCauses(result)
	if len(causes) == 0 {
		return nil
	}
	calculate := func(input character.Inputs) measuredState {
		evaluated := evaluateCharacter(input, records, profile, true)
		if evaluated.Sheet.Calculation == nil {
			return measuredState{}
		}
		return measuredState{markedValues(evaluated.Sheet), evaluated.Sheet.ConditionEffects.D20Adjustment, true}
	}
	without := func(skip int) character.Inputs {
		input := result.Inputs
		for index, cause := range causes {
			if index != skip {
				input = cause.without(input)
			}
		}
		return input
	}
	current := measuredState{markedValues(result.Sheet), result.Sheet.ConditionEffects.D20Adjustment, true}
	baseline := calculate(without(-1))
	if !baseline.ok {
		return nil
	}
	solos := make([]measuredState, len(causes))
	removals := map[int]measuredState{}
	removal := func(index int) measuredState {
		if len(causes) == 1 {
			return baseline
		}
		if _, done := removals[index]; !done {
			removals[index] = calculate(causes[index].without(result.Inputs))
		}
		return removals[index]
	}
	for index := range causes {
		if len(causes) == 1 {
			solos[index] = current
		} else {
			solos[index] = calculate(without(index))
		}
	}
	list := []character.TemporaryEffect{}
	for _, key := range current.values.order {
		now := current.values.values[key]
		base, exists := baseline.value(key)
		if !exists {
			continue
		}
		type attributed struct {
			index   int
			changes []measuredChange
			shared  bool
		}
		found := []attributed{}
		for index, solo := range solos {
			if value, exists := solo.value(key); solo.ok && exists {
				if changes := valueChanges(value, base, solo.d20-baseline.d20); len(changes) > 0 {
					found = append(found, attributed{index, changes, false})
				}
			}
		}
		total := valueChanges(now, base, current.d20-baseline.d20)
		if len(found) == 0 && len(total) == 0 {
			continue
		}
		if len(found) == 0 {
			// The effect appears only in combination: each cause it depends on.
			for index := range causes {
				if taken := removal(index); taken.ok {
					if value, exists := taken.value(key); exists {
						if changes := valueChanges(now, value, current.d20-taken.d20); len(changes) > 0 {
							found = append(found, attributed{index, changes, false})
						}
					}
				}
			}
		}
		if len(found) == 0 {
			// Nothing singles a cause out: they share the change.
			for index := range causes {
				found = append(found, attributed{index, total, true})
			}
		}
		// A number is in the shown value when the value holds this cause's
		// result, when the causes' numbers add up to the change, or when taking
		// the cause away changes the value by its number.
		sum, sets := 0, false
		for _, entry := range found {
			for _, change := range entry.changes {
				if change.channel == channelNumber && change.Kind == character.ChangeAdd {
					sum += *change.Value
				}
				sets = sets || change.Kind == character.ChangeSet
			}
		}
		adds := !sets && now.number != nil && *now.number-*base.number == sum
		effect := character.TemporaryEffect{Path: key.path, Row: key.row, Causes: []character.EffectCause{}}
		tones := []character.EffectTone{}
		for _, entry := range found {
			solo, _ := solos[entry.index].value(key)
			changes, causeTones := []character.EffectChange{}, []character.EffectTone{}
			for _, change := range entry.changes {
				switch {
				case entry.shared:
				case change.Kind == character.ChangeSet:
					change.Included = boolPointer(now.number != nil && *now.number == *change.Value)
				case change.channel == channelNumber:
					included := solo.number != nil && *now.number == *solo.number || adds
					if !included {
						taken, exists := removal(entry.index).value(key)
						included = exists && taken.number != nil && *now.number-*taken.number == *change.Value
					}
					change.Included = boolPointer(included)
				case change.channel == channelDamage && change.Kind == character.ChangeAdd:
					included := solo.damage != nil && *now.damage == *solo.damage
					if !included {
						taken, exists := removal(entry.index).value(key)
						included = exists && taken.damage != nil && *now.damage != *taken.damage
					}
					change.Included = boolPointer(included)
				}
				changes = append(changes, change.EffectChange)
				causeTones = append(causeTones, change.tone)
			}
			cause := causes[entry.index].cause
			cause.Changes, cause.Tone = changes, combinedTone(causeTones)
			effect.Causes = append(effect.Causes, cause)
			tones = append(tones, cause.Tone)
		}
		effect.Tone = combinedTone(tones)
		list = append(list, effect)
	}
	return list
}

// measuredState is a calculation's marked values and its condition D20
// adjustment; ok is false when the character could not be calculated.
type measuredState struct {
	values markedSet
	d20    int
	ok     bool
}

// value reads a marked value; a sense the state lacks has range 0.
func (state measuredState) value(key valueKey) (markedValue, bool) {
	if !state.ok {
		return markedValue{}, false
	}
	if value, exists := state.values.values[key]; exists {
		return value, true
	}
	if strings.HasPrefix(key.path, "senses.") {
		zero := 0
		return markedValue{number: &zero}, true
	}
	return markedValue{}, false
}

func boolPointer(value bool) *bool { return &value }

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

// measuredChange is a change with its tone and the part of the value it
// comes from: the number, the roll marks, the D20 adjustment or the damage.
type measuredChange struct {
	character.EffectChange
	tone    character.EffectTone
	channel changeChannel
}

type changeChannel int

const (
	channelNumber changeChannel = iota
	channelRoll
	channelD20
	channelDamage
)

// valueChanges compares a value with a cause and without it. d20 is the
// cause's share of the condition D20 adjustment, which no value includes.
// Numbers start as included; TemporaryEffects corrects that when causes
// overlap.
func valueChanges(current, previous markedValue, d20 int) []measuredChange {
	changes := []measuredChange{}
	change := func(channel changeChannel, kind character.ChangeKind, tone character.EffectTone, value *int, included *bool) {
		changes = append(changes, measuredChange{character.EffectChange{Kind: kind, Value: value, Included: included}, tone, channel})
	}
	toneOf := func(delta int) character.EffectTone {
		if delta < 0 {
			return character.TonePenalty
		}
		return character.ToneBonus
	}
	if current.number != nil && previous.number != nil && *current.number != *previous.number {
		delta := *current.number - *previous.number
		if current.speed && *current.number == 0 {
			change(channelNumber, character.ChangeSet, toneOf(delta), intPointer(0), boolPointer(true))
		} else {
			change(channelNumber, character.ChangeAdd, toneOf(delta), &delta, boolPointer(true))
		}
	}
	if current.roll > previous.roll {
		change(channelRoll, character.ChangeAdvantage, character.ToneBonus, nil, nil)
	} else if current.roll < previous.roll {
		change(channelRoll, character.ChangeDisadvantage, character.TonePenalty, nil, nil)
	}
	if current.d20 && d20 != 0 {
		change(channelD20, character.ChangeAdd, toneOf(d20), intPointer(d20), boolPointer(false))
	}
	if current.damage != nil && previous.damage != nil && *current.damage != *previous.damage {
		now, nowOK := parseDamage(*current.damage)
		before, beforeOK := parseDamage(*previous.damage)
		switch {
		case nowOK && beforeOK && now.dice == before.dice:
			delta := now.flat - before.flat
			change(channelDamage, character.ChangeAdd, toneOf(delta), &delta, boolPointer(true))
		case nowOK && beforeOK && now.average() > before.average():
			change(channelDamage, character.ChangeOther, character.ToneBonus, nil, nil)
		case nowOK && beforeOK && now.average() < before.average():
			change(channelDamage, character.ChangeOther, character.TonePenalty, nil, nil)
		default:
			change(channelDamage, character.ChangeOther, character.ToneMixed, nil, nil)
		}
	}
	return changes
}

func intPointer(value int) *int { return &value }

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
