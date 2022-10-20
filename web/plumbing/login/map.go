package login

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sync"
)

type mapStore struct {
	Mu       sync.Mutex
	Contents map[UserID]User
	Filename string
}

func (m mapStore) GetUser(ctx context.Context, id UserID) (User, error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	rv, ok := m.Contents[id]
	if !ok {
		return rv, ErrNotPresent
	}

	return rv, nil
}

func (m mapStore) StoreUser(ctx context.Context, sess User) error {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	return m.actuallyStoreUser(ctx, sess)
}
func (m mapStore) actuallyStoreUser(ctx context.Context, sess User) error {
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

func init() {
	newMapStore := func(_descriptor string) (Store, error) {
		return mapStore{
			Contents: make(map[UserID]User),
		}, nil
	}
	RegisterStorageMethod("", newMapStore)
	RegisterStorageMethod("map", newMapStore)
	RegisterStorageMethod("memory", newMapStore)

	newFSStore := func(fileName string) (Store, error) {
		rv := mapStore{
			Contents: make(map[UserID]User),
			Filename: fileName,
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
