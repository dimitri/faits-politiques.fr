package main

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

type DispositifFiscal struct {
	Numero, Libelle, Impot string
	MontantM               float64
}

type ImpotTotal struct {
	Impot     string
	NbDisp    int
	TotalMdEu float64
}

type StatsDepensesFiscales struct {
	Millesime, Annee                           int
	NbTotalMillesime                           int
	NbChiffres, NbNC, NbEpsilon                int
	TotalMdEur                                 float64
	TopDispositifs                             []DispositifFiscal
	ParImpot                                   []ImpotTotal
	MillesimeBeneficiaires                     int
	NbEntreprises, NbMenages, NbMixte, NbAutre int
}

func chargerStatsDepensesFiscales(ctx context.Context, pool *pgxpool.Pool) (*StatsDepensesFiscales, error) {
	s := &StatsDepensesFiscales{Millesime: 2026, Annee: 2025, MillesimeBeneficiaires: 2023}

	if err := pool.QueryRow(ctx, `SELECT count(DISTINCT numero) FROM core.depense_fiscale WHERE millesime=$1`,
		s.Millesime).Scan(&s.NbTotalMillesime); err != nil {
		return nil, err
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE mention IS NULL), count(*) FILTER (WHERE mention = 'nc'), count(*) FILTER (WHERE mention = 'ε'),
		       coalesce(sum(montant_eur) FILTER (WHERE mention IS NULL), 0)
		FROM core.depense_fiscale WHERE millesime=$1 AND annee=$2`,
		s.Millesime, s.Annee).Scan(&s.NbChiffres, &s.NbNC, &s.NbEpsilon, &s.TotalMdEur); err != nil {
		return nil, err
	}
	s.TotalMdEur /= 1e9

	rows, err := pool.Query(ctx, `
		SELECT numero, libelle, impot, montant_eur/1e6
		FROM core.depense_fiscale WHERE millesime=$1 AND annee=$2 AND montant_eur IS NOT NULL
		ORDER BY montant_eur DESC LIMIT 10`, s.Millesime, s.Annee)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d DispositifFiscal
		if err := rows.Scan(&d.Numero, &d.Libelle, &d.Impot, &d.MontantM); err != nil {
			rows.Close()
			return nil, err
		}
		s.TopDispositifs = append(s.TopDispositifs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	rows, err = pool.Query(ctx, `
		SELECT impot, count(*), sum(montant_eur)/1e9
		FROM core.depense_fiscale WHERE millesime=$1 AND annee=$2 AND montant_eur IS NOT NULL
		GROUP BY impot ORDER BY sum(montant_eur) DESC`, s.Millesime, s.Annee)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var it ImpotTotal
		if err := rows.Scan(&it.Impot, &it.NbDisp, &it.TotalMdEu); err != nil {
			rows.Close()
			return nil, err
		}
		s.ParImpot = append(s.ParImpot, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE nature = 'ENTREPRISES'), count(*) FILTER (WHERE nature = 'MENAGES'),
		       count(*) FILTER (WHERE nature = 'ENTREPRISES_ET_MENAGES'), count(*) FILTER (WHERE nature NOT IN ('ENTREPRISES','MENAGES','ENTREPRISES_ET_MENAGES'))
		FROM ref.depense_fiscale_beneficiaire WHERE millesime=$1`,
		s.MillesimeBeneficiaires).Scan(&s.NbEntreprises, &s.NbMenages, &s.NbMixte, &s.NbAutre); err != nil {
		return nil, err
	}

	return s, nil
}

// chargerTendanceDepensesFiscales : le total exécuté, année par année —
// chaque millésime du PLF ne publie l'exécution que pour une seule année
// (l'avant-dernière), jamais révisée dans un millésime ultérieur : sept
// millésimes donnent donc sept années d'exécution distinctes, sans doublon
// ni superposition à trancher.
func chargerTendanceDepensesFiscales(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, sum(montant_eur)/1e9 FROM core.depense_fiscale
		WHERE stade='EXECUTION' AND montant_eur IS NOT NULL
		GROUP BY annee ORDER BY annee`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var pts []PointAnnee
	for rows.Next() {
		var p PointAnnee
		if err := rows.Scan(&p.Annee, &p.Valeur); err != nil {
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
	return courbe(pts, format), nil
}
