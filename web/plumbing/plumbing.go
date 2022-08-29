package plumbing

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// AssetsEmbedded indicates whether or not static assets are embedded in this binary
func AssetsEmbedded() bool {
	return assetsEmbedded
}

// GetAssets gets an embedded static asset
func GetAsset(name string) ([]byte, error) {
	return getAsset(name)
}

func WriteJSON(w http.ResponseWriter, rv interface{}) {
	rvm, err := json.Marshal(rv)
	if err != nil {
		w.WriteHeader(500)
		rvm, _ = json.Marshal(struct {
			Headline string `json:"_"`
		}{fmt.Sprintf("error encoding to json: %s", err)})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(rvm)))
	w.Write(rvm)
}

func JSONMessage(w http.ResponseWriter, format string, elems ...interface{}) {
	WriteJSON(w, struct {
		Message string `json:"_"`
	}{fmt.Sprintf(format, elems...)})
}

type HTTPStatuser interface {
	StatusCode() int
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

type errUnauth struct{}

func (errUnauth) Error() string   { return "an API key is required for this request" }
func (errUnauth) StatusCode() int { return 401 }

var ErrUnauthorised error = errUnauth{}

type errBad struct {
	Message string
}

func (errBad) Error() string   { return "bad request" }
func (errBad) StatusCode() int { return 400 }

func BadRequest(format string, elems ...interface{}) error {
	return errBad{fmt.Sprintf(format, elems...)}
}
