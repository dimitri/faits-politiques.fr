package sitegen

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type StatsUnionEuropean struct {
	PIBSVG         template.HTML
	YearGdp        int
	SectorsSVG     template.HTML
	YearSectors    int
	TradeTable     template.HTML
	SectorsUSTable template.HTML
}

// loadUnionEuropean : le PIB, le commerce extérieur et la structure
// sectorielle de l'UE comparés aux États-Unis et à la Chine (Banque
// mondiale/Eurostat, déjà chargés dans core.indicateur_mondial — voir
// internal/international/pib_epargne_nette.go et commerce_extra_eu.go).
func loadUnionEuropean(ctx context.Context, pool *pgxpool.Pool) (*StatsUnionEuropean, error) {
	st := &StatsUnionEuropean{}

	// --- PIB : les trois blocs ont 2025, un seul millésime, une seule devise.
	rows, err := pool.Query(ctx, `
		SELECT pays_code, pays_label, valeur FROM core.indicateur_mondial
		WHERE indicateur='NY.GDP.MKTP.CD' AND pays_code IN ('EU','US','CN') AND annee=2025
		ORDER BY valeur DESC`)
	if err != nil {
		return nil, err
	}
	type gdp struct {
		Code, Label string
		Value       float64
	}
	var gdps []gdp
	for rows.Next() {
		var p gdp
		if err := rows.Scan(&p.Code, &p.Label, &p.Value); err != nil {
			rows.Close()
			return nil, err
		}
		gdps = append(gdps, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(gdps) == 3 {
		st.YearGdp = 2025
		max := gdps[0].Value
		var b strings.Builder
		b.WriteString(`<div class="barres">`)
		for _, p := range gdps {
			fmt.Fprintf(&b, `<div class="ligne"><span class="n">%s</span>`+
				`<span class="piste"><i style="width:%.1f%%"></i></span>`+
				`<span class="v">%s Md$</span></div>`,
				template.HTMLEscapeString(nameBlock(p.Code)), 100*p.Value/max, Count(int(p.Value/1e9)))
		}
		b.WriteString(`</div>`)
		st.PIBSVG = template.HTML(b.String())
	}

	// --- Structure sectorielle : EU et Chine ont 2025, comparés directement ;
	// les États-Unis (2021, dernière année publiée par la Banque mondiale
	// pour cet indicateur) sont dans un tableau séparé, jamais mélangés dans
	// le même graphique à une année qu'ils n'ont pas.
	sectors, yearSection, err := loadSectorsForBlocs(ctx, pool, []string{"EU", "CN"}, 2025)
	if err != nil {
		return nil, err
	}
	if len(sectors) > 0 {
		st.YearSectors = yearSection
		st.SectorsSVG = drawSectorsBlock(sectors)
	}
	sectorsUS, _, err := loadSectorsForBlocs(ctx, pool, []string{"US"}, 0)
	if err != nil {
		return nil, err
	}
	if len(sectorsUS) > 0 {
		var t strings.Builder
		t.WriteString(`<div class="scroll"><table><thead><tr><th>Bloc</th><th>Année</th>` +
			`<th>Primaire</th><th>Secondaire</th><th>Tertiaire</th></tr></thead><tbody>`)
		for _, s := range sectorsUS {
			fmt.Fprintf(&t, `<tr><td>%s</td><td>%d</td><td>%s %%</td><td>%s %%</td><td>%s %%</td></tr>`,
				template.HTMLEscapeString(nameBlock(s.Code)), s.Year,
				Decimal(s.Primary, 1), Decimal(s.Secondary, 1), Decimal(s.Tertiary, 1))
		}
		t.WriteString(`</tbody></table></div>`)
		st.SectorsUSTable = template.HTML(t.String())
	}

	// --- Commerce : EU (extra-UE, biens, Eurostat, EUR) vs US et Chine
	// (biens+services, Banque mondiale, USD) — deux sources, deux devises,
	// deux champs : un tableau explicite ligne par ligne, jamais un même
	// graphique qui laisserait croire à une conversion faite en silence.
	type lineTrade struct {
		Block, Currency, Field string
		Year                   int
		Export, Import         float64
	}
	var lines []lineTrade
	for _, e := range []struct {
		code, currency, field, indExp, indImp string
		divisor                               float64
	}{
		// EU_EXTRA_EXPORT/IMPORT_MEUR sont déjà en MILLIONS d'euros (voir
		// internal/international/commerce_extra_eu.go) : diviser par 1e3,
		// pas 1e6, pour obtenir des milliards — une division par 1e6 y
		// aurait lu des milliers de milliards (« 3 Md€ » au lieu de
		// « 2 646 Md€ »), repéré à l'écran avant publication.
		{"EU", "Md€", "biens seulement, hors commerce intra-UE", "EU_EXTRA_EXPORT_MEUR", "EU_EXTRA_IMPORT_MEUR", 1e3},
		{"US", "Md$", "biens et services", "NE.EXP.GNFS.CD", "NE.IMP.GNFS.CD", 1e9},
		{"CN", "Md$", "biens et services", "NE.EXP.GNFS.CD", "NE.IMP.GNFS.CD", 1e9},
	} {
		var year int
		var exp, imp float64
		err := pool.QueryRow(ctx, `
			SELECT e.annee, e.valeur, i.valeur FROM core.indicateur_mondial e
			JOIN core.indicateur_mondial i ON i.pays_code=e.pays_code AND i.annee=e.annee AND i.indicateur=$3
			WHERE e.pays_code=$1 AND e.indicateur=$2
			ORDER BY e.annee DESC LIMIT 1`, e.code, e.indExp, e.indImp).Scan(&year, &exp, &imp)
		if err != nil {
			continue
		}
		lines = append(lines, lineTrade{nameBlock(e.code), e.currency, e.field, year, exp / e.divisor, imp / e.divisor})
	}
	if len(lines) > 0 {
		var t strings.Builder
		t.WriteString(`<div class="scroll"><table><thead><tr><th>Bloc</th><th>Année</th>` +
			`<th>Exportations</th><th>Importations</th><th>Champ</th></tr></thead><tbody>`)
		for _, l := range lines {
			fmt.Fprintf(&t, `<tr><td>%s</td><td>%d</td><td>%s %s</td><td>%s %s</td><td class="small muted">%s</td></tr>`,
				template.HTMLEscapeString(l.Block), l.Year, Decimal(l.Export, 0), l.Currency,
				Decimal(l.Import, 0), l.Currency, template.HTMLEscapeString(l.Field))
		}
		t.WriteString(`</tbody></table></div>`)
		st.TradeTable = template.HTML(t.String())
	}

	return st, nil
}

func nameBlock(code string) string {
	switch code {
	case "EU":
		return "Union européenne"
	case "US":
		return "États-Unis"
	case "CN":
		return "Chine"
	}
	return code
}

type pointSectorBlock struct {
	Code                         string
	Year                         int
	Primary, Secondary, Tertiary float64
}

// loadSectorsForBlocs : pour chaque code, la valeur ajoutée par secteur (%) à
// l'année demandée (0 = dernière année disponible pour ce code).
func loadSectorsForBlocs(ctx context.Context, pool *pgxpool.Pool, codes []string, year int) ([]pointSectorBlock, int, error) {
	var out []pointSectorBlock
	yearUsed := 0
	for _, code := range codes {
		q := `SELECT annee,
		        max(valeur) FILTER (WHERE indicateur='NV.AGR.TOTL.ZS'),
		        max(valeur) FILTER (WHERE indicateur='NV.IND.TOTL.ZS'),
		        max(valeur) FILTER (WHERE indicateur='NV.SRV.TOTL.ZS')
		      FROM core.indicateur_mondial
		      WHERE pays_code=$1 AND indicateur IN ('NV.AGR.TOTL.ZS','NV.IND.TOTL.ZS','NV.SRV.TOTL.ZS')`
		var args []any
		args = append(args, code)
		if year > 0 {
			q += ` AND annee=$2 GROUP BY annee`
			args = append(args, year)
		} else {
			q += ` GROUP BY annee ORDER BY annee DESC LIMIT 1`
		}
		var p pointSectorBlock
		p.Code = code
		if err := pool.QueryRow(ctx, q, args...).Scan(&p.Year, &p.Primary, &p.Secondary, &p.Tertiary); err != nil {
			continue
		}
		out = append(out, p)
		yearUsed = p.Year
	}
	return out, yearUsed, nil
}

// drawSectorsBlock : une barre empilée par bloc (primaire, secondaire,
// tertiaire), même échelle 0-100 %.
func drawSectorsBlock(pts []pointSectorBlock) template.HTML {
	if len(pts) == 0 {
		return ""
	}
	const w, mr, ml, widthBar, gap = 720.0, 90.0, 130.0, 40.0, 26.0
	h := float64(len(pts))*(widthBar+gap) + gap
	widthAxis := w - ml - mr
	x := func(pct float64) float64 { return ml + widthAxis*pct/100 }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="secteurs-bloc" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Valeur ajoutée par secteur, en %% du PIB">`, w, h)
	for i, p := range pts {
		y := gap + float64(i)*(widthBar+gap)
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s</text>`, ml-10, y+widthBar/2+4, template.HTMLEscapeString(nameBlock(p.Code)))
		xx := ml
		seg := func(cl string, v float64, label string) {
			width := widthAxis * v / 100
			fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.0f">`+
				`<title>%s, %s : %s %%</title></rect>`,
				cl, xx, y, width, widthBar, template.HTMLEscapeString(nameBlock(p.Code)), label, Decimal(v, 1))
			if width > 28 {
				fmt.Fprintf(&b, `<text class="et-seg" x="%.1f" y="%.1f">%s %%</text>`, xx+width/2, y+widthBar/2+4, Decimal(v, 0))
			}
			xx += width
		}
		seg("primaire", p.Primary, "primaire")
		seg("secondaire", p.Secondary, "secondaire")
		seg("tertiaire", p.Tertiary, "tertiaire")
	}
	for _, pct := range []float64{0, 25, 50, 75, 100} {
		fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%d%%</text>`, x(pct), h-4, int(pct))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
