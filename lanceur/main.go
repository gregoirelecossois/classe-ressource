// Lanceur de la Classe Ressource.
//
// Double-clic sur ClasseRessource.exe :
//  1. un petit serveur local démarre (accessible uniquement depuis ce poste,
//     adresse 127.0.0.1) ; c'est lui qui lit et écrit les fichiers du dossier
//     de données, si bien que Chrome n'a AUCUNE autorisation à demander ;
//  2. Chrome (ou Edge à défaut) s'ouvre en mode application sur la page ;
//  3. quand la fenêtre est fermée, le lanceur s'arrête tout seul.
//
// Dossier de données : par défaut, le dossier qui contient ClasseRessource.exe.
// Pour en désigner un autre, écrire son chemin dans un fichier
// « dossier-donnees.txt » placé à côté de l'exécutable.
package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const versionLanceur = "1.0"

//go:embed ClasseRessource.html
var pageIntegree []byte

// Ports essayés dans l'ordre. Le port fait partie de l'« origine » de la page :
// il doit rester stable pour que le navigateur retrouve sa copie de secours.
var ports = []int{47811, 47812, 47813, 47814, 47815}

// attentePremierPlan : travail en cours pour amener la fenêtre au premier plan.
var attentePremierPlan sync.WaitGroup

var (
	dossierExe  string // dossier contenant l'exécutable
	dossierBase string // dossier de données (racine servie)
	jeton       string // secret partagé avec la page servie
	adresse     string // http://127.0.0.1:port/
)

func main() {
	exe, err := os.Executable()
	if err == nil {
		if r, e := filepath.EvalSymlinks(exe); e == nil {
			exe = r
		}
		dossierExe = filepath.Dir(exe)
	} else {
		dossierExe, _ = os.Getwd()
	}
	ouvrirJournal()

	dossierBase, err = trouverDossierBase()
	if err != nil {
		erreurFatale("Dossier de données inaccessible :\n\n" + err.Error())
	}
	log.Printf("lanceur %s, dossier de données : %s", versionLanceur, dossierBase)

	jeton = aleatoire(16)
	ecouteur, existante := reserverPort()
	if existante != "" {
		// Le lanceur tourne déjà pour ce dossier : on rouvre simplement une fenêtre.
		log.Printf("instance déjà active sur %s", existante)
		if _, err := ouvrirNavigateur(existante); err != nil {
			erreurFatale("Impossible d'ouvrir le navigateur :\n\n" + err.Error())
		}
		attentePremierPlan.Wait() // le temps d'amener la nouvelle fenêtre devant
		return
	}
	if ecouteur == nil {
		erreurFatale("Aucun port local disponible pour démarrer la Classe Ressource.\n" +
			"Fermez les autres fenêtres de l'application puis réessayez.")
	}
	adresse = fmt.Sprintf("http://127.0.0.1:%d/", ecouteur.Addr().(*net.TCPAddr).Port)

	serveur := &http.Server{Handler: routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := serveur.Serve(ecouteur); err != nil && !errors.Is(err, http.ErrServerClosed) {
			erreurFatale("Le serveur local s'est arrêté :\n\n" + err.Error())
		}
	}()
	log.Printf("serveur prêt sur %s", adresse)

	attente, err := ouvrirNavigateur(adresse)
	if err != nil {
		erreurFatale("Impossible d'ouvrir Chrome ou Edge :\n\n" + err.Error())
	}
	surveillerFin(attente)
}

// ---------------------------------------------------------------------------
// Dossier de données

