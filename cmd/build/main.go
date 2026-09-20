// Commande build : génère le site statique à partir de core.
//
// Aucune donnée n'est calculée ici qui ne vienne de la base : le rendu est une
// projection, pas une source. Toute page publiée doit pouvoir remonter à un
// document scellé (docs/perimetre.md §5.1).
package main

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

	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
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
	// Partage social (og:*, twitter:*) — voir cmd/build/social.go.
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

func main() {
	out := flag.String("out", "site", "répertoire de sortie")
	tpl := flag.String("templates", "web/templates", "gabarits")
	dataDir := flag.String("data", "data", "décisions éditoriales")
	root := flag.String("root", "", "préfixe d'URL")
	maxScrutins := flag.Int("max-scrutins", 0, "limite de pages scrutin (0 = toutes)")
	only := flag.String("only", "", "limite les sections coûteuses reconstruites : scrutin | communes | scrutin,communes (vide = tout). "+
		"À réserver à l'itération locale — un site construit avec -only est incomplet et ne doit jamais être mis en place tel quel.")
	cpuProfile := flag.String("cpuprofile", "", "écrit un profil CPU pprof à ce chemin (diagnostic, pas d'usage courant)")
	flag.Parse()

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "erreur : %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "erreur : %v\n", err)
			os.Exit(1)
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
	if err := run(chantier, *tpl, *dataDir, *root, *maxScrutins, *only); err != nil {
		if errors.Is(err, errRienAFaire) {
			// run() est retourné avant même de créer chantier/ : rien à
			// mettre en place, le site publié est déjà à jour.
			return
		}
		fmt.Fprintf(os.Stderr, "erreur : %v\n", err)
		os.Exit(1)
	}
	// -only produit un site DÉLIBÉRÉMENT incomplet — jamais ce que sert le
	// domaine réel. Le refus est ici, pas seulement dans la documentation du
	// drapeau : une commande tapée vite un jour de correctif ne doit pas
	// pouvoir vider /collectivites/commune/ ou /scrutin/ en production.
	if *only != "" {
		fmt.Printf("-only=%s : site partiel conservé dans %s/, PAS mis en place. "+
			"Inspectez-le, puis relancez sans -only pour publier.\n", *only, chantier)
		return
	}
	if err := mettreEnPlace(chantier, *out); err != nil {
		fmt.Fprintf(os.Stderr, "erreur à la mise en place : %v\n", err)
		os.Exit(1)
	}
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

func run(out, tplDir, dataDir, root string, maxScrutins int, only string) error {
	start := time.Now()
	// exclu : true si -only laisse cette section de côté. Vide (par défaut)
	// ne filtre rien — c'est la reconstruction complète que mettreEnPlace
	// doit voir ; -only sert à l'itération locale, jamais au déploiement.
	exclu := func(section string) bool {
		return only != "" && !strings.Contains(","+only+",", ","+section+",")
	}
	ctx := context.Background()
	// 8, pas le défaut de 4 : plusieurs requêtes indépendantes tournent
	// désormais de front (cmd/build/lieux_pages.go) sur une machine qui a sa
	// propre Postgres, pas une base managée partagée entre instances — voir
	// internal/store.OpenWithMaxConns.
	pool, err := store.OpenWithMaxConns(ctx, 8)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Cache de construction (cmd/build/cache.go) : ancien est ce que la
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
	fmt.Printf("  chargement initial : %s écoulées\n", time.Since(start).Round(time.Second))

	persons, err := loadPersons(ctx, pool, layout.Cov.Scrutins)
	if err != nil {
		return err
	}
	candidats, err := loadCandidats(filepath.Join(dataDir, "candidats.csv"), persons)
	if err != nil {
		return err
	}
	refs, tags, err := loadReferentiels(filepath.Join(dataDir, "referentiels.csv"))
	if err != nil {
		return err
	}
	orgs, err := loadOrganisations(ctx, pool, filepath.Join(dataDir, "organisations.csv"), tags)
	if err != nil {
		return err
	}
	groupes, err := loadGroupes(ctx, pool)
	if err != nil {
		return err
	}
	presidences, err := loadPresidences(filepath.Join(dataDir, "presidents.csv"))
	if err != nil {
		return err
	}
	for _, p := range persons {
		for i, m := range p.Mandats {
			if m.Type != "MINISTRE" {
				continue
			}
			if noms := presidencesDe(presidences, m.DebutISO, m.FinISO); len(noms) > 0 {
				p.Mandats[i].SousPresidence = strings.Join(noms, ", ")
			}
		}
	}

	coalitions, err := loadCoalitions(filepath.Join(dataDir, "coalitions.csv"), orgs)
	if err != nil {
		return err
	}
	// Un groupe porte une coalition dans son nom quand l'intitulé publié par
	// l'Assemblée la mentionne. C'est une constatation sur le libellé, pas une
	// affirmation sur la composition du groupe.
	for _, g := range groupes {
		for _, c := range coalitions {
			if strings.Contains(strings.ToLower(g.Nom), strings.ToLower(c.Libelle)) {
				g.Coalitions = append(g.Coalitions, c)
			}
		}
	}

	portraits, logos, err := loadMedias(ctx, pool)
	if err != nil {
		return err
	}
	if err := copyMedia("web/media", filepath.Join(out, "media")); err != nil {
		return err
	}
	// Fontes hébergées en propre : aucune requête vers un tiers, aucun traceur.
	if err := copyMedia("web/fonts", filepath.Join(out, "fonts")); err != nil {
		return err
	}
	for _, c := range candidats {
		c.Portrait = portraits[c.Slug]
	}
	for _, o := range orgs {
		if g, ok := groupes[o.GroupeANUID]; ok {
			o.Groupe = g
			g.Partis = append(g.Partis, o)
		}
		if o.OrgID != 0 {
			o.Logo = logos[o.OrgID]
		}
	}
	for _, c := range candidats {
		if o, ok := orgs[c.OrganisationSlug]; ok {
			c.Org = o
			o.Candidats = append(o.Candidats, c)
		}
	}
	layout.Cov.Candidats = len(candidats)
	for _, c := range candidats {
		if c.Person != nil && c.Person.HasVotes {
			layout.Cov.CandidatsAvecBilan++
		}
		// Une candidature à une primaire n'est pas encore une candidature à
		// l'élection : les pages les comptent à part.
		if c.Statut == "PRIMAIRE" {
			layout.Cov.CandidatsPrimaire++
		}
	}

	// --- accueil et pages de section
	deputes := make([]*Person, 0, len(persons))
	for _, p := range persons {
		deputes = append(deputes, p)
	}
	trierPersonnes(deputes)

	orgList := make([]*Organisation, 0, len(orgs))
	for _, o := range orgs {
		orgList = append(orgList, o)
	}
	sort.Slice(orgList, func(i, j int) bool {
		return CleTri(orgList[i].Libelle) < CleTri(orgList[j].Libelle)
	})

	grpList := make([]*Groupe, 0, len(groupes))
	for _, g := range groupes {
		grpList = append(grpList, g)
	}
	sort.Slice(grpList, func(i, j int) bool {
		return CleTri(grpList[i].Nom) < CleTri(grpList[j].Nom)
	})

	terr, err := loadTerritoires(ctx, pool)
	if err != nil {
		return err
	}

	derniers, err := derniersScrutins(ctx, pool, 60)
	if err != nil {
		return err
	}
	seuils, err := loadSeuils(filepath.Join(dataDir, "seuils.csv"))
	if err != nil {
		return err
	}

	layout.Cov.Organisations = len(orgs)
	_ = pool.QueryRow(ctx, `
		SELECT coalesce(to_char(max(fetched_at),'DD/MM/YYYY'),'')
		FROM raw.retrieval WHERE document_id IS NOT NULL`).Scan(&layout.DerniereIngestion)

	l := layout

	l = layout
	l.Title = "Candidats 2027"
	if err := write(page("candidats.gohtml"), filepath.Join(out, "candidats", "index.html"), struct {
		Layout
		Candidats []*Candidat
	}{l, candidats}); err != nil {
		return err
	}

	l = layout
	l.Title = "Partis"
	if err := write(page("partis.gohtml"), filepath.Join(out, "partis", "index.html"), struct {
		Layout
		Orgs []*Organisation
	}{l, orgList}); err != nil {
		return err
	}

	l = layout
	l.Title = "Assemblée nationale"
	if err := write(page("assemblee.gohtml"), filepath.Join(out, "assemblee", "index.html"), struct {
		Layout
		Groupes  []*Groupe
		Deputes  []*Person
		Scrutins []Vote
	}{l, grpList, deputes, derniers}); err != nil {
		return err
	}

	sources, err := loadSources(ctx, pool)
	if err != nil {
		return err
	}
	statsSources, err := chargerStatsGlobalesSources(ctx, pool)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Sources"
	if err := write(page("sources.gohtml"), filepath.Join(out, "sources", "index.html"), struct {
		Layout
		Flux  []SourceDetail
		Stats *StatsGlobalesSources
	}{l, sources, statsSources}); err != nil {
		return err
	}

	europe, err := loadEurope(ctx, pool)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Parlement européen"
	if err := write(page("europe.gohtml"), filepath.Join(out, "europe", "index.html"), struct {
		Layout
		E *StatsEurope
	}{l, europe}); err != nil {
		return err
	}

	// --- thèmes : index et une fiche par thème du Sénat
	var candSlugs []string
	orgParSlug := map[string]string{}
	for _, c := range candidats {
		if c.Person != nil {
			candSlugs = append(candSlugs, c.Person.Slug)
			orgParSlug[c.Person.Slug] = c.Organisation
		}
	}
	themes, err := loadThemes(ctx, pool, 60, candSlugs, orgParSlug)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Thèmes"
	if err := write(page("themes.gohtml"), filepath.Join(out, "themes", "index.html"), struct {
		Layout
		T *StatsThemes
	}{l, themes}); err != nil {
		return err
	}
	tth := page("theme.gohtml")
	for _, th := range themes.Themes {
		maxG := 0
		for _, g := range th.Groupes {
			if g.Total > maxG {
				maxG = g.Total
			}
		}
		l = layout
		l.Title = th.Label
		if err := write(tth, filepath.Join(out, "theme", th.Slug, "index.html"), struct {
			Layout
			Th        *Theme
			MaxGroupe int
		}{l, th, maxG}); err != nil {
			return err
		}
	}

	// --- Sénat : la section n'est plus « non couverte »
	senat, err := loadSenat(ctx, pool, persons)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Sénat"
	if err := write(page("senat.gohtml"), filepath.Join(out, "senat", "index.html"), struct {
		Layout
		Se *StatsSenat
	}{l, senat}); err != nil {
		return err
	}

	// --- cartes départementales, sécurité, présidentielle 2027 : les nouvelles
	// sections. Les cartes ex-/territoires/ vivent maintenant sous
	// /collectivites/carte/ (voir territoires.go, en tête de fichier) : une
	// page par carte, tracé fin, classement complet, série annuelle.
	tcd := page("carte-detail.gohtml")
	for _, c := range terr.Cartes {
		l = layout
		l.Title = c.Titre
		l.Description = c.Page.Question
		imageCarte(&l, out, "carte-"+c.Slug, c.Page.Carte.SVG)
		if err := write(tcd, filepath.Join(out, "collectivites", "carte", c.Slug, "index.html"),
			struct {
				Layout
				P PageCarte
			}{l, c.Page}); err != nil {
			return err
		}
	}
	// L'ancienne adresse ne renvoie plus un 404 muet : une page fixe, aussi
	// statique que le reste du site, qui pointe vers la nouvelle adresse.
	l = layout
	l.Title = "Page déplacée"
	if err := write(page("deplace.gohtml"), filepath.Join(out, "territoires", "index.html"),
		struct {
			Layout
			Vers, VersTitre string
		}{l, "/collectivites/", "Collectivités"}); err != nil {
		return err
	}
	for _, c := range terr.Cartes {
		l = layout
		l.Title = "Page déplacée"
		if err := write(page("deplace.gohtml"), filepath.Join(out, "territoires", c.Slug, "index.html"),
			struct {
				Layout
				Vers, VersTitre string
			}{l, "/collectivites/carte/" + c.Slug + "/", c.Titre}); err != nil {
			return err
		}
	}

	fr, err := loadFrise(ctx, pool, dataDir)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "La Ve République en chiffres"
	if err := write(page("frise.gohtml"), filepath.Join(out, "frise", "index.html"),
		struct {
			Layout
			F *StatsFrise
		}{l, fr}); err != nil {
		return err
	}

	det, err := loadDette(ctx, pool)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "La dette publique"
	if err := write(page("dette.gohtml"), filepath.Join(out, "dette", "index.html"),
		struct {
			Layout
			D *StatsDette
		}{l, det}); err != nil {
		return err
	}

	chom, err := loadChomage(ctx, pool)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Le taux de chômage"
	if err := write(page("chomage.gohtml"), filepath.Join(out, "chomage", "index.html"),
		struct {
			Layout
			C *StatsChomage
		}{l, chom}); err != nil {
		return err
	}

	// La vieillesse et la jeunesse : deux dossiers en miroir, voir
	// docs/vieillesse-donnees.md et docs/jeunesse-donnees.md. Chacun a sa
	// propre carte départementale ou régionale, sur le même modèle que
	// /collectivites/carte/ (voir territoires.go).
	vieil, err := loadVieillesse(ctx, pool)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "La vieillesse : combien, qui paie, et la dépendance"
	if err := write(page("vieillesse.gohtml"), filepath.Join(out, "vieillesse", "index.html"),
		struct {
			Layout
			V *StatsVieillesse
		}{l, vieil}); err != nil {
		return err
	}
	if vieil.CarteAPA.Slug != "" {
		l = layout
		l.Title = vieil.CarteAPA.Titre
		l.Description = vieil.CarteAPA.Question
		imageCarte(&l, out, "vieillesse-"+vieil.CarteAPA.Slug, vieil.CarteAPA.Page.Carte.SVG)
		if err := write(tcd, filepath.Join(out, "vieillesse", "carte", vieil.CarteAPA.Slug, "index.html"),
			struct {
				Layout
				P PageCarte
			}{l, vieil.CarteAPA.Page}); err != nil {
			return err
		}
	}

	jeun, err := loadJeunesse(ctx, pool)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "La jeunesse : études supérieures, apprentissage, premiers emplois"
	if err := write(page("jeunesse.gohtml"), filepath.Join(out, "jeunesse", "index.html"),
		struct {
			Layout
			J *StatsJeunesse
		}{l, jeun}); err != nil {
		return err
	}
	if jeun.CarteInsertion.Slug != "" {
		l = layout
		l.Title = jeun.CarteInsertion.Titre
		l.Description = jeun.CarteInsertion.Question
		imageCarte(&l, out, "jeunesse-"+jeun.CarteInsertion.Slug, jeun.CarteInsertion.Page.Carte.SVG)
		if err := write(tcd, filepath.Join(out, "jeunesse", "carte", jeun.CarteInsertion.Slug, "index.html"),
			struct {
				Layout
				P PageCarte
			}{l, jeun.CarteInsertion.Page}); err != nil {
			return err
		}
	}

	// Ce qu'il y a dans le dernier décile de niveau de vie (D10, non borné
	// dans le dossier pauvreté) : voir docs/repartition-richesse-donnees.md.
	riche, err := loadRichesse(ctx, pool)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "La répartition de la richesse en France"
	if err := write(page("richesse.gohtml"), filepath.Join(out, "richesse", "index.html"),
		struct {
			Layout
			R *StatsRichesse
		}{l, riche}); err != nil {
		return err
	}

	div, err := loadDividendes(ctx, pool)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Les dividendes versés"
	if err := write(page("dividendes.gohtml"), filepath.Join(out, "dividendes", "index.html"),
		struct {
			Layout
			D *StatsDividendes
		}{l, div}); err != nil {
		return err
	}

	sec, err := loadSecurite(ctx, pool)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Sécurité"
	// Pas de carte unique au niveau de l'index (chaque indicateur porte la
	// sienne) : la première sert de vignette représentative. Page.Carte, pas
	// Apercu — Apercu est la vignette miniature de la grille, trop petite
	// (viewBox réduit) pour être agrandie proprement en image de partage.
	if len(sec.Indicateurs) > 0 {
		imageCarte(&l, out, "securite", sec.Indicateurs[0].Page.Carte.SVG)
	}
	if err := write(page("securite.gohtml"), filepath.Join(out, "securite", "index.html"),
		struct {
			Layout
			S *StatsSecurite
		}{l, sec}); err != nil {
		return err
	}

	for _, ind := range sec.Indicateurs {
		l = layout
		l.Title = ind.Libelle
		l.Description = ind.Page.Question
		imageCarte(&l, out, "securite-"+ind.Slug, ind.Page.Carte.SVG)
		if err := write(tcd, filepath.Join(out, "securite", ind.Slug, "index.html"),
			struct {
				Layout
				P PageCarte
			}{l, ind.Page}); err != nil {
			return err
		}
	}

	// --- accueil : écrit après les cartes départementales et de sécurité,
	// dont il reprend quatre vignettes en onglets.
	acc, err := loadAccueil(ctx, pool, terr, sec)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Le budget réel de la France, sujet par sujet de la campagne 2027"
	l.Hero = true
	l.HeroTitre = "Le budget réel de la France, sujet par sujet de la campagne 2027"
	// Révision du 20 septembre 2026 (revue de la copie d'accueil, option A) :
	// le titre met en avant le vrai différenciateur du site — le budget réel,
	// pas un cadrage générique de campagne — comme le font Our World in Data
	// ou l'ONS avec leur mission en tête plutôt qu'une accroche. Le chapeau
	// énonce quatre réponses courtes (dépense, origine, décision, contrôle)
	// plutôt qu'un vague appel à « se faire son opinion », qui tranchait avec
	// le reste du registre (SHA-256, contrôles, sources). La date de
	// fraîcheur n'y est pas répétée : elle est déjà le premier repère du
	// bandeau juste en dessous (« Données arrêtées au »).
	l.HeroLede = "Retraites, santé, école, sécurité, immigration, dette : ce que l'État dépense, " +
		"d'où vient l'argent, qui décide et qui contrôle — sourcé document par document."
	l.HeroVisuel = heroMille(acc, root)
	layoutAccueil := l // le hero est prêt ; la page s'écrit plus bas, une fois
	// les dossiers de sujet chargés (acc.Familles[].Sujets[].D), pour afficher
	// le titre de chacun sur l'accueil sans le deviner en double.

	// Une page par fonction de la dépense publique (COFOG) : chaque ligne du
	// tableau « sur 1 000 € » de l'accueil y mène.
	pagesFonctions, err := chargerFonctions(ctx, pool, acc)
	if err != nil {
		return err
	}
	tf := page("fonction.gohtml")
	for _, f := range acc.Fonctions {
		pf := pagesFonctions[f.Code]
		l = layout
		l.Title = pf.Nom
		if err := write(tf, filepath.Join(out, "fonction", f.Slug, "index.html"),
			struct {
				Layout
				F *PageFonction
			}{l, pf}); err != nil {
			return err
		}
	}

	l = layout
	l.Title = "Qui décide"
	if err := write(page("qui-decide.gohtml"), filepath.Join(out, "qui-decide", "index.html"), l); err != nil {
		return err
	}

	e27, err := load2027(ctx, pool, candidats, dataDir)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Présidentielle 2027"
	if err := write(page("election2027.gohtml"), filepath.Join(out, "2027", "index.html"),
		struct {
			Layout
			E *Stats2027
		}{l, e27}); err != nil {
		return err
	}

	vues := map[string]bool{}
	for _, k := range e27.Candidats {
		if k.Page == nil || vues[k.Page.Slug] {
			continue
		}
		vues[k.Page.Slug] = true
		l = layout
		l.Title = k.Page.Titre
		l.Description = k.Page.Question
		imageCarte(&l, out, "2027-"+k.Page.Slug, k.Page.Carte.SVG)
		if err := write(tcd, filepath.Join(out, "2027", k.Page.Slug, "index.html"),
			struct {
				Layout
				P PageCarte
			}{l, *k.Page}); err != nil {
			return err
		}
	}

	avecFiche := map[string]bool{}
	for _, pp := range persons {
		avecFiche[pp.Slug] = true
	}
	gouv, err := loadGouvernement(ctx, pool, dataDir, avecFiche)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Gouvernement"
	if err := write(page("gouvernement.gohtml"),
		filepath.Join(out, "gouvernement", "index.html"), struct {
			Layout
			G *StatsGouvernement
		}{l, gouv}); err != nil {
		return err
	}

	l = layout
	l.Title = "Tous les décrets de composition"
	tousDecrets := *gouv
	tousDecrets.Decrets = gouv.Tous
	if err := write(page("decrets.gohtml"),
		filepath.Join(out, "gouvernement", "decrets", "index.html"), struct {
			Layout
			G *StatsGouvernement
		}{l, &tousDecrets}); err != nil {
		return err
	}

	col, err := loadCollectivites(ctx, pool)
	if err != nil {
		return err
	}
	// La carte d'index porte la population : un repère démographique neutre,
	// avant les cartes de dépenses, dette et recettes plus bas — plutôt que
	// de présenter un seul indicateur budgétaire comme LE chiffre par défaut.
	relierFiches(col, avecFiche)
	if err := cartesCollectivites(ctx, pool, col, indicPopulation); err != nil {
		return err
	}
	tableDep := tableDepenses(col.Poids, indicsCollectivite)
	parts := partsRecettes(col.Poids)
	l = layout
	l.Title = "Collectivités"
	imageCarte(&l, out, "collectivites", col.CarteDepts.SVG)
	if err := write(page("collectivites.gohtml"),
		filepath.Join(out, "collectivites", "index.html"), struct {
			Layout
			C            *StatsCollectivites
			T            *StatsTerritoires
			Ind          []IndicCollectivite
			TableDep     TableDepenses
			Parts        []PartRecette
			IndicLibelle string
		}{l, col, terr, indicsCollectivite, tableDep, parts, "Population"}); err != nil {
		return err
	}

	// --- les lieux : communes et intercommunalités, et la chaîne de lieux de
	// chaque mandat affiché ailleurs sur le site.
	fmt.Printf("    avant le résolveur de lieux : %s écoulées\n", time.Since(start).Round(time.Second))
	lieux, err := chargerResolveur(ctx, pool, root, col)
	if err != nil {
		return err
	}
	for _, pp := range persons {
		lieux.situerMandats(pp)
	}
	fmt.Printf("    résolveur de lieux : %s écoulées\n", time.Since(start).Round(time.Second))
	// Les cartes de situation : un fond commun écrit une fois, un calque par page.
	// Sur la carte de situation, chaque département mène à sa page ; un
	// département fusionné (Alsace, Corse, Martinique, Guyane) mène à la
	// collectivité qui tient son budget — Resolveur.urlDept résout déjà ce
	// repli via fusionConnue. Mandataire pour d'autres sections (région,
	// département) : chargé qu'importe si « communes » est recopiée.
	lienDept := func(code string) string {
		return strings.TrimPrefix(lieux.urlDept(code), root+"/")
	}
	fond, err := chargerFondSituation(ctx, pool, out, root, col.Exercice, lienDept)
	if err != nil {
		return err
	}
	fmt.Printf("    fond de situation chargé : %s écoulées\n", time.Since(start).Round(time.Second))

	communesInchangees, etatCommunes, err := sectionInchangee(ctx, pool, ancienCache, "communes")
	if err != nil {
		return err
	}
	nouveauCache.Sections["communes"] = etatCommunes
	if !exclu("communes") && communesInchangees {
		fmt.Println("    communes : données et gabarits inchangés, recopiées depuis le site précédent")
		if err := copierRepertoire(filepath.Join(siteActuel, "collectivites", "commune"),
			filepath.Join(out, "collectivites", "commune")); err != nil {
			return err
		}
		if err := copierRepertoire(filepath.Join(siteActuel, "collectivites", "epci"),
			filepath.Join(out, "collectivites", "epci")); err != nil {
			return err
		}
	} else if !exclu("communes") {
		pagesCom, err := chargerPagesCommunes(ctx, pool, lieux, avecFiche)
		if err != nil {
			return err
		}
		fmt.Printf("    pages communes chargées : %s écoulées\n", time.Since(start).Round(time.Second))
		for code, pc := range pagesCom {
			pc.Situation = fond.pourCommune(code, pc.Nom)
		}
		tcom := page("commune.gohtml")
		// Même raisonnement que pour les scrutins (scrutins.go) : chaque
		// commune ne lit que sa propre entrée de pagesCom et n'écrit que son
		// propre fichier, rien de partagé entre deux itérations.
		g := new(errgroup.Group)
		g.SetLimit(runtime.NumCPU())
		for code, pc := range pagesCom {
			g.Go(func() error {
				lp := layout
				lp.Title = pc.Nom + " (" + pc.Dept.Code + ")"
				return write(tcom, filepath.Join(out, "collectivites", "commune", code, "index.html"),
					struct {
						Layout
						C *PageCommune
					}{lp, pc})
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
		fmt.Printf("    pages communes écrites : %s écoulées\n", time.Since(start).Round(time.Second))
		pagesEPCI, err := chargerPagesEPCI(ctx, pool, lieux, col, avecFiche)
		if err != nil {
			return err
		}
		for siren, pe := range pagesEPCI {
			pe.Situation = fond.pourEPCI(siren, pe.Nom, pe.Finances)
		}
		tepci := page("epci.gohtml")
		for siren, pe := range pagesEPCI {
			l = layout
			l.Title = pe.Nom
			if err := write(tepci, filepath.Join(out, "collectivites", "epci", siren, "index.html"),
				struct {
					Layout
					E *PageEPCI
				}{l, pe}); err != nil {
				return err
			}
		}
	}
	ecart := ""
	switch {
	case exclu("communes"):
		ecart = " — pages non écrites (-only)"
	case communesInchangees:
		ecart = " — recopiées, données et gabarits inchangés"
	}
	fmt.Printf("  lieux : %d communes, %d intercommunalités%s (%s écoulées)\n",
		len(lieux.communes), len(lieux.epci), ecart, time.Since(start).Round(time.Second))

	// Les pages de région et de département, maintenant que le résolveur sait
	// quels départements composent une région.
	tcol := page("collectivite.gohtml")
	pagesCol, err := pagesCollectivites(ctx, pool, col, lieux)
	if err != nil {
		return err
	}

	// Les circonscriptions législatives, avec la même carte de situation.
	circos, err := chargerCirconscriptions(ctx, pool, lieux, persons)
	if err != nil {
		return err
	}
	if err := fond.chargerCirconscriptions(ctx, pool); err != nil {
		return err
	}
	popFrance := 0
	for _, pc := range circos {
		popFrance += pc.Population
	}
	tcirco := page("circonscription.gohtml")
	circosDept := map[string][]Lieu{}
	for code, pc := range circos {
		pc.Situation = fond.pourCirconscription(pc, popFrance)
		l = layout
		l.Title = pc.Titre
		if err := write(tcirco, filepath.Join(out, "circonscription", code, "index.html"),
			struct {
				Layout
				C *PageCirco
			}{l, pc}); err != nil {
			return err
		}
		circosDept[pc.Dept.Code] = append(circosDept[pc.Dept.Code], Lieu{Type: "CIRCONSCRIPTION",
			Code: code, Nom: pc.Ordinal + " circonscription", URL: root + "/circonscription/" + code + "/"})
	}
	for _, ls := range circosDept {
		sort.Slice(ls, func(i, j int) bool { return ls[i].Code < ls[j].Code })
	}
	fmt.Printf("  circonscriptions : %d pages\n", len(circos))

	for _, pc := range pagesCol {
		if pc.TypeURL == "departement" {
			pc.Situation = fond.pourDepartement(pc.Code, pc.Nom, pc.Lignes)
			for _, d := range codesCOGDe(pc.Code) {
				pc.Circonscriptions = append(pc.Circonscriptions, circosDept[d]...)
			}
		} else {
			pc.Situation = fond.pourRegion(pc.Code, pc.Nom, pc.Lignes)
		}
		l = layout
		l.Title = pc.Nom
		if err := write(tcol, filepath.Join(out, "collectivites", pc.TypeURL, pc.Slug,
			"index.html"), struct {
			Layout
			K PageCollectivite
		}{l, pc}); err != nil {
			return err
		}
	}

	agri, err := loadAgriculture(ctx, pool, dataDir)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Agriculture et alimentation"
	if err := write(page("agriculture.gohtml"), filepath.Join(out, "agriculture", "index.html"),
		struct {
			Layout
			A *StatsAgri
		}{l, agri}); err != nil {
		return err
	}

	bud, err := loadBudget(ctx, pool)
	if err != nil {
		return err
	}
	sect, err := loadSecteurs(ctx, pool)
	if err != nil {
		return err
	}
	circuit, err := loadCircuitCanaux(ctx, pool, presidences)
	if err != nil {
		return err
	}
	seuilsPauvrete, err := chargerSeuilsPauvrete(ctx, pool)
	if err != nil {
		return err
	}
	schemaTauxPauvrete, err := chargerTauxPauvreteSerie(ctx, pool)
	if err != nil {
		return err
	}
	schemaDepenseEnv, err := chargerDepenseEnvironnementale(ctx, pool)
	if err != nil {
		return err
	}
	schemaEmploiTotal, err := chargerEmploiTotalSalarie(ctx, pool)
	if err != nil {
		return err
	}
	statsClimatInternational, err := chargerClimatInternational(ctx, pool)
	if err != nil {
		return err
	}
	statsUnionEuropeenne, err := chargerUnionEuropeenne(ctx, pool)
	if err != nil {
		return err
	}
	carteBassins, err := chargerCarteBassins(ctx, pool)
	if err != nil {
		return err
	}
	carteEPTBEPAGE, err := chargerCarteEPTBEPAGE(ctx, pool)
	if err != nil {
		return err
	}
	schemaTendanceDepensesFiscales, err := chargerTendanceDepensesFiscales(ctx, pool)
	if err != nil {
		return err
	}
	statsDepensesFiscales, err := chargerStatsDepensesFiscales(ctx, pool)
	if err != nil {
		return err
	}
	carteIFI, err := chargerCarteIFI(ctx, pool)
	if err != nil {
		return err
	}
	statsAppareilProductif, err := chargerAppareilProductif(ctx, pool)
	if err != nil {
		return err
	}
	statsFrancophonie, err := chargerFrancophonie(ctx, pool)
	if err != nil {
		return err
	}
	statsEmpireColonial, err := chargerEmpireColonial(ctx, pool)
	if err != nil {
		return err
	}
	statsSGM, err := chargerSecondeGuerreMondiale(ctx, pool)
	if err != nil {
		return err
	}
	populationGuerres, err := chargerPopulationGuerres(ctx, pool)
	if err != nil {
		return err
	}
	controleFiscal, err := chargerControleFiscal(ctx, pool)
	if err != nil {
		return err
	}
	schemaPortsFrancais, err := chargerPortsFrancais(ctx, pool)
	if err != nil {
		return err
	}
	schemaPortsEurope, err := chargerPortsEurope(ctx, pool)
	if err != nil {
		return err
	}
	carteInfrastructurePorts, err := chargerCarteInfrastructurePorts(ctx, pool)
	if err != nil {
		return err
	}
	schemaReportModalPort, err := chargerReportModal(ctx, pool)
	if err != nil {
		return err
	}
	schemaReportModalConteneurs, err := chargerReportModalConteneurs(ctx, pool)
	if err != nil {
		return err
	}
	statsJustice, err := chargerJustice(ctx, pool)
	if err != nil {
		return err
	}
	schemaCarteSemiConducteurs, err := chargerCarteSemiConducteurs(ctx, pool)
	if err != nil {
		return err
	}
	schemaContributifNonContributif := dessinerContributifNonContributif()
	carteMusees, err := chargerCarteMusees(ctx, pool)
	if err != nil {
		return err
	}
	schemaEcartPrixDOM, err := chargerEcartPrixDOM(ctx, pool)
	if err != nil {
		return err
	}
	schemaAlimentaireDOM, err := chargerAlimentaireDOM(ctx, pool)
	if err != nil {
		return err
	}
	carteEtudiants, err := chargerCarteEtudiants(ctx, pool)
	if err != nil {
		return err
	}
	carteSRU, err := chargerCarteSRU(ctx, pool)
	if err != nil {
		return err
	}
	effortRecherche, err := chargerEffortRecherche(ctx, pool)
	if err != nil {
		return err
	}
	schemaSIPRI, err := chargerSIPRI(ctx, pool)
	if err != nil {
		return err
	}
	schemaHistoriqueImmigration, err := chargerHistoriqueImmigration(ctx, pool)
	if err != nil {
		return err
	}
	schemaAgeDepartRetraite, err := chargerAgeDepartRetraite(ctx, pool)
	if err != nil {
		return err
	}
	schemaTauxRemplacement, err := chargerTauxRemplacement(ctx, pool)
	if err != nil {
		return err
	}
	// La carte des médecins généralistes (territoires.go) existait déjà,
	// utilisée seulement par les onglets de l'accueil — jamais reprise sur
	// /sujets/sante/, qui n'a par ailleurs aucune carte du tout.
	var carteMedecins *CarteTerritoire
	for i := range terr.Cartes {
		if terr.Cartes[i].Slug == "medecins-generalistes" {
			carteMedecins = &terr.Cartes[i]
			break
		}
	}
	if bud != nil {
		l = layout
		l.Title = "Budget de l'État"
		if err := write(page("budget.gohtml"), filepath.Join(out, "budget", "index.html"),
			struct {
				Layout
				B *StatsBudget
				X *StatsSecteurs
				K *CircuitCanaux
			}{l, bud, sect, circuit}); err != nil {
			return err
		}
	}
	if circuit != nil {
		tdisp := page("dispositif.gohtml")
		for _, m := range circuit.GrandesMesures {
			l = layout
			l.Title = m.Libelle
			if err := write(tdisp, filepath.Join(out, "budget", "dispositif", m.Code, "index.html"),
				struct {
					Layout
					M MesureExoneration
				}{l, m}); err != nil {
				return err
			}
		}
	}

	soc, err := loadSocial(ctx, pool)
	if err != nil {
		return err
	}
	if soc != nil && soc.Total > 0 {
		l = layout
		l.Title = "Protection sociale"
		if err := write(page("social.gohtml"), filepath.Join(out, "protection-sociale", "index.html"),
			struct {
				Layout
				X *StatsSocial
			}{l, soc}); err != nil {
			return err
		}
	}

	// --- comprendre : les documents de méthode, rendus en pages
	docs, err := loadDocs("docs")
	if err != nil {
		return err
	}
	// Les schémas calculés depuis la base sont insérés dans les documents
	// Markdown à l'endroit d'un marqueur : le texte reste un fichier relisible
	// dans le dépôt, et le chiffre du schéma reste celui de la base.
	for _, d := range docs {
		if strings.Contains(string(d.Corps), "<!-- schema:canaux -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:canaux -->",
				`<figure class="schema">`+string(circuit.SVG)+`<figcaption>`+
					`Trois largeurs sont <strong>proportionnelles</strong>, à la même échelle, pour `+
					fmt.Sprint(circuit.AnneeCotisationsURSSAF)+`&nbsp;: les cotisations versées, les `+
					`exonérations (URSSAF, quatre catégories qui se somment exactement au total) et `+
					`la compensation qui leur répond (jaune budgétaire annexé au PLF 2024, exécution `+
					fmt.Sprint(circuit.AnneeCompensation)+`). Les autres flèches restent simples&nbsp;: `+
					`aucun montant comparable pour la même année n'a été trouvé. Non-compensation&nbsp;: `+
					`jaune budgétaire et LFSS `+fmt.Sprint(circuit.AnneeNonComp)+` — une mesure différente, `+
					`les mesures nouvelles décidées cette année-là, pas un solde cumulé comparable aux `+
					`largeurs ci-dessus. `+
					`<a href="`+root+`/budget/">La série annuelle des exonérations →</a></figcaption></figure>`))
		}
		if sect != nil && strings.Contains(string(d.Corps), "<!-- schema:s1311s1314 -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:s1311s1314 -->",
				`<figure class="schema">`+string(sect.CourbeS1311S1314)+`<figcaption>`+
					`Dépenses de l'administration centrale (S1311) contre la Sécurité sociale `+
					`(S1314), `+fmt.Sprint(sect.Debut)+`–`+fmt.Sprint(sect.Annee)+`.`+
					func() string {
						if sect.AnneeCroisement1314 > 0 {
							return ` La Sécurité sociale dépense plus que l'État à partir de ` +
								fmt.Sprint(sect.AnneeCroisement1314) + `.`
						}
						return ""
					}()+
					` <a href="`+root+`/budget/">Le détail par sous-secteur →</a></figcaption></figure>`))
		}
		if sect != nil && strings.Contains(string(d.Corps), "<!-- schema:financement34ans -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:financement34ans -->",
				`<figure class="schema">`+string(sect.EmpileesFinancement)+
					`<div class="legende">`+
					`<span><i class="f0"></i>Cotisations des employeurs</span>`+
					`<span><i class="f1"></i>Cotisations des assurés</span>`+
					`<span><i class="f2"></i>Recettes fiscales affectées</span>`+
					`<span><i class="f3"></i>Recettes fiscales générales</span></div>`+
					`<figcaption>Les 34 années, en 100&nbsp;% empilé — pas seulement `+
					fmt.Sprint(sect.DebutFin)+` et `+fmt.Sprint(sect.AnnFin)+
					`. Source&nbsp;: Eurostat ESSPROS, dataflow spr_rec_sumt. `+
					`<a href="`+root+`/budget/">Le tableau des deux dates →</a></figcaption></figure>`))
		}
		if seuilsPauvrete != nil && strings.Contains(string(d.Corps), "<!-- schema:seuils-pauvrete -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:seuils-pauvrete -->",
				`<figure class="schema">`+string(seuilsPauvrete.SVG)+`<figcaption>`+
					`<strong>Comment lire ce graphique</strong> : classez tous les Français du `+
					`niveau de vie le plus bas au plus haut, puis coupez cette file en dix tas `+
					`du même nombre de personnes — 10&nbsp;% de la population dans chaque tas, `+
					`pas 10&nbsp;% du revenu total. D1 est le tas le plus pauvre, D9 le neuvième. `+
					`Chaque barre est le plafond de son tas, `+fmt.Sprint(seuilsPauvrete.Annee)+
					` (Insee-Filosofi) : personne dans ce dixième de la population ne touche plus `+
					`que le montant affiché au-dessus de sa barre. Le dixième tas, D10 — les `+
					`10&nbsp;% les plus aisés —, n'a par définition aucun plafond&nbsp;: sa barre `+
					`s'estompe vers le haut plutôt que de s'arrêter net, pour montrer qu'elle `+
					`existe sans prétendre savoir où elle finit.`+
					`<br><strong>Le « niveau de vie »</strong> n'est pas le revenu du foyer tel quel, mais `+
					`ce revenu ramené à sa taille (un couple sans enfant n'a pas les mêmes besoins `+
					`qu'une famille de quatre) — de quoi comparer des foyers de toutes tailles sur `+
					`la même échelle, à peu près « ce que toucherait une personne seule pour vivre `+
					`aussi bien ».<br>`+
					`Les deux lignes pointillées sont les seuils de pauvreté&nbsp;: en dessous, `+
					`l'Insee et Eurostat considèrent qu'on est pauvre. Seul le premier dixième `+
					`(D1) plafonne sous le seuil à 60&nbsp;% (teinte plus sombre) ; le second (D2) `+
					`le chevauche — cohérent avec un taux de pauvreté à 60&nbsp;% légèrement `+
					`supérieur à 10&nbsp;%. L'axe part de zéro.`+
					`</figcaption></figure>`))
		}
		if schemaTauxPauvrete != "" && strings.Contains(string(d.Corps), "<!-- schema:taux-pauvrete-serie -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:taux-pauvrete-serie -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaTauxPauvrete)+`</div>`+
					`<figcaption>Taux de pauvreté (seuil à 60 % du niveau de vie médian), 1996-2023 — `+
					`Insee. La refonte de l'enquête en 2021 (ERFS nouvelle formule) rend les deux `+
					`périodes non strictement comparables.</figcaption></figure>`))
		}
		if schemaDepenseEnv != "" && strings.Contains(string(d.Corps), "<!-- schema:depense-environnementale -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:depense-environnementale -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaDepenseEnv)+`</div>`+
					`<figcaption>Dépense de protection de l'environnement, ensemble de l'économie — `+
					`Eurostat (env_epea_neep), millions d'euros courants convertis en milliards.`+
					`</figcaption></figure>`))
		}
		if schemaEmploiTotal != "" && strings.Contains(string(d.Corps), "<!-- schema:emploi-total-salarie -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:emploi-total-salarie -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaEmploiTotal)+`</div>`+
					`<figcaption>Emploi total et salarié, France, 1975-2025 — Eurostat (nama_10_pe). `+
					`L'écart entre les deux courbes est le nombre de non-salariés.</figcaption></figure>`))
		}
		if statsClimatInternational != nil && strings.Contains(string(d.Corps), "<!-- schema:ratification-accord-paris -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:ratification-accord-paris -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsClimatInternational.CourbeRatificationSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Nombre cumulé de pays ayant ratifié l'Accord de Paris — ONU, `+
						`collection des traités. %d pays au total, dont %d n'ont jamais ratifié (signature seule, `+
						`ou aucune des deux).</figcaption></figure>`,
						statsClimatInternational.NbPays, statsClimatInternational.NbJamaisRatifie)))
		}
		if statsUnionEuropeenne != nil {
			if statsUnionEuropeenne.PIBSVG != "" && strings.Contains(string(d.Corps), "<!-- schema:pib-blocs -->") {
				d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:pib-blocs -->",
					`<figure class="schema">`+string(statsUnionEuropeenne.PIBSVG)+
						fmt.Sprintf(`<figcaption>PIB, dollars courants, %d — Banque mondiale.</figcaption></figure>`,
							statsUnionEuropeenne.AnneePIB)))
			}
			if statsUnionEuropeenne.SecteursSVG != "" && strings.Contains(string(d.Corps), "<!-- schema:secteurs-blocs -->") {
				d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:secteurs-blocs -->",
					`<figure class="schema"><div class="carte-pleine">`+string(statsUnionEuropeenne.SecteursSVG)+`</div>`+
						fmt.Sprintf(`<figcaption>Valeur ajoutée par secteur, %% du PIB, %d — Banque mondiale.`+
							`</figcaption></figure>`, statsUnionEuropeenne.AnneeSecteurs)))
			}
			if statsUnionEuropeenne.CommerceTable != "" && strings.Contains(string(d.Corps), "<!-- tableau:commerce-blocs -->") {
				d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:commerce-blocs -->",
					string(statsUnionEuropeenne.CommerceTable)))
			}
			if statsUnionEuropeenne.SecteursUSTable != "" && strings.Contains(string(d.Corps), "<!-- tableau:secteurs-us -->") {
				d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:secteurs-us -->",
					string(statsUnionEuropeenne.SecteursUSTable)))
			}
		}
		if carteBassins != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-bassins -->") {
			var legende strings.Builder
			legende.WriteString(`<div class="repartition-legende">`)
			for _, bs := range carteBassins.Bassins {
				fmt.Fprintf(&legende, `<div><i style="background:%s"></i><span>%s</span></div>`,
					bs.Couleur, template.HTMLEscapeString(bs.Nom))
			}
			legende.WriteString(`</div>`)
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-bassins -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteBassins.SVG)+`</div>`+
					legende.String()+
					`<figcaption>Les 7 bassins hydrographiques de France métropolitaine — les 6 `+
					`comités de bassin classiques plus la Corse, distincte hydrographiquement mais `+
					`rattachée administrativement à Rhône-Méditerranée (§ 1.1). Un découpage qui ne `+
					`suit aucune limite régionale ou départementale — le bassin Loire-Bretagne, le `+
					`plus vaste, traverse une douzaine de régions et départements actuels. `+
					`Source&nbsp;: BD Topage 2025, Sandre/IGN, Licence Ouverte.</figcaption></figure>`))
		}
		if carteEPTBEPAGE != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-eptb-epage -->") {
			legende := fmt.Sprintf(`<div class="repartition-legende">`+
				`<div><i style="background:%s"></i><span>EPTB (%d)</span></div>`+
				`<div><i style="background:%s"></i><span>EPAGE (%d)</span></div>`+
				`<div><i style="background:%s"></i><span>Double statut (%d)</span></div></div>`,
				couleursTypeEPTBEPAGE["EPTB"], carteEPTBEPAGE.NbEPTB,
				couleursTypeEPTBEPAGE["EPAGE"], carteEPTBEPAGE.NbEPAGE,
				couleursTypeEPTBEPAGE["EPTB_EPAGE"], carteEPTBEPAGE.NbDouble)
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-eptb-epage -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteEPTBEPAGE.SVG)+`</div>`+
					legende+
					fmt.Sprintf(`<figcaption>%d structures sur %d trouvées dans BANATIC affichées ici : `+
						`en dessous de %.0f%% de membres reconstruits (communes ou EPCI déjà chargés dans `+
						`ce dépôt), le contour serait un fragment épars, plus trompeur qu'utile. Les zones `+
						`grises n'appartiennent à aucun EPTB/EPAGE reconstruit — pas forcément à aucun `+
						`EPTB/EPAGE réel (§ 1.2). Source&nbsp;: BANATIC (DGCL), contour reconstruit par ce `+
						`dépôt, pas téléchargé comme tel.</figcaption></figure>`,
						carteEPTBEPAGE.NbAffiches, carteEPTBEPAGE.NbTrouves, seuilResolutionEPTBEPAGE*100)))
		}
		if schemaTendanceDepensesFiscales != "" && strings.Contains(string(d.Corps), "<!-- schema:depenses-fiscales-tendance -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:depenses-fiscales-tendance -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaTendanceDepensesFiscales)+`</div>`+
					`<figcaption>Total exécuté, année par année — chaque millésime du PLF ne publie l'exécution `+
					`que pour une seule année, jamais révisée dans un millésime ultérieur : sept millésimes, `+
					`sept années, sans doublon à trancher.</figcaption></figure>`))
		}
		if statsDepensesFiscales != nil && strings.Contains(string(d.Corps), "<!-- tableau:depenses-fiscales-top -->") {
			var t strings.Builder
			t.WriteString(`<div class="scroll"><table><thead><tr><th>Dispositif</th><th>Impôt</th><th>Coût</th></tr></thead><tbody>`)
			for _, dsp := range statsDepensesFiscales.TopDispositifs {
				fmt.Fprintf(&t, `<tr><td>%s</td><td>%s</td><td>%s M€</td></tr>`,
					template.HTMLEscapeString(dsp.Libelle), template.HTMLEscapeString(dsp.Impot), Nombre(int(dsp.MontantM)))
			}
			t.WriteString(`</tbody></table></div>`)
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:depenses-fiscales-top -->", t.String()))
		}
		if statsDepensesFiscales != nil && strings.Contains(string(d.Corps), "<!-- tableau:depenses-fiscales-impot -->") {
			var t strings.Builder
			t.WriteString(`<div class="scroll"><table><thead><tr><th>Impôt</th><th>Dispositifs</th><th>Coût cumulé</th></tr></thead><tbody>`)
			for _, it := range statsDepensesFiscales.ParImpot {
				fmt.Fprintf(&t, `<tr><td>%s</td><td>%s</td><td>%s Md€</td></tr>`,
					template.HTMLEscapeString(it.Impot), Nombre(it.NbDisp), Decimal(it.TotalMdEu, 1))
			}
			t.WriteString(`</tbody></table></div>`)
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:depenses-fiscales-impot -->", t.String()))
		}
		if carteIFI != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-ifi -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-ifi -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteIFI.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>%d communes publiées par la DGFiP pour %d (plus de 20 000 `+
						`habitants et plus de 50 redevables à l'IFI — un seuil de publication de la DGFiP, `+
						`pas de ce dépôt). La surface de chaque cercle est proportionnelle au nombre de `+
						`redevables ; la couleur est uniforme. Les zones sans cercle n'ont pas de commune `+
						`publiée à ce niveau, pas forcément aucun redevable à l'IFI.</figcaption></figure>`,
						carteIFI.NbCommunes, carteIFI.Annee)))
		}
		if statsAppareilProductif != nil && strings.Contains(string(d.Corps), "<!-- schema:glissement-sectoriel -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:glissement-sectoriel -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsAppareilProductif.GlissementSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Part de l'emploi total par secteur, France, %d à %d — Eurostat, `+
						`nama_10_a10_e. « Services » est ici le complément (total moins agriculture, industrie et `+
						`construction), pas une addition de branches publiées séparément.</figcaption></figure>`,
						statsAppareilProductif.AnneeDebutGlissement, statsAppareilProductif.AnneeFinGlissement)))
		}
		if statsAppareilProductif != nil && strings.Contains(string(d.Corps), "<!-- schema:delocalisation-annuelle -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:delocalisation-annuelle -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsAppareilProductif.DelocalisationAnnuelleSVG)+`</div>`+
					`<figcaption>Emplois en équivalent temps plein détectés comme délocalisés chaque année, `+
					`2001-2017 — la bande couvre les scénarios bas à haut du modèle Insee, la ligne est le `+
					`scénario central. Un chiffre encadré par une fourchette, pas une mesure exacte.</figcaption></figure>`))
		}
		if statsAppareilProductif != nil && strings.Contains(string(d.Corps), "<!-- schema:delocalisation-departement -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:delocalisation-departement -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsAppareilProductif.DelocalisationDeptSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Cumul 1995-2017 des emplois délocalisés (scénario central), par `+
						`département de résidence de l'entreprise — %d départements métropolitains. La surface de `+
						`chaque cercle est proportionnelle au nombre d'emplois.</figcaption></figure>`,
						statsAppareilProductif.NbDepartements)))
		}
		if statsAppareilProductif != nil && strings.Contains(string(d.Corps), "<!-- tableau:delocalisation-csp -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:delocalisation-csp -->",
				string(statsAppareilProductif.DelocalisationCSPTable)))
		}
		if statsAppareilProductif != nil {
			for _, marqueur := range []struct {
				cle string
				st  *StatsCommerceSecteur
			}{
				{"schema:commerce-automobile", statsAppareilProductif.CommerceAutomobile},
				{"schema:commerce-textile", statsAppareilProductif.CommerceTextile},
				{"schema:commerce-electronique-tv", statsAppareilProductif.CommerceElectroniqueTV},
			} {
				if marqueur.st == nil {
					continue
				}
				m := "<!-- " + marqueur.cle + " -->"
				if strings.Contains(string(d.Corps), m) {
					d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), m,
						`<figure class="schema"><div class="carte-pleine">`+string(marqueur.st.SVG)+`</div>`+
							fmt.Sprintf(`<figcaption>Part de chaque partenaire dans les importations françaises de %s, `+
								`%d (point clair) et %d (point plein) — UN Comtrade, valeurs en dollars courants. `+
								`Total mondial : %s Md$ en %d, %s Md$ en %d.</figcaption></figure>`,
								marqueur.st.Libelle, marqueur.st.AnneeDebut, marqueur.st.AnneeFin,
								Decimal(marqueur.st.TotalUSDDebut/1e9, 1), marqueur.st.AnneeDebut,
								Decimal(marqueur.st.TotalUSDFin/1e9, 1), marqueur.st.AnneeFin)))
				}
			}
		}
		if statsFrancophonie != nil && strings.Contains(string(d.Corps), "<!-- schema:francophonie-carte -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:francophonie-carte -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsFrancophonie.CarteSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Part de francophones dans la population, par pays, 2025 — ODSEF/OIF. `+
						`%d des %d pays souverains du fichier source sont repérés sur ce fond de carte ; les pays en gris `+
						`n'ont pas de correspondance dans le fond Natural Earth utilisé ici, pas nécessairement aucune donnée.</figcaption></figure>`,
						statsFrancophonie.NbPaysCartes, statsFrancophonie.NbPaysTotal)))
		}
		if statsFrancophonie != nil && strings.Contains(string(d.Corps), "<!-- tableau:francophonie-top-pct -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:francophonie-top-pct -->",
				string(statsFrancophonie.TopParPctTable)))
		}
		if statsFrancophonie != nil && strings.Contains(string(d.Corps), "<!-- tableau:francophonie-top-nombre -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:francophonie-top-nombre -->",
				string(statsFrancophonie.TopParNombreTable)))
		}
		if statsEmpireColonial != nil && strings.Contains(string(d.Corps), "<!-- schema:empire-colonial-carte -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:empire-colonial-carte -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsEmpireColonial.CarteSVG)+`</div>`+
					`<div class="echelle"><span><i class="vague-1"></i>1953-1956</span>`+
					`<span><i class="vague-2"></i>1958-1962</span>`+
					`<span><i class="vague-3"></i>1975-1977</span></div>`+
					fmt.Sprintf(`<figcaption>%d des %d territoires listés ont une géométrie CShapes ; `+
						`survolez chaque territoire pour sa date d'indépendance exacte.</figcaption></figure>`,
						statsEmpireColonial.NbCartes, statsEmpireColonial.NbTotal)))
		}
		if statsEmpireColonial != nil && strings.Contains(string(d.Corps), "<!-- tableau:empire-colonial-territoires -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:empire-colonial-territoires -->",
				string(statsEmpireColonial.Table)))
		}
		if statsSGM != nil && strings.Contains(string(d.Corps), "<!-- schema:sgm-ligne-demarcation -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:sgm-ligne-demarcation -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsSGM.CarteSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Tracé de la ligne de démarcation entre zone occupée et zone libre, `+
						`1940-1942 (%s km) — Département de l'Ain. Ni l'annexion de fait de l'Alsace-Moselle ni `+
						`la zone d'occupation italienne (à partir de novembre 1942) n'ont de géométrie vérifiée `+
						`trouvée ; non représentées ici, voir § 2.</figcaption></figure>`, Decimal(statsSGM.LongueurKm, 0))))
		}
		if populationGuerres != nil && strings.Contains(string(d.Corps), "<!-- schema:population-guerres -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:population-guerres -->",
				`<figure class="schema"><div class="carte-pleine">`+string(populationGuerres.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Population de la France (hors Mayotte), recensements 1876-1999 (Insee) — `+
						`entre 1911 et 1921, la population recule de %s à %s habitants, soit %s (%s %%). Aucun recul `+
						`comparable n'est visible entre 1936 et 1954 : la source ne publie aucun point en 1946, l'année `+
						`où le recul de la Seconde Guerre mondiale aurait été le plus visible.</figcaption></figure>`,
						Nombre(int(populationGuerres.Pop1911)), Nombre(int(populationGuerres.Pop1921)),
						Nombre(int(populationGuerres.BaisseAbsolue)), Decimal(populationGuerres.BaissePct, 1))))
		}
		if controleFiscal != nil && strings.Contains(string(d.Corps), "<!-- schema:controle-fiscal -->") {
			calcule := ""
			if controleFiscal.DernierNotifieCalcule {
				calcule = " (déduit de l'écart notifié/encaissé publié, non cité tel quel par la source)"
			}
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:controle-fiscal -->",
				`<figure class="schema"><div class="carte-pleine">`+string(controleFiscal.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Résultats du contrôle fiscal, France entière, 2015-%d (Sénat, commission `+
						`des finances) — en %d, %s Md€ notifiés%s contre %s Md€ effectivement encaissés. Le notifié `+
						`2022 et 2023 n'a été retrouvé dans aucune des trois sources primaires consultées : la ligne `+
						`pointillée s'interrompt sur ces deux années plutôt que d'être devinée.</figcaption></figure>`,
						controleFiscal.DernierAnnee, controleFiscal.DernierAnnee,
						Decimal(controleFiscal.DernierNotifie, 1), calcule, Decimal(controleFiscal.DernierEncaisse, 1))))
		}
		if schemaPortsFrancais != "" && strings.Contains(string(d.Corps), "<!-- schema:ports-francais -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:ports-francais -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaPortsFrancais)+`</div>`+
					`<figcaption>Trafic total (marchandises et tare, entrées et sorties confondues), `+
					`quatre grands ports maritimes, 2000-2025 — SDES.</figcaption></figure>`))
		}
		if schemaPortsEurope != "" && strings.Contains(string(d.Corps), "<!-- schema:ports-europe -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:ports-europe -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaPortsEurope)+`</div>`+
					`<figcaption>Trafic total, dernière année disponible par port (Eurostat, mar_go_aa) — `+
					`l'année diffère selon le port, indiquée entre parenthèses.</figcaption></figure>`))
		}
		if carteInfrastructurePorts != nil && strings.Contains(string(d.Corps), "<!-- schema:ports-infrastructure -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:ports-infrastructure -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteInfrastructurePorts.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Autoroutes (%d tronçons) et voies ferrées classées « voie portuaire » `+
						`par SNCF Réseau (%d tronçons) à moins de 80 km de chacun des quatre ports (OpenStreetMap, `+
						`SNCF Réseau) — un accès physique, pas une mesure de trafic : aucune donnée ouverte ne `+
						`distingue le fret des voyageurs sur le réseau ferré français.</figcaption></figure>`,
						carteInfrastructurePorts.NbAutoroutes, carteInfrastructurePorts.NbVoiesFerrees)))
		}
		if schemaReportModalPort != "" && strings.Contains(string(d.Corps), "<!-- schema:report-modal-port -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:report-modal-port -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaReportModalPort)+`</div>`+
					`<figcaption>Part du fer et du fleuve dans le pré- et post-acheminement des marchandises, `+
					`2023 (DGITM, Observatoire de la performance portuaire) — un seul millésime, pas une série. `+
					`Marseille-Fos n'a publié qu'un plafond, pas un chiffre exact.</figcaption></figure>`))
		}
		if schemaReportModalConteneurs != "" && strings.Contains(string(d.Corps), "<!-- schema:report-modal-conteneurs -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:report-modal-conteneurs -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaReportModalConteneurs)+`</div>`+
					`<figcaption>Répartition modale du transport de CONTENEURS vers l'arrière-pays — jamais à `+
					`comparer aux chiffres tous-trafics ci-dessus. HAROPA et Hambourg ne publient pas de `+
					`répartition conteneurs seuls vérifiable.</figcaption></figure>`))
		}
		if statsJustice != nil && strings.Contains(string(d.Corps), "<!-- schema:justice-surpeuplement -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:justice-surpeuplement -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsJustice.TopSurpeuplementSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Les vingt quartiers d'établissement les plus densément peuplés sur %d `+
						`lignes chargées (établissement × quartier), ministère de la Justice, dernière donnée mensuelle. `+
						`La ligne verticale marque 100 %% (capacité atteinte).</figcaption></figure>`, statsJustice.NbLignes)))
		}
		if schemaCarteSemiConducteurs != "" && strings.Contains(string(d.Corps), "<!-- schema:semi-conducteurs-carte -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:semi-conducteurs-carte -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaCarteSemiConducteurs)+`</div>`+
					`<figcaption>Cinq sites de production identifiés par leur unité légale Sirene, géocodés à la `+
					`commune — pas à l'adresse exacte de l'usine. La Cour des comptes relève elle-même l'absence de `+
					`cartographie officielle de cette filière (§ 2).</figcaption></figure>`))
		}
		if strings.Contains(string(d.Corps), "<!-- schema:contributif-non-contributif -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:contributif-non-contributif -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaContributifNonContributif)+`</div>`+
					`<figcaption>Protection sociale par risque, 2024 (DREES) — la vieillesse est presque `+
					`entièrement contributive, les soins et la famille presque entièrement non contributifs. `+
					`Classement discuté poste par poste au paragraphe suivant.</figcaption></figure>`))
		}
		if carteMusees != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-musees -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-musees -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteMusees.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>%d musées labellisés « Musée de France » (Muséofile, ministère de la `+
						`Culture), répartis sur %d départements — la surface de chaque cercle est proportionnelle `+
						`au nombre de musées, pas à leur taille ou à leur fréquentation.</figcaption></figure>`,
						carteMusees.NbMusees, carteMusees.NbDepartements)))
		}
		if schemaEcartPrixDOM != "" && strings.Contains(string(d.Corps), "<!-- schema:ecart-prix-dom -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:ecart-prix-dom -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaEcartPrixDOM)+`</div>`+
					`<figcaption>Écart de prix (indice de Fisher) avec la France métropolitaine, 2010 et 2022 `+
					`— Insee, enquête de comparaison spatiale des prix. Mayotte : donnée 2010 non disponible dans `+
					`la source.</figcaption></figure>`))
		}
		if schemaAlimentaireDOM != "" && strings.Contains(string(d.Corps), "<!-- schema:alimentaire-dom -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:alimentaire-dom -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaAlimentaireDOM)+`</div>`+
					`<figcaption>Écart général contre écart sur les seuls produits alimentaires et boissons non `+
					`alcoolisées, 2022 (Insee) — les deux séries ne se ressemblent pas, jamais à confondre.</figcaption></figure>`))
		}
		if carteEtudiants != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-etudiants -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-etudiants -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteEtudiants.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>%s étudiants répartis sur %d communes, rentrée %d (SIES) — la surface `+
						`de chaque cercle est proportionnelle à l'effectif. Paris apparaît par arrondissement, comme `+
						`dans la source.</figcaption></figure>`,
						Nombre(int(carteEtudiants.Total)), carteEtudiants.NbCommunes, carteEtudiants.Annee)))
		}
		if carteSRU != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-sru -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-sru -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteSRU.SVG)+`</div>`+
					`<div class="echelle"><span><i class="sru-conforme"></i>Conforme ou au-delà</span>`+
					`<span><i class="sru-deficitaire"></i>Déficitaire</span>`+
					`<span><i class="sru-carencee"></i>Carencée</span></div>`+
					fmt.Sprintf(`<figcaption>%d communes soumises à la loi SRU au 1ᵉʳ janvier 2025 (DGALN/DHUP), dont `+
						`%d carencées et %d déficitaires non carencées. Taille du cercle proportionnelle à la `+
						`population.</figcaption></figure>`, carteSRU.NbCommunes, carteSRU.NbCarencees, carteSRU.NbDeficitaires)))
		}
		if effortRecherche != nil && strings.Contains(string(d.Corps), "<!-- schema:effort-recherche -->") {
			estim := ""
			if effortRecherche.Estimation {
				estim = " (dernier point estimé par l'OCDE)"
			}
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:effort-recherche -->",
				`<figure class="schema"><div class="carte-pleine">`+string(effortRecherche.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>DIRD (dépense intérieure de recherche et développement) rapportée au PIB, `+
						`%d à %d (Insee, sources MESR-SIES et OCDE)%s. En %d, la part portée par les entreprises seules `+
						`(DIRDE/PIB) est de %s %% en France contre %s %% en UE27.</figcaption></figure>`,
						effortRecherche.AnneeDebut, effortRecherche.AnneeFin, estim, effortRecherche.AnneeFin,
						Decimal(effortRecherche.DirdeFrDernier, 2), Decimal(effortRecherche.DirdeUeDernier, 2))))
		}
		if schemaSIPRI != "" && strings.Contains(string(d.Corps), "<!-- schema:sipri-milex -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:sipri-milex -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaSIPRI)+`</div>`+
					`<figcaption>Dépense militaire, % du PIB, 1949-2025 — SIPRI. France, Russie et `+
					`Arabie saoudite nommées (leurs trajectoires sont les plus commentées) ; les six autres `+
					`pays de la comparaison restent en gris, chacun identifiable au survol de son point final.`+
					`</figcaption></figure>`))
		}
		if schemaHistoriqueImmigration != "" && strings.Contains(string(d.Corps), "<!-- schema:historique-immigration -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:historique-immigration -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaHistoriqueImmigration)+`</div>`+
					`<figcaption>Part d'immigrés dans la population, 1921-2025 — Insee, recensements. Les bandes `+
					`marquent les changements de champ ou de protocole (métropole puis France, Mayotte incluse `+
					`en 2014, protocole de collecte revu en 2024) : chaque régime se lit pour lui-même, pas comme `+
					`une évolution lissée d'un bout à l'autre.</figcaption></figure>`))
		}
		if schemaAgeDepartRetraite != "" && strings.Contains(string(d.Corps), "<!-- schema:age-depart-retraite -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:age-depart-retraite -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaAgeDepartRetraite)+`</div>`+
					`<figcaption>Âge conjoncturel moyen de départ à la retraite, 2004-2022 — Drees. Le creux de 2010 `+
					`précède la réforme qui recule ensuite progressivement l'âge légal.</figcaption></figure>`))
		}
		if schemaTauxRemplacement != "" && strings.Contains(string(d.Corps), "<!-- schema:taux-remplacement-retraite -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:taux-remplacement-retraite -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaTauxRemplacement)+`</div>`+
					`<figcaption>Dispersion du taux de remplacement (niveau de vie), cohorte 2020 — Drees. Boîte : `+
					`du 1ᵉʳ au 3ᵉ quartile, avec la médiane ; tige : du 1ᵉʳ au 9ᵉ décile. 100 = pension égale au `+
					`revenu d'avant la retraite.</figcaption></figure>`))
		}
		if strings.Contains(string(d.Corps), "<!-- schema:holding-mere-fille -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:holding-mere-fille -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaHoldingMereFille())+`</div>`+
					`<figcaption>Exemple pédagogique sur un bénéfice initial rond (100 000 €) ; les taux (25 % `+
					`d'IS, 5 % de quote-part, 1 % en cas d'intégration fiscale, 30 % de PFU) sont réels et `+
					`sourcés — voir le Cadre et les Enjeux ci-dessus. Le point central : l'IS de la filiale `+
					`est payé AVANT que le dividende n'existe, pas une fois qu'il est dans la holding.</figcaption></figure>`))
		}
		if strings.Contains(string(d.Corps), "<!-- schema:tva-entreprises -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:tva-entreprises -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaTVAEntreprises())+`</div>`+
					`<figcaption>Exemple pédagogique (pas une chaîne de transactions réelles observées, voir `+
					`« Ce que les données ne disent pas ») : à chaque étage, TVA collectée moins TVA déductible `+
					`donne la TVA nette versée à l'État — la somme des trois retombe exactement sur la TVA payée `+
					`par le consommateur final.</figcaption></figure>`))
		}
		if carteMedecins != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-medecins-generalistes -->") {
			c := carteMedecins.Page.Carte
			var echelle strings.Builder
			echelle.WriteString(`<div class="echelle"><span class="u">` + template.HTMLEscapeString(c.Unite) + `</span>`)
			for i, b := range c.Bornes {
				fmt.Fprintf(&echelle, `<span><i style="background:%s"></i>%s</span>`, c.Teintes[i], b)
			}
			echelle.WriteString(`</div>`)
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-medecins-generalistes -->",
				`<figure class="schema"><div class="carte-pleine">`+string(c.SVG)+`</div>`+
					echelle.String()+
					`<figcaption>`+template.HTMLEscapeString(carteMedecins.Question)+` `+
					template.HTMLEscapeString(carteMedecins.Note)+` `+
					`<a href="`+root+`/collectivites/carte/medecins-generalistes/">Le classement des 101 `+
					`départements et la méthode →</a></figcaption></figure>`))
		}
	}

	// --- sujets : les dossiers au plan commun deviennent les pages de sujet,
	// et leur ancienne adresse sous /comprendre/ renvoie vers la nouvelle.
	if err := rattacherDocs(docs); err != nil {
		return err
	}
	reecrireLiensDocs(docs, root)
	if err := preparerSujets(ctx, pool, out, root, acc); err != nil {
		return err
	}
	if err := write(page("accueil.gohtml"), filepath.Join(out, "index.html"), struct {
		Layout
		A *DonneesAccueil
	}{layoutAccueil, acc}); err != nil {
		return err
	}
	l = layout
	l.Title = "Sujets de campagne"
	if err := write(page("sujets.gohtml"), filepath.Join(out, "sujets", "index.html"), struct {
		Layout
		Familles []*Famille
		Annee    int
	}{l, acc.Familles, acc.Annee}); err != nil {
		return err
	}
	l = layout
	l.Title = "Argent public"
	if err := write(page("argent-public.gohtml"), filepath.Join(out, "argent-public", "index.html"), struct {
		Layout
		A *DonneesAccueil
	}{l, acc}); err != nil {
		return err
	}
	ts := page("sujet.gohtml")
	var methode []*Doc
	for _, d := range docs {
		s := sujetDuDoc(d.Slug)
		if s == nil {
			methode = append(methode, d)
			continue
		}
		l = layout
		l.Title = s.Nom
		if err := write(ts, filepath.Join(out, filepath.FromSlash(s.URL()), "index.html"), struct {
			Layout
			S *Sujet
		}{l, s}); err != nil {
			return err
		}
		if err := redirection(filepath.Join(out, "comprendre", d.Slug, "index.html"), root+"/"+s.URL()); err != nil {
			return err
		}
		// La famille « Fiscalité » a regroupé cinq sujets auparavant répartis
		// entre « Argent public » et « Travail, économie » : deux d'entre eux
		// (tva, depenses-fiscales) changent donc d'adresse. Un lien externe ou
		// un signet vers l'ancienne adresse ne doit jamais tomber en erreur.
		if ancienneBase, deplace := anciennesBasesSujets[s.ID]; deplace {
			if err := redirection(filepath.Join(out, ancienneBase, s.ID, "index.html"), root+"/"+s.URL()); err != nil {
				return err
			}
		}
	}

	l = layout
	l.Title = "Documents de méthode"
	if err := write(page("comprendre.gohtml"), filepath.Join(out, "comprendre", "index.html"),
		struct {
			Layout
			Docs    []*Doc
			Groupes []GroupeDocs
		}{l, methode, GrouperDocs(methode)}); err != nil {
		return err
	}
	td := page("doc.gohtml")
	for _, d := range methode {
		var autres []*Doc
		for _, o := range methode {
			if o.Slug != d.Slug {
				autres = append(autres, o)
			}
		}
		l = layout
		l.Title = d.Titre
		if err := write(td, filepath.Join(out, "comprendre", d.Slug, "index.html"), struct {
			Layout
			D      *Doc
			Autres []*Doc
		}{l, d, autres}); err != nil {
			return err
		}
	}

	// L'index de recherche est écrit en dernier : il référence les thèmes, les
	// documents de méthode et les sénateurs, tous chargés plus haut.
	senateurs := map[string]bool{}
	for _, p := range senat.Senateurs2 {
		senateurs[p.Slug] = true
	}
	if err := ecrireIndex(out, persons, candidats, orgs, groupes, refs,
		themes.Themes, docs, senateurs); err != nil {
		return err
	}

	// --- fiches personnes (les candidats obtiennent en plus une URL dédiée)
	locaux, err := loadMandatsLocaux(ctx, pool, dataDir+"/candidats-mandats-locaux.csv")
	if err != nil {
		return err
	}
	situerMandatsLocaux(locaux, lieux)
	par2027 := map[string]*Candidat2027{}
	for _, k := range e27.Candidats {
		par2027[k.Slug] = k
	}

	tp := page("personne.gohtml")
	byCand := map[string]*Candidat{}
	for _, c := range candidats {
		if c.Person != nil {
			byCand[c.Person.Slug] = c
		}
	}
	if err := loadVotesBulk(ctx, pool, persons, 60); err != nil {
		return err
	}
	for _, p := range persons {
		l := layout
		l.Title = p.Prenom + " " + p.Nom
		data := struct {
			Layout
			P             *Person
			Cand          *Candidat
			Local         *RapprochementRNE
			K             *Candidat2027
			Defs          template.HTML
			TotalScrutins int
		}{l, p, byCand[p.Slug], nil, nil, "", layout.Cov.Scrutins}
		if c := byCand[p.Slug]; c != nil {
			data.Local, data.K = locaux[c.Slug], par2027[c.Slug]
			if data.K != nil && !data.K.Apercu.Vide {
				data.Defs = e27.Defs
			}
		}
		if err := write(tp, filepath.Join(out, "depute", p.Slug, "index.html"), data); err != nil {
			return err
		}
	}
	for _, c := range candidats {
		l := layout
		l.Title = c.Prenom + " " + c.Nom
		p := c.Person
		if p == nil {
			p = &Person{Slug: c.Slug, Prenom: c.Prenom, Nom: c.Nom}
		}
		k := par2027[c.Slug]
		var defs template.HTML
		if k != nil && !k.Apercu.Vide {
			defs = e27.Defs
		}
		data := struct {
			Layout
			P             *Person
			Cand          *Candidat
			Local         *RapprochementRNE
			K             *Candidat2027
			Defs          template.HTML
			TotalScrutins int
		}{l, p, c, locaux[c.Slug], k, defs, layout.Cov.Scrutins}
		if err := write(tp, filepath.Join(out, "candidat", c.Slug, "index.html"), data); err != nil {
			return err
		}
	}

	// --- fiches organisations
	to := page("organisation.gohtml")
	for _, o := range orgs {
		l := layout
		l.Title = o.Libelle
		if err := write(to, filepath.Join(out, "organisation", o.Slug, "index.html"),
			struct {
				Layout
				O *Organisation
			}{l, o}); err != nil {
			return err
		}
	}

	// --- fiches référentiels
	tr := page("referentiel.gohtml")
	for _, r := range refs {
		var concernees []*Organisation
		for _, o := range orgList {
			for _, c := range o.Classifications {
				if c.SetSlug == r.Slug {
					concernees = append(concernees, o)
					break
				}
			}
		}
		l := layout
		l.Title = r.Titre
		if err := write(tr, filepath.Join(out, "referentiel", r.Slug, "index.html"),
			struct {
				Layout
				R    *Referentiel
				Orgs []*Organisation
			}{l, r, concernees}); err != nil {
			return err
		}
	}

	// --- fiches groupes parlementaires
	tg := page("groupe.gohtml")
	for _, g := range groupes {
		l := layout
		l.Title = g.Nom
		if err := write(tg, filepath.Join(out, "groupe", g.Slug, "index.html"),
			struct {
				Layout
				G *Groupe
			}{l, g}); err != nil {
			return err
		}
	}

	// --- fiches scrutins
	// La citation d'un scrutin porte l'empreinte de la source dont il provient :
	// un lien qu'on colle vaut mieux qu'une capture d'écran.
	srcScrutins := SourceInfo{Attribution: "Assemblée nationale, open data, Licence Ouverte"}
	for _, s := range layout.Sources {
		if strings.Contains(strings.ToLower(s.Label), "scrutins") {
			srcScrutins = s
			break
		}
	}
	fmt.Printf("  avant les scrutins : %s écoulées\n", time.Since(start).Round(time.Second))
	var n int
	// maxScrutins tronque volontairement (itération locale, voir -max-scrutins) :
	// une construction tronquée ne doit jamais être prise pour la construction
	// complète à laquelle un lancement futur, sans troncature, se comparerait.
	scrutinsInchanges, etatScrutins, err := sectionInchangee(ctx, pool, ancienCache, "scrutin")
	if err != nil {
		return err
	}
	if maxScrutins == 0 {
		nouveauCache.Sections["scrutin"] = etatScrutins
	} else if prec, ok := ancienCache.Sections["scrutin"]; ok {
		nouveauCache.Sections["scrutin"] = prec // inchangé : ne pas écraser par un état tronqué
	}
	if !exclu("scrutin") && maxScrutins == 0 && scrutinsInchanges {
		n = ancienCache.Sections["scrutin"].N
		etatScrutins.N = n
		nouveauCache.Sections["scrutin"] = etatScrutins
		fmt.Printf("    scrutins : données et gabarits inchangés, recopiés depuis le site précédent (%d)\n", n)
		if err := copierRepertoire(filepath.Join(siteActuel, "scrutin"), filepath.Join(out, "scrutin")); err != nil {
			return err
		}
	} else if !exclu("scrutin") {
		if n, err = buildScrutins(ctx, pool, page("scrutin.gohtml"), layout, out, maxScrutins,
			seuils, srcScrutins); err != nil {
			return err
		}
		if maxScrutins == 0 {
			etatScrutins.N = n
			nouveauCache.Sections["scrutin"] = etatScrutins
		}
	}

	// Page d'erreur 404 : un fichier à la racine (pas .../404/index.html),
	// pour que le Caddyfile puisse la servir par un simple
	// rewrite * /{http.error.status_code}.html dans un bloc handle_errors,
	// sans connaître l'arborescence du site.
	l = layout
	l.Title = "Page introuvable"
	if err := write(page("404.gohtml"), filepath.Join(out, "404.html"), l); err != nil {
		return err
	}

	// Plan du site et robots.txt : en dernier, une fois que out/ porte
	// exactement l'arborescence publiée — voir cmd/build/sitemap.go.
	nSitemap, err := ecrireSitemap(out, layout.CanonicalBase)
	if err != nil {
		return err
	}
	if err := ecrireRobots(out, layout.CanonicalBase); err != nil {
		return err
	}
	fmt.Printf("  plan du site : %d URL, %s\n", nSitemap, layout.CanonicalBase+"/sitemap.xml")

	fmt.Printf("site généré dans %s/ : %d députés, %d candidats, %d organisations, %d groupes, %d scrutins (%s)\n",
		out, len(persons), len(candidats), len(orgs), len(groupes), n, time.Since(start).Round(time.Millisecond))

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
		[]byte("Répertoire produit par cmd/build. Effacé et réécrit à chaque construction.\n"), 0o644)
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
	err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id=b.scrutin_id
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
		       (SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id=b.scrutin_id
		         WHERE s.institution='SENAT'),
		       (SELECT count(DISTINCT b.person_id) FROM core.ballot b
		         JOIN core.scrutin s ON s.id=b.scrutin_id WHERE s.institution='SENAT'),
		       (SELECT count(*) FROM ref.topic WHERE taxonomy_version='senat'),
		       (SELECT count(DISTINCT commune_code) FROM core.commune_indicator)`).
		Scan(&c.Documents, &c.ScrutinsPE, &c.Themes,
			&c.ScrutinsSenat, &c.VotesSenat, &c.Senateurs, &c.ThemesSenat, &c.Communes)
	return c, err
}
