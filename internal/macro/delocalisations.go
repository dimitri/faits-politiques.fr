package macro

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

var SourceDelocalisationsInsee = archive.Source{
	Slug: "insee-delocalisations-2022", Label: "Insee — délocalisations d'unités légales et d'emplois, 1995-2017",
	Publisher: "Institut national de la statistique et des études économiques (Insee)", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : Insee, Ésane et CAM ; DGDDI, Douanes — \"Les entreprises en France\", édition 2022",
	Cadence:     "ponctuelle",
	Notes: "Des unités légales et des emplois DÉTECTÉS comme délocalisés par un modèle statistique " +
		"(régression logistique, forêt aléatoire, XGBoost — aire sous la courbe ROC de 0,54 à 0,80 selon la " +
		"méthode), pas un comptage administratif : trois scénarios bas/central/haut publiés tels quels, jamais " +
		"réduits à un seul chiffre certain. Champ : France, secteurs principalement marchands (hors agriculture " +
		"et finance), entreprises de 50 salariés ou plus. Rupture de série documentée par l'Insee en 2001 pour " +
		"les emplois ETP (Figure 4), d'où l'absence de données antérieures sur cette série précise.",
}

const urlDelocalisationsInsee = "https://www.insee.fr/fr/statistiques/fichier/6667029/ENTFRA22_D4.xlsx"

// anneeCourte convertit une date au format "01-01-95" en année à 4 chiffres :
// la période couverte (1995-2017) ne traverse jamais le siècle, donc la règle
// simple (yy<=30 -> 2000+yy, sinon 1900+yy) suffit et n'a pas besoin d'être
// plus générale.
func anneeCourte(s string) (int, error) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) != 3 {
		return 0, fmt.Errorf("date illisible : %q", s)
	}
	yy, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("année illisible dans %q : %w", s, err)
	}
	if yy <= 30 {
		return 2000 + yy, nil
	}
	return 1900 + yy, nil
}

func nombreDelocalisation(s string) (float64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0, fmt.Errorf("valeur vide")
	}
	return strconv.ParseFloat(s, 64)
}

// departementsMetropoleOrdre : les 96 départements métropolitains dans
// l'ordre officiel numérique-alphabétique de l'Insee (01 à 19, puis 2A, 2B,
// puis 21 à 95) — utilisé pour ré-associer la Figure 6 du fichier Insee, dont
// la colonne de code se décale d'une ligne entre "19 Corrèze" et "30 Gard"
// (un défaut du tableur source : la Corse y occupe deux lignes, 2A et 2B, ce
// que la numérotation à décalage fixe de la colonne ne représente pas
// correctement). Les NOMS de département de cette plage restent, eux, dans
// le bon ordre — c'est sur eux que s'appuie le rapprochement, avec un
// contrôle de cohérence nom par nom plutôt qu'une confiance aveugle en la
// position.
var departementsMetropoleOrdre = []struct{ Code, Nom string }{
	{"01", "Ain"}, {"02", "Aisne"}, {"03", "Allier"}, {"04", "Alpes-de-Haute-Provence"},
	{"05", "Hautes-Alpes"}, {"06", "Alpes-Maritimes"}, {"07", "Ardèche"}, {"08", "Ardennes"},
	{"09", "Ariège"}, {"10", "Aube"}, {"11", "Aude"}, {"12", "Aveyron"}, {"13", "Bouches-du-Rhône"},
	{"14", "Calvados"}, {"15", "Cantal"}, {"16", "Charente"}, {"17", "Charente-Maritime"},
	{"18", "Cher"}, {"19", "Corrèze"}, {"2A", "Corse-du-Sud"}, {"2B", "Haute-Corse"},
	{"21", "Côte-d'Or"}, {"22", "Côtes-d'Armor"}, {"23", "Creuse"}, {"24", "Dordogne"},
	{"25", "Doubs"}, {"26", "Drôme"}, {"27", "Eure"}, {"28", "Eure-et-Loir"}, {"29", "Finistère"},
	{"30", "Gard"}, {"31", "Haute-Garonne"}, {"32", "Gers"}, {"33", "Gironde"}, {"34", "Hérault"},
	{"35", "Ille-et-Vilaine"}, {"36", "Indre"}, {"37", "Indre-et-Loire"}, {"38", "Isère"},
	{"39", "Jura"}, {"40", "Landes"}, {"41", "Loir-et-Cher"}, {"42", "Loire"}, {"43", "Haute-Loire"},
	{"44", "Loire-Atlantique"}, {"45", "Loiret"}, {"46", "Lot"}, {"47", "Lot-et-Garonne"},
	{"48", "Lozère"}, {"49", "Maine-et-Loire"}, {"50", "Manche"}, {"51", "Marne"},
	{"52", "Haute-Marne"}, {"53", "Mayenne"}, {"54", "Meurthe-et-Moselle"}, {"55", "Meuse"},
	{"56", "Morbihan"}, {"57", "Moselle"}, {"58", "Nièvre"}, {"59", "Nord"}, {"60", "Oise"},
	{"61", "Orne"}, {"62", "Pas-de-Calais"}, {"63", "Puy-de-Dôme"}, {"64", "Pyrénées-Atlantiques"},
	{"65", "Hautes-Pyrénées"}, {"66", "Pyrénées-Orientales"}, {"67", "Bas-Rhin"}, {"68", "Haut-Rhin"},
	{"69", "Rhône"}, {"70", "Haute-Saône"}, {"71", "Saône-et-Loire"}, {"72", "Sarthe"},
	{"73", "Savoie"}, {"74", "Haute-Savoie"}, {"75", "Paris"}, {"76", "Seine-Maritime"},
	{"77", "Seine-et-Marne"}, {"78", "Yvelines"}, {"79", "Deux-Sèvres"}, {"80", "Somme"},
	{"81", "Tarn"}, {"82", "Tarn-et-Garonne"}, {"83", "Var"}, {"84", "Vaucluse"}, {"85", "Vendée"},
	{"86", "Vienne"}, {"87", "Haute-Vienne"}, {"88", "Vosges"}, {"89", "Yonne"},
	{"90", "Territoire de Belfort"}, {"91", "Essonne"}, {"92", "Hauts-de-Seine"},
	{"93", "Seine-Saint-Denis"}, {"94", "Val-de-Marne"}, {"95", "Val-d'Oise"},
}

