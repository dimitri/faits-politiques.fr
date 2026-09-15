package main

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Le circuit des exonérations de cotisations entre l'État et la Sécurité
// sociale.
//
// Ce schéma n'est PAS un diagramme de Sankey au sens où toute largeur y serait
// une quantité : les cinq canaux entre l'État et la Sécurité sociale ne sont
// publiés sous forme de flux chiffré que dans un PDF annexé au projet de loi
// de finances (le « jaune » Protection sociale) — pas une base machine, donc
// pas une série qu'on ingère et met à jour automatiquement. Mais trois
// largeurs SONT proportionnelles, chacune parce qu'un montant comparable a pu
// en être extrait et vérifié à la main pour une même année : les cotisations
// versées et les exonérations (URSSAF 2022), et la compensation qui leur
// répond (jaune budgétaire annexé au PLF 2024, exécution 2022 — voir
// CompensationTVA/CompensationCiblees). Pour tout le reste, dessiner une
// largeur reviendrait à l'inventer : les montants restent alors ÉCRITS sur
// des flèches simples, chacun avec sa source, et la légende le dit.
//
// Ce que le schéma montre est un fait de droit, pas un jugement : l'État décide
// d'exonérer des cotisations dues à la Sécurité sociale ; la loi du 25 juillet
// 1994 dite loi Veil lui impose de compenser cette perte ; une loi de
// financement peut y déroger, et une partie reste ainsi non compensée.
type CircuitCanaux struct {
	SVG            template.HTML
	AnneeExo       int
	Exonerations   float64
	ExoDebut       float64
	AnneeDebut     int
	AllegementsGen float64
	NonCompense    float64 // Md€, source : jaune budgétaire 2026
	NonCompenseTxt string
	AnneeNonComp   int
	PartTVA        string
	// CompensationTVA + CompensationCiblees, à AnneeCompensation — voir leur
	// commentaire à l'initialisation ci-dessous pour la source exacte de
	// chacun. AnneeCompensation est délibérément 2022, l'année de
	// AnneeCotisationsURSSAF : c'est ce qui permet de dessiner la compensation
	// à la même échelle que les deux bandes déjà proportionnelles.
	CompensationTVA, CompensationCiblees float64
	AnneeCompensation                    int
	SerieExo                             template.HTML
	// Categories : les quatre catégories d'exonération de l'année LA PLUS
	// RÉCENTE (AnneeExo) — sert le tableau et le graphe « année par année »,
	// jamais le schéma lui-même : y dessiner une largeur demanderait un
	// montant comparable pour l'autre bout du flux, que cette année-là n'a pas.
	Categories []CategorieExoneration
	// CategoriesRef : les mêmes quatre catégories, mais pour AnneeCotisationsURSSAF
	// — la seule année où un montant de cotisations EFFECTIVEMENT versées existe
	// à la même source. C'est cette version, plus ancienne, qui est dessinée en
	// largeurs proportionnelles dans le schéma : elle seule a un dénominateur.
	CategoriesRef []CategorieExoneration
	// EmpileesCategories : les mêmes catégories que Categories, empilées sur
	// toute la série 2004-2025 — la dominance des allégements généraux, tracée
	// dans le temps plutôt qu'affirmée pour une seule année.
	EmpileesCategories template.HTML
	// GrandesMesures : les dispositifs eux-mêmes, avec la présidence en
	// exercice à leur création et à leur fin — un REPÈRE CHRONOLOGIQUE, pas une
	// imputation. La base (core.exoneration_cotisation) ne relie aucune mesure
	// à un texte de loi précis : ce lien, quand il existe (loisExonerations),
	// vient d'une recherche à part, vérifiée article par article dans le texte
	// même du Journal officiel — jamais d'un rapprochement de dates ou de noms.
	GrandesMesures []MesureExoneration

	// Ordres de grandeur : trois montants qu'on aimerait mettre sur les trois
	// flèches du circuit, et qui ne peuvent PAS l'être honnêtement — trois
	// sources différentes, trois années différentes, trois périmètres
	// différents. Affichés à part, en barres, avec leur source et leur année
	// écrites sur chaque ligne : c'est la réponse à « peut-on chiffrer les deux
	// autres flèches ? » — oui, approximativement, mais pas sur la même règle
	// que le faisceau proportionnel ci-dessus.
	CotisationsVersees float64
	AnneeCotisations   int
	// Depuis le chargement de core.encaissement_urssaf (0068_urssaf_
	// encaissements.sql), une DEUXIÈME comparaison existe, cette fois sur la
	// MÊME source, le MÊME champ et la MÊME unité que le faisceau des
	// exonérations — seule l'année diffère, parce que l'URSSAF n'a plus mis à
	// jour ses encaissements depuis juillet 2023. C'est la réponse la plus
	// précise que la base puisse donner à « combien les entreprises versent-
	// elles, exactement, à la Sécurité sociale ? ».
	CotisationsVerseesURSSAF float64
	AnneeCotisationsURSSAF   int
	ExonerationsMemeAnnee    float64
	RatioCotisationsExo      string
	BarresOrdreGrandeur      template.HTML
	// RatioMasseSalariale : les exonérations rapportées à la masse salariale
	// du secteur privé (URSSAF), année par année — la lecture insensible à
	// l'inflation que le montant en euros courants, seul, ne permet pas.
	RatioMasseSalariale                template.HTML
	AnneeRatioMSDebut, AnneeRatioMSFin int
	PctRatioMSDebut, PctRatioMSFin     float64
}

