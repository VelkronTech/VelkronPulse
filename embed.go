package main

import (
	"embed"
	"io/fs"
)

//go:embed web/public/*
var webFS embed.FS

// embeddedFileSystem returns an http.FileSystem for the embedded frontend assets.
func embeddedFileSystem() fs.FS {
	sub, err := fs.Sub(webFS, "web/public")
	if err != nil {
		panic("failed to create embedded filesystem: " + err.Error())
	}
	return sub
}
