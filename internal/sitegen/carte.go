package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"sort"
	"strconv"
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
var ramp = []string{"#DCE9EC", "#B0CFD5", "#7FB0BA", "#4A8894", "#1E5C69"}

// absentFill : « aucune donnée » n'est pas « la valeur la plus basse ». Un
// département où aucune liste ne s'est présentée n'a pas fait zéro pour cent.
const absentFill = "#EFEBE2"

// Deux niveaux de détail, choisis d'après le nombre de pixels réellement
// dessinés et non « au cas où ». La tolérance est en degrés : 0,04° ≈ 4 km,
// soit un pixel sur une vignette de 260 px de large ; 0,012° ≈ 1,3 km, soit un
// pixel sur une carte de page de détail. Aller plus fin ne change rien à
// l'écran et multiplie le poids par trois.
const (
	toleranceOverview = 0.04
	toleranceFull     = 0.012
	// Un peu plus fin que tolPleine : les grands méandres d'un fleuve
	// (la Loire, en particulier) portent une information de reconnaissance
	// que la tolérance des contours administratifs aplatirait.
	toleranceWatercourse = 0.006
)

// riversSVG rend les grands cours d'eau chargés (geo.cours_eau) comme un
// calque de repère commun à toutes les cartes de France métropolitaine — à
// écrire juste après le fond (départements/communes) et avant les données,
// jamais avant ni après : ni caché sous la terre, ni recouvrant un point ou
// une teinte. `srid` doit être celui déjà utilisé pour le fond de la même
// carte, pour que les deux calques se superposent exactement ; `rel` et
// `digits` doivent reprendre exactement les mêmes valeurs que l'appel
// st_assvg du fond (1,0 en Lambert-93 sur les cartes départementales ; 0,4
// sur les petites cartes en degrés comme la carte des semi-conducteurs ou
// la ligne de démarcation, où une précision à zéro décimale écraserait le
// tracé).
func riversSVG(ctx context.Context, pool *pgxpool.Pool, srid, rel, digits int) (string, error) {
	rows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom, $1), $2::int), $3::int, $4::int)
		FROM geo.cours_eau`, toleranceWatercourse, srid, rel, digits)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return "", err
		}
		if d == "" {
			continue
		}
		fmt.Fprintf(&b, `<path class="fleuve" d="%s"/>`, d)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return b.String(), nil
}

type CellMap struct {
	Code, Name string
	Value      float64
	Absent     bool
}

// SetOutlines : les tracés d'un niveau de détail, chargés une fois par page.
//
// C'est ce qui rend tenable une page de quinze cartes. /securite/ pesait
// 5,8 Mo parce que chaque carte réécrivait les 96 tracés ; ici ils sont écrits
// une fois dans un <defs> et chaque carte n'est qu'une liste de <use>.
type SetOutlines struct {
	Defs    template.HTML
	ViewBox string
	Level   string
	Codes   []string
	Noms    map[string]string
	traces  map[string]string
	// labels : centre du plus grand cercle inscriptible dans chaque contour
	// (ST_MaximumInscribedCircle) — calculé pour tous les niveaux (coût
	// négligeable), mais seul pleine() sur les régions s'en sert : 17 formes
	// assez grandes pour porter un nom, contrairement aux 96 départements ou
	// aux milliers d'EPCI, trop nombreux ou trop petits pour ne pas se
	// chevaucher.
	labels map[string]struct{ X, Y float64 }
	// fleuves : le calque des grands cours d'eau (voir fleuvesSVG), chargé une
	// fois ici plutôt qu'à chaque apercu()/pleine() — département, région ou
	// EPCI partagent tous la même projection (Lambert-93) et donc le même
	// tracé de fleuves, vérifié une fois pour toutes les cartes qui utilisent
	// ce jeu de contours plutôt que pour chacune séparément.
	rivers string
	// Outre-mer : chacun dans SA projection, donc dans son propre repère. Les
	// poser dans le Lambert-93 de l'hexagone leur donnerait une forme et une
	// échelle fausses — la Guyane y ferait la taille d'un timbre déformé.
	overseas []outlineOnly
}

type outlineOnly struct {
	Code, Name, Trace, ViewBox string
	SRID                       int
}

// Tile : un outre-mer dessiné à part, à sa propre échelle.
type Tile struct {
	Code, Name string
	SVG        template.HTML
	Value      string
	Absent     bool
}

type Map struct {
	SVG          template.HTML
	Tiles        []Tile
	Bounds       []string
	Shades       []string
	Empty        bool
	Unit         string
	Total        int
	CountAbsents int

	class func(float64) int
	shade func(int) string
}

// setOutlines charge un niveau territorial. Le préfixe d'identifiant dépend du
// niveau : une page peut afficher une carte des départements et une carte des
// régions, et deux <path id="d11"> se marcheraient dessus.
func setOutlines(ctx context.Context, pool *pgxpool.Pool, level string, tolerance float64) (*SetOutlines, error) {
	d, codes, vb, noms, labels, err := outlines(ctx, pool, level, tolerance)
	if err != nil {
		return nil, err
	}
	pre := prefixLevel(level)
	var b strings.Builder
	for _, c := range codes {
		fmt.Fprintf(&b, `<path id="%s%s" d="%s"/>`, pre, c, d[c])
	}
	om, err := outlinesOverseas(ctx, pool, level, tolerance)
	if err != nil {
		return nil, err
	}
	for _, o := range om {
		noms[o.Code] = o.Name
	}
	// Même projection (Lambert-93, coordonnées relatives) quel que soit le
	// niveau territorial : un seul tracé de fleuves sert aux trois.
	rivers, err := riversSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	return &SetOutlines{
		Defs: template.HTML(`<svg width="0" height="0" aria-hidden="true" ` +
			`style="position:absolute"><defs>` + b.String() + `</defs></svg>`),
		ViewBox: vb, Level: level, Codes: codes, Noms: noms, traces: d, rivers: rivers,
		overseas: om, labels: labels,
	}, nil
}

// outlinesOverseas : un tracé et une boîte PAR territoire, chacun projeté dans
// le système légal de son territoire (RGAF09, UTM 22N, RGR92…), tel que la
// colonne srid_rendu le nomme.
func outlinesOverseas(ctx context.Context, pool *pgxpool.Pool, level string, tolerance float64) (
	[]outlineOnly, error) {

	rows, err := pool.Query(ctx, `
		SELECT code_insee, nom, srid_rendu,
		       st_assvg(st_transform(st_simplifypreservetopology(geom,$1), srid_rendu), 1, 0),
		       round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM geo.contour,
		     LATERAL (SELECT st_envelope(st_transform(geom, srid_rendu)) e) x
		WHERE niveau=$2 AND srid_rendu <> 2154
		ORDER BY code_insee`, tolerance, level)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []outlineOnly
	for rows.Next() {
		var c outlineOnly
		if err := rows.Scan(&c.Code, &c.Name, &c.SRID, &c.Trace, &c.ViewBox); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// tiles dessine les outre-mer d'une carte, chacun à son échelle.
func (j *SetOutlines) tiles(c Map, byCode map[string]CellMap,
	format func(float64) string) []Tile {

	var out []Tile
	for _, o := range j.overseas {
		cc, ok := byCode[o.Code]
		k := Tile{Code: o.Code, Name: o.Name, Absent: !ok || cc.Absent}
		if !k.Absent {
			k.Value = format(cc.Value)
		} else {
			k.Value = "aucune donnée"
		}
		k.SVG = template.HTML(`<svg viewBox="` + o.ViewBox + `" class="geo carton" ` +
			`role="img" aria-label="` + template.HTMLEscapeString(o.Name+" — "+k.Value) +
			`"><path d="` + o.Trace + `" fill="` + c.fill(cc) + `"><title>` +
			template.HTMLEscapeString(o.Name+" — "+k.Value) + `</title></path></svg>`)
		out = append(out, k)
	}
	return out
}

