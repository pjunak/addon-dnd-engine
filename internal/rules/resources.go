package rules

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

var frequencyPattern = regexp.MustCompile(`(?i)(\d+)\s*/\s*(shortOrLong|short|long)`)
var pactSlotsPattern = regexp.MustCompile(`\bpact[- ]slots?\b`)

func hydrateResources(
	decisions Object,
	sheet Object,
	classes []resolvedClass,
	sources []grantSource,
	records Records,
	ruleset Ruleset,
	pb int,
) {
	resources := make([]any, 0)
	abilities := object(sheet["abilities"])
	abilityMod := func(ability string) int {
		return integer(object(abilities[ability])["mod"], 0)
	}
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
		if resource["perLevel"] != nil {
			return max(0, int(math.Floor(number(resource["perLevel"], 0)*float64(level))))
		}
		if truth(resource["proficiencyBonus"]) {
			return pb
		}
		if ability := text(resource["abilityMod"]); ability != "" {
			return max(integer(resource["min"], 1), abilityMod(ability))
		}
		if resource["fixed"] != nil {
			return max(0, integer(resource["fixed"], 0))
		}
		return 0
	}
	normalizeRecharge := func(value any, level int) []any {
		if rest := text(value); rest != "" {
			return []any{Object{"on": rest, "amount": "full"}}
		}
		if values(value) != nil {
			declarations := objects(value)
			result := make([]any, 0, len(declarations))
			for _, declaration := range declarations {
				if text(declaration["on"]) == "" || integer(declaration["minLevel"], 1) > level {
					continue
				}
				amount := declaration["amount"]
				if amount == nil {
					amount = "full"
				}
				result = append(result, Object{"on": text(declaration["on"]), "amount": amount})
			}
			return result
		}
		return []any{Object{"on": "long", "amount": "full"}}
	}

	for _, current := range classes {
		subclass := recordByID(records, "subclass", current.Subclass)
		pools := make([]struct {
			resource Object
			source   Object
		}, 0)
		for _, resource := range objects(current.Record["classResources"]) {
			pools = append(pools, struct {
				resource Object
				source   Object
			}{resource, Object{"type": "class", "id": current.ID, "level": current.Level}})
		}
		for _, resource := range objects(subclass["classResources"]) {
			pools = append(pools, struct {
				resource Object
				source   Object
			}{resource, Object{"type": "subclass", "id": current.Subclass, "level": current.Level}})
		}
		for _, pool := range pools {
			resource := pool.resource
			if text(resource["key"]) == "" || integer(resource["minLevel"], 1) > current.Level {
				continue
			}
			identifier := strings.ToLower(text(resource["key"]) + " " + text(resource["name"]))
			if text(pool.source["type"]) == "class" && classHasPact(sheet, text(pool.source["id"])) &&
				pactSlotsPattern.MatchString(identifier) {
				continue
			}
			maximum := resolveMaximum(resource, current.Level)
			if maximum < 1 {
				continue
			}
			resources = append(resources, Object{
				"key": text(resource["key"]), "name": firstText(resource["name"], resource["key"]),
				"max": maximum, "kind": "pool", "recharge": normalizeRecharge(resource["recharge"], current.Level),
				"source": pool.source,
			})
		}
	}
	for _, source := range sources {
		for _, resource := range objects(source.Grants["resources"]) {
			if text(resource["key"]) == "" || integer(resource["minLevel"], 1) > source.Level {
				continue
			}
			maximum := resolveMaximum(resource, source.Level)
			if maximum < 1 {
				continue
			}
			resources = append(resources, Object{
				"key": text(resource["key"]), "name": firstText(resource["name"], resource["key"]),
				"max": maximum, "kind": "pool", "recharge": normalizeRecharge(resource["recharge"], source.Level),
				"source": source.Source,
			})
		}
	}

	appendHitDiceAndSpellSlots(&resources, sheet, classes, ruleset)
	for _, feat := range selectedFeats(decisions, records) {
		slot := object(object(feat["grants"])["spellSlot"])
		if slot == nil {
			continue
		}
		levelRule := object(slot["level"])
		divisor := max(1, integer(levelRule["divisor"], 1))
		rawLevel := float64(integer(sheet["totalLevel"], 1)) / float64(divisor)
		level := int(math.Ceil(rawLevel))
		if text(levelRule["round"]) == "down" {
			level = int(math.Floor(rawLevel))
		}
		level = max(integer(levelRule["min"], 1), min(integer(levelRule["max"], 9), level))
		resources = append(resources, Object{
			"key":  "feat-slot-" + text(feat["id"]),
			"name": fmt.Sprintf("%s (%s)", firstText(feat["name"], feat["id"]), ordinal(level)),
			"max":  max(1, integer(slot["count"], 1)), "kind": "slot", "level": level,
			"restriction": nullableText(slot["restriction"]),
			"recharge":    normalizeRecharge(slot["recharge"], integer(sheet["totalLevel"], 1)),
			"source":      Object{"type": "feat", "id": text(feat["id"])},
		})
	}
	for _, raw := range values(object(sheet["spellcasting"])["granted"]) {
		grant := object(raw)
		if text(grant["free"]) == "" {
			continue
		}
		maximum, rests := parseFrequency(text(grant["free"]))
		recharge := make([]any, 0, len(rests))
		for _, rest := range rests {
			recharge = append(recharge, Object{"on": rest, "amount": "full"})
		}
		resources = append(resources, Object{
			"key":  "charge-" + firstText(grant["ref"], grant["name"]),
			"name": firstText(grant["name"], grant["ref"]) + " (free cast)",
			"max":  maximum, "kind": "charge", "recharge": recharge,
			"source": firstObject(grant["source"], Object{"type": "spell"}),
		})
	}
	sheet["resources"] = resources
}

