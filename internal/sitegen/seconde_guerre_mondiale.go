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

type StatsSecondWorldWar struct {
	MapSVG                                                           template.HTML
	LengthKm                                                         float64
	LandingSVG                                                       template.HTML
	CountAxis, CountOccupied, CountAlly, CountNeutral, CountNonClass int
}

// roomEurope : un pays dessiné sur la carte d'Europe (chemin déjà en
// Lambert-93/LAEA, voir dessinerCarteSGM), avec l'ancre de son étiquette —
// le centre du plus grand cercle inscriptible dans sa forme
// (ST_MaximumInscribedCircle), garanti à l'intérieur du polygone, contrairement
// à un centroïde qui peut tomber hors d'une forme concave (la Norvège,
// notamment).
type roomEurope struct {
	class, name, path      string
	labelX, labelY, labelR float64
}

// pointLanding : une plage de débarquement, 1944 — coordonnées relevées
// sur la commune ou le lieu-dit portant le nom de la plage (vérifiées
// individuellement, pas une seule source pour les huit), pas la précision
// d'un relevé militaire d'époque : suffisant pour un repère sur cette carte,
// pas pour rejouer le débarquement mètre par mètre.
type pointLanding struct {
	name, sector, operation string
	lat, lon                float64
}

var pointsLanding = []pointLanding{
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
// Clés en anglais telles que nommées par CShapes 2.0 (geo.contour_europe_1940,
// internal/geo/europe_1940.go) depuis la version 9 de ce dossier — pas les
// noms Natural Earth d'avant (« Germany », « Italy ») : cette carte dessine
// désormais les frontières de 1940, pas celles d'aujourd'hui.
var statusBelligerent = map[string]string{
	"Germany (Prussia)": "axe", "Italy/Sardinia": "axe",
	"Poland": "occupe", "Norway": "occupe", "Denmark": "occupe",
	"Netherlands": "occupe", "Belgium": "occupe", "Luxembourg": "occupe",
	"United Kingdom": "allie",
	"Switzerland":    "neutre", "Spain": "neutre", "Portugal": "neutre", "Sweden": "neutre", "Ireland": "neutre",
}

// nomStatut : l'intitulé affiché dans la légende chiffrée, dans le même
// ordre que la légende de couleurs.
var nameStatus = []struct{ class, label string }{
	{"axe", "Axe"},
	{"occupe", "Occupé par l'Axe dès 1939-1940"},
	{"allie", "Allié"},
	{"neutre", "Neutre"},
	{"non-classe", "Hors classement (repère géographique)"},
}

// loadSecondWorldWar : la France (occupée/libre, ligne de
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
func loadSecondWorldWar(ctx context.Context, pool *pgxpool.Pool) (*StatsSecondWorldWar, error) {
	// France métropolitaine (Corse comprise), isolée des outre-mer par le
	// découpage en polygones distincts de Natural Earth (path 1 = Corse,
	// path 2 = continent ; les autres, Guyane, Réunion..., sont exclus).
	//
	// geo.contour_pays vient d'une source hors chaîne par défaut
	// (« contour-pays », internal/ingest/catalogue.go) : absente d'un simple
	// « fpctl ingest default », la table est vide et le st_union de la CTE france
	// ci-dessous — une agrégation, donc toujours une ligne — vaut NULL. Les
	// quatre premières colonnes passent déjà par sql.NullString pour cette
	// raison ; les deux dernières (le centre du plus grand cercle inscrit)
	// héritent du même NULL et doivent donc l'être aussi : un float64 nu
	// échouerait au Scan (« cannot scan NULL into *float64 ») AVANT que le
	// garde-fou !fond2154.Valid juste en dessous ait pu renvoyer « rien à
	// dessiner », et cette erreur ferait tomber toute la construction du site.
	var background2154, zoneOccupied3035, zoneFree3035, vb2154 sql.NullString
	var labelFranceXN, labelFranceYN sql.NullFloat64
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
		`).Scan(&background2154, &zoneOccupied3035, &zoneFree3035, &vb2154, &labelFranceXN, &labelFranceYN); err != nil {
		return nil, err
	}
	if !background2154.Valid || background2154.String == "" {
		return nil, nil
	}
	labelFranceX, labelFranceY := labelFranceXN.Float64, labelFranceYN.Float64

	// Les voisins viennent de geo.contour_europe_1940 (CShapes 2.0, coupe au
	// 1ᵉʳ septembre 1940 — voir internal/geo/europe_1940.go), pas des
	// frontières actuelles (geo.contour_pays) : une carte au sujet de 1940
	// dessinée avec les frontières d'aujourd'hui plaçait la Pologne à sa
	// taille actuelle (amputée à l'est par rapport à 1940) et faisait
	// apparaître l'Ukraine, la Biélorussie et les pays baltes comme des
	// États indépendants — aucun ne l'était à cette date, absorbés ou
	// partagés entre la Pologne d'avant-guerre et l'URSS (signalé
	// directement par l'utilisateur). geo.contour_europe_1940 ne contient
	// qu'une seule URSS, à ses frontières réelles de septembre 1940 : ni
	// minimisée ni coupée du cadrage.
	//
	// Une emprise fixe plutôt qu'un tampon spatial : l'URSS de 1940
	// s'étend jusqu'au Pacifique, un st_expand dessus fait exploser le
	// cadrage à l'échelle du globe. clipEuropeWGS84 sert de pré-filtre
	// st_intersects bon marché (index spatial) ; la découpe visuelle se
	// fait avec clipEurope3035, APRÈS projection. Bornes : jusqu'à 71,5°N
	// (Norvège et Finlande de 1940 entières) et jusqu'à 40°E (Moscou, pour
	// donner une échelle à l'URSS plutôt qu'une bande arbitrairement
	// étroite).
	//
	// st_makeenvelope ne pose que 4 sommets, aux coins du rectangle. Le
	// transformer directement (st_transform tel quel) ne fait que déplacer
	// CES 4 POINTS vers leurs coordonnées LAEA et relie les nouveaux points
	// par des droites — pas la vraie courbe que suit un méridien ou un
	// parallèle une fois reprojeté. Le bord ouest (Atlantique, longue côte
	// réelle à suivre) et le bord sud passaient inaperçus ainsi, mais le
	// bord est (40°E) tombe en pleine URSS, un immense pays d'un seul tenant
	// : la corde droite entre ses deux coins tranche le pays par une balafre
	// diagonale bien visible, au lieu de suivre le vrai méridien (repéré
	// directement sur la carte publiée). st_segmentize ajoute des sommets
	// tous les 0,5° AVANT la reprojection : la ligne transformée suit alors
	// de près la vraie courbe du méridien/parallèle, au lieu de sauter en
	// droite d'un coin à l'autre.
	const clipEuropeWGS84 = `st_makeenvelope(-10, 37.5, 40, 71.5, 4326)`
	const clipEurope3035 = `st_transform(st_segmentize(st_makeenvelope(-10, 37.5, 40, 71.5, 4326), 0.5), 3035)`
	const thresholdAreaM2 = 1e9
	const toleranceEuropeM = 8000.0 // mètres (EPSG:3035), après transformation

	rows, err := pool.Query(ctx, `
		WITH pays AS (
			SELECT nom, nom_en,
			       st_simplifypreservetopology(
			           st_intersection(st_transform(geom, 3035), `+clipEurope3035+`), $1) AS g
			FROM geo.contour_europe_1940
			WHERE st_intersects(geom, `+clipEuropeWGS84+`)
		)
		SELECT nom, nom_en, st_assvg(g, 1, 0),
		       st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM pays, LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l
		WHERE NOT st_isempty(g) AND st_area(g) > $2
		ORDER BY st_area(g) DESC`, toleranceEuropeM, thresholdAreaM2)
	if err != nil {
		return nil, err
	}
	var neighbors []roomEurope
	account := map[string]int{}
	for rows.Next() {
		var p roomEurope
		var nameIn string
		if err := rows.Scan(&p.name, &nameIn, &p.path, &p.labelX, &p.labelY, &p.labelR); err != nil {
			rows.Close()
			return nil, err
		}
		p.class = statusBelligerent[nameIn]
		if p.class == "" {
			p.class = "non-classe"
		}
		account[p.class]++
		neighbors = append(neighbors, p)
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
			FROM geo.contour_europe_1940 WHERE st_intersects(geom, `+clipEuropeWGS84+`)
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

	// geo.cours_eau ne couvre que la France (voir carte.go) : sur cette
	// carte d'Europe, geo.cours_eau_monde (Natural Earth, même échelle et
	// même emprise que le fond de pays ci-dessus) prend le relais — sans
	// lui, le Rhin ou le Danube s'arrêtaient net à la frontière française,
	// comme s'ils n'existaient qu'en France.
	riversFrance, err := riversSVG(ctx, pool, 2154, 1, 0)
	if err != nil {
		return nil, err
	}
	var riversEurope string
	fRows, err := pool.Query(ctx, `
		SELECT st_assvg(st_intersection(st_transform(geom, 3035), `+clipEurope3035+`), 1, 0)
		FROM geo.cours_eau_monde WHERE st_intersects(geom, `+clipEuropeWGS84+`)`)
	if err != nil {
		return nil, err
	}
	var feB strings.Builder
	for fRows.Next() {
		var d string
		if err := fRows.Scan(&d); err != nil {
			fRows.Close()
			return nil, err
		}
		if d == "" {
			continue
		}
		fmt.Fprintf(&feB, `<path class="fleuve" d="%s"/>`, d)
	}
	if err := fRows.Err(); err != nil {
		fRows.Close()
		return nil, err
	}
	fRows.Close()
	riversEurope = feB.String()

	// Les huit points de débarquement sont fixés en Go (coordonnées
	// vérifiées individuellement, voir pointsDebarquement) : une seule
	// requête les projette tous en Lambert-93, dans le même ordre, plutôt
	// qu'un calcul de projection refait à la main.
	lons := make([]float64, len(pointsLanding))
	lats := make([]float64, len(pointsLanding))
	for i, p := range pointsLanding {
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
	pointsProjected := make([]pointProjected, 0, len(pointsLanding))
	i := 0
	for ptRows.Next() {
		var x, y float64
		if err := ptRows.Scan(&x, &y); err != nil {
			ptRows.Close()
			return nil, err
		}
		p := pointsLanding[i]
		pointsProjected = append(pointsProjected, pointProjected{X: x, Y: -y, Name: p.name, Sector: p.sector, Operation: p.operation})
		i++
	}
	if err := ptRows.Err(); err != nil {
		ptRows.Close()
		return nil, err
	}
	ptRows.Close()

	var line2154, line3035 sql.NullString
	var lengthM float64
	err = pool.QueryRow(ctx, `
		SELECT st_assvg(st_transform(geom, 2154), 1, 0), st_assvg(st_transform(geom, 3035), 1, 0), longueur_m
		FROM geo.ligne_demarcation LIMIT 1`).
		Scan(&line2154, &line3035, &lengthM)

	// La ligne de démarcation avait disparu, dans les faits, depuis
	// l'invasion de la zone libre en novembre 1942 (voir le dossier, § 2) —
	// bien avant les deux débarquements de 1944. La montrer dessus comme un
	// partage occupée/libre encore actif serait faux ; dessinerDebarquements
	// la trace donc en simple repère estompé, jamais en aplat de zone.
	landingSVG := drawLandings(background2154.String, riversFrance, vb2154.String, line2154.String, pointsProjected)

	franceLabel := roomEurope{name: "France", labelX: labelFranceX, labelY: labelFranceY}
	st := &StatsSecondWorldWar{
		LandingSVG: landingSVG,
		CountAxis:  account["axe"], CountOccupied: account["occupe"], CountAlly: account["allie"],
		CountNeutral: account["neutre"], CountNonClass: account["non-classe"],
	}
	if err != nil {
		st.MapSVG = drawMapSGM(neighbors, zoneOccupied3035.String, zoneFree3035.String, riversEurope, vb3035, "", franceLabel)
		return st, nil
	}

	st.LengthKm = lengthM / 1000
	st.MapSVG = drawMapSGM(neighbors, zoneOccupied3035.String, zoneFree3035.String, riversEurope, vb3035, line3035.String, franceLabel)
	return st, nil
}

// pointProjected : une plage de débarquement projetée en Lambert-93 (X, Y déjà
// inversé pour l'affichage SVG — même convention que le reste du dépôt,
// cy = -y_lambert), prête à dessiner.
type pointProjected struct {
	X, Y                    float64
	Name, Sector, Operation string
}

// drawLandings : la France seule (même fond que dessinerCarteSGM,
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
func drawLandings(background, rivers, viewBox, line string, pts []pointProjected) template.HTML {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo france sgm debarquements" role="img" `+
		`aria-label="Les débarquements alliés en France, 1944 : Normandie (6 juin) et Provence (15 août)">`, viewBox)
	fmt.Fprintf(&b, `<path class="fond" d="%s"/>`, background)
	b.WriteString(rivers)
	if line != "" {
		fmt.Fprintf(&b, `<path class="ligne-demarcation ancienne" d="%s">`+
			`<title>Ligne de démarcation, juin 1940 - mars 1943 — disparue dans les faits dès novembre 1942, `+
			`bien avant les débarquements : simple repère géographique, pas un partage encore actif</title></path>`, line)
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
		var sumX, sumY float64
		for i := range xs {
			sumX += xs[i]
			sumY += ys[i]
		}
		cx, cy := sumX/float64(len(xs)), sumY/float64(len(ys))
		radius := 0.0
		for i := range xs {
			if d := math.Hypot(xs[i]-cx, ys[i]-cy); d > radius {
				radius = d
			}
		}
		radius += 32000
		class, name, offset := "overlord", "Normandie", -58000.0
		if op == "Dragoon" {
			class, name, offset = "dragoon", "Provence", 58000.0
		}
		fmt.Fprintf(&b, `<circle class="halo %s" cx="%.0f" cy="%.0f" r="%.0f"/>`, class, cx, cy, radius)
		fmt.Fprintf(&b, `<text class="repere %s" x="%.0f" y="%.0f" text-anchor="middle">%s</text>`,
			class, cx, cy+offset, name)
	}

	for _, p := range pts {
		class, date := "overlord", "6 juin 1944"
		if p.Operation == "Dragoon" {
			class, date = "dragoon", "15 août 1944"
		}
		fmt.Fprintf(&b, `<circle class="debarquement %s" cx="%.0f" cy="%.0f" r="14000">`+
			`<title>%s (%s) — %s, %s</title></circle>`,
			class, p.X, p.Y, p.Name, p.Operation, p.Sector, date)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// seuilEtiquettePays : rayon minimal (mètres, projection 3035) du plus grand
// cercle inscriptible dans un pays pour lui donner une étiquette — en
// dessous, le nom ne tiendrait pas lisiblement (un dixième du Luxembourg,
// le plus petit pays conservé par le filtre d'aire de chargerSecondeGuerreMondiale).
const thresholdLabelCountry = 8000.0

// drawMapSGM : la France dans son contexte européen — chaque pays de
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
func drawMapSGM(neighbors []roomEurope, zoneOccupied, zoneFree, rivers, viewBox, line string, france roomEurope) template.HTML {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="%s" class="geo france sgm" role="img" `+
		`aria-label="La France et ses voisins d'Europe, coupée en zone occupée et zone libre par la ligne de démarcation, 1940-1942">`, viewBox)
	// Un rectangle plein aux dimensions exactes du viewBox : la Manche, la mer
	// du Nord et la Baltique restaient blanches, indiscernables du fond de
	// page — pas une terre neutre non plus, une vraie mer.
	var widthViewBox float64
	if shares := strings.Fields(viewBox); len(shares) == 4 {
		fmt.Fprintf(&b, `<rect class="ocean" x="%s" y="%s" width="%s" height="%s"/>`,
			shares[0], shares[1], shares[2], shares[3])
		widthViewBox, _ = strconv.ParseFloat(shares[2], 64)
	}
	b.WriteString(rivers)
	for _, p := range neighbors {
		fmt.Fprintf(&b, `<path class="pays-p %s" d="%s"><title>%s</title></path>`,
			p.class, p.path, template.HTMLEscapeString(p.name))
	}
	if zoneOccupied != "" {
		fmt.Fprintf(&b, `<path class="fond occupee" d="%s"><title>Zone occupée</title></path>`, zoneOccupied)
	}
	if zoneFree != "" {
		fmt.Fprintf(&b, `<path class="fond libre" d="%s"><title>Zone libre</title></path>`, zoneFree)
	}
	if line != "" {
		fmt.Fprintf(&b, `<path class="ligne-demarcation" d="%s"><title>Ligne de démarcation, 1940-1942</title></path>`, line)
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
	scale := widthViewBox / 700 // ≈ la largeur réelle de la carte à l'écran, en pixels
	if scale <= 0 {
		scale = 1
	}
	label := func(p roomEurope, class string) {
		fmt.Fprintf(&b, `<g transform="translate(%.0f,%.0f) scale(%.2f)">`+
			`<text class="nom-pays %s" x="0" y="0" text-anchor="middle">%s</text></g>`,
			p.labelX, p.labelY, scale, class, template.HTMLEscapeString(p.name))
	}
	// Nommer aussi les pays hors classement submergeait l'Europe centrale et
	// les Balkans d'étiquettes tassées les unes sur les autres (jusqu'à
	// sortir du cadre pour la Macédoine du Nord) — repéré directement à la
	// vue de la carte publiée. Seuls les pays que cette carte classe
	// vraiment (voir statutBelligerant) sont donc nommés ; les autres
	// restent en couleur, sans étiquette, cohérent avec « hors classement »
	// qui dit déjà que ce dossier n'a rien de vérifié à affirmer sur eux.
	// Deux exceptions : l'URSS et la Finlande, deux entités uniques et sans
	// ambiguïté (pas un groupe de petits pays tassés) — les laisser sans nom
	// ferait deviner au lecteur ce que représentent ces deux grandes zones
	// à l'est, contraire à la demande explicite de ne pas les effacer de la
	// carte. Sans nom, la Finlande se distinguait mal de l'URSS voisine :
	// même teinte « hors classement », aucune autre différence visuelle.
	labeledDespiteNonClass := map[string]bool{"URSS": true, "Finlande": true}
	for _, p := range neighbors {
		if p.labelR < thresholdLabelCountry || (p.class == "non-classe" && !labeledDespiteNonClass[p.name]) {
			continue
		}
		label(p, "")
	}
	label(france, "nom-france")
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

type pointPopulation struct {
	Year       int
	Population int64
}

type PopulationWars struct {
	SVG              template.HTML
	Pop1911, Pop1921 int64
	DecreaseAbsolute int64
	DecreasePct      float64
	Pop1936, Pop1954 int64
}

// loadPopulationWars : la population communale agrégée au niveau
// national (core.population_historique_commune, Insee 1876-1999) — la
// seule série de ce dossier qui montre un choc démographique mesuré
// indépendamment de tout dénombrement militaire ou civil. Le creux de la
// Première Guerre mondiale (1911→1921) est directement lisible ; celui de
// la Seconde ne l'est pas, la source sautant de 1936 à 1954 sans point en
// 1946.
// loadPopulationWars lit mv.population_nationale_annee (internal/
// matview) — plus le GROUP BY sur la totalité de core.
// population_historique_commune (657k lignes) que cette fonction refaisait
// à chaque construction.
func loadPopulationWars(ctx context.Context, pool *pgxpool.Pool) (*PopulationWars, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, population FROM mv.population_nationale_annee
		ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []pointPopulation
	values := map[int]int64{}
	for rows.Next() {
		var p pointPopulation
		if err := rows.Scan(&p.Year, &p.Population); err != nil {
			return nil, err
		}
		pts = append(pts, p)
		values[p.Year] = p.Population
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pts) == 0 {
		return nil, nil
	}
	pg := &PopulationWars{
		Pop1911: values[1911], Pop1921: values[1921],
		Pop1936: values[1936], Pop1954: values[1954],
	}
	if pg.Pop1911 > 0 {
		pg.DecreaseAbsolute = pg.Pop1911 - pg.Pop1921
		pg.DecreasePct = float64(pg.DecreaseAbsolute) / float64(pg.Pop1911) * 100
	}
	pg.SVG = drawPopulationWars(pts)
	return pg, nil
}

func drawPopulationWars(pts []pointPopulation) template.HTML {
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 40.0, 14.0, 14.0, 26.0
	yearStart, yearEnd := pts[0].Year, pts[len(pts)-1].Year
	maxVal := int64(0)
	for _, p := range pts {
		if p.Population > maxVal {
			maxVal = p.Population
		}
	}
	maxValM := float64(maxVal) / 1e6 * 1.1
	x := func(year int) float64 { return ml + (w-ml-mr)*float64(year-yearStart)/float64(yearEnd-yearStart) }
	y := func(popM float64) float64 { return mt + (h-mt-mb)*(1-popM/maxValM) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe population-guerres" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Population de la France, %d à %d">`, w, h, yearStart, yearEnd)

	// Deux bandes : 1914-1918 et 1939-1945 — pas des zones de rupture de
	// série (comme dans le graphique immigration), mais les deux guerres
	// elles-mêmes, pour lire le creux de 1921 et l'absence de creux visible
	// autour de 1954 dans leur contexte.
	for _, war := range [][2]int{{1914, 1918}, {1939, 1945}} {
		fmt.Fprintf(&b, `<rect class="bande-guerre" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`,
			x(war[0]), mt, x(war[1])-x(war[0]), h-mt-mb)
	}
	for _, bracket := range []float64{0, 20, 40, 60} {
		if bracket > maxValM {
			continue
		}
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(bracket), w-mr, y(bracket))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%d M</text>`, ml-6, y(bracket)+3, int(bracket))
	}

	var coords []string
	for _, p := range pts {
		coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(p.Year), y(float64(p.Population)/1e6)))
	}
	fmt.Fprintf(&b, `<polyline class="ligne-pop" points="%s"/>`, strings.Join(coords, " "))
	for _, p := range pts {
		fmt.Fprintf(&b, `<circle class="pt-pop" cx="%.2f" cy="%.2f" r="2.6"><title>%d : %s habitants</title></circle>`,
			x(p.Year), y(float64(p.Population)/1e6), p.Year, Count(int(p.Population)))
	}
	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, yearStart)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, yearEnd)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
