//go:build windows

// Command installer is TeleCyborg's Windows setup: a per-user install (no admin rights) into
// %LOCALAPPDATA%\Programs\TeleCyborg with Start menu and desktop shortcuts, an entry in
// Settings → Apps, and an uninstaller. Run with /S for a silent install, /uninstall to remove.
package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var version = "dev"

//go:embed payload
var payload embed.FS

const (
	appName   = "TeleCyborg"
	publisher = "Jose Acosta Hernandez"
	uninstKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\TeleCyborg`
	vcURL     = "https://aka.ms/vs/17/release/vc_redist.x64.exe"
)

var (
	user32     = syscall.NewLazyDLL("user32.dll")
	messageBox = user32.NewProc("MessageBoxW")
	silent     bool
)

const (
	mbOK         = 0x0
	mbYesNo      = 0x4
	mbIconInfo   = 0x40
	mbIconWarn   = 0x30
	mbIconError  = 0x10
	mbIconAsk    = 0x20
	mbDefButton2 = 0x100
	idYes        = 6
)

func box(text string, flags uintptr) int {
	if silent {
		return idYes
	}
	t, _ := syscall.UTF16PtrFromString(appName + " " + version + " setup")
	m, _ := syscall.UTF16PtrFromString(text)
	r, _, _ := messageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
	return int(r)
}

func fatal(msg string) {
	box(msg, mbOK|mbIconError)
	os.Exit(1)
}

func installDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(base, "Programs", appName)
}

func main() {
	uninstall := false
	for _, a := range os.Args[1:] {
		switch strings.ToLower(a) {
		case "/s", "-s", "/silent", "--silent":
			silent = true
		case "/uninstall", "-uninstall", "--uninstall":
			uninstall = true
		}
	}
	if strings.Contains(strings.ToLower(filepath.Base(os.Args[0])), "uninstall") {
		uninstall = true
	}
	if uninstall {
		doUninstall()
		return
	}
	doInstall()
}

func doInstall() {
	data, err := payload.ReadFile("payload/payload.zip")
	if err != nil {
		fatal("This setup was built without TeleCyborg inside it. Download the setup again from the GitHub release.")
	}
	dir := installDir()
	if box(fmt.Sprintf("Install %s %s?\n\nIt is installed for your user only (no admin rights needed) in:\n%s\n\nIncludes the offline speech engine (whisper.cpp). Your scripts stay in %%APPDATA%%\\TeleCyborg.", appName, version, dir), mbYesNo|mbIconAsk) != idYes {
		return
	}
	closeRunning(dir)
	if err := extract(data, dir); err != nil {
		fatal("Setup could not copy the files:\n" + err.Error() + "\n\nClose TeleCyborg if it is open and run setup again.")
	}
	self, err := os.Executable()
	if err == nil {
		_ = copyFile(self, filepath.Join(dir, "uninstall.exe"))
	}
	exe := filepath.Join(dir, "telecyborg.exe")
	if err := shortcuts(exe, dir); err != nil {
		box("TeleCyborg is installed, but the shortcuts could not be created:\n"+err.Error()+"\n\nYou can start it from "+exe, mbOK|mbIconWarn)
	}
	if err := register(dir, exe); err != nil {
		box("TeleCyborg is installed, but it could not be added to Settings → Apps:\n"+err.Error(), mbOK|mbIconWarn)
	}
	if needsVCRedist() {
		if box("The offline speech engine needs the Microsoft Visual C++ runtime, which is not on this PC.\n\nDownload and install it from Microsoft now (about 25 MB, needs admin approval)?\n\nYou can skip this: the teleprompter works without it, only listening needs it.", mbYesNo|mbIconAsk) == idYes {
			if err := installVCRedist(); err != nil {
				box("The Visual C++ runtime could not be installed:\n"+err.Error()+"\n\nGet it later from "+vcURL, mbOK|mbIconWarn)
			}
		}
	}
	if silent {
		return
	}
	if box(appName+" "+version+" is installed.\n\nOpen it now?", mbYesNo|mbIconInfo) == idYes {
		cmd := exec.Command(exe)
		cmd.Dir = dir
		_ = cmd.Start()
	}
}

// closeRunning asks TeleCyborg (and its speech engine) to close so files can be replaced.
func closeRunning(dir string) {
	for _, name := range []string{"telecyborg.exe", "whisper-server.exe"} {
		c := exec.Command("taskkill", "/IM", name, "/F")
		c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
		_ = c.Run()
	}
	time.Sleep(400 * time.Millisecond)
}

func extract(data []byte, dir string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range zr.File {
		name := filepath.FromSlash(f.Name)
		if strings.Contains(name, "..") || filepath.IsAbs(name) {
			continue
		}
		dst := filepath.Join(dir, name)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(dst, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		var werr error
		for try := 0; try < 10; try++ {
			out, err := os.Create(dst)
			if err != nil {
				werr = err
				time.Sleep(300 * time.Millisecond)
				continue
			}
			_, werr = io.Copy(out, rc)
			out.Close()
			break
		}
		rc.Close()
		if werr != nil {
			return fmt.Errorf("%s: %w", f.Name, werr)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o755)
}

func startMenuDir() string {
	return filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`)
}

