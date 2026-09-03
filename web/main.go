package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
)

//go:embed static
var staticFS embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Serve the static/ subtree rooted at /, so /css/style.css works directly.
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))

	log.Printf("konfig-konector landing page → http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
