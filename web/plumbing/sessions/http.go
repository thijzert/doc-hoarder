package sessions

import (
	"context"
	"log"
	"net/http"
	"time"
)

type sessPool int

var sessionKey sessPool = 1

const CookieName string = "hoard-session"

func GetSession(r *http.Request) *Session {
	ctx := r.Context()
	if s, ok := ctx.Value(sessionKey).(*Session); ok {
		return s
	}
	return &Session{}
}

type sessHandler struct {
	store Store
	h     http.Handler
}

func (sh sessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var id SessionID
	cook, _ := r.Cookie(CookieName)
	if cook != nil {
		id = SessionID(cook.Value)
	}

	ctx := r.Context()
	sess, err := sh.store.GetSession(ctx, id)
	if err == ErrNotPresent {
		sess, err = sh.store.NewSession(ctx)
		id = sess.ID
	}
	if err != nil {
		log.Printf("error getting session: %s", err)
		w.WriteHeader(500)
		w.Write([]byte("session store error"))
	}

	lifetime := 12 * time.Hour

	cookie := http.Cookie{
		Name:     CookieName,
		Value:    string(id),
		Expires:  time.Now().Add(lifetime),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	w.Header().Add("Set-Cookie", cookie.String())
	log.Printf("Set-Cookie: %s", cookie.String())

	cctx := context.WithValue(ctx, sessionKey, &sess)
	r = r.WithContext(cctx)

	sh.h.ServeHTTP(w, r)

	if sess.Dirty {
		sess.ID = id
		sh.store.StoreSession(ctx, sess)
	}
}

func WithSession(store Store, h http.Handler) http.Handler {
	return sessHandler{
		store: store,
		h:     h,
	}
}
