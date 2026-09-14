// Package prefets charge la représentation de l'État dans les départements.
//
// Il est à part de core.mandate, et cette séparation est le point de conception
// du paquet : core.mandate ne contient que des mandats ÉLECTIFS. Un préfet est
// nommé par décret en conseil des ministres et révocable à tout moment. Les
// ranger ensemble ferait perdre la seule chose que core.mandate affirme avec
// certitude — que quelqu'un a été élu.
package prefets

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "prefets-v1"

// Les exports CSV de l'administration commencent souvent par une marque
// d'ordre des octets, qui colle au premier nom de colonne et le rend
// introuvable. On la retire plutôt que de renommer la colonne.
const bom = "\ufeff"

var Source = archive.Source{
	Slug: "prefets-archives-nationales", Label: "Préfets et préfètes français depuis 1800",
	Publisher: "Ministère de la Culture / Archives nationales", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : Archives nationales, préfets et préfètes français depuis 1800",
	Cadence:     "irrégulière",
	Notes: "Alimentée par les versements aux Archives, faits environ six ans après " +
		"la clôture du dossier de carrière : la série est donc en retard sur le " +
		"présent, même si elle contient des postes ouverts. Les dates sont " +
		"connues à l'année, pas au jour. Aucune couleur politique n'est publiée " +
		"et il n'en est pas déduit.",
}

const URL = "https://static.data.gouv.fr/resources/prefets-et-prefetes-francais-depuis-1800/20251013-193828/od-prefets-et-prefetes-francais-depuis-1800.csv"

// Un segment de carrière : « 1928-1929 : Territoire de Belfort ». La fin peut
// être ouverte — « 2023-.... » — pour un préfet en poste, ou incertaine —
// « 1815-[1819] » — quand l'archiviste n'a pas pu la fixer. Les trois cas sont
// distingués : une borne inventée vaudrait pire qu'une borne absente.
var reSegment = regexp.MustCompile(`^(\d{4})\s*-\s*(\d{4}|\[\d{4}\]|[.…]+)\s*:\s*(.+)$`)

// « 90 - Territoire de Belfort » : le code du département précède son nom dans
// la colonne des postes, mais pas dans celle des dates. On construit la
// correspondance ligne par ligne.
var rePoste = regexp.MustCompile(`^\s*(\w+)\s*-\s*(.+?)\s*$`)

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, Source)
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

	f, err := arch.Fetch(ctx, srcID, runID, URL, ".csv")
	if err != nil {
		return fail(err)
	}
	fh, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer fh.Close()
	r := csv.NewReader(fh)
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return fail(err)
	}
	if len(recs) < 2 {
		return fail(fmt.Errorf("fichier des préfets vide"))
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[strings.TrimSpace(strings.TrimPrefix(h, bom))] = i
	}
	champ := func(rec []string, nom string) string {
		if i, ok := col[nom]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	var rows [][]any
	var recus, rejetSansDate, rejetIllisible, rejetBornesInversees int
	for _, rec := range recs[1:] {
		nom := champ(rec, "Nom de famille")
		prenom := champ(rec, "Prénom(s)")
		wikidata := champ(rec, "Wikidata")
		naiss := anneeOuNil(champ(rec, "Année de naissance"))
		deces := anneeOuNil(champ(rec, "Année de mort"))

		codes := map[string]string{}
		for _, p := range strings.Split(champ(rec, "Poste en département"), "|") {
			if m := rePoste.FindStringSubmatch(p); m != nil {
				codes[m[2]] = m[1]
			}
		}

		dates := champ(rec, "Poste en département (dates)")
		if dates == "" {
			rejetSansDate++
			continue
		}
		for _, s := range strings.Split(dates, "|") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			recus++
			m := reSegment.FindStringSubmatch(s)
			if m == nil {
				rejetIllisible++
				continue
			}
			debut, _ := strconv.Atoi(m[1])
			poste := strings.TrimSpace(m[3])
			// daterange semi-ouvert : la fin exclusive est le 1er janvier de
			// l'année SUIVANT la dernière année d'exercice, faute de quoi un
			// préfet parti en 1929 n'aurait pas exercé en 1929.
			// Une fin antérieure au début est une coquille de la source : le
			// fichier contient « 1810-1015 : Mont-Blanc ». On la rejette sous
			// un motif nommé plutôt que de deviner l'année réelle — corriger
			// 1015 en 1815 serait plausible, et ce serait inventé.
			var fin any
			switch {
			case regexp.MustCompile(`^\d{4}$`).MatchString(m[2]):
				a, _ := strconv.Atoi(m[2])
				fin = fmt.Sprintf("%04d-01-01", a+1)
			case strings.HasPrefix(m[2], "["):
				a, _ := strconv.Atoi(strings.Trim(m[2], "[]"))
				fin = fmt.Sprintf("%04d-01-01", a+1)
			default:
				fin = nil // poste ouvert
			}
			if s, ok := fin.(string); ok && s <= fmt.Sprintf("%04d-01-01", debut) {
				rejetBornesInversees++
				continue
			}
			var code any
			if c, ok := codes[poste]; ok {
				code = c
			}
			rows = append(rows, []any{
				nom, nilSiVide(prenom), poste, code,
				fmt.Sprintf("%04d-01-01", debut), fin,
				naiss, deces, nilSiVide(wikidata), srcID,
			})
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `TRUNCATE core.prefet`); err != nil {
		return fail(err)
	}
	// daterange se construit en SQL : pgx n'a pas de type Go naturel pour lui,
	// et passer par une table temporaire évite 8 600 allers-retours.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE prefet_in (nom text, prenom text, poste text, code text,
		  debut date, fin date, naissance smallint, deces smallint, wikidata text,
		  source_id bigint) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"prefet_in"},
		[]string{"nom", "prenom", "poste", "code", "debut", "fin", "naissance",
			"deces", "wikidata", "source_id"}, pgx.CopyFromRows(rows)); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO core.prefet (nom, prenom, poste, code_departement, validity,
		                         precision_dates, annee_naissance, annee_deces,
		                         wikidata, source_id)
		SELECT nom, prenom, poste, code, daterange(debut, fin, '[)'), 'ANNEE',
		       naissance, deces, wikidata, source_id
		  FROM prefet_in`); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"lignes_recues":           recus,
		"lignes_chargees":         len(rows),
		"rejet_segment_illisible": rejetIllisible,
		"rejet_bornes_inversees":  rejetBornesInversees,
		// Hors du décompte `rejet_*` À DESSEIN : ce compteur est en PERSONNES,
		// les autres en segments. Une personne sans dates n'apporte aucun
		// segment reçu, donc l'ajouter à la somme fausserait l'invariant —
		// c'est exactement ce que la sonde de cmd/verify a détecté ici.
		"personnes":            len(recs) - 1,
		"personnes_sans_dates": rejetSansDate,
	}, "")
	fmt.Printf("  préfets : %d personnes, %d périodes d'exercice\n", len(recs)-1, len(rows))
	return nil
}

func anneeOuNil(s string) any {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 1600 && n < 2200 {
		return int16(n)
	}
	return nil
}

func nilSiVide(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
