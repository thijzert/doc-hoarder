package weberrors

import "fmt"

type errUnauth struct{}

func (errUnauth) Error() string   { return "an API key is required for this request" }
func (errUnauth) StatusCode() int { return 401 }
func (errUnauth) ErrorMessage() (string, string) {
	return "unauthorized", "an API key is required for this request"
}

var ErrUnauthorised error = errUnauth{}

type errLoginRequired struct{}

func (errLoginRequired) Error() string   { return "login required" }
func (errLoginRequired) StatusCode() int { return 401 }
func (errLoginRequired) ErrorMessage() (string, string) {
	return "unauthorized", "You must log in to view this resource"
}

var ErrLoginRequired error = errLoginRequired{}

type err404 struct{}

func (err404) Error() string   { return "not found" }
func (err404) StatusCode() int { return 404 }
func (err404) ErrorMessage() (string, string) {
	return "not found", "the requested resource or document could not be found"
}

var ErrNotFound error = err404{}

type errBad struct {
	Message string
}

func (errBad) Error() string                    { return "bad request" }
func (errBad) StatusCode() int                  { return 400 }
func (e errBad) ErrorMessage() (string, string) { return "bad request", e.Message }

func BadRequest(format string, elems ...interface{}) error {
	return errBad{fmt.Sprintf(format, elems...)}
}

type errForbidden struct {
	Message string
}

func (errForbidden) Error() string                    { return "forbidden" }
func (errForbidden) StatusCode() int                  { return 403 }
func (e errForbidden) ErrorMessage() (string, string) { return "forbidden", e.Message }

func Forbidden(format string, elems ...interface{}) error {
	return errForbidden{fmt.Sprintf(format, elems...)}
}
