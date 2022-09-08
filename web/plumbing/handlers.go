package plumbing

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
)

type Handler interface {
	Handle(r *http.Request) (interface{}, error)
}

type HandlerFunc func(r *http.Request) (interface{}, error)

func (f HandlerFunc) Handle(r *http.Request) (interface{}, error) {
	return f(r)
}

func AsJSON(h Handler) http.Handler {
	return jsonHandler{
		Handler: h,
	}
}

type jsonHandler struct {
	Handler Handler
}

func (j jsonHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type jsonErr struct {
		Headline string `json:"error"`
		Message  string `json:"_"`
	}

	code := 200

	rv, err := j.Handler.Handle(r)
	if err != nil {
		code = 500
		if sc, ok := err.(HTTPStatuser); ok {
			code = sc.StatusCode()
		}
		if red, ok := err.(httpRedirect); ok {
			w.Header().Set("Location", red.Location)
		}

		rrv := jsonErr{"internal server error", "an internal error has occurred"}
		if um, ok := err.(UserMessager); ok {
			rrv.Headline, rrv.Message = um.ErrorMessage()
		}

		rv = rrv
	}

	if bl, ok := rv.(Blob); ok {
		bl.WriteTo(w)
		return
	}

	rvm, err := json.Marshal(rv)
	if err != nil {
		code = 500
		rvm, _ = json.Marshal(jsonErr{"error encoding to json", err.Error()})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(rvm)))
	w.WriteHeader(code)
	w.Write(rvm)
}

func AsHTML(h Handler, templateName string) http.Handler {
	return htmlHandler{
		Handler:      h,
		TemplateName: templateName,
	}
}

type Blob struct {
	ContentType string
	Contents    []byte
	Header      http.Header
}

func (bl Blob) WriteTo(w http.ResponseWriter) {
	w.Header().Set("Content-Type", bl.ContentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bl.Contents)))
	if bl.Header != nil {
		for k, vs := range bl.Header {
			w.Header()[k] = vs
		}
	}
	w.Write(bl.Contents)
}

type htmlHandler struct {
	Handler      Handler
	TemplateName string
}

func (h htmlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	result, err := h.Handler.Handle(r)
	if err != nil {
		code := 500
		if sc, ok := err.(HTTPStatuser); ok {
			code = sc.StatusCode()
		}

		if red, ok := err.(httpRedirect); ok {
			w.Header().Set("Location", red.Location)
		}

		headline := "internal server error"
		message := "an internal error has occurred"
		if um, ok := err.(UserMessager); ok {
			headline, message = um.ErrorMessage()
		}

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(code)
		fmt.Fprintf(w, "<!DOCTYPE html>\n<html><head><base href=\".\"></head><body>\n\n")
		fmt.Fprintf(w, "<h1>%s</h1>", html.EscapeString(headline))
		fmt.Fprintf(w, "<p>%s</p>", html.EscapeString(message))
		fmt.Fprintf(w, "</body></html>")
		return
	}

	if bl, ok := result.(Blob); ok {
		bl.WriteTo(w)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<!DOCTYPE html>\n<html><head><base href=\".\"></head><body>\n\n"))
	w.Write([]byte("<p>Hello, world</p>\n"))

	w.Write([]byte("<p><a href=\"ext/hoard.xpi\">Download browser extension</a></p>\n"))

	if docids, ok := result.([]string); ok {
		w.Write([]byte("<ul>\n"))
		for _, docid := range docids {
			fmt.Fprintf(w, "\t<li><a href=\"documents/view/g%s/\">g%s</a></li>\n", docid, docid)
		}
		w.Write([]byte("</ul>\n"))
	}

	w.Write([]byte("</body></html>"))
}

type Headerer interface {
	HTTPHeader() http.Header
}
type Unwrapper interface {
	UnwrapResult() interface{}
}

func CORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h.ServeHTTP(w, r)
	})
}
