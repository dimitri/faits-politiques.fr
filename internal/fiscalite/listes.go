package fiscalite

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceListeUE = archive.Source{
	Slug: "ue-liste-juridictions-non-cooperatives", Label: "Commission européenne — historique de la liste UE des juridictions non coopératives",
	Publisher: "Commission européenne, DG TAXUD (d'après les conclusions du Conseil)", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Conditions d'utilisation du site Europa : réutilisation avec mention de la source",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Commission européenne, EU list of non-cooperative jurisdictions for tax purposes (mise à jour du 17 février 2026)",
	Cadence:     "semestrielle (février, octobre)",
	Notes: "La liste ne peut contenir aucun État membre : les critères du Conseil servent à examiner les " +
		"juridictions tierces. Composition transcrite du document scellé, version par version, et " +
		"contrôlée contre le nombre de juridictions qu'il annonce. Le document omet la révision du " +
		"17 octobre 2023, absente de la table.",
}

const listeUEURL = "https://taxation-customs.ec.europa.eu/document/download/3ceac073-5184-46a0-864a-b3038e4d9a6b_en?filename=eu_list_update_17-02-2026.pdf"

// annexeIUE : l'annexe I (liste noire) de chaque version, avec le nombre de
// juridictions que le document annonce. Transcrite à la main : le PDF est une
// frise graphique dont l'extraction de texte mêle les annexes I et II et les
// mouvements entre versions ; la vérification du nombre annoncé attrape une
// ligne oubliée.
var annexeIUE = []struct {
	date      string
	annonce   int
	juridicts []string
}{
	{"2017-12-05", 17, []string{"American Samoa", "Bahrain", "Barbados", "Republic of Korea", "United Arab Emirates", "Grenada", "Guam", "Macao SAR", "Marshall Islands", "Mongolia", "Namibia", "Palau", "Panama", "Saint Lucia", "Samoa", "Trinidad and Tobago", "Tunisia"}},
	{"2018-01-23", 9, []string{"American Samoa", "Bahrain", "Guam", "Marshall Islands", "Namibia", "Palau", "Saint Lucia", "Samoa", "Trinidad and Tobago"}},
	{"2018-03-13", 9, []string{"American Samoa", "Bahamas", "Guam", "Namibia", "Palau", "Saint Kitts and Nevis", "Samoa", "Trinidad and Tobago", "US Virgin Islands"}},
	{"2018-05-25", 7, []string{"American Samoa", "Guam", "Namibia", "Palau", "Samoa", "Trinidad and Tobago", "US Virgin Islands"}},
	{"2018-10-02", 6, []string{"American Samoa", "Guam", "Namibia", "Samoa", "Trinidad and Tobago", "US Virgin Islands"}},
	{"2018-11-06", 5, []string{"American Samoa", "Guam", "Samoa", "Trinidad and Tobago", "US Virgin Islands"}},
	{"2018-12-04", 5, []string{"American Samoa", "Guam", "Samoa", "Trinidad and Tobago", "US Virgin Islands"}},
	{"2019-03-12", 15, []string{"American Samoa", "Aruba", "Barbados", "Belize", "Bermuda", "Dominica", "Fiji", "Guam", "Marshall Islands", "Oman", "Samoa", "Trinidad and Tobago", "United Arab Emirates", "Vanuatu", "US Virgin Islands"}},
	{"2019-05-17", 12, []string{"American Samoa", "Belize", "Dominica", "Fiji", "Guam", "Marshall Islands", "Oman", "Samoa", "Trinidad and Tobago", "United Arab Emirates", "Vanuatu", "US Virgin Islands"}},
	{"2019-06-14", 11, []string{"American Samoa", "Belize", "Fiji", "Guam", "Marshall Islands", "Oman", "Samoa", "Trinidad and Tobago", "United Arab Emirates", "Vanuatu", "US Virgin Islands"}},
	{"2019-10-10", 9, []string{"American Samoa", "Belize", "Fiji", "Guam", "Oman", "Samoa", "Trinidad and Tobago", "Vanuatu", "US Virgin Islands"}},
	{"2019-11-08", 9, []string{"American Samoa", "Belize", "Fiji", "Guam", "Oman", "Samoa", "Trinidad and Tobago", "Vanuatu", "US Virgin Islands"}},
	{"2020-02-18", 12, []string{"American Samoa", "Cayman Islands", "Fiji", "Guam", "Palau", "Panama", "Samoa", "Seychelles", "Oman", "Trinidad and Tobago", "Vanuatu", "US Virgin Islands"}},
	{"2020-10-06", 12, []string{"American Samoa", "Anguilla", "Barbados", "Fiji", "Guam", "Palau", "Panama", "Samoa", "Seychelles", "Trinidad and Tobago", "Vanuatu", "US Virgin Islands"}},
	{"2021-02-22", 12, []string{"American Samoa", "Anguilla", "Dominica", "Fiji", "Guam", "Palau", "Panama", "Samoa", "Seychelles", "Trinidad and Tobago", "Vanuatu", "US Virgin Islands"}},
	{"2022-02-24", 9, []string{"American Samoa", "Fiji", "Guam", "Palau", "Panama", "Samoa", "Trinidad and Tobago", "US Virgin Islands", "Vanuatu"}},
	{"2022-10-04", 12, []string{"American Samoa", "Anguilla", "Bahamas", "Fiji", "Guam", "Palau", "Panama", "Samoa", "Trinidad and Tobago", "Turks and Caicos Islands", "US Virgin Islands", "Vanuatu"}},
	{"2023-02-14", 16, []string{"American Samoa", "Anguilla", "Bahamas", "British Virgin Islands", "Costa Rica", "Fiji", "Guam", "Marshall Islands", "Palau", "Panama", "Russian Federation", "Samoa", "Trinidad and Tobago", "Turks and Caicos Islands", "US Virgin Islands", "Vanuatu"}},
	{"2024-02-20", 12, []string{"American Samoa", "Anguilla", "Antigua and Barbuda", "Fiji", "Guam", "Palau", "Panama", "Russian Federation", "Samoa", "Trinidad and Tobago", "US Virgin Islands", "Vanuatu"}},
	{"2024-10-08", 11, []string{"American Samoa", "Anguilla", "Fiji", "Guam", "Palau", "Panama", "Russian Federation", "Samoa", "Trinidad and Tobago", "US Virgin Islands", "Vanuatu"}},
	{"2025-02-18", 11, []string{"American Samoa", "Anguilla", "Fiji", "Guam", "Palau", "Panama", "Russian Federation", "Samoa", "Trinidad and Tobago", "US Virgin Islands", "Vanuatu"}},
	{"2025-10-10", 11, []string{"American Samoa", "Anguilla", "Fiji", "Guam", "Palau", "Panama", "Russian Federation", "Samoa", "Trinidad and Tobago", "US Virgin Islands", "Vanuatu"}},
	{"2026-02-17", 10, []string{"American Samoa", "Anguilla", "Guam", "Palau", "Panama", "Russian Federation", "Turks and Caicos Islands", "US Virgin Islands", "Vanuatu", "Viet Nam"}},
}

