// TeleCyborg: a Matrix-styled teleprompter (stage 1). See README.md and docs/requirements.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"

	"github.com/jose00735/telecyborg/internal/server"
	"github.com/jose00735/telecyborg/internal/store"
)

var version = "v0.1.0-dev"

//go:embed web
var webFS embed.FS

func main() {
	browser := flag.Bool("browser", false, "open in the default browser instead of the TeleCyborg window")
	port := flag.String("port", "0", "local port to listen on (0 = any free port)")
	dir := flag.String("data", store.DefaultDir(), "folder for scripts, settings and rehearsals")
	noOpen := flag.Bool("no-open", false, "only print the address (for development)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("TeleCyborg", version)
		return
	}

	st, err := store.Open(*dir)
	if err != nil {
		fatal("could not open the data folder %s: %v", *dir, err)
	}
	ui, _ := fs.Sub(webFS, "web")
	srv := server.New(st, ui, version)
	url, err := srv.Listen(*port)
	if err != nil {
		fatal("could not start the local server: %v", err)
	}
	go func() {
		if err := srv.Serve(); err != nil {
			log.Println("server stopped:", err)
		}
	}()

	if *noOpen {
		fmt.Println("TeleCyborg", version, "listening on", url)
		select {}
	}
	run(srv, url, *browser)
}

func fatal(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	showError(msg)
	os.Exit(1)
}
