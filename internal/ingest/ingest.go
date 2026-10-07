// Package ingest télécharge les jeux de données, les scelle dans l'archive,
// puis reconstruit core à partir de raw. Appelé par fpctl (voir cmd/fpctl),
// qui construit ses sous-commandes en itérant le catalogue (catalogue.go)
// plutôt qu'en recopiant la liste des sources :
//
//	fpctl ingest default                  chaîne par défaut (le socle habituel)
//	fpctl ingest full                 littéralement tout le catalogue
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

	"github.com/faits-politiques/faits-politiques/internal/an"
	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/carto"
	"github.com/faits-politiques/faits-politiques/internal/checksum"
	"github.com/faits-politiques/faits-politiques/internal/communes"
	"github.com/faits-politiques/faits-politiques/internal/europe"
	"github.com/faits-politiques/faits-politiques/internal/geo"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/faits-politiques/faits-politiques/internal/matview"
	"github.com/faits-politiques/faits-politiques/internal/migrate"
	"github.com/faits-politiques/faits-politiques/internal/partis"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
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

// SocleParlementaire : les noms du socle, dans un ordre qui place déjà
// chaque source après ce dont elle dépend — pour un affichage (fpctl list
// deps) qui n'a besoin ni de pool ni de base : la structure du graphe est
// dans le code (Source.Dependances), la base n'en garde qu'un REFLET
// (dernière exécution, taille), jamais la référence.
func SocleParlementaire() []string {
	return append([]string(nil), socleParlementaire...)
}

// EstSurLeSocle : nom fait-il partie des sept sources dont les dépendances
// sont déclarées et vérifiées ?
func EstSurLeSocle(nom string) bool {
	for _, n := range socleParlementaire {
		if n == nom {
			return true
		}
	}
	return false
}

// ChaineParDefaut : l'ensemble des noms que « fpctl ingest default » charge
// réellement — le socle parlementaire plus runToutSupplement, jamais les
// quelque 90 autres sources du catalogue (délibérément hors chaîne par
// défaut : coûteuses, ponctuelles, ou exigeant une clé/un binaire
// particulier — voir le commentaire de RunTout). internal/verify s'en sert
// pour ne rejouer, par défaut, que les contrôles dont la source est dans cet
// ensemble : sans ça, « fpctl verify data » après un « fpctl ingest default »
// tout à fait normal échoue systématiquement sur des données que cet ingest
// n'a jamais eu vocation à charger.
func ChaineParDefaut() map[string]bool {
	m := make(map[string]bool, len(socleParlementaire)+len(runToutSupplement))
	for _, n := range socleParlementaire {
		m[n] = true
	}
	for _, n := range runToutSupplement {
		m[n] = true
	}
	return m
}

// prefetchDependants : noms d'étapes dont le Fetch attend le futur
// "prefetch" du registre qui les contient (voir demarrerPrefetch), plutôt
// que de découvrir leur fichier au fil de l'eau — exactement les noms que
// downloadTargetsFor sait déjà détecter, dans l'autre sens. amendements :
// son propre fichier (internal/an/amendements.go, 296 Mo observés) n'est
// prérécupéré avec le reste de l'Assemblée que depuis qu'il est listé dans
// an.DownloadTargets — avant, il se téléchargeait seul, au moment précis où
// cette étape démarrait, derrière normalize et le reste de sa vague.
var prefetchDependants = func() map[string]bool {
	m := map[string]bool{"download": true, "partis": true, "senat": true, "europe": true, "amendements": true}
	for _, n := range communesChaineNoms {
		m[n] = true
	}
	return m
}()

