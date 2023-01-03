package login

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/pkg/errors"
	weberrors "github.com/thijzert/doc-hoarder/web/plumbing/errors"
	"github.com/thijzert/doc-hoarder/web/plumbing/sessions"
	"golang.org/x/oauth2"
)

const cookieName string = "oidc-auth"

type OIDC struct {
	URL          string
	AppRoot      string
	ReturnURL    string
	ClientID     string
	ClientSecret string
	store        Store
	cookieKey    []byte
	provider     *oidc.Provider
	oauth2Config oauth2.Config
	verifier     *oidc.IDTokenVerifier
}

func OIDCFromURL(ctx context.Context, loginProvider string, userStore Store, thisApplication, callbackURL string) (OIDC, error) {
	var rv OIDC
	rv.store = userStore

	rv.cookieKey = make([]byte, 32)
	rand.Read(rv.cookieKey)

	ru, err := url.Parse(loginProvider)
	if err != nil {
		return rv, errors.Wrap(err, "error parsing oidc url")
	}
	if ru.User != nil {
		rv.ClientID = ru.User.Username()
		rv.ClientSecret, _ = ru.User.Password()
		ru.User = nil
	}
	rv.URL = ru.String()

	approot, err := url.Parse(thisApplication)
	if err != nil {
		return rv, errors.Wrap(err, "error parsing approot url")
	}
	rv.AppRoot = strings.TrimRight(approot.Path, "/")

	rv.ReturnURL = thisApplication + callbackURL

	rv.provider, err = oidc.NewProvider(ctx, rv.URL)
	if err != nil {
		return rv, errors.Wrap(err, "error setting up oidc provider")
	}

	// Configure an OpenID Connect aware OAuth2 client.
	rv.oauth2Config = oauth2.Config{
		ClientID:     rv.ClientID,
		ClientSecret: rv.ClientSecret,
		RedirectURL:  rv.ReturnURL,

		// Discovery returns the OAuth2 endpoints.
		Endpoint: rv.provider.Endpoint(),

		// "openid" is a required scope for OpenID Connect flows.
		Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
	}

	oidcConfig := &oidc.Config{
		ClientID: rv.ClientID,
	}
	rv.verifier = rv.provider.Verifier(oidcConfig)

	return rv, nil
}

func (o OIDC) Must(h http.Handler) http.Handler {
	return oidcHandler{
		oidc:            o,
		loginRequired:   true,
		handleCallbacks: true,
		h:               h,
	}
}
func (o OIDC) May(h http.Handler) http.Handler {
	return oidcHandler{
		oidc:            o,
		loginRequired:   false,
		handleCallbacks: true,
		h:               h,
	}
}

type loginKeyType int

var loginKey loginKeyType = 2

func GetUser(r *http.Request) (*User, bool) {
	ctx := r.Context()
	if s, ok := ctx.Value(loginKey).(*User); ok {
		return s, true
	}
	return &User{}, false
}

type no401Writer struct {
	W     http.ResponseWriter
	R     *http.Request
	On401 func(http.ResponseWriter, *http.Request)
}

func (w no401Writer) Header() http.Header {
	if w.W == nil {
		return nil
	}
	return w.W.Header()
}
func (w no401Writer) Write(buf []byte) (int, error) {
	if w.W == nil {
		return 0, nil
	}
	return w.W.Write(buf)
}
func (w no401Writer) WriteHeader(status int) {
	if status == 401 && w.On401 != nil {
		w.On401(w.W, w.R)
		w.W = nil
	} else {
		w.W.WriteHeader(status)
	}
}

type oidcHandler struct {
	oidc            OIDC
	loginRequired   bool
	handleCallbacks bool
	h               http.Handler
}

