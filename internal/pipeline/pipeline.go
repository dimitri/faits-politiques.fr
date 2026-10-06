// Package pipeline remplace la chaîne de branches "if only == X" par un
// graphe de dépendances déclaré : chaque étape nomme ce dont elle a besoin,
// et le demander lance ses prérequis d'abord, dans le bon ordre — jamais
// l'ordre du fichier source comme seule garantie.
//
// Trois défauts concrets de l'ancienne chaîne, trouvés en simulant un ingest
// à froid (base neuve) plutôt qu'en local où l'état déjà présent masquait
// tout : « normalize » appelait une étape de cartographie qui supposait
// « partis » déjà passé ; « senat » calculait la couverture du Parlement
// européen avant que « europe » ait tourné, avec un dénominateur nul ; et
// cette même étape, logée à l'intérieur du bloc « senat », ne se
// recalculait jamais une fois l'Europe réellement chargée. Aucun de ces
// bugs n'était visible dans le fichier — il fallait suivre l'ordre
// d'exécution pour les voir. Un graphe déclaré les rend visibles à la
// lecture, et un ordre incorrect devient une erreur de dépendance cyclique
// plutôt qu'un plantage profond dans du code sans rapport.
package pipeline

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

// stepNameKey n'est jamais exporté : seul StepName y donne accès, pour
// qu'un appelant ne puisse jamais écrire directement dans le ctx d'une
// étape en cours d'exécution.
type stepNameKey struct{}

// StepName : le nom de l'étape en train de s'exécuter, lu dans le ctx
// qu'Executer passe à Etape.Executer — pour un consommateur qui a besoin de
// savoir QUI l'appelle sans le faire remonter explicitement à travers
// chaque fonction intermédiaire (voir internal/sitegen, le traçage des
// tables lues par page). false si ctx ne vient pas d'un appel d'Executer
// (par exemple un test qui invoque directement Etape.Executer).
func StepName(ctx context.Context) (string, bool) {
	nom, ok := ctx.Value(stepNameKey{}).(string)
	return nom, ok
}

// Results : ce que les dépendances déjà exécutées d'une étape ont produit,
// indexé par nom — nil pour une étape qui n'agit que par effet de bord
// (le cas de tout l'ingest aujourd'hui). internal/matview et internal/sitegen
// s'en servent pour de vraies valeurs (une matvue rafraîchie, une page
// chargée) qu'une étape dépendante lit directement au lieu de rejouer le
// calcul ou de rouvrir une connexion pour le refaire.
type Results map[string]any

// Step : une unité nommée, ce dont elle dépend, ce qu'elle fait — et ce
// qu'elle produit, lu par ses dépendantes dans Results. Run garde sa propre
// logique d'idempotence (comme l'ingest aujourd'hui) — ce paquet ne décide
// que DE L'ORDRE (et, en option, du parallélisme), jamais de sauter une
// étape déjà faite : c'est à l'étape elle-même de le constater vite si
// c'est le cas.
type Step struct {
	Name         string
	Description  string
	Dependencies []string
	Run          func(ctx context.Context, deps Results) (any, error)
}

// Registry : les étapes connues, indexées par nom.
type Registry struct {
	steps map[string]Step
	order []string // ordre de déclaration, pour un tri stable à dépendances égales
	pool  *pgxpool.Pool
}

// NewRegistry : pool sert à publier la topologie et l'historique
// d'exécution dans core.pipeline_etape/pipeline_dependance (voir Publish et
// Run) — jamais à lire quoi que ce soit de la base pour décider de l'ordre,
// qui reste entièrement déterminé par le code Go.
func NewRegistry(pool *pgxpool.Pool) *Registry {
	return &Registry{steps: map[string]Step{}, pool: pool}
}

// Add enregistre une étape. Panique sur un nom en double ou une dépendance
// vers une étape inconnue : ce sont des erreurs de programmation, pas des
// cas à gérer à l'exécution.
func (r *Registry) Add(s Step) {
	if _, exists := r.steps[s.Name]; exists {
		panic(fmt.Sprintf("pipeline : étape %q déclarée deux fois", s.Name))
	}
	for _, d := range s.Dependencies {
		if _, ok := r.steps[d]; !ok {
			panic(fmt.Sprintf("pipeline : étape %q dépend de %q, non déclarée avant elle", s.Name, d))
		}
	}
	r.steps[s.Name] = s
	r.order = append(r.order, s.Name)
}

// Names : tous les noms d'étapes connus, dans l'ordre de déclaration — pour
// lister les valeurs valides d'un drapeau -only sans les recopier à la main,
// ou générer une sous-commande par étape (voir cmd/fpctl).
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Step renvoie la déclaration d'une étape par son nom (description,
// dépendances) — pour fpctl list deps et la génération des sous-commandes.
func (r *Registry) Step(name string) (Step, bool) {
	s, ok := r.steps[name]
	return s, ok
}

