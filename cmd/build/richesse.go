package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ce qu'il y a dans le dernier décile de niveau de vie (D10) : le dossier
// pauvreté (vieillesse... non, pauvrete-donnees.md) montre D1 à D9 avec un
// dixième décile délibérément « non borné ». Ce dossier ouvre ce dixième
// décile — 1 %, 0,1 %, 0,01 % les plus aisés — et lui adjoint le patrimoine,
// une notion distincte, jamais additionnée au revenu. Voir docs/
// repartition-richesse-donnees.md et la migration 0117.
type StatsRichesse struct {
	Seuils      []SeuilRevenu
	Courbe1Pct  template.HTML
	Courbe01Pct template.HTML
	AnneeDebut, AnneeFin int
	Part1PctDebut, Part1PctFin   float64
	Part01PctDebut, Part01PctFin float64
	Patrimoines []TranchePatrimoine
	AnneePatrimoine int

	// L'héritage comme facteur d'accès à la richesse (§ 4) et la
	// comparaison patrimoine/niveau de vie (§ 5) — même fiche Insee,
	// migration 0118.
	HeriteEnsemble, HeriteHaut     float64
	DonationEnsemble, DonationHaut float64
	GiniPatrimoine, GiniNiveauVie  float64
	Concentrations                 []Concentration
	SVGLorenz                      template.HTML
	Part10PctPatrimoine            float64

	// Population de référence (France métropolitaine, tous âges,
	// core.population_age_departement, migration 0115) : sert à convertir
	// les pourcentages de seuils et de parts en nombres de personnes — un
	// ORDRE DE GRANDEUR, pas le chiffre exact que publierait Filosofi
	// lui-même, dont le champ (ménages fiscaux à revenu positif ou nul)
	// est légèrement plus étroit que la population totale.
	PopulationApprox int
	AnneePopulation  int
	SVGSommetRevenu  template.HTML
	Repartition2021  []PartGroupe
	AnneeRepartitionRecente int

	// Écart concret (en fois) entre le seuil du 0,1 % les plus aisés et la
	// médiane, § 1 — sert à donner un ancrage en euros aux points de
	// pourcentage du § 2, sans reconstituer une masse totale historique
	// (aucune source ne publie la masse des revenus déclarés par UC pour
	// 2004/2013/2018/2021 dans un champ comparable à celui du § 2).
	RatioSommetMediane float64
}

type Concentration struct {
	Position                 string
	MassePatrimoine, MasseNiveauVie float64
}

type PartGroupe struct {
	Libelle, Couleur string
	PartPct          float64
	Personnes        int
}

type SeuilRevenu struct {
	Libelle, Code                string
	RevenuAvant, NiveauDeVie     int
	PartPct                      float64
	Personnes                    int
}

type TranchePatrimoine struct {
	Libelle                        string
	Seuil2015, Seuil2021           int
	Moyen2015, Moyen2021           int
	PartMasse2021                  float64
}

var libelleSeuil = map[string]string{
	"D5": "Médiane (50 %)", "D9": "10 % les plus aisés", "Q99": "1 % les plus aisés",
	"Q99_9": "0,1 % les plus aisés", "Q99_99": "0,01 % les plus aisés",
}
var ordreSeuil = []string{"D5", "D9", "Q99", "Q99_9", "Q99_99"}
var partSeuil = map[string]float64{"D5": 50, "D9": 10, "Q99": 1, "Q99_9": 0.1, "Q99_99": 0.01}

var libellePatrimoine = map[string]string{
	"P90_P95": "Du 90ᵉ au 95ᵉ centile", "P95_P99": "Du 95ᵉ au 99ᵉ centile", "SUP_P99": "Au-delà du 99ᵉ centile",
}
var ordrePatrimoine = []string{"P90_P95", "P95_P99", "SUP_P99"}

