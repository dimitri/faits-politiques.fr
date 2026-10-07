package sitegen

import (
	"context"
	"html/template"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Les dividendes, à l'échelle où la comptabilité nationale les publie.
//
// Trois pièges, qui décident de toute la lecture :
//
//  1. « Sociétés non financières » n'est pas « le CAC 40 » : c'est l'ensemble
//     des entreprises résidentes hors banques et assurances, de l'artisan en
//     société au groupe coté ;
//  2. les dividendes VERSÉS (opération D.42) sont bruts et non consolidés : une
//     filiale qui paie sa maison mère, qui paie à son tour ses actionnaires,
//     compte deux fois. Le total ne mesure donc pas ce qui sort des
//     entreprises vers les ménages ;
//  3. ce qui arrive aux ménages résidents est publié à part — et c'est un
//     cinquième du total versé.
type LineShared struct {
	Year                     int
	VA, Remun, EBE, Dividend float64
	Taxes                    float64 // impôts sur la production, nets des subventions : VA − Remun − EBE
	ShareRemun, ShareEBE     float64
	ShareTaxes               float64
	DivOnEBE                 float64
}

type StatsDividends struct {
	Start, End         int
	PaidSNF            float64
	PaidSF             float64
	ReceivedHouseholds float64
	EBE                float64
	DivOnEBE           float64
	DivOnEBEStart      float64
	ShareHouseholds    float64
	BarsRatio          template.HTML
	BarsAmounts        template.HTML
	Shared             []LineShared
	ISPaid             float64
	// FluxDebut / FluxFin : deux instantanés du partage de la valeur ajoutée,
	// dessinés en flux — une bande par destination, largeur proportionnelle à
	// sa part. 1971 est écarté comme point de départ : cette année-là, les
	// subventions dépassaient les impôts sur la production (valeur négative,
	// voir la note du tableau), qu'une largeur ne peut pas représenter. Le
	// premier exercice à trois parts positives sert de point de comparaison.
	FlowStart, FlowEnd         template.HTML
	YearFlowStart, YearFlowEnd int
	// EmpileesPartage : la même répartition en trois parts, mais en 100 %
	// empilé sur toute la série (moins 1971) — les deux instantanés du flux
	// ci-dessus montrent le début et la fin, ceci montre le trajet entre eux.
	StackedShared template.HTML
	// BarresRatioIS : l'impôt sur les sociétés payé, rapporté à l'EBE, sur le
	// même principe que le ratio dividendes/EBE — la deuxième ponction sur le
	// même profit, pour la comparer sans jamais convertir un euro courant.
	BarsRatioIS template.HTML
}

func loadDividends(ctx context.Context, pool *pgxpool.Pool) (*StatsDividends, error) {
	st := &StatsDividends{}
	rows, err := pool.Query(ctx, `
		SELECT annee,
		       max(valeur) FILTER (WHERE serie_code='dividendes.verses.snf')::float8,
		       max(valeur) FILTER (WHERE serie_code='dividendes.verses.sf')::float8,
		       max(valeur) FILTER (WHERE serie_code='dividendes.recus.menages')::float8,
		       max(valeur) FILTER (WHERE serie_code='ebe.snf')::float8,
		       max(valeur) FILTER (WHERE serie_code='valeur.ajoutee.snf')::float8,
		       max(valeur) FILTER (WHERE serie_code='remuneration.salaries.snf')::float8,
		       max(valeur) FILTER (WHERE serie_code='impots.revenu.payes.snf')::float8
		FROM core.macro_value
		WHERE serie_code IN ('dividendes.verses.snf','dividendes.verses.sf',
		  'dividendes.recus.menages','ebe.snf','valeur.ajoutee.snf',
		  'remuneration.salaries.snf','impots.revenu.payes.snf')
		GROUP BY annee ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ratio []PointYear
	var pairs []PairYear
	var ratioIS []PointYear
	var yearsSharedComplete []int
	remunPct := map[int]float64{}
	taxesPct := map[int]float64{}
	ebePct := map[int]float64{}
	for rows.Next() {
		var an int
		var snf, sf, households, ebe, va, rem, is *float64
		if err := rows.Scan(&an, &snf, &sf, &households, &ebe, &va, &rem, &is); err != nil {
			return nil, err
		}
		if snf == nil || ebe == nil || *ebe == 0 {
			continue
		}
		if st.Start == 0 {
			st.Start = an
			st.DivOnEBEStart = 100 * *snf / *ebe
		}
		st.End = an
		st.PaidSNF, st.EBE = *snf*1e6, *ebe*1e6
		st.DivOnEBE = 100 * *snf / *ebe
		if sf != nil {
			st.PaidSF = *sf * 1e6
		}
		if households != nil {
			st.ReceivedHouseholds = *households * 1e6
			pairs = append(pairs, PairYear{an, *snf * 1e6, *households * 1e6})
		}
		if is != nil {
			st.ISPaid = *is * 1e6
			ratioIS = append(ratioIS, PointYear{an, 100 * *is / *ebe})
		}
		ratio = append(ratio, PointYear{an, 100 * *snf / *ebe})
		if va != nil && rem != nil && *va > 0 && (an%10 == 1 || an == 2024 || an == 2008) {
			taxes := *va - *rem - *ebe
			st.Shared = append(st.Shared, LineShared{
				Year: an, VA: *va * 1e6, Remun: *rem * 1e6, EBE: *ebe * 1e6,
				Taxes: taxes * 1e6, Dividend: *snf * 1e6, ShareRemun: 100 * *rem / *va,
				ShareEBE: 100 * *ebe / *va, ShareTaxes: 100 * taxes / *va,
				DivOnEBE: 100 * *snf / *ebe,
			})
		}
		// La série COMPLÈTE (pas l'échantillon décennal de Partage), pour le
		// 100 % empilé : 1971 est écarté comme il l'est déjà du flux ci-dessus —
		// des impôts nets négatifs n'ont pas de hauteur de segment à dessiner.
		if va != nil && rem != nil && *va > 0 {
			taxes := *va - *rem - *ebe
			if taxes >= 0 {
				yearsSharedComplete = append(yearsSharedComplete, an)
				remunPct[an] = 100 * *rem / *va
				taxesPct[an] = 100 * taxes / *va
				ebePct[an] = 100 * *ebe / *va
			}
		}
	}
	if st.PaidSNF > 0 {
		st.ShareHouseholds = 100 * st.ReceivedHouseholds / (st.PaidSNF + st.PaidSF)
	}

	// Les deux instantanés du flux : le premier exercice de la table au
	// partage positif, et le dernier (2024).
	for _, line := range st.Shared {
		if line.Taxes >= 0 {
			st.YearFlowStart = line.Year
			st.FlowStart = flowSharedVA(line.Year, line.Remun, line.Taxes, line.EBE, line.VA)
			break
		}
	}
	if n := len(st.Shared); n > 0 {
		last := st.Shared[n-1]
		st.YearFlowEnd = last.Year
		st.FlowEnd = flowSharedVA(last.Year, last.Remun, last.Taxes, last.EBE, last.VA)
	}
	st.BarsRatio = curve(ratio, func(v float64) string { return Decimal(v, 1) + " %" })
	st.BarsRatioIS = curve(ratioIS, func(v float64) string { return Decimal(v, 1) + " %" })
	st.BarsAmounts = barsMatched(pairs,
		"Versés par les sociétés non financières", "Reçus par les ménages résidents", mdEur, 10)
	st.StackedShared = barsStackedAnnual(yearsSharedComplete, []SeriesStacked{
		{Label: "Rémunération des salariés", Color: colorsSharedVA[0], Values: remunPct},
		{Label: "Impôts sur la production (net)", Color: colorsSharedVA[1], Values: taxesPct},
		{Label: "Excédent brut d'exploitation", Color: colorsSharedVA[2], Values: ebePct},
	}, func(v float64) string { return Decimal(v, 1) + " %" })
	return st, rows.Err()
}
