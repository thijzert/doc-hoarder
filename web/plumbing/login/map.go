package login

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"sync"
)

type mapStore struct {
	Mu       sync.Mutex
	Contents map[UserID]User
	Filename string
	APIKeys  map[KeyID]APIKey
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
	err = en.Encode(struct {
		Profiles map[UserID]User
		APIKeys  map[KeyID]APIKey
	}{m.Contents, m.APIKeys})
	return err
}

func (m mapStore) GetAPIKey(ctx context.Context, id KeyID) (APIKey, error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	rv, ok := m.APIKeys[id]
	if !ok {
		return rv, ErrNotPresent
	}

	return rv, nil
}
func (m mapStore) GetUserByAPIKey(ctx context.Context, apikey string) (User, error) {
	id, _, found := strings.Cut(apikey, ":")
	if !found {
		return User{}, ErrNotPresent
	}
	key, err := m.GetAPIKey(ctx, KeyID(id))
	if err != nil {
		return User{}, err
	}
	if !key.Check(apikey) {
		return User{}, ErrNotPresent
	}

	return m.GetUser(ctx, key.User)
}

func (m mapStore) GetAPIKeysForUser(ctx context.Context, userID UserID) ([]APIKey, error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	var rv []APIKey
	for _, key := range m.APIKeys {
		if key.User == userID {
			rv = append(rv, key)
		}
	}
	return rv, nil
}

func (m mapStore) NewAPIKeyForUser(ctx context.Context, userID UserID, label string) (string, error) {
	key, secret := newAPIKey()
	key.User = userID
	key.Label = label

	m.Mu.Lock()
	defer m.Mu.Unlock()
	// TODO: check for collisions
	m.APIKeys[key.ID] = key

	err := m.saveContents(ctx)
	if err != nil {
		return "", err
	}
	return secret, nil
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
			APIKeys:  make(map[KeyID]APIKey),
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
		dummy := struct {
			Profiles *map[UserID]User
			APIKeys  *map[KeyID]APIKey
		}{&rv.Contents, &rv.APIKeys}
		err = dec.Decode(&dummy)
		return rv, err
	}
	RegisterStorageMethod("fs", newFSStore)
	RegisterStorageMethod("file", newFSStore)
	RegisterStorageMethod("json", newFSStore)
}
