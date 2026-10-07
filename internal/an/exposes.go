package an

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/balisage"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

// L'exposé des motifs est le seul texte qui dise l'objet d'une loi sans que
// nous ayons à l'écrire. L'open data parlementaire ne publie aucun résumé : un
// scrutin n'y porte qu'un libellé de procédure.
//
// Il est publié à une URL dérivable de l'identifiant du document, que nous
// détenons déjà dans raw.record :
//
//	https://www.assemblee-nationale.fr/dyn/opendata/{uid}.html
//
// Ce n'est PAS un résumé neutre : l'auteur y défend son texte. Il est stocké
// verbatim et cité comme tel ; aucune ligne de ce projet ne résume un texte à
// la place de son auteur.
var SourceExposes = archive.Source{
	Slug: "an-exposes", Label: "Assemblée nationale — exposés des motifs",
	Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
	License:     "Licence Ouverte",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Assemblée nationale, textes déposés",
	Cadence:     "au fil des dépôts",
	Notes: "Intention déclarée de l'auteur du texte, pas description neutre. " +
		"Seuls les textes déposés à l'Assemblée en portent un : un texte transmis " +
		"par le Sénat arrive sans exposé.",
}

const exposeBase = "https://www.assemblee-nationale.fr/dyn/opendata/"

// Les textes déposés à l'Assemblée, par opposition à ceux qu'elle enregistre en
// provenance du Sénat — ces derniers n'ont pas d'exposé de ce côté.
// Seule la législature en cours est servie par le point d'accès opendata : les
// documents des législatures antérieures y répondent 404. Le connecteur a
// d'abord parcouru la 13e, obtenu 190 refus et extrait zéro exposé — sans
// erreur, puisqu'un texte sans exposé est un cas normal.
var uidFiled = regexp.MustCompile(`^(PION|PRJL)ANR5L17B`)

var (
	reSpaceBefore = regexp.MustCompile(`\s+([,.;:!?])`)
	// L'exposé commence à l'un de ces marqueurs et court jusqu'au dispositif.
	reStart = regexp.MustCompile(`(?i)(EXPOSÉ DES MOTIFS|EXPOSE DES MOTIFS|Mesdames, Messieurs)`)
	reEnd   = regexp.MustCompile(`(?i)(PROPOSITION DE LOI|PROJET DE LOI|Article 1er|Article unique)`)
)

