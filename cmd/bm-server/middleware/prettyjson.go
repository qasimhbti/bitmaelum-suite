package middleware

import (
	"net/http"
)

// PrettyJson is a middleware that will set a response header based on the request URI. This can be used to output
// json data either indented or not.
type PrettyJson struct {
	http.ResponseWriter
}

// Middleware sets header based on query param
func (*PrettyJson) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		w.Header().Set("x-pretty-json", q.Get("pretty"))

		next.ServeHTTP(w, req)
	})
}
