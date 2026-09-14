package main

import (
	"context"
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

	var vb string
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,$%d::int)) e FROM geo.contour_cog
		      WHERE niveau='COMMUNE' AND %s) x`, pSrid, whereCommunes), argsSrid...).Scan(&vb); err != nil {
		return "", 0, err
	}
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
func carteCommunesEPCI(ctx context.Context, pool *pgxpool.Pool, siren string,
	millesime int, nomEPCI string) (template.HTML, int, error) {

	whereC := `code IN (SELECT commune_code FROM core.epci_membre ` +
		`WHERE epci_siren=$1 AND cog_millesime=$2) AND cog_millesime=$2`
	whereE := "code=$1 AND cog_millesime=$2"
	return carteMaillee(ctx, pool, whereC, whereE, []any{siren, millesime},
		"Communes membres de "+nomEPCI)
}
