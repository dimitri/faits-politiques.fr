// Package sitegen génère le site statique à partir de core — l'ancien
// binaire séparé fpbuild, importé directement par cmd/fpctl (voir Run)
// plutôt qu'exécuté en sous-processus. fpctl et fpbuild ne font plus qu'un
// seul binaire : plus de recompilation à la volée ni de second exécutable
// à trouver dans bin/.
//
// Aucune donnée n'est calculée ici qui ne vienne de la base : le rendu est une
// projection, pas une source. Toute page publiée doit pouvoir remonter à un
// document scellé (docs/perimetre.md §5.1).
package sitegen

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

var positionFr = map[string]string{
	"FOR": "Pour", "AGAINST": "Contre", "ABSTAIN": "Abstention",
	"ABSENT": "Non-votant", "NON_VOTING": "N'a pas pris part",
}

type SourceInfo struct{ Attribution, Label, Fetched, SHA string }

type Coverage struct {
	Scrutins, Ballots, Deputes, Orgs int
	Dossiers                         int
	Candidats, CandidatsAvecBilan    int
	CandidatsPrimaire                int
	Organisations                    int
	Documents                        int64
	ScrutinsPE, Themes               int
	ScrutinsSenat, VotesSenat        int
	Senateurs, ThemesSenat           int
	Communes                         int
}

type Layout struct {
	Title, Root, BuiltAt string
	// Noms des ressources partagées, empreinte comprise. Voir assets.go.
	CSS, JS             string
	Hero                bool
	HeroTitre, HeroLede string
	// HeroVisuel : un graphique à côté du titre (accueil), sous le titre sur mobile.
	HeroVisuel        template.HTML
	DerniereIngestion string
	Sources           []SourceInfo
	Cov               Coverage
	// Partage social (og:*, twitter:*) — voir internal/sitegen/social.go.
	// Description et Image sont vides par défaut ; base.gohtml retombe alors
	// sur une description générique et sur og-defaut.png. CanonicalBase est le
	// même pour toutes les pages : le domaine réel, jamais .Root (qui sert au
	// préfixe de prévisualisation locale, pas à une URL absolue).
	Description    string
	Image          string
	ImageW, ImageH int
	CanonicalBase  string
}

type Vote struct {
	Slug, Objet, Date, Position, PositionFr, Resultat string
	Rectifiee                                         bool
}

type Mandat struct {
	Type, Circo, Periode, Role, Portefeuille string
	DebutISO, FinISO                         string
	SousPresidence                           string
	CommuneCode                              string
	Lieux                                    []Lieu
}
type Affil struct{ Nom, Kind, Periode, Via string }

type Person struct {
	ID                                  int64
	Slug, Prenom, Nom, Groupe, Mandat   string
	Mandats                             []Mandat
	Affiliations                        []Affil
	Pour, Contre, Abstention, NonVotant int
	PctPour, PctContre, PctAbst         int
	Exprimes, PctExprimes, VotesShown   int
	HasVotes                            bool
	Votes                               []Vote
}

type Candidat struct {
	Slug, Nom, Prenom, Organisation, Statut     string
	DateDeclaration, SourceURL, SourceConsultee string
	SiteCampagne                                string
	OrganisationSlug                            string
	Person                                      *Person
	Org                                         *Organisation
	Portrait                                    *Media
}

