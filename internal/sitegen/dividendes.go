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
type LignePartage struct {
	Annee                     int
	VA, Remun, EBE, Dividende float64
	Impots                    float64 // impôts sur la production, nets des subventions : VA − Remun − EBE
	PartRemun, PartEBE        float64
	PartImpots                float64
	DivSurEBE                 float64
}

type StatsDividendes struct {
	Debut, Fin     int
	VersesSNF      float64
	VersesSF       float64
	RecusMenages   float64
	EBE            float64
	DivSurEBE      float64
	DivSurEBEDebut float64
	PartMenages    float64
	BarresRatio    template.HTML
	BarresMontants template.HTML
	Partage        []LignePartage
	ISPaye         float64
	// FluxDebut / FluxFin : deux instantanés du partage de la valeur ajoutée,
	// dessinés en flux — une bande par destination, largeur proportionnelle à
	// sa part. 1971 est écarté comme point de départ : cette année-là, les
	// subventions dépassaient les impôts sur la production (valeur négative,
	// voir la note du tableau), qu'une largeur ne peut pas représenter. Le
	// premier exercice à trois parts positives sert de point de comparaison.
	FluxDebut, FluxFin           template.HTML
	AnneeFluxDebut, AnneeFluxFin int
	// EmpileesPartage : la même répartition en trois parts, mais en 100 %
	// empilé sur toute la série (moins 1971) — les deux instantanés du flux
	// ci-dessus montrent le début et la fin, ceci montre le trajet entre eux.
	EmpileesPartage template.HTML
	// BarresRatioIS : l'impôt sur les sociétés payé, rapporté à l'EBE, sur le
	// même principe que le ratio dividendes/EBE — la deuxième ponction sur le
	// même profit, pour la comparer sans jamais convertir un euro courant.
	BarresRatioIS template.HTML
}

func loadDividendes(ctx context.Context, pool *pgxpool.Pool) (*StatsDividendes, error) {
	st := &StatsDividendes{}
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
	var ratio []PointAnnee
	var paires []PaireAnnee
	var ratioIS []PointAnnee
	var anneesPartageComplet []int
	remunPct := map[int]float64{}
	impotsPct := map[int]float64{}
	ebePct := map[int]float64{}
	for rows.Next() {
		var an int
		var snf, sf, men, ebe, va, rem, is *float64
		if err := rows.Scan(&an, &snf, &sf, &men, &ebe, &va, &rem, &is); err != nil {
			return nil, err
		}
		if snf == nil || ebe == nil || *ebe == 0 {
			continue
		}
		if st.Debut == 0 {
			st.Debut = an
			st.DivSurEBEDebut = 100 * *snf / *ebe
		}
		st.Fin = an
		st.VersesSNF, st.EBE = *snf*1e6, *ebe*1e6
		st.DivSurEBE = 100 * *snf / *ebe
		if sf != nil {
			st.VersesSF = *sf * 1e6
		}
		if men != nil {
			st.RecusMenages = *men * 1e6
			paires = append(paires, PaireAnnee{an, *snf * 1e6, *men * 1e6})
		}
		if is != nil {
			st.ISPaye = *is * 1e6
			ratioIS = append(ratioIS, PointAnnee{an, 100 * *is / *ebe})
		}
		ratio = append(ratio, PointAnnee{an, 100 * *snf / *ebe})
		if va != nil && rem != nil && *va > 0 && (an%10 == 1 || an == 2024 || an == 2008) {
			impots := *va - *rem - *ebe
			st.Partage = append(st.Partage, LignePartage{
				Annee: an, VA: *va * 1e6, Remun: *rem * 1e6, EBE: *ebe * 1e6,
				Impots: impots * 1e6, Dividende: *snf * 1e6, PartRemun: 100 * *rem / *va,
				PartEBE: 100 * *ebe / *va, PartImpots: 100 * impots / *va,
				DivSurEBE: 100 * *snf / *ebe,
			})
		}
		// La série COMPLÈTE (pas l'échantillon décennal de Partage), pour le
		// 100 % empilé : 1971 est écarté comme il l'est déjà du flux ci-dessus —
		// des impôts nets négatifs n'ont pas de hauteur de segment à dessiner.
		if va != nil && rem != nil && *va > 0 {
			impots := *va - *rem - *ebe
			if impots >= 0 {
				anneesPartageComplet = append(anneesPartageComplet, an)
				remunPct[an] = 100 * *rem / *va
				impotsPct[an] = 100 * impots / *va
				ebePct[an] = 100 * *ebe / *va
			}
		}
	}
	if st.VersesSNF > 0 {
		st.PartMenages = 100 * st.RecusMenages / (st.VersesSNF + st.VersesSF)
	}

	// Les deux instantanés du flux : le premier exercice de la table au
	// partage positif, et le dernier (2024).
	for _, ligne := range st.Partage {
		if ligne.Impots >= 0 {
			st.AnneeFluxDebut = ligne.Annee
			st.FluxDebut = fluxPartageVA(ligne.Annee, ligne.Remun, ligne.Impots, ligne.EBE, ligne.VA)
			break
		}
	}
	if n := len(st.Partage); n > 0 {
		last := st.Partage[n-1]
		st.AnneeFluxFin = last.Annee
		st.FluxFin = fluxPartageVA(last.Annee, last.Remun, last.Impots, last.EBE, last.VA)
	}
	st.BarresRatio = courbe(ratio, func(v float64) string { return Decimal(v, 1) + " %" })
	st.BarresRatioIS = courbe(ratioIS, func(v float64) string { return Decimal(v, 1) + " %" })
	st.BarresMontants = barresAppariees(paires,
		"Versés par les sociétés non financières", "Reçus par les ménages résidents", mdEur, 10)
	st.EmpileesPartage = barresEmpileesAnnuelles(anneesPartageComplet, []SerieEmpilee{
		{Libelle: "Rémunération des salariés", Couleur: couleursPartageVA[0], Valeurs: remunPct},
		{Libelle: "Impôts sur la production (net)", Couleur: couleursPartageVA[1], Valeurs: impotsPct},
		{Libelle: "Excédent brut d'exploitation", Couleur: couleursPartageVA[2], Valeurs: ebePct},
	}, func(v float64) string { return Decimal(v, 1) + " %" })
	return st, rows.Err()
}
