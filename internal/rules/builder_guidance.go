package rules

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/pjunak/addon-dnd-engine/character"
)

// BuilderGuidance is a read-only companion to the stable Builder plan. It keeps
// completion and option policy out of UI consumers without changing decisions.
func BuilderGuidance(decisions Object, plan character.Plan, records Records, profile Ruleset) character.Guidance {
	return builderGuidance(decisions, plan, records, profile, nil)
}

func builderGuidance(decisions Object, plan character.Plan, records Records, profile Ruleset, choiceOptions map[string][]character.Option) character.Guidance {
	normalized := NormalizeBuilderDecisions(decisions, records, profile)
	hydrated := Hydrate(normalized, records, profile)
	choices, classes := map[string]character.ChoiceGuidance{}, []character.ClassGuidance{}
	sections := []character.GuidanceSection{{ID: "foundation", Issues: []character.GuidanceIssue{}}, {ID: "progression", Issues: []character.GuidanceIssue{}}, {ID: "spells", Issues: []character.GuidanceIssue{}}}
	// add counts one decision and lists it as an issue unless done; the
	// returned issue is valid until the next call.
	add := func(group int, done bool, id, label, tab string, level int, args ...any) *character.GuidanceIssue {
		section := &sections[group]
		section.Total++
		if done {
			section.Complete++
			return nil
		}
		section.Issues = append(section.Issues, character.GuidanceIssue{ID: id, Text: guidanceText(label, args...), Tab: tab, Level: level})
		return &section.Issues[len(section.Issues)-1]
	}
	// An item's picks are ordinary steps, but they guide play rather than
	// block it, so equipping an item never stops the sheet.
	itemPick := func(issue *character.GuidanceIssue, source character.Source) {
		if issue != nil && source.Type == character.SourceItem {
			issue.Severity = character.SeverityWarning
		}
	}
	manual := truth(decisions["manualScores"])
	scoresValid, spent := true, 0
	for _, ability := range Abilities {
		score := plan.BaseStats[ability]
		if manual {
			scoresValid = scoresValid && score >= 1 && score <= profile.Constants.AbilityCapHard
		} else {
			cost, exists := profile.Constants.PointBuy.Cost[jsonNumber(score)]
			spent += cost
			scoresValid = scoresValid && exists
		}
	}
	add(0, scoresValid && (manual || spent == profile.Constants.PointBuy.Budget), "abilities", "Finish base ability scores", "character", 0)
	species := recordByID(records, "species", text(decisions["species"]))
	add(0, species != nil, "species", "Choose species", "character", 0)
	add(0, recordByID(records, "background", text(decisions["background"])) != nil, "background", "Choose background", "character", 0)
	if lineages := objects(species["lineages"]); len(lineages) > 0 {
		valid := false
		for _, lineage := range lineages {
			valid = valid || text(lineage["id"]) == text(decisions["lineage"])
		}
		add(0, valid, "lineage", "Choose lineage", "character", 0)
	}
	for groupIndex, group := range [][]character.PlanChoice{plan.CreationChoices, plan.CreationAbilityChoices, plan.ClassChoices} {
		for _, choice := range group {
			entry := builderChoiceGuidance(decisions, choice, hydrated.Sheet, records, choiceOptions[choice.ID])
			choices[choice.ID] = entry
			section, tab := 0, "character"
			if groupIndex == 2 {
				section, tab = 1, choice.ClassID
			}
			add(section, entry.Done, choice.ID, entry.LabelKey, tab, choiceSourceLevel(choice), labelArgs(entry.LabelArgs)...)
		}
	}
	for _, selected := range plan.Classes {
		id := selected.ClassID
		if id == "" {
			add(1, false, "add-class", "Choose first class", "add-class", 0)
			continue
		}
		class := recordByID(records, "class", id)
		add(1, class != nil, "class:"+id, "Review {0} levels", "levels", 0, firstText(class["name"], id))
		if class == nil {
			continue
		}
		subclasses := []character.Option{}
		for _, sub := range recordCatalog(records, "subclass") {
			if text(sub["classId"]) == id {
				subclasses = append(subclasses, guidanceOption(text(sub["id"]), sub))
			}
		}
		sortGuidanceOptions(subclasses)
		level, subclassLevel := max(1, selected.Level), classSubclassLevel(class)
		if subclassLevel > 0 && level >= subclassLevel && len(subclasses) > 0 {
			valid := false
			for _, sub := range subclasses {
				valid = valid || sub.ID == selected.Subclass
			}
			add(1, valid, "subclass:"+id, "Choose {0} subclass", id, subclassLevel, firstText(class["name"], id))
		}
		levels := []character.LevelGuidance{}
		for at := 1; at <= min(level, 20); at++ {
			features := []character.Option{}
			for _, feature := range hydrated.Sheet.Features {
				if feature.Source.Level == at && (feature.Source.ID == id || feature.Source.ID == selected.Subclass) {
					record := recordByID(records, "feature", feature.ID)
					if record == nil {
						record = Object{"name": firstText(feature.Name, feature.ID)}
					}
					features = append(features, guidanceOption(feature.ID, record))
				}
			}
			sortGuidanceOptions(features)
			levels = append(levels, character.LevelGuidance{Level: at, Features: features})
		}
		classes = append(classes, character.ClassGuidance{ClassID: id, Name: firstText(class["name"], id), Level: level, SubclassLevel: subclassLevel, Subclasses: subclasses, Levels: levels})
	}
	casting := hydrated.Sheet.Spellcasting
	for _, caster := range casting.PerClass {
		id := caster.ClassID
		name := firstText(recordByID(records, "class", id)["name"], id)
		if required := caster.CantripsKnown; required > 0 {
			add(2, len(unique(stringsOf(object(decisions["cantrips"])[id]))) == required, "cantrips:"+id, "Choose {0} cantrips", "spells", 0, name)
		}
		if caster.Prepares == "spellbook" && caster.SpellbookKnown > 0 {
			spellbook := unique(stringsOf(object(decisions["spellbook"])[id]))
			done := len(spellbook) >= caster.SpellbookKnown && len(spellbookSchoolShortfalls(caster, spellbook, records)) == 0
			add(2, done, "spellbook:"+id, "Choose {0} spellbook spells", "spells", 0, name)
		}
	}
	for _, choice := range casting.PendingChoices {
		itemPick(add(2, len(unique(choice.Picked)) >= max(1, choice.Choose), choice.Key, "Choose spells: {0}", "spells", 0, grantSourceLabel(choice.Source, records)), choice.Source)
	}
	for _, choice := range casting.CastingAbilityChoices {
		itemPick(add(2, choice.Selected != nil, choice.Key, "Choose casting ability: {0}", "spells", 0, grantSourceLabel(choice.Source, records)), choice.Source)
	}
	total, complete := 0, 0
	for _, section := range sections {
		total += section.Total
		complete += section.Complete
	}
	return character.Guidance{Choices: choices, Classes: classes, Sections: sections, Total: total, Complete: complete, Ready: total == complete, Derived: hydrated.Sheet.Derived, Warnings: hydrated.Warnings}
}