func loadRichesse(ctx context.Context, pool *pgxpool.Pool) (*StatsRichesse, error) {
	st := &StatsRichesse{}

	seuilRows, err := pool.Query(ctx, `
		SELECT seuil, revenu_avant_redistribution_eur, niveau_de_vie_eur
		FROM core.filosofi_haut_revenu`)
	if err != nil {
		return nil, err
	}
	parSeuil := map[string]SeuilRevenu{}
	for seuilRows.Next() {
		var code string
		var avant, niveau int
		if err := seuilRows.Scan(&code, &avant, &niveau); err != nil {
			seuilRows.Close()
			return nil, err
		}
		parSeuil[code] = SeuilRevenu{Libelle: libelleSeuil[code], Code: code, RevenuAvant: avant, NiveauDeVie: niveau}
	}
	seuilRows.Close()
	if err := seuilRows.Err(); err != nil {
		return nil, err
	}
	// 2021 : le millésime des seuils de revenu (§ 1) et de la série de parts
	// la plus récente (§ 2) — la population de référence doit être la même
	// année, jamais la plus récente disponible par ailleurs.
	const anneeReference = 2021
	var populationApprox sql.NullInt64
	if err := pool.QueryRow(ctx, `
		SELECT $1::smallint, sum(population) FROM core.population_age_departement
		WHERE code_departement !~ '^97' AND annee = $1`, anneeReference).
		Scan(&st.AnneePopulation, &populationApprox); err != nil {
		return nil, err
	}
	st.PopulationApprox = int(populationApprox.Int64)

	for _, code := range ordreSeuil {
		if s, ok := parSeuil[code]; ok {
			s.PartPct = partSeuil[code]
			s.Personnes = int(s.PartPct / 100 * float64(st.PopulationApprox))
			st.Seuils = append(st.Seuils, s)
		}
	}
	if mediane, ok := parSeuil["D5"]; ok {
		if sommet, ok := parSeuil["Q99_9"]; ok && mediane.RevenuAvant > 0 {
			st.RatioSommetMediane = float64(sommet.RevenuAvant) / float64(mediane.RevenuAvant)
		}
	}

	chargerSerie := func(groupe string) ([]PointAnnee, error) {
		rows, err := pool.Query(ctx, `
			SELECT annee, part_pct FROM core.revenu_part_groupe
			WHERE groupe = $1 ORDER BY annee`, groupe)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var pts []PointAnnee
		for rows.Next() {
			var p PointAnnee
			if err := rows.Scan(&p.Annee, &p.Valeur); err != nil {
				return nil, err
			}
			pts = append(pts, p)
		}
		return pts, rows.Err()
	}
	pct1, err := chargerSerie("1_PLUS_AISES")
	if err != nil {
		return nil, err
	}
	pct01, err := chargerSerie("0_1_PLUS_AISES")
	if err != nil {
		return nil, err
	}
	if len(pct1) > 0 {
		st.AnneeDebut, st.AnneeFin = pct1[0].Annee, pct1[len(pct1)-1].Annee
		st.Part1PctDebut, st.Part1PctFin = pct1[0].Valeur, pct1[len(pct1)-1].Valeur
		st.Courbe1Pct = courbe(pct1, func(v float64) string { return Decimal(v, 1) + " %" })
	}
	if len(pct01) > 0 {
		st.Part01PctDebut, st.Part01PctFin = pct01[0].Valeur, pct01[len(pct01)-1].Valeur
		st.Courbe01Pct = courbe(pct01, func(v float64) string { return Decimal(v, 1) + " %" })
	}

	// Répartition de la population pour l'année la plus récente (2021) des
	// quatre groupes non recouvrants, avec un nombre de personnes approché.
	repRows, err := pool.Query(ctx, `
		SELECT groupe, part_pct FROM core.revenu_part_groupe
		WHERE annee = (SELECT max(annee) FROM core.revenu_part_groupe)
		  AND groupe IN ('90_MODESTES','9_SUIVANTS','0_9_SUIVANTS','0_1_PLUS_AISES')`)
	if err != nil {
		return nil, err
	}
	libelleGroupe := map[string]string{
		"90_MODESTES": "90 % les plus modestes", "9_SUIVANTS": "9 % suivants",
		"0_9_SUIVANTS": "0,9 % suivants", "0_1_PLUS_AISES": "0,1 % les plus aisés",
	}
	couleurGroupe := map[string]string{
		"90_MODESTES": "#DCE9EC", "9_SUIVANTS": "#7FB0BA", "0_9_SUIVANTS": "#1E5C69", "0_1_PLUS_AISES": "#8C4B3A",
	}
	ordreGroupe := []string{"90_MODESTES", "9_SUIVANTS", "0_9_SUIVANTS", "0_1_PLUS_AISES"}
	parGroupe := map[string]float64{}
	for repRows.Next() {
		var g string
		var p float64
		if err := repRows.Scan(&g, &p); err != nil {
			repRows.Close()
			return nil, err
		}
		parGroupe[g] = p
	}
	repRows.Close()
	if err := repRows.Err(); err != nil {
		return nil, err
	}
	st.AnneeRepartitionRecente = st.AnneeFin
	for _, code := range ordreGroupe {
		p := parGroupe[code]
		st.Repartition2021 = append(st.Repartition2021, PartGroupe{
			Libelle: libelleGroupe[code], Couleur: couleurGroupe[code], PartPct: p,
			Personnes: int(p / 100 * float64(st.PopulationApprox)),
		})
	}

	patRows, err := pool.Query(ctx, `
		SELECT tranche, annee, seuil_bas_eur, patrimoine_moyen_eur, part_masse_pct
		FROM core.patrimoine_haut ORDER BY tranche, annee`)
	if err != nil {
		return nil, err
	}
	parTranche := map[string]*TranchePatrimoine{}
	for patRows.Next() {
		var tranche string
		var annee, seuil, moyen int
		var part *float64
		if err := patRows.Scan(&tranche, &annee, &seuil, &moyen, &part); err != nil {
			patRows.Close()
			return nil, err
		}
		t, ok := parTranche[tranche]
		if !ok {
			t = &TranchePatrimoine{Libelle: libellePatrimoine[tranche]}
			parTranche[tranche] = t
		}
		if annee == 2015 {
			t.Seuil2015, t.Moyen2015 = seuil, moyen
		} else {
			t.Seuil2021, t.Moyen2021 = seuil, moyen
			if part != nil {
				t.PartMasse2021 = *part
			}
			st.AnneePatrimoine = annee
		}
	}
	patRows.Close()
	if err := patRows.Err(); err != nil {
		return nil, err
	}
	for _, code := range ordrePatrimoine {
		if t, ok := parTranche[code]; ok {
			st.Patrimoines = append(st.Patrimoines, *t)
		}
	}

	var heriteEnsemble, heriteHaut, donationEnsemble, donationHaut sql.NullFloat64
	if err := pool.QueryRow(ctx, `
		SELECT max(part_herite_pct) FILTER (WHERE categorie='ENSEMBLE'),
		       max(part_herite_pct) FILTER (WHERE categorie='HAUT_PATRIMOINE_ET_NIVEAU_VIE'),
		       max(part_donation_pct) FILTER (WHERE categorie='ENSEMBLE'),
		       max(part_donation_pct) FILTER (WHERE categorie='HAUT_PATRIMOINE_ET_NIVEAU_VIE')
		FROM core.menage_heritage WHERE tranche_age = 'TOUS_AGES'`).
		Scan(&heriteEnsemble, &heriteHaut, &donationEnsemble, &donationHaut); err != nil {
		return nil, err
	}
	st.HeriteEnsemble, st.HeriteHaut = heriteEnsemble.Float64, heriteHaut.Float64
	st.DonationEnsemble, st.DonationHaut = donationEnsemble.Float64, donationHaut.Float64

	// ORDER BY ... LIMIT 1 sur une table pas encore chargée ne renvoie aucune
	// ligne (pgx.ErrNoRows), pas une ligne NULL.
	if err := pool.QueryRow(ctx, `
		SELECT indice_patrimoine, indice_niveau_vie FROM core.gini_patrimoine_niveau_vie
		ORDER BY annee DESC LIMIT 1`).
		Scan(&st.GiniPatrimoine, &st.GiniNiveauVie); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	concRows, err := pool.Query(ctx, `
		SELECT position_distribution, masse_patrimoine_pct, masse_niveau_vie_pct
		FROM core.concentration_patrimoine_niveau_vie
		ORDER BY array_position(ARRAY[
			'Inférieure au 2e décile', 'Inférieure au 4e décile',
			'Inférieure au 5e décile (médiane)', 'Supérieure au 5e décile (médiane)',
			'Supérieure au 8e décile', 'Supérieure au 9e décile'
		], position_distribution)`)
	if err != nil {
		return nil, err
	}
	for concRows.Next() {
		var c Concentration
		if err := concRows.Scan(&c.Position, &c.MassePatrimoine, &c.MasseNiveauVie); err != nil {
			concRows.Close()
			return nil, err
		}
		if strings.HasPrefix(c.Position, "Supérieure au 9") {
			st.Part10PctPatrimoine = c.MassePatrimoine
		}
		st.Concentrations = append(st.Concentrations, c)
	}
	concRows.Close()
	if err := concRows.Err(); err != nil {
		return nil, err
	}
	st.SVGLorenz = dessinerLorenz(st.Concentrations)

	// D9 à Q99,99 seulement : ce chapitre montre ce qu'il y a À L'INTÉRIEUR
	// du dixième décile (D5 est hors sujet ici, déjà dans le dossier
	// pauvreté). Même principe que le graphique des déciles D1-D9 de ce
	// dernier : un dégradé sur la dernière barre, parce que Q99,99 n'a pas
	// plus de plafond connu que D10 lui-même.
	var sommet []SeuilRevenu
	for _, s := range st.Seuils {
		if s.Code != "D5" {
			sommet = append(sommet, s)
		}
	}
	format := func(v float64) string { return Decimal(v, 0) + " €" }
	st.SVGSommetRevenu = dessinerSommetRevenu(sommet, format)

	return st, nil
}

