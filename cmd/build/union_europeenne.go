package main

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type StatsUnionEuropeenne struct {
	PIBSVG          template.HTML
	AnneePIB        int
	SecteursSVG     template.HTML
	AnneeSecteurs   int
	CommerceTable   template.HTML
	SecteursUSTable template.HTML
}

// chargerUnionEuropeenne : le PIB, le commerce extérieur et la structure
// sectorielle de l'UE comparés aux États-Unis et à la Chine (Banque
// mondiale/Eurostat, déjà chargés dans core.indicateur_mondial — voir
// internal/international/pib_epargne_nette.go et commerce_extra_eu.go).
func chargerUnionEuropeenne(ctx context.Context, pool *pgxpool.Pool) (*StatsUnionEuropeenne, error) {
	st := &StatsUnionEuropeenne{}

	// --- PIB : les trois blocs ont 2025, un seul millésime, une seule devise.
	rows, err := pool.Query(ctx, `
		SELECT pays_code, pays_label, valeur FROM core.indicateur_mondial
		WHERE indicateur='NY.GDP.MKTP.CD' AND pays_code IN ('EU','US','CN') AND annee=2025
		ORDER BY valeur DESC`)
	if err != nil {
		return nil, err
	}
	type pib struct {
		Code, Label string
		Valeur      float64
	}
	var pibs []pib
	for rows.Next() {
		var p pib
		if err := rows.Scan(&p.Code, &p.Label, &p.Valeur); err != nil {
			rows.Close()
			return nil, err
		}
		pibs = append(pibs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pibs) == 3 {
		st.AnneePIB = 2025
		max := pibs[0].Valeur
		var b strings.Builder
		b.WriteString(`<div class="barres">`)
		for _, p := range pibs {
			fmt.Fprintf(&b, `<div class="ligne"><span class="n">%s</span>`+
				`<span class="piste"><i style="width:%.1f%%"></i></span>`+
				`<span class="v">%s Md$</span></div>`,
				template.HTMLEscapeString(nomBloc(p.Code)), 100*p.Valeur/max, Nombre(int(p.Valeur/1e9)))
		}
		b.WriteString(`</div>`)
		st.PIBSVG = template.HTML(b.String())
	}

	// --- Structure sectorielle : EU et Chine ont 2025, comparés directement ;
	// les États-Unis (2021, dernière année publiée par la Banque mondiale
	// pour cet indicateur) sont dans un tableau séparé, jamais mélangés dans
	// le même graphique à une année qu'ils n'ont pas.
	secteurs, anneeSect, err := chargerSecteurs(ctx, pool, []string{"EU", "CN"}, 2025)
	if err != nil {
		return nil, err
	}
	if len(secteurs) > 0 {
		st.AnneeSecteurs = anneeSect
		st.SecteursSVG = dessinerSecteursBloc(secteurs)
	}
	secteursUS, _, err := chargerSecteurs(ctx, pool, []string{"US"}, 0)
	if err != nil {
		return nil, err
	}
	if len(secteursUS) > 0 {
		var t strings.Builder
		t.WriteString(`<div class="scroll"><table><thead><tr><th>Bloc</th><th>Année</th>` +
			`<th>Primaire</th><th>Secondaire</th><th>Tertiaire</th></tr></thead><tbody>`)
		for _, s := range secteursUS {
			fmt.Fprintf(&t, `<tr><td>%s</td><td>%d</td><td>%s %%</td><td>%s %%</td><td>%s %%</td></tr>`,
				template.HTMLEscapeString(nomBloc(s.Code)), s.Annee,
				Decimal(s.Primaire, 1), Decimal(s.Secondaire, 1), Decimal(s.Tertiaire, 1))
		}
		t.WriteString(`</tbody></table></div>`)
		st.SecteursUSTable = template.HTML(t.String())
	}

	// --- Commerce : EU (extra-UE, biens, Eurostat, EUR) vs US et Chine
	// (biens+services, Banque mondiale, USD) — deux sources, deux devises,
	// deux champs : un tableau explicite ligne par ligne, jamais un même
	// graphique qui laisserait croire à une conversion faite en silence.
	type ligneCommerce struct {
		Bloc, Devise, Champ string
		Annee               int
		Export, Import      float64
	}
	var lignes []ligneCommerce
	for _, e := range []struct {
		code, devise, champ, indExp, indImp string
		diviseur                            float64
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
		var annee int
		var exp, imp float64
		err := pool.QueryRow(ctx, `
			SELECT e.annee, e.valeur, i.valeur FROM core.indicateur_mondial e
			JOIN core.indicateur_mondial i ON i.pays_code=e.pays_code AND i.annee=e.annee AND i.indicateur=$3
			WHERE e.pays_code=$1 AND e.indicateur=$2
			ORDER BY e.annee DESC LIMIT 1`, e.code, e.indExp, e.indImp).Scan(&annee, &exp, &imp)
		if err != nil {
			continue
		}
		lignes = append(lignes, ligneCommerce{nomBloc(e.code), e.devise, e.champ, annee, exp / e.diviseur, imp / e.diviseur})
	}
	if len(lignes) > 0 {
		var t strings.Builder
		t.WriteString(`<div class="scroll"><table><thead><tr><th>Bloc</th><th>Année</th>` +
			`<th>Exportations</th><th>Importations</th><th>Champ</th></tr></thead><tbody>`)
		for _, l := range lignes {
			fmt.Fprintf(&t, `<tr><td>%s</td><td>%d</td><td>%s %s</td><td>%s %s</td><td class="small muted">%s</td></tr>`,
				template.HTMLEscapeString(l.Bloc), l.Annee, Decimal(l.Export, 0), l.Devise,
				Decimal(l.Import, 0), l.Devise, template.HTMLEscapeString(l.Champ))
		}
		t.WriteString(`</tbody></table></div>`)
		st.CommerceTable = template.HTML(t.String())
	}

	return st, nil
}

func nomBloc(code string) string {
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

type pointSecteurBloc struct {
	Code                            string
	Annee                           int
	Primaire, Secondaire, Tertiaire float64
}

// chargerSecteurs : pour chaque code, la valeur ajoutée par secteur (%) à
// l'année demandée (0 = dernière année disponible pour ce code).
func chargerSecteurs(ctx context.Context, pool *pgxpool.Pool, codes []string, annee int) ([]pointSecteurBloc, int, error) {
	var out []pointSecteurBloc
	anneeUtilisee := 0
	for _, code := range codes {
		q := `SELECT annee,
		        max(valeur) FILTER (WHERE indicateur='NV.AGR.TOTL.ZS'),
		        max(valeur) FILTER (WHERE indicateur='NV.IND.TOTL.ZS'),
		        max(valeur) FILTER (WHERE indicateur='NV.SRV.TOTL.ZS')
		      FROM core.indicateur_mondial
		      WHERE pays_code=$1 AND indicateur IN ('NV.AGR.TOTL.ZS','NV.IND.TOTL.ZS','NV.SRV.TOTL.ZS')`
		var args []any
		args = append(args, code)
		if annee > 0 {
			q += ` AND annee=$2 GROUP BY annee`
			args = append(args, annee)
		} else {
			q += ` GROUP BY annee ORDER BY annee DESC LIMIT 1`
		}
		var p pointSecteurBloc
		p.Code = code
		if err := pool.QueryRow(ctx, q, args...).Scan(&p.Annee, &p.Primaire, &p.Secondaire, &p.Tertiaire); err != nil {
			continue
		}
		out = append(out, p)
		anneeUtilisee = p.Annee
	}
	return out, anneeUtilisee, nil
}

// dessinerSecteursBloc : une barre empilée par bloc (primaire, secondaire,
// tertiaire), même échelle 0-100 %.
func dessinerSecteursBloc(pts []pointSecteurBloc) template.HTML {
	if len(pts) == 0 {
		return ""
	}
	const w, mr, ml, largeurBarre, gap = 720.0, 90.0, 130.0, 40.0, 26.0
	h := float64(len(pts))*(largeurBarre+gap) + gap
	largeurAxe := w - ml - mr
	x := func(pct float64) float64 { return ml + largeurAxe*pct/100 }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="secteurs-bloc" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Valeur ajoutée par secteur, en %% du PIB">`, w, h)
	for i, p := range pts {
		y := gap + float64(i)*(largeurBarre+gap)
		fmt.Fprintf(&b, `<text class="cat" x="%.1f" y="%.1f">%s</text>`, ml-10, y+largeurBarre/2+4, template.HTMLEscapeString(nomBloc(p.Code)))
		xx := ml
		seg := func(cl string, v float64, libelle string) {
			largeur := largeurAxe * v / 100
			fmt.Fprintf(&b, `<rect class="%s" x="%.1f" y="%.1f" width="%.1f" height="%.0f">`+
				`<title>%s, %s : %s %%</title></rect>`,
				cl, xx, y, largeur, largeurBarre, template.HTMLEscapeString(nomBloc(p.Code)), libelle, Decimal(v, 1))
			if largeur > 28 {
				fmt.Fprintf(&b, `<text class="et-seg" x="%.1f" y="%.1f">%s %%</text>`, xx+largeur/2, y+largeurBarre/2+4, Decimal(v, 0))
			}
			xx += largeur
		}
		seg("primaire", p.Primaire, "primaire")
		seg("secondaire", p.Secondaire, "secondaire")
		seg("tertiaire", p.Tertiaire, "tertiaire")
	}
	for _, pct := range []float64{0, 25, 50, 75, 100} {
		fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%d%%</text>`, x(pct), h-4, int(pct))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
