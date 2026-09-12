package main

import (
	"fmt"
	"html/template"
	"math"
	"strconv"
	"strings"
)

// Dimensions dont les pôles correspondent à l'axe gauche-droite. Les autres
// dimensions de CHES — GAL-TAN, position sur l'UE, immigration — ne sont PAS
// des axes gauche-droite : leur appliquer un dégradé rouge-bleu ferait croire
// à une équivalence qui n'existe pas, et c'est précisément la confusion que la
// littérature comparée s'attache à défaire.
var dimensionsGaucheDroite = map[string]bool{"lrgen": true, "lrecon": true}

var polesDe = map[string][2]string{
	"lrgen":            {"gauche", "droite"},
	"lrecon":           {"gauche économique", "droite économique"},
	"galtan":           {"GAL — libertés, minorités, écologie", "TAN — tradition, autorité, nation"},
	"eu_position":      {"opposition à l'intégration européenne", "soutien à l'intégration européenne"},
	"immigrate_policy": {"politique ouverte", "politique restrictive"},
}

// Jauge produit un demi-anneau gradué de 0 à 10 avec une aiguille à la valeur.
//
// Convention de couleurs FRANÇAISE : la gauche est rouge, la droite est bleue.
// C'est l'inverse de la convention anglo-américaine, et se tromper de sens sur
// un site politique français serait une erreur de fond, pas de style.
//
// Les dimensions qui ne sont pas un axe gauche-droite reçoivent un dégradé
// neutre à une seule teinte : la couleur n'y encode qu'une intensité de
// position sur l'axe nommé, jamais une famille politique.
func Jauge(dimension, valeur string) template.HTML {
	v, err := strconv.ParseFloat(strings.Replace(valeur, ",", ".", 1), 64)
	if err != nil || v < 0 || v > 10 {
		return ""
	}
	id := "g" + dimension
	politique := dimensionsGaucheDroite[dimension]

	var stops string
	if politique {
		stops = `<stop offset="0%" stop-color="#C0392B"/>` +
			`<stop offset="50%" stop-color="#B9B4AC"/>` +
			`<stop offset="100%" stop-color="#1F5FA8"/>`
	} else {
		stops = `<stop offset="0%" stop-color="#CFE6EC"/>` +
			`<stop offset="100%" stop-color="#175A6B"/>`
	}

	// Angle : 180° à gauche (0) vers 0° à droite (10).
	a := (180 - v*18) * math.Pi / 180
	const cx, cy = 100.0, 96.0
	x1, y1 := cx+46*math.Cos(a), cy-46*math.Sin(a)
	x2, y2 := cx+80*math.Cos(a), cy-80*math.Sin(a)

	poles := polesDe[dimension]
	svg := fmt.Sprintf(`<svg class="jauge" viewBox="0 0 200 116" role="img"
 aria-label="%s sur 10, de %s à %s">
<defs><linearGradient id="%s" x1="0" x2="1">%s</linearGradient></defs>
<path d="M 20 96 A 80 80 0 0 1 180 96" fill="none" stroke="url(#%s)" stroke-width="17"/>
<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="var(--encre)" stroke-width="3.2" stroke-linecap="round"/>
<circle cx="%g" cy="%g" r="4.4" fill="var(--encre)"/>
<text x="20" y="112" font-size="9" fill="currentColor" opacity=".65">0</text>
<text x="180" y="112" font-size="9" fill="currentColor" opacity=".65" text-anchor="end">10</text>
<text x="100" y="66" font-size="19" font-weight="650" text-anchor="middle" fill="var(--encre)">%s</text>
</svg>`,
		template.HTMLEscapeString(valeur),
		template.HTMLEscapeString(poles[0]), template.HTMLEscapeString(poles[1]),
		id, stops, id, x1, y1, x2, y2, cx, cy, template.HTMLEscapeString(valeur))
	return template.HTML(svg)
}

// PoleGauche et PoleDroit nomment littéralement les extrémités : un axe nommé
// par un jugement ferait perdre le procès en neutralité, nommé par son contenu
// il le rend sans objet.
func PoleGauche(dimension string) string { return polesDe[dimension][0] }
func PoleDroit(dimension string) string  { return polesDe[dimension][1] }