// dessinerSommetRevenu : les seuils D9, Q99, Q99,9, Q99,99 en barres — même
// principe que dessinerSeuilsPauvrete (axe à zéro, jamais tronqué), sans les
// lignes de seuil (qui n'ont pas leur place ici) mais avec le même dégradé
// sur la dernière barre : Q99,99 n'a pas plus de plafond connu que D10.
func dessinerSommetRevenu(seuils []SeuilRevenu, format func(float64) string) template.HTML {
	if len(seuils) == 0 {
		return ""
	}
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 8.0, 8.0, 30.0, 30.0
	max := 0.0
	for _, s := range seuils {
		if float64(s.NiveauDeVie) > max {
			max = float64(s.NiveauDeVie)
		}
	}
	max *= 1.15
	// Une barre de plus, réservée au dégradé au-delà de la dernière valeur
	// connue — le même principe que D10 dans dessinerSeuilsPauvrete.
	n := float64(len(seuils) + 1)
	pas := (w - ml - mr) / n
	gap := pas * 0.16
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe barres-an seuils-pauvrete" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="%s">`, w, h, template.HTMLEscapeString(fmt.Sprintf(
		"Du seuil des 10%% les plus aisés à celui des 0,01%% les plus aisés : de %s à %s",
		format(float64(seuils[0].NiveauDeVie)), format(float64(seuils[len(seuils)-1].NiveauDeVie)))))
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)

	for i, s := range seuils {
		x := ml + pas*float64(i) + gap/2
		top := y(float64(s.NiveauDeVie))
		fmt.Fprintf(&b, `<rect class="b" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
			`<title>%s — %s/an</title></rect>`,
			x, top, pas-gap, (h-mb)-top, s.Libelle, template.HTMLEscapeString(format(float64(s.NiveauDeVie))))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x+(pas-gap)/2, top-6, template.HTMLEscapeString(format(float64(s.NiveauDeVie))))
		fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x+(pas-gap)/2, h-8, template.HTMLEscapeString(strings.TrimPrefix(s.Code, "Q")))
	}

	xFade := ml + pas*float64(len(seuils)) + gap/2
	fmt.Fprintf(&b, `<rect class="d10" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="url(#sommetfade)">`+
		`<title>Au-delà du 0,01%% les plus aisés, aucun seuil publié — le patrimoine et le revenu les plus hauts ne sont pas bornés</title></rect>`,
		xFade, mt, pas-gap, (h-mb)-mt)
	fmt.Fprintf(&b, `<text class="et d10-et" x="%.1f" y="%.1f" text-anchor="middle">non borné</text>`,
		xFade+(pas-gap)/2, mt+34)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">au-delà</text>`,
		xFade+(pas-gap)/2, h-8)

	b.WriteString(`<defs><linearGradient id="sommetfade" x1="0" y1="1" x2="0" y2="0">` +
		`<stop offset="0%" stop-color="#7FB0BA"/>` +
		`<stop offset="75%" stop-color="#7FB0BA" stop-opacity=".35"/>` +
		`<stop offset="100%" stop-color="#7FB0BA" stop-opacity="0"/></linearGradient></defs>`)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// dessinerLorenz : la courbe de Lorenz — la façon la plus reconnue de
// montrer une concentration (population cumulée en abscisse, masse détenue
// cumulée en ordonnée), ici pour le patrimoine ET le niveau de vie sur le
// même repère, contre la diagonale d'égalité parfaite. Les points viennent
// des lignes « Inférieure au Nᵉ décile » de la fiche Insee, complétées par
// les deux extrémités (0,0) et (100,100), vraies par construction.
func dessinerLorenz(conc []Concentration) template.HTML {
	if len(conc) == 0 {
		return ""
	}
	// (population cumulée, patrimoine cumulé, niveau de vie cumulé), déduit
	// des libellés « Inférieure au Nᵉ décile » (population cumulée = le
	// centile) et « Supérieure au Nᵉ décile » (population cumulée = 100 -
	// le centile ; masse cumulée = 100 - la masse « supérieure » publiée).
	type pt struct{ x, patrimoine, niveauVie float64 }
	pts := []pt{{0, 0, 0}}
	for _, c := range conc {
		switch {
		case strings.HasPrefix(c.Position, "Inférieure au 2"):
			pts = append(pts, pt{20, c.MassePatrimoine, c.MasseNiveauVie})
		case strings.HasPrefix(c.Position, "Inférieure au 4"):
			pts = append(pts, pt{40, c.MassePatrimoine, c.MasseNiveauVie})
		case strings.HasPrefix(c.Position, "Inférieure au 5"):
			pts = append(pts, pt{50, c.MassePatrimoine, c.MasseNiveauVie})
		case strings.HasPrefix(c.Position, "Supérieure au 8"):
			pts = append(pts, pt{80, 100 - c.MassePatrimoine, 100 - c.MasseNiveauVie})
		case strings.HasPrefix(c.Position, "Supérieure au 9"):
			pts = append(pts, pt{90, 100 - c.MassePatrimoine, 100 - c.MasseNiveauVie})
		}
	}
	pts = append(pts, pt{100, 100, 100})

	const w, h, m = 400.0, 400.0, 30.0
	side := w - 2*m
	x := func(v float64) float64 { return m + side*v/100 }
	y := func(v float64) float64 { return h - m - side*v/100 }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="lorenz" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Courbe de Lorenz : le patrimoine s'écarte beaucoup plus de l'égalité parfaite que le niveau de vie">`, w, h)
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, m, h-m, w-m, h-m)
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, m, h-m, m, m)
	fmt.Fprintf(&b, `<line class="egalite" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, x(0), y(0), x(100), y(100))

	traceLigne := func(cl string, sel func(pt) float64) {
		var pts2 []string
		for _, p := range pts {
			pts2 = append(pts2, fmt.Sprintf("%.1f,%.1f", x(p.x), y(sel(p))))
		}
		fmt.Fprintf(&b, `<polyline class="%s" points="%s"/>`, cl, strings.Join(pts2, " "))
		for _, p := range pts {
			fmt.Fprintf(&b, `<circle class="%s-pt" cx="%.1f" cy="%.1f" r="3"><title>%d%% des ménages, %s%% de la masse</title></circle>`,
				cl, x(p.x), y(sel(p)), int(p.x), Decimal(sel(p), 1))
		}
	}
	traceLigne("ligne-patrimoine", func(p pt) float64 { return p.patrimoine })
	traceLigne("ligne-niveau-vie", func(p pt) float64 { return p.niveauVie })

	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">0</text>`, x(0), h-m+16)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">100%%</text>`, x(100), h-m+16)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">100%%</text>`, m-6, y(100)+4)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">0</text>`, m-6, y(0)+4)

	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
