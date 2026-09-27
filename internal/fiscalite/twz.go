package fiscalite

import (
	"context"
	"fmt"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceMissingProfits = archive.Source{
	Slug: "missing-profits-twz-wz", Label: "Tørsløv, Wier, Zucman — estimations du transfert de bénéfices vers les paradis fiscaux",
	Publisher: "T. Tørsløv, L. Wier, G. Zucman (missingprofits.world)", Tier: "SECONDARY_PRESS",
	Licence:     "Aucune licence déclarée ; données de réplication publiées par les auteurs",
	ReuseClass:  "RESTRICTED",
	Attribution: "Source : Tørsløv, Wier & Zucman (2023), The Missing Profits of Nations, Review of Economic Studies ; Wier & Zucman (2022), Global profit shifting 1975-2019, WIDER",
	Cadence:     "ponctuelle (mises à jour des auteurs)",
	Notes: "Estimations, pas mesures : le bénéfice « transféré » est déduit de la rentabilité anormale " +
		"des filiales étrangères dans les juridictions désignées comme paradis fiscaux, réparti ensuite " +
		"entre pays d'origine d'après les paiements intragroupe. Hypothèses discutées (liste des paradis, " +
		"clé de répartition) ; les auteurs publient bornes basse et haute. Valeurs en milliards de dollars " +
		"courants. Aucune licence : citées avec attribution, pas redistribuées en masse.",
}

const (
	urlTWZ2022 = "https://missingprofits.world/wp-content/uploads/2022/04/TWZ2022.xlsx"
	urlWZ2022  = "https://missingprofits.world/wp-content/uploads/2022/11/WZ2022.xlsb.xlsx"
)

func nombreCellule(r map[string]string, col string) (float64, bool) {
	s, ok := r[col]
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func IngestTWZ(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, arch, SourceMissingProfits, func(srcID, runID int64) (map[string]any, error) {
		type cle struct {
			pays string
			an   int
			ind  string
		}
		vus := map[cle]bool{}
		var lignes [][]any
		doublons := 0
		ajoute := func(pays string, an int, ind string, v float64, unite string, doc int64) {
			k := cle{pays, an, ind}
			if vus[k] {
				// Le classeur WZ2022 répète une ligne « India » (la seconde
				// porte les valeurs de l'Afrique du Sud) : la première fait foi.
				doublons++
				return
			}
			vus[k] = true
			lignes = append(lignes, []any{pays, an, ind, v, unite, srcID, doc})
		}

		// WZ2022, Table A : bénéfices transférés (Md$ ; positif = perdus par le
		// pays, négatif = reçus par le paradis) et perte ou gain d'impôt sur les
		// sociétés en part de l'impôt collecté, 2015 à 2019.
		f, err := arch.Fetch(ctx, srcID, runID, urlWZ2022, ".xlsx")
		if err != nil {
			return nil, err
		}
		x, err := openXLSX(f.Path)
		if err != nil {
			return nil, err
		}
		rows, err := x.rows("Table A")
		x.Close()
		if err != nil {
			return nil, err
		}
		annees := []struct {
			an              int
			colMd, colPerte string
		}{{2015, "C", "J"}, {2016, "D", "K"}, {2017, "E", "L"}, {2018, "F", "M"}, {2019, "G", "N"}}
		if rows[2]["C"] != "2015" || rows[2]["G"] != "2019" || rows[2]["N"] != "2019" {
			return nil, fmt.Errorf("WZ2022 Table A : en-tête inattendu %v", rows[2])
		}
		groupe := ""
		for _, r := range rows[3:] {
			nom := r["B"]
			if nom == "" {
				continue
			}
			if _, ok := nombreCellule(r, "H"); !ok && len(r) == 1 {
				groupe = nom // « OECD countries », « Tax havens »…
				continue
			}
			for _, a := range annees {
				if v, ok := nombreCellule(r, a.colMd); ok {
					ajoute(nom, a.an, "BENEFICES_TRANSFERES", v, "MD_USD", f.DocumentID)
				}
				if v, ok := nombreCellule(r, a.colPerte); ok {
					ind := "PERTE_IS_PART"
					if groupe == "Tax havens" {
						ind = "GAIN_IS_PART"
					}
					ajoute(nom, a.an, ind, v, "RATIO", f.DocumentID)
				}
			}
		}
		nWZ := len(lignes)

		// TWZ2022, Table 3 (année 2015) : bénéfices déclarés, part des
		// entreprises étrangères, bénéfices transférés en part des bénéfices.
		f, err = arch.Fetch(ctx, srcID, runID, urlTWZ2022, ".xlsx")
		if err != nil {
			return nil, err
		}
		x, err = openXLSX(f.Path)
		if err != nil {
			return nil, err
		}
		rows, err = x.rows("Table3")
		x.Close()
		if err != nil {
			return nil, err
		}
		if rows[2]["C"] != "Reported domestic profits" || rows[2]["H"] != "Shifted profits (% reported profits)" {
			return nil, fmt.Errorf("TWZ2022 Table3 : en-tête inattendu %v", rows[2])
		}
		colonnes := []struct{ col, ind, unite string }{
			{"C", "BENEFICES_DECLARES", "MD_USD"},
			{"D", "BENEFICES_DECLARES_ENTREPRISES_LOCALES", "MD_USD"},
			{"E", "BENEFICES_DECLARES_ENTREPRISES_ETRANGERES", "MD_USD"},
			{"H", "BENEFICES_TRANSFERES_PART_DECLARES", "RATIO"},
		}
		for _, r := range rows[3:] {
			nom := r["B"]
			if nom == "" || nom == "Check" || nom == "From Table 1" || nom == "Memo: Tax havens" {
				continue
			}
			for _, c := range colonnes {
				if v, ok := nombreCellule(r, c.col); ok {
					ajoute(nom, 2015, c.ind, v, c.unite, f.DocumentID)
				}
			}
		}
		if nWZ < 200 || len(lignes)-nWZ < 100 {
			return nil, fmt.Errorf("lecture incomplète : %d lignes WZ, %d lignes TWZ", nWZ, len(lignes)-nWZ)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)

		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE tmp_transfert_benefices_estimation (
				pays text, annee smallint, indicateur text, valeur numeric, unite text,
				source_id bigint, document_id bigint
			) ON COMMIT DROP`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_transfert_benefices_estimation"},
			[]string{"pays", "annee", "indicateur", "valeur", "unite", "source_id", "document_id"},
			pgx.CopyFromRows(lignes)); err != nil {
			return nil, err
		}

		// MERGE plutôt que DELETE+COPY : ce connecteur est l'unique propriétaire
		// de la table ; l'ancien DELETE (scopé sur source_id, mais portant sur
		// l'intégralité des lignes de ce connecteur) payait le prix des
		// triggers RI pour tout son périmètre à chaque republication, changement
		// ou non.
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.transfert_benefices_estimation AS tgt
			USING tmp_transfert_benefices_estimation AS src
			ON tgt.pays = src.pays AND tgt.annee = src.annee AND tgt.indicateur = src.indicateur
			WHEN MATCHED AND (tgt.valeur, tgt.unite, tgt.source_id, tgt.document_id)
			                  IS DISTINCT FROM (src.valeur, src.unite, src.source_id, src.document_id) THEN
			    UPDATE SET valeur = src.valeur, unite = src.unite, source_id = src.source_id,
			               document_id = src.document_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (pays, annee, indicateur, valeur, unite, source_id, document_id)
			    VALUES (src.pays, src.annee, src.indicateur, src.valeur, src.unite, src.source_id, src.document_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return nil, fmt.Errorf("fusion transfert_benefices_estimation : %w", err)
		}
		touchees := ct.RowsAffected()

		return map[string]any{"wz2022": nWZ, "twz2022": len(lignes) - nWZ, "doublons_ecartes": doublons,
			"touchees": touchees}, tx.Commit(ctx)
	})
}
