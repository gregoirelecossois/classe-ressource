#!/bin/sh
# Construit ClasseRessource.exe (Windows 64 bits) à la racine du dépôt.
# Prérequis : Go 1.21+ ; pour l'icône, l'outil rsrc :
#   go install github.com/akavel/rsrc@latest
set -e
cd "$(dirname "$0")"
cp ../source/ClasseRessource.html ClasseRessource.html         # page intégrée à l'exécutable
if command -v rsrc >/dev/null 2>&1 || [ -x "$HOME/go/bin/rsrc" ]; then
  "$(command -v rsrc || echo "$HOME/go/bin/rsrc")" -ico icone.ico -arch amd64 -o rsrc_windows_amd64.syso
fi
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w -H=windowsgui" -o ../ClasseRessource.exe .
echo "OK : $(cd .. && pwd)/ClasseRessource.exe"
