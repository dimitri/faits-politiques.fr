// Commande build : génère le site statique à partir de core.
//
// Aucune donnée n'est calculée ici qui ne vienne de la base : le rendu est une
// projection, pas une source. Toute page publiée doit pouvoir remonter à un
// document scellé (docs/perimetre.md §5.1).
package main

import (
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

	if err := run(*out, *tpl, *dataDir, *root, *maxScrutins); err != nil {
		fmt.Fprintf(os.Stderr, "erreur : %v\n", err)
		os.Exit(1)
	}
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
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"odd": func(i int) bool { return i%2 == 1 }}
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
	layout := Layout{Root: root, BuiltAt: time.Now().Format("2 January 2006 à 15:04"),
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

	l := layout
	l.Title = "Accueil"
	l.Hero = true
	l.HeroTitre = "Ce qui a été voté, décidé, proposé — et d'où on le sait."
	_ = pool.QueryRow(ctx, `
		SELECT coalesce(to_char(max(fetched_at),'DD/MM/YYYY'),'')
		FROM raw.retrieval WHERE document_id IS NOT NULL`).Scan(&l.DerniereIngestion)
	// Se définir par une absence (« rien n'est commenté ») oblige le lecteur à
	// deviner ce qu'il obtient. On dit les trois choses qu'il reçoit.
	l.HeroLede = "Chaque chiffre remonte à un document officiel archivé et horodaté. " +
		"Aucun verdict n'est rendu : vous obtenez le fait, sa source primaire, " +
		"et ce qu'elle ne permet pas de conclure."
	if err := write(page("accueil.gohtml"), filepath.Join(out, "index.html"), struct {
		Layout
		Derniers []FluxLigne
	}{l, flux}); err != nil {
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
	terr, err := loadTerritoires(ctx, pool)
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Territoires"
	if err := write(page("territoires.gohtml"), filepath.Join(out, "territoires", "index.html"),
		struct {
			Layout
			T *StatsTerritoires
		}{l, terr}); err != nil {
		return err
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

	// --- comprendre : les documents de méthode, rendus en pages
	docs, err := loadDocs("docs")
	if err != nil {
		return err
	}
	l = layout
	l.Title = "Comprendre"
	if err := write(page("comprendre.gohtml"), filepath.Join(out, "comprendre", "index.html"),
		struct {
			Layout
			Docs []*Doc
		}{l, docs}); err != nil {
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
			TotalScrutins int
		}{l, p, byCand[p.Slug], layout.Cov.Scrutins}
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
		data := struct {
			Layout
			P             *Person
			Cand          *Candidat
			TotalScrutins int
		}{l, p, c, layout.Cov.Scrutins}
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
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.ExecuteTemplate(f, "base", data)
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