// closure calcule, par DFS, l'ensemble des étapes nécessaires aux cibles
// demandées (elles-mêmes comprises) — détecte aussi les cycles et les noms
// inconnus, avant tout calcul de niveaux.
func (r *Registry) closure(targets []string) (map[string]bool, error) {
	needed := map[string]bool{}
	visited := map[string]int{} // 0 = jamais vu, 1 = en cours (cycle), 2 = fait
	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch visited[name] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("dépendance cyclique : %v -> %s", path, name)
		}
		s, ok := r.steps[name]
		if !ok {
			return fmt.Errorf("étape inconnue : %s", name)
		}
		visited[name] = 1
		for _, d := range s.Dependencies {
			if err := visit(d, append(path, name)); err != nil {
				return err
			}
		}
		visited[name] = 2
		needed[name] = true
		return nil
	}
	for _, t := range targets {
		if err := visit(t, nil); err != nil {
			return nil, err
		}
	}
	return needed, nil
}

// Levels regroupe les étapes nécessaires aux cibles demandées en VAGUES :
// chaque vague ne dépend que des vagues précédentes, donc les étapes d'une
// même vague sont indépendantes entre elles et peuvent s'exécuter en
// parallèle (voir Run, Concurrency). L'ordre des étapes DANS une vague suit
// l'ordre de déclaration, pour un plan reproductible d'un lancement à
// l'autre.
func (r *Registry) Levels(targets []string) ([][]string, error) {
	needed, err := r.closure(targets)
	if err != nil {
		return nil, err
	}
	done := map[string]bool{}
	var levels [][]string
	for len(done) < len(needed) {
		var wave []string
		for _, name := range r.order { // ordre de déclaration : un plan stable
			if !needed[name] || done[name] {
				continue
			}
			ready := true
			for _, d := range r.steps[name].Dependencies {
				if needed[d] && !done[d] {
					ready = false
					break
				}
			}
			if ready {
				wave = append(wave, name)
			}
		}
		if len(wave) == 0 {
			// La fermeture a déjà écarté les cycles ; ne devrait jamais arriver.
			return nil, fmt.Errorf("pipeline : aucune étape prête alors qu'il en reste — incohérence interne")
		}
		for _, name := range wave {
			done[name] = true
		}
		levels = append(levels, wave)
	}
	return levels, nil
}

// Options d'exécution. Concurrency <= 1 : séquentiel (comportement par
// défaut, celui qu'avait ce paquet avant) — le parallélisme est une
// optimisation qu'on choisit, jamais une surprise sur un pipeline qui
// partage un pool de connexions et des points d'accès externes rate-limités.
type Options struct {
	DryRun      bool
	Concurrency int
}

