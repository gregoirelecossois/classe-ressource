//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

// candidats renvoie les navigateurs utilisables, Chrome d'abord, Edge ensuite.
func candidats() []string {
	var l []string
	for _, nom := range []string{"chrome.exe", "msedge.exe"} {
		for _, racine := range []syscall.Handle{syscall.HKEY_CURRENT_USER, syscall.HKEY_LOCAL_MACHINE} {
			if p := lireAppPath(racine, nom); p != "" {
				l = append(l, p)
			}
		}
		sous := `Google\Chrome\Application\chrome.exe`
		if nom == "msedge.exe" {
			sous = `Microsoft\Edge\Application\msedge.exe`
		}
		for _, v := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "LOCALAPPDATA"} {
			if d := os.Getenv(v); d != "" {
				l = append(l, filepath.Join(d, sous))
			}
		}
	}
	return l
}

// lireAppPath lit HK..\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\<nom>.
func lireAppPath(racine syscall.Handle, nom string) string {
	cle, _ := syscall.UTF16PtrFromString(`SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\` + nom)
	var h syscall.Handle
	if syscall.RegOpenKeyEx(racine, cle, 0, syscall.KEY_READ, &h) != nil {
		return ""
	}
	defer syscall.RegCloseKey(h)
	var typ, taille uint32
	if syscall.RegQueryValueEx(h, nil, nil, &typ, nil, &taille) != nil || taille == 0 {
		return ""
	}
	buf := make([]uint16, taille/2+1)
	if syscall.RegQueryValueEx(h, nil, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &taille) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// ouvrirNavigateur ouvre l'application dans une fenêtre dédiée. Le canal
// renvoyé se ferme quand cette fenêtre est fermée (nil si non suivie).
func ouvrirNavigateur(url string) (<-chan struct{}, error) {
	profil := filepath.Join(dossierProfil(), "Navigateur")
	devant := preparerPremierPlan()
	for _, exe := range candidats() {
		if st, err := os.Stat(exe); err != nil || st.IsDir() {
			continue
		}
		cmd := exec.Command(exe,
			"--app="+url,
			"--user-data-dir="+profil,
			"--no-first-run",
			"--no-default-browser-check",
			"--start-maximized",
		)
		if err := cmd.Start(); err != nil {
			continue
		}
		go devant()
		fin := make(chan struct{})
		go func() { _ = cmd.Wait(); close(fin) }()
		return fin, nil
	}
	// Ni Chrome ni Edge : navigateur par défaut.
	if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start(); err != nil {
		return nil, errors.New("aucun navigateur trouvé (Chrome ou Edge)")
	}
	go devant()
	return nil, nil
}

func afficherMessage(titre, msg string) {
	boite := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(titre)
	m, _ := syscall.UTF16PtrFromString(msg)
	_, _, _ = boite.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}