// registreParlement construit le pipeline.Registre du socle parlementaire à
// partir du catalogue — une seule référence (catalogue.go) pour les deux :
// la liste plate que "fpctl ingest parlement" affiche, et le graphe que ce
// même socle exécute. Publie aussitôt la topologie en base
// (core.pipeline_etape/pipeline_dependance) : « fpctl ingest deps » reste à
// jour même si l'appel qui suit ne cible qu'une seule de ces sources.
//
// dryRun : jamais de récupération réelle pendant une simulation — demarrerPrefetch
// lance une vraie goroutine réseau dès qu'on l'appelle, donc on ne l'appelle
// simplement pas ici (pf reste nil, attendre() le traverse sans bloquer).
func registreParlement(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string,
	concurrence int, dryRun bool) (*pipeline.Registre, error) {
	reg := pipeline.NouveauRegistre(pool)
	var pf *prefetchFuture
	for _, nom := range socleParlementaire {
		source, ok := SourceParNom(nom)
		if !ok {
			panic(fmt.Sprintf("ingest : %q déclaré dans socleParlementaire mais absent du catalogue", nom))
		}
		// Copie propre à cette étape, pas le pointeur partagé : plusieurs
		// étapes du socle tournent de front dans une même vague
		// (internal/pipeline, Concurrence) — muter arch.Etape sur l'original
		// serait une course. Root/Pool restent partagés (immuables après
		// construction), seul Etape diffère par copie.
		archEtape := *arch
		archEtape.Etape = source.Nom
		reg.Ajouter(pipeline.Etape{
			Nom: source.Nom, Description: source.Description, Dependances: source.Dependances,
			Executer: func(ctx context.Context, _ pipeline.Results) (any, error) {
				if prefetchDependants[source.Nom] {
					var err error
					if ctx, err = pf.attendre(ctx); err != nil {
						return nil, err
					}
				}
				return nil, source.Executer(ctx, pool, &archEtape, rawDir)
			},
		})
	}
	// Déclenché seulement une fois la fermeture connue (reg.Noms(), juste
	// au-dessus) : ne bloque jamais la construction, seules les étapes qui
	// en dépendent (via pf.attendre, ci-dessus) patienteront si besoin.
	if !dryRun {
		pf = demarrerPrefetch(ctx, arch, downloadTargetsFor(reg.Noms()), concurrence)
	}
	if err := reg.Publier(ctx); err != nil {
		return nil, fmt.Errorf("publication de la topologie : %w", err)
	}
	return reg, nil
}

// registreDe construit un pipeline.Registre pour N'IMPORTE QUEL
// sous-ensemble du catalogue, pas seulement le socle — même mécanique que
// registreParlement (une copie d'Archive par étape, jamais le pointeur
// partagé, même futur de prérécupération non bloquant), généralisée. La
// quasi-totalité des sources hors socle n'ont aucune dépendance déclarée
// entre elles (Source.Dependances vide) : partagées dans une seule vague,
// elles tournent alors TOUTES de front jusqu'à Concurrence, là où
// RunCategorie les exécutait jusqu'ici une par une dans l'ordre du
// catalogue. Ne publie PAS en base : Publier réécrit core.pipeline_etape
// pour l'ensemble exact qu'on lui donne — le faire depuis un sous-ensemble
// effacerait le socle. Publier reste réservé à registreParlement, la seule
// vue complète et auditée du graphe.
func registreDe(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string, noms []string,
	concurrence int, dryRun bool) (*pipeline.Registre, error) {
	reg := pipeline.NouveauRegistre(pool)
	vus := map[string]bool{}
	var pf *prefetchFuture
	var ajouter func(nom string) error
	ajouter = func(nom string) error {
		if vus[nom] {
			return nil
		}
		vus[nom] = true
		source, ok := SourceParNom(nom)
		if !ok {
			return fmt.Errorf("source inconnue : %s (voir « fpctl ingest » pour la liste)", nom)
		}
		// Les dépendances d'abord : Registre.Ajouter panique si l'une
		// d'elles n'est pas déjà connue au moment où on ajoute nom.
		for _, d := range source.Dependances {
			if err := ajouter(d); err != nil {
				return err
			}
		}
		archEtape := *arch
		archEtape.Etape = source.Nom
		reg.Ajouter(pipeline.Etape{
			Nom: source.Nom, Description: source.Description, Dependances: source.Dependances,
			Executer: func(ctx context.Context, _ pipeline.Results) (any, error) {
				if prefetchDependants[source.Nom] {
					var err error
					if ctx, err = pf.attendre(ctx); err != nil {
						return nil, err
					}
				}
				return nil, source.Executer(ctx, pool, &archEtape, rawDir)
			},
		})
		return nil
	}
	for _, n := range noms {
		if err := ajouter(n); err != nil {
			return nil, err
		}
	}
	if !dryRun {
		pf = demarrerPrefetch(ctx, arch, downloadTargetsFor(reg.Noms()), concurrence)
	}
	return reg, nil
}

// actualiserMatviews ouvre son PROPRE pool, plus large que celui à 4
// connexions que store.Open donne au reste d'une commande d'ingestion — le
// même choix qu'internal/sitegen fait pour son propre besoin mesuré de
// parallélisme (store.OpenWithMaxConns), jamais en élargissant le défaut
// partagé. Sans ce second pool, matview.ActualiserToutesConcurrence pouvait
// demander autant de front qu'elle voulait : bridée aux 4 connexions du pool
// qu'on lui donnait, elle ne l'obtenait jamais.
func actualiserMatviews(ctx context.Context) error {
	const concurrence = 8
	mvPool, err := store.OpenWithMaxConns(ctx, concurrence)
	if err != nil {
		return err
	}
	defer mvPool.Close()
	return matview.ActualiserToutesConcurrence(ctx, mvPool, concurrence)
}