func trouverDossierBase() (string, error) {
	base := dossierExe
	// 1. argument --dossier=...
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "--dossier=") {
			base = strings.TrimPrefix(a, "--dossier=")
		}
	}
	// 2. fichier dossier-donnees.txt à côté de l'exécutable
	if b, err := os.ReadFile(filepath.Join(dossierExe, "dossier-donnees.txt")); err == nil {
		for _, l := range strings.Split(strings.TrimPrefix(string(b), "\ufeff"), "\n") {
			l = strings.Trim(strings.TrimSpace(l), `"`)
			if l != "" && !strings.HasPrefix(l, "#") {
				base = os.ExpandEnv(l)
				if !filepath.IsAbs(base) {
					base = filepath.Join(dossierExe, base)
				}
				break
			}
		}
	}
	base = filepath.Clean(base)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return base, fmt.Errorf("%s\n(%v)", base, err)
	}
	st, err := os.Stat(base)
	if err != nil || !st.IsDir() {
		return base, fmt.Errorf("%s n'est pas un dossier", base)
	}
	return base, nil
}

// cheminReel convertit un chemin relatif de la page ("a/b/c.json") en chemin
// disque, en interdisant toute sortie du dossier de données.
func cheminReel(rel string) (string, error) {
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return dossierBase, nil
	}
	parts := strings.Split(rel, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.ContainsAny(p, "\\:*?\"<>|\x00") {
			return "", fmt.Errorf("chemin refusé : %q", rel)
		}
	}
	return filepath.Join(append([]string{dossierBase}, parts...)...), nil
}

// ---------------------------------------------------------------------------
// Serveur HTTP

func routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/", servirPage)
	m.HandleFunc("/api/info", api(apiInfo, false))
	m.HandleFunc("/api/liste", api(apiListe, true))
	m.HandleFunc("/api/stat", api(apiStat, true))
	m.HandleFunc("/api/fichier", api(apiFichier, true))
	m.HandleFunc("/api/dossier", api(apiDossier, true))
	m.HandleFunc("/api/vie", api(apiVie, true))
	m.HandleFunc("/api/fin", apiFin)
	return m
}

// hoteLocal protège contre les pages web qui viseraient 127.0.0.1 par un nom
// de domaine détourné (« DNS rebinding »).
func hoteLocal(r *http.Request) bool {
	h := r.Host
	if i := strings.LastIndex(h, ":"); i >= 0 {
		h = h[:i]
	}
	return h == "127.0.0.1" || h == "localhost"
}

func api(f func(http.ResponseWriter, *http.Request), protege bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !hoteLocal(r) {
			http.Error(w, "hôte refusé", http.StatusForbidden)
			return
		}
		if protege && r.Header.Get("X-CR-Jeton") != jeton {
			repondreErreur(w, http.StatusForbidden, "NotAllowedError", "jeton invalide")
			return
		}
		f(w, r)
	}
}

func repondreJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func repondreErreur(w http.ResponseWriter, code int, nom, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"nom": nom, "message": msg})
}

func erreurDisque(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		repondreErreur(w, http.StatusNotFound, "NotFoundError", err.Error())
	case errors.Is(err, fs.ErrPermission):
		repondreErreur(w, http.StatusConflict, "NoModificationAllowedError", err.Error())
	default:
		repondreErreur(w, http.StatusInternalServerError, "Error", err.Error())
	}
}

func servirPage(w http.ResponseWriter, r *http.Request) {
	if !hoteLocal(r) {
		http.Error(w, "hôte refusé", http.StatusForbidden)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/index.html" && r.URL.Path != "/ClasseRessource.html" {
		http.NotFound(w, r)
		return
	}
	page := pageIntegree
	// Une version plus récente du fichier HTML posée à côté de l'exécutable est
	// prioritaire, à condition qu'elle sache fonctionner avec le lanceur.
	if b, err := os.ReadFile(filepath.Join(dossierExe, "ClasseRessource.html")); err == nil &&
		strings.Contains(string(b), "CR_LANCEUR") {
		page = b
	}
	infos, _ := json.Marshal(map[string]string{
		"jeton": jeton, "base": dossierBase, "nomBase": filepath.Base(dossierBase), "version": versionLanceur,
	})
	inj := []byte("<script>window.CR_LANCEUR=" + string(infos) + ";</script>")
	s := string(page)
	if i := strings.Index(strings.ToLower(s), "<head>"); i >= 0 {
		s = s[:i+6] + string(inj) + s[i+6:]
	} else {
		s = string(inj) + s
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, s)
}

