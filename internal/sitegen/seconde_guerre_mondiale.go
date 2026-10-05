package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type StatsSecondeGuerreMondiale struct {
	CarteSVG                                        template.HTML
	LongueurKm                                      float64
	DebarquementSVG                                 template.HTML
	NbAxe, NbOccupe, NbAllie, NbNeutre, NbNonClasse int
}

// pieceEurope : un pays dessiné sur la carte d'Europe (chemin déjà en
// Lambert-93/LAEA, voir dessinerCarteSGM), avec l'ancre de son étiquette —
// le centre du plus grand cercle inscriptible dans sa forme
// (ST_MaximumInscribedCircle), garanti à l'intérieur du polygone, contrairement
// à un centroïde qui peut tomber hors d'une forme concave (la Norvège,
// notamment).
type pieceEurope struct {
	classe, nom, chemin    string
	labelX, labelY, labelR float64
}

// pointDebarquement : une plage de débarquement, 1944 — coordonnées relevées
// sur la commune ou le lieu-dit portant le nom de la plage (vérifiées
// individuellement, pas une seule source pour les huit), pas la précision
// d'un relevé militaire d'époque : suffisant pour un repère sur cette carte,
// pas pour rejouer le débarquement mètre par mètre.
type pointDebarquement struct {
	nom, secteur, operation string
	lat, lon                float64
}

var pointsDebarquement = []pointDebarquement{
	// Opération Overlord, Normandie, 6 juin 1944 — cinq plages, ordre
	// ouest-est. Coordonnées : lieux-dits éponymes, Calvados/Manche.
	{"Utah", "1ʳᵉ armée américaine", "Overlord", 49.42, -1.17},
	{"Omaha", "1ʳᵉ armée américaine", "Overlord", 49.375, -0.89},
	{"Gold", "2ᵉ armée britannique", "Overlord", 49.34, -0.57},
	{"Juno", "1ʳᵉ armée canadienne", "Overlord", 49.335, -0.41},
	{"Sword", "2ᵉ armée britannique", "Overlord", 49.30, -0.32},
	// Opération Dragoon, Provence, 15 août 1944 — trois secteurs, ordre
	// ouest-est. Coordonnées : chef-lieu de chaque secteur (Cavalaire,
	// Sainte-Maxime, Saint-Raphaël), pas la plage précise, plusieurs
	// communes étant concernées par secteur.
	{"Alpha", "7ᵉ armée américaine (3ᵉ division)", "Dragoon", 43.17, 6.53},
	{"Delta", "7ᵉ armée américaine (45ᵉ division)", "Dragoon", 43.31, 6.64},
	{"Camel", "7ᵉ armée américaine (36ᵉ division)", "Dragoon", 43.43, 6.77},
}

// statutBelligerant : classement grossier de quelques pays d'Europe en
// 1940-1942, pour situer la France occupée dans son contexte — pas un
// atlas complet des statuts (qui ont changé plusieurs fois pendant la
// guerre, l'Italie notamment), seulement les cas non disputés. Quatre
// catégories, pas trois : « axe » (Allemagne, Italie à partir de 1940),
// « occupe » (envahi et occupé par l'Allemagne dès 1939-1940, jamais un
// belligérant de l'Axe au sens propre — la nuance manquait à la première
// version de cette carte, qui laissait la Pologne, la Norvège, le
// Danemark, les Pays-Bas, la Belgique et le Luxembourg sans couleur alors
// même qu'ils figuraient déjà dans la liste des pays dessinés), « allie »
// (Royaume-Uni), « neutre ». Tout pays hors de cette liste (Tchéquie,
// Hongrie, les Balkans...) reste délibérément non classé plutôt que
// deviné : présent sur la carte pour combler le repère géographique, sans
// couleur qui suggérerait un statut vérifié. Couleurs reprises de la
// légende du fichier Wikimedia « Map of participants in World War II »
// pour Axe/Alliés/Neutre, la convention la plus citée mais pas la seule
// qui existe — d'autres atlas emploient l'orange pour l'Axe.
var statutBelligerant = map[string]string{
	"Germany": "axe", "Italy": "axe",
	"Poland": "occupe", "Norway": "occupe", "Denmark": "occupe",
	"Netherlands": "occupe", "Belgium": "occupe", "Luxembourg": "occupe",
	"United Kingdom": "allie",
	"Switzerland":    "neutre", "Spain": "neutre", "Portugal": "neutre", "Sweden": "neutre", "Ireland": "neutre",
}