// Run construit le site avec args (les options -out/-only/-max-scrutins...
// de l'ancien fpbuild, telles quelles) et retourne une erreur au lieu de
// faire os.Exit — un flag.NewFlagSet à soi, jamais le FlagSet global de
// process (celui que fpctl utilise déjà pour ses propres drapeaux) : les
// deux ne doivent jamais se marcher dessus maintenant qu'ils vivent dans
// le même binaire.
func Run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	out := fs.String("out", "site", "répertoire de sortie")
	tpl := fs.String("templates", "web/templates", "gabarits")
	dataDir := fs.String("data", "data", "décisions éditoriales")
	root := fs.String("root", "", "préfixe d'URL")
	maxScrutins := fs.Int("max-scrutins", 0, "limite de pages scrutin (0 = toutes)")
	only := fs.String("only", "", "limite les pages reconstruites à cette liste (séparée par des virgules) : scrutin, communes, "+
		"ou l'un des noms de page filtrés par ecrire() plus bas (dette, chomage, securite, sujet:<id>, comprendre:<slug>...) — "+
		"voir cmd/fpctl/build.go, SECTIONS pour les regroupements par catégorie (fpctl build <catégorie>). Vide = tout. "+
		"À réserver à l'itération locale — un site construit avec -only est incomplet et ne doit jamais être mis en place tel quel.")
	cpuProfile := fs.String("cpuprofile", "", "écrit un profil CPU pprof à ce chemin (diagnostic, pas d'usage courant)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}
		defer pprof.StopCPUProfile()
	}

	// Le site est construit À CÔTÉ, puis mis en place d'un coup.
	//
	// Construire directement dans le répertoire servi commençait par l'effacer :
	// pendant les dix minutes de la construction, le serveur répondait 404 sur
	// toutes les pages pas encore réécrites — une page de département « toute
	// blanche » en pleine consultation. Le répertoire de construction est
	// désormais distinct, et l'échange final ne laisse le site absent que le
	// temps de deux renommages. Si la construction échoue, le site en ligne
	// n'est pas touché.
	chantier := strings.TrimRight(*out, "/") + ".construction"
	if err := run(ctx, chantier, *tpl, *dataDir, *root, *maxScrutins, *only); err != nil {
		if errors.Is(err, errRienAFaire) {
			// run() est retourné avant même de créer chantier/ : rien à
			// mettre en place, le site publié est déjà à jour.
			return nil
		}
		return err
	}
	// -only produit un site DÉLIBÉRÉMENT incomplet — jamais ce que sert le
	// domaine réel. Le refus est ici, pas seulement dans la documentation du
	// drapeau : une commande tapée vite un jour de correctif ne doit pas
	// pouvoir vider /collectivites/commune/ ou /scrutin/ en production.
	if *only != "" {
		fmt.Printf("-only=%s : site partiel conservé dans %s/, PAS mis en place. "+
			"Inspectez-le, puis relancez sans -only pour publier.\n", *only, chantier)
		return nil
	}
	if err := mettreEnPlace(chantier, *out); err != nil {
		return fmt.Errorf("mise en place : %w", err)
	}
	return nil
}

// mettreEnPlace remplace le site servi par celui qui vient d'être construit.
// L'ancien est renommé avant d'être effacé : le serveur ne voit jamais un
// répertoire à moitié supprimé.
func mettreEnPlace(chantier, out string) error {
	ancien := strings.TrimRight(out, "/") + ".precedent"
	if err := os.RemoveAll(ancien); err != nil {
		return err
	}
	if _, err := os.Stat(out); err == nil {
		if _, err := os.Stat(filepath.Join(out, marqueurSortie)); err != nil {
			return fmt.Errorf("%s ne porte pas %s : refus de le remplacer", out, marqueurSortie)
		}
		if err := os.Rename(out, ancien); err != nil {
			return err
		}
	}
	if err := os.Rename(chantier, out); err != nil {
		return err
	}
	return os.RemoveAll(ancien)
}

