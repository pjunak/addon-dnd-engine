package rules

import (
	"slices"

	"github.com/pjunak/addon-dnd-engine/character"
)

// A magic item's spellcastingFocus says its magic bonus also raises spell
// attack rolls, spell save DCs or both, for every spellcasting class or only
// the listed ones. A weapon or armor focus shares its baseItem bonus, so the
// owner picks one number for both; any other focus declares its own bonus.

// focusDecl is a record's spellcastingFocus.
type focusDecl struct {
	targets []character.EffectTarget
	classes []string
	// bonuses are the focus's own, declared only by a record without baseItem.
	bonuses []int
}

var focusTargets = []string{string(character.TargetSpellAttack), string(character.TargetSpellSaveDC)}

// spellcastingFocus reads a record's spellcastingFocus all or nothing,
// together with its baseItem: the bonus comes from exactly one of them. A
// record without one returns (nil, true).
func spellcastingFocus(record Object) (*focusDecl, bool) {
	raw, exists := record["spellcastingFocus"]
	if !exists {
		return nil, true
	}
	declared := object(raw)
	if declared == nil {
		return nil, false
	}
	decl := &focusDecl{}
	for key, value := range declared {
		valid := true
		switch key {
		case "appliesTo":
			decl.targets, valid = focusTargetList(value)
		case "classes":
			decl.classes, valid = baseItemIDs(value)
		case "bonus":
			decl.bonuses, valid = baseItemBonuses(value)
		default:
			valid = false
		}
		if !valid {
			return nil, false
		}
	}
	base, baseValid := magicBaseItem(record)
	switch {
	case len(decl.targets) == 0 || !baseValid:
		return nil, false
	case base != nil:
		return decl, len(base.bonuses) > 0 && decl.bonuses == nil
	default:
		return decl, decl.bonuses != nil
	}
}

// focusTargetList is one or both spell targets, each once.
func focusTargetList(value any) ([]character.EffectTarget, bool) {
	if !itemOneOf(value, focusTargets) {
		return nil, false
	}
	targets := []character.EffectTarget{}
	for _, name := range stringsOf(value) {
		target := character.EffectTarget(name)
		if slices.Contains(targets, target) {
			return nil, false
		}
		targets = append(targets, target)
	}
	return targets, true
}

// focusEffects are the spell attack and save DC effects an inventory instance
// of a spellcasting focus adds once its bonus is known. Callers decide whether
// the item is complete and in effect.
func focusEffects(item character.Item, records Records) []character.Effect {
	form := characterItemForm(item, records)
	if form == nil || form.focus == nil {
		return nil
	}
	return form.focus.effects(form.bonus)
}

// effects are what the focus adds with the instance's known bonus: an
// "add" effect per target, keyed by each listed class or by none for every
// class. Without a known bonus it adds nothing.
func (decl *focusDecl) effects(bonus int) []character.Effect {
	if bonus == 0 {
		return nil
	}
	keys := decl.classes
	if len(keys) == 0 {
		keys = []string{""}
	}
	effects := make([]character.Effect, 0, len(decl.targets)*len(keys))
	for _, target := range decl.targets {
		for _, key := range keys {
			effects = append(effects, character.Effect{Target: target, Key: key, Mode: character.ModeAdd, Value: bonus})
		}
	}
	return effects
}
