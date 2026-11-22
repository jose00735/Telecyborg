// TeleCyborg: a Matrix-styled teleprompter that listens, suggests answers and follows your voice.
// See README.md and docs/requirements.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/jose00735/telecyborg/internal/server"
	"github.com/jose00735/telecyborg/internal/store"
)

var version = "v1.0.0-dev"

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
	if exe, err := os.Executable(); err == nil {
		srv.Speech.ExeDir = filepath.Dir(exe)
	}
	srv.Open = openURL
	url, err := srv.Listen(*port)
	if err != nil {
		fatal("could not start the local server: %v", err)
	}
	go func() {
		if err := srv.Serve(); err != nil {
			log.Println("server stopped:", err)
		}
	}()

	// never leave the speech engine running behind us
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		srv.Speech.Stop()
		os.Exit(0)
	}()

	if *noOpen {
		fmt.Println("TeleCyborg", version, "listening on", url)
		done := make(chan struct{})
		srv.Quit = func() { close(done) }
		<-done
		srv.Speech.Stop()
		return
	}
	run(srv, url, *browser)
	srv.Speech.Stop()
}

func fatal(format string, a ...any) {
	// (deferred cleanups do not run after os.Exit; the speech engine is not started yet here)
	msg := fmt.Sprintf(format, a...)
	showError(msg)
	os.Exit(1)
}
