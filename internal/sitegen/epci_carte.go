package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// La carte des intercommunalités à fiscalité propre — communautés de
// communes, d'agglomération, urbaines, métropoles — devenue possible par
// l'arrivée de geo.contour_cog (IGN Admin Express COG CARTO), qui donne à
// chaque EPCI son propre contour, millésimé comme le répertoire des
// intercommunalités lui-même. Avant cette table, seule l'APPARTENANCE d'une
// commune à un groupement était connue (core.epci_membre) ; aucun tracé
// n'existait pour en dessiner la carte — voir le constat qui accompagnait
// encore la page il y a peu.
//
// La géométrie sert à dessiner, jamais à mesurer : les superficies publiées
// restent celles de l'IGN, jamais un calcul refait depuis le tracé simplifié.
//
// Cette carte réutilise le même moteur de choroplèthe que les régions et les
// départements (Carte, preparer, pleine, apercu) : seule la façon de charger
// les tracés change, parce que la source est une autre table, avec une autre
// clé (le SIREN, pas le code INSEE) et un autre millésime.
func setOutlinesEPCI(ctx context.Context, pool *pgxpool.Pool, vintage int,
	tolerance float64) (*SetOutlines, error) {

	rows, err := pool.Query(ctx, `
		SELECT code, nom,
		       st_assvg(st_transform(st_simplifypreservetopology(geom, $1), 2154), 1, 0)
		FROM geo.contour_cog
		WHERE niveau='EPCI' AND cog_millesime=$2 AND srid_rendu=2154
		ORDER BY code`, tolerance, vintage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	traces, noms := map[string]string{}, map[string]string{}
	var codes []string
	for rows.Next() {
		var c, n, p string
		if err := rows.Scan(&c, &n, &p); err != nil {
			return nil, err
		}
		traces[c], noms[c] = p, n
		codes = append(codes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// La boîte englobante couvre la MÉTROPOLE ENTIÈRE (via geo.contour, comme
	// pour les départements) plutôt que la seule emprise des EPCI : les
	// syndicats et zones sans EPCI à fiscalité propre restent dans le cadre,
	// au lieu de faire flotter la carte sur un territoire aux bords faux.
	//
	// st_extent est une agrégation : la ligne existe même sans contour
	// départemental, avec une valeur NULL. geo.contour n'a AUCUNE source dans
	// le catalogue (contours OpenStreetMap de la migration 0058, chargés hors
	// pipeline) alors que geo.contour_cog, elle, est remplie par « contours » /
	// « geo-courant » : après un « fpctl ingest default » ordinaire, cette requête
	// porte donc normalement sur zéro ligne pendant que les tracés d'EPCI
	// ci-dessus existent. Même défense que jeuContours
	// (internal/sitegen/carte.go) sur cette même table : une boîte vide plutôt
	// qu'un échec de toute la construction.
	var vbN sql.NullString
	err = pool.QueryRow(ctx, `
		SELECT round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM (SELECT st_extent(st_transform(geom,2154)) e FROM geo.contour
		      WHERE niveau='DEPARTEMENT' AND srid_rendu = 2154) x`).Scan(&vbN)
	if err != nil {
		return nil, err
	}
	vb := vbN.String

	om, err := outlinesOverseasCOG(ctx, pool, vintage, tolerance)
	if err != nil {
		return nil, err
	}
	for _, o := range om {
		noms[o.Code] = o.Name
	}

	var b strings.Builder
	for _, c := range codes {
		fmt.Fprintf(&b, `<path id="e%s" d="%s"/>`, c, traces[c])
	}
	return &SetOutlines{
		Defs: template.HTML(`<svg width="0" height="0" aria-hidden="true" ` +
			`style="position:absolute"><defs>` + b.String() + `</defs></svg>`),
		ViewBox: vb, Level: "EPCI", Codes: codes, Noms: noms, traces: traces,
		overseas: om,
	}, nil
}

// outlinesOverseasCOG : les EPCI d'outre-mer, chacun dans sa propre
// projection légale — même principe que contoursOutreMer pour geo.contour.
func outlinesOverseasCOG(ctx context.Context, pool *pgxpool.Pool, vintage int,
	tolerance float64) ([]outlineOnly, error) {

	rows, err := pool.Query(ctx, `
		SELECT code, nom, srid_rendu,
		       st_assvg(st_transform(st_simplifypreservetopology(geom,$1), srid_rendu), 1, 0),
		       round(st_xmin(e))||' '||round(-st_ymax(e))||' '||
		       round(st_xmax(e)-st_xmin(e))||' '||round(st_ymax(e)-st_ymin(e))
		FROM geo.contour_cog,
		     LATERAL (SELECT st_envelope(st_transform(geom, srid_rendu)) e) x
		WHERE niveau='EPCI' AND cog_millesime=$2 AND srid_rendu <> 2154
		ORDER BY code`, tolerance, vintage)
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
