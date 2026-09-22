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

	"github.com/faits-politiques/faits-politiques/internal/an"
	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/carto"
	"github.com/faits-politiques/faits-politiques/internal/checksum"
	"github.com/faits-politiques/faits-politiques/internal/communes"
	"github.com/faits-politiques/faits-politiques/internal/europe"
	"github.com/faits-politiques/faits-politiques/internal/geo"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/faits-politiques/faits-politiques/internal/macro"
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
				return nil, source.Executer(ctx, pool, &archEtape, rawDir)
			},
		})
	}
	if err := reg.Publier(ctx); err != nil {
		return nil, fmt.Errorf("publication de la topologie : %w", err)
	}
	return reg, nil
}

// registreDe construit un pipeline.Registre pour N'IMPORTE QUEL
// sous-ensemble du catalogue, pas seulement le socle — même mécanique que
// registreParlement (une copie d'Archive par étape, jamais le pointeur
// partagé), généralisée. La quasi-totalité des sources hors socle n'ont
// aucune dépendance déclarée entre elles (Source.Dependances vide) :
// partagées dans une seule vague, elles tournent alors TOUTES de front
// jusqu'à Concurrence, là où RunCategorie les exécutait jusqu'ici une par
// une dans l'ordre du catalogue. Ne publie PAS en base : Publier réécrit
// core.pipeline_etape pour l'ensemble exact qu'on lui donne — le faire
// depuis un sous-ensemble effacerait le socle. Publier reste réservé à
// registreParlement, la seule vue complète et auditée du graphe.
func registreDe(pool *pgxpool.Pool, arch *archive.Archive, rawDir string, noms []string) (*pipeline.Registre, error) {
	reg := pipeline.NouveauRegistre(pool)
	vus := map[string]bool{}
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
	pool, arch, fermer, err := contexte(ctx, rawDir, migDir)
	if err != nil {
		return err
	}
	defer fermer()
	reg, err := registreDe(pool, arch, rawDir, noms)
	if err != nil {
		return err
	}

	// reg.Noms() est déjà la fermeture résolue de noms (registreDe ne pose
	// que ce dont la cible a réellement besoin) : ce sont exactement les
	// connecteurs qui vont tourner ci-dessous, avant même de savoir dans
	// quel ordre le graphe les enchaînera. Une commande fpctl build qui
	// résout normalize+senat+europe+carto+themes attend aujourd'hui que
	// chacun ait fini pour lancer le suivant AVANT de commencer son propre
	// téléchargement — un ordre hérité de l'écriture du code, jamais une
	// vraie dépendance de données : aucun de ces téléchargements n'a besoin
	// qu'un autre ait fini pour commencer le sien.
	// -dry-run affiche le plan sans rien exécuter (pipeline.Registre.Executer
	// s'en charge plus bas) : télécharger quoi que ce soit ici irait à
	// l'encontre de cette promesse.
	dryRun := len(opts) > 0 && opts[0].DryRun
	if !dryRun {
		concurrence := 1
		if len(opts) > 0 && opts[0].Concurrence > 0 {
			concurrence = opts[0].Concurrence
		}
		ctx, err = PrefetchAll(ctx, arch, downloadTargetsFor(reg.Noms()), concurrence)
		if err != nil {
			return err
		}
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
	return out
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
	pool, arch, fermer, err := contexte(ctx, rawDir, migDir)
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
		reg, err := registreParlement(ctx, pool, arch, rawDir)
		if err != nil {
			return err
		}
		// registreParlement construit tout le socle, pas seulement nom : on
		// prétélécharge donc les cibles du socle entier plutôt que la
		// fermeture exacte de nom (que Niveaux calculerait, un peu de travail
		// en plus pour un seul appel visé) — un léger surcroît de
		// téléchargement pour une demande étroite, jamais une incorrection.
		if !dryRun {
			ctx, err = PrefetchAll(ctx, arch, downloadTargetsFor(reg.Noms()), concurrence)
			if err != nil {
				return err
			}
		}
		if _, err := reg.Executer(ctx, []string{nom}, opts...); err != nil {
			return err
		}
		if dryRun {
			return nil
		}
		return actualiserMatviews(ctx)
	}
	reg, err := registreDe(pool, arch, rawDir, []string{nom})
	if err != nil {
		return err
	}
	if !dryRun {
		ctx, err = PrefetchAll(ctx, arch, downloadTargetsFor(reg.Noms()), concurrence)
		if err != nil {
			return err
		}
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
	pool, arch, fermer, err := contexte(ctx, rawDir, migDir)
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
	if !dryRun {
		concurrence := 1
		if len(opts) > 0 && opts[0].Concurrence > 0 {
			concurrence = opts[0].Concurrence
		}
		// Une seule vague de téléchargement pour la catégorie ENTIÈRE,
		// socle et reste confondus, avant que l'un ou l'autre registre ne
		// tourne — même raison que RunSources : aucun de ces
		// téléchargements n'a besoin qu'un autre ait fini pour commencer
		// le sien.
		ctx, err = PrefetchAll(ctx, arch, downloadTargetsFor(append(append([]string{}, socle...), reste...)), concurrence)
		if err != nil {
			return err
		}
	}

	if len(socle) > 0 {
		reg, err := registreParlement(ctx, pool, arch, rawDir)
		if err != nil {
			return err
		}
		if _, err := reg.Executer(ctx, socle, opts...); err != nil {
			return err
		}
	}
	if len(reste) > 0 {
		reg, err := registreDe(pool, arch, rawDir, reste)
		if err != nil {
			return err
		}
		if _, err := reg.Executer(ctx, reste, opts...); err != nil {
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
//   - communes -> normalize : le RNE ne remplace un mandat national que
//     s'il n'en existe pas déjà un publié par l'Assemblée (« le RNE
//     complète, il n'écrase pas », commentaire au-dessus de
//     normaliserAssemblee plus bas) — l'ordre inverse a un jour détruit
//     1 419 mandats de député.
//   - associations -> communes : associations.Ingest rapproche chaque
//     association d'une commune via ref.commune (internal/associations/
//     associations.go:200-204), remplie par communes.IngestCOG.
//   - hatvp -> normalize, communes, senat : le rapprochement déclarant ->
//     personne (D-025, internal/hatvp/hatvp.go:236-242) lit core.person,
//     alimentée par les trois.
//   - amendements/exposes/interventions -> normalize : lisent
//     respectivement core.texte (internal/an/amendements.go:301,
//     internal/an/exposes.go:77-80) et core.person_identifier scheme
//     AN_ACTEUR (internal/an/interventions.go:92), remplis par
//     normaliserAssemblee.
//   - jorf -> normalize, communes, senat : le rapprochement mention -> élu
//     (internal/jorf/jorf.go:375-384) lit core.person.
//   - promulgation -> normalize, jorf : compare la référence NOR publiée
//     par l'Assemblée (core.dossier) à jo.texte.nor, rempli par jorf.Ingest
//     (voir déjà le commentaire de RunTout à ce sujet, plus bas).
//   - media -> normalize, partis, carto : les logos de partis se
//     rapprochent par identifiant CNCCFP (internal/ingest/media.go:58-60),
//     écrit par les trois.
var dependancesRunTout = map[string][]string{
	"communes":      {"normalize"},
	"associations":  {"communes"},
	"hatvp":         {"normalize", "communes", "senat"},
	"amendements":   {"normalize"},
	"exposes":       {"normalize"},
	"interventions": {"normalize"},
	"jorf":          {"normalize", "communes", "senat"},
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
	"presidentielle", "budget", "macro", "prefets", "agriculture", "entreprises", "campagne",
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
//     chargé par "communes" (dimensionLocale), jamais catalogue.go
//     "contours" (qui recharge le COG lui-même, un jeu de contours par
//     millésime) — un vrai écart avec le catalogue, pas une erreur : voir
//     le commentaire d'origine sur ce choix, conservé tel quel plus bas.
//   - "checksums" ferme le graphe : recalculerEmpreintes recalcule par
//     construction tout ce que checksum.Sections connaît (le domaine
//     entier), donc dépend de tout le reste plutôt que d'un sous-ensemble
//     précis — jamais un fan-in partiel qui laisserait une section
//     recalculée sur des données d'avant cette exécution.
func registreComplet(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) (*pipeline.Registre, error) {
	reg := pipeline.NouveauRegistre(pool)
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
	// Contours communaux et intercommunaux, un jeu par millésime du COG. Le
	// bloc communes (dimensionLocale) a déjà chargé le COG courant : inutile
	// de le recharger ici (contrairement à la source « contours » invoquée
	// seule, catégorie systeme, qui le recharge elle-même).
	reg.Ajouter(pipeline.Etape{
		Nom: "geo-courant", Description: "IGN boundaries by vintage",
		Dependances: []string{"communes"},
		Executer: func(ctx context.Context, _ pipeline.Results) (any, error) {
			return nil, geo.Ingest(ctx, pool, arch, filepath.Join("data", "geo-projections.csv"), communes.COGMillesime)
		},
	})
	if err := ajouter("checksums", reg.Noms()); err != nil {
		return nil, err
	}
	return reg, nil
}

// RunTout exécute la chaîne complète historique : pas littéralement toutes
// les sources du catalogue (plusieurs sont délibérément hors chaîne par
// défaut — coûteuses, ponctuelles, ou exigeant une clé/un binaire
// particulier), mais le socle que « fpctl ingest all » a toujours rechargé.
// Pour une catégorie entière, y compris ce qu'elle a de plus coûteux, voir
// RunCategorie (« fpctl ingest <catégorie> all »).
//
// Passe désormais par le même graphe de dépendances que RunSources/
// RunCategorie (registreComplet) plutôt qu'une chaîne Go séquentielle codée
// à la main : le téléchargement de chaque connecteur se fait d'abord, tout
// de front (PrefetchAll), puis les étapes tournent par vagues topologiques
// jusqu'à -j de front — presidentielle/budget/macro/prefets/agriculture/
// entreprises/campagne, entre autres, n'ont jamais eu besoin d'attendre le
// socle parlementaire, seulement de l'ordre du fichier source pour s'exécuter
// jusqu'ici.
func RunTout(ctx context.Context, rawDir, migDir string, opts ...pipeline.Options) error {
	start := time.Now()
	pool, arch, fermer, err := contexte(ctx, rawDir, migDir)
	if err != nil {
		return err
	}
	defer fermer()

	reg, err := registreComplet(ctx, pool, arch, rawDir)
	if err != nil {
		return err
	}

	dryRun := len(opts) > 0 && opts[0].DryRun
	if !dryRun {
		concurrence := 1
		if len(opts) > 0 && opts[0].Concurrence > 0 {
			concurrence = opts[0].Concurrence
		}
		ctx, err = PrefetchAll(ctx, arch, downloadTargetsFor(reg.Noms()), concurrence)
		if err != nil {
			return err
		}
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
	logs.Notice("governments of the Fifth Republic")
	if err := carto.IngestGouvernements(ctx, pool, filepath.Join("data", "gouvernements.csv")); err != nil {
		return err
	}
	logs.Notice("presidencies of the Republic")
	return carto.IngestPresidents(ctx, pool, filepath.Join("data", "presidents.csv"))
}

// dimensionLocale charge la dimension communale, dans un ordre contraint :
// ref.commune est référencé par tout le reste, et les résultats électoraux ne
// peuvent pas être rattachés à une commune qui n'existe pas encore.
func dimensionLocale(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	logs.Notice("geographic reference data")
	if err := communes.IngestCOG(ctx, pool, arch); err != nil {
		return err
	}
	logs.Notice("mayors")
	if err := communes.IngestRNE(ctx, pool, arch); err != nil {
		return err
	}
	logs.Notice("municipal elections")
	if err := communes.IngestMunicipales(ctx, pool, arch); err != nil {
		return err
	}
	logs.Notice("municipal accounts")
	if err := communes.IngestOFGL(ctx, pool, arch); err != nil {
		return err
	}
	logs.Notice("intermunicipal bodies and their powers")
	if err := communes.IngestBANATIC(ctx, pool, arch); err != nil {
		return err
	}
	logs.Notice("2020 municipal elections")
	if err := communes.IngestMunicipales2020(ctx, pool, arch); err != nil {
		return err
	}
	logs.Notice("regional, departmental and grouping accounts")
	if err := communes.IngestCollectivites(ctx, pool, arch); err != nil {
		return err
	}
	logs.Notice("recorded crime by municipality")
	return communes.IngestSSMSI(ctx, pool, arch)
}
