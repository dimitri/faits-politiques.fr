package sitegen

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Le financement des SDIS, une carte par département : la participation
// départementale (le premier financeur, docs/securite-civile-donnees.md), en
// euros par habitant desservi — un SDIS, un Siren, joint à son département
// via core.sdis (internal/securitecivile.IngestSDIS).
//
// Une seule carte, pas quinze comme /securite/ : la participation
// départementale est le fait le plus structurant du financement des SDIS
// (près de la moitié des recettes), les autres financeurs (communes,
// groupements, État) restent à l'échelle nationale, dans la série annuelle
// de cette même page (PageMap.Series/Curve) plutôt que sur leur propre carte
// — ajoutable plus tard sans rien changer à la table.
func loadFinancementSDIS(ctx context.Context, pool *pgxpool.Pool) (*PageMap, error) {
	// Une seule carte, jamais affichée en vignette d'index (pas de grille à
	// charger comme securite.go, un seul indicateur) : un seul tracé, au
	// niveau de détail complet, suffit.
	end, err := setOutlines(ctx, pool, "DEPARTEMENT", toleranceFull)
	if err != nil {
		return nil, err
	}

	// max(exercice) sur zéro ligne renvoie une ligne NULL, pas zéro ligne :
	// un *int planterait au Scan tant que le connecteur SDIS n'a pas tourné
	// (base vide fraîchement migrée, graine CI minimale...). sql.NullInt32
	// distingue ce cas pour ne pas publier une carte vide.
	var lastYear sql.NullInt32
	if err := pool.QueryRow(ctx, `SELECT max(exercice) FROM core.collectivite_budget WHERE niveau = 'SDIS'`).
		Scan(&lastYear); err != nil {
		return nil, err
	}
	if !lastYear.Valid {
		return nil, nil
	}

	eur := func(v float64) string { return Count(int(v+0.5)) + " €" }

	rows, err := pool.Query(ctx, `
		SELECT s.code_departement, s.nom, b.euros_par_hab
		  FROM core.collectivite_budget b
		  JOIN core.sdis s ON s.siren = b.code
		 WHERE b.niveau = 'SDIS' AND b.indicator_code = 'sdis.participation_departements'
		   AND b.exercice = $1`, lastYear)
	if err != nil {
		return nil, err
	}
	var cells []CellMap
	for rows.Next() {
		var cc CellMap
		var v *float64
		if err := rows.Scan(&cc.Code, &cc.Name, &v); err != nil {
			rows.Close()
			return nil, err
		}
		if v == nil {
			cc.Absent = true
		} else {
			cc.Value = *v
		}
		cells = append(cells, cc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	ranks := ranking(cells, end.Noms, eur)
	page := &PageMap{
		Slug: "securite-civile", Title: "Qui finance les pompiers : la participation des départements",
		Question: "Combien chaque département verse-t-il à son service départemental d'incendie et de secours " +
			"(SDIS), par habitant ?",
		Source:          "OFGL, comptes des SDIS (budget principal uniquement)",
		Section:         "Sécurité civile",
		SectionURL:      "sujets/securite-civile",
		SectionIndexURL: "sujets/securite-civile",
		Map:             full(end, cells, "€ par habitant", eur),
		// Summary (médiane/max/min) donne l'échelle sans le classement complet :
		// contrairement aux cartes de délinquance (carte-detail.gohtml, cas
		// général), un rang ici n'ordonne pas un fait, il ordonne un montage
		// institutionnel (budget transféré au groupement ou non, SDIS fusionné
		// à une métropole...) — l'exposer en tableau invite à lire un haut ou
		// un bas de classement comme un jugement sur le financement, que le
		// texte du dossier démonte justement (Rhône, Corse). D'où Ranking
		// volontairement vide : la carte situe, le classement complet
		// n'ajoute rien que la carte ne dise déjà, avec le risque en plus.
		Summary: summarizeRanking(ranks),
		Note: "Un SDIS (service départemental d'incendie et de secours) est un établissement public distinct du " +
			"département, qui le finance : c'est sa participation obligatoire que cette carte mesure, la " +
			"première source de financement des SDIS (un peu moins de la moitié de leurs recettes), mais pas la " +
			"seule — communes, groupements à fiscalité propre et État contribuent aussi (voir la série annuelle " +
			"ci-dessous, et le dossier pour le détail).",
	}

	srows, err := pool.Query(ctx, `
		SELECT exercice, sum(montant)
		  FROM core.collectivite_budget
		 WHERE niveau = 'SDIS' AND indicator_code = 'sdis.participation_departements'
		 GROUP BY exercice ORDER BY exercice`)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var p PointYear
		var v *float64
		if err := srows.Scan(&p.Year, &v); err != nil {
			srows.Close()
			return nil, err
		}
		if v != nil {
			p.Value = *v
			page.Series = append(page.Series, p)
		}
	}
	if err := srows.Err(); err != nil {
		return nil, err
	}
	srows.Close()
	if len(page.Series) > 1 {
		page.SeriesLegend = "Total national versé par les départements à leur SDIS, par an"
		page.Curve = curve(page.Series, func(v float64) string { return Decimal(v/1e9, 2) + " Md€" })
	}
	return page, nil
}
