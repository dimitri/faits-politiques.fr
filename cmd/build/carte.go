package main

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Cartes choroplèthes rendues en SVG par la base.
//
// Aucune bibliothèque de cartographie, aucun serveur de tuiles, aucune requête
// vers un tiers : PostGIS projette et écrit le tracé, le gabarit l'affiche. La
// contrepartie de l'ODbL est que « © les contributeurs OpenStreetMap » doit
// accompagner CHAQUE carte affichée — les gabarits l'incluent donc tous.
//
// La rampe est séquentielle, une seule teinte du clair au foncé : clarté OKLab
// monotone, pas d'au moins 9. Une rampe arc-en-ciel inventerait des ruptures.
var rampe = []string{"#DCE9EC", "#B0CFD5", "#7FB0BA", "#4A8894", "#1E5C69"}

// absentFill : « aucune donnée » n'est pas « la valeur la plus basse ». Un
// département où aucune liste ne s'est présentée n'a pas fait zéro pour cent.
const absentFill = "#EFEBE2"

// Deux niveaux de détail, choisis d'après le nombre de pixels réellement
// dessinés et non « au cas où ». La tolérance est en degrés : 0,04° ≈ 4 km,
// soit un pixel sur une vignette de 260 px de large ; 0,012° ≈ 1,3 km, soit un
// pixel sur une carte de page de détail. Aller plus fin ne change rien à
// l'écran et multiplie le poids par trois.
const (
	tolApercu = 0.04
	tolPleine = 0.012
)

type CaseCarte struct {
	Code, Nom string
	Valeur    float64
	Absent    bool
}

// JeuContours : les tracés d'un niveau de détail, chargés une fois par page.
//
// C'est ce qui rend tenable une page de quinze cartes. /securite/ pesait
// 5,8 Mo parce que chaque carte réécrivait les 96 tracés ; ici ils sont écrits
// une fois dans un <defs> et chaque carte n'est qu'une liste de <use>.
type JeuContours struct {
	Defs    template.HTML
	ViewBox string
	Niveau  string
	Codes   []string
	Noms    map[string]string
	traces  map[string]string
	// Outre-mer : chacun dans SA projection, donc dans son propre repère. Les
	// poser dans le Lambert-93 de l'hexagone leur donnerait une forme et une
	// échelle fausses — la Guyane y ferait la taille d'un timbre déformé.
	outremer []contourSeul
}

type contourSeul struct {
	Code, Nom, Trace, ViewBox string
	SRID                      int
}

// Carton : un outre-mer dessiné à part, à sa propre échelle.
type Carton struct {
	Code, Nom string
	SVG       template.HTML
	Valeur    string
	Absent    bool
}

type Carte struct {
	SVG       template.HTML
	Cartons   []Carton
	Bornes    []string
	Teintes   []string
	Vide      bool
	Unite     string
	Total     int
	NbAbsents int

	classe func(float64) int
	teinte func(int) string
}

// jeuContours charge un niveau territorial. Le préfixe d'identifiant dépend du
// niveau : une page peut afficher une carte des départements et une carte des
// régions, et deux <path id="d11"> se marcheraient dessus.
func jeuContours(ctx context.Context, pool *pgxpool.Pool, niveau string, tolerance float64) (*JeuContours, error) {
	d, codes, vb, noms, err := contours(ctx, pool, niveau, tolerance)
	if err != nil {
		return nil, err
	}
	pre := prefixeNiveau(niveau)
	var b strings.Builder
	for _, c := range codes {
		fmt.Fprintf(&b, `<path id="%s%s" d="%s"/>`, pre, c, d[c])
	}
	om, err := contoursOutreMer(ctx, pool, niveau, tolerance)
	if err != nil {
		return nil, err
	}
	for _, o := range om {
		noms[o.Code] = o.Nom
	}
	return &JeuContours{
		Defs: template.HTML(`<svg width="0" height="0" aria-hidden="true" ` +
			`style="position:absolute"><defs>` + b.String() + `</defs></svg>`),
		ViewBox: vb, Niveau: niveau, Codes: codes, Noms: noms, traces: d,
		outremer: om,
	}, nil
}

