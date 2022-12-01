package storage

import "context"

func init() {
	RegisterCacheMethod("", func(desc string, store DocStore) (DocumentCache, error) {

		return noCache{store}, nil
	})
}

type noCache struct {
	store DocStore
}

func (c noCache) GetDocuments(ctx context.Context, user_id string, limit Limit) ([]string, []DocumentMeta, error) {
	docids, err := c.store.DocumentIDs(ctx) // TODO: filter by read permissions
	if err != nil {
		return nil, nil, err
	}
	rvids := make([]string, 0, limit.Limit)
	rv := make([]DocumentMeta, 0, limit.Limit)
	found := 0
	for _, docid := range docids {
		trns, err := c.store.GetDocument(docid)
		if err != nil {
			return rvids, rv, err
		}
		meta, _ := ReadMeta(ctx, trns)
		trns.Rollback()

		canView := false
		if meta.Permissions.Public {
			canView = true
		}
		if user_id != "" {
			if meta.Permissions.Owner == user_id {
				canView = true
			}
			// TODO: apply ReadUsers/ReadGroups
		}
		if !canView {
			continue
		}

		found++
		if found <= limit.Offset {
			continue
		}

		rvids = append(rvids, docid)
		rv = append(rv, meta)
		if limit.Limit > 0 && len(rv) >= limit.Limit {
			break
		}
	}
	return rvids, rv, nil
}
func (c noCache) GetDocumentByURL(ctx context.Context, user_id, page_url string) (DocTransaction, bool, error) {
	docids, err := c.store.DocumentIDs(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, docid := range docids {
		trns, err := c.store.GetDocument(docid)
		if err != nil {
			return nil, false, err
		}
		meta, err := ReadMeta(ctx, trns)
		if err != nil {
			return nil, false, err
		}

		if meta.URL == page_url && meta.Permissions.Owner == user_id {
			return trns, true, nil
		}

		trns.Rollback()
	}
	return nil, false, nil
}
func (c noCache) GetDocumentMeta(ctx context.Context, doc_id string) (DocumentMeta, error) {
	trns, err := c.store.GetDocument(doc_id)
	if err != nil {
		return DocumentMeta{}, err
	}
	defer trns.Rollback()
	return ReadMeta(ctx, trns)
}
