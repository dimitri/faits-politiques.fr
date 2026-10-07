package sitegen

// Ce fichier construit le graphe de dépendances d'une construction — chaque
// page (le nom qu'-only reconnaît) et chaque donnée qu'elle lit devient un
// nœud nommé de internal/pipeline.Registre, plutôt que la longue chaîne
// séquentielle que run() exécutait auparavant en entier à chaque fois. Deux
// conséquences directes :
//
//   - "fpctl build page X" ne charge plus que ce dont X dépend RÉELLEMENT
//     (Registre.Niveaux calcule la fermeture transitive) — pas la totalité
//     du site comme avant, où seule l'ÉCRITURE finale était filtrée par
//     -only (voir l'ancien commentaire d'ecrire, toujours vrai pour ce qui
//     reste hors graphe : 404, plan du site, robots.txt).
//   - les nœuds indépendants d'une même vague tournent de front
//     (pipeline.Registre.Executer, un errgroup par vague) — une construction
//     complète ou par catégorie profite du même parallélisme qu'un ingest,
//     là où seuls trois endroits du code en profitaient avant (voir
//     lieux_pages.go, l'écriture des pages communes et scrutin).
//
// Ce que le graphe NE modélise PAS : deux amas déjà profondément
// entremêlés dans le code existant restent UN SEUL nœud grossier plutôt que
// d'être découpés artificiellement.
//
//   - "identite" (persons, candidats, référentiels, organisations, groupes,
//     présidences, coalitions, médias, avecFiche) : chaque étape mute la
//     précédente en place (les présidences annotent les mandats ministre
//     d'une personne, les coalitions annotent un groupe, les médias
//     annotent candidats ET organisations) — les séparer demanderait de
//     retracer ces mutations une par une pour un gain quasi nul, puisque
//     presque toutes les sections qui lisent CE nœud en lisent plusieurs
//     champs à la fois de toute façon.
//   - "sujets-data" (docs Markdown + une trentaine de schémas calculés,
//     insérés dans le texte à l'endroit d'un marqueur) : quel marqueur vit
//     dans quel document n'est connu qu'en relisant le Markdown, donc ce
//     nœud dépend de TOUS les schémas plutôt que d'un sous-ensemble déduit
//     à l'avance — une dépendance large mais honnête, pas une fausse
//     indépendance qui casserait un marqueur oublié.
//
// La granularité retenue partout ailleurs suit un principe simple : un nœud
// par chargerX/loadX du fichier d'origine, plus un nœud par section -only
// qui ne fait qu'écrire ce qu'un chargement a déjà produit — exactement la
// frontière que le code séquentiel dessinait déjà lui-même par ses propres
// variables et ses propres appels à ecrire().
import (
	"context"
	"fmt"
	"html/template"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dep extrait, avec son vrai type, ce qu'une dépendance a produit — un zéro
// (map/slice/pointeur nil) si le nœud demandé n'a pas tourné cette fois
// (impossible en pratique : Registre.Niveaux refuse une cible dont une
// dépendance n'a pas déjà été ajoutée au registre) ou n'a rien retourné.
func dep[T any](deps pipeline.Results, name string) T {
	v, _ := deps[name].(T)
	return v
}

// addNode enregistre un nœud dont la valeur produite a un type précis —
// tout le graphe passe par cet unique point pour éviter de répéter, à
// chaque nœud, la conversion vers Results (map[string]any) qu'Executer
// exige.
func addNode[T any](region *pipeline.Registre, name string, deps []string,
	fn func(ctx context.Context, d pipeline.Results) (T, error)) {
	region.Ajouter(pipeline.Etape{
		Nom: name, Description: "loading " + name, Dependances: deps,
		Executer: func(ctx context.Context, d pipeline.Results) (any, error) { return fn(ctx, d) },
	})
}

// identityBundle : voir la note en tête de fichier — un seul nœud pour ce
// que loadPersons, loadCandidats, loadReferentiels, loadOrganisations,
// loadGroupes, loadPresidences, loadCoalitions et loadMedias produisaient
// et s'entre-annotaient, dans le même ordre qu'avant.
type identityBundle struct {
	Persons      map[string]*Person
	Candidates   []*Candidate
	Refs         map[string]*Reference
	Tags         map[string]Tag
	Orgs         map[string]*Organization
	Groups       map[string]*Group
	Presidencies []Presidency
	Deputies     []*Person
	OrgList      []*Organization
	GrpList      []*Group
	WithProfile  map[string]bool
	// Cov porte les décomptes que seul ce nœud connaît (candidats, avec
	// bilan, primaire, organisations) — fusionnés dans Layout par
	// buildRegistry avant que la moindre page ne parte à l'écriture.
	Cov Coverage
}

type localGovBundle struct {
	Col      *StatsAuthorities
	TableDep TableExpenses
	Shares   []ShareRevenue
}

type districtsBundle struct {
	Circos     map[string]*PageDistrict
	PopFrance  int
	CircosDept map[string][]Place
}

// topicsReady : le marqueur d'exécution du gros nœud "sujets-data" — ses
// vrais résultats (docs mutés, acc.Familles rempli) restent dans les mêmes
// pointeurs que docs/accueil, déjà partagés ; ses dépendants n'ont besoin
// que de savoir qu'il a bien tourné avant de les lire.
type topicsReady struct{}

// buildRegistry déclare tous les nœuds d'une construction. env porte
// tout ce qu'un chargement ou une écriture a besoin de l'environment de
// run() (pool, chemins, gabarits) — jamais une dépendance au sens du
// graphe, seulement des constantes pour la durée de la construction.
type environment struct {
	pool               *pgxpool.Pool
	dataDir, out, root string
	start              time.Time
	layout             Layout // Sources/Cov(partiel)/DerniereIngestion déjà posés par run()
	page               func(string) *template.Template
	writeSection       func(section string, t *template.Template, path string, data any) error
	writeAlways        func(t *template.Template, path string, data any) error
	excluded           func(section string) bool
	maxElections       int
	previousCacheValue manifestCache
	currentSite        string
	newCache           *manifestCache
}

// previousCache : accesseur pour graphe_sections.go — un simple champ
// suffirait, mais nommer l'accès rend explicite qu'il ne s'agit jamais de
// newCache (celui-ci muté par les nœuds, celui-là jamais).
func (e *environment) previousCache() manifestCache { return e.previousCacheValue }

// layoutWithIdentity : e.layout complété des décomptes que seul le nœud
// identite calcule (candidats, avec bilan, primaire, organisations) — dans
// l'ancien run() séquentiel, identite les écrivait directement dans layout
// avant que la moindre page ne parte à l'écriture ; ici, où identite est un
// nœud comme un autre, seules web/templates/qui-decide.gohtml et accueil.
// gohtml lisent ces quatre champs (grep sur web/templates/*.gohtml) — leurs
// deux nœuds seuls dépendent d'identite et appellent ceci plutôt que de
// lire e.layout nu, qui ne les porterait jamais.
func (e *environment) layoutWithIdentity(id identityBundle) Layout {
	l := e.layout
	l.Cov.Candidates = id.Cov.Candidates
	l.Cov.CandidatesWithSummary = id.Cov.CandidatesWithSummary
	l.Cov.CandidatesPrimary = id.Cov.CandidatesPrimary
	l.Cov.Organizations = id.Cov.Organizations
	return l
}

func buildRegistry(env *environment) *pipeline.Registre {
	region := pipeline.NouveauRegistre(nil)
	e := env
	tcd := e.page("carte-detail.gohtml")

	// --- identité : voir la note de tête de fichier.
	addNode(region, "identite", nil, func(ctx context.Context, _ pipeline.Results) (identityBundle, error) {
		persons, err := loadPersons(ctx, e.pool, e.layout.Cov.Elections)
		if err != nil {
			return identityBundle{}, err
		}
		candidates, err := loadCandidates(filepath.Join(e.dataDir, "candidats.csv"), persons)
		if err != nil {
			return identityBundle{}, err
		}
		refs, tags, err := loadReferences(filepath.Join(e.dataDir, "referentiels.csv"))
		if err != nil {
			return identityBundle{}, err
		}
		orgs, err := loadOrganizations(ctx, e.pool, filepath.Join(e.dataDir, "organisations.csv"), tags)
		if err != nil {
			return identityBundle{}, err
		}
		groups, err := loadGroups(ctx, e.pool)
		if err != nil {
			return identityBundle{}, err
		}
		presidencies, err := loadPresidencies(filepath.Join(e.dataDir, "presidents.csv"))
		if err != nil {
			return identityBundle{}, err
		}
		for _, p := range persons {
			for i, m := range p.Terms {
				if m.Type != "MINISTRE" {
					continue
				}
				if noms := presidenciesOf(presidencies, m.StartISO, m.EndISO); len(noms) > 0 {
					p.Terms[i].SubPresidency = strings.Join(noms, ", ")
				}
			}
		}
		coalitions, err := loadCoalitions(filepath.Join(e.dataDir, "coalitions.csv"), orgs)
		if err != nil {
			return identityBundle{}, err
		}
		for _, g := range groups {
			for _, c := range coalitions {
				if strings.Contains(strings.ToLower(g.Name), strings.ToLower(c.Label)) {
					g.Coalitions = append(g.Coalitions, c)
				}
			}
		}
		portraits, logos, err := loadMedias(ctx, e.pool)
		if err != nil {
			return identityBundle{}, err
		}
		if err := copyMedia("web/media", filepath.Join(e.out, "media")); err != nil {
			return identityBundle{}, err
		}
		if err := copyMedia("web/fonts", filepath.Join(e.out, "fonts")); err != nil {
			return identityBundle{}, err
		}
		for _, c := range candidates {
			c.Portrait = portraits[c.Slug]
		}
		for _, o := range orgs {
			if g, ok := groups[o.GroupANUID]; ok {
				o.Group = g
				g.Parties = append(g.Parties, o)
			}
			if o.OrgID != 0 {
				o.Logo = logos[o.OrgID]
			}
		}
		for _, c := range candidates {
			if o, ok := orgs[c.OrganizationSlug]; ok {
				c.Org = o
				o.Candidates = append(o.Candidates, c)
			}
		}
		var cov Coverage
		cov.Candidates = len(candidates)
		for _, c := range candidates {
			if c.Person != nil && c.Person.HasVotes {
				cov.CandidatesWithSummary++
			}
			if c.Status == "PRIMAIRE" {
				cov.CandidatesPrimary++
			}
		}
		cov.Organizations = len(orgs)

		deputies := make([]*Person, 0, len(persons))
		for _, p := range persons {
			deputies = append(deputies, p)
		}
		sortPeople(deputies)
		orgList := make([]*Organization, 0, len(orgs))
		for _, o := range orgs {
			orgList = append(orgList, o)
		}
		sort.Slice(orgList, func(i, j int) bool { return KeySort(orgList[i].Label) < KeySort(orgList[j].Label) })
		grpList := make([]*Group, 0, len(groups))
		for _, g := range groups {
			grpList = append(grpList, g)
		}
		sort.Slice(grpList, func(i, j int) bool { return KeySort(grpList[i].Name) < KeySort(grpList[j].Name) })
		withProfile := map[string]bool{}
		for _, pp := range persons {
			withProfile[pp.Slug] = true
		}
		return identityBundle{persons, candidates, refs, tags, orgs, groups, presidencies,
			deputies, orgList, grpList, withProfile, cov}, nil
	})

	addNode(region, "territoires", nil, func(ctx context.Context, _ pipeline.Results) (*StatsTerritories, error) {
		return loadTerritories(ctx, e.pool)
	})
	addNode(region, "derniers-scrutins", nil, func(ctx context.Context, _ pipeline.Results) ([]Vote, error) {
		return lastElections(ctx, e.pool, 60)
	})
	addNode(region, "seuils", nil, func(_ context.Context, _ pipeline.Results) (map[string]Threshold, error) {
		return loadThresholds(filepath.Join(e.dataDir, "seuils.csv"))
	})
	addNode(region, "docs", nil, func(_ context.Context, _ pipeline.Results) ([]*Doc, error) {
		return loadDocs("docs")
	})

	// --- pages simples : un chargement, une écriture, jamais relu ailleurs.
	// Regroupées ici plutôt que dispersées dans l'ordre du fichier d'origine
	// (perdu de toute façon : Niveaux ordonne par dépendance, pas par
	// déclaration) — chacune une ligne, le même patron partout.
	addPageNode(region, e, "frise", nil, "La Ve République en chiffres", "frise.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsTimeline, error) {
			return loadTimeline(ctx, e.pool, e.dataDir)
		},
		func(l Layout, fr *StatsTimeline) (string, any) {
			return filepath.Join(e.out, "frise", "index.html"), struct {
				Layout
				F *StatsTimeline
			}{l, fr}
		})
	addPageNode(region, e, "dette", nil, "La dette publique", "dette.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsDebt, error) { return loadDebt(ctx, e.pool) },
		func(l Layout, d *StatsDebt) (string, any) {
			return filepath.Join(e.out, "dette", "index.html"), struct {
				Layout
				D *StatsDebt
			}{l, d}
		})
	addPageNode(region, e, "chomage", nil, "Le taux de chômage", "chomage.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsUnemployment, error) {
			return loadUnemployment(ctx, e.pool)
		},
		func(l Layout, c *StatsUnemployment) (string, any) {
			return filepath.Join(e.out, "chomage", "index.html"), struct {
				Layout
				C *StatsUnemployment
			}{l, c}
		})
	addNode(region, "richesse-data", nil, func(ctx context.Context, _ pipeline.Results) (*StatsWealth, error) {
		return loadWealth(ctx, e.pool)
	})
	addPageNode(region, e, "richesse", []string{"richesse-data"}, "La répartition de la richesse en France", "richesse.gohtml",
		func(_ context.Context, d pipeline.Results) (*StatsWealth, error) {
			return dep[*StatsWealth](d, "richesse-data"), nil
		},
		func(l Layout, r *StatsWealth) (string, any) {
			return filepath.Join(e.out, "richesse", "index.html"), struct {
				Layout
				R *StatsWealth
			}{l, r}
		})
	addPageNode(region, e, "dividendes", nil, "Les dividendes versés", "dividendes.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsDividends, error) {
			return loadDividends(ctx, e.pool)
		},
		func(l Layout, d *StatsDividends) (string, any) {
			return filepath.Join(e.out, "dividendes", "index.html"), struct {
				Layout
				D *StatsDividends
			}{l, d}
		})
	addNode(region, "agriculture-data", nil, func(ctx context.Context, _ pipeline.Results) (*StatsAgri, error) {
		return loadAgriculture(ctx, e.pool, e.dataDir)
	})
	addPageNode(region, e, "agriculture", []string{"agriculture-data"}, "Agriculture et alimentation", "agriculture.gohtml",
		func(_ context.Context, d pipeline.Results) (*StatsAgri, error) {
			return dep[*StatsAgri](d, "agriculture-data"), nil
		},
		func(l Layout, a *StatsAgri) (string, any) {
			return filepath.Join(e.out, "agriculture", "index.html"), struct {
				Layout
				A *StatsAgri
			}{l, a}
		})
	addNode(region, "social-data", nil, func(ctx context.Context, _ pipeline.Results) (*StatsSocial, error) {
		return loadSocial(ctx, e.pool)
	})

	addNode(region, "europe", nil, func(ctx context.Context, _ pipeline.Results) (*StatsEurope, error) {
		return loadEurope(ctx, e.pool)
	})
	region.Ajouter(pipeline.Etape{Nom: "europe-page", Description: "page Parlement européen", Dependances: []string{"europe"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			europe := dep[*StatsEurope](d, "europe")
			l := e.layout
			l.Title = "Parlement européen"
			return nil, e.writeSection("europe", e.page("europe.gohtml"), filepath.Join(e.out, "europe", "index.html"), struct {
				Layout
				E *StatsEurope
			}{l, europe})
		}})

	addNode(region, "themes", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (*StatsThemes, error) {
		id := dep[identityBundle](d, "identite")
		var candSlugs []string
		orgPerSlug := map[string]string{}
		for _, c := range id.Candidates {
			if c.Person != nil {
				candSlugs = append(candSlugs, c.Person.Slug)
				orgPerSlug[c.Person.Slug] = c.Organization
			}
		}
		return loadThemes(ctx, e.pool, 60, candSlugs, orgPerSlug)
	})
	region.Ajouter(pipeline.Etape{Nom: "themes-page", Description: "page thèmes", Dependances: []string{"themes"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			themes := dep[*StatsThemes](d, "themes")
			l := e.layout
			l.Title = "Thèmes"
			if err := e.writeSection("themes", e.page("themes.gohtml"), filepath.Join(e.out, "themes", "index.html"), struct {
				Layout
				T *StatsThemes
			}{l, themes}); err != nil {
				return nil, err
			}
			tth := e.page("theme.gohtml")
			for _, th := range themes.Themes {
				maxG := 0
				for _, g := range th.Groups {
					if g.Total > maxG {
						maxG = g.Total
					}
				}
				l := e.layout
				l.Title = th.Label
				if err := e.writeSection("themes", tth, filepath.Join(e.out, "theme", th.Slug, "index.html"), struct {
					Layout
					Th       *Theme
					MaxGroup int
				}{l, th, maxG}); err != nil {
					return nil, err
				}
			}
			return nil, nil
		}})

	addNode(region, "senat", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (*StatsSenate, error) {
		return loadSenate(ctx, e.pool, dep[identityBundle](d, "identite").Persons)
	})
	region.Ajouter(pipeline.Etape{Nom: "senat-page", Description: "page Sénat", Dependances: []string{"senat"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			senate := dep[*StatsSenate](d, "senat")
			l := e.layout
			l.Title = "Sénat"
			return nil, e.writeSection("senat", e.page("senat.gohtml"), filepath.Join(e.out, "senat", "index.html"), struct {
				Layout
				Se *StatsSenate
			}{l, senate})
		}})

	addPageNode(region, e, "vieillesse", []string{"territoires"}, "La vieillesse : combien, qui paie, et la dépendance", "vieillesse.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsOldAge, error) {
			return loadOldAge(ctx, e.pool)
		},
		func(l Layout, v *StatsOldAge) (string, any) {
			return filepath.Join(e.out, "vieillesse", "index.html"), struct {
				Layout
				V *StatsOldAge
			}{l, v}
		})
	region.Ajouter(pipeline.Etape{Nom: "vieillesse-carte", Description: "carte vieillesse", Dependances: []string{"vieillesse"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			old := dep[*StatsOldAge](d, "vieillesse")
			// nil est un résultat légitime de l'étape dont celle-ci dépend :
			// addPageNode (graphe_sections.go, rienAPublier) renvoie tel quel
			// le zéro du chargeur quand il saute la page. loadVieillesse ne
			// s'en sert pas aujourd'hui, mais loadRichesse si — ne pas faire
			// de ce détail une condition de non-plantage d'ici.
			if old == nil || old.MapAPA.Slug == "" {
				return nil, nil
			}
			l := e.layout
			l.Title = old.MapAPA.Title
			l.Description = old.MapAPA.Question
			imageMap(&l, e.out, "vieillesse-"+old.MapAPA.Slug, old.MapAPA.Page.Map.SVG)
			return nil, e.writeSection("vieillesse", tcd, filepath.Join(e.out, "vieillesse", "carte", old.MapAPA.Slug, "index.html"),
				struct {
					Layout
					P PageMap
				}{l, old.MapAPA.Page})
		}})

	addPageNode(region, e, "jeunesse", nil, "La jeunesse : études supérieures, apprentissage, premiers emplois", "jeunesse.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsYouth, error) {
			return loadYouth(ctx, e.pool)
		},
		func(l Layout, j *StatsYouth) (string, any) {
			return filepath.Join(e.out, "jeunesse", "index.html"), struct {
				Layout
				J *StatsYouth
			}{l, j}
		})
	region.Ajouter(pipeline.Etape{Nom: "jeunesse-carte", Description: "carte jeunesse", Dependances: []string{"jeunesse"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			jeun := dep[*StatsYouth](d, "jeunesse")
			// Même raison qu'au-dessus pour vieillesse-carte : le nil que
			// rienAPublier reconnaît traverse addPageNode jusqu'ici.
			if jeun == nil || jeun.MapInsertion.Slug == "" {
				return nil, nil
			}
			l := e.layout
			l.Title = jeun.MapInsertion.Title
			l.Description = jeun.MapInsertion.Question
			imageMap(&l, e.out, "jeunesse-"+jeun.MapInsertion.Slug, jeun.MapInsertion.Page.Map.SVG)
			return nil, e.writeSection("jeunesse", tcd, filepath.Join(e.out, "jeunesse", "carte", jeun.MapInsertion.Slug, "index.html"),
				struct {
					Layout
					P PageMap
				}{l, jeun.MapInsertion.Page})
		}})

	addNode(region, "securite", nil, func(ctx context.Context, _ pipeline.Results) (*StatsSecurity, error) {
		return loadSecurity(ctx, e.pool)
	})
	region.Ajouter(pipeline.Etape{Nom: "securite-page", Description: "page sécurité", Dependances: []string{"securite"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			sec := dep[*StatsSecurity](d, "securite")
			l := e.layout
			l.Title = "Sécurité"
			if len(sec.Indicators) > 0 {
				imageMap(&l, e.out, "securite", sec.Indicators[0].Page.Map.SVG)
			}
			if err := e.writeSection("securite", e.page("securite.gohtml"), filepath.Join(e.out, "securite", "index.html"), struct {
				Layout
				S *StatsSecurity
			}{l, sec}); err != nil {
				return nil, err
			}
			for _, ind := range sec.Indicators {
				l := e.layout
				l.Title = ind.Label
				l.Description = ind.Page.Question
				imageMap(&l, e.out, "securite-"+ind.Slug, ind.Page.Map.SVG)
				if err := e.writeSection("securite", tcd, filepath.Join(e.out, "securite", ind.Slug, "index.html"), struct {
					Layout
					P PageMap
				}{l, ind.Page}); err != nil {
					return nil, err
				}
			}
			return nil, nil
		}})

	addNode(region, "accueil-data", []string{"territoires", "securite"},
		func(ctx context.Context, d pipeline.Results) (*DataHome, error) {
			return loadHome(ctx, e.pool, dep[*StatsTerritories](d, "territoires"), dep[*StatsSecurity](d, "securite"))
		})
	addNode(region, "fonctions", []string{"accueil-data"},
		func(ctx context.Context, d pipeline.Results) (map[string]*PageFunction, error) {
			return loadFunctions(ctx, e.pool, dep[*DataHome](d, "accueil-data"))
		})

	addNode(region, "election2027", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (*Stats2027, error) {
		return load2027(ctx, e.pool, dep[identityBundle](d, "identite").Candidates, e.dataDir)
	})
	region.Ajouter(pipeline.Etape{Nom: "election2027-page", Description: "page présidentielle 2027", Dependances: []string{"election2027"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			e27 := dep[*Stats2027](d, "election2027")
			l := e.layout
			l.Title = "Présidentielle 2027"
			if err := e.writeSection("election2027", e.page("election2027.gohtml"), filepath.Join(e.out, "2027", "index.html"), struct {
				Layout
				E *Stats2027
			}{l, e27}); err != nil {
				return nil, err
			}
			views := map[string]bool{}
			for _, k := range e27.Candidates {
				if k.Page == nil || views[k.Page.Slug] {
					continue
				}
				views[k.Page.Slug] = true
				l := e.layout
				l.Title = k.Page.Title
				l.Description = k.Page.Question
				imageMap(&l, e.out, "2027-"+k.Page.Slug, k.Page.Map.SVG)
				if err := e.writeSection("election2027", tcd, filepath.Join(e.out, "2027", k.Page.Slug, "index.html"), struct {
					Layout
					P PageMap
				}{l, *k.Page}); err != nil {
					return nil, err
				}
			}
			return nil, nil
		}})

	addNode(region, "gouvernement", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (*StatsGovernment, error) {
		return loadGovernment(ctx, e.pool, e.dataDir, dep[identityBundle](d, "identite").WithProfile)
	})
	region.Ajouter(pipeline.Etape{Nom: "gouvernement-page", Description: "page gouvernement", Dependances: []string{"gouvernement"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			gouv := dep[*StatsGovernment](d, "gouvernement")
			l := e.layout
			l.Title = "Gouvernement"
			if err := e.writeSection("gouvernement", e.page("gouvernement.gohtml"), filepath.Join(e.out, "gouvernement", "index.html"),
				struct {
					Layout
					G *StatsGovernment
				}{l, gouv}); err != nil {
				return nil, err
			}
			l = e.layout
			l.Title = "Tous les décrets de composition"
			allDecrees := *gouv
			allDecrees.Decrees = gouv.All
			return nil, e.writeSection("gouvernement", e.page("decrets.gohtml"), filepath.Join(e.out, "gouvernement", "decrets", "index.html"),
				struct {
					Layout
					G *StatsGovernment
				}{l, &allDecrees})
		}})

	// --- collectivités, lieux, communes/EPCI, circonscriptions : une vraie
	// chaîne de dépendances, contrairement à identite/sujets-data — chacune
	// se sépare proprement de la suivante.
	addNode(region, "collectivites-data", []string{"identite"},
		func(ctx context.Context, d pipeline.Results) (localGovBundle, error) {
			id := dep[identityBundle](d, "identite")
			col, err := loadAuthorities(ctx, e.pool)
			if err != nil {
				return localGovBundle{}, err
			}
			linkProfiles(col, id.WithProfile)
			if err := mapsAuthorities(ctx, e.pool, col, indicatorPopulation); err != nil {
				return localGovBundle{}, err
			}
			return localGovBundle{col, tableExpenses(col.Weight, indicsAuthority), sharesRevenues(col.Weight)}, nil
		})
	region.Ajouter(pipeline.Etape{Nom: "collectivites-page", Description: "page collectivités",
		Dependances: []string{"collectivites-data", "territoires"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			cb := dep[localGovBundle](d, "collectivites-data")
			territory := dep[*StatsTerritories](d, "territoires")
			l := e.layout
			l.Title = "Collectivités"
			imageMap(&l, e.out, "collectivites", cb.Col.MapDepts.SVG)
			return nil, e.writeSection("collectivites", e.page("collectivites.gohtml"), filepath.Join(e.out, "collectivites", "index.html"),
				struct {
					Layout
					C              *StatsAuthorities
					T              *StatsTerritories
					Ind            []IndicatorAuthority
					TableDep       TableExpenses
					Shares         []ShareRevenue
					IndicatorLabel string
				}{l, cb.Col, territory, indicsAuthority, cb.TableDep, cb.Shares, "Population"})
		}})
	region.Ajouter(pipeline.Etape{Nom: "collectivites-cartes", Description: "cartes départementales",
		Dependances: []string{"territoires"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			territory := dep[*StatsTerritories](d, "territoires")
			for _, c := range territory.Maps {
				l := e.layout
				l.Title = c.Title
				l.Description = c.Page.Question
				imageMap(&l, e.out, "carte-"+c.Slug, c.Page.Map.SVG)
				if err := e.writeSection("collectivites", tcd, filepath.Join(e.out, "collectivites", "carte", c.Slug, "index.html"),
					struct {
						Layout
						P PageMap
					}{l, c.Page}); err != nil {
					return nil, err
				}
			}
			// L'ancienne adresse /territoires/ : jamais soumise à -only, une
			// redirection ne pèse rien et sa cible peut avoir changé même
			// quand "collectivites" n'est pas demandé.
			l := e.layout
			l.Title = "Page déplacée"
			if err := e.writeAlways(e.page("deplace.gohtml"), filepath.Join(e.out, "territoires", "index.html"),
				struct {
					Layout
					Vers, VersTitle string
				}{l, "/collectivites/", "Collectivités"}); err != nil {
				return nil, err
			}
			for _, c := range territory.Maps {
				l := e.layout
				l.Title = "Page déplacée"
				if err := e.writeAlways(e.page("deplace.gohtml"), filepath.Join(e.out, "territoires", c.Slug, "index.html"),
					struct {
						Layout
						Vers, VersTitle string
					}{l, "/collectivites/carte/" + c.Slug + "/", c.Title}); err != nil {
					return nil, err
				}
			}
			return nil, nil
		}})

	addNode(region, "lieux", []string{"identite", "collectivites-data"},
		func(ctx context.Context, d pipeline.Results) (*Resolver, error) {
			id := dep[identityBundle](d, "identite")
			cb := dep[localGovBundle](d, "collectivites-data")
			places, err := loadResolver(ctx, e.pool, e.root, cb.Col)
			if err != nil {
				return nil, err
			}
			for _, pp := range id.Persons {
				places.locateTerms(pp)
			}
			return places, nil
		})
	addNode(region, "fond-situation", []string{"lieux", "collectivites-data"},
		func(ctx context.Context, d pipeline.Results) (*backgroundSituation, error) {
			places := dep[*Resolver](d, "lieux")
			cb := dep[localGovBundle](d, "collectivites-data")
			linkDept := func(code string) string { return strings.TrimPrefix(places.urlDept(code), e.root+"/") }
			return loadBackgroundSituation(ctx, e.pool, e.out, e.root, cb.Col.FiscalYear, linkDept)
		})

	region.Ajouter(pipeline.Etape{Nom: "communes", Description: "communes et intercommunalités",
		Dependances: []string{"lieux", "fond-situation", "identite", "collectivites-data"},
		Executer: func(ctx context.Context, d pipeline.Results) (any, error) {
			return nil, buildMunicipalitiesAndEPCI(ctx, e, d)
		}})

	addNode(region, "circonscriptions-data", []string{"lieux", "identite", "fond-situation"},
		func(ctx context.Context, d pipeline.Results) (districtsBundle, error) {
			places := dep[*Resolver](d, "lieux")
			id := dep[identityBundle](d, "identite")
			background := dep[*backgroundSituation](d, "fond-situation")
			circos, err := loadDistricts(ctx, e.pool, places, id.Persons)
			if err != nil {
				return districtsBundle{}, err
			}
			if err := background.loadDistricts(ctx, e.pool); err != nil {
				return districtsBundle{}, err
			}
			popFrance := 0
			for _, pc := range circos {
				popFrance += pc.Population
			}
			circosDept := map[string][]Place{}
			tcirco := e.page("circonscription.gohtml")
			for code, pc := range circos {
				pc.Situation = background.forDistrict(pc, popFrance)
				l := e.layout
				l.Title = pc.Title
				if err := e.writeSection("circonscriptions", tcirco, filepath.Join(e.out, "circonscription", code, "index.html"), struct {
					Layout
					C *PageDistrict
				}{l, pc}); err != nil {
					return districtsBundle{}, err
				}
				circosDept[pc.Dept.Code] = append(circosDept[pc.Dept.Code], Place{Type: "CIRCONSCRIPTION",
					Code: code, Name: pc.Ordinal + " circonscription", URL: e.root + "/circonscription/" + code + "/"})
			}
			for _, ls := range circosDept {
				sort.Slice(ls, func(i, j int) bool { return ls[i].Code < ls[j].Code })
			}
			fmt.Printf("  circonscriptions : %d pages\n", len(circos))
			return districtsBundle{circos, popFrance, circosDept}, nil
		})

	region.Ajouter(pipeline.Etape{Nom: "collectivites-pages-locales", Description: "pages région/département",
		Dependances: []string{"collectivites-data", "lieux", "fond-situation", "circonscriptions-data"},
		Executer: func(ctx context.Context, d pipeline.Results) (any, error) {
			cb := dep[localGovBundle](d, "collectivites-data")
			places := dep[*Resolver](d, "lieux")
			background := dep[*backgroundSituation](d, "fond-situation")
			cd := dep[districtsBundle](d, "circonscriptions-data")
			pagesCol, err := pagesAuthorities(ctx, e.pool, cb.Col, places)
			if err != nil {
				return nil, err
			}
			tcol := e.page("collectivite.gohtml")
			for _, pc := range pagesCol {
				if pc.TypeURL == "departement" {
					pc.Situation = background.forDepartment(pc.Code, pc.Name, pc.Lines)
					for _, dcode := range codesCOGOf(pc.Code) {
						pc.Districts = append(pc.Districts, cd.CircosDept[dcode]...)
					}
				} else {
					pc.Situation = background.forRegion(pc.Code, pc.Name, pc.Lines)
				}
				l := e.layout
				l.Title = pc.Name
				if err := e.writeSection("collectivites", tcol, filepath.Join(e.out, "collectivites", pc.TypeURL, pc.Slug, "index.html"),
					struct {
						Layout
						K PageAuthority
					}{l, pc}); err != nil {
					return nil, err
				}
			}
			return nil, nil
		}})

	// --- budget : trois chargements partagés avec sujets-data (sect,
	// circuit) plus un qui lui est propre (bud).
	addNode(region, "budget-data", nil, func(ctx context.Context, _ pipeline.Results) (*StatsBudget, error) {
		return loadBudget(ctx, e.pool)
	})
	addNode(region, "secteurs", nil, func(ctx context.Context, _ pipeline.Results) (*StatsSectors, error) {
		return loadSectors(ctx, e.pool)
	})
	addNode(region, "circuit-canaux", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (*CircuitChannels, error) {
		return loadCircuitChannels(ctx, e.pool, dep[identityBundle](d, "identite").Presidencies)
	})
	region.Ajouter(pipeline.Etape{Nom: "budget", Description: "page budget",
		Dependances: []string{"budget-data", "secteurs", "circuit-canaux"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			bud := dep[*StatsBudget](d, "budget-data")
			section := dep[*StatsSectors](d, "secteurs")
			circuit := dep[*CircuitChannels](d, "circuit-canaux")
			if bud != nil {
				l := e.layout
				l.Title = "Budget de l'État"
				if err := e.writeSection("budget", e.page("budget.gohtml"), filepath.Join(e.out, "budget", "index.html"), struct {
					Layout
					B *StatsBudget
					X *StatsSectors
					K *CircuitChannels
				}{l, bud, section, circuit}); err != nil {
					return nil, err
				}
			}
			if circuit != nil {
				tdisp := e.page("dispositif.gohtml")
				for _, m := range circuit.LargeMeasures {
					l := e.layout
					l.Title = m.Label
					if err := e.writeSection("budget", tdisp, filepath.Join(e.out, "budget", "dispositif", m.Code, "index.html"), struct {
						Layout
						M MeasureExemption
					}{l, m}); err != nil {
						return nil, err
					}
				}
			}
			return nil, nil
		}})
	region.Ajouter(pipeline.Etape{Nom: "social", Description: "page protection sociale", Dependances: []string{"social-data"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			soc := dep[*StatsSocial](d, "social-data")
			if soc == nil || soc.Total <= 0 {
				return nil, nil
			}
			l := e.layout
			l.Title = "Protection sociale"
			return nil, e.writeSection("social", e.page("social.gohtml"), filepath.Join(e.out, "social", "index.html"), struct {
				Layout
				X *StatsSocial
			}{l, soc})
		}})

	// --- qui décide : aucune donnée propre, mais son gabarit lit
	// Cov.Organisations/Cov.Candidats (partis, candidats 2027) — voir
	// layoutWithIdentity.
	region.Ajouter(pipeline.Etape{Nom: "qui-decide", Description: "page qui décide", Dependances: []string{"identite"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			l := e.layoutWithIdentity(dep[identityBundle](d, "identite"))
			l.Title = "Qui décide"
			return nil, e.writeSection("qui-decide", e.page("qui-decide.gohtml"), filepath.Join(e.out, "qui-decide", "index.html"), l)
		}})

	// --- sources et Assemblée nationale : dépendent d'identite pour la
	// liste des députés/groupes/candidats affichée.
	addNode(region, "sources-page", nil, func(ctx context.Context, _ pipeline.Results) ([]SourceDetail, error) {
		return loadSources(ctx, e.pool)
	})
	addNode(region, "sources-stats", nil, func(ctx context.Context, _ pipeline.Results) (*StatsGlobalSources, error) {
		return loadStatsGlobalSources(ctx, e.pool)
	})
	region.Ajouter(pipeline.Etape{Nom: "sources", Description: "page sources",
		Dependances: []string{"sources-page", "sources-stats"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			l := e.layout
			l.Title = "Sources"
			return nil, e.writeSection("sources", e.page("sources.gohtml"), filepath.Join(e.out, "sources", "index.html"), struct {
				Layout
				Flow  []SourceDetail
				Stats *StatsGlobalSources
			}{l, dep[[]SourceDetail](d, "sources-page"), dep[*StatsGlobalSources](d, "sources-stats")})
		}})
	region.Ajouter(pipeline.Etape{Nom: "candidats", Description: "page candidats", Dependances: []string{"identite"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			id := dep[identityBundle](d, "identite")
			l := e.layout
			l.Title = "Candidats 2027"
			return nil, e.writeSection("candidats", e.page("candidats.gohtml"), filepath.Join(e.out, "candidats", "index.html"), struct {
				Layout
				Candidates []*Candidate
			}{l, id.Candidates})
		}})
	region.Ajouter(pipeline.Etape{Nom: "partis", Description: "page partis", Dependances: []string{"identite"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			id := dep[identityBundle](d, "identite")
			l := e.layout
			l.Title = "Partis"
			return nil, e.writeSection("partis", e.page("partis.gohtml"), filepath.Join(e.out, "partis", "index.html"), struct {
				Layout
				Orgs []*Organization
			}{l, id.OrgList})
		}})
	region.Ajouter(pipeline.Etape{Nom: "assemblee", Description: "page Assemblée nationale",
		Dependances: []string{"identite", "derniers-scrutins"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			id := dep[identityBundle](d, "identite")
			last := dep[[]Vote](d, "derniers-scrutins")
			l := e.layout
			l.Title = "Assemblée nationale"
			return nil, e.writeSection("assemblee", e.page("assemblee.gohtml"), filepath.Join(e.out, "assemblee", "index.html"), struct {
				Layout
				Groups    []*Group
				Deputies  []*Person
				Elections []Vote
			}{l, id.GrpList, id.Deputies, last})
		}})

	// --- la trentaine de schémas propres à sujets-data (voir la note de
	// tête de fichier : sujets-data en dépend TOUS, sans sous-ensemble
	// déduit à l'avance).
	type htmlLoader = func(context.Context, *pgxpool.Pool) (template.HTML, error)
	for name, fn := range map[string]htmlLoader{
		"taux-pauvrete-serie":        loadRatePovertySeries,
		"depense-environnementale":   loadExpenseEnvironmental,
		"emploi-total-salarie":       loadEmploymentTotalEmployee,
		"depenses-fiscales-tendance": loadTrendExpensesFiscal,
		"ports-francais":             loadPortsFrench,
		"ports-europe":               loadPortsEurope,
		"report-modal-port":          loadReportModal,
		"report-modal-conteneurs":    loadReportModalContainers,
		"carte-semi-conducteurs":     loadMapSemiconductors,
		"ecart-prix-dom":             loadGapPriceDOM,
		"alimentaire-dom":            loadFoodDOM,
		"sipri":                      loadSIPRI,
		"historique-immigration":     loadHistoryImmigration,
		"age-depart-retraite":        loadAgeStartPension,
		"taux-remplacement":          loadRateReplacement,
	} {
		fn := fn
		addNode(region, name, nil, func(ctx context.Context, _ pipeline.Results) (template.HTML, error) { return fn(ctx, e.pool) })
	}
	addNode(region, "contributif-non-contributif", nil,
		func(_ context.Context, _ pipeline.Results) (template.HTML, error) {
			return drawContributoryNonContributory(), nil
		})
	addNode(region, "seuils-pauvrete", nil, func(ctx context.Context, _ pipeline.Results) (*ThresholdsPoverty, error) {
		return loadThresholdsPoverty(ctx, e.pool)
	})
	addNode(region, "climat-international", nil, func(ctx context.Context, _ pipeline.Results) (*StatsClimateInternational, error) {
		return loadClimateInternational(ctx, e.pool)
	})
	addNode(region, "union-europeenne", nil, func(ctx context.Context, _ pipeline.Results) (*StatsUnionEuropean, error) {
		return loadUnionEuropean(ctx, e.pool)
	})
	addNode(region, "carte-bassins", nil, func(ctx context.Context, _ pipeline.Results) (*MapBasins, error) {
		return loadMapBasins(ctx, e.pool)
	})
	addNode(region, "carte-eptb-epage", nil, func(ctx context.Context, _ pipeline.Results) (*MapEPTBEPAGE, error) {
		return loadMapEPTBEPAGE(ctx, e.pool)
	})
	addNode(region, "depenses-fiscales-stats", nil, func(ctx context.Context, _ pipeline.Results) (*StatsExpensesFiscal, error) {
		return loadStatsExpensesFiscal(ctx, e.pool)
	})
	addNode(region, "carte-ifi", nil, func(ctx context.Context, _ pipeline.Results) (*MapIFI, error) {
		return loadMapIFI(ctx, e.pool)
	})
	addNode(region, "appareil-productif", nil, func(ctx context.Context, _ pipeline.Results) (*StatsApparatusProductive, error) {
		return loadApparatusProductive(ctx, e.pool)
	})
	addNode(region, "francophonie", nil, func(ctx context.Context, _ pipeline.Results) (*StatsFrancophonie, error) {
		return loadFrancophonie(ctx, e.pool)
	})
	addNode(region, "empire-colonial", nil, func(ctx context.Context, _ pipeline.Results) (*StatsEmpireColonial, error) {
		return loadEmpireColonial(ctx, e.pool)
	})
	addNode(region, "seconde-guerre-mondiale", nil, func(ctx context.Context, _ pipeline.Results) (*StatsSecondWorldWar, error) {
		return loadSecondWorldWar(ctx, e.pool)
	})
	addNode(region, "population-guerres", nil, func(ctx context.Context, _ pipeline.Results) (*PopulationWars, error) {
		return loadPopulationWars(ctx, e.pool)
	})
	addNode(region, "controle-fiscal", nil, func(ctx context.Context, _ pipeline.Results) (*ControlFiscal, error) {
		return loadControlFiscal(ctx, e.pool)
	})
	addNode(region, "carte-infrastructure-ports", nil, func(ctx context.Context, _ pipeline.Results) (*MapInfrastructurePorts, error) {
		return loadMapInfrastructurePorts(ctx, e.pool)
	})
	addNode(region, "justice", nil, func(ctx context.Context, _ pipeline.Results) (*StatsJustice, error) {
		return loadJustice(ctx, e.pool)
	})
	addNode(region, "carte-musees", nil, func(ctx context.Context, _ pipeline.Results) (*MapMuseums, error) {
		return loadMapMuseums(ctx, e.pool)
	})
	addNode(region, "carte-etudiants", nil, func(ctx context.Context, _ pipeline.Results) (*MapStudents, error) {
		return loadMapStudents(ctx, e.pool)
	})
	addNode(region, "carte-sru", nil, func(ctx context.Context, _ pipeline.Results) (*MapSRU, error) {
		return loadMapSRU(ctx, e.pool)
	})
	addNode(region, "sru-prelevement", nil, func(ctx context.Context, _ pipeline.Results) (*LevySRU, error) {
		return loadLevySRU(ctx, e.pool)
	})
	addNode(region, "effort-recherche", nil, func(ctx context.Context, _ pipeline.Results) (*EffortSearch, error) {
		return loadEffortSearch(ctx, e.pool)
	})
	addNode(region, "indochine-partition", nil, func(ctx context.Context, _ pipeline.Results) (*StatsIndochinaPartition, error) {
		return loadIndochinaPartition(ctx, e.pool)
	})
	addNode(region, "medecins-evolution", nil, func(ctx context.Context, _ pipeline.Results) (template.HTML, error) {
		return loadSeriesDoctors(ctx, e.pool)
	})

	// --- sujets-data : voir la note de tête de fichier.
	topicsDeps := []string{"docs", "identite", "accueil-data", "territoires",
		"secteurs", "circuit-canaux", "seuils-pauvrete", "taux-pauvrete-serie", "depense-environnementale",
		"emploi-total-salarie", "climat-international", "union-europeenne", "carte-bassins", "carte-eptb-epage",
		"depenses-fiscales-tendance", "depenses-fiscales-stats", "carte-ifi", "appareil-productif", "francophonie",
		"empire-colonial", "seconde-guerre-mondiale", "population-guerres", "controle-fiscal", "ports-francais",
		"ports-europe", "carte-infrastructure-ports", "report-modal-port", "report-modal-conteneurs", "justice",
		"carte-semi-conducteurs", "contributif-non-contributif", "carte-musees", "ecart-prix-dom", "alimentaire-dom",
		"carte-etudiants", "carte-sru", "sru-prelevement", "effort-recherche", "sipri", "historique-immigration",
		"age-depart-retraite", "taux-remplacement", "indochine-partition", "medecins-evolution",
	}
	addNode(region, "sujets-data", topicsDeps, func(ctx context.Context, d pipeline.Results) (topicsReady, error) {
		return topicsReady{}, buildTopicsData(ctx, e, d)
	})

	// identite en dépendance directe (pas seulement via sujets-data, qui
	// l'inclut déjà transitivement) : Executer ne remonte que les
	// dépendances DIRECTES d'un nœud dans deps, jamais toute la fermeture —
	// accueil.gohtml lit Cov.Candidats/CandidatsPrimaire/CandidatsAvecBilan
	// (voir layoutWithIdentity), qu'il faut donc nommer ici aussi.
	region.Ajouter(pipeline.Etape{Nom: "accueil", Description: "page d'accueil",
		Dependances: []string{"accueil-data", "sujets-data", "identite"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			acc := dep[*DataHome](d, "accueil-data")
			l := e.layoutWithIdentity(dep[identityBundle](d, "identite"))
			l.Hero = true
			l.HeroTitle = "Le budget réel de la France, sujet par sujet de la campagne 2027"
			l.HeroLede = "Retraites, santé, école, sécurité, immigration, dette : ce que l'État dépense, " +
				"d'où vient l'argent, qui décide et qui contrôle — sourcé document par document."
			l.HeroVisual = heroThousand(acc, e.root)
			l.Title = "Le budget réel de la France, sujet par sujet de la campagne 2027"
			return nil, e.writeSection("accueil", e.page("accueil.gohtml"), filepath.Join(e.out, "index.html"), struct {
				Layout
				A *DataHome
			}{l, acc})
		}})
	region.Ajouter(pipeline.Etape{Nom: "sujets", Description: "index des sujets",
		Dependances: []string{"accueil-data", "sujets-data"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			acc := dep[*DataHome](d, "accueil-data")
			l := e.layout
			l.Title = "Sujets de campagne"
			return nil, e.writeSection("sujets", e.page("sujets.gohtml"), filepath.Join(e.out, "sujets", "index.html"), struct {
				Layout
				Families []*Family
				Year     int
			}{l, acc.Families, acc.Year})
		}})
	region.Ajouter(pipeline.Etape{Nom: "argent-public", Description: "argent public",
		Dependances: []string{"accueil-data", "fonctions", "sujets-data"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			acc := dep[*DataHome](d, "accueil-data")
			pagesFunctions := dep[map[string]*PageFunction](d, "fonctions")
			tf := e.page("fonction.gohtml")
			for _, f := range acc.Functions {
				pf := pagesFunctions[f.Code]
				l := e.layout
				l.Title = pf.Name
				if err := e.writeSection("argent-public", tf, filepath.Join(e.out, "fonction", f.Slug, "index.html"), struct {
					Layout
					F *PageFunction
				}{l, pf}); err != nil {
					return nil, err
				}
			}
			l := e.layout
			l.Title = "Argent public"
			return nil, e.writeSection("argent-public", e.page("argent-public.gohtml"), filepath.Join(e.out, "argent-public", "index.html"),
				struct {
					Layout
					A *DataHome
				}{l, acc})
		}})
	region.Ajouter(pipeline.Etape{Nom: "comprendre", Description: "documents de méthode et sujets",
		Dependances: []string{"sujets-data", "docs"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			return nil, buildGuidesAndTopics(e, dep[[]*Doc](d, "docs"))
		}})

	// --- fiches : personnes, candidats, organisations, référentiels, groupes.
	addNode(region, "mandats-locaux", []string{"lieux"},
		func(ctx context.Context, d pipeline.Results) (map[string]*ReconciliationRNE, error) {
			premises, err := loadTermsPremises(ctx, e.pool, e.dataDir+"/candidats-mandats-locaux.csv")
			if err != nil {
				return nil, err
			}
			locateTermsPremises(premises, dep[*Resolver](d, "lieux"))
			return premises, nil
		})
	addNode(region, "votes-bulk", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (bool, error) {
		id := dep[identityBundle](d, "identite")
		if err := loadVotesBulk(ctx, e.pool, id.Persons, 60); err != nil {
			return false, err
		}
		return true, nil
	})
	region.Ajouter(pipeline.Etape{Nom: "fiches", Description: "fiches personnes/candidats/organisations",
		Dependances: []string{"identite", "election2027", "mandats-locaux", "votes-bulk"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			return nil, buildProfilePages(e, d)
		}})

	// --- recherche : écrit en dernier dans le code d'origine (référence
	// thèmes, docs et sénateurs déjà chargés) — désormais sa propre section
	// -only comme les autres, PAS toujours écrite quel que soit -only : une
	// construction partielle sert déjà, documenté ailleurs, à l'itération
	// locale, jamais à la publication (voir Run) — un index de recherche
	// stale sur un chantier partiel n'aggrave pas ce que -only assume déjà.
	region.Ajouter(pipeline.Etape{Nom: "recherche", Description: "index de recherche",
		Dependances: []string{"identite", "themes", "sujets-data", "docs", "senat"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			id := dep[identityBundle](d, "identite")
			themes := dep[*StatsThemes](d, "themes")
			docs := dep[[]*Doc](d, "docs")
			senate := dep[*StatsSenate](d, "senat")
			senators := map[string]bool{}
			for _, p := range senate.Senators2 {
				senators[p.Slug] = true
			}
			return nil, writeIndex(e.out, id.Persons, id.Candidates, id.Orgs, id.Groups, id.Refs,
				themes.Themes, docs, senators)
		}})

	// --- scrutin : garde son court-circuit par empreinte, verbatim.
	region.Ajouter(pipeline.Etape{Nom: "scrutin", Description: "pages de scrutin", Dependances: []string{"seuils"},
		Executer: func(ctx context.Context, d pipeline.Results) (any, error) {
			thresholds := dep[map[string]Threshold](d, "seuils")
			return buildBallotSection(ctx, e, thresholds)
		}})

	return region
}
