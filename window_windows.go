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
	procSetAffinity  = user32.NewProc("SetWindowDisplayAffinity")
	procDpiForSystem = user32.NewProc("GetDpiForSystem")
)

// iconID is the icon group in rsrc_windows_amd64.syso (1 is the manifest).
const iconID = 2

// scale converts 96-dpi sizes to the screen's, since the manifest makes TeleCyborg DPI aware.
func scale(n int) int {
	if procDpiForSystem.Find() != nil {
		return n
	}
	dpi, _, _ := procDpiForSystem.Call()
	if dpi < 96 {
		return n
	}
	return n * int(dpi) / 96
}

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

// SetHideFromCapture uses WDA_EXCLUDEFROMCAPTURE (Windows 10 2004+) so screen sharing and
// recordings show nothing where the prompter is.
func (n *nativeWindow) SetHideFromCapture(hide bool) error {
	if n.w == nil {
		return errors.New("no window")
	}
	var aff uintptr // WDA_NONE
	if hide {
		aff = 0x11 // WDA_EXCLUDEFROMCAPTURE
	}
	done := make(chan error, 1)
	n.w.Dispatch(func() {
		r, _, err := procSetAffinity.Call(uintptr(n.w.Window()), aff)
		if r == 0 {
			done <- errors.New("this version of Windows cannot hide the window from capture: " + err.Error())
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
				Width:  uint(scale(1180)),
				Height: uint(scale(780)),
				Center: true,
				IconId: iconID,
			},
		})
		if w != nil {
			nw := &nativeWindow{w: w}
			srv.Window = nw
			srv.Native = true
			srv.Quit = func() { w.Dispatch(func() { w.Terminate() }) }
			w.SetSize(scale(760), scale(520), webview2.HintMin)
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

func openURL(u string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
}

func showError(msg string) {
	t, _ := syscall.UTF16PtrFromString("TeleCyborg")
	m, _ := syscall.UTF16PtrFromString(msg)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x30)
}
