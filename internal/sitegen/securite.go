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
type IndicSecurite struct {
	Code, Libelle, Question string
	Slug                    string
	Apercu                  Carte
	Page                    PageCarte
	Serie                   []PointAnnee
	National                float64
	Diffuses, Masques       int
}

type PointAnnee struct {
	Annee  int
	Valeur float64
}

type StatsSecurite struct {
	Indicateurs  []IndicSecurite
	Defs         template.HTML
	Annee, Debut int
	Communes     int
}

var libelleSecurite = map[string][2]string{
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

func loadSecurite(ctx context.Context, pool *pgxpool.Pool) (*StatsSecurite, error) {
	vign, err := jeuContours(ctx, pool, "DEPARTEMENT", tolApercu)
	if err != nil {
		return nil, err
	}
	fin, err := jeuContours(ctx, pool, "DEPARTEMENT", tolPleine)
	if err != nil {
		return nil, err
	}
	st := &StatsSecurite{Annee: 2025, Debut: 2016, Defs: vign.Defs}
	_ = pool.QueryRow(ctx, `SELECT count(DISTINCT commune_code) FROM core.commune_delinquance`).
		Scan(&st.Communes)

	tx := func(v float64) string { return Decimal(v, 1) + " ‰" }

	for code, lib := range libelleSecurite {
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
			WHERE indicateur_code=$1 AND annee=$2`, code, st.Annee)
		if err != nil {
			return nil, err
		}
		var cases []CaseCarte
		for rows.Next() {
			var cc CaseCarte
			var v *float64
			if err := rows.Scan(&cc.Code, &cc.Nom, &v); err != nil {
				rows.Close()
				return nil, err
			}
			if v == nil {
				cc.Absent = true
			} else {
				cc.Valeur = *v
			}
			cases = append(cases, cc)
		}
		rows.Close()

		rangs := classement(cases, vign.Noms, tx)
		ind := IndicSecurite{Code: code, Libelle: lib[0], Question: lib[1],
			Slug:   strings.ReplaceAll(code, "_", "-"),
			Apercu: apercu(vign, cases, "faits pour 1 000 habitants", tx)}
		ind.Page = PageCarte{
			Slug: ind.Slug, Titre: lib[0], Question: lib[1],
			Source:          "SSMSI, bases communales de la délinquance enregistrée",
			Section:         "Sécurité",
			SectionURL:      "securite",
			SectionIndexURL: "securite",
			Carte:           pleine(fin, cases, "faits pour 1 000 habitants", tx),
			Resume:          resumerClassement(rangs),
			Classement:      rangs,
		}

		srows, err := pool.Query(ctx, `
			SELECT annee, 1000.0*sum(nombre)/nullif(sum(population),0)
			FROM mv.securite_dept_annee WHERE indicateur_code=$1
			GROUP BY 1 ORDER BY 1`, code)
		if err != nil {
			return nil, err
		}
		for srows.Next() {
			var p PointAnnee
			var v *float64
			if err := srows.Scan(&p.Annee, &v); err != nil {
				break
			}
			if v != nil {
				p.Valeur = *v
				ind.Serie = append(ind.Serie, p)
			}
		}
		srows.Close()
		if n := len(ind.Serie); n > 0 {
			ind.National = ind.Serie[n-1].Valeur
			ind.Page.Serie = ind.Serie
			ind.Page.SerieLegende = "Taux national, faits pour 1 000 habitants"
			ind.Page.Courbe = courbe(ind.Serie, tx)
		}
		_ = pool.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE NOT diffuse), count(*)
			FROM core.commune_delinquance WHERE indicateur_code=$1 AND annee=$2`,
			code, st.Annee).Scan(&ind.Masques, &ind.Diffuses)
		st.Indicateurs = append(st.Indicateurs, ind)
	}
	// Ordre stable : du fait le plus fréquent au plus rare.
	for i := 0; i < len(st.Indicateurs); i++ {
		for j := i + 1; j < len(st.Indicateurs); j++ {
			if st.Indicateurs[j].National > st.Indicateurs[i].National {
				st.Indicateurs[i], st.Indicateurs[j] = st.Indicateurs[j], st.Indicateurs[i]
			}
		}
	}
	for i := range st.Indicateurs {
		ind := &st.Indicateurs[i]
		ind.Page.Note = "Les communes dont le SSMSI ne diffuse pas la valeur sont " +
			"exclues du calcul, jamais comptées comme zéro : sous un certain " +
			"nombre de faits, publier reviendrait à identifier les personnes."
		for _, autre := range st.Indicateurs {
			if autre.Slug != ind.Slug {
				ind.Page.Voisines = append(ind.Page.Voisines,
					LienCarte{Slug: autre.Slug, Titre: autre.Libelle})
			}
		}
	}
	return st, nil
}
