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
func dep[T any](deps pipeline.Results, nom string) T {
	v, _ := deps[nom].(T)
	return v
}

// addNode enregistre un nœud dont la valeur produite a un type précis —
// tout le graphe passe par cet unique point pour éviter de répéter, à
// chaque nœud, la conversion vers Results (map[string]any) qu'Executer
// exige.
func addNode[T any](reg *pipeline.Registre, nom string, deps []string,
	fn func(ctx context.Context, d pipeline.Results) (T, error)) {
	reg.Ajouter(pipeline.Etape{
		Nom: nom, Description: "loading " + nom, Dependances: deps,
		Executer: func(ctx context.Context, d pipeline.Results) (any, error) { return fn(ctx, d) },
	})
}

// identityBundle : voir la note en tête de fichier — un seul nœud pour ce
// que loadPersons, loadCandidats, loadReferentiels, loadOrganisations,
// loadGroupes, loadPresidences, loadCoalitions et loadMedias produisaient
// et s'entre-annotaient, dans le même ordre qu'avant.
type identityBundle struct {
	Persons     map[string]*Person
	Candidats   []*Candidat
	Refs        map[string]*Referentiel
	Tags        map[string]Tag
	Orgs        map[string]*Organisation
	Groupes     map[string]*Groupe
	Presidences []Presidence
	Deputes     []*Person
	OrgList     []*Organisation
	GrpList     []*Groupe
	AvecFiche   map[string]bool
	// Cov porte les décomptes que seul ce nœud connaît (candidats, avec
	// bilan, primaire, organisations) — fusionnés dans Layout par
	// buildRegistry avant que la moindre page ne parte à l'écriture.
	Cov Coverage
}

type localGovBundle struct {
	Col      *StatsCollectivites
	TableDep TableDepenses
	Parts    []PartRecette
}

type districtsBundle struct {
	Circos     map[string]*PageCirco
	PopFrance  int
	CircosDept map[string][]Lieu
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
	maxScrutins        int
	previousCacheValue manifesteCache
	currentSite        string
	newCache           *manifesteCache
}

// previousCache : accesseur pour graphe_sections.go — un simple champ
// suffirait, mais nommer l'accès rend explicite qu'il ne s'agit jamais de
// newCache (celui-ci muté par les nœuds, celui-là jamais).
func (e *environment) previousCache() manifesteCache { return e.previousCacheValue }

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
	l.Cov.Candidats = id.Cov.Candidats
	l.Cov.CandidatsAvecBilan = id.Cov.CandidatsAvecBilan
	l.Cov.CandidatsPrimaire = id.Cov.CandidatsPrimaire
	l.Cov.Organisations = id.Cov.Organisations
	return l
}