func run(ctx context.Context, out, tplDir, dataDir, root string, maxScrutins int, only string) error {
	start := time.Now()
	// exclu : true si -only laisse cette section de côté. Vide (par défaut)
	// ne filtre rien — c'est la reconstruction complète que mettreEnPlace
	// doit voir ; -only sert à l'itération locale, jamais au déploiement.
	exclu := func(section string) bool {
		return only != "" && !strings.Contains(","+only+",", ","+section+",")
	}
	// ecrire : un filtre posé sur write(), inchangé depuis avant ce graphe —
	// mais chaque section n'est plus qu'une CIBLE parmi d'autres du registre
	// (voir graphe.go, construireRegistre) : demander -only=X ne charge plus
	// que ce dont X dépend réellement, la fermeture transitive calculée par
	// internal/pipeline.Registre.Niveaux, jamais la totalité du site.
	ecrire := func(section string, t *template.Template, path string, data any) error {
		if exclu(section) {
			return nil
		}
		return write(t, path, data)
	}
	// 8, pas le défaut de 4 : plusieurs requêtes indépendantes tournent
	// désormais de front (internal/sitegen/lieux_pages.go, et depuis ce
	// graphe une bonne partie du reste du site aussi) sur une machine qui a
	// sa propre Postgres, pas une base managée partagée entre instances —
	// voir internal/store.OpenWithMaxConns.
	pool, err := store.OpenWithMaxConns(ctx, 8)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Cache de construction (internal/sitegen/cache.go) : ancien est ce que la
	// construction précédente a produit, lu dans le site actuellement publié
	// (out est le chantier, pas encore mis en place) ; nouveau est réécrit à
	// la fin, quoi qu'il arrive, pour que la prochaine construction ait
	// quelque chose à comparer même si tout a été refait cette fois.
	siteActuel := strings.TrimSuffix(out, ".construction")
	ancienCache := chargerManifeste(siteActuel)
	nouveauCache := manifesteCache{Sections: map[string]etatSection{}}

	// Court-circuit global : avant tout chargement, la question la moins
	// chère à poser est aussi la plus rentable — rien n'a changé DU TOUT
	// depuis la dernière construction ? -only et -max-scrutins produisent
	// délibérément un site partiel : jamais la référence à laquelle comparer.
	if only == "" && maxScrutins == 0 {
		communesOK, _, err := sectionInchangee(ctx, pool, ancienCache, "communes")
		if err != nil {
			return err
		}
		scrutinOK, _, err := sectionInchangee(ctx, pool, ancienCache, "scrutin")
		if err != nil {
			return err
		}
		resteOK, _, err := resteInchange(ctx, pool, ancienCache)
		if err != nil {
			return err
		}
		if communesOK && scrutinOK && resteOK {
			fmt.Printf("rien n'a changé depuis la dernière construction (%s écoulées)\n",
				time.Since(start).Round(time.Millisecond))
			return errRienAFaire
		}
	}

	fns := template.FuncMap{"jauge": Jauge, "poleG": PoleGauche, "poleD": PoleDroit,
		"lower": strings.ToLower, "nb": Nombre, "octets": Octets, "ico": Icone,
		"marque": Marque, "grille": Grille, "pct": Pourcent, "nb64": Nombre64,
		"mdEur": mdEur, "pctFr": pctFr, "dec": Decimal, "eurHab": eurHab, "montant": Montant,
		"echelon": func(t string) string { return libelleEchelon[t] },
		"risqueCouleur": func(code string) string {
			if c := couleurRisque[code]; c != "" {
				return c
			}
			return "#8A7F6B"
		},
		"libMandat": libelleMandat,
		// dict : passer plusieurs valeurs à un sous-gabarit, qui n'en reçoit
		// qu'une. Sert à transmettre Root avec la liste des décrets.
		"dict": func(kv ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i+1 < len(kv); i += 2 {
				m[fmt.Sprint(kv[i])] = kv[i+1]
			}
			return m
		},
		"sub64": func(a, b float64) float64 { return a - b },
		"add64": func(a, b float64) float64 { return a + b },
		"int":   func(f float64) int { return int(f) },
		"add":   func(a, b int) int { return a + b },
		"sub":   func(a, b int) int { return a - b },
		"mul":   func(a, b int) int { return a * b },
		"odd":   func(i int) bool { return i%2 == 1 }}
	base := template.Must(template.New("base.gohtml").Funcs(fns).
		ParseFiles(filepath.Join(tplDir, "base.gohtml")))
	page := func(name string) *template.Template {
		t := template.Must(base.Clone())
		return template.Must(t.ParseFiles(filepath.Join(tplDir, name)))
	}

	if err := preparerSortie(out); err != nil {
		return err
	}

	assets, err := copierAssets("web/assets", out)
	if err != nil {
		return err
	}
	// La date de construction et celle des données appartiennent au pied de
	// page, donc à TOUTES les pages : les poser sur la seule page d'accueil
	// laissait « Données arrêtées au . » partout ailleurs.
	layout := Layout{Root: root, BuiltAt: dateFr(time.Now()),
		CSS: assets.CSS, JS: assets.JS,
		// Le domaine réel : nécessaire pour og:image, dont la spécification
		// exige une URL absolue même quand .Root est vide (déploiement à la
		// racine, comme aujourd'hui). Pas un drapeau de ligne de commande —
		// ce site n'a qu'un domaine, comme NonCompense ailleurs est une
		// constante plutôt qu'un paramètre pour une valeur qui ne varie pas.
		CanonicalBase: "https://faits-politiques.fr",
		Image:         "/media/og-defaut.png", ImageW: 1200, ImageH: 630}
	if layout.Sources, err = sources(ctx, pool); err != nil {
		return err
	}
	if layout.Cov, err = coverage(ctx, pool); err != nil {
		return err
	}
	_ = pool.QueryRow(ctx, `
		SELECT coalesce(to_char(max(fetched_at),'DD/MM/YYYY'),'')
		FROM raw.retrieval WHERE document_id IS NOT NULL`).Scan(&layout.DerniereIngestion)
	fmt.Printf("  chargement initial : %s écoulées\n", time.Since(start).Round(time.Second))

	env := &environnement{
		pool: pool, dataDir: dataDir, out: out, root: root, start: start,
		layout: layout, page: page, ecrire: ecrire, write: write, exclu: exclu,
		maxScrutins: maxScrutins, ancienCache: ancienCache, siteActuel: siteActuel,
		nouveauCache: &nouveauCache,
	}
	reg := construireRegistre(env)
	cibles := ciblesDe(only)

	// runtime.NumCPU(), pas la Concurrence -j de l'ingest (qui n'existe pas
	// ici) : la plupart des nœuds d'une même vague sont des requêtes
	// indépendantes contre le même pool à 8 connexions (OpenWithMaxConns
	// ci-dessus) — au-delà, une vague large mettrait simplement en file
	// d'attente plutôt que d'accélérer quoi que ce soit.
	resultats, err := reg.Executer(ctx, cibles, pipeline.Options{Concurrence: runtime.NumCPU()})
	if err != nil {
		return err
	}

	// Page d'erreur 404 : un fichier à la racine (pas .../404/index.html),
	// pour que le Caddyfile puisse la servir par un simple
	// rewrite * /{http.error.status_code}.html dans un bloc handle_errors,
	// sans connaître l'arborescence du site.
	l := layout
	l.Title = "Page introuvable"
	if err := write(page("404.gohtml"), filepath.Join(out, "404.html"), l); err != nil {
		return err
	}

	// Plan du site et robots.txt : en dernier, une fois que out/ porte
	// exactement l'arborescence publiée — voir internal/sitegen/sitemap.go.
	nSitemap, err := ecrireSitemap(out, layout.CanonicalBase)
	if err != nil {
		return err
	}
	if err := ecrireRobots(out, layout.CanonicalBase); err != nil {
		return err
	}
	fmt.Printf("  plan du site : %d URL, %s\n", nSitemap, layout.CanonicalBase+"/sitemap.xml")

	id := dep[identiteBundle](resultats, "identite")
	n, _ := resultats["scrutin"].(int)
	fmt.Printf("site généré dans %s/ : %d députés, %d candidats, %d organisations, %d groupes, %d scrutins (%s)\n",
		out, len(id.Persons), len(id.Candidats), len(id.Orgs), len(id.Groupes), n, time.Since(start).Round(time.Millisecond))

	// « reste » (voir resteInchange) : comme pour scrutin, jamais à partir
	// d'une construction tronquée par -only/-max-scrutins — elle serait prise
	// pour une construction complète par le prochain lancement, sans troncature.
	if only == "" && maxScrutins == 0 {
		_, etatReste, err := resteInchange(ctx, pool, ancienCache)
		if err != nil {
			return err
		}
		nouveauCache.Sections["reste"] = etatReste
	}

	// Écrit quoi qu'il arrive, y compris pour une section refaite cette fois :
	// c'est cet état-ci, celui que ce chantier vient de produire, auquel la
	// prochaine construction devra se comparer une fois mis en place.
	if err := nouveauCache.ecrire(out); err != nil {
		return err
	}
	return nil
}