func appendHitDiceAndSpellSlots(resources *[]any, sheet Object, classes []resolvedClass, ruleset Ruleset) {
	byDie := make(map[string]int)
	for _, current := range classes {
		if die := text(current.Record["hitDie"]); die != "" {
			byDie[die] += current.Level
		}
	}
	dice := sortedKeys(byDie)
	amount := "full"
	if ruleset.Constants.Rest.LongRestHitDice == "half" {
		amount = "halfLevel"
	}
	for _, die := range dice {
		*resources = append(*resources, Object{
			"key": "hit-dice-" + die, "name": "Hit Dice (" + die + ")", "max": byDie[die],
			"kind": "hitdice", "die": die,
			"recharge": []any{Object{"on": "long", "amount": amount}},
			"source":   Object{"type": "class"},
		})
	}
	for index, count := range integersOf(object(sheet["spellcasting"])["slots"]) {
		if count < 1 {
			continue
		}
		*resources = append(*resources, Object{
			"key": fmt.Sprintf("slot-%d", index+1), "name": "Spell Slots (" + ordinal(index+1) + ")",
			"max": count, "kind": "slot",
			"recharge": []any{Object{"on": "long", "amount": "full"}},
			"source":   Object{"type": "spellcasting"},
		})
	}
	for _, raw := range values(object(sheet["spellcasting"])["perClass"]) {
		entry := object(raw)
		pact := object(entry["pact"])
		if pact == nil || integer(pact["slots"], 0) < 1 {
			continue
		}
		*resources = append(*resources, Object{
			"key": "pact-slot", "name": "Pact Slots (" + ordinal(integer(pact["level"], 0)) + ")",
			"max": integer(pact["slots"], 0), "kind": "slot",
			"recharge": []any{Object{"on": "short", "amount": "full"}},
			"source":   Object{"type": "pactMagic", "id": text(entry["classId"])},
		})
	}
}

func classHasPact(sheet Object, classID string) bool {
	for _, raw := range values(object(sheet["spellcasting"])["perClass"]) {
		entry := object(raw)
		if text(entry["classId"]) == classID && object(entry["pact"]) != nil {
			return true
		}
	}
	return false
}

func parseFrequency(value string) (int, []string) {
	match := frequencyPattern.FindStringSubmatch(value)
	if len(match) != 3 {
		return 1, []string{"long"}
	}
	rests := []string{strings.ToLower(match[2])}
	if rests[0] == "shortorlong" {
		rests = []string{"short", "long"}
	}
	return max(1, integer(match[1], 1)), rests
}

func ordinal(level int) string {
	values := []string{"1st", "2nd", "3rd", "4th", "5th", "6th", "7th", "8th", "9th"}
	if level >= 1 && level <= len(values) {
		return values[level-1]
	}
	return fmt.Sprintf("%dth", level)
}

func sortedKeys(values map[string]int) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func firstObject(values ...any) Object {
	for _, value := range values {
		if current := object(value); current != nil {
			return current
		}
	}
	return Object{}
}
