package rules

import (
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

func grantOwner(source character.Source) string {
	acquisition := ""
	if source.Acquisition != nil {
		acquisition = source.Acquisition.ID
	}
	return character.AcquisitionOwner(string(source.Type)+":"+source.ID, acquisition)
}

// Acquisition and item resources belong to their owner, so two acquisitions or
// two item instances never share spent uses: "item:<instance>:<key>".
func grantResourceKey(source character.Source, key string) string {
	if source.Acquisition != nil || source.Type == character.SourceItem {
		return grantOwner(source) + ":" + key
	}
	return key
}

// itemGrantOwner is the inventory instance that owns an item grant key
// ("item:<instance>:<local key>"), or "" for any other key. Local keys have no
// colon but instance IDs may, so the longest matching instance owns the key.
func itemGrantOwner(key string, inventory []character.Item) string {
	owner := ""
	for _, item := range inventory {
		if item.ID != "" && len(item.ID) > len(owner) && strings.HasPrefix(key, "item:"+item.ID+":") {
			owner = item.ID
		}
	}
	return owner
}

// grantSourceReference is the record a grant source stands for. An item source
// is keyed by its inventory instance and names its record in ref.
func grantSourceReference(source character.Source) character.Reference {
	if source.Ref != nil {
		return *source.Ref
	}
	return character.Reference{Kind: string(source.Type), ID: source.ID}
}

// grantSourceLabel names a grant source in guidance by its record's name, or
// its titled ID without the record. Provider IDs need not read as names
// ("hb.emberkin"). An item's ID is an inventory instance, so it is named by the
// item instead.
func grantSourceLabel(source character.Source, records Records) string {
	if source.Type == character.SourceItem {
		return firstText(source.Name, guidanceLabel(source.ID))
	}
	ref := grantSourceReference(source)
	return firstText(recordView(records, ref.Kind, ref.ID)["name"], guidanceLabel(source.ID))
}

// An activation's resource is a resource key, or {key, cost} when one use
// spends more than one point.
func grantActivationResource(source character.Source, raw any) *character.ActivationResource {
	if key, ok := raw.(string); ok && key != "" {
		raw = Object{"key": key}
	}
	declared := object(raw)
	if declared == nil {
		return nil
	}
	return &character.ActivationResource{Key: grantResourceKey(source, text(declared["key"])), Cost: integer(declared["cost"], 0)}
}

func grantFreeResourceKey(grant character.SpellGrant) string {
	if grant.ResourceKey != "" {
		return grant.ResourceKey
	}
	return "charge:" + spellGrantKey(grant)
}