// trierPersonnes : ordre alphabétique sur le nom puis le prénom. Ce site ne
// classe pas les personnes autrement.
func trierPersonnes(l []*Person) {
	sort.Slice(l, func(i, j int) bool {
		return CleTri(l[i].Nom+" "+l[i].Prenom) < CleTri(l[j].Nom+" "+l[j].Prenom)
	})
}

// Le site se veut « régénéré intégralement depuis l'archive brute » — c'est
// écrit au pied de chaque page. Sans nettoyage, c'était faux : le répertoire de
// sortie accumulait les pages des périmètres précédents. Après le resserrement
// des fiches personnes aux mandats nationaux, 34 744 fiches de maires y
// subsistaient, servies alors qu'aucune construction ne les produisait plus.
//
// On n'efface que ce que ce programme a écrit : le marqueur en atteste. Un
// répertoire non vide qui ne le porte pas fait échouer la construction plutôt
// que d'être supprimé.
const marqueurSortie = ".site-genere"

func preparerSortie(out string) error {
	info, err := os.Stat(out)
	if os.IsNotExist(err) {
		return ecrireMarqueur(out)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s n'est pas un répertoire", out)
	}
	if _, err := os.Stat(filepath.Join(out, marqueurSortie)); err != nil {
		entries, err := os.ReadDir(out)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("%s n'est pas vide et ne porte pas %s : "+
				"refus de l'effacer", out, marqueurSortie)
		}
	} else if err := os.RemoveAll(out); err != nil {
		return err
	}
	return ecrireMarqueur(out)
}

