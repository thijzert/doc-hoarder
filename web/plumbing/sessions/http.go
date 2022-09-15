package sessions

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	weberrors "github.com/thijzert/doc-hoarder/web/plumbing/errors"
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
		sh.HTTPError(w, r, err)
		return
	}

	lifetime := 12 * time.Hour

	cookie := http.Cookie{
		Name:     CookieName,
		Value:    string(id),
		Path:     "/",
		Expires:  time.Now().Add(lifetime),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	w.Header().Add("Set-Cookie", cookie.String())

	cctx := context.WithValue(ctx, sessionKey, &sess)
	r = r.WithContext(cctx)

	sh.h.ServeHTTP(w, r)

	if sess.Dirty {
		sess.ID = id
		sh.store.StoreSession(ctx, sess)
	} else {
		sh.store.TouchSession(ctx, id)
	}
}

func (sh sessHandler) HTTPError(w http.ResponseWriter, r *http.Request, err error) {
	if erh, ok := sh.h.(weberrors.ErrorHandler); ok {
		erh.HTTPError(w, r, err)
	} else {
		w.WriteHeader(500)
		fmt.Fprintf(w, "session error: %s", err)
	}
}

func WithSession(store Store, h http.Handler) http.Handler {
	return sessHandler{
		store: store,
		h:     h,
	}
}
