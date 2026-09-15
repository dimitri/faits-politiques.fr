// Commande build : génère le site statique à partir de core.
//
// Aucune donnée n'est calculée ici qui ne vienne de la base : le rendu est une
// projection, pas une source. Toute page publiée doit pouvoir remonter à un
// document scellé (docs/perimetre.md §5.1).
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	DerniereIngestion   string
	Sources             []SourceInfo
	Cov                 Coverage
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
	flag.Parse()

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
	if err := run(chantier, *tpl, *dataDir, *root, *maxScrutins); err != nil {
		fmt.Fprintf(os.Stderr, "erreur : %v\n", err)
		os.Exit(1)
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

func run(out, tplDir, dataDir, root string, maxScrutins int) error {
	start := time.Now()
	ctx := context.Background()
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	fns := template.FuncMap{"jauge": Jauge, "poleG": PoleGauche, "poleD": PoleDroit,
		"lower": strings.ToLower, "nb": Nombre, "ico": Icone,
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
		CSS: assets.CSS, JS: assets.JS}
	if layout.Sources, err = sources(ctx, pool); err != nil {
		return err
	}
	if layout.Cov, err = coverage(ctx, pool); err != nil {
		return err
	}

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
	flux, err := derniersFlux(ctx, pool, 8)
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
	l.Title = "Accueil"
	l.Hero = true
	l.HeroTitre = "Ce qui a été voté, décidé, proposé — et d'où on le sait."
	// Se définir par une absence (« rien n'est commenté ») oblige le lecteur à
	// deviner ce qu'il obtient. On dit les trois choses qu'il reçoit.
	l.HeroLede = "Chaque chiffre remonte à un document officiel archivé et horodaté. " +
		"Aucun verdict n'est rendu : vous obtenez le fait, sa source primaire, " +
		"et ce qu'elle ne permet pas de conclure."
	// La carte de une. Choix éditorial : celle qui répond à une intuition
	// fausse plutôt que celle qui a la plus jolie donnée. « Les municipales
	// sont-elles des élections de partis ? » — non, et la carte le montre sans
	// une phrase de commentaire.
	const carteUne = "part-partisane"
	var une *CarteTerritoire
	for i := range terr.Cartes {
		if terr.Cartes[i].Slug == carteUne {
			une = &terr.Cartes[i]
		}
	}
	if err := write(page("accueil.gohtml"), filepath.Join(out, "index.html"), struct {
		Layout
		Derniers []FluxLigne
		Une      *CarteTerritoire
		Defs     template.HTML
	}{l, flux, une, terr.Defs}); err != nil {
		return err
	}

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
	l = layout
	l.Title = "Sources"
	if err := write(page("sources.gohtml"), filepath.Join(out, "sources", "index.html"), struct {
		Layout
		Flux []SourceDetail
	}{l, sources}); err != nil {
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

	// --- territoires, sécurité, présidentielle 2027 : les nouvelles sections
	l = layout
	l.Title = "Territoires"
	if err := write(page("territoires.gohtml"), filepath.Join(out, "territoires", "index.html"),
		struct {
			Layout
			T *StatsTerritoires
		}{l, terr}); err != nil {
		return err
	}

	// Une page par carte : tracé fin, classement complet, série annuelle.
	tcd := page("carte-detail.gohtml")
	for _, c := range terr.Cartes {
		l = layout
		l.Title = c.Titre
		if err := write(tcd, filepath.Join(out, "territoires", c.Slug, "index.html"),
			struct {
				Layout
				P PageCarte
			}{l, c.Page}); err != nil {
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
		if err := write(tcd, filepath.Join(out, "securite", ind.Slug, "index.html"),
			struct {
				Layout
				P PageCarte
			}{l, ind.Page}); err != nil {
			return err
		}
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
	// La carte d'index porte le fonctionnement : c'est le budget qui tourne
	// chaque année, celui qui décrit le mieux ce que la collectivité fait.
	relierFiches(col, avecFiche)
	const indicCarte = "ofgl.fonctionnement_par_hab"
	if err := cartesCollectivites(ctx, pool, col, indicCarte); err != nil {
		return err
	}
	type barreNiveau struct {
		Titre  string
		Graphe template.HTML
	}
	var barres []barreNiveau
	for _, ind := range indicsCollectivite {
		barres = append(barres, barreNiveau{ind.Libelle, barresNiveaux(col.Poids, ind.Code)})
	}
	l = layout
	l.Title = "Collectivités"
	if err := write(page("collectivites.gohtml"),
		filepath.Join(out, "collectivites", "index.html"), struct {
			Layout
			C            *StatsCollectivites
			Ind          []IndicCollectivite
			Barres       []barreNiveau
			IndicLibelle string
		}{l, col, indicsCollectivite, barres, "Dépenses de fonctionnement"}); err != nil {
		return err
	}

	// --- les lieux : communes et intercommunalités, et la chaîne de lieux de
	// chaque mandat affiché ailleurs sur le site.
	lieux, err := chargerResolveur(ctx, pool, root, col)
	if err != nil {
		return err
	}
	for _, pp := range persons {
		lieux.situerMandats(pp)
	}
	pagesCom, err := chargerPagesCommunes(ctx, pool, lieux, avecFiche)
	if err != nil {
		return err
	}
	tcom := page("commune.gohtml")
	for code, pc := range pagesCom {
		l = layout
		l.Title = pc.Nom + " (" + pc.Dept.Code + ")"
		if err := write(tcom, filepath.Join(out, "collectivites", "commune", code, "index.html"),
			struct {
				Layout
				C *PageCommune
			}{l, pc}); err != nil {
			return err
		}
	}
	pagesEPCI, err := chargerPagesEPCI(ctx, pool, lieux, col, avecFiche)
	if err != nil {
		return err
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
	fmt.Printf("  lieux : %d communes, %d intercommunalités\n", len(pagesCom), len(pagesEPCI))

	// Les pages de région et de département, maintenant que le résolveur sait
	// quels départements composent une région.
	tcol := page("collectivite.gohtml")
	pagesCol, err := pagesCollectivites(ctx, pool, col, lieux)
	if err != nil {
		return err
	}
	for _, pc := range pagesCol {
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
	}

	l = layout
	l.Title = "Comprendre"
	if err := write(page("comprendre.gohtml"), filepath.Join(out, "comprendre", "index.html"),
		struct {
			Layout
			Docs    []*Doc
			Groupes []GroupeDocs
		}{l, docs, GrouperDocs(docs)}); err != nil {
		return err
	}
	td := page("doc.gohtml")
	for _, d := range docs {
		var autres []*Doc
		for _, o := range docs {
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
	for _, p := range persons {
		if err := loadVotes(ctx, pool, p, 60); err != nil {
			return err
		}
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
	n, err := buildScrutins(ctx, pool, page("scrutin.gohtml"), layout, out, maxScrutins,
		seuils, srcScrutins)
	if err != nil {
		return err
	}

	fmt.Printf("site généré dans %s/ : %d députés, %d candidats, %d organisations, %d groupes, %d scrutins (%s)\n",
		out, len(persons), len(candidats), len(orgs), len(groupes), n, time.Since(start).Round(time.Millisecond))
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
