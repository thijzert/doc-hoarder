package sessions

import (
	"context"
	"sync"
	"time"
)

type mapStore struct {
	Mu       sync.Mutex
	Contents map[SessionID]Session
}

func (m mapStore) NewSession(ctx context.Context) (Session, error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	var id SessionID
	var rv Session

	for i := 0; i < 32; i++ {
		id = NewSessionID()
		if _, ok := m.Contents[id]; !ok {
			break
		}
		if err := ctx.Err(); err != nil {
			return rv, err
		}
	}

	rv.ID = id
	rv.Started = time.Now()
	rv.LastSeen = time.Now()

	err := m.actuallyStoreSession(ctx, rv)
	return rv, err
}

func (m mapStore) GetSession(ctx context.Context, id SessionID) (Session, error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	rv, ok := m.Contents[id]
	if !ok {
		return rv, ErrNotPresent
	}

	return rv, nil
}

func (m mapStore) StoreSession(ctx context.Context, sess Session) error {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	return m.actuallyStoreSession(ctx, sess)
}
func (m mapStore) actuallyStoreSession(ctx context.Context, sess Session) error {
	m.Contents[sess.ID] = sess

	// TODO: save to FS

	return nil
}

func (m mapStore) TouchSession(ctx context.Context, id SessionID) error {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	rv, ok := m.Contents[id]
	if !ok {
		return ErrNotPresent
	}

	rv.LastSeen = time.Now()
	return m.actuallyStoreSession(ctx, rv)
}

func init() {
	newMapStore := func(descriptor string) (Store, error) {
		return mapStore{
			Contents: make(map[SessionID]Session),
		}, nil
	}
	RegisterStorageMethod("", newMapStore)
	RegisterStorageMethod("map", newMapStore)
	RegisterStorageMethod("memory", newMapStore)
}
