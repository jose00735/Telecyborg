// Command hubtest runs the real phone-sync hub on a temporary data folder and accepts every
// pairing, for the Android interop test (android/app/src/test/.../InteropTest.kt).
//
//	go run ./tools/hubtest -dir /tmp/pc
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jose00735/telecyborg/internal/hub"
	"github.com/jose00735/telecyborg/internal/pack"
	"github.com/jose00735/telecyborg/internal/store"
)

func main() {
	dir := flag.String("dir", "", "data folder")
	port := flag.Int("port", 0, "port (0 = any)")
	flag.Parse()
	st, err := store.Open(*dir)
	if err != nil {
		panic(err)
	}
	ps, _ := pack.Open(*dir)
	h, err := hub.New(*dir, st, ps)
	if err != nil {
		panic(err)
	}
	h.Addr = fmt.Sprintf("127.0.0.1:%d", *port)
	if err := h.Start(); err != nil {
		panic(err)
	}
	h.OpenPairing()
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			if s := h.Status(true); s.Pending != nil && s.Pending.State == "compare" {
				fmt.Println("CODE", s.Pending.Code)
				_ = h.Decide(true)
			}
		}
	}()
	fmt.Printf("READY %d\n", h.Status(true).Port)
	_ = os.Stdout.Sync()
	select {}
}
