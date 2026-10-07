package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StatsIndochinaPartition alimente la figure dédiée de
// docs/guerres-decolonisation-donnees.md (§ 1 et § 5) : l'Indochine
// française avant les accords de Genève, puis la partition du Viêt Nam
// qu'ils actent, jusqu'à la chute de Saïgon en 1975.
//
// Ces données ne rejoignent PAS la carte de l'empire colonial
// (dessinerCarteEmpireColonial, empire_colonial.go) : cette dernière classe
// chaque territoire par « vague de décolonisation », c'est-à-dire par date
// d'indépendance vis-à-vis de la France — un cadre qui ne s'applique pas à
// la République démocratique du Viêt Nam (Nord) ni à la République du Viêt
// Nam (Sud), nées après le départ français, pas de son fait direct (leur
// géométrie vit dans geo.indochine_partition_1954, table séparée — voir la
// migration 0151). Forcer ces deux entités dans la carte des 22 territoires
// aurait fait comme si elles avaient « une date d'indépendance » de la
// France, ce qui est faux et aurait cassé la lecture des trois vagues déjà
// établie (§ 2 du dossier empire colonial). D'où une figure séparée, plus
// petite, à deux volets (avant / après Genève) plutôt qu'un ajout à la carte
// existante.
type StatsIndochinaPartition struct {
	MapSVG     template.HTML
	Population template.HTML
}

// loadIndochinaPartition construit la figure « avant / après Genève » :
// à gauche, l'Indochine française à sa dernière extension (Cambodge, Laos,
// Viêt Nam unifié — les mêmes géométries CShapes que geo.territoire_colonial,
// § 2 du dossier empire colonial) ; à droite, la partition de 1954
// (geo.indochine_partition_1954), avec le Laos et le Cambodge redessinés en
// ton neutre pour situer géographiquement les deux Viêt Nam sans laisser
// croire qu'ils ont, eux aussi, été partitionnés.
func loadIndochinaPartition(ctx context.Context, pool *pgxpool.Pool) (*StatsIndochinaPartition, error) {
	const tolerance = 0.01 // degrés : région bien plus petite que le fond « monde » de empire_colonial.go

	before, err := roomsBeforeGeneva(ctx, pool, tolerance)
	if err != nil {
		return nil, err
	}
	after, err := roomsAfterGeneva(ctx, pool, tolerance)
	if err != nil {
		return nil, err
	}
	if len(before) == 0 && len(after) == 0 {
		return nil, nil
	}

	// Chine, Thaïlande, Birmanie, Malaisie : sans elles, le Cambodge, le Laos
	// et le Viêt Nam flottaient seuls sur un fond uni, reconnaissables
	// seulement par qui connaît déjà leur silhouette. Chacune ne porte ici
	// que sa part la plus proche (voir piecesVoisinsAsie) — un repère de
	// contexte, jamais le sujet de la carte. Les mêmes voisins apparaissent
	// sur les deux volets : l'orientation géographique ne change pas entre
	// 1954 et après.
	neighbors, err := roomsNeighborsAsia(ctx, pool, tolerance)
	if err != nil {
		return nil, err
	}
	before = append(append([]roomIndochina{}, neighbors...), before...)
	after = append(append([]roomIndochina{}, neighbors...), after...)

	population, err := loadPopulationIndochina(ctx, pool)
	if err != nil {
		return nil, err
	}

	// Emprise mesurée sur les données (Cambodge, Laos, Viêt Nam, plus la
	// part retenue des quatre pays voisins), pas une convention arbitraire.
	// Elle laisse aussi de la mer visible à l'est et au sud (mer de Chine
	// méridionale, golfe de Thaïlande) — sans quoi le fond bleu clair de la
	// carte (voir .geo.indochine-1954, style.css) ne se distinguait pas
	// d'un pays non chargé.
	const viewBox = "93 -25 17 21"

	// Seuil de rayon (degrés) sous lequel un nom ne tiendrait pas
	// lisiblement — aucun des sept pays de cette carte n'est concerné
	// (rayon minimal observé environ 0,87°), gardé pour ne pas dépendre
	// silencieusement de cette hypothèse si la liste change un jour.
	const thresholdLabelIndochina = 0.3
	draw := func(pp []roomIndochina, aria string) string {
		var b strings.Builder
		fmt.Fprintf(&b, `<svg viewBox="%s" class="geo monde indochine-1954" role="img" aria-label="%s">`,
			viewBox, template.HTMLEscapeString(aria))
		for _, p := range pp {
			fmt.Fprintf(&b, `<path class="%s" d="%s" data-nom="%s"><title>%s</title></path>`,
				p.class, p.path, template.HTMLEscapeString(p.name), template.HTMLEscapeString(p.title))
		}
		// Viewbox à l'échelle du degré (17 unités de large, contrairement
		// aux cartes en Lambert-93/LAEA du dossier Seconde Guerre mondiale,
		// qui se comptent en centaines de milliers d'unités) : une taille de
		// police directe reste ici dans une plage normale, jamais assez
		// grande pour buter sur le rendu de glyphes de Chromium contourné
		// là-bas par une transformation matricielle — inutile ici.
		for _, p := range pp {
			if p.labelR < thresholdLabelIndochina {
				continue
			}
			class := "repere-asie"
			if p.class == "voisin" {
				class += " repere-contexte"
			}
			fmt.Fprintf(&b, `<text class="%s" x="%.3f" y="%.3f" text-anchor="middle">%s</text>`,
				class, p.labelX, p.labelY, template.HTMLEscapeString(p.name))
		}
		b.WriteString(`</svg>`)
		return b.String()
	}

	// L'étiquette de survol (site.js) cherche un « .etiquette-survol » dans
	// le même parent que « .carte-pleine » : présente ici même sans
	// JavaScript (juste vide), elle ne casse rien pour qui ne l'exécute pas.
	var out strings.Builder
	out.WriteString(`<div class="deux-cartes">`)
	fmt.Fprintf(&out, `<div><div class="carte-pleine">%s</div><p class="etiquette-survol" aria-live="polite"></p>`+
		`<p class="sous-legende">Avant les accords de Genève</p></div>`,
		draw(before, "L'Indochine française avant les accords de Genève, à sa dernière extension"))
	fmt.Fprintf(&out, `<div><div class="carte-pleine">%s</div><p class="etiquette-survol" aria-live="polite"></p>`+
		`<p class="sous-legende">Après (1954-1975)</p></div>`,
		draw(after, "La partition du Viêt Nam après les accords de Genève de 1954, jusqu'à la chute de Saïgon en 1975"))
	out.WriteString(`</div>`)

	return &StatsIndochinaPartition{MapSVG: template.HTML(out.String()), Population: population}, nil
}

