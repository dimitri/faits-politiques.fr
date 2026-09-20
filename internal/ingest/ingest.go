// Package ingest télécharge les jeux de données, les scelle dans l'archive,
// puis reconstruit core à partir de raw. Appelé par fpctl (voir cmd/fpctl),
// qui construit ses sous-commandes en itérant le catalogue (catalogue.go)
// plutôt qu'en recopiant la liste des sources :
//
//	fpctl ingest all                    chaîne complète (le socle habituel)
//	fpctl ingest <catégorie>             liste les sources de la catégorie
//	fpctl ingest <catégorie> all         toutes les sources de la catégorie
//	fpctl ingest <catégorie> <source>    une source précise
package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/agriculture"
	"github.com/faits-politiques/faits-politiques/internal/an"
	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/associations"
	"github.com/faits-politiques/faits-politiques/internal/budget"
	"github.com/faits-politiques/faits-politiques/internal/campagne"
	"github.com/faits-politiques/faits-politiques/internal/carto"
	"github.com/faits-politiques/faits-politiques/internal/checksum"
	"github.com/faits-politiques/faits-politiques/internal/communes"
	"github.com/faits-politiques/faits-politiques/internal/entreprises"
	"github.com/faits-politiques/faits-politiques/internal/europe"
	"github.com/faits-politiques/faits-politiques/internal/geo"
	"github.com/faits-politiques/faits-politiques/internal/hatvp"
	"github.com/faits-politiques/faits-politiques/internal/jorf"
	"github.com/faits-politiques/faits-politiques/internal/macro"
	"github.com/faits-politiques/faits-politiques/internal/migrate"
	"github.com/faits-politiques/faits-politiques/internal/partis"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/faits-politiques/faits-politiques/internal/prefets"
	"github.com/faits-politiques/faits-politiques/internal/presidentielle"
	"github.com/faits-politiques/faits-politiques/internal/senat"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// socleParlementaire : les seules sources du catalogue dont les dépendances
// sont déclarées et vérifiées (Source.Dependances) — le socle audité une
// bonne fois pour toutes (voir internal/pipeline et le commentaire de
// Source.Dependances), pas encore tout le catalogue. RunSource/RunCategorie
// les résolvent et les exécutent via un pipeline.Registre (vagues
// topologiques, concurrence, simulation) plutôt qu'un simple appel direct.
var socleParlementaire = []string{"download", "partis", "normalize", "carto", "senat", "europe", "themes"}

func estSurLeSocle(nom string) bool {
	for _, n := range socleParlementaire {
		if n == nom {
			return true
		}
	}
	return false
}

// registreParlement construit le pipeline.Registre du socle parlementaire à
// partir du catalogue — une seule référence (catalogue.go) pour les deux :
// la liste plate que "fpctl ingest parlement" affiche, et le graphe que ce
// même socle exécute. Publie aussitôt la topologie en base
// (core.pipeline_etape/pipeline_dependance) : « fpctl ingest deps » reste à
// jour même si l'appel qui suit ne cible qu'une seule de ces sources.
func registreParlement(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) (*pipeline.Registre, error) {
	reg := pipeline.NouveauRegistre(pool)
	for _, nom := range socleParlementaire {
		source, ok := SourceParNom(nom)
		if !ok {
			panic(fmt.Sprintf("ingest : %q déclaré dans socleParlementaire mais absent du catalogue", nom))
		}
		reg.Ajouter(pipeline.Etape{
			Nom: source.Nom, Description: source.Description, Dependances: source.Dependances,
			Executer: func(ctx context.Context) error { return source.Executer(ctx, pool, arch, rawDir) },
		})
	}
	if err := reg.Publier(ctx); err != nil {
		return nil, fmt.Errorf("publication de la topologie : %w", err)
	}
	return reg, nil
}