func prefixLevel(level string) string {
	switch level {
	case "REGION":
		return "r"
	case "EPCI":
		return "e"
	default:
		return "d"
	}
}

// overview : la vignette d'une page d'index. Elle renvoie aux tracés du <defs>
// partagé et ne porte AUCUNE infobulle — 96 titres par carte, quinze cartes,
// c'est 90 Ko de texte que personne ne survolera sur une image de 260 px. Le
// détail est à un clic, sur la page de la carte.
func overview(j *SetOutlines, cells []CellMap, unit string, format func(float64) string) Map {
	c := prepare(cells, unit, format)
	if c.Empty {
		return c
	}
	byCode := index(cells)
	pre := prefixLevel(j.Level)
	var b strings.Builder
	for _, code := range j.Codes {
		fmt.Fprintf(&b, `<use href="#%s%s" fill="%s"/>`, pre, code, c.fill(byCode[code]))
	}
	// Les fleuves en dernier : un calque de repère par-dessus les teintes,
	// jamais dessous où la couleur de la donnée les masquerait.
	b.WriteString(j.rivers)
	c.SVG = j.envelopper(b.String(), true)
	c.Tiles = j.tiles(c, byCode, format)
	return c
}

// full : la carte d'une page de détail. Une seule par page, donc les tracés
// y sont écrits en clair, au niveau de détail fin, avec les infobulles.
func full(j *SetOutlines, cells []CellMap, unit string, format func(float64) string) Map {
	c := prepare(cells, unit, format)
	if c.Empty {
		return c
	}
	byCode := index(cells)
	var b strings.Builder
	for _, code := range j.Codes {
		cc, ok := byCode[code]
		title := j.Noms[code] + " — aucune donnée"
		if ok && !cc.Absent {
			title = j.Noms[code] + " — " + format(cc.Value)
		}
		fmt.Fprintf(&b, `<path d="%s" fill="%s"><title>%s</title></path>`,
			j.traces[code], c.fill(cc), template.HTMLEscapeString(title))
	}
	// Les fleuves en dernier : un calque de repère par-dessus les teintes,
	// jamais dessous où la couleur de la donnée les masquerait.
	b.WriteString(j.rivers)
	if j.Level == "REGION" {
		b.WriteString(labelsRegions(j))
	}
	c.SVG = j.envelopper(b.String(), false)
	c.Tiles = j.tiles(c, byCode, format)
	return c
}

