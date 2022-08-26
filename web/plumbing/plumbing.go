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
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(rvm)))
	w.Write(rvm)
}
