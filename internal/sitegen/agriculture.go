package sitegen

import (
	"context"
	"encoding/csv"
	"fmt"
	"html/template"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Agriculture et alimentation. Aucune carte : ce n'est pas un oubli.
//
// Les deux séries ingérées — usage des sols et bilan alimentaire — sont
// NATIONALES. Les poser sur une carte départementale supposerait de répartir un
// total national entre 96 départements, c'est-à-dire d'inventer la donnée
// manquante. La page le dit et montre les séries telles qu'elles existent.
type SerieAgri struct {
	Code, Titre, Unite, Note string
	Debut, Fin               int
	Points                   []PointAnnee
	Courbe                   template.HTML
	Premier, Dernier         string
	Variation                float64
}

type PartageProduit struct {
	Code, Libelle   string
	Humain, Betail  float64
	PartBetail      float64
	Interieure      float64
	PartHorsAliment float64
	AvecInterieure  bool
}

type StatsAgri struct {
	Series       []SerieAgri
	Partage      []PartageProduit
	AnneeBilan   int
	Kcal         float64
	KcalVegetal  float64
	KcalAnimal   float64
	AnneeKcal    int
	NbProduits   int
	TotalHumain  float64
	TotalBetail  float64
	Rendement    []PointAnnee
	CourbeRend   template.HTML
	KcalHectare  float64
	NourrisPar1k float64
	SurfaceHa    float64
	Autonomie    []LigneAutonomie

	RevenuFrance    []PointAnnee
	CourbeRevenu    template.HTML
	RevenuDernierFR float64
	RevenuDernierUE float64
	AnneeRevenu     int
}

// Le taux d'auto-approvisionnement : production ÷ disponibilité intérieure.
// C'est une division entre deux colonnes publiées, pas un modèle. Elle ne dit
// PAS que le pays se nourrit lui-même : un taux de 176 % sur les céréales
// coexiste avec des importations, parce qu'on n'exporte pas et n'importe pas
// les mêmes qualités ni aux mêmes saisons.
type LigneAutonomie struct {
	Libelle                string
	Production, Interieure float64
	Taux                   float64
}

// milliers : les séries agricoles sont publiées en « 1000 ha » et « 1000 No ».
// On garde l'unité de la source plutôt que de multiplier par mille pour le
// plaisir d'aligner des zéros.
func milliers(v float64) string { return Nombre(int(v + 0.5)) }

func loadProduitsAlimentaires(path string) ([][3]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var out [][3]string
	for i, c := range rows {
		if i == 0 || len(c) < 3 {
			continue
		}
		out = append(out, [3]string{strings.TrimSpace(c[0]), strings.TrimSpace(c[1]), c[2]})
	}
	return out, nil
}

func loadAgriculture(ctx context.Context, pool *pgxpool.Pool, dataDir string) (*StatsAgri, error) {
	st := &StatsAgri{}

	def := []struct{ code, titre, note string }{
		{"terres.agricoles", "Terres agricoles",
			"Surface agricole utilisée : cultures, prairies permanentes et cultures permanentes réunies."},
		{"terres.arables", "Terres arables",
			"Les terres labourées. Une baisse peut venir d'une urbanisation, d'un boisement ou d'un passage en prairie : la série ne dit pas lequel."},
		{"terres.prairies", "Prairies permanentes", ""},
		{"emploi.agricole", "Emploi agricole", "Personnes, pas équivalents temps plein."},
		{"emploi.agroalimentaire", "Emploi agroalimentaire",
			"Industrie de transformation, pas les exploitations."},
	}
	for _, d := range def {
		rows, err := pool.Query(ctx, `
			SELECT annee, valeur::float8, unite FROM core.agriculture_indicateur
			WHERE code=$1 ORDER BY annee`, d.code)
		if err != nil {
			return nil, err
		}
		s := SerieAgri{Code: d.code, Titre: d.titre, Note: d.note}
		for rows.Next() {
			var p PointAnnee
			if err := rows.Scan(&p.Annee, &p.Valeur, &s.Unite); err != nil {
				break
			}
			s.Points = append(s.Points, p)
		}
		rows.Close()
		if len(s.Points) < 2 {
			continue
		}
		s.Debut, s.Fin = s.Points[0].Annee, s.Points[len(s.Points)-1].Annee
		s.Premier = milliers(s.Points[0].Valeur)
		s.Dernier = milliers(s.Points[len(s.Points)-1].Valeur)
		if s.Points[0].Valeur != 0 {
			s.Variation = 100 * (s.Points[len(s.Points)-1].Valeur - s.Points[0].Valeur) / s.Points[0].Valeur
		}
		s.Courbe = courbe(s.Points, milliers)
		st.Series = append(st.Series, s)
	}

	_ = pool.QueryRow(ctx, `SELECT max(annee) FROM core.bilan_alimentaire`).Scan(&st.AnneeBilan)
	_ = pool.QueryRow(ctx, `
		SELECT max(valeur) FILTER (WHERE produit_code='grand_total')::float8,
		       max(valeur) FILTER (WHERE produit_code='vegetal_products')::float8,
		       max(valeur) FILTER (WHERE produit_code='animal_products')::float8
		FROM core.bilan_alimentaire
		WHERE element='Food supply (kcal/capita/day)' AND annee=$1`,
		st.AnneeBilan).Scan(&st.Kcal, &st.KcalVegetal, &st.KcalAnimal)
	st.AnneeKcal = st.AnneeBilan

	// Calories nourries à l'hectare : le seul indicateur de cette page qui
	// rapporte la production à la surface. Il vient d'une vue de la base, pas
	// d'un calcul fait ici.
	rrows, err := pool.Query(ctx, `
		SELECT annee, kcal_par_hectare_an::float8, surface_agricole_ha::float8,
		       habitants_nourris_par_10000_ha::float8
		FROM derived.calories_par_hectare ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	for rrows.Next() {
		var p PointAnnee
		var surf, nourris float64
		if err := rrows.Scan(&p.Annee, &p.Valeur, &surf, &nourris); err != nil {
			break
		}
		st.Rendement = append(st.Rendement, p)
		st.KcalHectare, st.SurfaceHa, st.NourrisPar1k = p.Valeur, surf, nourris
	}
	rrows.Close()
	if len(st.Rendement) > 1 {
		st.CourbeRend = courbe(st.Rendement, func(v float64) string {
			return Decimal(v/1e6, 2) + " Mkcal"
		})
	}

	arows, err := pool.Query(ctx, `
		SELECT libelle, production::float8, disponibilite_interieure::float8, taux_pct::float8
		FROM derived.autonomie_alimentaire
		WHERE annee=$1 AND production>2000 ORDER BY production DESC`, st.AnneeBilan)
	if err != nil {
		return nil, err
	}
	for arows.Next() {
		var a LigneAutonomie
		if err := arows.Scan(&a.Libelle, &a.Production, &a.Interieure, &a.Taux); err != nil {
			break
		}
		st.Autonomie = append(st.Autonomie, a)
	}
	arows.Close()

	rrows2, err := pool.Query(ctx, `
		SELECT annee, euro_par_uta FROM core.revenu_agricole_reel
		WHERE geo_code='FR' ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	for rrows2.Next() {
		var p PointAnnee
		if err := rrows2.Scan(&p.Annee, &p.Valeur); err != nil {
			break
		}
		st.RevenuFrance = append(st.RevenuFrance, p)
	}
	rrows2.Close()
	if len(st.RevenuFrance) > 1 {
		st.CourbeRevenu = courbe(st.RevenuFrance, func(v float64) string { return Nombre(int(v)) + " €" })
		dernier := st.RevenuFrance[len(st.RevenuFrance)-1]
		st.AnneeRevenu = dernier.Annee
		st.RevenuDernierFR = dernier.Valeur
		_ = pool.QueryRow(ctx, `
			SELECT euro_par_uta FROM core.revenu_agricole_reel WHERE geo_code='EU27_2020' AND annee=$1`,
			st.AnneeRevenu).Scan(&st.RevenuDernierUE)
	}

	produits, err := loadProduitsAlimentaires(dataDir + "/produits-alimentaires.csv")
	if err != nil {
		return nil, err
	}
	st.NbProduits = len(produits)
	for _, p := range produits {
		q := PartageProduit{Code: p[0], Libelle: p[1]}
		var f, fe, di *float64
		_ = pool.QueryRow(ctx, `
			SELECT max(valeur) FILTER (WHERE element='Food')::float8,
			       max(valeur) FILTER (WHERE element='Feed')::float8,
			       max(valeur) FILTER (WHERE element='Domestic supply quantity')::float8
			FROM core.bilan_alimentaire WHERE produit_code=$1 AND annee=$2`,
			p[0], st.AnneeBilan).Scan(&f, &fe, &di)
		if f != nil {
			q.Humain = *f
		}
		if fe != nil {
			q.Betail = *fe
		}
		if q.Humain+q.Betail == 0 {
			continue
		}
		q.PartBetail = 100 * q.Betail / (q.Humain + q.Betail)
		if di != nil && *di > 0 {
			q.Interieure, q.AvecInterieure = *di, true
			q.PartHorsAliment = 100 * (*di - q.Humain - q.Betail) / *di
		}
		st.TotalHumain += q.Humain
		st.TotalBetail += q.Betail
		st.Partage = append(st.Partage, q)
	}
	return st, nil
}

func pctAgri(v float64) string { return strings.ReplaceAll(fmt.Sprintf("%.0f", v), ".", ",") + " %" }