// contexte : ce que chaque point d'entrée (RunTout/RunSource/RunCategorie)
// ouvre avant de faire quoi que ce soit — les migrations en attente, le
// répertoire de l'archive scellée, le pool. Commun aux trois, pour que
// « fpctl ingest budget dette » applique les migrations en attente tout
// aussi sûrement que la chaîne complète.
func contexte(ctx context.Context, rawDir, migDir string) (pool *pgxpool.Pool, arch *archive.Archive, fermer func(), err error) {
	pool, err = store.Open(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	fmt.Println("migrations")
	if err := migrate.Up(ctx, pool, migDir); err != nil {
		pool.Close()
		return nil, nil, nil, err
	}
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		pool.Close()
		return nil, nil, nil, err
	}
	return pool, &archive.Archive{Root: rawDir, Pool: pool}, pool.Close, nil
}

// RunSource exécute une seule source du catalogue, par son nom. Une source
// du socle parlementaire (voir socleParlementaire) résout et exécute
// d'abord ses dépendances — « fpctl ingest parlement normalize » sur une
// base neuve déclenche automatiquement download puis partis, dans l'ordre,
// sans qu'on ait à les nommer soi-même ; opts porte -dry-run/-j pour ce
// même socle (ignorés, silencieusement, pour tout le reste du catalogue,
// qui reste à exécution simple).
func RunSource(ctx context.Context, rawDir, migDir, nom string, opts ...pipeline.Options) error {
	if nom == "migrate" {
		pool, err := store.Open(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()
		fmt.Println("migrations")
		return migrate.Up(ctx, pool, migDir)
	}
	source, ok := SourceParNom(nom)
	if !ok {
		return fmt.Errorf("source inconnue : %s (voir « fpctl ingest » pour la liste)", nom)
	}
	pool, arch, fermer, err := contexte(ctx, rawDir, migDir)
	if err != nil {
		return err
	}
	defer fermer()
	if estSurLeSocle(nom) {
		reg, err := registreParlement(ctx, pool, arch, rawDir)
		if err != nil {
			return err
		}
		return reg.Executer(ctx, []string{nom}, opts...)
	}
	if len(opts) > 0 && opts[0].DryRun {
		return fmt.Errorf("-dry-run n'est pas pris en charge pour %s (hors du socle audité, voir socleParlementaire)", nom)
	}
	fmt.Printf("\n%s\n", source.Description)
	return source.Executer(ctx, pool, arch, rawDir)
}

// RunCategorie exécute toutes les sources d'une catégorie — un choix
// délibéré, plus large que la chaîne par défaut (RunTout) : une source
// marquée « hors chaîne par défaut » (coûteuse, ou exigeant une clé/un
// binaire particulier) reste hors de RunTout mais fait pleinement partie de
// sa catégorie ici — demander une catégorie entière est une décision
// explicite, pas un oubli.
//
// Les sources du socle parlementaire présentes dans la catégorie (voir
// socleParlementaire) passent d'abord, ensemble, par internal/pipeline :
// vagues topologiques, jusqu'à opts[0].Concurrence de front par vague — pour
// « parlement », ça place « editorial, senat, europe » dans une même vague,
// concurremment. Le reste de la catégorie suit, dans l'ordre du catalogue,
// à exécution simple.
func RunCategorie(ctx context.Context, rawDir, migDir, categorie string, opts ...pipeline.Options) error {
	sources := SourcesDeCategorie(categorie)
	if len(sources) == 0 {
		return fmt.Errorf("catégorie inconnue : %s (voir « fpctl ingest » pour la liste)", categorie)
	}
	pool, arch, fermer, err := contexte(ctx, rawDir, migDir)
	if err != nil {
		return err
	}
	defer fermer()
	start := time.Now()

	var socle []string
	for _, s := range sources {
		if estSurLeSocle(s.Nom) {
			socle = append(socle, s.Nom)
		}
	}
	if len(socle) > 0 {
		reg, err := registreParlement(ctx, pool, arch, rawDir)
		if err != nil {
			return err
		}
		if err := reg.Executer(ctx, socle, opts...); err != nil {
			return err
		}
	}
	if len(opts) > 0 && opts[0].DryRun {
		if len(socle) == len(sources) {
			return nil
		}
		return fmt.Errorf("-dry-run n'est pas pris en charge pour le reste de %s (hors du socle audité)", categorie)
	}

	for _, s := range sources {
		if estSurLeSocle(s.Nom) {
			continue
		}
		fmt.Printf("\n%s\n", s.Description)
		if err := s.Executer(ctx, pool, arch, rawDir); err != nil {
			return fmt.Errorf("%s : %w", s.Nom, err)
		}
	}
	fmt.Printf("\ncatégorie %s terminée en %s\n", categorie, time.Since(start).Round(time.Second))
	return nil
}

// RunTout exécute la chaîne complète historique : pas littéralement toutes
// les sources du catalogue (plusieurs sont délibérément hors chaîne par
// défaut — coûteuses, ponctuelles, ou exigeant une clé/un binaire
// particulier), mais le socle que « fpctl ingest all » a toujours rechargé.
// Pour une catégorie entière, y compris ce qu'elle a de plus coûteux, voir
// RunCategorie (« fpctl ingest <catégorie> all »).
func RunTout(ctx context.Context, rawDir, migDir string) error {
	start := time.Now()
	pool, arch, fermer, err := contexte(ctx, rawDir, migDir)
	if err != nil {
		return err
	}
	defer fermer()

	fmt.Println("\ntéléchargement et scellement")
	if err := telechargerAssemblee(ctx, pool, arch); err != nil {
		return err
	}

	fmt.Println("\nréférentiels sur les organisations politiques")
	if err := ingestPartis(ctx, pool, arch); err != nil {
		return err
	}

	// La normalisation de l'Assemblée vient AVANT le RNE et le Sénat.
	// L'ordre n'est pas cosmétique : le RNE n'insère un mandat de député que si
	// l'Assemblée n'en a pas déjà publié un (« le RNE complète, il n'écrase
	// pas »). Tant que la normalisation passait en dernier, ce garde-fou ne
	// gardait rien — le RNE arrivait le premier avec sa version pauvre, sans
	// circonscription ni date de fin, et la contrainte d'exclusion faisait
	// rejeter celle de l'Assemblée. En silence.
	fmt.Println("\nnormalisation raw -> core")
	if err := normaliserAssemblee(ctx, pool); err != nil {
		return err
	}

	fmt.Println("\nSénat")
	if err := ingestSenat(ctx, pool, arch, rawDir); err != nil {
		return err
	}

	fmt.Println("\nParlement européen")
	if err := europe.Ingest(ctx, pool, arch); err != nil {
		return err
	}

	if err := dimensionLocale(ctx, pool, arch); err != nil {
		return err
	}

	fmt.Println("\ntissu associatif")
	if err := associations.Ingest(ctx, pool, arch); err != nil {
		return err
	}

	fmt.Println("\ndéclarations d'intérêts et de patrimoine")
	if err := hatvp.Ingest(ctx, pool, arch); err != nil {
		return err
	}

	fmt.Println("\nélection présidentielle, population par âge, participation comparée")
	if err := presidentielle.Ingest(ctx, pool, arch); err != nil {
		return err
	}
	if err := presidentielle.IngestPopulation(ctx, pool, arch); err != nil {
		return err
	}
	if err := presidentielle.IngestTurnout(ctx, pool, arch); err != nil {
		return err
	}

	fmt.Println("\nbudget de l'État et de la Sécurité sociale")
	if err := budget.Ingest(ctx, pool, arch); err != nil {
		return err
	}

	fmt.Println("\ngrandes séries nationales")
	if err := ingestMacro(ctx, pool, arch); err != nil {
		return err
	}
	if err := prefets.Ingest(ctx, pool, arch); err != nil {
		return err
	}

	// Contours communaux et intercommunaux, un jeu par millésime du COG. Le
	// bloc communes (dimensionLocale) a déjà chargé le COG courant : inutile
	// de le recharger ici (contrairement à la source « contours » invoquée
	// seule, catégorie systeme, qui le recharge elle-même).
	fmt.Println("\ncontours IGN par millésime")
	if err := geo.Ingest(ctx, pool, arch, filepath.Join("data", "geo-projections.csv"), communes.COGMillesime); err != nil {
		return err
	}

	fmt.Println("\nbilans alimentaires et appareil de production agricole")
	if err := agriculture.Ingest(ctx, pool, arch); err != nil {
		return err
	}

	fmt.Println("\ncomptes déposés des grandes sociétés")
	if err := entreprises.Ingest(ctx, pool, arch); err != nil {
		return err
	}

	// Les travaux qui s'appuient sur core.texte et core.dossier viennent après
	// la normalisation, jamais avant : ils y font référence par clé étrangère.
	fmt.Println("\namendements et exposés sommaires")
	if err := an.IngestAmendements(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nexposés des motifs")
	if err := an.IngestExposes(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\ninterventions en séance")
	if err := an.IngestInterventions(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\ncomptes de campagne")
	if err := campagne.Ingest(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nactes nominatifs du Journal officiel")
	if err := jorf.Ingest(ctx, pool, arch, 60); err != nil {
		return err
	}
	// Après jorf.Ingest, jamais avant : le rattachement compare la référence
	// NOR publiée par l'Assemblée à jo.texte.nor, qui vient d'être rempli.
	fmt.Println("\nrattachement des dossiers à la loi promulguée")
	if err := an.PromulgationDossiers(ctx, pool); err != nil {
		return err
	}

	if err := cartographie(ctx, pool); err != nil {
		return err
	}
	// Après senat ET europe, jamais avant : un thème calculé avant que
	// l'Europe ait tourné manquait toute la couverture PARLEMENT_EUROPEEN —
	// le bug qui a motivé le graphe de dépendances déclaré (voir la source
	// « themes » du catalogue et internal/pipeline).
	fmt.Println("\nthèmes applicables aux scrutins")
	if err := carto.Themes(ctx, pool); err != nil {
		return err
	}

	fmt.Println("\nportraits et logos librement réutilisables")
	if err := ingestMedia(ctx, pool, arch, "data", "web/media"); err != nil {
		return err
	}

	// En dernier : bon marché (quelques secondes, mesuré), et une exécution
	// partielle (une seule catégorie, par exemple) peut très bien avoir
	// touché une table dont dépend une section du cache de cmd/build
	// (core.texte_expose fait partie de la section « scrutin »).
	if err := recalculerEmpreintes(ctx, pool); err != nil {
		return err
	}

	fmt.Printf("\nterminé en %s\n", time.Since(start).Round(time.Second))
	return nil
}

// --- ce que RunTout et le catalogue partagent : les blocs multi-étapes du
// socle parlementaire, extraits une fois pour ne jamais diverger entre la
// chaîne complète et « fpctl ingest parlement <source> ».

func telechargerAssemblee(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	fetched, err := an.Download(ctx, arch)
	if err != nil {
		return err
	}
	fmt.Println("\nextraction vers raw.record")
	for slug, f := range fetched {
		n, err := an.Extract(ctx, pool, f)
		if err != nil {
			return fmt.Errorf("%s : %w", slug, err)
		}
		fmt.Printf("  %-12s %d enregistrements\n", slug, n)
	}
	return nil
}

func ingestPartis(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := partis.IngestComptes(ctx, pool, arch); err != nil {
		return err
	}
	if err := partis.IngestPopuList(ctx, pool, arch); err != nil {
		return err
	}
	return partis.IngestCHES(ctx, pool, arch)
}

func normaliserAssemblee(ctx context.Context, pool *pgxpool.Pool) error {
	if err := an.Normalize(ctx, pool); err != nil {
		return err
	}
	// Les déports sont déjà dans raw.record : normalisation seule.
	return an.NormalizeDeports(ctx, pool)
}

func ingestSenat(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
	if err := senat.Ingest(ctx, pool, arch, filepath.Join(rawDir, "senat-work")); err != nil {
		return err
	}
	// Le répertoire des sénateurs vient après les votes : il complète les
	// personnes issues des scrutins, et crée les sénateurs plus anciens
	// qu'aucun vote n'a fait connaître.
	if err := senat.IngestSenateurs(ctx, pool, arch); err != nil {
		return err
	}
	// Puis la fusion, car le Sénat ne partage aucun identifiant avec les
	// autres sources : sans elle, un sénateur également conseiller municipal
	// existe en deux fiches, chacune amputée de la moitié de sa vie publique.
	// Elle vient avant les mandats et les commissions pour qu'ils se
	// rattachent à la fiche unique.
	if err := senat.Fusionner(ctx, pool); err != nil {
		return err
	}
	if err := senat.NormalizeMandats(ctx, pool); err != nil {
		return err
	}
	if err := senat.IngestCommissions(ctx, pool, arch); err != nil {
		return err
	}
	return senat.NormalizePresentations(ctx, pool)
}

// ingestMacro : les grandes séries nationales, plus la représentation de
// l'État — regroupées ici car RunTout les recharge ensemble depuis toujours ;
// prefets.Ingest reste appelable seul (catégorie systeme, source « prefets »).
func ingestMacro(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := macro.Ingest(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestRSA(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestPrestationsSolidarite(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestRecettesFiscales(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestChomageINSEE(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestMinimaSociaux(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestAgeDepartRetraite(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestDemandeursEmploi(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestPrimeActivite(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestTauxRemplacement(ctx, pool, arch); err != nil {
		return err
	}
	return macro.IngestCotisantsRetraites(ctx, pool, arch)
}

func ingestSocle(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := macro.IngestPauvrete(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestAideAlimentaire(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestPauvreteTauxEU(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestMenagesDREES(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestMenagesEffectif(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestPensionsEIR(ctx, pool, arch); err != nil {
		return err
	}
	if err := macro.IngestChomageUnedic(ctx, pool, arch); err != nil {
		return err
	}
	return macro.IngestFilosofiDeciles(ctx, pool, arch)
}

// recalculerEmpreintes met à jour core.section_checksum pour chaque section
// que cmd/build sait recopier plutôt que reconstruire (checksum.Sections).
// Ne décide de rien côté construction — seulement ce que cmd/build lira pour
// décider, lui, si les données d'une section ont changé.
func recalculerEmpreintes(ctx context.Context, pool *pgxpool.Pool) error {
	fmt.Println("\nempreintes des sections (cache de construction)")
	noms := make([]string, 0, len(checksum.Sections))
	for section := range checksum.Sections {
		noms = append(noms, section)
	}
	sort.Strings(noms)
	for _, section := range noms {
		h, err := checksum.Section(ctx, pool, checksum.Sections[section])
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO core.section_checksum (section, data_hash, updated_at)
			VALUES ($1, $2, now())
			ON CONFLICT (section) DO UPDATE
			SET data_hash = excluded.data_hash, updated_at = excluded.updated_at`,
			section, h); err != nil {
			return err
		}
		fmt.Printf("  %s : %s\n", section, h[:12])
	}
	return nil
}

// cartographie charge les décisions de rattachement puis en déduit le thème
// applicable à chaque scrutin. Les deux vont ensemble : sans le rattachement
// parti -> groupe, un thème ne se relie à aucune famille politique.
// cartographie charge les décisions de rattachement (partis -> groupes,
// gouvernements, présidences) — mais PAS les thèmes : ceux-ci dépendent du
// Sénat ET de l'Europe (voir la source « themes » du catalogue), jamais
// prêts au même moment que cette seule cartographie éditoriale.
func cartographie(ctx context.Context, pool *pgxpool.Pool) error {
	fmt.Println("\ncartographie éditoriale")
	if err := carto.Ingest(ctx, pool, filepath.Join("data", "organisations.csv")); err != nil {
		return err
	}
	fmt.Println("\ngouvernements de la Ve République")
	if err := carto.IngestGouvernements(ctx, pool, filepath.Join("data", "gouvernements.csv")); err != nil {
		return err
	}
	fmt.Println("\nprésidences de la République")
	return carto.IngestPresidents(ctx, pool, filepath.Join("data", "presidents.csv"))
}

// dimensionLocale charge la dimension communale, dans un ordre contraint :
// ref.commune est référencé par tout le reste, et les résultats électoraux ne
// peuvent pas être rattachés à une commune qui n'existe pas encore.
func dimensionLocale(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	fmt.Println("\nréférentiel géographique")
	if err := communes.IngestCOG(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nmaires")
	if err := communes.IngestRNE(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nélections municipales")
	if err := communes.IngestMunicipales(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\ncomptes des communes")
	if err := communes.IngestOFGL(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nintercommunalités et compétences")
	if err := communes.IngestBANATIC(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nélections municipales 2020")
	if err := communes.IngestMunicipales2020(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\ncomptes des régions, départements et groupements")
	if err := communes.IngestCollectivites(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\ndélinquance enregistrée par commune")
	return communes.IngestSSMSI(ctx, pool, arch)
}