func desktopDir() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Desktop")
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// shortcuts creates .lnk files through the WScript.Shell COM object (via PowerShell).
func shortcuts(exe, dir string) error {
	var b strings.Builder
	b.WriteString("$ErrorActionPreference='Stop';$s=New-Object -ComObject WScript.Shell;")
	for _, lnk := range []string{filepath.Join(startMenuDir(), appName+".lnk"), filepath.Join(desktopDir(), appName+".lnk")} {
		fmt.Fprintf(&b, "$l=$s.CreateShortcut(%s);$l.TargetPath=%s;$l.WorkingDirectory=%s;$l.IconLocation=%s;$l.Description='Teleprompter that listens and follows your voice';$l.Save();",
			psQuote(lnk), psQuote(exe), psQuote(dir), psQuote(exe+",0"))
	}
	c := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", b.String())
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func dirSizeKB(dir string) uint32 {
	var n int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return uint32(n / 1024)
}

func register(dir, exe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	un := filepath.Join(dir, "uninstall.exe")
	vals := map[string]string{
		"DisplayName": appName, "DisplayVersion": strings.TrimPrefix(version, "v"), "Publisher": publisher,
		"DisplayIcon": exe + ",0", "InstallLocation": dir, "UninstallString": `"` + un + `" /uninstall`,
		"QuietUninstallString": `"` + un + `" /uninstall /S`, "URLInfoAbout": "https://github.com/jose00735/telecyborg",
		"InstallDate": time.Now().Format("20060102"),
	}
	for name, v := range vals {
		if err := k.SetStringValue(name, v); err != nil {
			return err
		}
	}
	_ = k.SetDWordValue("NoModify", 1)
	_ = k.SetDWordValue("NoRepair", 1)
	_ = k.SetDWordValue("EstimatedSize", dirSizeKB(dir))
	return nil
}

func doUninstall() {
	dir := installDir()
	if box("Uninstall "+appName+"?", mbYesNo|mbIconAsk) != idYes {
		return
	}
	data := filepath.Join(os.Getenv("APPDATA"), appName)
	removeData := !silent && box("Also delete your scripts, content packs, rehearsal history and speech models?\n\n"+data+"\n\nChoose No to keep them for a later install.", mbYesNo|mbIconWarn|mbDefButton2) == idYes
	closeRunning(dir)
	_ = os.Remove(filepath.Join(startMenuDir(), appName+".lnk"))
	_ = os.Remove(filepath.Join(desktopDir(), appName+".lnk"))
	_ = registry.DeleteKey(registry.CURRENT_USER, uninstKey)
	if removeData {
		_ = os.RemoveAll(data)
	}
	// remove everything but this running uninstaller, then let cmd delete the folder after we exit
	ents, _ := os.ReadDir(dir)
	self, _ := os.Executable()
	for _, e := range ents {
		p := filepath.Join(dir, e.Name())
		if strings.EqualFold(p, self) {
			continue
		}
		_ = os.RemoveAll(p)
	}
	c := exec.Command("cmd.exe", "/C", "ping -n 3 127.0.0.1 >NUL & rmdir /S /Q \""+dir+"\"")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	_ = c.Start()
	box(appName+" was removed.", mbOK|mbIconInfo)
}

func needsVCRedist() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\x64`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err == nil {
		defer k.Close()
		if v, _, err := k.GetIntegerValue("Installed"); err == nil && v == 1 {
			return false
		}
	}
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return false
	}
	for _, f := range []string{"vcruntime140.dll", "vcruntime140_1.dll", "msvcp140.dll", "vcomp140.dll"} {
		if _, err := os.Stat(filepath.Join(sys, f)); err != nil {
			return true
		}
	}
	return false
}

func installVCRedist() error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(vcURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}
	tmp := filepath.Join(os.TempDir(), "vc_redist.x64.exe")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()
	defer os.Remove(tmp)
	// the redistributable asks for elevation itself
	c := exec.Command(tmp, "/install", "/passive", "/norestart")
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && (ee.ExitCode() == 1638 || ee.ExitCode() == 3010) {
			return nil // newer version present / restart needed
		}
		return err
	}
	return nil
}
