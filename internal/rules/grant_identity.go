package rules

import "github.com/pjunak/addon-dnd-engine/character"

func grantOwner(source Object) string {
	return character.AcquisitionOwner(text(source["type"])+":"+text(source["id"]), text(object(source["acquisition"])["id"]))
}

func grantResourceKey(source Object, key string) string {
	if object(source["acquisition"]) != nil {
		return grantOwner(source) + ":" + key
	}
	return key
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