// choiceSourceLevel is the level a choice's source grants it at, 1 when unknown.
func choiceSourceLevel(choice character.PlanChoice) int {
	if choice.Source != nil && choice.Source.Level != 0 {
		return choice.Source.Level
	}
	return 1
}

func labelArgs(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func builderChoiceGuidance(decisions Object, choice character.PlanChoice, sheet *character.Calculation, records Records, options []character.Option) character.ChoiceGuidance {
	label, args := choiceHeading(choice, records)
	if acquired := choice.Acquisition; acquired != nil {
		label = "{0} · {1} · {2}"
		args = []any{choice.Name, choice.ChoiceName, acquired.Name}
		if choice.ChoiceName == "" {
			label = "{0} · {1}"
			args = []any{choice.Name, acquired.Name}
		}
		if acquired.ClassID != "" {
			label = "{0} · {1} level {2}"
			if len(args) == 3 {
				label = "{0} · {1} · {2} level {3}"
			}
			args = append(args, acquired.Level)
		}
	}
	if choice.Kind == character.ChoiceASIMode {
		label = "{0} level {1} advancement"
		args = []any{firstText(recordView(records, "class", choice.ClassID)["name"], guidanceLabel(choice.ClassID)), max(1, choice.Level)}
	}
	if choice.Kind == character.ChoiceAbilityBudget {
		label = "Assign origin ability points"
	}
	if options == nil {
		options = builderChoiceOptions(choice, sheet, records)
	}
	entry := character.ChoiceGuidance{ID: choice.ID, Text: guidanceText(label, args...), Options: options, Required: max(1, choice.Count)}
	switch choice.Kind {
	case character.ChoiceAbilityBudget:
		entry.Picked, entry.Required, entry.Done = guidanceAbility(decisions, choice)
		return entry
	case character.ChoiceASIMode:
		mode := text(object(decisions["featureChoices"])[choice.ID])
		if mode == "asi" {
			entry.Picked, entry.Required, entry.Done = guidanceAbility(decisions, *choice.Ability)
		}
		feat := choice.Feat.Choice()
		entry.FeatOptions = builderChoiceOptions(feat, sheet, records)
		if mode == "feat" {
			selected := text(object(decisions["featureChoices"])[feat.ID])
			valid := false
			for _, option := range entry.FeatOptions {
				valid = valid || option.ID == selected
			}
			if valid {
				entry.Picked, entry.Done = 1, true
				if descriptor := choice.Feat.Ability; descriptor != nil && len(descriptor.Eligible) > 0 {
					entry.Picked, entry.Required, entry.Done = guidanceAbility(decisions, *descriptor)
				}
			}
		}
		return entry
	case character.ChoiceSkills, character.ChoiceExpertise, character.ChoiceSkillExpertise, character.ChoiceSkillUpgrade,
		character.ChoiceTools, character.ChoiceProficiencies, character.ChoiceWeaponMastery, character.ChoiceLanguages,
		character.ChoiceSavingThrows, character.ChoiceWeapons, character.ChoiceArmor, character.ChoiceResistances,
		character.ChoiceImmunities, character.ChoiceFeat, character.ChoiceEnumerated, character.ChoiceSize:
	}
	valid := map[string]bool{}
	for _, option := range options {
		valid[option.ID] = true
	}
	for _, value := range choiceValues(decisions, choice) {
		if valid[value] {
			entry.Picked++
		}
	}
	entry.Done = entry.Picked >= entry.Required
	return entry
}

func guidanceAbility(decisions Object, descriptor character.PlanChoice) (int, int, bool) {
	budget, total, valid := max(1, descriptor.Budget), 0, true
	perAbilityMax := budget
	if descriptor.PerAbilityMax != 0 {
		perAbilityMax = descriptor.PerAbilityMax
	}
	for _, grant := range objects(decisions["abilityGrants"]) {
		if text(grant["id"]) == descriptor.ID {
			for ability, amount := range object(grant["assign"]) {
				value := integer(amount, 0)
				total += max(0, value)
				valid = valid && value >= 0 && value <= perAbilityMax && (descriptor.Eligible == nil || contains(descriptor.Eligible, ability))
			}
		}
	}
	return total, budget, valid && total == budget
}

func builderChoiceOptions(choice character.PlanChoice, sheet *character.Calculation, records Records) []character.Option {
	pool := choice.From
	if pool == nil {
		if spec := choiceKindSpecs[choice.Kind]; spec.pool != nil {
			pool = spec.pool(records)
		}
	}
	result := []character.Option{}
	for _, id := range unique(pool) {
		var record Object
		switch choice.Kind {
		case character.ChoiceFeat:
			record = recordByID(records, "feat", id)
			if record == nil || choice.Category != "" && choice.Category != text(record["category"]) || len(choice.Categories) > 0 && !contains(choice.Categories, text(record["category"])) {
				continue
			}
		case character.ChoiceWeaponMastery:
			record = recordByID(records, "weapon", id)
			if record == nil {
				continue
			}
		case character.ChoiceTools:
			record = recordByID(records, "tool", id)
		case character.ChoiceExpertise:
			if sheet == nil || !sheet.Skills[canonicalSkill(id)].Proficient {
				continue
			}
		case character.ChoiceProficiencies:
			if strings.HasPrefix(id, "tool:") {
				record = recordByID(records, "tool", strings.TrimPrefix(id, "tool:"))
			}
		case character.ChoiceEnumerated:
			if kind := choice.OptionKind; kind != "" {
				record = recordByID(records, kind, id)
				break
			}
			record = recordByID(records, "feature", id)
			if choice.FromCategory != "" && (record == nil || !optionPrerequisiteMet(record, sheet)) {
				continue
			}
		case character.ChoiceSkills, character.ChoiceSkillExpertise, character.ChoiceSkillUpgrade, character.ChoiceLanguages,
			character.ChoiceSavingThrows, character.ChoiceWeapons, character.ChoiceArmor, character.ChoiceResistances,
			character.ChoiceImmunities, character.ChoiceASIMode, character.ChoiceAbilityBudget, character.ChoiceSize:
		}
		option := guidanceOption(id, record)
		if choice.Kind == character.ChoiceSize {
			option.LabelKey = id
		}
		result = append(result, option)
	}
	sortGuidanceOptions(result)
	return result
}

func guidanceOption(id string, record Object) character.Option {
	result := character.Option{ID: id, Label: firstText(record["name"], guidanceLabel(id))}
	if description := text(record["text"]); description != "" {
		runes := []rune(description)
		result.Description = string(runes[:min(1200, len(runes))])
	}
	return result
}

// choiceHeading names a choice whose data has no prompt: what is chosen,
// then where it comes from, so internal IDs such as
// "species:elf:elf-keen-senses" never reach the page.
func choiceHeading(choice character.PlanChoice, records Records) (string, []any) {
	id := choice.ID
	if choice.Prompt != "" {
		return choice.Prompt, []any{}
	}
	source := character.Source{}
	if choice.Source != nil {
		source = *choice.Source
	}
	sourceName := grantSourceLabel(source, records)
	if sourceName == "" {
		return guidanceLabel(id), []any{}
	}
	if template := choiceKindSpecs[choice.Kind].heading; template != "" {
		// A class feature named after its choice ("Weapon Mastery") would repeat
		// the kind; the class names the choice better.
		if noun := strings.TrimSuffix(template, " · {0}"); strings.EqualFold(noun, sourceName) {
			if class := text(recordByID(records, "class", choice.ClassID)["name"]); class != "" {
				return template, []any{class}
			}
			return noun, []any{}
		}
		return template, []any{sourceName}
	}
	// Other choices are named after their own trait, without the owner prefix
	// the data repeats ("elf-lineage-spell" of Elf reads "Lineage Spell · Elf").
	own := id[strings.LastIndex(id, ":")+1:]
	own = strings.TrimPrefix(own, source.ID+"-")
	if name := guidanceLabel(own); name != "" && !strings.EqualFold(name, sourceName) {
		return "{0} · {1}", []any{name, sourceName}
	}
	return guidanceLabel(id), []any{}
}

func sortGuidanceOptions(options []character.Option) {
	sort.Slice(options, func(i, j int) bool {
		if options[i].Label == options[j].Label {
			return options[i].ID < options[j].ID
		}
		return options[i].Label < options[j].Label
	})
}

// guidanceText is Engine wording with an English label and a template key, so
// consumers translate it without translating record names.
func guidanceText(template string, args ...any) character.Text {
	replacements, values := []string{}, make([]string, 0, len(args))
	for index, arg := range args {
		value := fmt.Sprint(arg)
		replacements = append(replacements, fmt.Sprintf("{%d}", index), value)
		values = append(values, value)
	}
	return character.Text{Label: strings.NewReplacer(replacements...).Replace(template), LabelKey: template, LabelArgs: values}
}

func guidanceLabel(value string) string {
	var result strings.Builder
	for index, char := range value {
		if unicode.IsUpper(char) && index > 0 {
			result.WriteByte(' ')
		}
		if char == '-' || char == '_' || char == ':' {
			char = ' '
		}
		result.WriteRune(char)
	}
	parts := strings.Fields(result.String())
	for index, part := range parts {
		chars := []rune(part)
		chars[0] = unicode.ToUpper(chars[0])
		parts[index] = string(chars)
	}
	return strings.Join(parts, " ")
}
