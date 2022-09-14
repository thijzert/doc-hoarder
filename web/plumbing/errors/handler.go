package weberrors

import (
	"net/http"
)

type ErrorHandler interface {
	HTTPError(http.ResponseWriter, *http.Request, error)
}