type CategorieExoneration struct {
	Libelle string
	Montant float64
	Part    float64
}

// categoriesExoneration : les grandes catégories d'exonération d'une année,
// et leur part du total DE CETTE ANNÉE — jamais d'une autre. Factorisé parce
// que le schéma en a besoin pour deux années différentes : la plus récente
// (le tableau, le graphe annuel) et celle qui a un vrai comparant côté
// cotisations versées (le faisceau proportionnel du schéma lui-même).
func categoriesExoneration(ctx context.Context, pool *pgxpool.Pool, annee int) ([]CategorieExoneration, error) {
	rows, err := pool.Query(ctx, `
		SELECT grande_categorie, sum(montant_eur) FROM core.exoneration_cotisation
		WHERE annee=$1 GROUP BY grande_categorie ORDER BY sum(montant_eur) DESC`, annee)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CategorieExoneration
	var total float64
	for rows.Next() {
		var lib string
		var m float64
		if err := rows.Scan(&lib, &m); err != nil {
			return nil, err
		}
		if i := strings.IndexByte(lib, '_'); i >= 0 {
			lib = lib[i+1:]
		}
		out = append(out, CategorieExoneration{Libelle: lib, Montant: m})
		total += m
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if total > 0 {
			out[i].Part = 100 * out[i].Montant / total
		}
	}
	return out, nil
}

type MesureExoneration struct {
	Code, Libelle, Categorie string
	Debut, Fin               int
	EnCours                  bool
	MontantMax               float64
	AnneeMontantMax          int
	Presidents               []string
	// Serie : le montant annuel du dispositif seul, sur toute sa durée
	// publiée — la « conséquence chiffrée » que la base peut effectivement
	// donner à elle seule, sans le texte de loi.
	Serie template.HTML
	// Loi : le texte qui a créé ce dispositif, quand une recherche dans le
	// corpus du Journal officiel l'a confirmé — voir loisExonerations. Nil
	// pour toute mesure qui n'a pas (encore) été vérifiée ainsi : l'absence
	// ne veut jamais dire « aucune loi », seulement « non recherché ici ».
	Loi *LoiExoneration
}

// LoiExoneration : la loi d'origine d'un dispositif, telle que vérifiée par
// lecture directe de son article dans jo.bloc — jamais déduite de la
// proximité d'une date ou du nom du dispositif. TexteURL n'est renseigné que
// si jo.texte confirme, AU MOMENT DE CETTE CONSTRUCTION, que le texte est
// encore dans le corpus chargé : un identifiant écrit en dur ne doit jamais
// pointer vers une page qui n'existe pas.
type LoiExoneration struct {
	Numero, Date, Article string
	TexteURL              string
}

