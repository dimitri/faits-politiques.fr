package main

import (
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
)

// Images de partage social pour les pages qui portent une vraie carte
// géographique (pas les milliers de pages scrutin/commune/député, qui
// retombent sur web/media/og-defaut.png — voir Layout.Image dans main.go).
// La carte elle-même vient de la base et change d'une construction à
// l'autre ; la rendre à chaque fois, plutôt que de la fabriquer une fois à la
// main, est ce qui garde l'image de partage fidèle aux données affichées.
//
// Les cartes elles-mêmes (fill des polygones) sont déjà en couleurs fixes,
// écrites en attribut par carte.go — seuls le trait de contour et les
// frontières dépendent de var(--xxx), résolues seulement dans un navigateur
// avec la feuille de style chargée. Les jetons du thème clair sont donc
// injectés en dur ici : une image de partage n'a pas de thème sombre.
const cssCarteClair = `
.geo{width:100%;height:auto;display:block}
.geo path{stroke:#FAF8F3;stroke-width:1100;stroke-linejoin:round}
.geo use{stroke:#FAF8F3;stroke-width:3000;stroke-linejoin:round}
.geo.maille .maille-c path{fill:#FFFFFF;stroke:#E4E0D6}
.geo.maille .maille-e path{fill:none;stroke:#125863;stroke-linejoin:round}
.geo .frontieres path{fill:none}
.geo .frontieres .dep{stroke:#17181D;stroke-width:.7px;stroke-opacity:.5}
.geo .frontieres .reg{stroke:#17181D;stroke-width:1.8px;stroke-opacity:.85}
.cartons .geo.carton path{stroke:#FAF8F3;stroke-width:0}
`

var reViewBox = regexp.MustCompile(`viewBox="([^"]+)"`)

// rasterizerCarte convertit une carte SVG (issue de carte.go / carte_maillee.go)
// en PNG, à côté du fichier construit. Utilise rsvg-convert, déjà présent sur
// la machine — pas de nouvelle dépendance dans le dépôt. Renvoie les
// dimensions RÉELLES du PNG produit : la France n'entre pas dans un cadre
// 1200×630 sans déformation (son emprise est presque carrée une fois les
// outre-mer inclus) — mieux vaut le dire exactement à og:image:width/height
// qu'annoncer un ratio que le fichier ne respecte pas.
func rasterizerCarte(svg template.HTML, outPath string) (w, h int, err error) {
	s := string(svg)
	m := reViewBox.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, fmt.Errorf("rasterizerCarte : pas de viewBox dans le SVG")
	}
	vbW, errW := strconv.ParseFloat(vbPart(m[1], 2), 64)
	vbH, errH := strconv.ParseFloat(vbPart(m[1], 3), 64)
	if errW != nil || errH != nil || vbW <= 0 || vbH <= 0 {
		return 0, 0, fmt.Errorf("rasterizerCarte : viewBox illisible %q", m[1])
	}
	const largeur = 1200
	hauteur := int(largeur*vbH/vbW + 0.5)

	// Fond plein plutôt que transparent : une image de partage sur fond
	// blanc du client (Slack, iMessage…) ne doit pas dépendre de son thème.
	fond := fmt.Sprintf(`<rect x="%s" y="%s" width="%s" height="%s" fill="#FAF8F3"/>`,
		vbPart(m[1], 0), vbPart(m[1], 1), vbPart(m[1], 2), vbPart(m[1], 3))
	i := regexp.MustCompile(`<svg[^>]*>`).FindStringIndex(s)
	if i == nil {
		return 0, 0, fmt.Errorf("rasterizerCarte : balise <svg> introuvable")
	}
	s = s[:i[1]] + "<style>" + cssCarteClair + "</style>" + fond + s[i[1]:]

	tmp, err := os.CreateTemp("", "carte-*.svg")
	if err != nil {
		return 0, 0, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(s); err != nil {
		tmp.Close()
		return 0, 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, 0, err
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, 0, err
	}
	cmd := exec.Command("rsvg-convert", "-w", strconv.Itoa(largeur), "-b", "#FAF8F3",
		"-o", outPath, tmp.Name())
	cmbOut, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("rsvg-convert : %w (%s)", err, cmbOut)
	}
	return largeur, hauteur, nil
}

// imageCarte rend une carte en PNG sous out/media/og/<slug>.png et renseigne
// Layout.Image/.ImageW/.ImageH en conséquence — sur erreur, elle n'échoue pas
// la construction (une image de partage manquante n'est jamais une raison de
// ne pas publier une page) : elle journalise et laisse Layout.Image à sa
// valeur par défaut.
func imageCarte(l *Layout, out, slug string, svg template.HTML) {
	w, h, err := rasterizerCarte(svg, filepath.Join(out, "media", "og", slug+".png"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "  avertissement (image de partage %s) : %v\n", slug, err)
		return
	}
	l.Image = "/media/og/" + slug + ".png"
	l.ImageW, l.ImageH = w, h
}

// vbPart extrait le i-ième nombre d'un attribut viewBox ("x y w h").
func vbPart(vb string, i int) string {
	var parts [4]string
	n := 0
	start := 0
	for j := 0; j <= len(vb); j++ {
		if j == len(vb) || vb[j] == ' ' {
			if start < j {
				if n < 4 {
					parts[n] = vb[start:j]
				}
				n++
			}
			start = j + 1
		}
	}
	if i < 4 {
		return parts[i]
	}
	return "0"
}