// IngestExposes récupère les exposés des textes déposés à l'Assemblée. Le débit
// est volontairement bas : c'est un site public, pas une API, et rien ne presse.
func IngestExposes(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceExposes)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	rows, err := pool.Query(ctx, `
		SELECT t.id, t.source_uid FROM core.texte t
		 WHERE t.institution = 'ASSEMBLEE_NATIONALE'
		   AND t.source_uid ~ '^(PION|PRJL)ANR5L17B'
		   AND NOT EXISTS (SELECT 1 FROM core.texte_expose e WHERE e.texte_id = t.id)
		   AND NOT EXISTS (SELECT 1 FROM core.texte_expose_verification v WHERE v.texte_id = t.id)
		 ORDER BY t.id`)
	if err != nil {
		return fail(err)
	}
	type target struct {
		id  int64
		uid string
	}
	var targets []target
	for rows.Next() {
		var c target
		if err := rows.Scan(&c.id, &c.uid); err != nil {
			rows.Close()
			return fail(err)
		}
		if uidFiled.MatchString(c.uid) {
			targets = append(targets, c)
		}
	}
	rows.Close()

	var found, missingExpose, failures, processed int64
	// verify note qu'un texte a été VÉRIFIÉ, trouvé ou non — voir la
	// migration 0177 : sans elle, missingExpose/failures redemandaient la même
	// absence à chaque passage, indéfiniment.
	verify := func(id int64, trouve bool, raison string) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO core.texte_expose_verification (texte_id, trouve, raison)
			VALUES ($1,$2,$3)
			ON CONFLICT (texte_id) DO NOTHING`, id, trouve, raison)
		return err
	}

	// exposeConcurrency : mesuré sur le passif complet de la 17e législature
	// (~2 500 textes) — une seule connexion à 400 ms d'intervalle (voir le
	// Sleep plus bas, inchangé PAR connexion) y passait près d'une heure,
	// très au-dessus du reste d'un ingest complet (fpctl ingest default tourne
	// par ailleurs jusqu'à -j connecteurs indépendants de front). Cinq
	// connexions de front restent une cadence raisonnable pour un site
	// public (12,5 req/s au total, chacune espacée des siennes par le même
	// Sleep qu'avant) sans dépendre d'une API que ce site n'offre pas.
	const exposeConcurrency = 5
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(exposeConcurrency)
	for _, c := range targets {
		c := c
		g.Go(func() error {
			defer time.Sleep(400 * time.Millisecond)
			url := exposeBase + c.uid + ".html"
			f, err := arch.Fetch(gctx, srcID, runID, url, ".html")
			if err != nil {
				// Un texte absent du site n'est pas une erreur fatale : il est
				// compté et signalé, le chargement continue.
				atomic.AddInt64(&failures, 1)
				if err := verify(c.id, false, "page inaccessible"); err != nil {
					return fmt.Errorf("%s : %w", c.uid, err)
				}
			} else if texte, ok := extractExpose(f.Path); !ok {
				atomic.AddInt64(&missingExpose, 1)
				if err := verify(c.id, false, "aucun exposé identifié dans la page"); err != nil {
					return fmt.Errorf("%s : %w", c.uid, err)
				}
			} else {
				if _, err := pool.Exec(gctx, `
					INSERT INTO core.texte_expose
					  (texte_id, source_uid, url, integral, chapeau, n_caracteres, source_id)
					VALUES ($1,$2,$3,$4,$5,$6,$7)
					ON CONFLICT (texte_id) DO NOTHING`,
					c.id, c.uid, url, texte, chapeau(texte), len([]rune(texte)), srcID); err != nil {
					return fmt.Errorf("%s : %w", c.uid, err)
				}
				if err := verify(c.id, true, ""); err != nil {
					return fmt.Errorf("%s : %w", c.uid, err)
				}
				atomic.AddInt64(&found, 1)
			}
			if n := atomic.AddInt64(&processed, 1); n%200 == 0 {
				logs.Notice(fmt.Sprintf("statements of reasons: %d/%d processed, %d found",
					n, len(targets), atomic.LoadInt64(&found)))
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"exposes": found, "sans_expose": missingExpose, "echecs": failures}, "")
	logs.Notice(fmt.Sprintf("statements of reasons: %d/%d found (%d without one, %d unreachable)",
		found, len(targets), missingExpose, failures))
	return nil
}

// extractExpose isole l'exposé entre son titre et le début du dispositif.
func extractExpose(path string) (string, bool) {
	b, err := readFile(path)
	if err != nil {
		return "", false
	}
	// Le découpage est fait par un analyseur lexical, pas par une expression
	// régulière : voir internal/balisage. Les balises de BLOC deviennent un
	// saut de ligne, les balises EN LIGNE disparaissent sans laisser d'espace —
	// les remplacer toutes par un séparateur coupait les mots au milieu :
	// « <span>M</span>esdames » donnait « M esdames », et l'artefact s'est lu
	// dans le texte publié.
	t := balisage.Text(string(b))
	// L'espace insécable avant une ponctuation double est correct en français ;
	// l'espace ordinaire avant une virgule ou un point ne l'est pas.
	t = reSpaceBefore.ReplaceAllString(t, "$1")
	lines := strings.Split(t, "\n")
	// Le marqueur est cherché sur une version APLATIE. Le titre est souvent
	// balisé mot par mot — <b>EXPOSÉ</b> DES MOTIFS — et le découpage en
	// lignes le coupait en deux : la recherche échouait sur les 131 premiers
	// textes sans que rien ne le signale, un texte sans exposé étant un cas
	// normal et non une erreur.
	flat := strings.Join(lines, " ")

	d := reStart.FindStringIndex(flat)
	if d == nil {
		return "", false
	}
	rest := flat[d[1]:]
	if f := reEnd.FindStringIndex(rest); f != nil && f[0] > 200 {
		rest = rest[:f[0]]
	}
	rest = strings.TrimSpace(rest)
	// Un exposé de moins de 200 caractères n'en est pas un : c'est une amorce
	// tronquée, et la stocker donnerait l'illusion d'une présentation.
	if len([]rune(rest)) < 200 {
		return "", false
	}
	return rest, true
}

// chapeau prend les premiers paragraphes jusqu'à une fin de phrase. C'est un
// EXTRAIT : il ne reformule rien et ne choisit pas ce qui compte.
func chapeau(texte string) string {
	r := []rune(strings.Join(strings.Fields(strings.ReplaceAll(texte, "\n", " ")), " "))
	const max = 700
	if len(r) <= max {
		return string(r)
	}
	coupe := max
	for i := max; i > max/2; i-- {
		if r[i] == '.' || r[i] == '!' || r[i] == '?' {
			coupe = i + 1
			break
		}
	}
	return strings.TrimSpace(string(r[:coupe])) + " […]"
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// ReparseExposes recalcule les textes à partir des fichiers DÉJÀ archivés, sans
// rien retélécharger. Écrit pour corriger une extraction fautive sans repasser
// trois heures sur le site de l'Assemblée — et parce que l'archive scellée est
// faite pour ça : ce qui a été récupéré une fois n'a pas à l'être deux fois.
func ReparseExposes(ctx context.Context, pool *pgxpool.Pool, racine string) error {
	rows, err := pool.Query(ctx, `
		SELECT e.texte_id, d.storage_key
		  FROM core.texte_expose e
		  JOIN raw.retrieval r ON r.url = e.url AND r.document_id IS NOT NULL
		  JOIN raw.document d ON d.id = r.document_id
		 GROUP BY e.texte_id, d.storage_key`)
	if err != nil {
		return err
	}
	type target struct {
		id  int64
		key string
	}
	var targets []target
	for rows.Next() {
		var c target
		if err := rows.Scan(&c.id, &c.key); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, c)
	}
	rows.Close()

	var batch [][]any
	for _, c := range targets {
		texte, ok := extractExpose(filepath.Join(racine, c.key))
		if !ok {
			continue
		}
		batch = append(batch, []any{c.id, texte, chapeau(texte), len([]rune(texte))})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_expose (texte_id bigint, integral text, chapeau text, n_caracteres int)
		ON COMMIT DROP`); err != nil {
		return err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_expose"},
		[]string{"texte_id", "integral", "chapeau", "n_caracteres"},
		pgx.CopyFromRows(batch)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE core.texte_expose e
		   SET integral = t.integral, chapeau = t.chapeau, n_caracteres = t.n_caracteres
		  FROM tmp_expose t
		 WHERE t.texte_id = e.texte_id`); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	logs.Notice(fmt.Sprintf("%d statements of reasons re-extracted", len(batch)))
	return nil
}