// RunSources exécute plusieurs sources du catalogue à la fois — vagues
// topologiques, jusqu'à opts[0].Concurrence de front par vague, exactement
// comme RunCategorie pour le socle qu'elle contient, généralisé à
// n'importe quelle liste : les préalables d'une section de fpctl build,
// par exemple (voir cmd/fpctl/build.go, ingestPrealables), au lieu de les
// ingérer un par un dans une boucle qui ignorait qu'ils n'ont, pour la
// plupart, aucune dépendance entre eux.
func RunSources(ctx context.Context, rawDir, migDir string, noms []string, opts ...pipeline.Options) error {
	if len(noms) == 0 {
		return nil
	}
	pool, arch, fermer, err := contexte(ctx, rawDir, migDir, concurrenceDuPool(opts))
	if err != nil {
		return err
	}
	defer fermer()

	// Une commande fpctl build qui résout normalize+senat+europe+carto+themes
	// attendait ici que chacun ait fini pour lancer le suivant AVANT de
	// commencer son propre téléchargement — un ordre hérité de l'écriture du
	// code, jamais une vraie dépendance de données : aucun de ces
	// téléchargements n'a besoin qu'un autre ait fini pour commencer le
	// sien. registreDe démarre maintenant leur récupération dès que sa
	// fermeture est connue (demarrerPrefetch), sans bloquer la construction
	// ni les étapes qui n'en dépendent pas.
	dryRun := len(opts) > 0 && opts[0].DryRun
	concurrence := 1
	if len(opts) > 0 && opts[0].Concurrence > 0 {
		concurrence = opts[0].Concurrence
	}
	reg, err := registreDe(ctx, pool, arch, rawDir, noms, concurrence, dryRun)
	if err != nil {
		return err
	}

	if _, err := reg.Executer(ctx, noms, opts...); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	// raw -> core est fait ; core -> mv avant de rendre la main, jamais
	// après. fpctl build appelle sitegen.RunSections juste après RunSources :
	// sans cette étape ICI, une page qui lit une matvue (14 fichiers de
	// internal/sitegen le font) verrait un raw -> core tout frais à côté
	// d'un mv.* resté sur l'exécution précédente — le même bogue que
	// RunTout évite déjà en bout de chaîne complète, mais dont fpctl build
	// n'avait jamais hérité.
	return actualiserMatviews(ctx)
}

// downloadTargetsFor réunit les cibles de téléchargement des connecteurs
// présents dans noms (la fermeture résolue par registreDe) — un connecteur
// qui n'expose pas de DownloadTargets n'a simplement rien à y ajouter
// (carto/themes/normalize ne téléchargent rien).
func downloadTargetsFor(noms []string) []archive.DownloadTarget {
	present := map[string]bool{}
	for _, n := range noms {
		present[n] = true
	}
	var out []archive.DownloadTarget
	if present["download"] {
		out = append(out, an.DownloadTargets()...)
	}
	if present["partis"] {
		out = append(out, partis.DownloadTargets()...)
	}
	if present["senat"] {
		out = append(out, senat.DownloadTargets()...)
	}
	if present["europe"] {
		out = append(out, europe.DownloadTargets()...)
	}
	// "communes" (l'alias) entraîne toujours tout communes-cog..communes-ssmsi
	// avec elle, mais un appel qui ne cible qu'un maillon précis de la chaîne
	// (fpctl ingest collectivites communes-ofgl, par exemple) ne fait PAS
	// apparaître "communes" dans noms — seulement ce maillon et ses
	// ancêtres. communes.DownloadTargets() reste la même liste complète dans
	// les deux cas (elle n'est pas découpée par maillon) : plus simple, et
	// sans incorrection puisqu'un fichier déjà prérécupéré ne l'est jamais
	// deux fois (archive.WithPrefetched).
	for _, maillon := range communesChaineNoms {
		if present[maillon] {
			out = append(out, communes.DownloadTargets()...)
			break
		}
	}
	return out
}

