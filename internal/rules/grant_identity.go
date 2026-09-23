package rules

import (
	"sort"

	"github.com/pjunak/addon-dnd-engine/character"
)

func grantOwner(source Object) string {
	return character.AcquisitionOwner(text(source["type"])+":"+text(source["id"]), text(object(source["acquisition"])["id"]))
}

func grantLegacyOwner(source Object) string { return text(source["type"]) + ":" + text(source["id"]) }

func grantResourceKey(source Object, key string) string {
	if object(source["acquisition"]) != nil {
		return grantOwner(source) + ":" + key
	}
	return key
}

func grantActivationResource(source Object, raw any) any {
	if object(raw) == nil {
		return nil
	}
	resource := cloneObjectDeep(object(raw))
	resource["key"] = grantResourceKey(source, text(resource["key"]))
	return resource
}

func grantFreeResourceKey(grant Object) string {
	if key := text(grant["resourceKey"]); key != "" {
		return key
	}
	return "charge:" + spellGrantKey(grant)
}

// Old unscoped state is attached only when exactly one owner exists. Ambiguous
// aliases remain authored input and are exposed for explicit assignment in the
// Builder. Reading never writes a migration or duplicates spent uses.
func normalizeCharacterGrantState(input *character.Inputs, sheet Object) bool {
	casting := object(sheet["spellcasting"])
	changed := moveGrantAliases(input.Build.Spells.GrantChoices, objects(casting["pendingChoices"]))
	changed = moveGrantAliases(input.Build.Spells.CastingAbilities, objects(casting["castingAbilityChoices"])) || changed
	changed = moveGrantAliases(input.Play.ResourceUses, objects(sheet["resources"])) || changed
	changed = moveGrantAliases(input.Play.ActiveFeatures, objects(sheet["activations"])) || changed
	return changed
}

func moveGrantAliases[T any](saved map[string]T, descriptors []Object) bool {
	aliases := map[string][]string{}
	for _, descriptor := range descriptors {
		alias, key := text(descriptor["legacyKey"]), text(descriptor["key"])
		if alias != "" && alias != key {
			aliases[alias] = append(aliases[alias], key)
		}
	}
	keys := make([]string, 0, len(aliases))
	for alias := range aliases {
		keys = append(keys, alias)
	}
	sort.Strings(keys)
	changed := false
	for _, alias := range keys {
		owners := unique(aliases[alias])
		value, exists := saved[alias]
		if !exists || len(owners) != 1 {
			continue
		}
		if _, occupied := saved[owners[0]]; occupied {
			continue
		}
		saved[owners[0]] = value
		delete(saved, alias)
		changed = true
	}
	return changed
}