// Run résout les dépendances des cibles demandées et exécute chaque étape
// nécessaire, une fois — les prérequis silencieusement oubliés deviennent
// structurellement impossibles plutôt que découverts un par un, en
// production, à la lecture d'un message d'erreur sans rapport avec ce qui
// manque réellement.
//
// Le répartiteur est continu, pas vague par vague (Levels reste utile pour
// LIRE le plan — printPlan, fpctl ingest --dry-run — mais ne gouverne plus
// l'exécution) : une étape part dès que SES PROPRES dépendances sont
// faites, jamais en attendant que tout le reste de sa vague nominale ait
// fini. Deux étapes réelles de ce dépôt le montrent : "media" ne dépend que
// de "carto" (rapide), pas de "communes" (9+ minutes) — les deux tombaient
// pourtant dans la même vague nominale que "communes", et media attendait
// sa fin pour rien. Jusqu'à Concurrency étapes tournent de front, au total,
// pas par vague ; dès qu'une échoue, aucune étape non encore lancée ne
// démarre — ses dépendantes ne peuvent de toute façon jamais devenir prêtes
// (voir plus bas, le compteur restant n'est décrémenté que sur succès), mais
// un échec arrête aussi tout le reste, lancé ou non, pas seulement la
// branche touchée : la garantie d'origine ("aucune vague suivante ne
// démarre") vaut pour le graphe entier, pas étape par étape.
//
// Le Results renvoyé porte ce que chaque étape exécutée a produit — vide
// (valeurs nil) pour un registre dont les étapes n'agissent que par effet
// de bord, comme l'ingest.
func (r *Registry) Run(ctx context.Context, targets []string, opts ...Options) (Results, error) {
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	levels, err := r.Levels(targets)
	if err != nil {
		return nil, err
	}
	if opt.DryRun {
		r.printPlan(levels, opt.Concurrency)
		return nil, nil
	}
	limit := opt.Concurrency
	if limit < 1 {
		limit = 1
	}

	// needed/priority : la fermeture déjà calculée par Levels, aplatie —
	// priority rejoue l'ordre de déclaration pour départager deux étapes
	// prêtes en même temps, un plan reproductible d'un lancement à l'autre
	// (même but que l'ordre DANS une vague avant cette réécriture).
	var neededOrder []string
	priority := map[string]int{}
	for _, wave := range levels {
		for _, name := range wave {
			priority[name] = len(neededOrder)
			neededOrder = append(neededOrder, name)
		}
	}

	// dependents/remaining : le graphe inverse des Dependencies, et pour
	// chaque étape le nombre de prérequis pas encore terminés — une étape
	// rejoint ready dès que son remaining tombe à zéro.
	dependents := map[string][]string{}
	remaining := map[string]int{}
	for _, name := range neededOrder {
		for _, d := range r.steps[name].Dependencies {
			if _, needed := priority[d]; needed {
				remaining[name]++
				dependents[d] = append(dependents[d], name)
			}
		}
	}
	var ready []string
	for _, name := range neededOrder {
		if remaining[name] == 0 {
			ready = append(ready, name)
		}
	}

	results := Results{}
	durations := map[string]time.Duration{}
	var mu sync.Mutex
	startTotal := time.Now()

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)

	type arrival struct {
		name string
		err  error
	}
	// Bufferisé à la taille de la fermeture entière : un envoi n'attend
	// jamais que le répartiteur soit disponible pour le lire, qu'il soit
	// occupé à lancer d'autres étapes ou bloqué dans g.Go (voir launch) —
	// seul le sémaphore interne de g borne le nombre de goroutines en vol.
	arrivals := make(chan arrival, len(neededOrder))

	launch := func(name string) {
		s := r.steps[name]
		// Lu sous mu : contrairement à l'ancien découpage par vagues (où
		// g.Wait() garantissait la vague précédente entièrement écrite
		// avant que celle-ci ne lise results), une étape peut désormais
		// démarrer pendant que d'autres, sans rapport, sont encore en
		// cours d'écriture — la même carte results est donc lue ET écrite
		// en concurrence, ce qui réclame le même mutex des deux côtés.
		mu.Lock()
		deps := Results{}
		for _, d := range s.Dependencies {
			deps[d] = results[d]
		}
		mu.Unlock()
		g.Go(func() error {
			// logs.Notice, pas fmt.Printf : plusieurs étapes sans rapport de
			// dépendance tournent de front, et internal/logs sait déjà
			// sérialiser proprement des écritures concurrentes sur le même
			// stderr (voir internal/logs/lock.go) — un mutex posé ici ferait
			// la même chose en moins bien.
			logs.Notice(s.Description)
			start := time.Now()
			value, err := s.Run(context.WithValue(gctx, stepNameKey{}, name), deps)
			duration := time.Since(start)
			if err != nil {
				err = fmt.Errorf("%s (après %s) : %w", name, duration.Round(time.Millisecond), err)
				arrivals <- arrival{name, err}
				return err
			}
			// Le départ de chaque étape se lit déjà dans les logs (NOTICE
			// ci-dessus, horodaté par internal/logs) — sans cette ligne
			// d'arrivée, retrouver COMBIEN de temps une étape a pris
			// demandait de recouper deux horodatages à la main, impossible
			// dès que plusieurs étapes tournent de front (leurs lignes
			// s'entrelacent). Les deux bornes dans le même paquet, jamais
			// recalculées ailleurs.
			logs.Notice(fmt.Sprintf("%s : terminé en %s", name, duration.Round(time.Millisecond)))
			mu.Lock()
			results[name] = value
			durations[name] = duration
			mu.Unlock()
			if r.pool != nil {
				if _, err := r.pool.Exec(gctx,
					`UPDATE core.pipeline_etape SET derniere_execution_reussie = now() WHERE nom = $1`,
					name); err != nil {
					err = fmt.Errorf("%s : journal d'exécution : %w", name, err)
					arrivals <- arrival{name, err}
					return err
				}
			}
			arrivals <- arrival{name, nil}
			return nil
		})
	}

	// La boucle de répartition elle-même : inFlight compte les étapes
	// lancées dont l'arrivée n'est pas encore lue ici, et borne elle-même
	// le nombre d'appels à g.Go (jamais au-delà de limit) — PAS g.Go/son
	// sémaphore. Lui laisser bloquer reviendrait à pouvoir lancer une
	// (limit+1)-ième étape avant d'avoir lu l'échec éventuel de l'une des
	// limit déjà en vol (le sémaphore se libère dès qu'une goroutine
	// revient, pas quand CE répartiteur a lu son arrival) : un échec
	// pourrait alors laisser partir une étape qui n'aurait jamais dû
	// démarrer. En ne faisant jamais patienter le répartiteur à
	// l'intérieur de g.Go, chaque échec est vu avant toute nouvelle
	// répartition.
	stopped := false
	inFlight := 0
	for {
		if !stopped {
			for len(ready) > 0 && inFlight < limit {
				idx := 0
				for i := 1; i < len(ready); i++ {
					if priority[ready[i]] < priority[ready[idx]] {
						idx = i
					}
				}
				name := ready[idx]
				ready = append(ready[:idx], ready[idx+1:]...)
				launch(name)
				inFlight++
			}
		}
		if inFlight == 0 {
			break
		}
		a := <-arrivals
		inFlight--
		if a.err != nil {
			// Ni ses dépendantes (qui ne peuvent de toute façon jamais
			// devenir prêtes : remaining ne descend que sur succès) ni le
			// reste du graphe non encore lancé ne démarrent — la même
			// garantie qu'avant, au niveau du graphe entier plutôt que
			// vague par vague.
			stopped = true
			continue
		}
		for _, dep := range dependents[a.name] {
			remaining[dep]--
			if remaining[dep] == 0 {
				ready = append(ready, dep)
			}
		}
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	printDurations(durations, time.Since(startTotal))
	return results, nil
}