func apiInfo(w http.ResponseWriter, r *http.Request) {
	repondreJSON(w, map[string]string{
		"app": "classe-ressource", "version": versionLanceur, "base": dossierBase,
	})
}

type entree struct {
	Nom     string `json:"nom"`
	Dossier bool   `json:"dossier"`
	Taille  int64  `json:"taille"`
	Modif   int64  `json:"modif"`
}

func decrire(nom string, st fs.FileInfo) entree {
	return entree{Nom: nom, Dossier: st.IsDir(), Taille: st.Size(), Modif: st.ModTime().UnixMilli()}
}

func apiListe(w http.ResponseWriter, r *http.Request) {
	p, err := cheminReel(r.URL.Query().Get("chemin"))
	if err != nil {
		repondreErreur(w, http.StatusBadRequest, "TypeError", err.Error())
		return
	}
	lus, err := os.ReadDir(p)
	if err != nil {
		erreurDisque(w, err)
		return
	}
	res := []entree{}
	for _, e := range lus {
		if strings.HasPrefix(e.Name(), ".cr-tmp-") {
			continue
		}
		st, err := e.Info()
		if err != nil {
			continue
		}
		res = append(res, decrire(e.Name(), st))
	}
	repondreJSON(w, res)
}

func apiStat(w http.ResponseWriter, r *http.Request) {
	p, err := cheminReel(r.URL.Query().Get("chemin"))
	if err != nil {
		repondreErreur(w, http.StatusBadRequest, "TypeError", err.Error())
		return
	}
	st, err := os.Stat(p)
	if err != nil {
		erreurDisque(w, err)
		return
	}
	repondreJSON(w, decrire(st.Name(), st))
}

func apiDossier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "méthode", http.StatusMethodNotAllowed)
		return
	}
	p, err := cheminReel(r.URL.Query().Get("chemin"))
	if err != nil {
		repondreErreur(w, http.StatusBadRequest, "TypeError", err.Error())
		return
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		erreurDisque(w, err)
		return
	}
	st, _ := os.Stat(p)
	repondreJSON(w, decrire(st.Name(), st))
}

var verrouEcriture sync.Mutex

