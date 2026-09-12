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
}

type Layout struct {
	Title, Root, BuiltAt string
	Hero                 bool
	HeroTitre, HeroLede  string
	DerniereIngestion    string
	Sources              []SourceInfo
	Cov                  Coverage
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
		"lower": strings.ToLower, "nb": Nombre, "ico": Icone}
	base := template.Must(template.New("base.gohtml").Funcs(fns).
		ParseFiles(filepath.Join(tplDir, "base.gohtml")))
	page := func(name string) *template.Template {
		t := template.Must(base.Clone())
		return template.Must(t.ParseFiles(filepath.Join(tplDir, name)))
	}

	layout := Layout{Root: root, BuiltAt: time.Now().Format("2 January 2006 à 15:04")}
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
	sort.Slice(deputes, func(i, j int) bool {
		return CleTri(deputes[i].Nom+" "+deputes[i].Prenom) < CleTri(deputes[j].Nom+" "+deputes[j].Prenom)
	})

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

	layout.Cov.Organisations = len(orgs)

	l := layout
	l.Title = "Accueil"
	l.Hero = true
	l.HeroTitre = "Ce que les responsables politiques ont réellement voté"
	_ = pool.QueryRow(ctx, `
		SELECT coalesce(to_char(max(fetched_at),'DD/MM/YYYY'),'')
		FROM raw.retrieval WHERE document_id IS NOT NULL`).Scan(&l.DerniereIngestion)
	l.HeroLede = "Chaque chiffre remonte à un document officiel archivé et horodaté. " +
		"Rien n'est commenté : le site documente ce qui a été proposé, voté et décidé, " +
		"et laisse la conclusion au lecteur."
	if err := write(page("accueil.gohtml"), filepath.Join(out, "index.html"), l); err != nil {
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
	for _, sec := range []struct{ dir, titre, tpl string }{
		{"europe", "Parlement européen", "europe.gohtml"},
		{"themes", "Thèmes", "themes.gohtml"},
	} {
		l = layout
		l.Title = sec.titre
		if err := write(page(sec.tpl), filepath.Join(out, sec.dir, "index.html"), struct {
			Layout
			E *StatsEurope
		}{l, europe}); err != nil {
			return err
		}
	}

	// Section non couverte : déclarée comme telle, avec sa raison typée.
	// Une absence affichée vaut mieux qu'une absence tue.
	tv := page("vide.gohtml")
	type vide struct {
		Layout
		Titre, Chapeau, Raison, Code string
	}
	sections := []struct {
		dir string
		v   vide
	}{
		{"senat", vide{Titre: "Sénat",
			Chapeau: "Aucune donnée du Sénat n'est ingérée à ce jour.",
			Raison: "Le Sénat publie ses données sous Licence Ouverte, en dumps PostgreSQL " +
				"complets — sénateurs, dossiers depuis 1977, amendements, questions, comptes " +
				"rendus. Le connecteur reste à écrire, et il est d'une autre nature que les " +
				"précédents : il faut restaurer un dump entier pour en extraire ce qui nous " +
				"intéresse. Une particularité l'attend : les scrutins y sont publiés PAR " +
				"GROUPE, avec la liste nominative des seules exceptions. On ne pourra donc " +
				"jamais y dire comment un sénateur donné a voté, sauf s'il figure parmi les " +
				"exceptions nommées — le schéma le prévoit déjà (granularité GROUP).",
			Code: "NOT_INGESTED"}},
	}
	for _, sec := range sections {
		sec.v.Layout = layout
		sec.v.Layout.Title = sec.v.Titre
		if err := write(tv, filepath.Join(out, sec.dir, "index.html"), sec.v); err != nil {
			return err
		}
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
	n, err := buildScrutins(ctx, pool, page("scrutin.gohtml"), layout, out, maxScrutins)
	if err != nil {
		return err
	}

	fmt.Printf("site généré dans %s/ : %d députés, %d candidats, %d organisations, %d groupes, %d scrutins (%s)\n",
		out, len(persons), len(candidats), len(orgs), len(groupes), n, time.Since(start).Round(time.Millisecond))
	return nil
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
	if err != nil {
		return c, err
	}
	err = pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM raw.document),
		       (SELECT count(*) FROM core.scrutin WHERE institution='PARLEMENT_EUROPEEN'),
		       (SELECT count(DISTINCT topic_code) FROM core.topic_assignment)`).
		Scan(&c.Documents, &c.ScrutinsPE, &c.Themes)
	return c, err
}
