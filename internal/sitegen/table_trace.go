package sitegen

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// tableTracer observe, pendant une construction, les tables (core./ref./
// geo./derived./jo./mv.) que chaque nœud du graphe (voir graphe.go,
// addNode/addPageNode) lit réellement — MESURÉES à l'exécution via
// pgx.QueryTracer, jamais devinées par relecture du code. Le nœud en cours
// est lu dans le ctx que le répartiteur (internal/pipeline) a déjà enrichi
// de son nom (pipeline.StepName) : aucun des appels à addNode/addPageNode
// dans graphe.go/graphe_sections.go n'a besoin d'être touché pour que ce
// traçage fonctionne.
//
// Pourquoi mesurer plutôt que relire le code : une liste tenue à la main
// (internal/matview.TablesDirectes, avant ce paquet) dérive silencieusement
// du code réel dès qu'une page change sa requête — exactement ce qui a
// fait échouer build-pr sur core.medecin_secteur_effectif (PR Prochaines
// étapes, session du 6 octobre 2026) avant d'être corrigé à la main. Une
// mesure prise à chaque construction ne peut pas dériver : elle REFLÈTE le
// code qui vient de tourner, jamais un instantané figé au moment où
// quelqu'un a pensé à la mettre à jour.
type tableTracer struct {
	mu       sync.Mutex
	parNoeud map[string]map[string]bool
}

func newTableTracer() *tableTracer {
	return &tableTracer{parNoeud: map[string]map[string]bool{}}
}

// tableRe : FROM/JOIN suivi d'un identifiant qualifié schema.table — les
// seules formes que ce dépôt écrit (jamais de nom non qualifié sur core/
// ref/geo/derived/jo/mv, voir la revue qui a introduit internal/matview).
// Insensible à la casse pour couvrir un éventuel FROM/JOIN en majuscules
// recopié d'ailleurs ; les noms de table de ce dépôt sont eux-mêmes toujours
// en minuscules.
var tableRe = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*)`)

// TraceQueryStart implémente pgx.QueryTracer. Les requêtes lancées hors
// d'un nœud du graphe (aucun pipeline.StepName dans ctx — par exemple
// l'ouverture du pool elle-même, ou un appel direct hors construction) ne
// sont pas comptées : ce traceur ne répond qu'à « que lit CETTE page ».
func (t *tableTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	nom, ok := pipeline.StepName(ctx)
	if !ok {
		return ctx
	}
	matches := tableRe.FindAllStringSubmatch(data.SQL, -1)
	if len(matches) == 0 {
		return ctx
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	set := t.parNoeud[nom]
	if set == nil {
		set = map[string]bool{}
		t.parNoeud[nom] = set
	}
	for _, m := range matches {
		set[strings.ToLower(m[1])] = true
	}
	return ctx
}

// TraceQueryEnd : rien à faire, tout se joue au départ de la requête (le
// SQL lui-même, pas son résultat).
func (t *tableTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// Tables : les tables observées pour un nœud donné, triées — nil si ce
// nœud n'a tourné aucune requête directement identifiable (un nœud qui ne
// fait que lire les Results d'une dépendance et écrire un gabarit, par
// exemple).
func (t *tableTracer) Tables(nom string) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	set := t.parNoeud[nom]
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for table := range set {
		out = append(out, table)
	}
	sort.Strings(out)
	return out
}

// Noeuds : tous les nœuds pour lesquels au moins une table a été observée,
// triés — pour PublierTables, qui réécrit le reflet en base nœud par nœud.
func (t *tableTracer) Noeuds() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.parNoeud))
	for nom := range t.parNoeud {
		out = append(out, nom)
	}
	sort.Strings(out)
	return out
}

// transitiveTables : l'union des tables observées pour nom ET pour tout ce
// dont il dépend, transitivement (reg porte déjà ce graphe — Etape.
// Dependances — nul besoin d'y revenir par une requête). Stocker cette
// union plutôt que les seules tables lues DIRECTEMENT par nom fait que
// lire core.sitegen_table_usage répond tout de suite à « de quoi ai-je
// besoin pour reconstruire CETTE page », sans que l'appelant (fpctl list
// deps) n'ait à son tour à redérouler le graphe de dépendances.
func transitiveTables(reg *pipeline.Registre, tracer *tableTracer, nom string) []string {
	vus := map[string]bool{}
	set := map[string]bool{}
	var visiter func(string)
	visiter = func(n string) {
		if vus[n] {
			return
		}
		vus[n] = true
		for _, t := range tracer.Tables(n) {
			set[t] = true
		}
		e, ok := reg.Etape(n)
		if !ok {
			return
		}
		for _, d := range e.Dependances {
			visiter(d)
		}
	}
	visiter(nom)
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// publierTables réécrit core.sitegen_table_usage pour les nœuds qui
// viennent de tourner (executes, les clés de ce que reg.Executer a
// renvoyé) — DELETE puis INSERT bornés à CES nœuds, jamais à la totalité
// de la table : une construction partielle (-only, -max-scrutins) ne doit
// pas effacer le reflet des pages qu'elle n'a pas touchées cette fois.
// Pas de transaction longue à retenir : le volume (quelques centaines de
// lignes au plus, un nœud a rarement plus d'une poignée de tables dans sa
// fermeture transitive) ne le justifie pas.
func publierTables(ctx context.Context, pool *pgxpool.Pool, reg *pipeline.Registre, tracer *tableTracer, executes []string) error {
	if len(executes) == 0 {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`DELETE FROM core.sitegen_table_usage WHERE etape = ANY($1::text[])`, executes); err != nil {
		// build-pr (restauration du seul périmètre CI, voir cmd/fpctl/
		// dump.go) ne fait jamais tourner les migrations — cette table n'y
		// existe donc jamais, par construction, puisqu'elle n'appartient
		// pas à internal/matview.Perimetre(). Rien à publier dans cet
		// environnement-là ; ce n'est jamais une raison de faire échouer la
		// construction du site elle-même.
		if strings.Contains(err.Error(), "does not exist") {
			return nil
		}
		return fmt.Errorf("nettoyage du reflet des tables lues : %w", err)
	}
	for _, nom := range executes {
		for _, table := range transitiveTables(reg, tracer, nom) {
			if _, err := tx.Exec(ctx,
				`INSERT INTO core.sitegen_table_usage (etape, table_qualifiee) VALUES ($1, $2)`,
				nom, table); err != nil {
				return fmt.Errorf("%s -> %s : %w", nom, table, err)
			}
		}
	}
	return tx.Commit(ctx)
}

// TablesPubliees lit le dernier reflet connu de core.sitegen_table_usage,
// groupé par nœud — nil, sans erreur, si la table n'existe pas encore
// (une base migrée mais jamais passée par un « fpctl build site » depuis
// cette migration) ou si elle est vide : un appelant comme fpctl list deps
// doit pouvoir dire « inconnu, lancez fpctl build site » plutôt que
// planter.
func TablesPubliees(ctx context.Context, pool *pgxpool.Pool) (map[string][]string, error) {
	rows, err := pool.Query(ctx, `SELECT etape, table_qualifiee FROM core.sitegen_table_usage ORDER BY etape, table_qualifiee`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var etape, table string
		if err := rows.Scan(&etape, &table); err != nil {
			return nil, err
		}
		out[etape] = append(out[etape], table)
	}
	return out, rows.Err()
}