// printDurations : un résumé trié par coût décroissant, pour répondre tout
// de suite à « qu'est-ce qui a pris du temps ? » sans recouper des lignes
// NOTICE entrelacées à la main — surtout utile sous Concurrency > 1, où
// l'ordre d'apparition dans les logs ne reflète plus l'ordre de déclaration
// ni le coût réel. Les dix étapes les plus lentes suffisent : le but est de
// repérer un goulot, pas de remplacer un vrai profil (pprof) si le besoin
// allait plus loin que ça.
func printDurations(durations map[string]time.Duration, wallTotal time.Duration) {
	if len(durations) == 0 {
		return
	}
	type row struct {
		name     string
		duration time.Duration
	}
	sorted := make([]row, 0, len(durations))
	var sum time.Duration
	for name, d := range durations {
		sorted = append(sorted, row{name, d})
		sum += d
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].duration > sorted[j].duration })
	n := len(sorted)
	if n > 10 {
		n = 10
	}
	logs.Notice(fmt.Sprintf("étapes les plus coûteuses (somme des étapes : %s, mur : %s, %d étapes) :",
		sum.Round(time.Second), wallTotal.Round(time.Second), len(sorted)))
	for _, l := range sorted[:n] {
		logs.Notice(fmt.Sprintf("  %s : %s", l.name, l.duration.Round(time.Millisecond)))
	}
}

func (r *Registry) printPlan(levels [][]string, concurrency int) {
	fmt.Println("simulation (rien n'est exécuté) :")
	for i, wave := range levels {
		var desc []string
		for _, name := range wave {
			desc = append(desc, name)
		}
		parallel := ""
		if concurrency > 1 && len(wave) > 1 {
			parallel = fmt.Sprintf(" (jusqu'à %d en parallèle)", min(concurrency, len(wave)))
		}
		fmt.Printf("  %d. %s%s\n", i+1, strings.Join(desc, ", "), parallel)
	}
}

// Publish réécrit la topologie déclarée dans core.pipeline_etape et
// core.pipeline_dependance — un reflet, jamais la source de vérité, qui
// reste le code Go. Une étape retirée du registre (donc absente de cet
// appel) disparaît de la base par la même occasion : ON DELETE CASCADE
// emporte ses dépendances avec elle. derniere_execution_reussie n'est
// jamais touchée ici, seulement par Run.
func (r *Registry) Publish(ctx context.Context) error {
	if r.pool == nil {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	names := r.Names()
	if _, err := tx.Exec(ctx,
		`DELETE FROM core.pipeline_etape WHERE nom <> ALL($1::text[])`, names); err != nil {
		return fmt.Errorf("nettoyage des étapes disparues : %w", err)
	}
	for _, name := range names {
		s := r.steps[name]
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.pipeline_etape (nom, description) VALUES ($1, $2)
			ON CONFLICT (nom) DO UPDATE SET description = excluded.description`,
			name, s.Description); err != nil {
			return fmt.Errorf("%s : %w", name, err)
		}
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM core.pipeline_dependance WHERE etape = ANY($1::text[])`, names); err != nil {
		return fmt.Errorf("nettoyage des dépendances : %w", err)
	}
	for _, name := range names {
		for _, dep := range r.steps[name].Dependencies {
			if _, err := tx.Exec(ctx,
				`INSERT INTO core.pipeline_dependance (etape, depend_de) VALUES ($1, $2)`,
				name, dep); err != nil {
				return fmt.Errorf("%s -> %s : %w", name, dep, err)
			}
		}
	}
	return tx.Commit(ctx)
}
