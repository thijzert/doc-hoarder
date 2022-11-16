package plumbing

import (
	"net/http"

	weberrors "github.com/thijzert/doc-hoarder/web/plumbing/errors"
)

type landingPageHandler struct {
	h http.Handler
}

func LandingPageOnly(h http.Handler) http.Handler {
	return landingPageHandler{h}
}

func (l landingPageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		l.h.ServeHTTP(w, r)
	}

	if erh, ok := l.h.(weberrors.ErrorHandler); ok {
		erh.HTTPError(w, r, ErrNotFound)
	} else {
		w.WriteHeader(404)
		w.Write([]byte("404 not found"))
	}
}
