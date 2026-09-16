package main

import (
	"context"
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
// <!-- schema:seuils-pauvrete --> (cmd/build/main.go).
type SeuilsPauvrete struct {
	Annee           int
	Seuil50, Seuil60 float64
	SVG             template.HTML
}

type pointDecile struct {
	Decile int
	Valeur float64
}

func chargerSeuilsPauvrete(ctx context.Context, pool *pgxpool.Pool) (*SeuilsPauvrete, error) {
	var annee int
	if err := pool.QueryRow(ctx,
		`SELECT max(annee) FROM core.filosofi_decile_national`).Scan(&annee); err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT decile, niveau_vie_mensuel FROM core.filosofi_decile_national
		WHERE annee = $1 ORDER BY decile`, annee)
	if err != nil {
		return nil, err
	}
	var deciles []pointDecile
	for rows.Next() {
		var p pointDecile
		if err := rows.Scan(&p.Decile, &p.Valeur); err != nil {
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

	st := &SeuilsPauvrete{Annee: annee}
	if err := pool.QueryRow(ctx, `
		SELECT max(seuil_euros) FILTER (WHERE seuil_relatif = 0.5),
		       max(seuil_euros) FILTER (WHERE seuil_relatif = 0.6)
		FROM core.pauvrete_seuil_annuel WHERE annee = $1`, annee).
		Scan(&st.Seuil50, &st.Seuil60); err != nil {
		return nil, err
	}

	format := func(v float64) string { return Decimal(v, 0) + " €" }
	st.SVG = dessinerSeuilsPauvrete(deciles, st.Seuil50, st.Seuil60, format)
	return st, nil
}

// dessinerSeuilsPauvrete : neuf barres (les plafonds de chaque décile de
// niveau de vie, D1 à D9, sur une échelle qui part de zéro — jamais tronquée,
// comme partout ailleurs sur ce site) et deux lignes pointillées, aux seuils
// à 50 % et 60 % de la médiane. Les deux sont dans la même unité (€/mois) :
// pas besoin d'une seconde échelle, contrairement à courbeAvecLigne.
func dessinerSeuilsPauvrete(deciles []pointDecile, seuil50, seuil60 float64, format func(float64) string) template.HTML {
	if len(deciles) == 0 {
		return ""
	}
	// mr large : une vraie colonne vide à droite des neuf barres, pour les
	// étiquettes des deux seuils. Sans elle, la ligne d'un seuil traverse la
	// plupart des barres (seule D1 est plus basse que les deux seuils) et
	// aucun endroit du tracé n'est libre pour écrire dessus.
	const w, h, ml, mr, mt, mb = 720.0, 270.0, 8.0, 118.0, 30.0, 30.0
	max := seuil60
	for _, p := range deciles {
		if p.Valeur > max {
			max = p.Valeur
		}
	}
	max *= 1.15 // de l'air au-dessus de la barre la plus haute, pour son étiquette
	// Dix créneaux, pas neuf : le dixième est réservé à D10, qui n'a pas de
	// plafond à dessiner (voir le dégradé plus bas), mais doit garder sa
	// place dans la rangée pour que la lecture "dix tas de 10 %" reste vraie
	// à l'œil, pas seulement dans le texte.
	n := float64(len(deciles) + 1)
	pas := (w - ml - mr) / n
	gap := pas * 0.16
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/max) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe barres-an seuils-pauvrete" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="%s">`, w, h, template.HTMLEscapeString(fmt.Sprintf(
		"Les dix pour cent de Français au niveau de vie le plus bas (premier décile) vivent avec "+
			"%s ou moins par mois ; le seuil de pauvreté à 60%% du niveau de vie médian est à %s ; "+
			"le dixième décile, le plus aisé, n'a pas de plafond connu",
		format(deciles[0].Valeur), format(seuil60))))
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
	ligneSeuil := func(seuil float64, pct string, decale float64) {
		yy := y(seuil)
		fmt.Fprintf(&b, `<line class="ligne-seuil" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			ml, yy, w-mr, yy)
		fmt.Fprintf(&b, `<text class="et-seuil" x="%.1f" y="%.1f">%s — %s</text>`,
			w-mr+8, yy+decale, pct, template.HTMLEscapeString(format(seuil)))
	}
	ligneSeuil(seuil60, "60 %", -7)
	ligneSeuil(seuil50, "50 %", 16)

	for i, p := range deciles {
		x := ml + pas*float64(i) + gap/2
		top := y(p.Valeur)
		// D1 seul plafonne sous le seuil à 60 % : la teinte le distingue des
		// huit autres barres, sans qu'aucun texte n'ait besoin de le répéter.
		cl := "b"
		if p.Valeur < seuil60 {
			cl = "b der"
		}
		fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
			`<title>%d %% des Français ont un niveau de vie inférieur à %s par mois</title></rect>`,
			cl, x, top, pas-gap, (h-mb)-top, (p.Decile)*10, template.HTMLEscapeString(format(p.Valeur)))
		// Une valeur au-dessus de CHAQUE barre : avec seulement neuf barres,
		// contrairement à une série de trente ans, le graphique reste lisible
		// et un lecteur non statisticien n'a besoin de survoler aucune barre
		// pour lire le montant.
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x+(pas-gap)/2, top-6, template.HTMLEscapeString(format(p.Valeur)))
		fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">D%d</text>`,
			x+(pas-gap)/2, h-8, p.Decile)
	}

	// D10 : aucune donnée ne le borne (le neuvième décile est déjà le
	// dernier plafond que la table publie), donc aucune barre de hauteur
	// définie — seulement ce dégradé, du haut de D9 jusqu'au sommet du
	// graphique, pour dire "continue au-delà de ce qui est montré ici".
	xD10 := ml + pas*float64(len(deciles)) + gap/2
	fmt.Fprintf(&b, `<rect class="d10" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="url(#d10fade)">`+
		`<title>Le dixième décile (10&#37; les plus aisés) n'a pas de plafond : son niveau de vie le plus haut n'est pas borné</title></rect>`,
		xD10, mt, pas-gap, (h-mb)-mt)
	fmt.Fprintf(&b, `<text class="et d10-et" x="%.1f" y="%.1f" text-anchor="middle">non borné</text>`,
		xD10+(pas-gap)/2, mt+34)
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">D10</text>`,
		xD10+(pas-gap)/2, h-8)

	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