// concurrenceDuPool : la taille du pool Postgres que contexte doit ouvrir —
// au moins 4 (le défaut historique de store.Open, jamais réduit), mais
// élargi si -j demande plus de front que ça. Sans cet ajustement, -j restait
// sans effet réel au-delà de 4 : reg.Executer lance bien plus de goroutines,
// mais elles font toutes la queue pour la même poignée de 4 connexions —
// exactement le défaut qu'actualiserMatviews (plus bas) corrige déjà pour
// son propre pool, jamais recopié ici jusqu'à présent.
func concurrenceDuPool(opts []pipeline.Options) int32 {
	if len(opts) > 0 && int32(opts[0].Concurrence) > 4 {
		return int32(opts[0].Concurrence)
	}
	return 4
}

// contexte : ce que chaque point d'entrée (RunTout/RunSource/RunCategorie)
// ouvre avant de faire quoi que ce soit — les migrations en attente, le
// répertoire de l'archive scellée, le pool. Commun aux trois, pour que
// « fpctl ingest budget dette » applique les migrations en attente tout
// aussi sûrement que la chaîne complète.
func contexte(ctx context.Context, rawDir, migDir string, maxConns int32) (pool *pgxpool.Pool, arch *archive.Archive, fermer func(), err error) {
	pool, err = store.OpenWithMaxConns(ctx, maxConns)
	if err != nil {
		return nil, nil, nil, err
	}
	logs.Notice("migrations")
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
// sans qu'on ait à les nommer soi-même. -dry-run marche pour tout le
// catalogue désormais (registreDe en fait un registre à une seule étape,
// aussi simple à simuler que le socle) ; -j n'a simplement rien à
// paralléliser pour une source seule.
func RunSource(ctx context.Context, rawDir, migDir, nom string, opts ...pipeline.Options) error {
	if nom == "migrate" {
		pool, err := store.Open(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()
		logs.Notice("migrations")
		return migrate.Up(ctx, pool, migDir)
	}
	if _, ok := SourceParNom(nom); !ok {
		return fmt.Errorf("source inconnue : %s (voir « fpctl ingest » pour la liste)", nom)
	}
	pool, arch, fermer, err := contexte(ctx, rawDir, migDir, concurrenceDuPool(opts))
	if err != nil {
		return err
	}
	defer fermer()
	dryRun := len(opts) > 0 && opts[0].DryRun
	concurrence := 1
	if len(opts) > 0 && opts[0].Concurrence > 0 {
		concurrence = opts[0].Concurrence
	}
	if EstSurLeSocle(nom) {
		// registreParlement construit tout le socle, pas seulement nom : la
		// prérécupération porte donc sur les cibles du socle entier plutôt
		// que la fermeture exacte de nom (que Niveaux calculerait, un peu de
		// travail en plus pour un seul appel visé) — un léger surcroît de
		// téléchargement pour une demande étroite, jamais une incorrection.
		reg, err := registreParlement(ctx, pool, arch, rawDir, concurrence, dryRun)
		if err != nil {
			return err
		}
		if _, err := reg.Executer(ctx, []string{nom}, opts...); err != nil {
			return err
		}
		if dryRun {
			return nil
		}
		return actualiserMatviews(ctx)
	}
	reg, err := registreDe(ctx, pool, arch, rawDir, []string{nom}, concurrence, dryRun)
	if err != nil {
		return err
	}
	if _, err := reg.Executer(ctx, []string{nom}, opts...); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	// raw -> core est fait pour cette source ; core -> mv avant de rendre
	// la main, jamais après — voir le même choix dans RunSources.
	return actualiserMatviews(ctx)
}

// RunCategorie exécute toutes les sources d'une catégorie — un choix
// délibéré, plus large que la chaîne par défaut (RunTout) : une source
// marquée « hors chaîne par défaut » (coûteuse, ou exigeant une clé/un
// binaire particulier) reste hors de RunTout mais fait pleinement partie de
// sa catégorie ici — demander une catégorie entière est une décision
// explicite, pas un oubli.
//
// Les sources du socle parlementaire présentes dans la catégorie (voir
// socleParlementaire) passent d'abord, à part, par le registre audité et
// publié (registreParlement) — c'est le seul sous-ensemble dont les
// dépendances peuvent sortir de cette catégorie (normalize dépend de
// download et partis, tous deux « parlement », mais rien ne garantit
// qu'une future dépendance du socle y reste), et le seul dont la
// topologie doit se retrouver dans core.pipeline_etape (fpctl list deps) ;
// publier depuis un registre partiel l'amputerait. Le reste de la
// catégorie suit, TOUT ENSEMBLE, par registreDe : la quasi-totalité de ces
// sources n'ont aucune dépendance déclarée entre elles (Source.Dependances
// vide), donc partagent une seule vague et tournent de front jusqu'à
// opts[0].Concurrence, plutôt que l'ancienne boucle séquentielle qui les
// ingérait une par une dans l'ordre du catalogue sans jamais en profiter.
func RunCategorie(ctx context.Context, rawDir, migDir, categorie string, opts ...pipeline.Options) error {
	sources := SourcesDeCategorie(categorie)
	if len(sources) == 0 {
		return fmt.Errorf("catégorie inconnue : %s (voir « fpctl ingest » pour la liste)", categorie)
	}
	pool, arch, fermer, err := contexte(ctx, rawDir, migDir, concurrenceDuPool(opts))
	if err != nil {
		return err
	}
	defer fermer()
	start := time.Now()

	var socle, reste []string
	for _, s := range sources {
		if EstSurLeSocle(s.Nom) {
			socle = append(socle, s.Nom)
		} else {
			reste = append(reste, s.Nom)
		}
	}

	dryRun := len(opts) > 0 && opts[0].DryRun
	concurrence := 1
	if len(opts) > 0 && opts[0].Concurrence > 0 {
		concurrence = opts[0].Concurrence
	}
	// Construire les DEUX registres d'abord, avant d'exécuter l'un ou
	// l'autre : chacun démarre sa PROPRE récupération de front dès qu'il
	// connaît sa fermeture (registreParlement/registreDe, demarrerPrefetch),
	// et les deux futurs doivent partir avant que reg.Executer(socle) ne
	// bloque plus bas — sinon la récupération de reste n'aurait démarré
	// qu'une fois tout le socle déjà exécuté. Leurs cibles ne se recoupent
	// jamais (le socle ne télécharge rien que reste télécharge aussi), donc
	// deux futurs non bloquants lancés de front reviennent au même résultat
	// qu'un seul PrefetchAll bloquant pour les deux combinés — sans la
	// barrière commune devant les deux registres.
	var regSocle, regReste *pipeline.Registre
	if len(socle) > 0 {
		regSocle, err = registreParlement(ctx, pool, arch, rawDir, concurrence, dryRun)
		if err != nil {
			return err
		}
	}
	if len(reste) > 0 {
		regReste, err = registreDe(ctx, pool, arch, rawDir, reste, concurrence, dryRun)
		if err != nil {
			return err
		}
	}
	if regSocle != nil {
		if _, err := regSocle.Executer(ctx, socle, opts...); err != nil {
			return err
		}
	}
	if regReste != nil {
		if _, err := regReste.Executer(ctx, reste, opts...); err != nil {
			return err
		}
	}
	if dryRun {
		return nil
	}
	// raw -> core est fait pour toute la catégorie ; core -> mv avant de
	// rendre la main, jamais après — voir le même choix dans RunSources.
	if err := actualiserMatviews(ctx); err != nil {
		return err
	}
	logs.Notice(fmt.Sprintf("category %s done in %s", categorie, time.Since(start).Round(time.Second)))
	return nil
}

// dependancesRunTout : dépendances RÉELLES entre sources hors socle,
// établies par lecture de code (chaque preuve est un fichier:ligne précis,
// pas une supposition) — mais volontairement PAS ajoutées à
// Source.Dependances dans catalogue.go : un appel isolé (fpctl ingest
// collectivites communes, par exemple) doit rester léger, comme aujourd'hui
// — il ne réclamerait sinon plus tout le socle parlementaire juste pour
// recharger des élus locaux. registreComplet applique ces dépendances-ci
// UNIQUEMENT dans le cadre de la chaîne complète, où toutes ces sources
// tournent de toute façon ensemble.
//
//   - communes-rne -> normalize : le RNE ne remplace un mandat national que
//     s'il n'en existe pas déjà un publié par l'Assemblée (« le RNE
//     complète, il n'écrase pas », commentaire au-dessus de
//     normaliserAssemblee plus bas) — l'ordre inverse a un jour détruit
//     1 419 mandats de député. Portée sur communes-rne précisément (pas sur
//     l'alias « communes ») depuis que la chaîne communes-* existe comme
//     maillons nommés (catalogue.go) : c'est RNE, et lui seul, qui écrit
//     core.person/core.mandate parmi les 8 maillons — communes-cog n'a
//     aucune raison d'attendre normalize.
//   - associations -> communes-cog : associations.Ingest rapproche chaque
//     association d'une commune via ref.commune (internal/associations/
//     associations.go:248), rempli par communes.IngestCOG seul — pas besoin
//     du reste de la chaîne (RNE, OFGL, BANATIC...).
//   - hatvp -> normalize, communes-rne, senat : le rapprochement déclarant ->
//     personne (internal/hatvp/hatvp.go:292) lit core.person, alimentée par
//     les trois — core.person vient de normalize et de communes-rne
//     (internal/communes/rne.go:219), pas du reste de la chaîne communes-*.
//   - amendements/exposes/interventions -> normalize : lisent
//     respectivement core.texte (internal/an/amendements.go:301,
//     internal/an/exposes.go:77-80) et core.person_identifier scheme
//     AN_ACTEUR (internal/an/interventions.go:92), remplis par
//     normaliserAssemblee.
//   - jorf -> normalize, communes-rne, senat : le rapprochement mention ->
//     élu (internal/jorf/jorf.go:375-384) lit core.person — même source que
//     hatvp ci-dessus, pas le reste de la chaîne communes-*.
//   - promulgation -> normalize, jorf : compare la référence NOR publiée
//     par l'Assemblée (core.dossier) à jo.texte.nor, rempli par jorf.Ingest
//     (voir déjà le commentaire de RunTout à ce sujet, plus bas).
//   - media -> normalize, partis, carto : les logos de partis se
//     rapprochent par identifiant CNCCFP (internal/ingest/media.go:58-60),
//     écrit par les trois.
var dependancesRunTout = map[string][]string{
	"communes-rne":  {"normalize"},
	"associations":  {"communes-cog"},
	"hatvp":         {"normalize", "communes-rne", "senat"},
	"amendements":   {"normalize"},
	"exposes":       {"normalize"},
	"interventions": {"normalize"},
	"jorf":          {"normalize", "communes-rne", "senat"},
	"promulgation":  {"normalize", "jorf"},
	"media":         {"normalize", "partis", "carto"},
}

// runToutSupplement : les sources hors socle que RunTout a toujours
// enchaînées, dans un ordre où la dépendance de chacune (dependancesRunTout,
// plus haut) est déjà ajoutée avant elle — Registre.Ajouter panique sinon.
// presidentielle, budget, macro, prefets, agriculture, entreprises et
// campagne n'ont aucune dépendance ici : lecture exhaustive de chaque
// paquet (aucune référence à core.person, core.mandate, ref.commune,
// core.texte, core.dossier ou jo.texte hors du sien) — ils tournent donc
// dans la première vague venue, y compris de front avec le socle lui-même.
var runToutSupplement = []string{
	"presidentielle", "budget", "prefets", "agriculture", "entreprises", "campagne",
	// macro-* d'abord (aucune dépendance entre eux, voir catalogue.go), puis
	// l'alias "macro" qui les ferme — même raison que la chaîne communes-* :
	// ajouter ne résout pas les dépendances transitivement ici.
	"macro-eurostat", "macro-rsa", "macro-prestations-solidarite", "macro-recettes-fiscales",
	"macro-chomage-insee", "macro-minima-sociaux", "macro-age-retraite", "macro-demandeurs-emploi",
	"macro-prime-activite", "macro-taux-remplacement", "macro-cotisants-retraites", "macro",
	// La chaîne communes-* d'abord (son propre ordre, déclaré dans
	// Source.Dependances), puis l'alias "communes" qui la ferme — ajouter
	// ne résout PAS les dépendances transitivement (contrairement à
	// registreDe) : chaque nom doit déjà apparaître plus haut dans cette
	// liste avant d'être cité en Dependances, d'où cet ordre explicite.
	"communes-cog", "communes-rne", "communes-municipales", "communes-ofgl",
	"communes-banatic", "communes-municipales2020", "communes-collectivites", "communes-ssmsi",
	"communes", "associations", "hatvp",
	"amendements", "exposes", "interventions",
	"jorf", "promulgation", "media",
}

// registreComplet construit le graphe complet que RunTout exécute : le
// socle parlementaire (les mêmes 7 étapes que registreParlement, jamais
// republiées une seconde fois — Publier reste réservé à registreParlement,
// la seule vue auditée du graphe, voir son commentaire), plus
// runToutSupplement, plus deux étapes sans équivalent exact dans le
// catalogue :
//
//   - "geo-courant" : RunTout appelle geo.Ingest en réutilisant le COG déjà
//     chargé par la chaîne "communes-*" (catalogue.go), jamais
//     "contours" (qui recharge le COG lui-même, un jeu de contours par
//     millésime) — un vrai écart avec le catalogue, pas une erreur : voir
//     le commentaire d'origine sur ce choix, conservé tel quel plus bas.
//   - "checksums" ferme le graphe : recalculerEmpreintes recalcule par
//     construction tout ce que checksum.Sections connaît (le domaine
//     entier), donc dépend de tout le reste plutôt que d'un sous-ensemble
//     précis — jamais un fan-in partiel qui laisserait une section
//     recalculée sur des données d'avant cette exécution.
func registreComplet(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string,
	concurrence int, dryRun bool) (*pipeline.Registre, error) {
	reg := pipeline.NouveauRegistre(pool)
	var pf *prefetchFuture
	ajouter := func(nom string, extraDeps []string) error {
		source, ok := SourceParNom(nom)
		if !ok {
			return fmt.Errorf("registreComplet : source inconnue : %s", nom)
		}
		archEtape := *arch
		archEtape.Etape = source.Nom
		deps := append(append([]string{}, source.Dependances...), extraDeps...)
		reg.Ajouter(pipeline.Etape{
			Nom: source.Nom, Description: source.Description, Dependances: deps,
			Executer: func(ctx context.Context, _ pipeline.Results) (any, error) {
				if prefetchDependants[source.Nom] {
					var err error
					if ctx, err = pf.attendre(ctx); err != nil {
						return nil, err
					}
				}
				return nil, source.Executer(ctx, pool, &archEtape, rawDir)
			},
		})
		return nil
	}
	for _, nom := range socleParlementaire {
		if err := ajouter(nom, nil); err != nil {
			return nil, err
		}
	}
	for _, nom := range runToutSupplement {
		if err := ajouter(nom, dependancesRunTout[nom]); err != nil {
			return nil, err
		}
	}
	// Contours communaux et intercommunaux, un jeu par millésime du COG.
	// communes-cog (chaîne communes-*, catalogue.go) a déjà chargé le COG
	// courant : inutile de le recharger ici (contrairement à la source
	// « contours » invoquée seule, catégorie systeme, qui le recharge
	// elle-même). Dépend de communes-cog précisément, pas de l'alias
	// "communes" ni du reste de la chaîne (RNE, OFGL...) qui ne concerne pas
	// le COG.
	reg.Ajouter(pipeline.Etape{
		Nom: "geo-courant", Description: "IGN boundaries by vintage",
		Dependances: []string{"communes-cog"},
		Executer: func(ctx context.Context, _ pipeline.Results) (any, error) {
			return nil, geo.Ingest(ctx, pool, arch, filepath.Join("data", "geo-projections.csv"), communes.COGVintage)
		},
	})
	if err := ajouter("checksums", reg.Noms()); err != nil {
		return nil, err
	}
	if !dryRun {
		pf = demarrerPrefetch(ctx, arch, downloadTargetsFor(reg.Noms()), concurrence)
	}
	return reg, nil
}

// RunTout exécute la chaîne complète historique : pas littéralement toutes
// les sources du catalogue (plusieurs sont délibérément hors chaîne par
// défaut — coûteuses, ponctuelles, ou exigeant une clé/un binaire
// particulier), mais le socle que « fpctl ingest default » a toujours rechargé.
// Pour une catégorie entière, y compris ce qu'elle a de plus coûteux, voir
// RunCategorie (« fpctl ingest <catégorie> all »).
//
// Passe désormais par le même graphe de dépendances que RunSources/
// RunCategorie (registreComplet) plutôt qu'une chaîne Go séquentielle codée
// à la main — presidentielle/budget/macro/prefets/agriculture/entreprises/
// campagne, entre autres, n'ont jamais eu besoin d'attendre le socle
// parlementaire, seulement de l'ordre du fichier source pour s'exécuter
// jusqu'ici. Le téléchargement de chaque connecteur démarre de front dès
// que la fermeture du graphe est connue (registreComplet, demarrerPrefetch),
// mais SANS bloquer le graphe entier devant lui : une étape qui n'en a pas
// besoin (carto, normalize...) ne patiente jamais sur des fichiers que
// d'autres attendent.
func RunTout(ctx context.Context, rawDir, migDir string, opts ...pipeline.Options) error {
	start := time.Now()
	pool, arch, fermer, err := contexte(ctx, rawDir, migDir, concurrenceDuPool(opts))
	if err != nil {
		return err
	}
	defer fermer()

	dryRun := len(opts) > 0 && opts[0].DryRun
	concurrence := 1
	if len(opts) > 0 && opts[0].Concurrence > 0 {
		concurrence = opts[0].Concurrence
	}
	reg, err := registreComplet(ctx, pool, arch, rawDir, concurrence, dryRun)
	if err != nil {
		return err
	}

	if _, err := reg.Executer(ctx, reg.Noms(), opts...); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	// Même logique qu'ailleurs (voir RunSources) : core -> mv avant de
	// rendre la main, jamais après.
	if err := actualiserMatviews(ctx); err != nil {
		return err
	}

	logs.Notice(fmt.Sprintf("done in %s", time.Since(start).Round(time.Second)))
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
	logs.Notice("extracting into raw.record")
	for slug, f := range fetched {
		n, err := an.Extract(ctx, pool, f)
		if err != nil {
			return fmt.Errorf("%s : %w", slug, err)
		}
		logs.Notice(fmt.Sprintf("%s: %s extracted into raw.record", slug, logs.Plural(n, "record")))
	}
	return nil
}

func ingestPartis(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := partis.IngestAccounts(ctx, pool, arch); err != nil {
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
	if err := senat.MergePersons(ctx, pool); err != nil {
		return err
	}
	if err := senat.NormalizeMandates(ctx, pool); err != nil {
		return err
	}
	if err := senat.IngestCommissions(ctx, pool, arch); err != nil {
		return err
	}
	return senat.NormalizePresentations(ctx, pool)
}

// ingestMacro/ingestSocle ont disparu : leurs appels (onze pour l'un, huit
// pour l'autre) enchaînaient à la main des connecteurs qui n'ont, par
// lecture directe, aucune dépendance entre eux — chacun sa propre table
// (ref.macro_serie pour macro-eurostat, une table core.* dédiée pour tous
// les autres), jamais la lecture de ce qu'un autre vient d'écrire. Devenus
// des Source nommées à part entière (catalogue.go, préfixes "macro-" et
// "socle-"), SANS Dependances entre elles — à la différence de la chaîne
// communes-*, rien n'impose ici un ordre, donc elles tournent de front
// jusqu'à Concurrence plutôt que l'une après l'autre : un vrai gain, pas
// seulement une chronométrie individuelle. Les alias "macro"/"socle"
// regroupent chacun les leurs (Dependances sur la liste complète).

// recalculerEmpreintes met à jour core.section_checksum pour chaque section
// que internal/sitegen sait recopier plutôt que reconstruire (checksum.Sections).
// Ne décide de rien côté construction — seulement ce que internal/sitegen lira pour
// décider, lui, si les données d'une section ont changé.
func recalculerEmpreintes(ctx context.Context, pool *pgxpool.Pool) error {
	logs.Notice("section fingerprints (build cache)")
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
		logs.Notice(fmt.Sprintf("%s: fingerprint %s", section, h[:12]))
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
	logs.Notice("editorial mapping")
	if err := carto.Ingest(ctx, pool, filepath.Join("data", "organisations.csv")); err != nil {
		return err
	}
	// Présidences AVANT gouvernements : le rapprochement du Premier ministre
	// dans IngestGouvernements cherche un core.mandate de type
	// PRESIDENT_REPUBLIQUE parmi les mandats nationaux qui le distinguent
	// d'un homonyme (internal/carto/gouvernements.go, mandate_type IN (...,
	// 'PRESIDENT_REPUBLIQUE')) — mais ce type de mandat n'est écrit que par
	// IngestPresidents (internal/carto/presidents.go). Dans l'ordre inverse,
	// cette branche ne pouvait jamais matcher : un président jamais député
	// ni sénateur ni ministre par ailleurs (Georges Pompidou, Premier
	// ministre 1962, data/gouvernements.csv) restait sans
	// premier_ministre_person_id, silencieusement, le temps d'un premier
	// passage.
	logs.Notice("presidencies of the Republic")
	if err := carto.IngestPresidents(ctx, pool, filepath.Join("data", "presidents.csv")); err != nil {
		return err
	}
	logs.Notice("governments of the Fifth Republic")
	return carto.IngestGouvernements(ctx, pool, filepath.Join("data", "gouvernements.csv"))
}

// dimensionLocale a disparu : les 8 sous-étapes qu'elle enchaînait à la main
// (COG -> RNE -> municipales -> OFGL -> BANATIC -> municipales 2020 ->
// collectivités -> SSMSI) sont maintenant des Source nommées à part entière
// (catalogue.go, préfixe "communes-"), chaînées par Dependances comme tout
// le reste du catalogue — internal/pipeline les chronomètre individuellement
// (afficherDurees) sans qu'il soit besoin d'un helper dédié ici. Voir le
// commentaire au-dessus de ces entrées dans catalogue.go pour le pourquoi de
// la chaîne stricte (verrou ACCESS EXCLUSIVE de bulkload.SansContraintesFK)
// et pour l'alias "communes" qui les regroupe.
