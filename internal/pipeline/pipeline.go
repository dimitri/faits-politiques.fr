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
// étape nécessaire, une fois — les prérequis silencieusement oubliés
// deviennent structurellement impossibles plutôt que découverts un par un,
// en production, à la lecture d'un message d'erreur sans rapport avec ce qui
// manque réellement.
//
// Le répartiteur est continu, pas vague par vague (Niveaux reste utile pour
// LIRE le plan — afficherPlan, fpctl ingest --dry-run — mais ne gouverne
// plus l'exécution) : une étape part dès que SES PROPRES dépendances sont
// faites, jamais en attendant que tout le reste de sa vague nominale ait
// fini. Deux étapes réelles de ce dépôt le montrent : "media" ne dépend que
// de "carto" (rapide), pas de "communes" (9+ minutes) — les deux tombaient
// pourtant dans la même vague nominale que "communes", et media attendait
// sa fin pour rien. Jusqu'à Concurrence étapes tournent de front, au total,
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

	// besoin/priorite : la fermeture déjà calculée par Niveaux, aplatie —
	// priorite rejoue l'ordre de déclaration pour départager deux étapes
	// prêtes en même temps, un plan reproductible d'un lancement à l'autre
	// (même but que l'ordre DANS une vague avant cette réécriture).
	var ordreBesoin []string
	priorite := map[string]int{}
	for _, vague := range niveaux {
		for _, nom := range vague {
			priorite[nom] = len(ordreBesoin)
			ordreBesoin = append(ordreBesoin, nom)
		}
	}

	// dependants/restants : le graphe inverse des Dependances, et pour
	// chaque étape le nombre de prérequis pas encore terminés — une étape
	// rejoint pret dès que son restants tombe à zéro.
	dependants := map[string][]string{}
	restants := map[string]int{}
	for _, nom := range ordreBesoin {
		for _, d := range r.etapes[nom].Dependances {
			if _, besoin := priorite[d]; besoin {
				restants[nom]++
				dependants[d] = append(dependants[d], nom)
			}
		}
	}
	var pret []string
	for _, nom := range ordreBesoin {
		if restants[nom] == 0 {
			pret = append(pret, nom)
		}
	}

	resultats := Results{}
	durees := map[string]time.Duration{}
	var mu sync.Mutex
	debutTotal := time.Now()

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limite)

	type arrivee struct {
		nom string
		err error
	}
	// Bufferisé à la taille de la fermeture entière : un envoi n'attend
	// jamais que le répartiteur soit disponible pour le lire, qu'il soit
	// occupé à lancer d'autres étapes ou bloqué dans g.Go (voir lancer) —
	// seul le sémaphore interne de g borne le nombre de goroutines en vol.
	arrivees := make(chan arrivee, len(ordreBesoin))

	lancer := func(nom string) {
		e := r.etapes[nom]
		// Lu sous mu : contrairement à l'ancien découpage par vagues (où
		// g.Wait() garantissait la vague précédente entièrement écrite
		// avant que celle-ci ne lise resultats), une étape peut désormais
		// démarrer pendant que d'autres, sans rapport, sont encore en
		// cours d'écriture — la même carte résultats est donc lue ET
		// écrite en concurrence, ce qui réclame le même mutex des deux
		// côtés.
		mu.Lock()
		deps := Results{}
		for _, d := range e.Dependances {
			deps[d] = resultats[d]
		}
		mu.Unlock()
		g.Go(func() error {
			// logs.Notice, pas fmt.Printf : plusieurs étapes sans rapport de
			// dépendance tournent de front, et internal/logs sait déjà
			// sérialiser proprement des écritures concurrentes sur le même
			// stderr (voir internal/logs/lock.go) — un mutex posé ici ferait
			// la même chose en moins bien.
			logs.Notice(e.Description)
			debut := time.Now()
			valeur, err := e.Executer(gctx, deps)
			duree := time.Since(debut)
			if err != nil {
				err = fmt.Errorf("%s (après %s) : %w", nom, duree.Round(time.Millisecond), err)
				arrivees <- arrivee{nom, err}
				return err
			}
			// Le départ de chaque étape se lit déjà dans les logs (NOTICE
			// ci-dessus, horodaté par internal/logs) — sans cette ligne
			// d'arrivée, retrouver COMBIEN de temps une étape a pris
			// demandait de recouper deux horodatages à la main, impossible
			// dès que plusieurs étapes tournent de front (leurs lignes
			// s'entrelacent). Les deux bornes dans le même paquet, jamais
			// recalculées ailleurs.
			logs.Notice(fmt.Sprintf("%s : terminé en %s", nom, duree.Round(time.Millisecond)))
			mu.Lock()
			resultats[nom] = valeur
			durees[nom] = duree
			mu.Unlock()
			if r.pool != nil {
				if _, err := r.pool.Exec(gctx,
					`UPDATE core.pipeline_etape SET derniere_execution_reussie = now() WHERE nom = $1`,
					nom); err != nil {
					err = fmt.Errorf("%s : journal d'exécution : %w", nom, err)
					arrivees <- arrivee{nom, err}
					return err
				}
			}
			arrivees <- arrivee{nom, nil}
			return nil
		})
	}

	// La boucle de répartition elle-même : enVol compte les étapes lancées
	// dont l'arrivée n'est pas encore lue ici, et borne elle-même le
	// nombre d'appels à g.Go (jamais au-delà de limite) — PAS g.Go/son
	// sémaphore. Lui laisser bloquer reviendrait à pouvoir lancer une
	// (limite+1)-ième étape avant d'avoir lu l'échec éventuel de l'une des
	// limite déjà en vol (le sémaphore se libère dès qu'une goroutine
	// revient, pas quand CE répartiteur a lu son arrivee) : un échec
	// pourrait alors laisser partir une étape qui n'aurait jamais dû
	// démarrer. En ne faisant jamais patienter le répartiteur à
	// l'intérieur de g.Go, chaque échec est vu avant toute nouvelle
	// répartition.
	arret := false
	enVol := 0
	for {
		if !arret {
			for len(pret) > 0 && enVol < limite {
				idx := 0
				for i := 1; i < len(pret); i++ {
					if priorite[pret[i]] < priorite[pret[idx]] {
						idx = i
					}
				}
				nom := pret[idx]
				pret = append(pret[:idx], pret[idx+1:]...)
				lancer(nom)
				enVol++
			}
		}
		if enVol == 0 {
			break
		}
		a := <-arrivees
		enVol--
		if a.err != nil {
			// Ni ses dépendantes (qui ne peuvent de toute façon jamais
			// devenir prêtes : restants ne descend que sur succès) ni le
			// reste du graphe non encore lancé ne démarrent — la même
			// garantie qu'avant, au niveau du graphe entier plutôt que
			// vague par vague.
			arret = true
			continue
		}
		for _, dep := range dependants[a.nom] {
			restants[dep]--
			if restants[dep] == 0 {
				pret = append(pret, dep)
			}
		}
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	afficherDurees(durees, time.Since(debutTotal))
	return resultats, nil
}

