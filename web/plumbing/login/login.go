package login

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrNotPresent = errors.New("user not present")
var ErrLoginRequired error

type errLoginRequired struct{}

func (errLoginRequired) Error() string   { return "login required" }
func (errLoginRequired) StatusCode() int { return 401 }
func (errLoginRequired) ErrorMessage() (string, string) {
	return "authorization required", "You must log in to view this resource"
}

func init() {
	ErrLoginRequired = errLoginRequired{}
}

type UserID string

type User struct {
	ID    UserID
	Dirty bool `json:"-",xml:"-"`

	FullName  string
	GivenName string
}

type Store interface {
	GetUser(context.Context, UserID) (User, error)
	StoreUser(context.Context, User) error
	GetAPIKey(ctx context.Context, id KeyID) (APIKey, error)
	GetUserByAPIKey(ctx context.Context, apikey string) (User, error)
	GetAPIKeysForUser(ctx context.Context, userID UserID) ([]APIKey, error)
	NewAPIKeyForUser(ctx context.Context, userID UserID, label string) (string, error)
	DisableAPIKey(ctx context.Context, userID UserID, id KeyID) error
}

type StorageMethod func(string) (Store, error)

var allStorageMethods map[string]StorageMethod

func RegisterStorageMethod(name string, f StorageMethod) {
	if allStorageMethods == nil {
		allStorageMethods = make(map[string]StorageMethod)
	}
	allStorageMethods[name] = f
}

func GetUserStore(descriptor string) (Store, error) {
	i := strings.IndexRune(descriptor, ':')
	name := ""
	if i >= 0 {
		name = descriptor[:i]
		descriptor = descriptor[i+1:]
	}
	if allStorageMethods == nil {
		return nil, fmt.Errorf("no storage backends have been initialized")
	}

	if f, ok := allStorageMethods[name]; ok {
		return f(descriptor)
	}
	return nil, fmt.Errorf("storage backend '%s' not registered", name)
}
