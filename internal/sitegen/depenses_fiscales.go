package sitegen

import (
	"context"
	"html/template"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Les dépenses fiscales (niches fiscales) : core.depense_fiscale et
// ref.depense_fiscale_beneficiaire, déjà chargées (7 millésimes PLF
// 2020-2026), jamais présentées pour elles-mêmes — seulement mentionnées en
// passant dans docs/dette-donnees.md sous l'angle « budget vert ». Voir
// docs/depenses-fiscales-donnees.md.

type SchemeFiscal struct {
	Number, Label, Tax string
	AmountM            float64
}

type TaxTotal struct {
	Tax         string
	CountScheme int
	TotalMdEu   float64
}

type StatsExpensesFiscal struct {
	Vintage, Year                                           int
	CountTotalVintage                                       int
	CountFigures, CountNC, CountEpsilon                     int
	TotalMdEur                                              float64
	TopSchemes                                              []SchemeFiscal
	PerTax                                                  []TaxTotal
	VintageBeneficiaries                                    int
	CountCompanies, CountHouseholds, CountMixed, CountOther int
}

func loadStatsExpensesFiscal(ctx context.Context, pool *pgxpool.Pool) (*StatsExpensesFiscal, error) {
	s := &StatsExpensesFiscal{Vintage: 2026, Year: 2025, VintageBeneficiaries: 2023}

	if err := pool.QueryRow(ctx, `SELECT count(DISTINCT numero) FROM core.depense_fiscale WHERE millesime=$1`,
		s.Vintage).Scan(&s.CountTotalVintage); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE mention IS NULL), count(*) FILTER (WHERE mention = 'nc'), count(*) FILTER (WHERE mention = 'ε'),
		       coalesce(sum(montant_eur) FILTER (WHERE mention IS NULL), 0)
		FROM core.depense_fiscale WHERE millesime=$1 AND annee=$2`,
		s.Vintage, s.Year).Scan(&s.CountFigures, &s.CountNC, &s.CountEpsilon, &s.TotalMdEur); err != nil {
		return nil, err
	}
	s.TotalMdEur /= 1e9

	rows, err := pool.Query(ctx, `
		SELECT numero, libelle, impot, montant_eur/1e6
		FROM core.depense_fiscale WHERE millesime=$1 AND annee=$2 AND montant_eur IS NOT NULL
		ORDER BY montant_eur DESC LIMIT 10`, s.Vintage, s.Year)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d SchemeFiscal
		if err := rows.Scan(&d.Number, &d.Label, &d.Tax, &d.AmountM); err != nil {
			rows.Close()
			return nil, err
		}
		s.TopSchemes = append(s.TopSchemes, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	rows, err = pool.Query(ctx, `
		SELECT impot, count(*), sum(montant_eur)/1e9
		FROM core.depense_fiscale WHERE millesime=$1 AND annee=$2 AND montant_eur IS NOT NULL
		GROUP BY impot ORDER BY sum(montant_eur) DESC`, s.Vintage, s.Year)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var it TaxTotal
		if err := rows.Scan(&it.Tax, &it.CountScheme, &it.TotalMdEu); err != nil {
			rows.Close()
			return nil, err
		}
		s.PerTax = append(s.PerTax, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE nature = 'ENTREPRISES'), count(*) FILTER (WHERE nature = 'MENAGES'),
		       count(*) FILTER (WHERE nature = 'ENTREPRISES_ET_MENAGES'), count(*) FILTER (WHERE nature NOT IN ('ENTREPRISES','MENAGES','ENTREPRISES_ET_MENAGES'))
		FROM ref.depense_fiscale_beneficiaire WHERE millesime=$1`,
		s.VintageBeneficiaries).Scan(&s.CountCompanies, &s.CountHouseholds, &s.CountMixed, &s.CountOther); err != nil {
		return nil, err
	}

	return s, nil
}

// loadTrendExpensesFiscal : le total exécuté, année par année —
// chaque millésime du PLF ne publie l'exécution que pour une seule année
// (l'avant-dernière), jamais révisée dans un millésime ultérieur : sept
// millésimes donnent donc sept années d'exécution distinctes, sans doublon
// ni superposition à trancher.
func loadTrendExpensesFiscal(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, sum(montant_eur)/1e9 FROM core.depense_fiscale
		WHERE stade='EXECUTION' AND montant_eur IS NOT NULL
		GROUP BY annee ORDER BY annee`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []PointYear
	for rows.Next() {
		var p PointYear
		if err := rows.Scan(&p.Year, &p.Value); err != nil {
			return "", err
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(pts) == 0 {
		return "", nil
	}
	format := func(v float64) string { return Decimal(v, 1) + " Md€" }
	return curve(pts, format), nil
}