// labelsRegions : le nom de chaque région, au centre du plus grand
// cercle inscriptible dans son contour — dix-sept formes assez grandes pour
// porter un nom sans jamais se chevaucher, à la différence des départements
// ou des EPCI (voir le commentaire de JeuContours.labels). Un <text
// font-size="..."> direct à l'échelle du viewBox (plusieurs centaines de
// milliers d'unités Lambert-93) ne rendrait qu'un trait au lieu de lettres
// lisibles (constaté sur la carte d'Europe du dossier Seconde Guerre
// mondiale) — un <g transform="scale(...)"> autour d'un texte à taille
// normale contourne le problème.
func labelsRegions(j *SetOutlines) string {
	// ViewBox peut être la chaîne vide : jeuContours renvoie une boîte vide
	// quand geo.contour est vide (voir son commentaire sur sql.NullString —
	// aucune source du catalogue ne remplit cette table, contours
	// OpenStreetMap de la migration 0058 chargés hors pipeline), et indexer
	// [2] sur zéro champ paniquerait au lieu de ne rien dessiner.
	fields := strings.Fields(j.ViewBox)
	if len(fields) < 3 {
		return ""
	}
	width, _ := strconv.ParseFloat(fields[2], 64)
	if width <= 0 {
		return ""
	}
	scale := width / 700 // ≈ la largeur réelle de la carte à l'écran, en pixels
	var b strings.Builder
	for _, code := range j.Codes {
		l, ok := j.labels[code]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, `<g transform="translate(%.0f,%.0f) scale(%.2f)">`+
			`<text class="nom-region" x="0" y="0" text-anchor="middle">%s</text></g>`,
			l.X, l.Y, scale, template.HTMLEscapeString(j.Noms[code]))
	}
	return b.String()
}

