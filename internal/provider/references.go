package provider

import (
	"context"
	"errors"

	"github.com/pjunak/ttrpg-codex/sdk/go/workerrpc"
)

// A profile can depend on one rule without loading the entire reference-prose
// catalog into the calculation snapshot. Every candidate still shares the
// catalog's generation/revision and duplicate identities remain ambiguous.
func (client *Client) referencedRecord(ctx context.Context, meta *workerrpc.Meta, expected Identity, kind, id string) (Record, bool, error) {
	members, identities := []*Client{client}, []Identity{expected}
	if len(client.members) > 0 {
		catalogs, combined, err := client.catalogs(ctx, meta)
		if err != nil {
			return Record{}, false, err
		}
		if !sameProviderContent(combined.Identity, expected) {
			return Record{}, false, incompatible("rules sources changed while loading a referenced record")
		}
		members, identities = nil, nil
		for index, catalog := range catalogs {
			if catalog.Kinds[kind] > 0 {
				members = append(members, client.members[index])
				identities = append(identities, catalog.Identity)
			}
		}
	}
	var selected Record
	found := false
	for index, member := range members {
		identity, record, err := member.Get(ctx, meta, kind, id)
		if err != nil {
			var rpc *workerrpc.RPCError
			if errors.As(err, &rpc) && rpc.Data != nil && rpc.Data.Kind == workerrpc.KindNotFound {
				continue
			}
			return Record{}, false, err
		}
		if !sameProviderContent(identity, identities[index]) {
			return Record{}, false, incompatible("rules sources changed while loading a referenced record")
		}
		if found {
			return Record{}, false, incompatible("a referenced rule has multiple source owners")
		}
		record.SourceIdentity = identity
		selected, found = record, true
	}
	return selected, found, nil
}
