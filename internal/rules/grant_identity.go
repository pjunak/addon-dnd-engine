package rules

import (
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

func grantOwner(source Object) string {
	return character.AcquisitionOwner(text(source["type"])+":"+text(source["id"]), text(object(source["acquisition"])["id"]))
}

// Acquisition and item resources belong to their owner, so two acquisitions or
// two item instances never share spent uses: "item:<instance>:<key>".
func grantResourceKey(source Object, key string) string {
	if object(source["acquisition"]) != nil || text(source["type"]) == "item" {
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
func grantSourceReference(source Object) character.Reference {
	if ref := object(source["ref"]); ref != nil {
		return character.Reference{Kind: text(ref["kind"]), ID: text(ref["id"])}
	}
	return character.Reference{Kind: text(source["type"]), ID: text(source["id"])}
}

// grantSourceLabel names a grant source in guidance. An item's ID is an
// inventory instance, so it is named by the item instead.
func grantSourceLabel(source Object) string {
	if text(source["type"]) == "item" {
		return firstText(source["name"], guidanceLabel(text(source["id"])))
	}
	return guidanceLabel(text(source["id"]))
}

// An activation's resource is a resource key, or {key, cost} when one use
// spends more than one point.
func grantActivationResource(source Object, raw any) any {
	if key, ok := raw.(string); ok && key != "" {
		raw = Object{"key": key}
	}
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
