package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Le § 1 de docs/pauvrete-donnees.md décrit les seuils à 50 % et 60 % de la
// médiane, et les compare aux déciles du niveau de vie — tout en abstrait,
// en euros cités dans le texte. Ce fichier construit le même repère en
// graphique, à partir des mêmes tables (core.filosofi_decile_national,
// core.pauvrete_seuil_annuel), inséré dans le document par le marqueur
// <!-- schema:seuils-pauvrete --> (internal/sitegen/main.go).
type ThresholdsPoverty struct {
	Year                     int
	Threshold50, Threshold60 float64
	SVG                      template.HTML
}

type pointDecile struct {
	Decile int
	Value  float64
}

func loadThresholdsPoverty(ctx context.Context, pool *pgxpool.Pool) (*ThresholdsPoverty, error) {
	// max(...) est une agrégation : la ligne existe même sans Filosofi encore
	// ingéré, avec une année NULL.
	var yearN sql.NullInt64
	if err := pool.QueryRow(ctx,
		`SELECT max(annee) FROM core.filosofi_decile_national`).Scan(&yearN); err != nil {
		return nil, err
	}
	year := int(yearN.Int64)

	rows, err := pool.Query(ctx, `
		SELECT decile, niveau_vie_mensuel FROM core.filosofi_decile_national
		WHERE annee = $1 ORDER BY decile`, year)
	if err != nil {
		return nil, err
	}
	var deciles []pointDecile
	for rows.Next() {
		var p pointDecile
		if err := rows.Scan(&p.Decile, &p.Value); err != nil {
			rows.Close()
			return nil, err
		}
		deciles = append(deciles, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(deciles) == 0 {
		return nil, nil
	}

	st := &ThresholdsPoverty{Year: year}
	// max(...) FILTER(...) est une agrégation : la ligne existe même sans
	// seuil encore ingéré pour cette année, avec des valeurs NULL.
	var threshold50, threshold60 sql.NullFloat64
	if err := pool.QueryRow(ctx, `
		SELECT max(seuil_euros) FILTER (WHERE seuil_relatif = 0.5),
		       max(seuil_euros) FILTER (WHERE seuil_relatif = 0.6)
		FROM core.pauvrete_seuil_annuel WHERE annee = $1`, year).
		Scan(&threshold50, &threshold60); err != nil {
		return nil, err
	}
	st.Threshold50, st.Threshold60 = threshold50.Float64, threshold60.Float64

	format := func(v float64) string { return Decimal(v, 0) + " €" }
	st.SVG = drawThresholdsPoverty(deciles, st.Threshold50, st.Threshold60, format)
	return st, nil
}

// loadRatePovertySeries : la série 1996-2023 (56 lignes, deux seuils)
// était chargée mais réduite à cinq années récentes en tableau — la
// question posée (« quelle part de la population ») justifie un axe ancré
// à zéro, donc courbePaliers convient tel quel, sans l'adaptation faite pour
// l'âge de départ à la retraite (une variable d'échelle, pas une part).
func loadRatePovertySeries(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, taux_pauvrete_pct FROM core.pauvrete_seuil_annuel
		WHERE seuil_relatif = 0.6 ORDER BY annee`)
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
	brackets := []Bracket{
		{Of: pts[0].Year, A: 2020, Label: "Avant la refonte ERFS"},
		{Of: 2021, A: pts[len(pts)-1].Year, Label: "Après 2021"},
	}
	format := func(v float64) string { return Decimal(v, 1) + " %" }
	return curveBrackets(pts, brackets, format), nil
}

// drawThresholdsPoverty : neuf barres (les plafonds de chaque décile de
// niveau de vie, D1 à D9, sur une échelle qui part de zéro — jamais tronquée,
// comme partout ailleurs sur ce site) et deux lignes pointillées, aux seuils
// à 50 % et 60 % de la médiane. Les deux sont dans la même unité (€/mois) :
// pas besoin d'une seconde échelle, contrairement à courbeAvecLigne.
func drawThresholdsPoverty(deciles []pointDecile, threshold50, threshold60 float64, format func(float64) string) template.HTML {
	if len(deciles) == 0 {
		return ""
	}
	// mr large : une vraie colonne vide à droite des neuf barres, pour les
	// étiquettes des deux seuils. Sans elle, la ligne d'un seuil traverse la
	// plupart des barres (seule D1 est plus basse que les deux seuils) et
	// aucun endroit du tracé n'est libre pour écrire dessus.
	const w, h, ml, mr, mt, mb = 720.0, 270.0, 8.0, 118.0, 30.0, 30.0
	max := threshold60
	for _, p := range deciles {
		if p.Value > max {
			max = p.Value
		}
	}
	max *= 1.15 // de l'air au-dessus de la barre la plus haute, pour son étiquette
	// Dix créneaux, pas neuf : le dixième est réservé à D10, qui n'a pas de
	// plafond à dessiner (voir le dégradé plus bas), mais doit garder sa
	// place dans la rangée pour que la lecture "dix tas de 10 %" reste vraie
	// à l'œil, pas seulement dans le texte.
	n := float64(len(deciles) + 1)
	step := (w - ml - mr) / n
	gap := step * 0.16
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe barres-an seuils-pauvrete" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="%s">`, w, h, template.HTMLEscapeString(fmt.Sprintf(
		"Les dix pour cent de Français au niveau de vie le plus bas (premier décile) vivent avec "+
			"%s ou moins par mois ; le seuil de pauvreté à 60%% du niveau de vie médian est à %s ; "+
			"le dixième décile, le plus aisé, n'a pas de plafond connu",
		format(deciles[0].Value), format(threshold60))))
	// Le dégradé de D10 : la même teinte que les autres barres en bas, vers
	// transparent en haut — une barre qui s'estompe plutôt qu'une barre qui
	// s'arrête, pour qu'« aucun plafond » se voie sans phrase à côté.
	b.WriteString(`<defs><linearGradient id="d10fade" x1="0" y1="1" x2="0" y2="0">` +
		`<stop offset="0%" stop-color="#7FB0BA"/>` +
		`<stop offset="75%" stop-color="#7FB0BA" stop-opacity=".35"/>` +
		`<stop offset="100%" stop-color="#7FB0BA" stop-opacity="0"/></linearGradient></defs>`)
	fmt.Fprintf(&b, `<line class="axe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		ml, h-mb, w-mr, h-mb)

	// Les lignes traversent tout le tracé (jusqu'à w-mr) ; leur étiquette,
	// elle, vit uniquement dans la colonne vide au-delà — jamais superposée à
	// une barre ni au chiffre qui la surmonte. Les deux seuils ne sont qu'à
	// une vingtaine d'euros l'un de l'autre : chaque étiquette est décalée
	// (au-dessus pour 60 %, en dessous pour 50 %) pour ne pas se chevaucher.
	lineThreshold := func(threshold float64, pct string, shifted float64) {
		yy := y(threshold)
		fmt.Fprintf(&b, `<line class="ligne-seuil" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			ml, yy, w-mr, yy)
		fmt.Fprintf(&b, `<text class="et-seuil" x="%.1f" y="%.1f">%s — %s</text>`,
			w-mr+8, yy+shifted, pct, template.HTMLEscapeString(format(threshold)))
	}
	lineThreshold(threshold60, "60 %", -7)
	lineThreshold(threshold50, "50 %", 16)

	for i, p := range deciles {
		x := ml + step*float64(i) + gap/2
		top := y(p.Value)
		// D1 seul plafonne sous le seuil à 60 % : la teinte le distingue des
		// huit autres barres, sans qu'aucun texte n'ait besoin de le répéter.
		cl := "b"
		if p.Value < threshold60 {
			cl = "b der"
		}
		fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
			`<title>%d %% des Français ont un niveau de vie inférieur à %s par mois</title></rect>`,
			cl, x, top, step-gap, (h-mb)-top, (p.Decile)*10, template.HTMLEscapeString(format(p.Value)))
		// Une valeur au-dessus de CHAQUE barre : avec seulement neuf barres,
		// contrairement à une série de trente ans, le graphique reste lisible
		// et un lecteur non statisticien n'a besoin de survoler aucune barre
		// pour lire le montant.
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x+(step-gap)/2, top-6, template.HTMLEscapeString(format(p.Value)))
		fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">D%d</text>`,
			x+(step-gap)/2, h-8, p.Decile)
	}

	// D10 : aucune donnée ne le borne (le neuvième décile est déjà le
	// dernier plafond que la table publie), donc aucune barre de hauteur
	// définie — seulement ce dégradé, du haut de D9 jusqu'au sommet du
	// graphique, pour dire "continue au-delà de ce qui est montré ici".
	xD10 := ml + step*float64(len(deciles)) + gap/2
	fmt.Fprintf(&b, `<rect class="d10" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="url(#d10fade)">`+
		`<title>Le dixième décile (10&#37; les plus aisés) n'a pas de plafond : son niveau de vie le plus haut n'est pas borné</title></rect>`,
		xD10, mt, step-gap, (h-mb)-mt)
	fmt.Fprintf(&b, `<text class="et d10-et" x="%.1f" y="%.1f" text-anchor="middle">non borné</text>`,
		xD10+(step-gap)/2, mt+34)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">D10</text>`,
		xD10+(step-gap)/2, h-8)

	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
