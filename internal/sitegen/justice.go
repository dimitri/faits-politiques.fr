package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type etablissementSurpeuple struct {
	Etablissement, Quartier, Direction string
	Densite                            float64
	Detenus, Capacite                  int
}

type StatsJustice struct {
	TopSurpeuplementSVG template.HTML
	NbLignes            int
	NbAnomalies         int
	TotalDetenus        int
	TotalCapacite       int
}

// chargerJustice : les vingt quartiers d'établissement les plus densément
// peuplés (densité carcérale = détenus / capacité opérationnelle, calculée
// par la source elle-même). Une ligne à densité infinie (capacité
// officielle nulle mais des détenus réels — un artefact de la source, pas
// une erreur de lecture) est exclue du classement et comptée à part.
func chargerJustice(ctx context.Context, pool *pgxpool.Pool) (*StatsJustice, error) {
	st := &StatsJustice{}
	// sum(...) est une agrégation : la ligne existe même sans établissement
	// encore ingéré, avec des sommes NULL — count(*) reste, lui, toujours 0
	// dans ce cas, d'où le garde-fou qui suit.
	var totalDetenus, totalCapacite sql.NullInt64
	if err := pool.QueryRow(ctx, `
		SELECT count(*), sum(ecroues_detenus), sum(capacite_operationnelle)
		FROM core.etablissement_penitentiaire`).
		Scan(&st.NbLignes, &totalDetenus, &totalCapacite); err != nil {
		return nil, err
	}
	if st.NbLignes == 0 {
		return nil, nil
	}
	st.TotalDetenus, st.TotalCapacite = int(totalDetenus.Int64), int(totalCapacite.Int64)
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM core.etablissement_penitentiaire
		WHERE capacite_operationnelle = 0 AND ecroues_detenus > 0`).Scan(&st.NbAnomalies); err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT etablissement, quartier, direction_interregionale, densite_pct, ecroues_detenus, capacite_operationnelle
		FROM core.etablissement_penitentiaire
		WHERE densite_pct IS NOT NULL AND capacite_operationnelle > 0
		ORDER BY densite_pct DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var top []etablissementSurpeuple
	for rows.Next() {
		var e etablissementSurpeuple
		if err := rows.Scan(&e.Etablissement, &e.Quartier, &e.Direction, &e.Densite, &e.Detenus, &e.Capacite); err != nil {
			return nil, err
		}
		top = append(top, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	st.TopSurpeuplementSVG = dessinerTopSurpeuplement(top)
	return st, nil
}

func dessinerTopSurpeuplement(top []etablissementSurpeuple) template.HTML {
	if len(top) == 0 {
		return ""
	}
	const w, mr, ml, largeurBarre, gap = 720.0, 70.0, 260.0, 15.0, 6.0
	h := float64(len(top))*(largeurBarre+gap) + gap
	largeurAxe := w - ml - mr
	max := top[0].Densite

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="barres-surpeuplement" role="img" `+
		`aria-label="Vingt quartiers d'établissement les plus densément peuplés">`, w, h)
	fmt.Fprintf(&b, `<line class="seuil" x1="%.1f" y1="0" x2="%.1f" y2="%.1f"/>`,
		ml+largeurAxe*100/max, ml+largeurAxe*100/max, h)
	for i, e := range top {
		y := gap + float64(i)*(largeurBarre+gap)
		largeur := largeurAxe * e.Densite / max
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s (%s)</text>`,
			ml-8, y+largeurBarre/2+3, template.HTMLEscapeString(e.Etablissement), template.HTMLEscapeString(e.Quartier))
		fmt.Fprintf(&b, `<rect class="barre" x="%.1f" y="%.1f" width="%.1f" height="%.0f">`+
			`<title>%s, %s (%s) : %d détenus pour %d places, %s %%</title></rect>`,
			ml, y, largeur, largeurBarre,
			template.HTMLEscapeString(e.Etablissement), template.HTMLEscapeString(e.Quartier), template.HTMLEscapeString(e.Direction),
			e.Detenus, e.Capacite, Decimal(e.Densite, 0))
		fmt.Fprintf(&b, `<text class="val" x="%.1f" y="%.1f">%s %%</text>`,
			ml+largeur+6, y+largeurBarre/2+3, Decimal(e.Densite, 0))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