// loadPopulationIndochina : trois petits graphiques en barres (Viêt Nam,
// Cambodge, Laos), pas une seule courbe partagée — le Viêt Nam pèse dix fois
// le Cambodge ou le Laos sur toute la période, une échelle commune écraserait
// les deux plus petits pays à une ligne plate. Réutilise courbe()
// (cartepage.go), déjà en place pour ce même patron ailleurs sur le site.
// Source : CLIO-INFRA (core.population_indochine_historique, migration
// 0153) — frontières ACTUELLES, pas coloniales, voir le commentaire de la
// migration.
func loadPopulationIndochina(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	format := func(v float64) string { return Count(int(v+0.5)) + " milliers" }
	var out strings.Builder
	out.WriteString(`<div class="deux-cartes">`)
	for _, country := range []string{"Vietnam", "Cambodge", "Laos"} {
		rows, err := pool.Query(ctx, `
			SELECT annee, population_milliers::float8 FROM core.population_indochine_historique
			WHERE pays = $1 ORDER BY annee`, country)
		if err != nil {
			return "", err
		}
		var pts []PointYear
		for rows.Next() {
			var p PointYear
			if err := rows.Scan(&p.Year, &p.Value); err != nil {
				rows.Close()
				return "", err
			}
			pts = append(pts, p)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return "", err
		}
		rows.Close()
		if len(pts) < 2 {
			continue
		}
		fmt.Fprintf(&out, `<div>%s<p class="sous-legende">%s</p></div>`, curve(pts, format), country)
	}
	out.WriteString(`</div>`)
	return template.HTML(out.String()), nil
}

type roomIndochina struct {
	class, name, title, path string
	labelX, labelY, labelR   float64
}

// nameShort : le nom affiché au survol (étiquette courte) plutôt que le nom
// de colonne complet — geo.territoire_colonial nomme le Viêt Nam par son
// découpage colonial (Cochinchine, Annam, Tonkin), exact pour un intitulé
// de tableau, imprononçable pour une étiquette de carte.
func nameShort(territory string) string {
	if territory == "Vietnam (Cochinchine, Annam, Tonkin)" {
		return "Viêt Nam"
	}
	return territory
}

// nameShortPartition : « République démocratique du Viêt Nam (Nord) » et
// « République du Viêt Nam (Sud) » débordaient de leur carte, moitié moins
// large que celle de l'empire colonial — le nom complet officiel reste dans
// le titre (survol), l'étiquette n'a besoin que de distinguer les deux.
func nameShortPartition(territory, camp string) string {
	if camp == "nord" {
		return "Viêt Nam (Nord)"
	}
	if camp == "sud" {
		return "Viêt Nam (Sud)"
	}
	return territory
}

