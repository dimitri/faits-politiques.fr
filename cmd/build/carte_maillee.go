package main

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Cartes « maillées » : la mosaïque des communes d'un territoire, avec le
// contour de leurs intercommunalités en surimpression. Ce n'est PAS une
// choroplèthe — aucune valeur n'y est encodée par couleur, donc pas de rampe
// ni de légende — mais une carte de repérage : elle répond à « où est ce
// département dans son détail communal, et comment se découpe-t-il en
// intercommunalités ». `geo.contour_cog` (IGN Admin Express COG CARTO) est la
// seule source qui porte un tracé par commune ; avant son arrivée, cette
// carte était impossible.
//
// Chaque territoire est dessiné dans SA PROPRE boîte englobante, à sa propre
// échelle : contrairement aux choroplèthes nationales, ces cartes ne se
// superposent pas les unes aux autres et n'ont donc pas besoin d'un repère
// commun.
const tolMaillee = 0.0008 // ≈ 80 m — assez fin pour une commune, inutile en dessous

// carteMaillee dessine un ensemble de communes (mosaïque fine) et, par
// dessus, le contour de leurs intercommunalités (trait épais) — sans jamais
// mêler les deux avec une couleur de valeur.
func carteMaillee(ctx context.Context, pool *pgxpool.Pool,
	whereCommunes, whereEPCI string, args []any, ariaLabel string) (template.HTML, int, error) {

	var srid int
	if err := pool.QueryRow(ctx,
		`SELECT srid_rendu FROM geo.contour_cog WHERE niveau='COMMUNE' AND `+whereCommunes+` LIMIT 1`,
		args...).Scan(&srid); err != nil {
		return "", 0, nil // aucun tracé (ex. millésime absent) : la carte est simplement omise
	}

	argsSrid := append(append([]any{}, args...), srid)
	pSrid := len(args) + 1

	// st_extent est une agrégation : la ligne existe même sans contour trouvé,
	// avec une valeur NULL (voir cmd/build/carte.go) — aucun tracé possible
	// alors, comme pour srid ci-dessus.
	var vbN sql.NullString
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,$%d::int)) e FROM geo.contour_cog
		      WHERE niveau='COMMUNE' AND %s) x`, pSrid, whereCommunes), argsSrid...).Scan(&vbN); err != nil {
		return "", 0, err
	}
	if !vbN.Valid {
		return "", 0, nil
	}
	vb := vbN.String
	champs := strings.Fields(vb)
	largeur, _ := strconv.ParseFloat(champs[2], 64)
	if largeur <= 0 {
		largeur = 1000
	}

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,%v),$%d::int),1,0)
		FROM geo.contour_cog WHERE niveau='COMMUNE' AND %s ORDER BY code`,
		tolMaillee, pSrid, whereCommunes), argsSrid...)
	if err != nil {
		return "", 0, err
	}
	var communes strings.Builder
	n := 0
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			rows.Close()
			return "", 0, err
		}
		fmt.Fprintf(&communes, `<path d="%s"/>`, d)
		n++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", 0, err
	}

	argsEPCI := append(append([]any{}, args...), srid)
	rowsE, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT nom, st_assvg(st_transform(st_simplifypreservetopology(geom,%v),$%d::int),1,0)
		FROM geo.contour_cog WHERE niveau='EPCI' AND %s ORDER BY nom`,
		tolMaillee, pSrid, whereEPCI), argsEPCI...)
	if err != nil {
		return "", 0, err
	}
	var epcis strings.Builder
	for rowsE.Next() {
		var nom, d string
		if err := rowsE.Scan(&nom, &d); err != nil {
			rowsE.Close()
			return "", 0, err
		}
		fmt.Fprintf(&epcis, `<path d="%s"><title>%s</title></path>`,
			d, template.HTMLEscapeString(nom))
	}
	rowsE.Close()
	if err := rowsE.Err(); err != nil {
		return "", 0, err
	}

	// Le trait des intercommunalités reste un REPÈRE, jamais la vedette de la
	// carte : trop épais, il écrase la mosaïque des communes qu'il est censé
	// éclairer — vu sur un département aux formes découpées (Finistère).
	swC, swE := largeur/900, largeur/420
	svg := fmt.Sprintf(
		`<svg viewBox="%s" class="geo maille" role="img" aria-label="%s">`+
			`<g class="maille-c" stroke-width="%.0f">%s</g>`+
			`<g class="maille-e" stroke-width="%.0f">%s</g></svg>`,
		vb, template.HTMLEscapeString(ariaLabel), swC, communes.String(), swE, epcis.String())
	return template.HTML(svg), n, nil
}

// carteCommunesDepartement : toutes les communes d'un département, avec le
// contour des intercommunalités qui y ont leur siège. Un « département »
// budgétaire peut recouvrir plusieurs codes géographiques du COG — l'Alsace en
// réunit deux (67, 68) sous un seul budget — d'où la liste plutôt qu'un code.
func carteCommunesDepartement(ctx context.Context, pool *pgxpool.Pool, codesDept []string,
	millesime int, nomDept string) (template.HTML, int, error) {

	cond := "code_departement = ANY($1) AND cog_millesime=$2"
	return carteMaillee(ctx, pool, cond, cond, []any{codesDept, millesime},
		"Communes et intercommunalités du département "+nomDept)
}

// codesCOGDe : les codes géographiques du COG que recouvre un code
// budgétaire de département. Un fait de droit (fusions Alsace, Métropole de
// Lyon), documenté ici plutôt que déduit d'une donnée.
func codesCOGDe(codeBudget string) []string {
	switch codeBudget {
	case "67A":
		return []string{"67", "68"}
	case "691":
		return []string{"69"}
	default:
		return []string{codeBudget}
	}
}

// carteCommunesEPCI : les seules communes membres d'une intercommunalité, et
// son propre contour en surimpression — sans le reste du département autour.
// Gardée comme repli pour carteCommunesEPCIAvecContexte (millésime ou
// département de rattachement absent) — voir cette fonction pour la carte
// affichée en temps normal sur une page d'EPCI.
func carteCommunesEPCI(ctx context.Context, pool *pgxpool.Pool, siren string,
	millesime int, nomEPCI string) (template.HTML, int, error) {

	whereC := `code IN (SELECT commune_code FROM core.epci_membre ` +
		`WHERE epci_siren=$1 AND cog_millesime=$2) AND cog_millesime=$2`
	whereE := "code=$1 AND cog_millesime=$2"
	return carteMaillee(ctx, pool, whereC, whereE, []any{siren, millesime},
		"Communes membres de "+nomEPCI)
}

// contexteDept : le fond de carte d'un département — toutes ses communes,
// calculé UNE SEULE FOIS par département plutôt qu'une fois par groupement
// qui y a son siège (un département compte souvent plusieurs dizaines
// d'EPCI : refaire cette requête pour chacun avait fait passer la
// génération des pages d'intercommunalité de ~20 s à plus de 5 min).
type contexteDept struct {
	Srid       int
	ViewBox    string
	Largeur    float64
	CommunesSVG string
}

// chargerContexteDept charge le fond de carte d'un département, à appeler une
// fois par département puis à réutiliser pour chaque groupement qui y a son
// siège (voir carteCommunesEPCIAvecContexte).
func chargerContexteDept(ctx context.Context, pool *pgxpool.Pool, codeDept string, millesime int) (*contexteDept, error) {
	var c contexteDept
	if err := pool.QueryRow(ctx,
		`SELECT srid_rendu FROM geo.contour_cog
		 WHERE niveau='COMMUNE' AND code_departement=$1 AND cog_millesime=$2 LIMIT 1`,
		codeDept, millesime).Scan(&c.Srid); err != nil {
		return nil, nil // département sans contour dans ce millésime : repli sur carteCommunesEPCI
	}

	// st_extent est une agrégation : la ligne existe même sans contour trouvé,
	// avec une valeur NULL (voir cmd/build/carte.go).
	var viewBoxN sql.NullString
	if err := pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,$1::int)) e FROM geo.contour_cog
		      WHERE niveau='COMMUNE' AND code_departement=$2 AND cog_millesime=$3) x`,
		c.Srid, codeDept, millesime).Scan(&viewBoxN); err != nil {
		return nil, err
	}
	if !viewBoxN.Valid {
		return nil, nil // département sans contour dans ce millésime : repli sur carteCommunesEPCI
	}
	c.ViewBox = viewBoxN.String
	champs := strings.Fields(c.ViewBox)
	c.Largeur, _ = strconv.ParseFloat(champs[2], 64)
	if c.Largeur <= 0 {
		c.Largeur = 1000
	}

	// Toutes les communes du département, sans exclure celles d'un groupement
	// en particulier : la mosaïque mise en évidence d'un groupement est
	// dessinée PAR-DESSUS ce fond dans le même ordre de calques, ce qui la
	// rend visible sans qu'il faille exclure ses communes ici — un fond
	// unique sert donc tous les groupements du département sans distinction.
	rows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),$2::int),1,0)
		FROM geo.contour_cog WHERE niveau='COMMUNE' AND code_departement=$3 AND cog_millesime=$4
		ORDER BY code`,
		tolMaillee, c.Srid, codeDept, millesime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, `<path d="%s"/>`, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	c.CommunesSVG = b.String()
	return &c, nil
}

// carteCommunesEPCIAvecContexte : comme carteCommunesEPCI, mais cadrée sur le
// département de rattachement (ctxDept, chargé une fois par
// chargerContexteDept) plutôt que sur la seule emprise du groupement — les
// autres communes du département apparaissent en gris neutre derrière la
// mosaïque mise en évidence. Une carte de groupement isolée, sans rien
// autour, ne montre pas où il se situe parmi ses voisins. Retombe sur
// carteCommunesEPCI si aucun contexte départemental n'est disponible
// (outre-mer sans siège identifié, par exemple).
func carteCommunesEPCIAvecContexte(ctx context.Context, pool *pgxpool.Pool, ctxDept *contexteDept,
	siren string, millesime int, nomEPCI string) (template.HTML, int, error) {

	if ctxDept == nil {
		return carteCommunesEPCI(ctx, pool, siren, millesime, nomEPCI)
	}

	rows, err := pool.Query(ctx, `
		SELECT st_assvg(st_transform(st_simplifypreservetopology(geom,$1),$2::int),1,0)
		FROM geo.contour_cog WHERE niveau='COMMUNE' AND cog_millesime=$4
		  AND code IN (SELECT commune_code FROM core.epci_membre
		               WHERE epci_siren=$3 AND cog_millesime=$4)
		ORDER BY code`,
		tolMaillee, ctxDept.Srid, siren, millesime)
	if err != nil {
		return "", 0, err
	}
	var communes strings.Builder
	n := 0
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			rows.Close()
			return "", 0, err
		}
		fmt.Fprintf(&communes, `<path d="%s"/>`, d)
		n++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", 0, err
	}
	if n == 0 {
		return carteCommunesEPCI(ctx, pool, siren, millesime, nomEPCI)
	}

	rowsE, err := pool.Query(ctx, `
		SELECT nom, st_assvg(st_transform(st_simplifypreservetopology(geom,$1),$2::int),1,0)
		FROM geo.contour_cog WHERE niveau='EPCI' AND code=$3 AND cog_millesime=$4`,
		tolMaillee, ctxDept.Srid, siren, millesime)
	if err != nil {
		return "", 0, err
	}
	var epcis strings.Builder
	for rowsE.Next() {
		var nom, d string
		if err := rowsE.Scan(&nom, &d); err != nil {
			rowsE.Close()
			return "", 0, err
		}
		fmt.Fprintf(&epcis, `<path d="%s"><title>%s</title></path>`, d, template.HTMLEscapeString(nom))
	}
	rowsE.Close()
	if err := rowsE.Err(); err != nil {
		return "", 0, err
	}

	// Même logique d'épaisseur proportionnelle que carteMaillee — le contexte
	// reste le plus fin des trois traits, jamais la vedette de la carte.
	swCtx, swC, swE := ctxDept.Largeur/1400, ctxDept.Largeur/900, ctxDept.Largeur/420
	svg := fmt.Sprintf(
		`<svg viewBox="%s" class="geo maille" role="img" aria-label="%s">`+
			`<g class="maille-ctx" stroke-width="%.0f">%s</g>`+
			`<g class="maille-c" stroke-width="%.0f">%s</g>`+
			`<g class="maille-e" stroke-width="%.0f">%s</g></svg>`,
		ctxDept.ViewBox, template.HTMLEscapeString(nomEPCI+" et ses environs dans le département"),
		swCtx, ctxDept.CommunesSVG, swC, communes.String(), swE, epcis.String())
	return template.HTML(svg), n, nil
}