// nomStatut : l'intitulé affiché dans la légende chiffrée, dans le même
// ordre que la légende de couleurs.
var nomStatut = []struct{ classe, libelle string }{
	{"axe", "Axe"},
	{"occupe", "Occupé par l'Axe dès 1939-1940"},
	{"allie", "Allié"},
	{"neutre", "Neutre"},
	{"non-classe", "Hors classement (repère géographique)"},
}

// chargerSecondeGuerreMondiale : la France (occupée/libre, ligne de
// démarcation) resituée dans l'Europe de l'Ouest plutôt que seule sur un
// fond vide — même source que le fond de la carte Francophonie
// (geo.contour_pays), quelques pays voisins classés par statut (voir
// statutBelligerant), les autres en fond neutre plutôt que faux.
//
// Deux projections, jamais les degrés WGS84 bruts : Lambert-93 (2154), la
// convention de ce dépôt pour toute carte de la seule France, pour les
// débarquements ; LAEA Europe (3035) pour la carte resituant la France
// parmi ses voisins — Lambert-93 n'est défini que sur la France, une carte
// d'Europe de l'Ouest a besoin d'une projection valide sur tout son
// cadrage. Avant cette version, les deux cartes utilisaient st_assvg
// directement sur des degrés : à la latitude de la France, un degré de
// longitude vaut environ 0,68 fois un degré de latitude en distance réelle
// (cosinus de 47°) — tout y paraissait environ 47 % trop large d'ouest en
// est, un vrai défaut visuel, pas un choix.
func chargerSecondeGuerreMondiale(ctx context.Context, pool *pgxpool.Pool) (*StatsSecondeGuerreMondiale, error) {
	// France métropolitaine (Corse comprise), isolée des outre-mer par le
	// découpage en polygones distincts de Natural Earth (path 1 = Corse,
	// path 2 = continent ; les autres, Guyane, Réunion..., sont exclus).
	var fond2154, zoneOccupee3035, zoneLibre3035, vb2154 sql.NullString
	var labelFranceX, labelFranceY float64
	if err := pool.QueryRow(ctx, `
		WITH france AS (
			SELECT st_union(geom) g
			FROM (SELECT (ST_Dump(geom)).path AS path, (ST_Dump(geom)).geom AS geom
			      FROM geo.contour_pays WHERE nom_fr='France') d
			WHERE d.path[1] IN (1, 2)
		), proj AS (
			SELECT st_transform(g, 2154) AS g2154, st_transform(g, 3035) AS g3035 FROM france
		),
		-- La France coupée en deux aplats (zone occupée / zone libre), pas un
		-- seul fond uniforme avec la ligne de démarcation en simple repère
		-- dessus (repéré à la vue de la carte publiée : le partage réel de
		-- 1940-1942 ne se voyait pas). geo.ligne_demarcation s'arrête à
		-- 1,5-1,6 km de la frontière réelle des deux côtés (vérifié par
		-- requête) — trop court pour que ST_Split traverse le polygone de
		-- part en part ; prolongée de 0,05° (~5 km) dans le prolongement de
		-- son propre tracé avant découpage.
		ligne AS (SELECT geom FROM geo.ligne_demarcation),
		norm AS (
			SELECT geom,
			       (st_x(st_endpoint(geom))-st_x(st_startpoint(geom)))/st_length(geom) AS ux,
			       (st_y(st_endpoint(geom))-st_y(st_startpoint(geom)))/st_length(geom) AS uy
			FROM ligne
		), ligneEtendue AS (
			SELECT st_addpoint(
			         st_addpoint(geom, st_makepoint(st_x(st_endpoint(geom))+ux*0.05, st_y(st_endpoint(geom))+uy*0.05)),
			         st_makepoint(st_x(st_startpoint(geom))-ux*0.05, st_y(st_startpoint(geom))-uy*0.05),
			         0) AS geom
			FROM norm
		), morceaux AS (
			SELECT (st_dump(st_split(f.g, e.geom))).geom AS g
			FROM france f, ligneEtendue e
		), classes AS (
			SELECT g, row_number() OVER (ORDER BY st_area(g) DESC) AS rn FROM morceaux
		), principaux AS (
			-- Paris (occupé dès 1940) et Vichy (siège du gouvernement de la
			-- zone libre) distinguent sans ambiguïté les deux plus grands
			-- morceaux issus du découpage.
			SELECT rn, g,
			       CASE WHEN st_contains(g, st_setsrid(st_makepoint(2.35, 48.85), 4326)) THEN 'occupee'
			            WHEN st_contains(g, st_setsrid(st_makepoint(3.42, 46.13), 4326)) THEN 'libre' END AS zone
			FROM classes WHERE rn <= 2
		), rattaches AS (
			-- La Corse (jamais traversée par la ligne, sous l'administration
			-- de Vichy jusqu'à l'occupation italienne de novembre 1942 — hors
			-- du repère 1940 de cette carte) et un éclat résiduel du découpage
			-- sont rattachés à la zone principale la plus proche
			-- géométriquement, jamais par un nom codé en dur.
			SELECT o.g, p.zone,
			       row_number() OVER (PARTITION BY o.g ORDER BY st_distance(p.g, o.g)) AS rk
			FROM (SELECT g FROM classes WHERE rn > 2) o CROSS JOIN principaux p
		), zones AS (
			SELECT zone, st_transform(st_union(g), 3035) AS g FROM (
				SELECT g, zone FROM principaux
				UNION ALL
				SELECT g, zone FROM rattaches WHERE rk = 1
			) x GROUP BY zone
		)
		SELECT (SELECT st_assvg(g2154, 1, 0) FROM proj),
		       (SELECT st_assvg(g, 1, 0) FROM zones WHERE zone = 'occupee'),
		       (SELECT st_assvg(g, 1, 0) FROM zones WHERE zone = 'libre'),
		       (SELECT round(st_xmin(g2154))||' '||round(-st_ymax(g2154))||' '||
		               round(st_xmax(g2154)-st_xmin(g2154))||' '||round(st_ymax(g2154)-st_ymin(g2154)) FROM proj),
		       (SELECT st_x((ST_MaximumInscribedCircle(g3035)).center) FROM proj),
		       (SELECT -st_y((ST_MaximumInscribedCircle(g3035)).center) FROM proj)
		`).Scan(&fond2154, &zoneOccupee3035, &zoneLibre3035, &vb2154, &labelFranceX, &labelFranceY); err != nil {
		return nil, err
	}
	if !fond2154.Valid || fond2154.String == "" {
		return nil, nil
	}

	// Une emprise fixe plutôt qu'un tampon spatial autour de la France : la
	// géométrie France de Natural Earth inclut les outre-mer (Guyane,
	// Réunion...), et un st_expand sur leur union fait exploser le tampon à
	// l'échelle du globe — vérifié, 159 pays retournés jusqu'en Afghanistan.
	// Cette emprise reste centrée sur l'Europe de l'Ouest ; elle mord
	// nécessairement, à ses bords, sur l'Europe centrale et les Balkans
	// (aucun rectangle ne peut suivre exactement un contour historique) —
	// ces pays-là sont dessinés, pour ne laisser aucun vide dans le cadrage,
	// mais restent hors de statutBelligerant : non classés, pas classés
	// neutres par erreur. La Finlande (cobelligérante de l'Allemagne contre
	// l'URSS, jamais occupée) est un des cas que ce classement à quatre
	// catégories ne peut pas trancher correctement ; si elle apparaît dans
	// l'emprise, elle reste donc, comme la Tchéquie ou la Hongrie, non
	// classée plutôt que devinée.
	//
	// Plusieurs pays portent aussi des territoires lointains dans Natural
	// Earth (Pays-Bas → Caraïbes, Espagne → Canaries, Portugal → Açores,
	// Norvège → Svalbard) : l'intersection avec cette emprise les retire
	// tous d'un coup. Le filtre d'aire (1000 km² après projection) élimine
	// ensuite les esquilles qu'une frontière tangente à l'emprise laisse
	// passer (Kaliningrad, coin de Lituanie...) — pas les vrais petits pays
	// (le Luxembourg, le plus petit conservé, en fait dix fois plus).
	// clipEuropeWGS84 : un pré-filtre st_intersects bon marché (index
	// spatial), pas la découpe visuelle. La découpe visuelle se fait plus
	// bas avec clipEurope3035, APRÈS projection : un rectangle WGS84 (bords
	// = méridiens/parallèles, des droites) découpé AVANT projection se
	// retrouve avec des bords courbes une fois reprojeté en LAEA (3035),
	// visibles comme des bords inclinés ne suivant pas le cadre rectangulaire
	// de la carte (repéré à la vue de la carte publiée). Bornes : jusqu'à
	// 71,5°N et 32,5°E pour inclure la Finlande et la Norvège en entier
	// (toutes deux tronquées par l'ancienne borne à 59°N) — la Finlande
	// reste malgré tout non classée (voir plus haut), la carte s'agrandit
	// seulement pour ne plus l'effacer du cadrage.
	const clipEuropeWGS84 = `st_makeenvelope(-10, 34, 32.5, 71.5, 4326)`
	const clipEurope3035 = `st_transform(st_makeenvelope(-10, 34, 32.5, 71.5, 4326), 3035)`
	const seuilAireM2 = 1e9
	const tolEuropeM = 8000.0 // mètres (EPSG:3035), après transformation

	rows, err := pool.Query(ctx, `
		WITH pays AS (
			SELECT nom_fr, nom_en,
			       st_simplifypreservetopology(
			           st_intersection(st_transform(geom, 3035), `+clipEurope3035+`), $1) AS g
			FROM geo.contour_pays
			WHERE nom_fr <> 'France' AND st_intersects(geom, `+clipEuropeWGS84+`)
		)
		SELECT nom_fr, nom_en, st_assvg(g, 1, 0),
		       st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM pays, LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l
		WHERE NOT st_isempty(g) AND st_area(g) > $2
		ORDER BY st_area(g) DESC`, tolEuropeM, seuilAireM2)
	if err != nil {
		return nil, err
	}
	var voisins []pieceEurope
	compte := map[string]int{}
	for rows.Next() {
		var p pieceEurope
		var nomEn string
		if err := rows.Scan(&p.nom, &nomEn, &p.chemin, &p.labelX, &p.labelY, &p.labelR); err != nil {
			rows.Close()
			return nil, err
		}
		p.classe = statutBelligerant[nomEn]
		if p.classe == "" {
			p.classe = "non-classe"
		}
		compte[p.classe]++
		voisins = append(voisins, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	var vb3035 string
	if err := pool.QueryRow(ctx, `
		WITH pays AS (
			SELECT st_intersection(st_transform(geom, 3035), `+clipEurope3035+`) AS geom
			FROM geo.contour_pays WHERE nom_fr <> 'France' AND st_intersects(geom, `+clipEuropeWGS84+`)
			UNION ALL
			SELECT st_transform(geom, 3035) FROM (SELECT (ST_Dump(geom)).path AS path, (ST_Dump(geom)).geom AS geom
			                   FROM geo.contour_pays WHERE nom_fr='France') d
			WHERE d.path[1] IN (1, 2)
		), u AS (SELECT st_union(geom) g FROM pays)
		SELECT round(st_xmin(g))||' '||round(-st_ymax(g))||' '||
		       round(st_xmax(g)-st_xmin(g))||' '||round(st_ymax(g)-st_ymin(g))
		FROM u`).Scan(&vb3035); err != nil {
		return nil, err
	}

	// geo.cours_eau ne couvre que la France (voir carte.go) — un calque
	// Europe entière (Rhin, Danube...) existe par ailleurs (geo.cours_eau_monde)
	// mais appartient à un chantier séparé, pas encore disponible ici : la
	// carte d'Europe reste donc sans cours d'eau hors de France pour cette
	// version, plutôt que de dépendre d'une table absente.
	fleuvesFrance, err := fleuvesSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	var fleuvesEurope string

	// Les huit points de débarquement sont fixés en Go (coordonnées
	// vérifiées individuellement, voir pointsDebarquement) : une seule
	// requête les projette tous en Lambert-93, dans le même ordre, plutôt
	// qu'un calcul de projection refait à la main.
	lons := make([]float64, len(pointsDebarquement))
	lats := make([]float64, len(pointsDebarquement))
	for i, p := range pointsDebarquement {
		lons[i], lats[i] = p.lon, p.lat
	}
	ptRows, err := pool.Query(ctx, `
		SELECT st_x(g), st_y(g)
		FROM unnest($1::float8[], $2::float8[]) WITH ORDINALITY AS v(lon, lat, ord)
		CROSS JOIN LATERAL (SELECT st_transform(st_setsrid(st_makepoint(v.lon, v.lat), 4326), 2154) g) t
		ORDER BY v.ord`, lons, lats)
	if err != nil {
		return nil, err
	}
	pointsProjetes := make([]pointProjete, 0, len(pointsDebarquement))
	i := 0
	for ptRows.Next() {
		var x, y float64
		if err := ptRows.Scan(&x, &y); err != nil {
			ptRows.Close()
			return nil, err
		}
		p := pointsDebarquement[i]
		pointsProjetes = append(pointsProjetes, pointProjete{X: x, Y: -y, Nom: p.nom, Secteur: p.secteur, Operation: p.operation})
		i++
	}
	if err := ptRows.Err(); err != nil {
		ptRows.Close()
		return nil, err
	}
	ptRows.Close()

	var ligne2154, ligne3035 sql.NullString
	var longueurM float64
	err = pool.QueryRow(ctx, `
		SELECT st_assvg(st_transform(geom, 2154), 1, 0), st_assvg(st_transform(geom, 3035), 1, 0), longueur_m
		FROM geo.ligne_demarcation LIMIT 1`).
		Scan(&ligne2154, &ligne3035, &longueurM)

	// La ligne de démarcation avait disparu, dans les faits, depuis
	// l'invasion de la zone libre en novembre 1942 (voir le dossier, § 2) —
	// bien avant les deux débarquements de 1944. La montrer dessus comme un
	// partage occupée/libre encore actif serait faux ; dessinerDebarquements
	// la trace donc en simple repère estompé, jamais en aplat de zone.
	debarquementSVG := dessinerDebarquements(fond2154.String, fleuvesFrance, vb2154.String, ligne2154.String, pointsProjetes)

	franceEtiquette := pieceEurope{nom: "France", labelX: labelFranceX, labelY: labelFranceY}
	st := &StatsSecondeGuerreMondiale{
		DebarquementSVG: debarquementSVG,
		NbAxe:           compte["axe"], NbOccupe: compte["occupe"], NbAllie: compte["allie"],
		NbNeutre: compte["neutre"], NbNonClasse: compte["non-classe"],
	}
	if err != nil {
		st.CarteSVG = dessinerCarteSGM(voisins, zoneOccupee3035.String, zoneLibre3035.String, fleuvesEurope, vb3035, "", franceEtiquette)
		return st, nil
	}

	st.LongueurKm = longueurM / 1000
	st.CarteSVG = dessinerCarteSGM(voisins, zoneOccupee3035.String, zoneLibre3035.String, fleuvesEurope, vb3035, ligne3035.String, franceEtiquette)
	return st, nil
}

// pointProjete : une plage de débarquement projetée en Lambert-93 (X, Y déjà
// inversé pour l'affichage SVG — même convention que le reste du dépôt,
// cy = -y_lambert), prête à dessiner.
type pointProjete struct {
	X, Y                    float64
	Nom, Secteur, Operation string
}

// dessinerDebarquements : la France seule (même fond que dessinerCarteSGM,
// sans les voisins — Normandie et Provence sont à l'intérieur du pays, pas à
// sa frontière), un point par plage de débarquement (pointsDebarquement,
// ci-dessus), coloré par opération. Un halo et une étiquette par opération,
// en plus des points : à l'échelle de la France entière, cinq plages sur
// 30 km de côte (Overlord) ou trois secteurs sur 25 km (Dragoon) restent de
// petits points quasi confondus — le halo dit où regarder avant le détail.
// Pas de tracé de front ni de zone contrôlée : seulement le lieu et la date
// des deux débarquements alliés en France métropolitaine en 1944. La ligne
// de démarcation, quand elle est fournie, n'est qu'un repère estompé — elle
// avait disparu dans les faits depuis novembre 1942 (voir le dossier, § 2),
// bien avant ces deux dates, jamais un partage occupée/libre encore actif.
func dessinerDebarquements(fond, fleuves, viewBox, ligne string, pts []pointProjete) template.HTML {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo france sgm debarquements" role="img" `+
		`aria-label="Les débarquements alliés en France, 1944 : Normandie (6 juin) et Provence (15 août)">`, viewBox)
	fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, fond)
	b.WriteString(fleuves)
	if ligne != "" {
		fmt.Fprintf(&b, `<path class="ligne-demarcation ancienne" d="%s">`+
			`<title>Ligne de démarcation, juin 1940 - mars 1943 — disparue dans les faits dès novembre 1942, `+
			`bien avant les débarquements : simple repère géographique, pas un partage encore actif</title></path>`, ligne)
	}

	for _, op := range []string{"Overlord", "Dragoon"} {
		var xs, ys []float64
		for _, p := range pts {
			if p.Operation == op {
				xs = append(xs, p.X)
				ys = append(ys, p.Y)
			}
		}
		if len(xs) == 0 {
			continue
		}
		var sommeX, sommeY float64
		for i := range xs {
			sommeX += xs[i]
			sommeY += ys[i]
		}
		cx, cy := sommeX/float64(len(xs)), sommeY/float64(len(ys))
		rayon := 0.0
		for i := range xs {
			if d := math.Hypot(xs[i]-cx, ys[i]-cy); d > rayon {
				rayon = d
			}
		}
		rayon += 32000
		classe, nom, decalage := "overlord", "Normandie", -58000.0
		if op == "Dragoon" {
			classe, nom, decalage = "dragoon", "Provence", 58000.0
		}
		fmt.Fprintf(&b, `<circle class="halo %s" cx="%.0f" cy="%.0f" r="%.0f"/>`, classe, cx, cy, rayon)
		fmt.Fprintf(&b, `<text class="repere %s" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`,
			classe, cx, cy+decalage, nom)
	}

	for _, p := range pts {
		classe, date := "overlord", "6 juin 1944"
		if p.Operation == "Dragoon" {
			classe, date = "dragoon", "15 août 1944"
		}
		fmt.Fprintf(&b, `<circle class="debarquement %s" cx="%.0f" cy="%.0f" r="14000">`+
			`<title>%s (%s) — %s, %s</title></circle>`,
			classe, p.X, p.Y, p.Nom, p.Operation, p.Secteur, date)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// seuilEtiquettePays : rayon minimal (mètres, projection 3035) du plus grand
// cercle inscriptible dans un pays pour lui donner une étiquette — en
// dessous, le nom ne tiendrait pas lisiblement (un dixième du Luxembourg,
// le plus petit pays conservé par le filtre d'aire de chargerSecondeGuerreMondiale).
const seuilEtiquettePays = 8000.0

// dessinerCarteSGM : la France dans son contexte européen — chaque pays de
// l'emprise (voir chargerSecondeGuerreMondiale) rempli par statut quand il
// est connu, en gris neutre sinon, jamais un vide ; les grands cours d'eau
// (Natural Earth) et le nom de chaque pays en France comme repères ; le
// tracé de la ligne de démarcation. La France elle-même est maintenant
// coupée en deux aplats, zone occupée et zone libre (zoneOccupee/zoneLibre,
// calculés par chargerSecondeGuerreMondiale à partir de geo.ligne_demarcation
// — une vraie géométrie de zone dérivée d'une donnée vérifiée, pas devinée),
// là où une version antérieure ne traçait qu'un fond uniforme sous la ligne
// (le partage réel de 1940-1942 ne se voyait pas — repéré à la vue de la
// carte publiée).
func dessinerCarteSGM(voisins []pieceEurope, zoneOccupee, zoneLibre, fleuves, viewBox, ligne string, france pieceEurope) template.HTML {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo france sgm" role="img" `+
		`aria-label="La France et ses voisins d'Europe, coupée en zone occupée et zone libre par la ligne de démarcation, 1940-1942">`, viewBox)
	// Un rectangle plein aux dimensions exactes du viewBox : la Manche, la mer
	// du Nord et la Baltique restaient blanches, indiscernables du fond de
	// page — pas une terre neutre non plus, une vraie mer.
	var largeurViewBox float64
	if parts := strings.Fields(viewBox); len(parts) == 4 {
		fmt.Fprintf(&b, `<rect class="ocean" x="%s" y="%s" width="%s" height="%s"/>`,
			parts[0], parts[1], parts[2], parts[3])
		largeurViewBox, _ = strconv.ParseFloat(parts[2], 64)
	}
	b.WriteString(fleuves)
	for _, p := range voisins {
		fmt.Fprintf(&b, `<path class="pays-p %s" d="%s"><title>%s</title></path>`,
			p.classe, p.chemin, template.HTMLEscapeString(p.nom))
	}
	if zoneOccupee != "" {
		fmt.Fprintf(&b, `<path class="fond occupee" d="%s"><title>Zone occupée</title></path>`, zoneOccupee)
	}
	if zoneLibre != "" {
		fmt.Fprintf(&b, `<path class="fond libre" d="%s"><title>Zone libre</title></path>`, zoneLibre)
	}
	if ligne != "" {
		fmt.Fprintf(&b, `<path class="ligne-demarcation" d="%s"><title>Ligne de démarcation, 1940-1942</title></path>`, ligne)
	}

	// Un <text font-size="..."> direct, réglé à l'échelle du viewBox (des
	// centaines de milliers d'unités), rendait un simple trait au lieu de
	// lettres lisibles : au-delà d'une certaine taille, Chromium semble
	// buter sur le rendu des glyphes (reproduit en isolation, y compris en
	// dehors de ce dépôt — pas un bug de ce fichier). Contournement : le
	// texte garde une taille de police normale (quelques pixels), et c'est
	// un <g transform="... scale(k)"> qui l'agrandit à l'échelle de la
	// carte — une transformation matricielle, jamais sujette au même
	// problème de rendu de police.
	echelle := largeurViewBox / 700 // ≈ la largeur réelle de la carte à l'écran, en pixels
	if echelle <= 0 {
		echelle = 1
	}
	etiquette := func(p pieceEurope, classe string) {
		fmt.Fprintf(&b, `<g transform="translate(%.0f,%.0f) scale(%.2f)">`+
			`<text class="nom-pays %s" x="0" y="0" text-anchor="middle">%s</text></g>`,
			p.labelX, p.labelY, echelle, classe, template.HTMLEscapeString(p.nom))
	}
	// Nommer aussi les pays hors classement submergeait l'Europe centrale et
	// les Balkans d'étiquettes tassées les unes sur les autres (jusqu'à
	// sortir du cadre pour la Macédoine du Nord) — repéré directement à la
	// vue de la carte publiée. Seuls les pays que cette carte classe
	// vraiment (voir statutBelligerant) sont donc nommés ; les autres
	// restent en couleur, sans étiquette, cohérent avec « hors classement »
	// qui dit déjà que ce dossier n'a rien de vérifié à affirmer sur eux.
	for _, p := range voisins {
		if p.labelR < seuilEtiquettePays || p.classe == "non-classe" {
			continue
		}
		etiquette(p, "")
	}
	etiquette(france, "nom-france")
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type pointPopulation struct {
	Annee      int
	Population int64
}

type PopulationGuerres struct {
	SVG              template.HTML
	Pop1911, Pop1921 int64
	BaisseAbsolue    int64
	BaissePct        float64
	Pop1936, Pop1954 int64
}

// chargerPopulationGuerres : la population communale agrégée au niveau
// national (core.population_historique_commune, Insee 1876-1999) — la
// seule série de ce dossier qui montre un choc démographique mesuré
// indépendamment de tout dénombrement militaire ou civil. Le creux de la
// Première Guerre mondiale (1911→1921) est directement lisible ; celui de
// la Seconde ne l'est pas, la source sautant de 1936 à 1954 sans point en
// 1946.
// chargerPopulationGuerres lit mv.population_nationale_annee (internal/
// matview) — plus le GROUP BY sur la totalité de core.
// population_historique_commune (657k lignes) que cette fonction refaisait
// à chaque construction.
func chargerPopulationGuerres(ctx context.Context, pool *pgxpool.Pool) (*PopulationGuerres, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, population FROM mv.population_nationale_annee
		ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []pointPopulation
	valeurs := map[int]int64{}
	for rows.Next() {
		var p pointPopulation
		if err := rows.Scan(&p.Annee, &p.Population); err != nil {
			return nil, err
		}
		pts = append(pts, p)
		valeurs[p.Annee] = p.Population
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pts) == 0 {
		return nil, nil
	}
	pg := &PopulationGuerres{
		Pop1911: valeurs[1911], Pop1921: valeurs[1921],
		Pop1936: valeurs[1936], Pop1954: valeurs[1954],
	}
	if pg.Pop1911 > 0 {
		pg.BaisseAbsolue = pg.Pop1911 - pg.Pop1921
		pg.BaissePct = float64(pg.BaisseAbsolue) / float64(pg.Pop1911) * 100
	}
	pg.SVG = dessinerPopulationGuerres(pts)
	return pg, nil
}

func dessinerPopulationGuerres(pts []pointPopulation) template.HTML {
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 40.0, 14.0, 14.0, 26.0
	anneeDebut, anneeFin := pts[0].Annee, pts[len(pts)-1].Annee
	maxVal := int64(0)
	for _, p := range pts {
		if p.Population > maxVal {
			maxVal = p.Population
		}
	}
	maxValM := float64(maxVal) / 1e6 * 1.1
	x := func(annee int) float64 { return ml + (w-ml-mr)*float64(annee-anneeDebut)/float64(anneeFin-anneeDebut) }
	y := func(popM float64) float64 { return mt + (h-mt-mb)*(1-popM/maxValM) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe population-guerres" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Population de la France, %d à %d">`, w, h, anneeDebut, anneeFin)

	// Deux bandes : 1914-1918 et 1939-1945 — pas des zones de rupture de
	// série (comme dans le graphique immigration), mais les deux guerres
	// elles-mêmes, pour lire le creux de 1921 et l'absence de creux visible
	// autour de 1954 dans leur contexte.
	for _, guerre := range [][2]int{{1914, 1918}, {1939, 1945}} {
		fmt.Fprintf(&b, `<rect class="bande-guerre" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`,
			x(guerre[0]), mt, x(guerre[1])-x(guerre[0]), h-mt-mb)
	}
	for _, palier := range []float64{0, 20, 40, 60} {
		if palier > maxValM {
			continue
		}
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(palier), w-mr, y(palier))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%d M</text>`, ml-6, y(palier)+3, int(palier))
	}

	var coords []string
	for _, p := range pts {
		coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(p.Annee), y(float64(p.Population)/1e6)))
	}
	fmt.Fprintf(&b, `<polyline class="ligne-pop" points="%s"/>`, strings.Join(coords, " "))
	for _, p := range pts {
		fmt.Fprintf(&b, `<circle class="pt-pop" cx="%.2f" cy="%.2f" r="2.6"><title>%d : %s habitants</title></circle>`,
			x(p.Annee), y(float64(p.Population)/1e6), p.Annee, Nombre(int(p.Population)))
	}
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, anneeDebut)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, anneeFin)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
