package sessions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SessionID string

func NewSessionID() SessionID {
	buf := make([]byte, 24)
	rand.Read(buf)
	return SessionID(hex.EncodeToString(buf))
}

type Session struct {
	ID        SessionID
	Dirty     bool `json:"-",xml:"-"`
	Started   time.Time
	LastSeen  time.Time
	Destroyed bool `json:"destroyed,omitEmpty"`

	LoggedInAs string

	// FIXME: decouple from OpenID / OAuth
	LoginSession string
	AccessToken  string
	RefreshToken string
}

type Store interface {
	NewSession(context.Context) (Session, error)
	GetSession(context.Context, SessionID) (Session, error)
	StoreSession(context.Context, Session) error
	TouchSession(context.Context, SessionID) error
	Prune(context.Context) error
}

var ErrNotPresent = errors.New("session not present")

type StorageMethod func(string) (Store, error)

var allStorageMethods map[string]StorageMethod

func RegisterStorageMethod(name string, f StorageMethod) {
	if allStorageMethods == nil {
		allStorageMethods = make(map[string]StorageMethod)
	}
	allStorageMethods[name] = f
}

func GetStore(descriptor string) (Store, error) {
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