// loisExonerations : recherche menée en dehors de la base, en remontant du nom
// du dispositif (tel que l'URSSAF le publie) à sa loi, puis en LISANT
// l'article visé dans jo.bloc pour confirmer qu'il crée bien ce dispositif —
// jamais l'inverse. Deux hypothèses de départ, plausibles mais fausses, ont
// été écartées de cette façon : la réduction famille (151) ne vient pas de la
// LFSS 2015 (2014-1554, qui ne fait que RÉFÉRENCER l'article) mais de la loi
// 2014-892 qui le CRÉE ; la réduction maladie (161) ne vient pas de la
// LFSS 2019 (2018-1203) mais de la LFSS 2018 (2017-1836) qui la précède.
// Couvre les sept mesures des « allégements généraux » qui franchissent le
// seuil d'affichage (500 M€ de pic) ; les autres catégories n'ont pas été
// recherchées et n'ont donc pas d'entrée ici.
var loisExonerations = map[string]LoiExoneration{
	"111": {Numero: "2003-47", Date: "17 janvier 2003", Article: "9",
		TexteURL: "JORFTEXT000000594652"},
	"112": {Numero: "2018-1203", Date: "22 décembre 2018", Article: "8",
		TexteURL: "JORFTEXT000037847585"},
	"131": {Numero: "2007-1223", Date: "21 août 2007", Article: "1er",
		TexteURL: "JORFTEXT000000278649"},
	"132": {Numero: "2007-1223", Date: "21 août 2007", Article: "1er",
		TexteURL: "JORFTEXT000000278649"},
	"141": {Numero: "2012-1510", Date: "29 décembre 2012", Article: "66",
		TexteURL: "JORFTEXT000026857857"},
	"151": {Numero: "2014-892", Date: "8 août 2014", Article: "2",
		TexteURL: "JORFTEXT000029349687"},
	"161": {Numero: "2017-1836", Date: "30 décembre 2017", Article: "9",
		TexteURL: "JORFTEXT000036339090"},
}