// normaliserNomDept réduit un nom de département à ses lettres et chiffres en
// minuscules, accents supprimés, pour comparer "Île-et-Vilaine" (tel qu'écrit
// dans la Figure 6, avec une coquille de l'Insee) à "Ille-et-Vilaine"
// (l'orthographe officielle) sans dépendre d'une correspondance exacte de
// ponctuation ou d'accentuation.
func normaliserNomDept(s string) string {
	var b strings.Builder
	for _, r := range s {
		r = unicode.ToLower(r)
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == 'è', r == 'é', r == 'ê', r == 'ë':
			b.WriteRune('e')
		case r == 'à', r == 'â':
			b.WriteRune('a')
		case r == 'î', r == 'ï':
			b.WriteRune('i')
		case r == 'ô':
			b.WriteRune('o')
		case r == 'ù', r == 'û':
			b.WriteRune('u')
		case r == 'ç':
			b.WriteRune('c')
		}
	}
	out := b.String()
	if out == "ileetvilaine" {
		out = "illeetvilaine" // coquille connue de la Figure 6 ("Île" pour "Ille")
	}
	return out
}

// IngestDelocalisationsInsee charge les Figures 2, 4, 6 et 7 de l'étude Insee
// sur les délocalisations d'unités légales et d'emplois, 1995-2017.
func IngestDelocalisationsInsee(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceDelocalisationsInsee)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "delocalisations-insee-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlDelocalisationsInsee, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wb, err := excelize.OpenFile(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur illisible : %w", err))
	}
	defer wb.Close()

	// --- Figures 2 et 4 : unités légales et emplois ETP par année et par
	// scénario, fusionnées par année (seule la colonne "Détection des
	// délocalisations" de chaque scénario est retenue, pas la borne
	// inférieure ni l'écart-type publiés à côté).
	annuel := map[int]*[6]*int{} // annee -> [ul_bas, ul_central, ul_haut, etp_bas, etp_central, etp_haut]
	get := func(annee int) *[6]*int {
		if annuel[annee] == nil {
			annuel[annee] = &[6]*int{}
		}
		return annuel[annee]
	}
	lireTriScenarios := func(feuille string, ligneDebut int, decalage int) error {
		rows, err := wb.GetRows(feuille)
		if err != nil {
			return fmt.Errorf("%s : %w", feuille, err)
		}
		if len(rows) < ligneDebut {
			return fmt.Errorf("%s : moins de %d lignes", feuille, ligneDebut)
		}
		n := 0
		for i, r := range rows[ligneDebut-1:] {
			if len(r) < 10 {
				break // fin du tableau : les lignes de notes suivent, avec moins de colonnes
			}
			annee, err := anneeCourte(r[0])
			if err != nil {
				return fmt.Errorf("%s, ligne %d : %w", feuille, ligneDebut+i, err)
			}
			for s, col := range []int{1, 4, 7} { // bas, central, haut : "Détection des délocalisations"
				v, err := nombreDelocalisation(r[col])
				if err != nil {
					return fmt.Errorf("%s, ligne %d, scénario %d : %w", feuille, ligneDebut+i, s, err)
				}
				vi := int(v)
				get(annee)[decalage+s] = &vi
			}
			n++
		}
		if n == 0 {
			return fmt.Errorf("%s : aucune ligne lue", feuille)
		}
		return nil
	}
	if err := lireTriScenarios("Figure 2", 5, 0); err != nil {
		return fail(err)
	}
	if err := lireTriScenarios("Figure 4", 5, 3); err != nil {
		return fail(err)
	}

	var lignesAnnuelles [][]any
	for annee, v := range annuel {
		lignesAnnuelles = append(lignesAnnuelles, []any{annee, v[0], v[1], v[2], v[3], v[4], v[5], srcID})
	}

	// --- Figure 6 : cumul départemental 1995-2017, rapproché par nom de
	// département (voir departementsMetropoleOrdre).
	fig6, err := wb.GetRows("Figure 6")
	if err != nil {
		return fail(fmt.Errorf("Figure 6 : %w", err))
	}
	if len(fig6) < 3+len(departementsMetropoleOrdre) {
		return fail(fmt.Errorf("Figure 6 : %d lignes, %d attendues au minimum", len(fig6), 3+len(departementsMetropoleOrdre)))
	}
	var lignesDept [][]any
	for i, dep := range departementsMetropoleOrdre {
		r := fig6[3+i]
		if len(r) < 3 {
			return fail(fmt.Errorf("Figure 6, ligne %d : colonnes manquantes", 4+i))
		}
		// Trois colonnes (code, nom, valeur) — seul le NOM sert au
		// rapprochement, le code affiché n'étant pas fiable sur cette plage
		// (voir le commentaire de departementsMetropoleOrdre).
		nomLu := strings.TrimSpace(r[1])
		if normaliserNomDept(nomLu) != normaliserNomDept(dep.Nom) {
			return fail(fmt.Errorf("Figure 6, ligne %d : nom lu %q ne correspond pas au département attendu %q (%s) — "+
				"le format de la Figure 6 a peut-être changé", 4+i, nomLu, dep.Nom, dep.Code))
		}
		v, err := nombreDelocalisation(r[2])
		if err != nil {
			return fail(fmt.Errorf("Figure 6, ligne %d (%s) : %w", 4+i, dep.Nom, err))
		}
		lignesDept = append(lignesDept, []any{dep.Code, dep.Nom, int(v), srcID})
	}

	// --- Figure 7 : catégorie socioprofessionnelle, lignes 5 à 19 (0-indexé
	// 4 à 18), avant la section "Âge" qui suit dans la même feuille.
	fig7, err := wb.GetRows("Figure 7")
	if err != nil {
		return fail(fmt.Errorf("Figure 7 : %w", err))
	}
	var lignesCSP [][]any
	for i := 4; i < len(fig7); i++ {
		r := fig7[i]
		if len(r) == 1 && strings.TrimSpace(r[0]) == "Âge" {
			break // fin de la section catégorie socioprofessionnelle
		}
		if len(r) < 3 {
			continue
		}
		champGeneral, err := nombreDelocalisation(r[len(r)-2])
		if err != nil {
			return fail(fmt.Errorf("Figure 7, ligne %d : %w", i+1, err))
		}
		posteDeloc, err := nombreDelocalisation(r[len(r)-1])
		if err != nil {
			return fail(fmt.Errorf("Figure 7, ligne %d : %w", i+1, err))
		}
		csp := strings.TrimSpace(strings.Join(r[:len(r)-2], " "))
		if csp == "" {
			return fail(fmt.Errorf("Figure 7, ligne %d : catégorie vide", i+1))
		}
		lignesCSP = append(lignesCSP, []any{csp, champGeneral, posteDeloc, srcID})
	}
	if len(lignesCSP) == 0 {
		return fail(fmt.Errorf("Figure 7 : aucune catégorie socioprofessionnelle lue"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY sur les trois tables : les anciens DELETE
	// (tables entières, ce connecteur en est l'unique propriétaire) payaient
	// le prix des triggers RI pour l'intégralité de chaque table à chaque
	// republication de l'étude Insee, changement ou non — une étude
	// ponctuelle qui ne change quasiment jamais.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_delocalisation_annuelle (
			annee int, unites_legales_bas int, unites_legales_central int, unites_legales_haut int,
			emplois_etp_bas int, emplois_etp_central int, emplois_etp_haut int, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_delocalisation_annuelle"},
		[]string{"annee", "unites_legales_bas", "unites_legales_central", "unites_legales_haut",
			"emplois_etp_bas", "emplois_etp_central", "emplois_etp_haut", "source_id"},
		pgx.CopyFromRows(lignesAnnuelles)); err != nil {
		return fail(fmt.Errorf("core.delocalisation_annuelle : %w", err))
	}
	ctAnnuelle, err := tx.Exec(ctx, `
		MERGE INTO core.delocalisation_annuelle AS tgt
		USING tmp_delocalisation_annuelle AS src
		ON tgt.annee = src.annee
		WHEN MATCHED AND (tgt.unites_legales_bas, tgt.unites_legales_central, tgt.unites_legales_haut,
		                   tgt.emplois_etp_bas, tgt.emplois_etp_central, tgt.emplois_etp_haut, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.unites_legales_bas, src.unites_legales_central, src.unites_legales_haut,
		                   src.emplois_etp_bas, src.emplois_etp_central, src.emplois_etp_haut, src.source_id) THEN
		    UPDATE SET unites_legales_bas = src.unites_legales_bas,
		               unites_legales_central = src.unites_legales_central,
		               unites_legales_haut = src.unites_legales_haut,
		               emplois_etp_bas = src.emplois_etp_bas, emplois_etp_central = src.emplois_etp_central,
		               emplois_etp_haut = src.emplois_etp_haut, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (annee, unites_legales_bas, unites_legales_central, unites_legales_haut,
		            emplois_etp_bas, emplois_etp_central, emplois_etp_haut, source_id)
		    VALUES (src.annee, src.unites_legales_bas, src.unites_legales_central, src.unites_legales_haut,
		            src.emplois_etp_bas, src.emplois_etp_central, src.emplois_etp_haut, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion annuelle : %w", err))
	}

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_delocalisation_departement (
			code_departement text, nom_departement text, emplois_delocalises_1995_2017 int, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_delocalisation_departement"},
		[]string{"code_departement", "nom_departement", "emplois_delocalises_1995_2017", "source_id"},
		pgx.CopyFromRows(lignesDept)); err != nil {
		return fail(fmt.Errorf("core.delocalisation_departement : %w", err))
	}
	ctDept, err := tx.Exec(ctx, `
		MERGE INTO core.delocalisation_departement AS tgt
		USING tmp_delocalisation_departement AS src
		ON tgt.code_departement = src.code_departement
		WHEN MATCHED AND (tgt.nom_departement, tgt.emplois_delocalises_1995_2017, tgt.source_id)
		                  IS DISTINCT FROM (src.nom_departement, src.emplois_delocalises_1995_2017, src.source_id) THEN
		    UPDATE SET nom_departement = src.nom_departement,
		               emplois_delocalises_1995_2017 = src.emplois_delocalises_1995_2017, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (code_departement, nom_departement, emplois_delocalises_1995_2017, source_id)
		    VALUES (src.code_departement, src.nom_departement, src.emplois_delocalises_1995_2017, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion départements : %w", err))
	}

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_delocalisation_csp (
			categorie_socioprofessionnelle text, part_champ_general_pct numeric,
			part_postes_delocalises_pct numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_delocalisation_csp"},
		[]string{"categorie_socioprofessionnelle", "part_champ_general_pct", "part_postes_delocalises_pct", "source_id"},
		pgx.CopyFromRows(lignesCSP)); err != nil {
		return fail(fmt.Errorf("core.delocalisation_csp : %w", err))
	}
	ctCSP, err := tx.Exec(ctx, `
		MERGE INTO core.delocalisation_csp AS tgt
		USING tmp_delocalisation_csp AS src
		ON tgt.categorie_socioprofessionnelle = src.categorie_socioprofessionnelle
		WHEN MATCHED AND (tgt.part_champ_general_pct, tgt.part_postes_delocalises_pct, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.part_champ_general_pct, src.part_postes_delocalises_pct, src.source_id) THEN
		    UPDATE SET part_champ_general_pct = src.part_champ_general_pct,
		               part_postes_delocalises_pct = src.part_postes_delocalises_pct, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (categorie_socioprofessionnelle, part_champ_general_pct, part_postes_delocalises_pct, source_id)
		    VALUES (src.categorie_socioprofessionnelle, src.part_champ_general_pct,
		            src.part_postes_delocalises_pct, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion CSP : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	touchees := ctAnnuelle.RowsAffected() + ctDept.RowsAffected() + ctCSP.RowsAffected()
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"annees": len(lignesAnnuelles), "departements": len(lignesDept), "csp": len(lignesCSP),
		"touchees": touchees,
	}, "")
	fmt.Printf("  Délocalisations Insee : %d années, %d départements, %d catégories socioprofessionnelles (%d touchées par la fusion)\n",
		len(lignesAnnuelles), len(lignesDept), len(lignesCSP), touchees)
	return nil
}
