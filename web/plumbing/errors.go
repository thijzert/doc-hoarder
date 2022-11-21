package plumbing

import (
	"net/http"

	weberrors "github.com/thijzert/doc-hoarder/web/plumbing/errors"
)

type HTTPStatuser interface {
	StatusCode() int
}

type UserMessager interface {
	ErrorMessage() (string, string)
}

func HTMLError(w http.ResponseWriter, err error) {
	JSONError(w, err)
}

func JSONError(w http.ResponseWriter, err error) {
	st := 500
	if s, ok := err.(HTTPStatuser); ok {
		st = s.StatusCode()
	}

	w.WriteHeader(st)
	JSONMessage(w, "%s", err.Error())
}

var ErrUnauthorised = weberrors.ErrUnauthorised // TODO: remove
var ErrNotFound = weberrors.ErrNotFound         // TODO: remove
var BadRequest = weberrors.BadRequest           // TODO: remove
var Forbidden = weberrors.Forbidden             // TODO: remove

type httpRedirect struct {
	Code     int
	Location string
}

func (httpRedirect) Error() string { return "redirect" }
func (r httpRedirect) StatusCode() int {
	if r.Code >= 300 && r.Code < 400 {
		return r.Code
	}
	return 302
}
func (httpRedirect) ErrorMessage() (string, string) {
	return "redirect", "you are being redirected"
}

func Redirect(code int, location string) error {
	return httpRedirect{code, location}
}