func apiFichier(w http.ResponseWriter, r *http.Request) {
	p, err := cheminReel(r.URL.Query().Get("chemin"))
	if err != nil || p == dossierBase {
		repondreErreur(w, http.StatusBadRequest, "TypeError", "chemin de fichier invalide")
		return
	}
	switch r.Method {
	case http.MethodGet:
		f, err := os.Open(p)
		if err != nil {
			erreurDisque(w, err)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			repondreErreur(w, http.StatusNotFound, "TypeMismatchError", "pas un fichier")
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-CR-Modif", fmt.Sprint(st.ModTime().UnixMilli()))
		_, _ = io.Copy(w, f)

	case http.MethodPut:
		corps, err := io.ReadAll(io.LimitReader(r.Body, 512<<20))
		if err != nil {
			repondreErreur(w, http.StatusBadRequest, "Error", err.Error())
			return
		}
		verrouEcriture.Lock()
		err = ecrireAtomique(p, corps)
		verrouEcriture.Unlock()
		if err != nil {
			log.Printf("écriture %s : %v", p, err)
			erreurDisque(w, err)
			return
		}
		st, err := os.Stat(p)
		if err != nil {
			erreurDisque(w, err)
			return
		}
		repondreJSON(w, decrire(st.Name(), st))

	default:
		http.Error(w, "méthode", http.StatusMethodNotAllowed)
	}
}

// ecrireAtomique écrit d'abord un fichier temporaire puis le renomme : un
// fichier n'est jamais laissé à moitié écrit, même en cas de coupure réseau.
func ecrireAtomique(p string, corps []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(p), ".cr-tmp-"+aleatoire(6))
	if err := os.WriteFile(tmp, corps, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		// Certains partages réseau refusent le remplacement par renommage :
		// écriture directe en dernier recours.
		return os.WriteFile(p, corps, 0o644)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Durée de vie : le lanceur s'arrête quand plus aucune fenêtre n'est ouverte.

var (
	verrouPages sync.Mutex
	pages       = map[string]time.Time{}
	pageVue     bool
)

func apiVie(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("page")
	if id != "" {
		verrouPages.Lock()
		pages[id] = time.Now()
		pageVue = true
		verrouPages.Unlock()
	}
	repondreJSON(w, map[string]bool{"ok": true})
}

// apiFin est appelée par navigator.sendBeacon à la fermeture de la page
// (sendBeacon ne permet pas d'en-tête : le jeton passe dans l'adresse).
func apiFin(w http.ResponseWriter, r *http.Request) {
	if !hoteLocal(r) || r.URL.Query().Get("jeton") != jeton {
		http.Error(w, "refusé", http.StatusForbidden)
		return
	}
	verrouPages.Lock()
	delete(pages, r.URL.Query().Get("page"))
	verrouPages.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// surveillerFin bloque jusqu'à ce que l'application ne soit plus utilisée.
// attente se ferme quand la fenêtre du navigateur lancé par nous se ferme
// (nil si le navigateur n'a pas pu être suivi).
func surveillerFin(attente <-chan struct{}) {
	debut := time.Now()
	navigateurOuvert := attente != nil
	tic := time.NewTicker(3 * time.Second)
	defer tic.Stop()
	for {
		select {
		case <-attente:
			navigateurOuvert = false
			attente = nil
			log.Printf("fenêtre du navigateur fermée")
		case <-tic.C:
		}
		if navigateurOuvert {
			continue
		}
		verrouPages.Lock()
		vivantes := 0
		for id, t := range pages {
			// une page en arrière-plan peut n'envoyer de signal qu'une fois par minute
			if time.Since(t) < 150*time.Second {
				vivantes++
			} else {
				delete(pages, id)
			}
		}
		vue := pageVue
		verrouPages.Unlock()
		if vivantes > 0 {
			continue
		}
		if !vue && time.Since(debut) < 2*time.Minute {
			continue // le navigateur met peut-être du temps à démarrer
		}
		log.Printf("plus aucune fenêtre ouverte : arrêt")
		return
	}
}

// ---------------------------------------------------------------------------
// Divers

func reserverPort() (net.Listener, string) {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	for _, p := range ports {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return l, ""
		}
		// Port occupé : est-ce déjà nous, pour le même dossier ?
		url := fmt.Sprintf("http://127.0.0.1:%d/", p)
		rep, err := client.Get(url + "api/info")
		if err != nil {
			continue
		}
		var info map[string]string
		_ = json.NewDecoder(rep.Body).Decode(&info)
		rep.Body.Close()
		if info["app"] == "classe-ressource" && strings.EqualFold(info["base"], dossierBase) {
			return nil, url
		}
	}
	return nil, ""
}

func aleatoire(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func dossierProfil() string {
	d, err := os.UserCacheDir() // %LOCALAPPDATA% sous Windows
	if err != nil || d == "" {
		d = os.TempDir()
	}
	return filepath.Join(d, "ClasseRessource")
}

func ouvrirJournal() {
	d := dossierProfil()
	_ = os.MkdirAll(d, 0o755)
	p := filepath.Join(d, "lanceur.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 1<<20 {
		_ = os.Remove(p)
	}
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(f, os.Stderr))
	}
}

func erreurFatale(msg string) {
	log.Printf("ERREUR : %s", msg)
	afficherMessage("Classe Ressource", msg)
	os.Exit(1)
}