func (o oidcHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	initiateLogin := func(w http.ResponseWriter, r *http.Request) {
		stateValues := url.Values{}
		stateValues.Set("ru", o.oidc.AppRoot+r.URL.Path)
		state := stateValues.Encode()

		mac := hmac.New(sha256.New, o.oidc.cookieKey)
		mac.Write([]byte(state))
		stateAuth := hex.EncodeToString(mac.Sum(nil))

		lifetime := 20 * time.Minute
		cookie := http.Cookie{
			Name:     cookieName,
			Value:    stateAuth,
			Path:     "/",
			Expires:  time.Now().Add(lifetime),
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteNoneMode,
		}
		w.Header().Add("Set-Cookie", cookie.String())

		http.Redirect(w, r, o.oidc.oauth2Config.AuthCodeURL(state), 302)
	}

	sess := sessions.GetSession(r)
	if sess == nil {
		sess = &sessions.Session{}
	}

	id := UserID(sess.LoggedInAs)
	if id == "" && o.handleCallbacks {
		// TODO: check for callback requests
	}
	if id == "" {
		if o.loginRequired {
			initiateLogin(w, r)
			return
		} else {
			ww := no401Writer{w, r, initiateLogin}
			o.h.ServeHTTP(ww, r)
		}
		return
	}

	user, err := o.oidc.store.GetUser(r.Context(), id)
	if err == ErrNotPresent {
		user = User{
			ID: id,
		}
		err = o.oidc.store.StoreUser(ctx, user)
	}
	if err != nil {
		// error getting user id
		o.HTTPError(w, r, err)
		return
	}

	// Store the user data in the request, and pass it to the next handler
	ctx = context.WithValue(ctx, loginKey, &user)
	r = r.WithContext(ctx)

	o.h.ServeHTTP(w, r)
}

func (sh oidcHandler) HTTPError(w http.ResponseWriter, r *http.Request, err error) {
	if erh, ok := sh.h.(weberrors.ErrorHandler); ok {
		erh.HTTPError(w, r, err)
	} else {
		w.WriteHeader(500)
		fmt.Fprintf(w, "login error: %s", err)
	}
}

func (o OIDC) Callback() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessions.GetSession(r)
		if sess == nil {
			sess = &sessions.Session{}
		}

		ctx := r.Context()

		state := r.URL.Query().Get("state")
		stateValues, err := url.ParseQuery(state)
		if err != nil {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		mac := hmac.New(sha256.New, o.cookieKey)
		mac.Write([]byte(state))
		expectedAuth := mac.Sum(nil)
		stateAuth := []byte{}
		if c, err := r.Cookie(cookieName); err == nil && c != nil {
			stateAuth, err = hex.DecodeString(c.Value)
		}
		if !hmac.Equal(expectedAuth, stateAuth) {
			http.Error(w, "state did not match", http.StatusBadRequest)
			return
		}

		oauth2Token, err := o.oauth2Config.Exchange(ctx, r.URL.Query().Get("code"))
		if err != nil {
			http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rawIDToken, ok := oauth2Token.Extra("id_token").(string)
		if !ok {
			http.Error(w, "No id_token field in oauth2 token.", http.StatusInternalServerError)
			return
		}
		idToken, err := o.verifier.Verify(ctx, rawIDToken)
		if err != nil {
			http.Error(w, "Failed to verify ID Token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var claims struct {
			UserID        string `json:"sub,omitEmpty"`
			Email         string `json:"email,omitEmpty"`
			EmailVerified bool   `json:"email_verified,omitEmpty"`
			FullName      string `json:"name,omitEmpty"`
			GivenName     string `json:"given_name,omitEmpty"`
		}
		if err := idToken.Claims(&claims); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// FIXME: I'm assuming that reaching this point means the user and these claims are authenticated. Are they?
		sess.LoggedInAs = claims.UserID
		sess.Dirty = true

		user, err := o.store.GetUser(ctx, UserID(claims.UserID))
		if err == ErrNotPresent {
			user = User{
				ID: UserID(claims.UserID),
			}
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		user.FullName = claims.FullName
		user.GivenName = claims.GivenName

		err = o.store.StoreUser(ctx, user)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Delete the extra auth cookie
		cookie := http.Cookie{
			Name:     cookieName,
			Value:    "",
			Path:     "/",
			Expires:  time.Now().Add(-5 * time.Minute),
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteNoneMode,
		}
		w.Header().Add("Set-Cookie", cookie.String())

		ru := stateValues.Get("ru")
		if ru == "" {
			ru = o.AppRoot
		}
		// http.Redirect(w, r, ru, 302)
		w.Header().Add("Content-Type", "text/html")

		ruhtml := html.EscapeString(ru)
		fmt.Fprintf(w, "<meta http-equiv=\"refresh\" content=\"0;%s\" />", ruhtml)
		fmt.Fprintf(w, "<p><a href=\"%s\">Redirecting to %s</a></p>", ruhtml, ruhtml)
	})
}
