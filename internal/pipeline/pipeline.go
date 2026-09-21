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
	"strings"
	"sync"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

// Results : ce que les dépendances déjà exécutées d'une étape ont produit,
// indexé par nom — nil pour une étape qui n'agit que par effet de bord
// (le cas de tout l'ingest aujourd'hui). internal/matview et internal/sitegen
// s'en servent pour de vraies valeurs (une matvue rafraîchie, une page
// chargée) qu'une étape dépendante lit directement au lieu de rejouer le
// calcul ou de rouvrir une connexion pour le refaire.
type Results map[string]any

// Etape : une unité nommée, ce dont elle dépend, ce qu'elle fait — et ce
// qu'elle produit, lu par ses dépendantes dans Results. Executer garde sa
// propre logique d'idempotence (comme l'ingest aujourd'hui) — ce paquet ne
// décide que DE L'ORDRE (et, en option, du parallélisme), jamais de sauter
// une étape déjà faite : c'est à l'étape elle-même de le constater vite si
// c'est le cas.
type Etape struct {
	Nom         string
	Description string
	Dependances []string
	Executer    func(ctx context.Context, deps Results) (any, error)
}

// Registre : les étapes connues, indexées par nom.
type Registre struct {
	etapes map[string]Etape
	ordre  []string // ordre de déclaration, pour un tri stable à dépendances égales
	pool   *pgxpool.Pool
}

// NouveauRegistre : pool sert à publier la topologie et l'historique
// d'exécution dans core.pipeline_etape/pipeline_dependance (voir Publier et
// Executer) — jamais à lire quoi que ce soit de la base pour décider de
// l'ordre, qui reste entièrement déterminé par le code Go.
func NouveauRegistre(pool *pgxpool.Pool) *Registre {
	return &Registre{etapes: map[string]Etape{}, pool: pool}
}

// Ajouter enregistre une étape. Panique sur un nom en double ou une
// dépendance vers une étape inconnue : ce sont des erreurs de programmation,
// pas des cas à gérer à l'exécution.
func (r *Registre) Ajouter(e Etape) {
	if _, existe := r.etapes[e.Nom]; existe {
		panic(fmt.Sprintf("pipeline : étape %q déclarée deux fois", e.Nom))
	}
	for _, d := range e.Dependances {
		if _, ok := r.etapes[d]; !ok {
			panic(fmt.Sprintf("pipeline : étape %q dépend de %q, non déclarée avant elle", e.Nom, d))
		}
	}
	r.etapes[e.Nom] = e
	r.ordre = append(r.ordre, e.Nom)
}

// Noms : tous les noms d'étapes connus, dans l'ordre de déclaration — pour
// lister les valeurs valides d'un drapeau -only sans les recopier à la main,
// ou générer une sous-commande par étape (voir cmd/fpctl).
func (r *Registre) Noms() []string {
	out := make([]string, len(r.ordre))
	copy(out, r.ordre)
	return out
}

// Etape renvoie la déclaration d'une étape par son nom (description,
// dépendances) — pour fpctl list deps et la génération des sous-commandes.
func (r *Registre) Etape(nom string) (Etape, bool) {
	e, ok := r.etapes[nom]
	return e, ok
}

// fermeture calcule, par DFS, l'ensemble des étapes nécessaires aux cibles
// demandées (elles-mêmes comprises) — détecte aussi les cycles et les noms
// inconnus, avant tout calcul de niveaux.
func (r *Registre) fermeture(cibles []string) (map[string]bool, error) {
	besoin := map[string]bool{}
	visite := map[string]int{} // 0 = jamais vu, 1 = en cours (cycle), 2 = fait
	var visiter func(nom string, chemin []string) error
	visiter = func(nom string, chemin []string) error {
		switch visite[nom] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("dépendance cyclique : %v -> %s", chemin, nom)
		}
		e, ok := r.etapes[nom]
		if !ok {
			return fmt.Errorf("étape inconnue : %s", nom)
		}
		visite[nom] = 1
		for _, d := range e.Dependances {
			if err := visiter(d, append(chemin, nom)); err != nil {
				return err
			}
		}
		visite[nom] = 2
		besoin[nom] = true
		return nil
	}
	for _, c := range cibles {
		if err := visiter(c, nil); err != nil {
			return nil, err
		}
	}
	return besoin, nil
}

// Niveaux regroupe les étapes nécessaires aux cibles demandées en VAGUES :
// chaque vague ne dépend que des vagues précédentes, donc les étapes d'une
// même vague sont indépendantes entre elles et peuvent s'exécuter en
// parallèle (voir Executer, Concurrence). L'ordre des étapes DANS une vague
// suit l'ordre de déclaration, pour un plan reproductible d'un lancement à
// l'autre.
func (r *Registre) Niveaux(cibles []string) ([][]string, error) {
	besoin, err := r.fermeture(cibles)
	if err != nil {
		return nil, err
	}
	fait := map[string]bool{}
	var niveaux [][]string
	for len(fait) < len(besoin) {
		var vague []string
		for _, nom := range r.ordre { // ordre de déclaration : un plan stable
			if !besoin[nom] || fait[nom] {
				continue
			}
			pret := true
			for _, d := range r.etapes[nom].Dependances {
				if besoin[d] && !fait[d] {
					pret = false
					break
				}
			}
			if pret {
				vague = append(vague, nom)
			}
		}
		if len(vague) == 0 {
			// La fermeture a déjà écarté les cycles ; ne devrait jamais arriver.
			return nil, fmt.Errorf("pipeline : aucune étape prête alors qu'il en reste — incohérence interne")
		}
		for _, nom := range vague {
			fait[nom] = true
		}
		niveaux = append(niveaux, vague)
	}
	return niveaux, nil
}

