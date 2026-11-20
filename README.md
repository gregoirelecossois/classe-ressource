# Classe Ressource

Application de suivi des passages en Classe Ressource.

## Lancer l'application : `ClasseRessource.exe`

Un double-clic sur **`ClasseRessource.exe`** suffit :

1. l'application s'ouvre dans une fenêtre Chrome dédiée (Edge si Chrome est absent) ;
2. elle se branche **toute seule** sur le dossier de données de l'année scolaire en cours,
   sans aucune autorisation à donner ;
3. on tape son nom, et on arrive sur le tableau de bord.

Quand on ferme la fenêtre, le lanceur s'arrête de lui-même.

### Où sont les données ?

Dans le **dossier qui contient `ClasseRessource.exe`** : l'exe se place au-dessus des
dossiers d'année, et choisit tout seul celui de l'année en cours (grâce au fichier
`annee.json` qu'il contient, quel que soit le nom du dossier) :

```
📁 Q:\CLASSE RESSOURCE\Application Classe Ressource
   ClasseRessource.exe
   📁 Données classe Ressource 2026 - 2027
   📁 Données classe Ressource 2027 - 2028   ← créé au passage d'année, sur le même modèle de nom
```

S'il trouve plusieurs dossiers pour la même année (une copie de sauvegarde, par exemple),
il écarte ceux dont le nom contient « Copie », « Sauvegarde », « Backup »…, prend celui dont
les saisies sont les plus récentes, et le signale à l'écran.

Tous les postes qui lancent **le même** `ClasseRessource.exe` (ou une copie posée dans le
même dossier) travaillent sur les mêmes données.

> Pour garder l'exécutable ailleurs (sur le bureau, par exemple), créez à côté de lui un
> fichier texte `dossier-donnees.txt` contenant uniquement le chemin du dossier de données,
> par exemple `\\serveur\partage\Classe Ressource` ou `S:\Classe Ressource`.

### Installation conseillée (une fois pour toutes)

1. Copiez `ClasseRessource.exe` dans le dossier partagé de l'établissement.
   *Vous utilisiez déjà l'application ?* Posez l'exe **dans le dossier qui contient votre
   dossier de données actuel** (par exemple `Application Classe Ressource`, au-dessus de
   `Données classe Ressource 2026 - 2027`) : il est retrouvé tel quel, rien n'est perdu.
2. Sur chaque poste : clic droit sur l'exe ▸ *Afficher plus d'options* ▸ *Envoyer vers* ▸
   *Bureau (créer un raccourci)*.
3. Au tout premier lancement, Windows peut afficher « Windows a protégé votre ordinateur »
   (l'exécutable n'est pas signé numériquement) : cliquez sur *Informations complémentaires*
   puis *Exécuter quand même*. Cela ne se produit qu'une fois. Si le fichier vient d'un
   téléchargement, vous pouvez aussi faire clic droit ▸ *Propriétés* ▸ cocher *Débloquer*.

### Changement d'année scolaire

À partir du **1er août**, au premier lancement, l'application propose de passer à la
nouvelle année :

- l'année écoulée reste **intacte** dans son dossier (élèves, passages, statistiques), avec
  une archive JSON + XLSX en plus, et reste consultable (*Réglages ▸ Données ▸ Consulter une
  autre année*) ;
- la nouvelle année reprend tous les paramétrages (motifs, provenances, suites,
  appréciations, classes, horaires, réglages) ;
- élèves et enseignants sont à réimporter depuis Pronote.

Les autres postes basculent d'eux-mêmes à leur lancement suivant. On peut aussi répondre
*Plus tard* : l'année écoulée s'ouvre alors en consultation seule.

### En cas de souci

- Une fenêtre « Avertissement de sécurité » à **chaque** lancement depuis le réseau :
  décochez *Toujours demander avant d'ouvrir ce fichier*, ou bien copiez l'exe sur chaque
  poste avec un `dossier-donnees.txt` qui pointe vers le dossier partagé.
- Le journal du lanceur se trouve dans `%LOCALAPPDATA%\ClasseRessource\lanceur.log`.
- « Dossier de données inaccessible » : vérifiez que le lecteur réseau est connecté, puis
  *Réessayer*.
- Aucun fichier HTML n'est nécessaire à côté de l'exe : l'application est intégrée dedans.
  Le dossier de l'application ne contient donc que `ClasseRessource.exe` et les dossiers
  d'année. (`source/ClasseRessource.html` reste disponible dans ce dépôt et fonctionne seul, par
  double-clic avec demande d'autorisation de Chrome, en cas de secours.)

## Pour les développeurs

Organisation du dépôt :

- `ClasseRessource.exe` : l'application à distribuer (le seul fichier utile aux collègues) ;
- `source/ClasseRessource.html` : le code de l'application (interface, données, statistiques) ;
- `lanceur/` : le code du lanceur Windows qui enferme la page dans l'exe.

Le lanceur (Go, sans dépendance) est dans `lanceur/`. Il sert la page sur
`http://127.0.0.1:47811` et lit/écrit les fichiers du dossier de données à la place du
navigateur ; la page imite pour cela les poignées de dossier de Chrome
(`DossierLanceur` / `FichierLanceur`), le reste du code étant identique dans les deux modes.

Reconstruire l'exécutable après une modification de `source/ClasseRessource.html` :

```bash
./lanceur/construire.sh
```
