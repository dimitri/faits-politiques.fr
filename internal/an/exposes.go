package an

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/balisage"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	Licence:     "Licence Ouverte",
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
var uidDepose = regexp.MustCompile(`^(PION|PRJL)ANR5L17B`)

var (
	reEspaceAvant = regexp.MustCompile(`\s+([,.;:!?])`)
	// L'exposé commence à l'un de ces marqueurs et court jusqu'au dispositif.
	reDebut = regexp.MustCompile(`(?i)(EXPOSÉ DES MOTIFS|EXPOSE DES MOTIFS|Mesdames, Messieurs)`)
	reFin   = regexp.MustCompile(`(?i)(PROPOSITION DE LOI|PROJET DE LOI|Article 1er|Article unique)`)
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
		 ORDER BY t.id`)
	if err != nil {
		return fail(err)
	}
	type cible struct {
		id  int64
		uid string
	}
	var cibles []cible
	for rows.Next() {
		var c cible
		if err := rows.Scan(&c.id, &c.uid); err != nil {
			rows.Close()
			return fail(err)
		}
		if uidDepose.MatchString(c.uid) {
			cibles = append(cibles, c)
		}
	}
	rows.Close()

	var trouves, sansExpose, echecs int
	for i, c := range cibles {
		url := exposeBase + c.uid + ".html"
		f, err := arch.Fetch(ctx, srcID, runID, url, ".html")
		if err != nil {
			// Un texte absent du site n'est pas une erreur fatale : il est
			// compté et signalé, le chargement continue.
			echecs++
			continue
		}
		texte, ok := extraireExpose(f.Path)
		if !ok {
			sansExpose++
			continue
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO core.texte_expose
			  (texte_id, source_uid, url, integral, chapeau, n_caracteres, source_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (texte_id) DO NOTHING`,
			c.id, c.uid, url, texte, chapeau(texte), len([]rune(texte)), srcID); err != nil {
			return fail(fmt.Errorf("%s : %w", c.uid, err))
		}
		trouves++
		if (i+1)%200 == 0 {
			logs.Notice("exposés en cours", "traites", i+1, "total", len(cibles), "exposes", trouves)
		}
		// Un site public n'est pas une API : une requête toutes les 400 ms.
		time.Sleep(400 * time.Millisecond)
	}

	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"exposes": trouves, "sans_expose": sansExpose, "echecs": echecs}, "")
	logs.Notice("exposés des motifs", "recuperes", trouves, "total", len(cibles),
		"sans_expose", sansExpose, "echecs", echecs)
	return nil
}

// extraireExpose isole l'exposé entre son titre et le début du dispositif.
func extraireExpose(path string) (string, bool) {
	b, err := lireFichier(path)
	if err != nil {
		return "", false
	}
	// Le découpage est fait par un analyseur lexical, pas par une expression
	// régulière : voir internal/balisage. Les balises de BLOC deviennent un
	// saut de ligne, les balises EN LIGNE disparaissent sans laisser d'espace —
	// les remplacer toutes par un séparateur coupait les mots au milieu :
	// « <span>M</span>esdames » donnait « M esdames », et l'artefact s'est lu
	// dans le texte publié.
	t := balisage.Texte(string(b))
	// L'espace insécable avant une ponctuation double est correct en français ;
	// l'espace ordinaire avant une virgule ou un point ne l'est pas.
	t = reEspaceAvant.ReplaceAllString(t, "$1")
	lignes := strings.Split(t, "\n")
	// Le marqueur est cherché sur une version APLATIE. Le titre est souvent
	// balisé mot par mot — <b>EXPOSÉ</b> DES MOTIFS — et le découpage en
	// lignes le coupait en deux : la recherche échouait sur les 131 premiers
	// textes sans que rien ne le signale, un texte sans exposé étant un cas
	// normal et non une erreur.
	plat := strings.Join(lignes, " ")

	d := reDebut.FindStringIndex(plat)
	if d == nil {
		return "", false
	}
	reste := plat[d[1]:]
	if f := reFin.FindStringIndex(reste); f != nil && f[0] > 200 {
		reste = reste[:f[0]]
	}
	reste = strings.TrimSpace(reste)
	// Un exposé de moins de 200 caractères n'en est pas un : c'est une amorce
	// tronquée, et la stocker donnerait l'illusion d'une présentation.
	if len([]rune(reste)) < 200 {
		return "", false
	}
	return reste, true
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

func lireFichier(path string) ([]byte, error) {
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
	type cible struct {
		id  int64
		key string
	}
	var cibles []cible
	for rows.Next() {
		var c cible
		if err := rows.Scan(&c.id, &c.key); err != nil {
			rows.Close()
			return err
		}
		cibles = append(cibles, c)
	}
	rows.Close()

	var lignes [][]any
	for _, c := range cibles {
		texte, ok := extraireExpose(filepath.Join(racine, c.key))
		if !ok {
			continue
		}
		lignes = append(lignes, []any{c.id, texte, chapeau(texte), len([]rune(texte))})
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
		pgx.CopyFromRows(lignes)); err != nil {
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
	logs.Notice("exposés réextraits", "count", len(lignes))
	return nil
}
