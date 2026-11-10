//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"

	"github.com/jose00735/telecyborg/internal/server"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procSetWindowPos = user32.NewProc("SetWindowPos")
	procMessageBox   = user32.NewProc("MessageBoxW")
)

const (
	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpShowWindow = 0x0040
)

type nativeWindow struct{ w webview2.WebView }

func (n *nativeWindow) SetOnTop(on bool) error {
	if n.w == nil {
		return errors.New("no window")
	}
	after := ^uintptr(1) // HWND_NOTOPMOST = -2
	if on {
		after = ^uintptr(0) // HWND_TOPMOST = -1
	}
	done := make(chan error, 1)
	n.w.Dispatch(func() {
		hwnd := uintptr(n.w.Window())
		r, _, err := procSetWindowPos.Call(hwnd, after, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
		if r == 0 {
			done <- err
			return
		}
		done <- nil
	})
	return <-done
}

func run(srv *server.Server, url string, browser bool) {
	if !browser {
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			Debug:     os.Getenv("TELECYBORG_DEBUG") != "",
			AutoFocus: true,
			WindowOptions: webview2.WindowOptions{
				Title:  "TeleCyborg",
				Width:  1180,
				Height: 780,
				Center: true,
			},
		})
		if w != nil {
			nw := &nativeWindow{w: w}
			srv.Window = nw
			srv.Native = true
			srv.Quit = func() { w.Dispatch(func() { w.Terminate() }) }
			w.SetSize(760, 520, webview2.HintMin)
			w.Navigate(url)
			w.Run()
			w.Destroy()
			return
		}
		showError("TeleCyborg could not open its window because the Microsoft Edge WebView2 runtime is missing.\n\nIt will open in your browser instead. To get the window, install WebView2 from microsoft.com/edge/webview2.")
	}
	quit := make(chan struct{})
	srv.Quit = func() { close(quit) }
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	<-quit
}

func showError(msg string) {
	t, _ := syscall.UTF16PtrFromString("TeleCyborg")
	m, _ := syscall.UTF16PtrFromString(msg)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x30)
}