// roomsBeforeGeneva : Cambodge, Laos et le Viêt Nam unifié, à leur dernière
// extension avant l'indépendance — les trois lignes de
// geo.territoire_colonial pour l'Asie du Sud-Est (§ 2 du dossier empire
// colonial), redessinées ici dans une seule couleur neutre : ce volet montre
// une étendue, pas une chronologie (déjà montrée par la carte des 22
// territoires).
func roomsBeforeGeneva(ctx context.Context, pool *pgxpool.Pool, tolerance float64) ([]roomIndochina, error) {
	rows, err := pool.Query(ctx, `
		SELECT territoire, note_independance,
		       st_assvg(g, 0, 2), st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM (SELECT territoire, note_independance, st_simplifypreservetopology(geom, $1) AS g
		      FROM geo.territoire_colonial
		      WHERE territoire IN ('Cambodge', 'Laos', 'Vietnam (Cochinchine, Annam, Tonkin)')
		        AND geom IS NOT NULL) x,
		     LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l`, tolerance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pp []roomIndochina
	for rows.Next() {
		var territory, note, path string
		var p roomIndochina
		if err := rows.Scan(&territory, &note, &path, &p.labelX, &p.labelY, &p.labelR); err != nil {
			return nil, err
		}
		p.class, p.name, p.title, p.path = "avant", nameShort(territory), territory+" — "+note, path
		pp = append(pp, p)
	}
	return pp, rows.Err()
}

// roomsAfterGeneva : la République démocratique du Viêt Nam (Nord) et la
// République du Viêt Nam (Sud) (geo.indochine_partition_1954), plus le Laos
// et le Cambodge redessinés en ton neutre — déjà indépendants depuis 1953,
// ils ne sont pas concernés par la partition, mais les faire disparaître
// aurait laissé les deux Viêt Nam flotter sans repère régional.
func roomsAfterGeneva(ctx context.Context, pool *pgxpool.Pool, tolerance float64) ([]roomIndochina, error) {
	var pp []roomIndochina

	ctxRows, err := pool.Query(ctx, `
		SELECT territoire, note_independance,
		       st_assvg(g, 0, 2), st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM (SELECT territoire, note_independance, st_simplifypreservetopology(geom, $1) AS g
		      FROM geo.territoire_colonial
		      WHERE territoire IN ('Cambodge', 'Laos') AND geom IS NOT NULL) x,
		     LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l`, tolerance)
	if err != nil {
		return nil, err
	}
	for ctxRows.Next() {
		var territory, note, path string
		var p roomIndochina
		if err := ctxRows.Scan(&territory, &note, &path, &p.labelX, &p.labelY, &p.labelR); err != nil {
			ctxRows.Close()
			return nil, err
		}
		p.class, p.name, p.title, p.path = "contexte", nameShort(territory), territory+" — déjà indépendant en 1954. "+note, path
		pp = append(pp, p)
	}
	if err := ctxRows.Err(); err != nil {
		ctxRows.Close()
		return nil, err
	}
	ctxRows.Close()

	shareRows, err := pool.Query(ctx, `
		SELECT territoire, camp, note, date_debut::text, date_fin::text,
		       st_assvg(g, 0, 2), st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM (SELECT territoire, camp, note, date_debut, date_fin, st_simplifypreservetopology(geom, $1) AS g
		      FROM geo.indochine_partition_1954 WHERE geom IS NOT NULL) x,
		     LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l
		ORDER BY camp`, tolerance)
	if err != nil {
		return nil, err
	}
	defer shareRows.Close()
	for shareRows.Next() {
		var territory, camp, note, start, end, path string
		var p roomIndochina
		if err := shareRows.Scan(&territory, &camp, &note, &start, &end, &path, &p.labelX, &p.labelY, &p.labelR); err != nil {
			return nil, err
		}
		p.class, p.name, p.title, p.path = camp, nameShortPartition(territory, camp), territory+" ("+start+" – "+end+") — "+note, path
		pp = append(pp, p)
	}
	return pp, shareRows.Err()
}

// roomsNeighborsAsia : Chine, Thaïlande, Birmanie et Malaisie, simples repères
// géographiques — jamais le sujet de cette carte, jamais colorés comme le
// Cambodge, le Laos ou le Viêt Nam. Chacune n'est gardée que sur la part la
// plus proche de l'Indochine (emprise fixe avant simplification, même
// technique que le clipOuestEurope de cmd/build/seconde_guerre_mondiale.go) :
// la Chine seule s'étend jusqu'au 53ᵉ parallèle et la Malaisie jusqu'à
// Bornéo, bien au-delà de ce qu'une carte de l'Indochine doit montrer.
func roomsNeighborsAsia(ctx context.Context, pool *pgxpool.Pool, tolerance float64) ([]roomIndochina, error) {
	rows, err := pool.Query(ctx, `
		SELECT nom_fr, st_assvg(g, 0, 2), st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM (SELECT nom_fr, st_simplifypreservetopology(
		                 st_intersection(geom, st_makeenvelope(93, 4, 112, 25, 4326)), $1) AS g
		      FROM geo.contour_pays
		      WHERE nom_fr IN ('République populaire de Chine', 'Thaïlande', 'Birmanie', 'Malaisie')) x,
		     LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l`, tolerance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nameUsual := map[string]string{"République populaire de Chine": "Chine"}
	var pp []roomIndochina
	for rows.Next() {
		var nameFr, path string
		var p roomIndochina
		if err := rows.Scan(&nameFr, &path, &p.labelX, &p.labelY, &p.labelR); err != nil {
			return nil, err
		}
		name := nameUsual[nameFr]
		if name == "" {
			name = nameFr
		}
		p.class, p.name, p.title, p.path = "voisin", name, nameFr+" — repère géographique", path
		pp = append(pp, p)
	}
	return pp, rows.Err()
}
