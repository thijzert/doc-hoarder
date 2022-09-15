package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sync"
	"time"
)

type mapStore struct {
	MaxDuration time.Duration
	Mu          sync.Mutex
	Contents    map[SessionID]Session
	Filename    string
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

	return m.saveContents(ctx)
}
func (m mapStore) saveContents(ctx context.Context) error {
	if m.Filename == "" {
		return nil
	}

	// Save map to filesystem
	f, err := os.Create(m.Filename)
	if err != nil {
		return err
	}
	defer f.Close()
	en := json.NewEncoder(f)
	en.SetIndent("", "\t")
	err = en.Encode(m.Contents)
	return err
}

func (m mapStore) Prune(ctx context.Context) error {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	rm := []SessionID{}
	for id, sess := range m.Contents {
		if time.Since(sess.LastSeen) > m.MaxDuration {
			rm = append(rm, id)
		}
	}
	for _, id := range rm {
		delete(m.Contents, id)
	}

	return m.saveContents(ctx)
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
	newMapStore := func(_descriptor string) (Store, error) {
		return mapStore{
			MaxDuration: 72 * time.Hour,
			Contents:    make(map[SessionID]Session),
		}, nil
	}
	RegisterStorageMethod("", newMapStore)
	RegisterStorageMethod("map", newMapStore)
	RegisterStorageMethod("memory", newMapStore)

	newFSStore := func(fileName string) (Store, error) {
		rv := mapStore{
			MaxDuration: 72 * time.Hour,
			Contents:    make(map[SessionID]Session),
			Filename:    fileName,
		}

		f, err := os.Open(fileName)
		if errors.Is(err, fs.ErrNotExist) {
			return rv, nil
		} else if err != nil {
			return nil, err
		}
		defer f.Close()

		dec := json.NewDecoder(f)
		err = dec.Decode(&rv.Contents)
		return rv, err
	}
	RegisterStorageMethod("fs", newFSStore)
	RegisterStorageMethod("file", newFSStore)
	RegisterStorageMethod("json", newFSStore)
}
