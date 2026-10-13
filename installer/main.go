//go:build windows

package main

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

//go:embed assets/SlangDiscovery.exe
var packed embed.FS
var user32 = syscall.NewLazyDLL("user32.dll")
var messageBox = user32.NewProc("MessageBoxW")

func message(text, title string, style uintptr) int {
	a, _ := syscall.UTF16PtrFromString(text)
	b, _ := syscall.UTF16PtrFromString(title)
	n, _, _ := messageBox.Call(0, uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b)), style)
	return int(n)
}
func psQuote(t string) string { return "'" + strings.ReplaceAll(t, "'", "''") + "'" }
func shortcut(target, install string) error {
	// WScript.Shell COM creates the real *.lnk files. No elevation or third-party installer required.
	script := fmt.Sprintf(`$shell = New-Object -ComObject WScript.Shell;
$desktop = $shell.SpecialFolders.Item('Desktop');
$link = $shell.CreateShortcut((Join-Path $desktop 'Slang Discovery Engine.lnk'));
$link.TargetPath = %s;
$link.WorkingDirectory = %s;
$link.Description = 'Slang Discovery Engine — local research console';
$link.Save();
$menu = [Environment]::GetFolderPath('StartMenu');
$lnk = $shell.CreateShortcut((Join-Path $menu 'Slang Discovery Engine.lnk'));
$lnk.TargetPath = %s;
$lnk.WorkingDirectory = %s;
$lnk.Save();`, psQuote(target), psQuote(install), psQuote(target), psQuote(install))
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	b, e := cmd.CombinedOutput()
	if e != nil {
		return fmt.Errorf("masaustu kisayolu yaratilamadi: %s (%w)", string(b), e)
	}
	return nil
}
func install() error {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return fmt.Errorf("LOCALAPPDATA bulunamadi")
	}
	dir := filepath.Join(base, "Programs", "SlangDiscoveryEngine")
	if e := os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	binary, e := packed.ReadFile("assets/SlangDiscovery.exe")
	if e != nil {
		return e
	}
	target := filepath.Join(dir, "SlangDiscovery.exe")
	temp := target + ".new"
	if e := os.WriteFile(temp, binary, 0755); e != nil {
		return e
	}
	// An active instance can block replacement; retain existing app and report the error.
	if e := os.Rename(temp, target); e != nil {
		_ = os.Remove(target)
		if e = os.Rename(temp, target); e != nil {
			return fmt.Errorf("uygulama acik olabilir; kapatin ve tekrar deneyin: %w", e)
		}
	}
	if e := shortcut(target, dir); e != nil {
		return e
	}
	return nil
}
func main() {
	if message("Slang Discovery Engine kurulacak.\n\nKonum: %LOCALAPPDATA%\\Programs\\SlangDiscoveryEngine\nMasaustu ve Baslat menusune kisayol eklenecek.\n\nDevam edilsin mi?", "Slang Discovery Engine / Setup", 0x24) != 6 {
		return
	}
	if e := install(); e != nil {
		message("Kurulum basarisiz:\n"+e.Error(), "Setup Hata", 0x10)
		return
	}
	if message("Kurulum tamamlandi.\n\nMasaustunde 'Slang Discovery Engine' kisayolu olusturuldu.\n\nSimdi uygulamayi acmak ister misiniz?", "Slang Discovery Engine", 0x24) == 6 {
		target := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "SlangDiscoveryEngine", "SlangDiscovery.exe")
		_ = exec.Command(target).Start()
	}
}
