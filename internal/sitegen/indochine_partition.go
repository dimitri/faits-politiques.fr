package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StatsIndochinePartition alimente la figure dédiée de
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
type StatsIndochinePartition struct {
	CarteSVG   template.HTML
	Population template.HTML
}

// chargerIndochinePartition construit la figure « avant / après Genève » :
// à gauche, l'Indochine française à sa dernière extension (Cambodge, Laos,
// Viêt Nam unifié — les mêmes géométries CShapes que geo.territoire_colonial,
// § 2 du dossier empire colonial) ; à droite, la partition de 1954
// (geo.indochine_partition_1954), avec le Laos et le Cambodge redessinés en
// ton neutre pour situer géographiquement les deux Viêt Nam sans laisser
// croire qu'ils ont, eux aussi, été partitionnés.
func chargerIndochinePartition(ctx context.Context, pool *pgxpool.Pool) (*StatsIndochinePartition, error) {
	const tol = 0.01 // degrés : région bien plus petite que le fond « monde » de empire_colonial.go

	avant, err := piecesAvantGeneve(ctx, pool, tol)
	if err != nil {
		return nil, err
	}
	apres, err := piecesApresGeneve(ctx, pool, tol)
	if err != nil {
		return nil, err
	}
	if len(avant) == 0 && len(apres) == 0 {
		return nil, nil
	}

	// Chine, Thaïlande, Birmanie, Malaisie : sans elles, le Cambodge, le Laos
	// et le Viêt Nam flottaient seuls sur un fond uni, reconnaissables
	// seulement par qui connaît déjà leur silhouette. Chacune ne porte ici
	// que sa part la plus proche (voir piecesVoisinsAsie) — un repère de
	// contexte, jamais le sujet de la carte. Les mêmes voisins apparaissent
	// sur les deux volets : l'orientation géographique ne change pas entre
	// 1954 et après.
	voisins, err := piecesVoisinsAsie(ctx, pool, tol)
	if err != nil {
		return nil, err
	}
	avant = append(append([]pieceIndochine{}, voisins...), avant...)
	apres = append(append([]pieceIndochine{}, voisins...), apres...)

	population, err := chargerPopulationIndochine(ctx, pool)
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
	const seuilEtiquetteIndochine = 0.3
	dessiner := func(pp []pieceIndochine, aria string) string {
		var b strings.Builder
		fmt.Fprintf(&b, `<svg viewBox="%s" class="geo monde indochine-1954" role="img" aria-label="%s">`,
			viewBox, template.HTMLEscapeString(aria))
		for _, p := range pp {
			fmt.Fprintf(&b, `<path class="%s" d="%s" data-nom="%s"><title>%s</title></path>`,
				p.classe, p.chemin, template.HTMLEscapeString(p.nom), template.HTMLEscapeString(p.titre))
		}
		// Viewbox à l'échelle du degré (17 unités de large, contrairement
		// aux cartes en Lambert-93/LAEA du dossier Seconde Guerre mondiale,
		// qui se comptent en centaines de milliers d'unités) : une taille de
		// police directe reste ici dans une plage normale, jamais assez
		// grande pour buter sur le rendu de glyphes de Chromium contourné
		// là-bas par une transformation matricielle — inutile ici.
		for _, p := range pp {
			if p.labelR < seuilEtiquetteIndochine {
				continue
			}
			classe := "repere-asie"
			if p.classe == "voisin" {
				classe += " repere-contexte"
			}
			fmt.Fprintf(&b, `<text class="%s" x="%.3f" y="%.3f" text-anchor="middle">%s</text>`,
				classe, p.labelX, p.labelY, template.HTMLEscapeString(p.nom))
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
		dessiner(avant, "L'Indochine française avant les accords de Genève, à sa dernière extension"))
	fmt.Fprintf(&out, `<div><div class="carte-pleine">%s</div><p class="etiquette-survol" aria-live="polite"></p>`+
		`<p class="sous-legende">Après (1954-1975)</p></div>`,
		dessiner(apres, "La partition du Viêt Nam après les accords de Genève de 1954, jusqu'à la chute de Saïgon en 1975"))
	out.WriteString(`</div>`)

	return &StatsIndochinePartition{CarteSVG: template.HTML(out.String()), Population: population}, nil
}

// chargerPopulationIndochine : trois petits graphiques en barres (Viêt Nam,
// Cambodge, Laos), pas une seule courbe partagée — le Viêt Nam pèse dix fois
// le Cambodge ou le Laos sur toute la période, une échelle commune écraserait
// les deux plus petits pays à une ligne plate. Réutilise courbe()
// (cartepage.go), déjà en place pour ce même patron ailleurs sur le site.
// Source : CLIO-INFRA (core.population_indochine_historique, migration
// 0153) — frontières ACTUELLES, pas coloniales, voir le commentaire de la
// migration.
func chargerPopulationIndochine(ctx context.Context, pool *pgxpool.Pool) (template.HTML, error) {
	format := func(v float64) string { return Nombre(int(v+0.5)) + " milliers" }
	var out strings.Builder
	out.WriteString(`<div class="deux-cartes">`)
	for _, pays := range []string{"Vietnam", "Cambodge", "Laos"} {
		rows, err := pool.Query(ctx, `
			SELECT annee, population_milliers::float8 FROM core.population_indochine_historique
			WHERE pays = $1 ORDER BY annee`, pays)
		if err != nil {
			return "", err
		}
		var pts []PointAnnee
		for rows.Next() {
			var p PointAnnee
			if err := rows.Scan(&p.Annee, &p.Valeur); err != nil {
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
		fmt.Fprintf(&out, `<div>%s<p class="sous-legende">%s</p></div>`, courbe(pts, format), pays)
	}
	out.WriteString(`</div>`)
	return template.HTML(out.String()), nil
}

type pieceIndochine struct {
	classe, nom, titre, chemin string
	labelX, labelY, labelR     float64
}

// nomCourt : le nom affiché au survol (étiquette courte) plutôt que le nom
// de colonne complet — geo.territoire_colonial nomme le Viêt Nam par son
// découpage colonial (Cochinchine, Annam, Tonkin), exact pour un intitulé
// de tableau, imprononçable pour une étiquette de carte.
func nomCourt(territoire string) string {
	if territoire == "Vietnam (Cochinchine, Annam, Tonkin)" {
		return "Viêt Nam"
	}
	return territoire
}

// nomCourtPartition : « République démocratique du Viêt Nam (Nord) » et
// « République du Viêt Nam (Sud) » débordaient de leur carte, moitié moins
// large que celle de l'empire colonial — le nom complet officiel reste dans
// le titre (survol), l'étiquette n'a besoin que de distinguer les deux.
func nomCourtPartition(territoire, camp string) string {
	if camp == "nord" {
		return "Viêt Nam (Nord)"
	}
	if camp == "sud" {
		return "Viêt Nam (Sud)"
	}
	return territoire
}

// piecesAvantGeneve : Cambodge, Laos et le Viêt Nam unifié, à leur dernière
// extension avant l'indépendance — les trois lignes de
// geo.territoire_colonial pour l'Asie du Sud-Est (§ 2 du dossier empire
// colonial), redessinées ici dans une seule couleur neutre : ce volet montre
// une étendue, pas une chronologie (déjà montrée par la carte des 22
// territoires).
func piecesAvantGeneve(ctx context.Context, pool *pgxpool.Pool, tol float64) ([]pieceIndochine, error) {
	rows, err := pool.Query(ctx, `
		SELECT territoire, note_independance,
		       st_assvg(g, 0, 2), st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM (SELECT territoire, note_independance, st_simplifypreservetopology(geom, $1) AS g
		      FROM geo.territoire_colonial
		      WHERE territoire IN ('Cambodge', 'Laos', 'Vietnam (Cochinchine, Annam, Tonkin)')
		        AND geom IS NOT NULL) x,
		     LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l`, tol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pp []pieceIndochine
	for rows.Next() {
		var territoire, note, chemin string
		var p pieceIndochine
		if err := rows.Scan(&territoire, &note, &chemin, &p.labelX, &p.labelY, &p.labelR); err != nil {
			return nil, err
		}
		p.classe, p.nom, p.titre, p.chemin = "avant", nomCourt(territoire), territoire+" — "+note, chemin
		pp = append(pp, p)
	}
	return pp, rows.Err()
}

// piecesApresGeneve : la République démocratique du Viêt Nam (Nord) et la
// République du Viêt Nam (Sud) (geo.indochine_partition_1954), plus le Laos
// et le Cambodge redessinés en ton neutre — déjà indépendants depuis 1953,
// ils ne sont pas concernés par la partition, mais les faire disparaître
// aurait laissé les deux Viêt Nam flotter sans repère régional.
func piecesApresGeneve(ctx context.Context, pool *pgxpool.Pool, tol float64) ([]pieceIndochine, error) {
	var pp []pieceIndochine

	ctxRows, err := pool.Query(ctx, `
		SELECT territoire, note_independance,
		       st_assvg(g, 0, 2), st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM (SELECT territoire, note_independance, st_simplifypreservetopology(geom, $1) AS g
		      FROM geo.territoire_colonial
		      WHERE territoire IN ('Cambodge', 'Laos') AND geom IS NOT NULL) x,
		     LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l`, tol)
	if err != nil {
		return nil, err
	}
	for ctxRows.Next() {
		var territoire, note, chemin string
		var p pieceIndochine
		if err := ctxRows.Scan(&territoire, &note, &chemin, &p.labelX, &p.labelY, &p.labelR); err != nil {
			ctxRows.Close()
			return nil, err
		}
		p.classe, p.nom, p.titre, p.chemin = "contexte", nomCourt(territoire), territoire+" — déjà indépendant en 1954. "+note, chemin
		pp = append(pp, p)
	}
	if err := ctxRows.Err(); err != nil {
		ctxRows.Close()
		return nil, err
	}
	ctxRows.Close()

	partRows, err := pool.Query(ctx, `
		SELECT territoire, camp, note, date_debut::text, date_fin::text,
		       st_assvg(g, 0, 2), st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM (SELECT territoire, camp, note, date_debut, date_fin, st_simplifypreservetopology(geom, $1) AS g
		      FROM geo.indochine_partition_1954 WHERE geom IS NOT NULL) x,
		     LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l
		ORDER BY camp`, tol)
	if err != nil {
		return nil, err
	}
	defer partRows.Close()
	for partRows.Next() {
		var territoire, camp, note, debut, fin, chemin string
		var p pieceIndochine
		if err := partRows.Scan(&territoire, &camp, &note, &debut, &fin, &chemin, &p.labelX, &p.labelY, &p.labelR); err != nil {
			return nil, err
		}
		p.classe, p.nom, p.titre, p.chemin = camp, nomCourtPartition(territoire, camp), territoire+" ("+debut+" – "+fin+") — "+note, chemin
		pp = append(pp, p)
	}
	return pp, partRows.Err()
}

// piecesVoisinsAsie : Chine, Thaïlande, Birmanie et Malaisie, simples repères
// géographiques — jamais le sujet de cette carte, jamais colorés comme le
// Cambodge, le Laos ou le Viêt Nam. Chacune n'est gardée que sur la part la
// plus proche de l'Indochine (emprise fixe avant simplification, même
// technique que le clipOuestEurope de cmd/build/seconde_guerre_mondiale.go) :
// la Chine seule s'étend jusqu'au 53ᵉ parallèle et la Malaisie jusqu'à
// Bornéo, bien au-delà de ce qu'une carte de l'Indochine doit montrer.
func piecesVoisinsAsie(ctx context.Context, pool *pgxpool.Pool, tol float64) ([]pieceIndochine, error) {
	rows, err := pool.Query(ctx, `
		SELECT nom_fr, st_assvg(g, 0, 2), st_x((ic).center), -st_y((ic).center), (ic).radius
		FROM (SELECT nom_fr, st_simplifypreservetopology(
		                 st_intersection(geom, st_makeenvelope(93, 4, 112, 25, 4326)), $1) AS g
		      FROM geo.contour_pays
		      WHERE nom_fr IN ('République populaire de Chine', 'Thaïlande', 'Birmanie', 'Malaisie')) x,
		     LATERAL (SELECT ST_MaximumInscribedCircle(g) AS ic) l`, tol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nomUsuel := map[string]string{"République populaire de Chine": "Chine"}
	var pp []pieceIndochine
	for rows.Next() {
		var nomFr, chemin string
		var p pieceIndochine
		if err := rows.Scan(&nomFr, &chemin, &p.labelX, &p.labelY, &p.labelR); err != nil {
			return nil, err
		}
		nom := nomUsuel[nomFr]
		if nom == "" {
			nom = nomFr
		}
		p.classe, p.nom, p.titre, p.chemin = "voisin", nom, nomFr+" — repère géographique", chemin
		pp = append(pp, p)
	}
	return pp, rows.Err()
}
