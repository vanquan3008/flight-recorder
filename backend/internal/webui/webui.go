// Package webui serves the operator console embedded in the TracePilot binary.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist/*
var files embed.FS

var indexHTML, _ = files.ReadFile("dist/index.html")

type Handler struct{ assets http.Handler }

func New() Handler {
	subtree, _ := fs.Sub(files, "dist")
	return Handler{assets: http.FileServer(http.FS(subtree))}
}

func (handler Handler) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("X-Content-Type-Options", "nosniff")
	responseWriter.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; connect-src 'self'; img-src 'self'; script-src 'self'; style-src 'self'")
	if request.URL.Path == "/" {
		responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = responseWriter.Write(indexHTML)
		return
	}
	if request.URL.Path == "/theme-init.js" {
		// Unhashed so it can run before first paint; revalidate instead of caching as immutable.
		responseWriter.Header().Set("Cache-Control", "no-cache")
		handler.assets.ServeHTTP(responseWriter, request)
		return
	}
	assetPath := path.Clean(request.URL.Path)
	if strings.HasPrefix(assetPath, "/assets/") {
		request.URL.Path = assetPath
		responseWriter.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		handler.assets.ServeHTTP(responseWriter, request)
		return
	}
	http.NotFound(responseWriter, request)
}