// afficherDurees : un résumé trié par coût décroissant, pour répondre tout
// de suite à « qu'est-ce qui a pris du temps ? » sans recouper des lignes
// NOTICE entrelacées à la main — surtout utile sous Concurrence > 1, où
// l'ordre d'apparition dans les logs ne reflète plus l'ordre de déclaration
// ni le coût réel. Les dix étapes les plus lentes suffisent : le but est de
// repérer un goulot, pas de remplacer un vrai profil (pprof) si le besoin
// allait plus loin que ça.
func afficherDurees(durees map[string]time.Duration, totalMur time.Duration) {
	if len(durees) == 0 {
		return
	}
	type ligne struct {
		nom   string
		duree time.Duration
	}
	tri := make([]ligne, 0, len(durees))
	var somme time.Duration
	for nom, d := range durees {
		tri = append(tri, ligne{nom, d})
		somme += d
	}
	sort.Slice(tri, func(i, j int) bool { return tri[i].duree > tri[j].duree })
	n := len(tri)
	if n > 10 {
		n = 10
	}
	logs.Notice(fmt.Sprintf("étapes les plus coûteuses (somme des étapes : %s, mur : %s, %d étapes) :",
		somme.Round(time.Second), totalMur.Round(time.Second), len(tri)))
	for _, l := range tri[:n] {
		logs.Notice(fmt.Sprintf("  %s : %s", l.nom, l.duree.Round(time.Millisecond)))
	}
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
