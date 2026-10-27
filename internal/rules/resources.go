package rules

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

var frequencyPattern = regexp.MustCompile(`(?i)(\d+)\s*/\s*(shortOrLong|short|long)`)

func hydrateResources(
	sheet *character.Calculation,
	classes []resolvedClass,
	sources []grantSource,
	records Records,
	ruleset Ruleset,
	pb int,
) {
	resources := make([]character.Resource, 0)
	resolveMaximum := func(resource Object, level int) int {
		if progression := objects(resource["progression"]); len(progression) > 0 {
			maximum := 0
			for _, row := range progression {
				if integer(row["level"], 1) <= level {
					maximum = integer(row["max"], maximum)
				}
			}
			return maximum
		}
		// The declared parts add up: perLevel (times the level, rounded down),
		// the proficiency bonus, an ability modifier (times multiplier), fixed
		// and add. An ability-based pool has at least min, which defaults to 1.
		total, minimum := 0, 0
		if resource["perLevel"] != nil {
			total += int(math.Floor(number(resource["perLevel"], 0) * float64(level)))
		}
		if truth(resource["proficiencyBonus"]) {
			total += pb
		}
		if ability := text(resource["abilityMod"]); ability != "" {
			total += sheet.Abilities[ability].Mod * max(1, integer(resource["multiplier"], 1))
			minimum = 1
		}
		if resource["fixed"] != nil {
			total += integer(resource["fixed"], 0)
		}
		total += integer(resource["add"], 0)
		return max(integer(resource["min"], minimum), total)
	}
	pool := func(resource Object, key string, level int, source character.Source) character.Resource {
		return character.Resource{
			Key: key, Name: firstText(resource["name"], resource["key"]),
			Max: resolveMaximum(resource, level), Kind: character.ResourcePool, Recharge: normalizeRecharge(resource["recharge"], level),
			Source: source,
		}
	}

	for _, current := range classes {
		subclass := recordByID(records, "subclass", current.Subclass)
		type declared struct {
			resource Object
			source   character.Source
		}
		pools := make([]declared, 0)
		for _, resource := range objects(current.Record["classResources"]) {
			pools = append(pools, declared{resource, character.Source{Type: character.SourceClass, ID: current.ID, Level: current.Level}})
		}
		for _, resource := range objects(subclass["classResources"]) {
			pools = append(pools, declared{resource, character.Source{Type: character.SourceSubclass, ID: current.Subclass, Level: current.Level}})
		}
		for _, entry := range pools {
			if text(entry.resource["key"]) == "" || integer(entry.resource["minLevel"], 1) > current.Level {
				continue
			}
			if row := pool(entry.resource, text(entry.resource["key"]), current.Level, entry.source); row.Max >= 1 {
				resources = append(resources, row)
			}
		}
	}
	for _, source := range sources {
		for _, resource := range objects(source.Grants["resources"]) {
			if text(resource["key"]) == "" || integer(resource["minLevel"], 1) > source.Level {
				continue
			}
			row := pool(resource, grantResourceKey(source.Source, text(resource["key"])), source.Level, source.Source)
			if row.Max < 1 {
				continue
			}
			if source.Inactive {
				inactive := false
				row.Active = &inactive
			}
			resources = append(resources, row)
		}
	}

	appendHitDiceAndSpellSlots(&resources, sheet, classes, ruleset)
	for _, source := range sources {
		if source.Source.Type != character.SourceFeat {
			continue
		}
		feat := source.Record
		slot := object(source.Grants["spellSlot"])
		if slot == nil {
			continue
		}
		levelRule := object(slot["level"])
		divisor := max(1, integer(levelRule["divisor"], 1))
		rawLevel := float64(sheet.TotalLevel) / float64(divisor)
		level := int(math.Ceil(rawLevel))
		if text(levelRule["round"]) == "down" {
			level = int(math.Floor(rawLevel))
		}
		level = max(integer(levelRule["min"], 1), min(integer(levelRule["max"], 9), level))
		resources = append(resources, character.Resource{
			Key:  grantResourceKey(source.Source, "feat-slot-"+text(feat["id"])),
			Name: fmt.Sprintf("%s (%s)", firstText(feat["name"], feat["id"]), ordinal(level)),
			Max:  max(1, integer(slot["count"], 1)), Kind: character.ResourceSlot, Level: level,
			Restriction: text(slot["restriction"]),
			Recharge:    normalizeRecharge(slot["recharge"], sheet.TotalLevel),
			Source:      source.Source,
		})
	}
	for _, grant := range sheet.Spellcasting.Granted {
		if grant.Free == nil || *grant.Free == "" {
			continue
		}
		maximum, rests := parseFrequency(*grant.Free)
		resources = append(resources, character.Resource{
			Key:  grantFreeResourceKey(grant),
			Name: firstText(grant.Name, grant.Ref) + " (free cast)",
			Max:  maximum, Kind: character.ResourceCharge, Recharge: fullRecharge(rests...),
			Source: grant.Source,
		})
	}
	// The allowance belongs to the granting choice even while its spell is
	// cleared or awaiting a replacement. Selection changes cannot refresh it.
	for _, choice := range sheet.Spellcasting.PendingChoices {
		if choice.Free == "" || choice.Choose != 1 {
			continue
		}
		key := "charge:" + choice.Key
		exists := false
		for _, resource := range resources {
			exists = exists || resource.Key == key
		}
		if exists {
			continue
		}
		maximum, rests := parseFrequency(choice.Free)
		resources = append(resources, character.Resource{Key: key, Name: grantSourceLabel(choice.Source, records) + " (free cast)", Max: maximum, Kind: character.ResourceCharge, Recharge: fullRecharge(rests...), Source: choice.Source})
	}
	sheet.Resources = resources
}

