package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed playground/*
var playgroundEmbed embed.FS

func playgroundHandler() http.Handler {
	sub, err := fs.Sub(playgroundEmbed, "playground")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/playground/", http.FileServer(http.FS(sub)))
}