// Options d'exécution. Concurrence <= 1 : séquentiel (comportement par
// défaut, celui qu'avait ce paquet avant) — le parallélisme est une
// optimisation qu'on choisit, jamais une surprise sur un pipeline qui
// partage un pool de connexions et des points d'accès externes rate-limités.
type Options struct {
	DryRun      bool
	Concurrence int
}

// Executer résout les dépendances des cibles demandées et exécute chaque
// étape nécessaire, une fois, vague par vague — les prérequis
// silencieusement oubliés deviennent structurellement impossibles plutôt
// que découverts un par un, en production, à la lecture d'un message
// d'erreur sans rapport avec ce qui manque réellement. À l'intérieur d'une
// vague, jusqu'à Concurrence étapes tournent de front ; dès qu'une échoue,
// le contexte des autres est annulé et aucune vague suivante ne démarre.
//
// Le Results renvoyé porte ce que chaque étape exécutée a produit — vide
// (valeurs nil) pour un registre dont les étapes n'agissent que par effet
// de bord, comme l'ingest.
func (r *Registre) Executer(ctx context.Context, cibles []string, opts ...Options) (Results, error) {
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	niveaux, err := r.Niveaux(cibles)
	if err != nil {
		return nil, err
	}
	if opt.DryRun {
		r.afficherPlan(niveaux, opt.Concurrence)
		return nil, nil
	}
	limite := opt.Concurrence
	if limite < 1 {
		limite = 1
	}

	resultats := Results{}
	var mu sync.Mutex
	for _, vague := range niveaux {
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(limite)
		for _, nom := range vague {
			nom := nom
			e := r.etapes[nom]
			// Construit avant de lancer la vague, pas depuis la goroutine :
			// les vagues précédentes sont déjà entièrement écrites
			// (g.Wait() ci-dessous s'en assure), donc cette lecture n'a pas
			// besoin de mu — seules les ÉCRITURES concurrentes dans une même
			// vague en ont besoin.
			deps := Results{}
			for _, d := range e.Dependances {
				deps[d] = resultats[d]
			}
			g.Go(func() error {
				// logs.Notice, pas fmt.Printf : plusieurs étapes de la même
				// vague narrent de front (Concurrence > 1), et internal/logs
				// sait déjà sérialiser proprement des écritures concurrentes
				// sur le même stderr (voir internal/logs/lock.go) — un mutex
				// posé ici ferait la même chose en moins bien.
				logs.Notice(e.Description, "etape", nom)
				valeur, err := e.Executer(gctx, deps)
				if err != nil {
					return fmt.Errorf("%s : %w", nom, err)
				}
				mu.Lock()
				resultats[nom] = valeur
				mu.Unlock()
				if r.pool != nil {
					if _, err := r.pool.Exec(gctx,
						`UPDATE core.pipeline_etape SET derniere_execution_reussie = now() WHERE nom = $1`,
						nom); err != nil {
						return fmt.Errorf("%s : journal d'exécution : %w", nom, err)
					}
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return nil, err
		}
	}
	return resultats, nil
}

func (r *Registre) afficherPlan(niveaux [][]string, concurrence int) {
	fmt.Println("simulation (rien n'est exécuté) :")
	for i, vague := range niveaux {
		var desc []string
		for _, nom := range vague {
			desc = append(desc, nom)
		}
		parallele := ""
		if concurrence > 1 && len(vague) > 1 {
			parallele = fmt.Sprintf(" (jusqu'à %d en parallèle)", min(concurrence, len(vague)))
		}
		fmt.Printf("  %d. %s%s\n", i+1, strings.Join(desc, ", "), parallele)
	}
}

// Publier réécrit la topologie déclarée dans core.pipeline_etape et
// core.pipeline_dependance — un reflet, jamais la source de vérité, qui
// reste le code Go. Une étape retirée du registre (donc absente de cet
// appel) disparaît de la base par la même occasion : ON DELETE CASCADE
// emporte ses dépendances avec elle. derniere_execution_reussie n'est
// jamais touchée ici, seulement par Executer.
func (r *Registre) Publier(ctx context.Context) error {
	if r.pool == nil {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	noms := r.Noms()
	if _, err := tx.Exec(ctx,
		`DELETE FROM core.pipeline_etape WHERE nom <> ALL($1::text[])`, noms); err != nil {
		return fmt.Errorf("nettoyage des étapes disparues : %w", err)
	}
	for _, nom := range noms {
		e := r.etapes[nom]
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.pipeline_etape (nom, description) VALUES ($1, $2)
			ON CONFLICT (nom) DO UPDATE SET description = excluded.description`,
			nom, e.Description); err != nil {
			return fmt.Errorf("%s : %w", nom, err)
		}
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM core.pipeline_dependance WHERE etape = ANY($1::text[])`, noms); err != nil {
		return fmt.Errorf("nettoyage des dépendances : %w", err)
	}
	for _, nom := range noms {
		for _, dep := range r.etapes[nom].Dependances {
			if _, err := tx.Exec(ctx,
				`INSERT INTO core.pipeline_dependance (etape, depend_de) VALUES ($1, $2)`,
				nom, dep); err != nil {
				return fmt.Errorf("%s -> %s : %w", nom, dep, err)
			}
		}
	}
	return tx.Commit(ctx)
}