// contoursOutreMer : un tracé et une boîte PAR territoire, chacun projeté dans
// le système légal de son territoire (RGAF09, UTM 22N, RGR92…), tel que la
// colonne srid_rendu le nomme.
func contoursOutreMer(ctx context.Context, pool *pgxpool.Pool, niveau string, tolerance float64) (
	[]contourSeul, error) {

	rows, err := pool.Query(ctx, `
		SELECT code_insee, nom, srid_rendu,
		       st_assvg(st_transform(st_simplifypreservetopology(geom,$1), srid_rendu), 1, 0),
		       round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM geo.contour,
		     LATERAL (SELECT st_envelope(st_transform(geom, srid_rendu)) e) x
		WHERE niveau=$2 AND srid_rendu <> 2154
		ORDER BY code_insee`, tolerance, niveau)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contourSeul
	for rows.Next() {
		var c contourSeul
		if err := rows.Scan(&c.Code, &c.Nom, &c.SRID, &c.Trace, &c.ViewBox); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// cartons dessine les outre-mer d'une carte, chacun à son échelle.
func (j *JeuContours) cartons(c Carte, byCode map[string]CaseCarte,
	format func(float64) string) []Carton {

	var out []Carton
	for _, o := range j.outremer {
		cc, ok := byCode[o.Code]
		k := Carton{Code: o.Code, Nom: o.Nom, Absent: !ok || cc.Absent}
		if !k.Absent {
			k.Valeur = format(cc.Valeur)
		} else {
			k.Valeur = "aucune donnée"
		}
		k.SVG = template.HTML(`<svg viewBox="` + o.ViewBox + `" class="geo carton" ` +
			`role="img" aria-label="` + template.HTMLEscapeString(o.Nom+" — "+k.Valeur) +
			`"><path d="` + o.Trace + `" fill="` + c.remplissage(cc) + `"><title>` +
			template.HTMLEscapeString(o.Nom+" — "+k.Valeur) + `</title></path></svg>`)
		out = append(out, k)
	}
	return out
}

func prefixeNiveau(niveau string) string {
	switch niveau {
	case "REGION":
		return "r"
	case "EPCI":
		return "e"
	default:
		return "d"
	}
}

// apercu : la vignette d'une page d'index. Elle renvoie aux tracés du <defs>
// partagé et ne porte AUCUNE infobulle — 96 titres par carte, quinze cartes,
// c'est 90 Ko de texte que personne ne survolera sur une image de 260 px. Le
// détail est à un clic, sur la page de la carte.
func apercu(j *JeuContours, cases []CaseCarte, unite string, format func(float64) string) Carte {
	c := preparer(cases, unite, format)
	if c.Vide {
		return c
	}
	byCode := indexer(cases)
	pre := prefixeNiveau(j.Niveau)
	var b strings.Builder
	for _, code := range j.Codes {
		fmt.Fprintf(&b, `<use href="#%s%s" fill="%s"/>`, pre, code, c.remplissage(byCode[code]))
	}
	c.SVG = j.envelopper(b.String(), true)
	c.Cartons = j.cartons(c, byCode, format)
	return c
}

// pleine : la carte d'une page de détail. Une seule par page, donc les tracés
// y sont écrits en clair, au niveau de détail fin, avec les infobulles.
func pleine(j *JeuContours, cases []CaseCarte, unite string, format func(float64) string) Carte {
	c := preparer(cases, unite, format)
	if c.Vide {
		return c
	}
	byCode := indexer(cases)
	var b strings.Builder
	for _, code := range j.Codes {
		cc, ok := byCode[code]
		titre := j.Noms[code] + " — aucune donnée"
		if ok && !cc.Absent {
			titre = j.Noms[code] + " — " + format(cc.Valeur)
		}
		fmt.Fprintf(&b, `<path d="%s" fill="%s"><title>%s</title></path>`,
			j.traces[code], c.remplissage(cc), template.HTMLEscapeString(titre))
	}
	c.SVG = j.envelopper(b.String(), false)
	c.Cartons = j.cartons(c, byCode, format)
	return c
}

func indexer(cases []CaseCarte) map[string]CaseCarte {
	m := make(map[string]CaseCarte, len(cases))
	for _, c := range cases {
		if c.Code != "" {
			m[c.Code] = c
		}
	}
	return m
}

func (c Carte) remplissage(cc CaseCarte) string {
	if cc.Code == "" || cc.Absent {
		return absentFill
	}
	return c.teinte(c.classe(cc.Valeur))
}

func libelleNiveau(n string) string {
	switch n {
	case "REGION":
		return "région"
	case "EPCI":
		return "intercommunalité"
	default:
		return "département"
	}
}

func (j *JeuContours) envelopper(corps string, apercu bool) template.HTML {
	cl, role := "geo", `role="img" aria-label="Carte de France par `+libelleNiveau(j.Niveau)+`"`
	if apercu {
		// Une vignette décorative : la page de détail porte le contenu, la
		// répéter au lecteur d'écran n'ajoute rien et allonge la liste.
		cl, role = "geo apercu", `aria-hidden="true" focusable="false"`
	}
	return template.HTML(`<svg viewBox="` + j.ViewBox + `" class="` + cl + `" ` + role + `>` +
		corps + `</svg>`)
}

// contours renvoie le tracé SVG de chaque entité métropolitaine d'un niveau,
// en Lambert-93. Les outre-mer ont chacun leur projection et se dessineraient
// en cartons : ils ne partagent pas ce repère, et sont écartés ici.
//
// Le tracé est demandé en coordonnées RELATIVES (st_assvg(..., 1, ...)) : les
// écarts entre points voisins tiennent en deux ou trois chiffres là où une
// abscisse Lambert-93 en demande sept. À tolérance égale, un tiers de poids en
// moins, au pixel près identique.
func contours(ctx context.Context, pool *pgxpool.Pool, niveau string, tolerance float64) (
	map[string]string, []string, string, map[string]string, error) {

	rows, err := pool.Query(ctx, `
		SELECT code_insee, nom,
		       st_assvg(st_transform(st_simplifypreservetopology(geom, $1), 2154), 1, 0)
		FROM geo.contour
		WHERE niveau = $2 AND srid_rendu = 2154
		ORDER BY code_insee`, tolerance, niveau)
	if err != nil {
		return nil, nil, "", nil, err
	}
	defer rows.Close()
	d, noms := map[string]string{}, map[string]string{}
	var codes []string
	for rows.Next() {
		var c, n, p string
		if err := rows.Scan(&c, &n, &p); err != nil {
			return nil, nil, "", nil, err
		}
		d[c], noms[c] = p, n
		codes = append(codes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, "", nil, err
	}
	// La boîte englobante est fixe : elle ne dépend pas des données affichées,
	// donc toutes les cartes du site se superposent exactement.
	var vb string
	// La boîte est celle des DÉPARTEMENTS quel que soit le niveau demandé :
	// régions et départements couvrent le même territoire, et une boîte commune
	// fait que les deux cartes se superposent exactement à l'écran.
	err = pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour
		      WHERE niveau='DEPARTEMENT' AND srid_rendu = 2154) x`).Scan(&vb)
	return d, codes, vb, noms, err
}

// preparer calcule les classes, les bornes et les teintes — la partie commune
// à toutes les formes de rendu. Les classes sont des quintiles sur les seules
// valeurs présentes, jamais plus nombreuses que les valeurs distinctes.
func preparer(cases []CaseCarte, unite string, format func(float64) string) Carte {
	var vals []float64
	var absents int
	for _, c := range cases {
		if c.Absent {
			absents++
		} else {
			vals = append(vals, c.Valeur)
		}
	}
	if len(vals) == 0 {
		return Carte{Vide: true}
	}
	sort.Float64s(vals)

	distinct := 1
	for i := 1; i < len(vals); i++ {
		if vals[i] != vals[i-1] {
			distinct++
		}
	}
	nc := len(rampe)
	if distinct < nc {
		nc = distinct
	}
	seuils := make([]float64, 0, nc-1)
	for i := 1; i < nc; i++ {
		seuils = append(seuils, vals[len(vals)*i/nc])
	}

	c := Carte{Unite: unite, Total: len(vals), NbAbsents: absents}
	c.classe = func(v float64) int {
		for i, s := range seuils {
			if v < s {
				return i
			}
		}
		return nc - 1
	}
	// La rampe garde ses extrêmes : avec trois classes on prend le clair, le
	// médian et le foncé, pas les trois premiers pas.
	c.teinte = func(k int) string {
		if nc == 1 {
			return rampe[len(rampe)-1]
		}
		return rampe[k*(len(rampe)-1)/(nc-1)]
	}

	// Les bornes affichées sont celles des données, pas des nombres ronds
	// inventés : le lecteur doit pouvoir retrouver la classe d'une valeur.
	debut := 0
	for i := 0; i < nc; i++ {
		fin := len(vals)
		if i < nc-1 {
			fin = len(vals) * (i + 1) / nc
		}
		if fin <= debut {
			fin = debut + 1
		}
		if fin > len(vals) {
			fin = len(vals)
		}
		if debut >= len(vals) {
			break
		}
		c.Bornes = append(c.Bornes, format(vals[debut])+" – "+format(vals[fin-1]))
		c.Teintes = append(c.Teintes, c.teinte(i))
		debut = fin
	}
	return c
}
