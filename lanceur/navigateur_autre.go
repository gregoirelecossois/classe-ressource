//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Version de développement (Linux / macOS) : utile pour tester le lanceur.
func ouvrirNavigateur(url string) (<-chan struct{}, error) {
	if os.Getenv("CR_SANS_NAVIGATEUR") != "" {
		fmt.Println("Adresse :", url)
		return nil, nil
	}
	profil := filepath.Join(dossierProfil(), "Navigateur")
	for _, exe := range []string{"google-chrome", "chromium", "chromium-browser", "microsoft-edge"} {
		if p, err := exec.LookPath(exe); err == nil {
			cmd := exec.Command(p, "--app="+url, "--user-data-dir="+profil, "--no-first-run")
			if err := cmd.Start(); err == nil {
				fin := make(chan struct{})
				go func() { _ = cmd.Wait(); close(fin) }()
				return fin, nil
			}
		}
	}
	return nil, exec.Command("xdg-open", url).Start()
}

func afficherMessage(titre, msg string) { fmt.Fprintln(os.Stderr, titre+" : "+msg) }