// tilesHTML : même rendu que le template nommé "cartons" (base.gohtml),
// pour les rares cas où une carte s'insère depuis du Go plutôt que depuis un
// gabarit — voir l'injection de carteMedecins dans main.go.
func tilesHTML(tiles []Tile) string {
	if len(tiles) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="cartons"><span class="t">Outre-mer</span>`)
	for _, c := range tiles {
		cl := ""
		if c.Absent {
			cl = "vide"
		}
		fmt.Fprintf(&b, `<figure class="%s">%s<figcaption>%s<span>%s</span></figcaption></figure>`,
			cl, c.SVG, template.HTMLEscapeString(c.Name), template.HTMLEscapeString(c.Value))
	}
	b.WriteString(`</div><p class="note-cartons">Chaque carton a <strong>sa propre échelle</strong> et sa propre ` +
		`projection&nbsp;: les surfaces ne se comparent pas d'un carton à l'autre, ni à l'hexagone.</p>`)
	return b.String()
}

func index(cells []CellMap) map[string]CellMap {
	m := make(map[string]CellMap, len(cells))
	for _, c := range cells {
		if c.Code != "" {
			m[c.Code] = c
		}
	}
	return m
}

func (c Map) fill(cc CellMap) string {
	if cc.Code == "" || cc.Absent {
		return absentFill
	}
	return c.shade(c.class(cc.Value))
}

func labelLevel(n string) string {
	switch n {
	case "REGION":
		return "région"
	case "EPCI":
		return "intercommunalité"
	default:
		return "département"
	}
}

func (j *SetOutlines) envelopper(body string, overview bool) template.HTML {
	cl, role := "geo", `role="img" aria-label="Carte de France par `+labelLevel(j.Level)+`"`
	if overview {
		// Une vignette décorative : la page de détail porte le contenu, la
		// répéter au lecteur d'écran n'ajoute rien et allonge la liste.
		cl, role = "geo apercu", `aria-hidden="true" focusable="false"`
	}
	return template.HTML(`<svg viewBox="` + j.ViewBox + `" class="` + cl + `" ` + role + `>` +
		body + `</svg>`)
}

