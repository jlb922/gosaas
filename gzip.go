package gosaas

import (
	"fmt"
	"net/http"

	"github.com/NYTimes/gziphandler"
)

// Gzip is a middleware that compresses the responses
func Gzip(next http.Handler) http.Handler {
	fmt.Println("in gzip")
	return gziphandler.GzipHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(ctx))
	}))
}