var (
	reLigneTableau = regexp.MustCompile(`(?s)<tr>(.*?)</tr>`)
	reCellule      = regexp.MustCompile(`(?s)<td([^>]*)>(.*?)</td>`)
	reRowspan      = regexp.MustCompile(`rowspan="(\d+)"`)
	reBalise       = regexp.MustCompile(`<[^>]+>`)
)

func texteCellule(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(reBalise.ReplaceAllString(s, " "))), " ")
}

// etncDepuisJO lit, dans le corpus du Journal officiel déjà chargé, chaque
// arrêté qui fixe la liste COMPLÈTE des ETNC sous forme de tableau. Deux
// rédactions : depuis 2020, un tableau à deux colonnes (juridiction, motif),
// les motifs couvrant plusieurs lignes par rowspan ; en 2010 et 2016, une
// grille de noms sans en-tête ni motif. Les arrêtés de 2011 à 2015 ne font
// qu'ajouter ou retirer des noms dans le texte : ces versions ne sont pas
// reconstituées, pour ne pas publier une liste que le JO n'écrit pas.
func etncDepuisJO(ctx context.Context, pool *pgxpool.Pool) ([][]any, int, error) {
	rows, err := pool.Query(ctx, `
		SELECT t.id, t.date_texte, string_agg(b.contenu, E'\n' ORDER BY b.ordre)
		FROM jo.texte t JOIN jo.bloc b ON b.texte_id = t.id
		WHERE t.nature = 'ARRETE' AND t.titre_complet ILIKE '%article 238-0 A du code général des impôts%'
		GROUP BY t.id, t.date_texte
		ORDER BY t.date_texte`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out [][]any
	arretes := 0
	for rows.Next() {
		var id string
		var date time.Time
		var contenu string
		if err := rows.Scan(&id, &date, &contenu); err != nil {
			return nil, 0, err
		}
		i := strings.Index(contenu, "<table")
		if i < 0 {
			continue
		}
		j := strings.Index(contenu[i:], "</table>")
		if j < 0 {
			return nil, 0, fmt.Errorf("%s : tableau non fermé", id)
		}
		table := contenu[i : i+j]
		avecMotif := strings.Contains(table, "<th")
		motif, restant := "", 0
		vus := map[string]bool{}
		ajoute := func(nom, motif string) {
			if nom == "" || vus[nom] {
				return
			}
			vus[nom] = true
			out = append(out, []any{"ETNC_FR", date, nom, nul(motif), id})
		}
		for _, tr := range reLigneTableau.FindAllStringSubmatch(table, -1) {
			cells := reCellule.FindAllStringSubmatch(tr[1], -1)
			if len(cells) == 0 {
				continue // ligne d'en-tête (th)
			}
			if !avecMotif {
				for _, c := range cells {
					ajoute(texteCellule(c[2]), "")
				}
				continue
			}
			if len(cells) >= 2 {
				motif = texteCellule(cells[1][2])
				restant = 1
				if m := reRowspan.FindStringSubmatch(cells[1][1]); m != nil {
					fmt.Sscan(m[1], &restant)
				}
			} else if restant <= 0 {
				return nil, 0, fmt.Errorf("%s : ligne sans motif hors d'un rowspan", id)
			}
			restant--
			ajoute(texteCellule(cells[0][2]), motif)
		}
		if len(vus) > 0 {
			arretes++
		}
	}
	return out, arretes, rows.Err()
}

func nul(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func IngestListes(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	var jorfID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM raw.source WHERE slug = 'jorf'`).Scan(&jorfID); err != nil {
		return fmt.Errorf("source du corpus JORF introuvable (charger -only=jorf-complet) : %w", err)
	}
	return executer(ctx, arch, SourceListeUE, func(srcID, runID int64) (map[string]any, error) {
		f, err := arch.Fetch(ctx, srcID, runID, listeUEURL, ".pdf")
		if err != nil {
			return nil, err
		}
		var lignes [][]any
		for _, v := range annexeIUE {
			if len(v.juridicts) != v.annonce {
				return nil, fmt.Errorf("version %s : %d juridictions transcrites pour %d annoncées", v.date, len(v.juridicts), v.annonce)
			}
			d, _ := time.Parse("2006-01-02", v.date)
			for _, j := range v.juridicts {
				lignes = append(lignes, []any{"UE_ANNEXE_I", d, j, nil, nil, srcID, f.DocumentID})
			}
		}
		etnc, arretes, err := etncDepuisJO(ctx, pool)
		if err != nil {
			return nil, err
		}
		if arretes < 8 {
			return nil, fmt.Errorf("seulement %d arrêtés ETNC lus dans le corpus du JO", arretes)
		}
		for _, e := range etnc {
			lignes = append(lignes, append(e, jorfID, nil))
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE tmp_juridiction_non_cooperative (
				liste text, version date, juridiction text, motif text,
				jo_texte_id text, source_id bigint, document_id bigint
			) ON COMMIT DROP`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_juridiction_non_cooperative"},
			[]string{"liste", "version", "juridiction", "motif", "jo_texte_id", "source_id", "document_id"},
			pgx.CopyFromRows(lignes)); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			MERGE INTO ref.juridiction_non_cooperative AS tgt
			USING tmp_juridiction_non_cooperative AS src
			     ON tgt.liste = src.liste AND tgt.version = src.version AND tgt.juridiction = src.juridiction
			WHEN MATCHED AND (tgt.motif, tgt.jo_texte_id, tgt.source_id, tgt.document_id)
			     IS DISTINCT FROM (src.motif, src.jo_texte_id, src.source_id, src.document_id)
			THEN UPDATE SET motif = src.motif, jo_texte_id = src.jo_texte_id,
			     source_id = src.source_id, document_id = src.document_id
			WHEN NOT MATCHED BY TARGET THEN
			     INSERT (liste, version, juridiction, motif, jo_texte_id, source_id, document_id)
			     VALUES (src.liste, src.version, src.juridiction, src.motif, src.jo_texte_id,
			             src.source_id, src.document_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
			return nil, fmt.Errorf("fusion juridiction_non_cooperative : %w", err)
		}
		return map[string]any{"versions_ue": len(annexeIUE), "arretes_etnc": arretes, "lignes": len(lignes)}, tx.Commit(ctx)
	})
}