// outlines renvoie le tracé SVG de chaque entité métropolitaine d'un niveau,
// en Lambert-93. Les outre-mer ont chacun leur projection et se dessineraient
// en cartons : ils ne partagent pas ce repère, et sont écartés ici.
//
// Le tracé est demandé en coordonnées RELATIVES (st_assvg(..., 1, ...)) : les
// écarts entre points voisins tiennent en deux ou trois chiffres là où une
// abscisse Lambert-93 en demande sept. À tolérance égale, un tiers de poids en
// moins, au pixel près identique.
func outlines(ctx context.Context, pool *pgxpool.Pool, level string, tolerance float64) (
	map[string]string, []string, string, map[string]string, map[string]struct{ X, Y float64 }, error) {

	rows, err := pool.Query(ctx, `
		SELECT code_insee, nom, st_assvg(g, 1, 0), st_x((ic).center), -st_y((ic).center)
		FROM (SELECT code_insee, nom, st_transform(st_simplifypreservetopology(geom, $1), 2154) AS g
		      FROM geo.contour WHERE niveau = $2 AND srid_rendu = 2154) x,
		     LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l
		ORDER BY code_insee`, tolerance, level)
	if err != nil {
		return nil, nil, "", nil, nil, err
	}
	defer rows.Close()
	d, noms := map[string]string{}, map[string]string{}
	labels := map[string]struct{ X, Y float64 }{}
	var codes []string
	for rows.Next() {
		var c, n, p string
		var lx, ly float64
		if err := rows.Scan(&c, &n, &p, &lx, &ly); err != nil {
			return nil, nil, "", nil, nil, err
		}
		d[c], noms[c] = p, n
		labels[c] = struct{ X, Y float64 }{lx, ly}
		codes = append(codes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, "", nil, nil, err
	}
	// La boîte englobante est fixe : elle ne dépend pas des données affichées,
	// donc toutes les cartes du site se superposent exactement.
	var vb string
	// La boîte est celle des DÉPARTEMENTS quel que soit le niveau demandé :
	// régions et départements couvrent le même territoire, et une boîte commune
	// fait que les deux cartes se superposent exactement à l'écran.
	//
	// st_extent est une fonction d'agrégation : elle renvoie toujours une
	// ligne, même sur zéro contour trouvé (geo.contour pas encore ingéré) — sa
	// valeur est alors NULL, et la concaténation qui suit hérite de ce NULL.
	// *string ne peut pas recevoir NULL : passer par sql.NullString et
	// renvoyer une boîte vide dans ce cas, plutôt que de faire échouer toute
	// la construction pour une carte qui n'a de toute façon rien à dessiner.
	var vbNull sql.NullString
	err = pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour
		      WHERE niveau='DEPARTEMENT' AND srid_rendu = 2154) x`).Scan(&vbNull)
	if err != nil {
		return nil, nil, "", nil, nil, err
	}
	vb = vbNull.String
	return d, codes, vb, noms, labels, nil
}

// prepare calcule les classes, les bornes et les teintes — la partie commune
// à toutes les formes de rendu. Les classes sont des quintiles sur les seules
// valeurs présentes, jamais plus nombreuses que les valeurs distinctes.
func prepare(cells []CellMap, unit string, format func(float64) string) Map {
	var vals []float64
	var absents int
	for _, c := range cells {
		if c.Absent {
			absents++
		} else {
			vals = append(vals, c.Value)
		}
	}
	if len(vals) == 0 {
		return Map{Empty: true}
	}
	sort.Float64s(vals)

	distinct := 1
	for i := 1; i < len(vals); i++ {
		if vals[i] != vals[i-1] {
			distinct++
		}
	}
	nc := len(ramp)
	if distinct < nc {
		nc = distinct
	}
	thresholds := make([]float64, 0, nc-1)
	for i := 1; i < nc; i++ {
		thresholds = append(thresholds, vals[len(vals)*i/nc])
	}

	c := Map{Unit: unit, Total: len(vals), CountAbsents: absents}
	c.class = func(v float64) int {
		for i, s := range thresholds {
			if v < s {
				return i
			}
		}
		return nc - 1
	}
	// La rampe garde ses extrêmes : avec trois classes on prend le clair, le
	// médian et le foncé, pas les trois premiers pas.
	c.shade = func(k int) string {
		if nc == 1 {
			return ramp[len(ramp)-1]
		}
		return ramp[k*(len(ramp)-1)/(nc-1)]
	}

	// Les bornes affichées sont celles des données, pas des nombres ronds
	// inventés : le lecteur doit pouvoir retrouver la classe d'une valeur.
	start := 0
	for i := 0; i < nc; i++ {
		end := len(vals)
		if i < nc-1 {
			end = len(vals) * (i + 1) / nc
		}
		if end <= start {
			end = start + 1
		}
		if end > len(vals) {
			end = len(vals)
		}
		if start >= len(vals) {
			break
		}
		c.Bounds = append(c.Bounds, format(vals[start])+" – "+format(vals[end-1]))
		c.Shades = append(c.Shades, c.shade(i))
		start = end
	}
	return c
}
