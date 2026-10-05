package immigration

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceTitresSejour = archive.Source{
	Slug: "dgef-titres-sejour-stocks", Label: "DGEF/MIOM — stock de titres de séjour valides",
	Publisher: "Direction générale des étrangers en France, ministère de l'Intérieur", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : ministère de l'Intérieur, DGEF-DSED",
	Cadence:     "semestrielle, arrêtée",
	Notes: "Champ : ressortissants de pays tiers, hors Britanniques (suivis à part depuis " +
		"le Brexit). Seul le tableau des stocks par zone est chargé : le détail par motif " +
		"mélange plusieurs tableaux, des années « (provisoire) »/« (définitif) » et des " +
		"notes de bas de page dans un même fichier CSV, une structure trop instable pour " +
		"un connecteur fiable — voir docs/immigration-donnees.md.",
}

const titresSejourURL = "https://static.data.gouv.fr/resources/titres-de-sejour-publication-du-27-juin-2024/" +
	"20240730-112233/les-titres-de-sejour-au-27-juin-2024-stocks.csv"

// latin1VersUTF8 décode un fichier ISO-8859-1 (l'encodage par défaut de la
// plupart des exports de l'administration française) sans dépendance externe :
// chaque octet 0-255 de Latin-1 désigne EXACTEMENT le point de code Unicode de
// même valeur, la conversion est donc une simple relecture octet par octet.
func latin1VersUTF8(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	runes := make([]rune, len(b))
	for i, c := range b {
		runes[i] = rune(c)
	}
	return string(runes), nil
}

func IngestTitresSejour(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceTitresSejour)
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

	f, err := arch.Fetch(ctx, srcID, runID, titresSejourURL, ".csv")
	if err != nil {
		return fail(err)
	}
	// Fichier en Latin-1 (ISO-8859-1), comme la plupart des exports de
	// l'administration française : décodé au vol, pas dans un fichier
	// intermédiaire, pour ne pas dupliquer l'archive scellée.
	raw, err := latin1VersUTF8(f.Path)
	if err != nil {
		return fail(err)
	}

	// Le premier tableau du fichier (voir la constante du commentaire de
	// SourceTitresSejour) : une ligne d'années, puis une ligne par zone. Le
	// nombre de tabulations qui précèdent la première valeur varie d'une ligne
	// à l'autre (0, 1 ou 2 selon la longueur du libellé) : on ne se fie donc
	// PAS à la position des champs, seulement à l'ORDRE des champs non vides.
	lignes := strings.Split(raw, "\n")
	champsUtiles := func(l string) []string {
		var out []string
		for _, c := range strings.Split(strings.TrimRight(l, "\r"), "\t") {
			if c = strings.TrimSpace(c); c != "" {
				out = append(out, c)
			}
		}
		return out
	}

	var annees []int
	for _, l := range lignes {
		champs := champsUtiles(l)
		if len(champs) < 5 {
			continue
		}
		var candidats []int
		ok := true
		for _, c := range champs {
			a, err := strconv.Atoi(c)
			if err != nil || a < 2000 || a > 2100 {
				ok = false
				break
			}
			candidats = append(candidats, a)
		}
		if ok {
			annees = candidats
			break
		}
	}
	if len(annees) == 0 {
		return fail(fmt.Errorf("ligne d'années introuvable"))
	}

	zoneCode := map[string]string{"France métropolitaine": "METROPOLE", "DOM": "DOM", "COM": "COM"}
	var rows [][]any
	for _, l := range lignes {
		champs := champsUtiles(l)
		if len(champs) < 2 {
			continue
		}
		code, ok := zoneCode[champs[0]]
		if !ok {
			continue
		}
		valeurs := champs[1:]
		for i, a := range annees {
			if i >= len(valeurs) {
				break
			}
			v, err := strconv.Atoi(valeurs[i])
			if err != nil {
				continue
			}
			rows = append(rows, []any{a, code, v, srcID})
		}
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune zone reconnue (France métropolitaine, DOM, COM)"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_titre_sejour_stock (
			annee smallint, zone text, effectif int, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_titre_sejour_stock"},
		[]string{"annee", "zone", "effectif", "source_id"}, pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("titre_sejour_stock : %w", err))
	}

	// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire de
	// la table ; l'ancien DELETE payait le prix des triggers RI pour
	// l'intégralité de la table à chaque republication, changement ou non.
	ct, err := tx.Exec(ctx, `
		MERGE INTO core.titre_sejour_stock AS tgt
		USING tmp_titre_sejour_stock AS src
		ON tgt.annee = src.annee AND tgt.zone = src.zone
		WHEN MATCHED AND (tgt.effectif, tgt.source_id) IS DISTINCT FROM (src.effectif, src.source_id) THEN
		    UPDATE SET effectif = src.effectif, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, zone, effectif, source_id)
		    VALUES (src.annee, src.zone, src.effectif, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion titre_sejour_stock : %w", err))
	}
	touchees := ct.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes_chargees": touchees, "annees": len(annees)}, "")
	fmt.Printf("  stock de titres de séjour : %d lignes touchées, %d à %d\n",
		touchees, annees[0], annees[len(annees)-1])
	return nil
}