func buildRegistry(env *environment) *pipeline.Registre {
	reg := pipeline.NouveauRegistre(nil)
	e := env
	tcd := e.page("carte-detail.gohtml")

	// --- identité : voir la note de tête de fichier.
	addNode(reg, "identite", nil, func(ctx context.Context, _ pipeline.Results) (identityBundle, error) {
		persons, err := loadPersons(ctx, e.pool, e.layout.Cov.Scrutins)
		if err != nil {
			return identityBundle{}, err
		}
		candidats, err := loadCandidats(filepath.Join(e.dataDir, "candidats.csv"), persons)
		if err != nil {
			return identityBundle{}, err
		}
		refs, tags, err := loadReferentiels(filepath.Join(e.dataDir, "referentiels.csv"))
		if err != nil {
			return identityBundle{}, err
		}
		orgs, err := loadOrganisations(ctx, e.pool, filepath.Join(e.dataDir, "organisations.csv"), tags)
		if err != nil {
			return identityBundle{}, err
		}
		groupes, err := loadGroupes(ctx, e.pool)
		if err != nil {
			return identityBundle{}, err
		}
		presidences, err := loadPresidences(filepath.Join(e.dataDir, "presidents.csv"))
		if err != nil {
			return identityBundle{}, err
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
		coalitions, err := loadCoalitions(filepath.Join(e.dataDir, "coalitions.csv"), orgs)
		if err != nil {
			return identityBundle{}, err
		}
		for _, g := range groupes {
			for _, c := range coalitions {
				if strings.Contains(strings.ToLower(g.Nom), strings.ToLower(c.Libelle)) {
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
		var cov Coverage
		cov.Candidats = len(candidats)
		for _, c := range candidats {
			if c.Person != nil && c.Person.HasVotes {
				cov.CandidatsAvecBilan++
			}
			if c.Statut == "PRIMAIRE" {
				cov.CandidatsPrimaire++
			}
		}
		cov.Organisations = len(orgs)

		deputes := make([]*Person, 0, len(persons))
		for _, p := range persons {
			deputes = append(deputes, p)
		}
		trierPersonnes(deputes)
		orgList := make([]*Organisation, 0, len(orgs))
		for _, o := range orgs {
			orgList = append(orgList, o)
		}
		sort.Slice(orgList, func(i, j int) bool { return CleTri(orgList[i].Libelle) < CleTri(orgList[j].Libelle) })
		grpList := make([]*Groupe, 0, len(groupes))
		for _, g := range groupes {
			grpList = append(grpList, g)
		}
		sort.Slice(grpList, func(i, j int) bool { return CleTri(grpList[i].Nom) < CleTri(grpList[j].Nom) })
		avecFiche := map[string]bool{}
		for _, pp := range persons {
			avecFiche[pp.Slug] = true
		}
		return identityBundle{persons, candidats, refs, tags, orgs, groupes, presidences,
			deputes, orgList, grpList, avecFiche, cov}, nil
	})

	addNode(reg, "territoires", nil, func(ctx context.Context, _ pipeline.Results) (*StatsTerritoires, error) {
		return loadTerritoires(ctx, e.pool)
	})
	addNode(reg, "derniers-scrutins", nil, func(ctx context.Context, _ pipeline.Results) ([]Vote, error) {
		return derniersScrutins(ctx, e.pool, 60)
	})
	addNode(reg, "seuils", nil, func(_ context.Context, _ pipeline.Results) (map[string]Seuil, error) {
		return loadSeuils(filepath.Join(e.dataDir, "seuils.csv"))
	})
	addNode(reg, "docs", nil, func(_ context.Context, _ pipeline.Results) ([]*Doc, error) {
		return loadDocs("docs")
	})

	// --- pages simples : un chargement, une écriture, jamais relu ailleurs.
	// Regroupées ici plutôt que dispersées dans l'ordre du fichier d'origine
	// (perdu de toute façon : Niveaux ordonne par dépendance, pas par
	// déclaration) — chacune une ligne, le même patron partout.
	addPageNode(reg, e, "frise", nil, "La Ve République en chiffres", "frise.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsFrise, error) {
			return loadFrise(ctx, e.pool, e.dataDir)
		},
		func(l Layout, fr *StatsFrise) (string, any) {
			return filepath.Join(e.out, "frise", "index.html"), struct {
				Layout
				F *StatsFrise
			}{l, fr}
		})
	addPageNode(reg, e, "dette", nil, "La dette publique", "dette.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsDette, error) { return loadDette(ctx, e.pool) },
		func(l Layout, d *StatsDette) (string, any) {
			return filepath.Join(e.out, "dette", "index.html"), struct {
				Layout
				D *StatsDette
			}{l, d}
		})
	addPageNode(reg, e, "chomage", nil, "Le taux de chômage", "chomage.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsChomage, error) {
			return loadChomage(ctx, e.pool)
		},
		func(l Layout, c *StatsChomage) (string, any) {
			return filepath.Join(e.out, "chomage", "index.html"), struct {
				Layout
				C *StatsChomage
			}{l, c}
		})
	addNode(reg, "richesse-data", nil, func(ctx context.Context, _ pipeline.Results) (*StatsRichesse, error) {
		return loadRichesse(ctx, e.pool)
	})
	addPageNode(reg, e, "richesse", []string{"richesse-data"}, "La répartition de la richesse en France", "richesse.gohtml",
		func(_ context.Context, d pipeline.Results) (*StatsRichesse, error) {
			return dep[*StatsRichesse](d, "richesse-data"), nil
		},
		func(l Layout, r *StatsRichesse) (string, any) {
			return filepath.Join(e.out, "richesse", "index.html"), struct {
				Layout
				R *StatsRichesse
			}{l, r}
		})
	addPageNode(reg, e, "dividendes", nil, "Les dividendes versés", "dividendes.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsDividendes, error) {
			return loadDividendes(ctx, e.pool)
		},
		func(l Layout, d *StatsDividendes) (string, any) {
			return filepath.Join(e.out, "dividendes", "index.html"), struct {
				Layout
				D *StatsDividendes
			}{l, d}
		})
	addNode(reg, "agriculture-data", nil, func(ctx context.Context, _ pipeline.Results) (*StatsAgri, error) {
		return loadAgriculture(ctx, e.pool, e.dataDir)
	})
	addPageNode(reg, e, "agriculture", []string{"agriculture-data"}, "Agriculture et alimentation", "agriculture.gohtml",
		func(_ context.Context, d pipeline.Results) (*StatsAgri, error) {
			return dep[*StatsAgri](d, "agriculture-data"), nil
		},
		func(l Layout, a *StatsAgri) (string, any) {
			return filepath.Join(e.out, "agriculture", "index.html"), struct {
				Layout
				A *StatsAgri
			}{l, a}
		})
	addNode(reg, "social-data", nil, func(ctx context.Context, _ pipeline.Results) (*StatsSocial, error) {
		return loadSocial(ctx, e.pool)
	})

	addNode(reg, "europe", nil, func(ctx context.Context, _ pipeline.Results) (*StatsEurope, error) {
		return loadEurope(ctx, e.pool)
	})
	reg.Ajouter(pipeline.Etape{Nom: "europe-page", Description: "page Parlement européen", Dependances: []string{"europe"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			europe := dep[*StatsEurope](d, "europe")
			l := e.layout
			l.Title = "Parlement européen"
			return nil, e.writeSection("europe", e.page("europe.gohtml"), filepath.Join(e.out, "europe", "index.html"), struct {
				Layout
				E *StatsEurope
			}{l, europe})
		}})

	addNode(reg, "themes", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (*StatsThemes, error) {
		id := dep[identityBundle](d, "identite")
		var candSlugs []string
		orgParSlug := map[string]string{}
		for _, c := range id.Candidats {
			if c.Person != nil {
				candSlugs = append(candSlugs, c.Person.Slug)
				orgParSlug[c.Person.Slug] = c.Organisation
			}
		}
		return loadThemes(ctx, e.pool, 60, candSlugs, orgParSlug)
	})
	reg.Ajouter(pipeline.Etape{Nom: "themes-page", Description: "page thèmes", Dependances: []string{"themes"},
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
				for _, g := range th.Groupes {
					if g.Total > maxG {
						maxG = g.Total
					}
				}
				l := e.layout
				l.Title = th.Label
				if err := e.writeSection("themes", tth, filepath.Join(e.out, "theme", th.Slug, "index.html"), struct {
					Layout
					Th        *Theme
					MaxGroupe int
				}{l, th, maxG}); err != nil {
					return nil, err
				}
			}
			return nil, nil
		}})

	addNode(reg, "senat", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (*StatsSenat, error) {
		return loadSenat(ctx, e.pool, dep[identityBundle](d, "identite").Persons)
	})
	reg.Ajouter(pipeline.Etape{Nom: "senat-page", Description: "page Sénat", Dependances: []string{"senat"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			senat := dep[*StatsSenat](d, "senat")
			l := e.layout
			l.Title = "Sénat"
			return nil, e.writeSection("senat", e.page("senat.gohtml"), filepath.Join(e.out, "senat", "index.html"), struct {
				Layout
				Se *StatsSenat
			}{l, senat})
		}})

	addPageNode(reg, e, "vieillesse", []string{"territoires"}, "La vieillesse : combien, qui paie, et la dépendance", "vieillesse.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsVieillesse, error) {
			return loadVieillesse(ctx, e.pool)
		},
		func(l Layout, v *StatsVieillesse) (string, any) {
			return filepath.Join(e.out, "vieillesse", "index.html"), struct {
				Layout
				V *StatsVieillesse
			}{l, v}
		})
	reg.Ajouter(pipeline.Etape{Nom: "vieillesse-carte", Description: "carte vieillesse", Dependances: []string{"vieillesse"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			vieil := dep[*StatsVieillesse](d, "vieillesse")
			if vieil.CarteAPA.Slug == "" {
				return nil, nil
			}
			l := e.layout
			l.Title = vieil.CarteAPA.Titre
			l.Description = vieil.CarteAPA.Question
			imageCarte(&l, e.out, "vieillesse-"+vieil.CarteAPA.Slug, vieil.CarteAPA.Page.Carte.SVG)
			return nil, e.writeSection("vieillesse", tcd, filepath.Join(e.out, "vieillesse", "carte", vieil.CarteAPA.Slug, "index.html"),
				struct {
					Layout
					P PageCarte
				}{l, vieil.CarteAPA.Page})
		}})

	addPageNode(reg, e, "jeunesse", nil, "La jeunesse : études supérieures, apprentissage, premiers emplois", "jeunesse.gohtml",
		func(ctx context.Context, _ pipeline.Results) (*StatsJeunesse, error) {
			return loadJeunesse(ctx, e.pool)
		},
		func(l Layout, j *StatsJeunesse) (string, any) {
			return filepath.Join(e.out, "jeunesse", "index.html"), struct {
				Layout
				J *StatsJeunesse
			}{l, j}
		})
	reg.Ajouter(pipeline.Etape{Nom: "jeunesse-carte", Description: "carte jeunesse", Dependances: []string{"jeunesse"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			jeun := dep[*StatsJeunesse](d, "jeunesse")
			if jeun.CarteInsertion.Slug == "" {
				return nil, nil
			}
			l := e.layout
			l.Title = jeun.CarteInsertion.Titre
			l.Description = jeun.CarteInsertion.Question
			imageCarte(&l, e.out, "jeunesse-"+jeun.CarteInsertion.Slug, jeun.CarteInsertion.Page.Carte.SVG)
			return nil, e.writeSection("jeunesse", tcd, filepath.Join(e.out, "jeunesse", "carte", jeun.CarteInsertion.Slug, "index.html"),
				struct {
					Layout
					P PageCarte
				}{l, jeun.CarteInsertion.Page})
		}})

	addNode(reg, "securite", nil, func(ctx context.Context, _ pipeline.Results) (*StatsSecurite, error) {
		return loadSecurite(ctx, e.pool)
	})
	reg.Ajouter(pipeline.Etape{Nom: "securite-page", Description: "page sécurité", Dependances: []string{"securite"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			sec := dep[*StatsSecurite](d, "securite")
			l := e.layout
			l.Title = "Sécurité"
			if len(sec.Indicateurs) > 0 {
				imageCarte(&l, e.out, "securite", sec.Indicateurs[0].Page.Carte.SVG)
			}
			if err := e.writeSection("securite", e.page("securite.gohtml"), filepath.Join(e.out, "securite", "index.html"), struct {
				Layout
				S *StatsSecurite
			}{l, sec}); err != nil {
				return nil, err
			}
			for _, ind := range sec.Indicateurs {
				l := e.layout
				l.Title = ind.Libelle
				l.Description = ind.Page.Question
				imageCarte(&l, e.out, "securite-"+ind.Slug, ind.Page.Carte.SVG)
				if err := e.writeSection("securite", tcd, filepath.Join(e.out, "securite", ind.Slug, "index.html"), struct {
					Layout
					P PageCarte
				}{l, ind.Page}); err != nil {
					return nil, err
				}
			}
			return nil, nil
		}})

	addNode(reg, "accueil-data", []string{"territoires", "securite"},
		func(ctx context.Context, d pipeline.Results) (*DonneesAccueil, error) {
			return loadAccueil(ctx, e.pool, dep[*StatsTerritoires](d, "territoires"), dep[*StatsSecurite](d, "securite"))
		})
	addNode(reg, "fonctions", []string{"accueil-data"},
		func(ctx context.Context, d pipeline.Results) (map[string]*PageFonction, error) {
			return chargerFonctions(ctx, e.pool, dep[*DonneesAccueil](d, "accueil-data"))
		})

	addNode(reg, "election2027", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (*Stats2027, error) {
		return load2027(ctx, e.pool, dep[identityBundle](d, "identite").Candidats, e.dataDir)
	})
	reg.Ajouter(pipeline.Etape{Nom: "election2027-page", Description: "page présidentielle 2027", Dependances: []string{"election2027"},
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
			vues := map[string]bool{}
			for _, k := range e27.Candidats {
				if k.Page == nil || vues[k.Page.Slug] {
					continue
				}
				vues[k.Page.Slug] = true
				l := e.layout
				l.Title = k.Page.Titre
				l.Description = k.Page.Question
				imageCarte(&l, e.out, "2027-"+k.Page.Slug, k.Page.Carte.SVG)
				if err := e.writeSection("election2027", tcd, filepath.Join(e.out, "2027", k.Page.Slug, "index.html"), struct {
					Layout
					P PageCarte
				}{l, *k.Page}); err != nil {
					return nil, err
				}
			}
			return nil, nil
		}})

	addNode(reg, "gouvernement", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (*StatsGouvernement, error) {
		return loadGouvernement(ctx, e.pool, e.dataDir, dep[identityBundle](d, "identite").AvecFiche)
	})
	reg.Ajouter(pipeline.Etape{Nom: "gouvernement-page", Description: "page gouvernement", Dependances: []string{"gouvernement"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			gouv := dep[*StatsGouvernement](d, "gouvernement")
			l := e.layout
			l.Title = "Gouvernement"
			if err := e.writeSection("gouvernement", e.page("gouvernement.gohtml"), filepath.Join(e.out, "gouvernement", "index.html"),
				struct {
					Layout
					G *StatsGouvernement
				}{l, gouv}); err != nil {
				return nil, err
			}
			l = e.layout
			l.Title = "Tous les décrets de composition"
			tousDecrets := *gouv
			tousDecrets.Decrets = gouv.Tous
			return nil, e.writeSection("gouvernement", e.page("decrets.gohtml"), filepath.Join(e.out, "gouvernement", "decrets", "index.html"),
				struct {
					Layout
					G *StatsGouvernement
				}{l, &tousDecrets})
		}})

	// --- collectivités, lieux, communes/EPCI, circonscriptions : une vraie
	// chaîne de dépendances, contrairement à identite/sujets-data — chacune
	// se sépare proprement de la suivante.
	addNode(reg, "collectivites-data", []string{"identite"},
		func(ctx context.Context, d pipeline.Results) (localGovBundle, error) {
			id := dep[identityBundle](d, "identite")
			col, err := loadCollectivites(ctx, e.pool)
			if err != nil {
				return localGovBundle{}, err
			}
			relierFiches(col, id.AvecFiche)
			if err := cartesCollectivites(ctx, e.pool, col, indicPopulation); err != nil {
				return localGovBundle{}, err
			}
			return localGovBundle{col, tableDepenses(col.Poids, indicsCollectivite), partsRecettes(col.Poids)}, nil
		})
	reg.Ajouter(pipeline.Etape{Nom: "collectivites-page", Description: "page collectivités",
		Dependances: []string{"collectivites-data", "territoires"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			cb := dep[localGovBundle](d, "collectivites-data")
			terr := dep[*StatsTerritoires](d, "territoires")
			l := e.layout
			l.Title = "Collectivités"
			imageCarte(&l, e.out, "collectivites", cb.Col.CarteDepts.SVG)
			return nil, e.writeSection("collectivites", e.page("collectivites.gohtml"), filepath.Join(e.out, "collectivites", "index.html"),
				struct {
					Layout
					C            *StatsCollectivites
					T            *StatsTerritoires
					Ind          []IndicCollectivite
					TableDep     TableDepenses
					Parts        []PartRecette
					IndicLibelle string
				}{l, cb.Col, terr, indicsCollectivite, cb.TableDep, cb.Parts, "Population"})
		}})
	reg.Ajouter(pipeline.Etape{Nom: "collectivites-cartes", Description: "cartes départementales",
		Dependances: []string{"territoires"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			terr := dep[*StatsTerritoires](d, "territoires")
			for _, c := range terr.Cartes {
				l := e.layout
				l.Title = c.Titre
				l.Description = c.Page.Question
				imageCarte(&l, e.out, "carte-"+c.Slug, c.Page.Carte.SVG)
				if err := e.writeSection("collectivites", tcd, filepath.Join(e.out, "collectivites", "carte", c.Slug, "index.html"),
					struct {
						Layout
						P PageCarte
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
					Vers, VersTitre string
				}{l, "/collectivites/", "Collectivités"}); err != nil {
				return nil, err
			}
			for _, c := range terr.Cartes {
				l := e.layout
				l.Title = "Page déplacée"
				if err := e.writeAlways(e.page("deplace.gohtml"), filepath.Join(e.out, "territoires", c.Slug, "index.html"),
					struct {
						Layout
						Vers, VersTitre string
					}{l, "/collectivites/carte/" + c.Slug + "/", c.Titre}); err != nil {
					return nil, err
				}
			}
			return nil, nil
		}})

	addNode(reg, "lieux", []string{"identite", "collectivites-data"},
		func(ctx context.Context, d pipeline.Results) (*Resolveur, error) {
			id := dep[identityBundle](d, "identite")
			cb := dep[localGovBundle](d, "collectivites-data")
			lieux, err := chargerResolveur(ctx, e.pool, e.root, cb.Col)
			if err != nil {
				return nil, err
			}
			for _, pp := range id.Persons {
				lieux.situerMandats(pp)
			}
			return lieux, nil
		})
	addNode(reg, "fond-situation", []string{"lieux", "collectivites-data"},
		func(ctx context.Context, d pipeline.Results) (*fondSituation, error) {
			lieux := dep[*Resolveur](d, "lieux")
			cb := dep[localGovBundle](d, "collectivites-data")
			lienDept := func(code string) string { return strings.TrimPrefix(lieux.urlDept(code), e.root+"/") }
			return chargerFondSituation(ctx, e.pool, e.out, e.root, cb.Col.Exercice, lienDept)
		})

	reg.Ajouter(pipeline.Etape{Nom: "communes", Description: "communes et intercommunalités",
		Dependances: []string{"lieux", "fond-situation", "identite", "collectivites-data"},
		Executer: func(ctx context.Context, d pipeline.Results) (any, error) {
			return nil, buildCommunesAndEPCI(ctx, e, d)
		}})

	addNode(reg, "circonscriptions-data", []string{"lieux", "identite", "fond-situation"},
		func(ctx context.Context, d pipeline.Results) (districtsBundle, error) {
			lieux := dep[*Resolveur](d, "lieux")
			id := dep[identityBundle](d, "identite")
			fond := dep[*fondSituation](d, "fond-situation")
			circos, err := chargerCirconscriptions(ctx, e.pool, lieux, id.Persons)
			if err != nil {
				return districtsBundle{}, err
			}
			if err := fond.chargerCirconscriptions(ctx, e.pool); err != nil {
				return districtsBundle{}, err
			}
			popFrance := 0
			for _, pc := range circos {
				popFrance += pc.Population
			}
			circosDept := map[string][]Lieu{}
			tcirco := e.page("circonscription.gohtml")
			for code, pc := range circos {
				pc.Situation = fond.pourCirconscription(pc, popFrance)
				l := e.layout
				l.Title = pc.Titre
				if err := e.writeSection("circonscriptions", tcirco, filepath.Join(e.out, "circonscription", code, "index.html"), struct {
					Layout
					C *PageCirco
				}{l, pc}); err != nil {
					return districtsBundle{}, err
				}
				circosDept[pc.Dept.Code] = append(circosDept[pc.Dept.Code], Lieu{Type: "CIRCONSCRIPTION",
					Code: code, Nom: pc.Ordinal + " circonscription", URL: e.root + "/circonscription/" + code + "/"})
			}
			for _, ls := range circosDept {
				sort.Slice(ls, func(i, j int) bool { return ls[i].Code < ls[j].Code })
			}
			fmt.Printf("  circonscriptions : %d pages\n", len(circos))
			return districtsBundle{circos, popFrance, circosDept}, nil
		})

	reg.Ajouter(pipeline.Etape{Nom: "collectivites-pages-locales", Description: "pages région/département",
		Dependances: []string{"collectivites-data", "lieux", "fond-situation", "circonscriptions-data"},
		Executer: func(ctx context.Context, d pipeline.Results) (any, error) {
			cb := dep[localGovBundle](d, "collectivites-data")
			lieux := dep[*Resolveur](d, "lieux")
			fond := dep[*fondSituation](d, "fond-situation")
			cd := dep[districtsBundle](d, "circonscriptions-data")
			pagesCol, err := pagesCollectivites(ctx, e.pool, cb.Col, lieux)
			if err != nil {
				return nil, err
			}
			tcol := e.page("collectivite.gohtml")
			for _, pc := range pagesCol {
				if pc.TypeURL == "departement" {
					pc.Situation = fond.pourDepartement(pc.Code, pc.Nom, pc.Lignes)
					for _, dcode := range codesCOGDe(pc.Code) {
						pc.Circonscriptions = append(pc.Circonscriptions, cd.CircosDept[dcode]...)
					}
				} else {
					pc.Situation = fond.pourRegion(pc.Code, pc.Nom, pc.Lignes)
				}
				l := e.layout
				l.Title = pc.Nom
				if err := e.writeSection("collectivites", tcol, filepath.Join(e.out, "collectivites", pc.TypeURL, pc.Slug, "index.html"),
					struct {
						Layout
						K PageCollectivite
					}{l, pc}); err != nil {
					return nil, err
				}
			}
			return nil, nil
		}})

	// --- budget : trois chargements partagés avec sujets-data (sect,
	// circuit) plus un qui lui est propre (bud).
	addNode(reg, "budget-data", nil, func(ctx context.Context, _ pipeline.Results) (*StatsBudget, error) {
		return loadBudget(ctx, e.pool)
	})
	addNode(reg, "secteurs", nil, func(ctx context.Context, _ pipeline.Results) (*StatsSecteurs, error) {
		return loadSecteurs(ctx, e.pool)
	})
	addNode(reg, "circuit-canaux", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (*CircuitCanaux, error) {
		return loadCircuitCanaux(ctx, e.pool, dep[identityBundle](d, "identite").Presidences)
	})
	reg.Ajouter(pipeline.Etape{Nom: "budget", Description: "page budget",
		Dependances: []string{"budget-data", "secteurs", "circuit-canaux"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			bud := dep[*StatsBudget](d, "budget-data")
			sect := dep[*StatsSecteurs](d, "secteurs")
			circuit := dep[*CircuitCanaux](d, "circuit-canaux")
			if bud != nil {
				l := e.layout
				l.Title = "Budget de l'État"
				if err := e.writeSection("budget", e.page("budget.gohtml"), filepath.Join(e.out, "budget", "index.html"), struct {
					Layout
					B *StatsBudget
					X *StatsSecteurs
					K *CircuitCanaux
				}{l, bud, sect, circuit}); err != nil {
					return nil, err
				}
			}
			if circuit != nil {
				tdisp := e.page("dispositif.gohtml")
				for _, m := range circuit.GrandesMesures {
					l := e.layout
					l.Title = m.Libelle
					if err := e.writeSection("budget", tdisp, filepath.Join(e.out, "budget", "dispositif", m.Code, "index.html"), struct {
						Layout
						M MesureExoneration
					}{l, m}); err != nil {
						return nil, err
					}
				}
			}
			return nil, nil
		}})
	reg.Ajouter(pipeline.Etape{Nom: "social", Description: "page protection sociale", Dependances: []string{"social-data"},
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
	reg.Ajouter(pipeline.Etape{Nom: "qui-decide", Description: "page qui décide", Dependances: []string{"identite"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			l := e.layoutWithIdentity(dep[identityBundle](d, "identite"))
			l.Title = "Qui décide"
			return nil, e.writeSection("qui-decide", e.page("qui-decide.gohtml"), filepath.Join(e.out, "qui-decide", "index.html"), l)
		}})

	// --- sources et Assemblée nationale : dépendent d'identite pour la
	// liste des députés/groupes/candidats affichée.
	addNode(reg, "sources-page", nil, func(ctx context.Context, _ pipeline.Results) ([]SourceDetail, error) {
		return loadSources(ctx, e.pool)
	})
	addNode(reg, "sources-stats", nil, func(ctx context.Context, _ pipeline.Results) (*StatsGlobalesSources, error) {
		return chargerStatsGlobalesSources(ctx, e.pool)
	})
	reg.Ajouter(pipeline.Etape{Nom: "sources", Description: "page sources",
		Dependances: []string{"sources-page", "sources-stats"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			l := e.layout
			l.Title = "Sources"
			return nil, e.writeSection("sources", e.page("sources.gohtml"), filepath.Join(e.out, "sources", "index.html"), struct {
				Layout
				Flux  []SourceDetail
				Stats *StatsGlobalesSources
			}{l, dep[[]SourceDetail](d, "sources-page"), dep[*StatsGlobalesSources](d, "sources-stats")})
		}})
	reg.Ajouter(pipeline.Etape{Nom: "candidats", Description: "page candidats", Dependances: []string{"identite"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			id := dep[identityBundle](d, "identite")
			l := e.layout
			l.Title = "Candidats 2027"
			return nil, e.writeSection("candidats", e.page("candidats.gohtml"), filepath.Join(e.out, "candidats", "index.html"), struct {
				Layout
				Candidats []*Candidat
			}{l, id.Candidats})
		}})
	reg.Ajouter(pipeline.Etape{Nom: "partis", Description: "page partis", Dependances: []string{"identite"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			id := dep[identityBundle](d, "identite")
			l := e.layout
			l.Title = "Partis"
			return nil, e.writeSection("partis", e.page("partis.gohtml"), filepath.Join(e.out, "partis", "index.html"), struct {
				Layout
				Orgs []*Organisation
			}{l, id.OrgList})
		}})
	reg.Ajouter(pipeline.Etape{Nom: "assemblee", Description: "page Assemblée nationale",
		Dependances: []string{"identite", "derniers-scrutins"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			id := dep[identityBundle](d, "identite")
			derniers := dep[[]Vote](d, "derniers-scrutins")
			l := e.layout
			l.Title = "Assemblée nationale"
			return nil, e.writeSection("assemblee", e.page("assemblee.gohtml"), filepath.Join(e.out, "assemblee", "index.html"), struct {
				Layout
				Groupes  []*Groupe
				Deputes  []*Person
				Scrutins []Vote
			}{l, id.GrpList, id.Deputes, derniers})
		}})

	// --- la trentaine de schémas propres à sujets-data (voir la note de
	// tête de fichier : sujets-data en dépend TOUS, sans sous-ensemble
	// déduit à l'avance).
	type htmlLoader = func(context.Context, *pgxpool.Pool) (template.HTML, error)
	for nom, fn := range map[string]htmlLoader{
		"taux-pauvrete-serie":        chargerTauxPauvreteSerie,
		"depense-environnementale":   chargerDepenseEnvironnementale,
		"emploi-total-salarie":       chargerEmploiTotalSalarie,
		"depenses-fiscales-tendance": chargerTendanceDepensesFiscales,
		"ports-francais":             chargerPortsFrancais,
		"ports-europe":               chargerPortsEurope,
		"report-modal-port":          chargerReportModal,
		"report-modal-conteneurs":    chargerReportModalConteneurs,
		"carte-semi-conducteurs":     chargerCarteSemiConducteurs,
		"ecart-prix-dom":             chargerEcartPrixDOM,
		"alimentaire-dom":            chargerAlimentaireDOM,
		"sipri":                      chargerSIPRI,
		"historique-immigration":     chargerHistoriqueImmigration,
		"age-depart-retraite":        chargerAgeDepartRetraite,
		"taux-remplacement":          chargerTauxRemplacement,
	} {
		fn := fn
		addNode(reg, nom, nil, func(ctx context.Context, _ pipeline.Results) (template.HTML, error) { return fn(ctx, e.pool) })
	}
	addNode(reg, "contributif-non-contributif", nil,
		func(_ context.Context, _ pipeline.Results) (template.HTML, error) {
			return dessinerContributifNonContributif(), nil
		})
	addNode(reg, "seuils-pauvrete", nil, func(ctx context.Context, _ pipeline.Results) (*SeuilsPauvrete, error) {
		return chargerSeuilsPauvrete(ctx, e.pool)
	})
	addNode(reg, "climat-international", nil, func(ctx context.Context, _ pipeline.Results) (*StatsClimatInternational, error) {
		return chargerClimatInternational(ctx, e.pool)
	})
	addNode(reg, "union-europeenne", nil, func(ctx context.Context, _ pipeline.Results) (*StatsUnionEuropeenne, error) {
		return chargerUnionEuropeenne(ctx, e.pool)
	})
	addNode(reg, "carte-bassins", nil, func(ctx context.Context, _ pipeline.Results) (*CarteBassins, error) {
		return chargerCarteBassins(ctx, e.pool)
	})
	addNode(reg, "carte-eptb-epage", nil, func(ctx context.Context, _ pipeline.Results) (*CarteEPTBEPAGE, error) {
		return chargerCarteEPTBEPAGE(ctx, e.pool)
	})
	addNode(reg, "depenses-fiscales-stats", nil, func(ctx context.Context, _ pipeline.Results) (*StatsDepensesFiscales, error) {
		return chargerStatsDepensesFiscales(ctx, e.pool)
	})
	addNode(reg, "carte-ifi", nil, func(ctx context.Context, _ pipeline.Results) (*CarteIFI, error) {
		return chargerCarteIFI(ctx, e.pool)
	})
	addNode(reg, "appareil-productif", nil, func(ctx context.Context, _ pipeline.Results) (*StatsAppareilProductif, error) {
		return chargerAppareilProductif(ctx, e.pool)
	})
	addNode(reg, "francophonie", nil, func(ctx context.Context, _ pipeline.Results) (*StatsFrancophonie, error) {
		return chargerFrancophonie(ctx, e.pool)
	})
	addNode(reg, "empire-colonial", nil, func(ctx context.Context, _ pipeline.Results) (*StatsEmpireColonial, error) {
		return chargerEmpireColonial(ctx, e.pool)
	})
	addNode(reg, "seconde-guerre-mondiale", nil, func(ctx context.Context, _ pipeline.Results) (*StatsSecondeGuerreMondiale, error) {
		return chargerSecondeGuerreMondiale(ctx, e.pool)
	})
	addNode(reg, "population-guerres", nil, func(ctx context.Context, _ pipeline.Results) (*PopulationGuerres, error) {
		return chargerPopulationGuerres(ctx, e.pool)
	})
	addNode(reg, "controle-fiscal", nil, func(ctx context.Context, _ pipeline.Results) (*ControleFiscal, error) {
		return chargerControleFiscal(ctx, e.pool)
	})
	addNode(reg, "carte-infrastructure-ports", nil, func(ctx context.Context, _ pipeline.Results) (*CarteInfrastructurePorts, error) {
		return chargerCarteInfrastructurePorts(ctx, e.pool)
	})
	addNode(reg, "justice", nil, func(ctx context.Context, _ pipeline.Results) (*StatsJustice, error) {
		return chargerJustice(ctx, e.pool)
	})
	addNode(reg, "carte-musees", nil, func(ctx context.Context, _ pipeline.Results) (*CarteMusees, error) {
		return chargerCarteMusees(ctx, e.pool)
	})
	addNode(reg, "carte-etudiants", nil, func(ctx context.Context, _ pipeline.Results) (*CarteEtudiants, error) {
		return chargerCarteEtudiants(ctx, e.pool)
	})
	addNode(reg, "carte-sru", nil, func(ctx context.Context, _ pipeline.Results) (*CarteSRU, error) {
		return chargerCarteSRU(ctx, e.pool)
	})
	addNode(reg, "effort-recherche", nil, func(ctx context.Context, _ pipeline.Results) (*EffortRecherche, error) {
		return chargerEffortRecherche(ctx, e.pool)
	})

	// --- sujets-data : voir la note de tête de fichier.
	sujetsDeps := []string{"docs", "identite", "accueil-data", "territoires",
		"secteurs", "circuit-canaux", "seuils-pauvrete", "taux-pauvrete-serie", "depense-environnementale",
		"emploi-total-salarie", "climat-international", "union-europeenne", "carte-bassins", "carte-eptb-epage",
		"depenses-fiscales-tendance", "depenses-fiscales-stats", "carte-ifi", "appareil-productif", "francophonie",
		"empire-colonial", "seconde-guerre-mondiale", "population-guerres", "controle-fiscal", "ports-francais",
		"ports-europe", "carte-infrastructure-ports", "report-modal-port", "report-modal-conteneurs", "justice",
		"carte-semi-conducteurs", "contributif-non-contributif", "carte-musees", "ecart-prix-dom", "alimentaire-dom",
		"carte-etudiants", "carte-sru", "effort-recherche", "sipri", "historique-immigration", "age-depart-retraite",
		"taux-remplacement",
	}
	addNode(reg, "sujets-data", sujetsDeps, func(ctx context.Context, d pipeline.Results) (topicsReady, error) {
		return topicsReady{}, buildTopicsData(ctx, e, d)
	})

	// identite en dépendance directe (pas seulement via sujets-data, qui
	// l'inclut déjà transitivement) : Executer ne remonte que les
	// dépendances DIRECTES d'un nœud dans deps, jamais toute la fermeture —
	// accueil.gohtml lit Cov.Candidats/CandidatsPrimaire/CandidatsAvecBilan
	// (voir layoutWithIdentity), qu'il faut donc nommer ici aussi.
	reg.Ajouter(pipeline.Etape{Nom: "accueil", Description: "page d'accueil",
		Dependances: []string{"accueil-data", "sujets-data", "identite"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			acc := dep[*DonneesAccueil](d, "accueil-data")
			l := e.layoutWithIdentity(dep[identityBundle](d, "identite"))
			l.Hero = true
			l.HeroTitre = "Le budget réel de la France, sujet par sujet de la campagne 2027"
			l.HeroLede = "Retraites, santé, école, sécurité, immigration, dette : ce que l'État dépense, " +
				"d'où vient l'argent, qui décide et qui contrôle — sourcé document par document."
			l.HeroVisuel = heroMille(acc, e.root)
			l.Title = "Le budget réel de la France, sujet par sujet de la campagne 2027"
			return nil, e.writeSection("accueil", e.page("accueil.gohtml"), filepath.Join(e.out, "index.html"), struct {
				Layout
				A *DonneesAccueil
			}{l, acc})
		}})
	reg.Ajouter(pipeline.Etape{Nom: "sujets", Description: "index des sujets",
		Dependances: []string{"accueil-data", "sujets-data"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			acc := dep[*DonneesAccueil](d, "accueil-data")
			l := e.layout
			l.Title = "Sujets de campagne"
			return nil, e.writeSection("sujets", e.page("sujets.gohtml"), filepath.Join(e.out, "sujets", "index.html"), struct {
				Layout
				Familles []*Famille
				Annee    int
			}{l, acc.Familles, acc.Annee})
		}})
	reg.Ajouter(pipeline.Etape{Nom: "argent-public", Description: "argent public",
		Dependances: []string{"accueil-data", "fonctions", "sujets-data"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			acc := dep[*DonneesAccueil](d, "accueil-data")
			pagesFonctions := dep[map[string]*PageFonction](d, "fonctions")
			tf := e.page("fonction.gohtml")
			for _, f := range acc.Fonctions {
				pf := pagesFonctions[f.Code]
				l := e.layout
				l.Title = pf.Nom
				if err := e.writeSection("argent-public", tf, filepath.Join(e.out, "fonction", f.Slug, "index.html"), struct {
					Layout
					F *PageFonction
				}{l, pf}); err != nil {
					return nil, err
				}
			}
			l := e.layout
			l.Title = "Argent public"
			return nil, e.writeSection("argent-public", e.page("argent-public.gohtml"), filepath.Join(e.out, "argent-public", "index.html"),
				struct {
					Layout
					A *DonneesAccueil
				}{l, acc})
		}})
	reg.Ajouter(pipeline.Etape{Nom: "comprendre", Description: "documents de méthode et sujets",
		Dependances: []string{"sujets-data", "docs"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			return nil, buildGuidesAndTopics(e, dep[[]*Doc](d, "docs"))
		}})

	// --- fiches : personnes, candidats, organisations, référentiels, groupes.
	addNode(reg, "mandats-locaux", []string{"lieux"},
		func(ctx context.Context, d pipeline.Results) (map[string]*RapprochementRNE, error) {
			locaux, err := loadMandatsLocaux(ctx, e.pool, e.dataDir+"/candidats-mandats-locaux.csv")
			if err != nil {
				return nil, err
			}
			situerMandatsLocaux(locaux, dep[*Resolveur](d, "lieux"))
			return locaux, nil
		})
	addNode(reg, "votes-bulk", []string{"identite"}, func(ctx context.Context, d pipeline.Results) (bool, error) {
		id := dep[identityBundle](d, "identite")
		if err := loadVotesBulk(ctx, e.pool, id.Persons, 60); err != nil {
			return false, err
		}
		return true, nil
	})
	reg.Ajouter(pipeline.Etape{Nom: "fiches", Description: "fiches personnes/candidats/organisations",
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
	reg.Ajouter(pipeline.Etape{Nom: "recherche", Description: "index de recherche",
		Dependances: []string{"identite", "themes", "sujets-data", "docs", "senat"},
		Executer: func(_ context.Context, d pipeline.Results) (any, error) {
			id := dep[identityBundle](d, "identite")
			themes := dep[*StatsThemes](d, "themes")
			docs := dep[[]*Doc](d, "docs")
			senat := dep[*StatsSenat](d, "senat")
			senateurs := map[string]bool{}
			for _, p := range senat.Senateurs2 {
				senateurs[p.Slug] = true
			}
			return nil, ecrireIndex(e.out, id.Persons, id.Candidats, id.Orgs, id.Groupes, id.Refs,
				themes.Themes, docs, senateurs)
		}})

	// --- scrutin : garde son court-circuit par empreinte, verbatim.
	reg.Ajouter(pipeline.Etape{Nom: "scrutin", Description: "pages de scrutin", Dependances: []string{"seuils"},
		Executer: func(ctx context.Context, d pipeline.Results) (any, error) {
			seuils := dep[map[string]Seuil](d, "seuils")
			return buildBallotSection(ctx, e, seuils)
		}})

	return reg
}
