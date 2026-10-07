package sitegen

import (
	"context"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Sécurité : les faits enregistrés par la police et la gendarmerie.
//
// Le mot compte. Ce ne sont pas « les crimes commis » mais les faits ENREGISTRÉS :
// une hausse peut venir d'une hausse des faits, d'une hausse des plaintes, ou
// d'un changement d'enregistrement. Le SSMSI le dit lui-même, et le site aussi.
type IndicatorSecurity struct {
	Code, Label, Question string
	Slug                  string
	Overview              Map
	Page                  PageMap
	Series                []PointYear
	National              float64
	Published, Masks      int
}

type PointYear struct {
	Year  int
	Value float64
}

type StatsSecurity struct {
	Indicators     []IndicatorSecurity
	Defs           template.HTML
	Year, Start    int
	Municipalities int
}

var labelSecurity = map[string][2]string{
	"violences_physiques_intrafamiliales":            {"Violences intrafamiliales", "Où les violences dans la famille sont-elles le plus enregistrées ?"},
	"violences_physiques_hors_cadre_familial":        {"Violences hors famille", "Coups et blessures volontaires hors du cadre familial."},
	"violences_sexuelles":                            {"Violences sexuelles", "Faits enregistrés, très sensibles au taux de plainte."},
	"cambriolages_de_logement":                       {"Cambriolages de logement", "Le fait le plus déclaré, donc le mieux mesuré."},
	"vols_sans_violence_contre_des_personnes":        {"Vols sans violence", "Vols à la tire et assimilés."},
	"vols_violents_sans_arme":                        {"Vols violents sans arme", ""},
	"vols_avec_armes":                                {"Vols avec armes", "Faits rares : à ce niveau, une seule affaire déplace un territoire."},
	"vols_de_vehicule":                               {"Vols de véhicule", ""},
	"vols_dans_les_vehicules":                        {"Vols dans les véhicules", ""},
	"vols_d_accessoires_sur_vehicules":               {"Vols d'accessoires sur véhicules", ""},
	"destructions_et_degradations_volontaires":       {"Destructions et dégradations", ""},
	"trafic_de_stupefiants":                          {"Trafic de stupéfiants", "Mesure d'abord l'activité des services : sans plainte de victime, un fait n'est enregistré que s'il est constaté."},
	"usage_de_stupefiants":                           {"Usage de stupéfiants", "Même réserve : c'est une mesure de l'action publique autant que de l'usage."},
	"usage_de_stupefiants_afd":                       {"Usage de stupéfiants — amendes forfaitaires", "Faits d'usage traités par amende forfaitaire délictuelle, que le SSMSI publie sur une ligne distincte."},
	"escroqueries_et_fraudes_aux_moyens_de_paiement": {"Escroqueries et fraudes", ""},
}

func loadSecurity(ctx context.Context, pool *pgxpool.Pool) (*StatsSecurity, error) {
	vign, err := setOutlines(ctx, pool, "DEPARTEMENT", toleranceOverview)
	if err != nil {
		return nil, err
	}
	end, err := setOutlines(ctx, pool, "DEPARTEMENT", toleranceFull)
	if err != nil {
		return nil, err
	}
	st := &StatsSecurity{Year: 2025, Start: 2016, Defs: vign.Defs}
	_ = pool.QueryRow(ctx, `SELECT count(DISTINCT commune_code) FROM mv.commune_delinquance_dernier`).
		Scan(&st.Municipalities)

	tx := func(v float64) string { return Decimal(v, 1) + " ‰" }

	for code, lib := range labelSecurity {
		// Taux pour 1 000 habitants, agrégé au département : on additionne les
		// FAITS et les HABITANTS, jamais les taux — une moyenne de taux
		// donnerait le même poids à une commune de 200 âmes et à Marseille.
		// mv.securite_dept_annee (internal/matview) porte déjà nombre/
		// population sommés — plus le scan de core.commune_delinquance
		// (5,2 millions de lignes) que cette requête refaisait deux fois
		// par indicateur (ici, et pour la série nationale plus bas).
		rows, err := pool.Query(ctx, `
			SELECT code_departement, nom_departement,
			       1000.0*nombre/nullif(population,0)
			FROM mv.securite_dept_annee
			WHERE indicateur_code=$1 AND annee=$2`, code, st.Year)
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
		rows.Close()

		ranks := ranking(cells, vign.Noms, tx)
		ind := IndicatorSecurity{Code: code, Label: lib[0], Question: lib[1],
			Slug:     strings.ReplaceAll(code, "_", "-"),
			Overview: overview(vign, cells, "faits pour 1 000 habitants", tx)}
		ind.Page = PageMap{
			Slug: ind.Slug, Title: lib[0], Question: lib[1],
			Source:          "SSMSI, bases communales de la délinquance enregistrée",
			Section:         "Sécurité",
			SectionURL:      "securite",
			SectionIndexURL: "securite",
			Map:             full(end, cells, "faits pour 1 000 habitants", tx),
			Summary:         summarizeRanking(ranks),
			Ranking:         ranks,
		}

		srows, err := pool.Query(ctx, `
			SELECT annee, 1000.0*sum(nombre)/nullif(sum(population),0)
			FROM mv.securite_dept_annee WHERE indicateur_code=$1
			GROUP BY 1 ORDER BY 1`, code)
		if err != nil {
			return nil, err
		}
		for srows.Next() {
			var p PointYear
			var v *float64
			if err := srows.Scan(&p.Year, &v); err != nil {
				break
			}
			if v != nil {
				p.Value = *v
				ind.Series = append(ind.Series, p)
			}
		}
		srows.Close()
		if n := len(ind.Series); n > 0 {
			ind.National = ind.Series[n-1].Value
			ind.Page.Series = ind.Series
			ind.Page.SeriesLegend = "Taux national, faits pour 1 000 habitants"
			ind.Page.Curve = curve(ind.Series, tx)
		}
		_ = pool.QueryRow(ctx, `
			SELECT n_masque, n_diffuse + n_masque
			FROM mv.commune_delinquance_national WHERE indicateur_code=$1 AND annee=$2`,
			code, st.Year).Scan(&ind.Masks, &ind.Published)
		st.Indicators = append(st.Indicators, ind)
	}
	// Ordre stable : du fait le plus fréquent au plus rare.
	for i := 0; i < len(st.Indicators); i++ {
		for j := i + 1; j < len(st.Indicators); j++ {
			if st.Indicators[j].National > st.Indicators[i].National {
				st.Indicators[i], st.Indicators[j] = st.Indicators[j], st.Indicators[i]
			}
		}
	}
	for i := range st.Indicators {
		ind := &st.Indicators[i]
		ind.Page.Note = "Les communes dont le SSMSI ne diffuse pas la valeur sont " +
			"exclues du calcul, jamais comptées comme zéro : sous un certain " +
			"nombre de faits, publier reviendrait à identifier les personnes."
		for _, other := range st.Indicators {
			if other.Slug != ind.Slug {
				ind.Page.Neighboring = append(ind.Page.Neighboring,
					LinkMap{Slug: other.Slug, Title: other.Label})
			}
		}
	}
	return st, nil
}