func ecrireMarqueur(out string) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, marqueurSortie),
		[]byte("Répertoire produit par internal/sitegen. Effacé et réécrit à chaque construction.\n"), 0o644)
}

func write(t *template.Template, path string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// La page est rendue en mémoire puis relue pour la ponctuation française :
	// c'est le seul point de passage commun aux gabarits, aux libellés venus de
	// la base et aux documents Markdown de « Comprendre ».
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base", data); err != nil {
		return err
	}
	return os.WriteFile(path, corrigerTypographie(buf.Bytes()), 0o644)
}

func sources(ctx context.Context, pool *pgxpool.Pool) ([]SourceInfo, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT ON (s.slug) s.attribution_text, s.label,
		       to_char(r.fetched_at,'DD/MM/YYYY'), encode(d.sha256,'hex')
		FROM raw.source s
		JOIN raw.retrieval r ON r.source_id = s.id AND r.document_id IS NOT NULL
		JOIN raw.document d ON d.id = r.document_id
		ORDER BY s.slug, r.fetched_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceInfo
	for rows.Next() {
		var s SourceInfo
		if err := rows.Scan(&s.Attribution, &s.Label, &s.Fetched, &s.SHA); err != nil {
			return nil, err
		}
		s.SHA = s.SHA[:16] + "…"
		out = append(out, s)
	}
	return out, rows.Err()
}

func coverage(ctx context.Context, pool *pgxpool.Pool) (Coverage, error) {
	var c Coverage
	// « personnes » comptait tous les acteurs historiques publiés par
	// l'Assemblée, pas les députés : la page annonçait 3 127 députés pour une
	// assemblée qui en compte 577. Le décompte porte désormais sur les
	// personnes ayant effectivement détenu un mandat de député.
	// mv.scrutin_vote_nominal (internal/matview) remplace core.ballot ici :
	// aucune de ces trois requêtes ne distingue position/position_rectifiee
	// (seulement des count(*)/count(DISTINCT)), donc rien à préserver de ce
	// côté — voir loadPersons (load.go) pour le cas où ça compterait.
	err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM mv.scrutin_vote_nominal mv JOIN core.scrutin s ON s.id=mv.scrutin_id
		         WHERE s.institution='ASSEMBLEE_NATIONALE'),
		       (SELECT count(DISTINCT person_id) FROM core.mandate WHERE mandate_type = 'DEPUTE'),
		       (SELECT count(*) FROM core.organization),
		       (SELECT count(*) FROM core.scrutin WHERE institution='ASSEMBLEE_NATIONALE'),
		       (SELECT count(*) FROM core.dossier)`).
		Scan(&c.Ballots, &c.Deputes, &c.Orgs, &c.Scrutins, &c.Dossiers)
	if err != nil {
		return c, err
	}
	err = pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM raw.document),
		       (SELECT count(*) FROM core.scrutin WHERE institution='PARLEMENT_EUROPEEN'),
		       (SELECT count(DISTINCT topic_code) FROM core.topic_assignment),
		       (SELECT count(*) FROM core.scrutin WHERE institution='SENAT'),
		       (SELECT count(*) FROM mv.scrutin_vote_nominal mv JOIN core.scrutin s ON s.id=mv.scrutin_id
		         WHERE s.institution='SENAT'),
		       (SELECT count(DISTINCT mv.person_id) FROM mv.scrutin_vote_nominal mv
		         JOIN core.scrutin s ON s.id=mv.scrutin_id WHERE s.institution='SENAT'),
		       (SELECT count(*) FROM ref.topic WHERE taxonomy_version='senat'),
		       (SELECT count(DISTINCT commune_code) FROM core.commune_indicator)`).
		Scan(&c.Documents, &c.ScrutinsPE, &c.Themes,
			&c.ScrutinsSenat, &c.VotesSenat, &c.Senateurs, &c.ThemesSenat, &c.Communes)
	return c, err
}