func loadCircuitCanaux(ctx context.Context, pool *pgxpool.Pool, presidences []Presidence) (*CircuitCanaux, error) {
	c := &CircuitCanaux{
		// Ces deux chiffres ne sont PAS dans la base : ils viennent du jaune
		// budgétaire « Bilan des relations financières entre l'État et la
		// protection sociale » annexé au PLF 2026, et de la LFSS 2026. Ils sont
		// cités tels quels dans docs/budget-donnees.md § 1.3, avec leur source.
		NonCompense: 2.63e9, AnneeNonComp: 2026, PartTVA: "29,05 %",
		// La compensation, pour 2022 — la seule année où elle peut se comparer
		// honnêtement aux deux bandes déjà proportionnelles (cotisations et
		// exonérations URSSAF, elles-mêmes datées 2022 faute de mise à jour
		// depuis). Deux mécanismes, deux lignes du même document :
		//   - CompensationTVA : « TVA nette », art. L. 241-2 du code de la
		//     sécurité sociale, bénéficiaire CNAM/ACOSS — c'est la fraction de
		//     TVA qui compense les allégements généraux « pour solde de tout
		//     compte » depuis 2011. Jaune budgétaire annexé au PLF 2024,
		//     annexe 1, exécution 2022 : 56 972 M€.
		//   - CompensationCiblees : les exonérations ciblées, seules encore
		//     compensées par crédits budgétaires. Même document, tableau 16
		//     (« état des sommes restant dues... au 31/12/2022 »), coût total
		//     de la mesure en 2022 : 7 702 M€.
		// Les deux mécanismes sont disjoints (aucune mesure n'est comptée deux
		// fois) : leur somme est la compensation totale de l'année, pas une
		// estimation. Ce que ce chiffre NE dit pas : combien, sur les 73,5 Md€
		// d'exonérations 2022, reste non compensé — cette soustraction n'est
		// publiée nulle part et ce site ne la calcule pas lui-même (voir
		// AnneeNonComp : le seul chiffre de non-compensation publié est celui
		// de 2026, une mesure différente — les mesures nouvelles décidées
		// cette année-là, pas un solde cumulé).
		CompensationTVA: 56.972e9, CompensationCiblees: 7.702e9, AnneeCompensation: 2022,
	}
	err := pool.QueryRow(ctx, `
		SELECT max(annee), min(annee) FROM core.exoneration_cotisation`).
		Scan(&c.AnneeExo, &c.AnneeDebut)
	if err != nil {
		return nil, err
	}
	_ = pool.QueryRow(ctx, `
		SELECT sum(montant_eur) FILTER (WHERE annee=$1),
		       sum(montant_eur) FILTER (WHERE annee=$2),
		       sum(montant_eur) FILTER (WHERE annee=$1 AND grande_categorie LIKE '1_%')
		FROM core.exoneration_cotisation`, c.AnneeExo, c.AnneeDebut).
		Scan(&c.Exonerations, &c.ExoDebut, &c.AllegementsGen)

	rows, err := pool.Query(ctx, `
		SELECT annee, sum(montant_eur) FROM core.exoneration_cotisation
		GROUP BY annee ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	var pts []PointAnnee
	for rows.Next() {
		var p PointAnnee
		if err := rows.Scan(&p.Annee, &p.Valeur); err != nil {
			break
		}
		pts = append(pts, p)
	}
	rows.Close()
	c.SerieExo = courbe(pts, mdEur)
	c.NonCompenseTxt = Decimal(c.NonCompense/1e9, 2) + "\u202fMd€"

	c.Categories, err = categoriesExoneration(ctx, pool, c.AnneeExo)
	if err != nil {
		return nil, err
	}

	// Les mêmes catégories, empilées année par année — la dominance des
	// allégements généraux n'est affirmée qu'au fil du texte ; ceci la trace.
	// L'ordre des catégories, et leurs teintes, sont ceux de c.Categories
	// (l'année la plus récente) : une catégorie ne change jamais de couleur
	// d'une année à l'autre, même absente certaines années.
	catrows, err := pool.Query(ctx, `
		SELECT annee, grande_categorie, sum(montant_eur) FROM core.exoneration_cotisation
		GROUP BY annee, grande_categorie ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	valeursCat := map[string]map[int]float64{}
	var anneesCat []int
	vuAnnee := map[int]bool{}
	for catrows.Next() {
		var an int
		var cat string
		var m float64
		if err := catrows.Scan(&an, &cat, &m); err != nil {
			catrows.Close()
			return nil, err
		}
		if valeursCat[cat] == nil {
			valeursCat[cat] = map[int]float64{}
		}
		valeursCat[cat][an] = m
		if !vuAnnee[an] {
			vuAnnee[an] = true
			anneesCat = append(anneesCat, an)
		}
	}
	catrows.Close()
	if err := catrows.Err(); err != nil {
		return nil, err
	}
	var seriesCat []SerieEmpilee
	for i, cat := range c.Categories {
		// Categories porte déjà le libellé sans préfixe ; retrouver la clé
		// brute (avec préfixe) suffit à indexer valeursCat construit ci-dessus.
		for code, vals := range valeursCat {
			lib := code
			if j := strings.IndexByte(lib, '_'); j >= 0 {
				lib = lib[j+1:]
			}
			if lib == cat.Libelle {
				seriesCat = append(seriesCat, SerieEmpilee{
					Libelle: cat.Libelle, Couleur: fmt.Sprintf("r%d", i%6), Valeurs: vals,
				})
				break
			}
		}
	}
	// La couleur inline attend un code CSS, pas une classe : on la remplace
	// par la vraie teinte, dans le même ordre que la légende du circuit.
	teintesRuban := []string{"#1E5C69", "#B0763A", "#6B5CA5", "#A34F86", "#3D6FA0", "#4A8894"}
	for i := range seriesCat {
		seriesCat[i].Couleur = teintesRuban[i%6]
	}
	c.EmpileesCategories = barresEmpileesAnnuelles(anneesCat, seriesCat, mdEur)

	// Les grandes mesures elles-mêmes, chronologiquement, avec la présidence en
	// exercice. Seuil à 500 M€ de pic : en-deçà, une mesure ne pèse pas assez
	// pour mériter une ligne dans une liste déjà longue.
	mrows, err := pool.Query(ctx, `
		SELECT code_mesure, mesure, min(annee), max(annee),
		       (array_agg(montant_eur ORDER BY montant_eur DESC))[1],
		       (array_agg(annee ORDER BY montant_eur DESC))[1],
		       max(categorie)
		FROM core.exoneration_cotisation
		GROUP BY code_mesure, mesure HAVING max(montant_eur) > 500e6
		ORDER BY min(annee)`)
	if err != nil {
		return nil, err
	}
	for mrows.Next() {
		var lib string
		var m MesureExoneration
		if err := mrows.Scan(&m.Code, &lib, &m.Debut, &m.Fin, &m.MontantMax, &m.AnneeMontantMax,
			&m.Categorie); err != nil {
			mrows.Close()
			return nil, err
		}
		if i := strings.IndexByte(lib, '_'); i >= 0 {
			lib = lib[i+1:]
		}
		m.Libelle = lib
		if i := strings.IndexByte(m.Categorie, '_'); i >= 0 {
			m.Categorie = m.Categorie[i+1:]
		}
		m.EnCours = m.Fin == c.AnneeExo
		debutISO := fmt.Sprintf("%d-01-01", m.Debut)
		finISO := ""
		if !m.EnCours {
			finISO = fmt.Sprintf("%d-12-31", m.Fin)
		}
		m.Presidents = presidencesDe(presidences, debutISO, finISO)
		c.GrandesMesures = append(c.GrandesMesures, m)
	}
	mrows.Close()
	if err := mrows.Err(); err != nil {
		return nil, err
	}
	for i := range c.GrandesMesures {
		m := &c.GrandesMesures[i]
		srows, err := pool.Query(ctx, `
			SELECT annee, montant_eur FROM core.exoneration_cotisation
			WHERE code_mesure=$1 ORDER BY annee`, m.Code)
		if err != nil {
			return nil, err
		}
		var pts []PointAnnee
		for srows.Next() {
			var p PointAnnee
			if err := srows.Scan(&p.Annee, &p.Valeur); err != nil {
				srows.Close()
				return nil, err
			}
			pts = append(pts, p)
		}
		srows.Close()
		if err := srows.Err(); err != nil {
			return nil, err
		}
		m.Serie = courbe(pts, mdEur)
		if loi, ok := loisExonerations[m.Code]; ok {
			var existe bool
			if err := pool.QueryRow(ctx, `SELECT true FROM jo.texte WHERE id=$1`,
				loi.TexteURL).Scan(&existe); err == nil && existe {
				loi.TexteURL = "https://www.legifrance.gouv.fr/jorf/id/" + loi.TexteURL
				m.Loi = &loi
			}
		}
	}

	// L'ordre de grandeur des cotisations versées : ESSPROS (Eurostat), pas
	// URSSAF — c'est la seule source qui publie un TOTAL de cotisations en
	// euros dans la base, et elle ne partage ni l'année ni exactement le
	// périmètre de l'exonération URSSAF ci-dessus (elle inclut l'assurance
	// chômage et les retraites complémentaires, que la Sécurité sociale au
	// sens strict ne couvre pas).
	_ = pool.QueryRow(ctx, `
		SELECT max(annee) FILTER (WHERE serie_code='protection.financement.cotisations.employeurs')
		FROM core.macro_value`).Scan(&c.AnneeCotisations)
	var cotEmpl, cotProt float64
	_ = pool.QueryRow(ctx, `
		SELECT sum(valeur) FILTER (WHERE serie_code='protection.financement.cotisations.employeurs'),
		       sum(valeur) FILTER (WHERE serie_code='protection.financement.cotisations.protegees')
		FROM core.macro_value WHERE annee=$1`, c.AnneeCotisations).Scan(&cotEmpl, &cotProt)
	c.CotisationsVersees = (cotEmpl + cotProt) * 1e6
	// La comparaison à la même source : encaissements URSSAF des entreprises
	// (secteur privé hors GEN + GEN) contre exonérations URSSAF, TOUTES DEUX
	// pour la dernière année où les encaissements existent — 2022 au moment de
	// l'écriture, pas 2025. Comparer des chiffres de la même source à des
	// années différentes serait aussi trompeur que de mélanger les sources.
	if err := pool.QueryRow(ctx, `
		SELECT max(annee) FROM core.encaissement_urssaf`).Scan(&c.AnneeCotisationsURSSAF); err != nil {
		return nil, err
	}
	if err := pool.QueryRow(ctx, `
		SELECT coalesce(sum(montant_eur) FILTER (WHERE categorie_entreprise), 0)
		  FROM core.encaissement_urssaf WHERE annee = $1`, c.AnneeCotisationsURSSAF).
		Scan(&c.CotisationsVerseesURSSAF); err != nil {
		return nil, err
	}
	if err := pool.QueryRow(ctx, `
		SELECT coalesce(sum(montant_eur), 0) FROM core.exoneration_cotisation
		 WHERE annee = $1`, c.AnneeCotisationsURSSAF).Scan(&c.ExonerationsMemeAnnee); err != nil {
		return nil, err
	}
	if c.ExonerationsMemeAnnee > 0 {
		c.RatioCotisationsExo = Decimal(c.CotisationsVerseesURSSAF/c.ExonerationsMemeAnnee, 1)
	}
	c.CategoriesRef, err = categoriesExoneration(ctx, pool, c.AnneeCotisationsURSSAF)
	if err != nil {
		return nil, err
	}

	c.BarresOrdreGrandeur = barresOrdreGrandeur([]ligneOrdreGrandeur{
		{"Cotisations versées par les entreprises", c.CotisationsVerseesURSSAF,
			fmt.Sprintf("URSSAF, encaissements %d — même source et même champ que les exonérations ci-dessous", c.AnneeCotisationsURSSAF)},
		{fmt.Sprintf("Exonérations décidées par l'État (%d)", c.AnneeCotisationsURSSAF), c.ExonerationsMemeAnnee,
			fmt.Sprintf("URSSAF, %d — champ secteur privé", c.AnneeCotisationsURSSAF)},
		{"Cotisations versées (employeurs + assurés, tous régimes)", c.CotisationsVersees,
			fmt.Sprintf("ESSPROS/Eurostat, %d — champ protection sociale, plus large et plus récent", c.AnneeCotisations)},
		{"Exonérations décidées par l'État", c.Exonerations,
			fmt.Sprintf("URSSAF, %d — champ secteur privé", c.AnneeExo)},
		{"dont non compensé", c.NonCompense,
			fmt.Sprintf("jaune budgétaire, %d", c.AnneeNonComp)},
	})

	// Les exonérations rapportées à la masse salariale du secteur privé
	// (URSSAF, même champ) : un ratio, insensible à l'inflation — contrairement
	// au montant en euros courants de la série ci-dessus. Seules les années où
	// la masse salariale a ses quatre trimestres entrent dans le calcul : une
	// année incomplète sous-estimerait le dénominateur.
	rmrows, err := pool.Query(ctx, `
		SELECT e.annee, e.tot, m.brut
		FROM (SELECT annee, sum(montant_eur) tot FROM core.exoneration_cotisation GROUP BY annee) e
		JOIN (SELECT annee, sum(brut_50j_eur) brut FROM core.masse_salariale
		      WHERE perimetre='SECTEUR_PRIVE_URSSAF' GROUP BY annee HAVING count(*)=4) m
		  ON m.annee=e.annee
		ORDER BY e.annee`)
	if err != nil {
		return nil, err
	}
	var ratioMS []PointAnnee
	for rmrows.Next() {
		var an int
		var exo, brut float64
		if err := rmrows.Scan(&an, &exo, &brut); err != nil {
			rmrows.Close()
			return nil, err
		}
		if brut > 0 {
			ratioMS = append(ratioMS, PointAnnee{Annee: an, Valeur: 100 * exo / brut})
		}
	}
	rmrows.Close()
	if err := rmrows.Err(); err != nil {
		return nil, err
	}
	if n := len(ratioMS); n > 0 {
		c.AnneeRatioMSDebut, c.AnneeRatioMSFin = ratioMS[0].Annee, ratioMS[n-1].Annee
		c.PctRatioMSDebut, c.PctRatioMSFin = ratioMS[0].Valeur, ratioMS[n-1].Valeur
		c.RatioMasseSalariale = courbe(ratioMS, func(v float64) string { return Decimal(v, 1) + " %" })
	}

	c.SVG = c.dessiner()
	return c, nil
}

type ligneOrdreGrandeur struct {
	Libelle string
	Valeur  float64
	Source  string
}

// barresOrdreGrandeur : des barres à échelle commune, mais dont les montants
// viennent de trois sources et de trois années qui ne coïncident pas — chaque
// ligne écrit la sienne, pour que personne ne les prenne pour trois mesures
// d'une même chose.
func barresOrdreGrandeur(lignes []ligneOrdreGrandeur) template.HTML {
	var max float64
	for _, l := range lignes {
		if l.Valeur > max {
			max = l.Valeur
		}
	}
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="barres barres-og">`)
	for _, l := range lignes {
		fmt.Fprintf(&b, `<div class="ligne"><span class="n">%s<span class="src-ligne">%s</span></span>`+
			`<span class="piste"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%s</span></div>`,
			template.HTMLEscapeString(l.Libelle), template.HTMLEscapeString(l.Source),
			100*l.Valeur/max, mdEur(l.Valeur))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func (c *CircuitCanaux) dessiner() template.HTML {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	totalDu := c.CotisationsVerseesURSSAF + c.ExonerationsMemeAnnee
	w(`<svg class="circuit" viewBox="0 0 760 430" role="img" aria-labelledby="circuit-t circuit-d">`)
	w(`<title id="circuit-t">Le circuit des exonérations de cotisations</title>`)
	compensationTotale := c.CompensationTVA + c.CompensationCiblees
	w(`<desc id="circuit-d">Les employeurs doivent des cotisations à la Sécurité sociale. En %d, `+
		`%s de cotisations dues se répartissent, à l'échelle, en %s effectivement versés et %s `+
		`exonérés par l'État — ces derniers détaillés en quatre catégories. La loi Veil de 1994 `+
		`oblige l'État à compenser cette perte ; en %d, %s l'ont été, pour l'essentiel par une `+
		`fraction de TVA — à la même échelle que les deux bandes précédentes. %s restent `+
		`officiellement non compensés, mais en %d, une année différente et une mesure différente `+
		`(les mesures nouvelles décidées cette année-là, pas un solde).</desc>`,
		c.AnneeCotisationsURSSAF, mdEur(totalDu), mdEur(c.CotisationsVerseesURSSAF),
		mdEur(c.ExonerationsMemeAnnee), c.AnneeCompensation, mdEur(compensationTotale),
		Decimal(c.NonCompense/1e9, 2)+" Md€", c.AnneeNonComp)
	w(`<defs><marker id="fl" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">` +
		`<path d="M0 0L10 5L0 10z" class="pointe"/></marker></defs>`)

	boite := func(x, y, wd, h float64, cl, titre, sous string) {
		w(`<rect class="noeud %s" x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="8"/>`, cl, x, y, wd, h)
		w(`<text class="nt" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`,
			x+wd/2, y+h/2-(func() float64 {
				if sous != "" {
					return 4
				}
				return -4
			}()), template.HTMLEscapeString(titre))
		if sous != "" {
			w(`<text class="ns" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`,
				x+wd/2, y+h/2+13, template.HTMLEscapeString(sous))
		}
	}
	fleche := func(d, cl string) { w(`<path class="flux %s" d="%s" marker-end="url(#fl)"/>`, cl, d) }
	etiq := func(x, y float64, anchor, cl, txt string) {
		w(`<text class="et %s" x="%.0f" y="%.0f" text-anchor="%s">%s</text>`,
			cl, x, y, anchor, template.HTMLEscapeString(txt))
	}

	// Les trois acteurs
	boite(20, 170, 170, 70, "", "Employeurs", "cotisations dues")
	boite(570, 170, 170, 70, "secu", "Sécurité sociale", "caisses et régimes")
	boite(295, 20, 170, 70, "etat", "État", "décide les exonérations")

	// Le nœud Employeurs se scinde en DEUX flux proportionnels l'un à l'autre —
	// ce qu'un vrai flux exige, et que le schéma n'avait pas avant l'arrivée de
	// core.encaissement_urssaf (0068) : jusque-là, aucun montant de cotisations
	// EFFECTIVEMENT versées n'existait à la même source que les exonérations.
	// AnneeCotisationsURSSAF est la seule année où les deux existent, à la même
	// source (URSSAF), au même champ (categorie_entreprise), dans la même unité
	// — donc la seule où une largeur commune est honnête.
	hExo := 0.0
	if totalDu > 0 {
		hExo = 70.0 * c.ExonerationsMemeAnnee / totalDu
	}

	// 1. Cotisations effectivement versées — bande pleine, proportionnelle,
	// occupant le bas du bord droit d'Employeurs jusqu'à Sécurité sociale.
	{
		y0, y1 := 170.0+hExo, 240.0
		fmt.Fprintf(&b, `<path class="ruban versee" d="M190,%.1f L566,%.1f L566,%.1f L190,%.1f Z" `+
			`marker-end="url(#fl)"><title>cotisations effectivement versées — %s (%d)</title></path>`,
			y0, y0, y1, y1, mdEur(c.CotisationsVerseesURSSAF), c.AnneeCotisationsURSSAF)
		my := (y0 + y1) / 2
		etiq(378, my-4, "middle", "fort", "cotisations versées "+mdEur(c.CotisationsVerseesURSSAF))
		etiq(378, my+13, "middle", "", fmt.Sprintf("URSSAF, encaissements %d", c.AnneeCotisationsURSSAF))
	}

	// 2. L'exonération — la bande complémentaire, À LA MÊME ÉCHELLE que celle
	// des cotisations versées ci-dessus, elle-même détaillée en quatre
	// catégories (CategoriesRef, calculées pour la même année).
	{
		const x0, x1 = 190.0, 295.0 // Employeurs (bord droit) → État (bord gauche)
		y0Base, y1Base := 170.0+hExo, 90.0
		xm := (x0 + x1) / 2
		cum := 0.0
		for i, cat := range c.CategoriesRef {
			h := hExo * cat.Part / 100
			y0t, y0b := y0Base-cum-h, y0Base-cum
			y1t, y1b := y1Base-cum-h, y1Base-cum
			fmt.Fprintf(&b, `<path class="ruban r%d" d="M%.1f,%.1f C%.1f,%.1f %.1f,%.1f %.1f,%.1f `+
				`L%.1f,%.1f C%.1f,%.1f %.1f,%.1f %.1f,%.1f Z"><title>%s — %s (%s %% des exonérations)</title></path>`,
				i%6, x0, y0t, xm, y0t, xm, y1t, x1, y1t,
				x1, y1b, xm, y1b, xm, y0b, x0, y0b,
				template.HTMLEscapeString(cat.Libelle), template.HTMLEscapeString(mdEur(cat.Montant)),
				Decimal(cat.Part, 1))
			cum += h
		}
	}
	etiq(135, 108, "end", "fort", "exonérations "+mdEur(c.ExonerationsMemeAnnee))
	etiq(135, 126, "end", "", fmt.Sprintf("décidées par la loi — URSSAF, %d", c.AnneeCotisationsURSSAF))
	etiq(135, 143, "end", "", "quatre catégories, largeurs proportionnelles")

	// 3. La compensation, obligation de la loi Veil — à la MÊME ÉCHELLE que les
	// deux bandes ci-dessus depuis que le jaune budgétaire annexé au PLF 2024
	// donne, pour 2022, deux montants qui se comparent honnêtement aux
	// cotisations et exonérations URSSAF de cette même année : la TVA nette
	// affectée (compense les allégements généraux) et les exonérations
	// ciblées compensées par crédits budgétaires (voir le commentaire sur
	// CompensationTVA/CompensationCiblees). L'épaisseur du trait porte donc un
	// vrai chiffre, pas seulement sa couleur — un curseur épais plutôt qu'une
	// flèche fine, seul moyen d'afficher une largeur sur un tracé courbe.
	{
		compensationTotale := c.CompensationTVA + c.CompensationCiblees
		epaisseur := 0.0
		if totalDu > 0 {
			epaisseur = 70.0 * compensationTotale / totalDu
		}
		w(`<path class="flux comp" style="stroke-width:%.1fpx;stroke-linecap:round" `+
			`d="M465 60 C540 70 640 110 650 166" marker-end="url(#fl)">`+
			`<title>compensation par l'État — %s (%d)</title></path>`,
			epaisseur, mdEur(compensationTotale), c.AnneeCompensation)
	}
	etiq(490, 30, "start", "fort", "compensation par l'État "+mdEur(c.CompensationTVA+c.CompensationCiblees))
	etiq(490, 47, "start", "", fmt.Sprintf("loi Veil (1994) — jaune budgétaire, %d", c.AnneeCompensation))
	etiq(490, 64, "start", "", "TVA affectée + exonérations ciblées")

	// 4. Le reste non compensé : la ligne qu'on doit voir
	w(`<rect class="noeud manque" x="275" y="300" width="210" height="104" rx="8"/>`)
	w(`<text class="nt manque-t" x="380" y="336" text-anchor="middle">%s</text>`,
		template.HTMLEscapeString(Decimal(c.NonCompense/1e9, 2)+"\u202fMd€"))
	w(`<text class="ns" x="380" y="358" text-anchor="middle">non compensés en %d</text>`, c.AnneeNonComp)
	w(`<text class="ns" x="380" y="376" text-anchor="middle">perte définitive</text>`)
	w(`<text class="ns" x="380" y="392" text-anchor="middle">pour la Sécurité sociale</text>`)
	fleche("M380 90 L380 296", "manque-f")
	etiq(390, 250, "start", "", "une loi de financement")
	etiq(390, 266, "start", "", "peut déroger à la")
	etiq(390, 282, "start", "", "compensation")

	w(`</svg>`)
	return template.HTML(b.String())
}
