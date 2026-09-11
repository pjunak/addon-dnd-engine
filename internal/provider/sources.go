package provider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/pjunak/ttrpg-codex/sdk/go/workerrpc"
)

// Bindings come from initialization, never from a registry probe or known package ID.
func FromWorker(worker workerrpc.NativeWorkerContext) (*Client, error) {
	ids := []string{}
	for _, raw := range worker.Initialization.Services {
		var binding struct {
			Contract        string `json:"contract"`
			ProviderAddonID string `json:"providerAddonId"`
		}
		if err := json.Unmarshal(raw, &binding); err != nil {
			return nil, err
		}
		if binding.Contract == RulesDataContract {
			ids = append(ids, binding.ProviderAddonID)
		}
	}
	return New(worker.Peer, ids...)
}

func (client *Client) catalogs(ctx context.Context, meta *workerrpc.Meta) ([]CatalogResult, CatalogResult, error) {
	result := CatalogResult{Kinds: make(map[string]int)}
	catalogs := make([]CatalogResult, 0, len(client.members))
	identities := make([]Identity, 0, len(client.members))
	profiles := 0
	for _, member := range client.members {
		catalog, err := member.Catalog(ctx, meta)
		if err != nil {
			return nil, result, err
		}
		catalogs = append(catalogs, catalog)
		identities = append(identities, catalog.Identity)
		if catalog.Kinds["ruleset"] > 0 {
			result.Identity = catalog.Identity
			profiles += catalog.Kinds["ruleset"]
		}
		result.RecordCount += catalog.RecordCount
		if result.RecordCount > 100000 {
			return nil, result, incompatible("combined rules sources exceed the record limit")
		}
		for kind, count := range catalog.Kinds {
			result.Kinds[kind] += count
		}
	}
	if profiles != 1 {
		return nil, result, incompatible("combined rules sources must publish exactly one complete ruleset")
	}
	encoded, _ := json.Marshal(identities)
	digest := sha256.Sum256(encoded)
	result.Identity.ContentRevision = hex.EncodeToString(digest[:])
	return catalogs, result, nil
}

func (client *Client) combinedCatalog(ctx context.Context, meta *workerrpc.Meta) (CatalogResult, error) {
	_, combined, err := client.catalogs(ctx, meta)
	return combined, err
}

func (client *Client) combinedRecords(ctx context.Context, meta *workerrpc.Meta, kind string) (Identity, []Record, error) {
	client.queryMu.Lock()
	defer client.queryMu.Unlock()
	catalogs, catalog, err := client.catalogs(ctx, meta)
	if err != nil {
		return Identity{}, nil, err
	}
	key := catalog.Identity.ContentRevision + ":" + kind
	if cached, ok := client.queryCache[key]; ok {
		return cached.Identity, cached.Records, nil
	}
	records := make([]Record, 0, catalog.Kinds[kind])
	owners := make(map[string]string)
	for index, member := range client.members {
		if catalogs[index].Kinds[kind] == 0 {
			continue
		}
		loaded, err := member.loadKind(ctx, meta, catalogs[index].Identity, kind, catalogs[index].Kinds[kind])
		if err != nil {
			return Identity{}, nil, err
		}
		for _, record := range loaded {
			if previous, exists := owners[record.ID]; exists {
				return Identity{}, nil, incompatible("duplicate " + kind + "/" + record.ID + " in " + previous + " and " + member.providerID)
			}
			owners[record.ID] = member.providerID
			records = append(records, record)
		}
	}
	confirmed, err := client.combinedCatalog(ctx, meta)
	if err != nil {
		return Identity{}, nil, err
	}
	if confirmed.Identity != catalog.Identity {
		return Identity{}, nil, incompatible("rules sources changed while loading records")
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	// Keep only the current content snapshot; disabled records cannot survive in a cache.
	for old := range client.queryCache {
		if len(old) < 65 || old[:64] != catalog.Identity.ContentRevision {
			delete(client.queryCache, old)
		}
	}
	if client.queryCache == nil {
		client.queryCache = make(map[string]QueryResult)
	}
	client.queryCache[key] = QueryResult{Identity: catalog.Identity, Records: records}
	return catalog.Identity, records, nil
}

func (client *Client) combinedQuery(ctx context.Context, meta *workerrpc.Meta, query Query) (QueryResult, error) {
	identity, records, err := client.combinedRecords(ctx, meta, query.Kind)
	if err != nil {
		return QueryResult{}, err
	}
	stamp := sha256.Sum256([]byte(identity.ContentRevision + "\x00" + query.Kind))
	start := 0
	if query.Cursor != "" {
		cursor, err := base64.RawURLEncoding.DecodeString(query.Cursor)
		if err != nil || len(cursor) != 20 || base64.RawURLEncoding.EncodeToString(cursor) != query.Cursor || string(cursor[:16]) != string(stamp[:16]) {
			return QueryResult{}, invalid("rules-data cursor is stale or invalid")
		}
		start = int(binary.BigEndian.Uint32(cursor[16:]))
		if start >= len(records) {
			return QueryResult{}, invalid("rules-data cursor is out of range")
		}
	}
	end := min(start+query.Limit, len(records))
	result := QueryResult{Identity: identity, Records: make([]Record, 0, end-start)}
	for _, record := range records[start:end] {
		result.Records = append(result.Records, cloneRecord(record))
	}
	if end < len(records) {
		cursor := make([]byte, 20)
		copy(cursor, stamp[:16])
		binary.BigEndian.PutUint32(cursor[16:], uint32(end))
		result.NextCursor = base64.RawURLEncoding.EncodeToString(cursor)
	}
	return result, nil
}
