package web

import "embed"

//go:embed *.html *.css
var StaticFS embed.FS