// normalizeRecharge reads a resource's recharge: a rest name, a list of
// {on, amount?, minLevel?} or nothing (a long rest). A declaration with an
// unknown rest never recharges, so it is left out.
func normalizeRecharge(value any, level int) []character.Recharge {
	if rest := text(value); rest != "" {
		return fullRecharge(character.RechargeOn(rest))
	}
	if values(value) != nil {
		result := []character.Recharge{}
		for _, declaration := range objects(value) {
			on := character.RechargeOn(text(declaration["on"]))
			if !on.Valid() || integer(declaration["minLevel"], 1) > level {
				continue
			}
			amount := character.RechargeAmount{Full: true}
			if declaration["amount"] != nil {
				amount = rechargeAmountOf(declaration["amount"])
			}
			result = append(result, character.Recharge{On: on, Amount: amount})
		}
		return result
	}
	return fullRecharge(character.RechargeLong)
}

func fullRecharge(rests ...character.RechargeOn) []character.Recharge {
	result := []character.Recharge{}
	for _, rest := range rests {
		if rest.Valid() {
			result = append(result, character.Recharge{On: rest, Amount: character.RechargeAmount{Full: true}})
		}
	}
	return result
}

// rechargeAmountOf reads a declared amount: "full", "halfLevel", a number of
// uses or {abilityMod}. Anything else restores nothing.
func rechargeAmountOf(value any) character.RechargeAmount {
	switch text(value) {
	case "full":
		return character.RechargeAmount{Full: true}
	case "halfLevel":
		return character.RechargeAmount{HalfLevel: true}
	}
	if ability := text(object(value)["abilityMod"]); ability != "" {
		return character.RechargeAmount{Ability: ability}
	}
	return character.RechargeAmount{Uses: max(0, integer(value, 0))}
}

func appendHitDiceAndSpellSlots(resources *[]character.Resource, sheet *character.Calculation, classes []resolvedClass, ruleset Ruleset) {
	byDie := make(map[string]int)
	dice := make([]string, 0)
	for _, current := range classes {
		if die := text(current.Record["hitDie"]); die != "" {
			if _, exists := byDie[die]; !exists {
				dice = append(dice, die)
			}
			byDie[die] += current.Level
		}
	}
	amount := character.RechargeAmount{Full: true}
	if ruleset.Constants.Rest.LongRestHitDice == "half" {
		amount = character.RechargeAmount{HalfLevel: true}
	}
	for _, die := range dice {
		*resources = append(*resources, character.Resource{
			Key: "hit-dice-" + die, Name: "Hit Dice (" + die + ")", Max: byDie[die],
			Kind: character.ResourceHitDice, Die: die,
			Recharge: []character.Recharge{{On: character.RechargeLong, Amount: amount}},
			Source:   character.Source{Type: character.SourceClass},
		})
	}
	for index, count := range sheet.Spellcasting.Slots {
		if count < 1 {
			continue
		}
		*resources = append(*resources, character.Resource{
			Key: fmt.Sprintf("slot-%d", index+1), Name: "Spell Slots (" + ordinal(index+1) + ")",
			Max: count, Kind: character.ResourceSlot, Level: index + 1,
			Recharge: fullRecharge(character.RechargeLong),
			Source:   character.Source{Type: character.SourceSpellcasting},
		})
	}
	for _, entry := range sheet.Spellcasting.PerClass {
		pact := entry.Pact
		if pact == nil || pact.Slots < 1 {
			continue
		}
		*resources = append(*resources, character.Resource{
			Key: "pact-slot", Name: "Pact Slots (" + ordinal(pact.Level) + ")",
			Max: pact.Slots, Kind: character.ResourceSlot, Level: pact.Level,
			Recharge: fullRecharge(character.RechargeShort),
			Source:   character.Source{Type: character.SourcePactMagic, ID: entry.ClassID},
		})
	}
}

func parseFrequency(value string) (int, []character.RechargeOn) {
	match := frequencyPattern.FindStringSubmatch(value)
	if len(match) != 3 {
		return 1, []character.RechargeOn{character.RechargeLong}
	}
	rests := []character.RechargeOn{character.RechargeOn(strings.ToLower(match[2]))}
	if rests[0] == "shortorlong" {
		rests = []character.RechargeOn{character.RechargeShort, character.RechargeLong}
	}
	count, _ := strconv.Atoi(match[1])
	return max(1, count), rests
}

func ordinal(level int) string {
	values := []string{"1st", "2nd", "3rd", "4th", "5th", "6th", "7th", "8th", "9th"}
	if level >= 1 && level <= len(values) {
		return values[level-1]
	}
	return fmt.Sprintf("%dth", level)
}
