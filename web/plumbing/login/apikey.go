package login

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"strings"

	weberrors "github.com/thijzert/doc-hoarder/web/plumbing/errors"
	"golang.org/x/crypto/bcrypt"
)

type KeyID string
type APIKey struct {
	ID        KeyID
	User      UserID
	Label     string
	HashValue []byte
}

func sum512(s []byte) []byte {
	buf := sha512.Sum512(s)
	rv := make([]byte, len(buf))
	for i, c := range buf {
		rv[i] = c
	}
	return rv
}

func (a APIKey) Check(apikey string) bool {
	id, _, found := strings.Cut(apikey, ":")
	if !found {
		return false
	}
	if KeyID(id) != a.ID {
		return false
	}

	shapass := sum512([]byte(apikey))
	err := bcrypt.CompareHashAndPassword(a.HashValue, shapass)
	return err == nil
}

func newAPIKey() (APIKey, string) {
	buf := make([]byte, 40)
	rand.Read(buf)

	id := hex.EncodeToString(buf[:8])
	secret := hex.EncodeToString(buf[8:])

	apikey := id + ":" + secret
	shapass := sum512([]byte(apikey))
	hashvalue, _ := bcrypt.GenerateFromPassword(shapass, bcrypt.DefaultCost)

	rv := APIKey{
		ID:        KeyID(id),
		HashValue: hashvalue,
	}
	return rv, apikey
}

type keyMuster struct {
	store Store
	h     http.Handler
}

func (k keyMuster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.FormValue("api_key")
	if len(key) < 32 {
		k.HTTPError(w, r, weberrors.ErrUnauthorised)
		return
	}

	ctx := r.Context()
	user, err := k.store.GetUserByAPIKey(ctx, key)
	if err != nil {
		if err == ErrNotPresent {
			err = weberrors.ErrUnauthorised
		}
		k.HTTPError(w, r, err)
		return
	}

	// Store the user data in the request, and pass it to the next handler
	ctx = context.WithValue(ctx, loginKey, &user)
	r = r.WithContext(ctx)

	k.h.ServeHTTP(w, r)
}

func (k keyMuster) HTTPError(w http.ResponseWriter, r *http.Request, err error) {
	if erh, ok := k.h.(weberrors.ErrorHandler); ok {
		erh.HTTPError(w, r, err)
	} else {
		w.WriteHeader(500)
		w.Write([]byte("{\"_\": \"unauthorized\"})"))
	}
}

func MustHaveAPIKey(m Store) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return keyMuster{
			store: m,
			h:     h,
		}
	}
}