package rules

import "github.com/pjunak/addon-dnd-engine/character"

// Eligibility is a lasting class/trait capability, independent of today's spent
// slots. Item spells never supply the prerequisite for attuning another item.
func hasIntrinsicSpellcasting(sheet *character.Calculation) bool {
	casting := sheet.Spellcasting
	for _, class := range casting.PerClass {
		if class.CantripsKnown > 0 || class.PreparedLimit > 0 && class.MaxSpellLevel > 0 {
			return true
		}
	}
	for _, grant := range casting.Granted {
		switch grant.Source.Type {
		case character.SourceClass, character.SourceSubclass, character.SourceSpecies, character.SourceBackground, character.SourceFeat, character.SourceFeature:
		case character.SourceItem, character.SourceSpellcasting, character.SourcePactMagic, character.SourceArmor,
			character.SourceCondition, character.SourceSpell, character.SourceASI, character.SourceAbility:
			continue
		default:
			continue
		}
		level := -1
		if grant.Level != nil {
			level = *grant.Level
		}
		if grant.Ref != "" && (level == 0 || level > 0 && grant.Free != nil && *grant.Free != "") {
			return true
		}
	}
	return false
}
