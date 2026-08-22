package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

func webHandler() http.Handler {
	files, fileSystemError := fs.Sub(staticFiles, "static")
	if fileSystemError != nil {
		panic(fileSystemError)
	}
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; style-src 'self'; script-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		http.FileServer(http.FS(files)).ServeHTTP(responseWriter, request)
	})
}
