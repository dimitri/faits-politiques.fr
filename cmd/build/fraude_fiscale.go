package main

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pointControleFiscal struct {
	Annee                float64 // en Md€, converti depuis M€
	Notifie, Encaisse    *float64
	NotifieCalcule       bool
}

type ControleFiscal struct {
	SVG                                template.HTML
	DernierAnnee                       int
	DernierNotifie, DernierEncaisse    float64
	DernierNotifieCalcule              bool
	EcartMoyenPct                      float64
}

// chargerControleFiscal : les résultats du contrôle fiscal, 2015-2024
// (core.controle_fiscal_resultats) — notifié (droits et pénalités mis en
// recouvrement) contre encaissé (effectivement recouvré). Le notifié 2022
// et 2023 est absent de la table (non retrouvé dans une source primaire) :
// la ligne notifiée est tracée en deux segments plutôt que de relier 2021
// à 2024 par une droite qui inventerait deux années de données.
func chargerControleFiscal(ctx context.Context, pool *pgxpool.Pool) (*ControleFiscal, error) {
	rows, err := pool.Query(ctx, `
		SELECT annee, montant_notifie_m, notifie_calcule, montant_encaisse_m
		FROM core.controle_fiscal_resultats ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type ligne struct {
		annee          int
		notifie        *float64
		notifieCalcule bool
		encaisse       float64
	}
	var lignes []ligne
	var sommeNotifie, sommeEncaisse float64
	var nComparable int
	for rows.Next() {
		var l ligne
		var notifieM *float64
		if err := rows.Scan(&l.annee, &notifieM, &l.notifieCalcule, &l.encaisse); err != nil {
			return nil, err
		}
		if notifieM != nil {
			v := *notifieM / 1000
			l.notifie = &v
			sommeNotifie += v
			sommeEncaisse += l.encaisse / 1000
			nComparable++
		}
		lignes = append(lignes, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(lignes) == 0 {
		return nil, nil
	}

	cf := &ControleFiscal{}
	dernier := lignes[len(lignes)-1]
	cf.DernierAnnee = dernier.annee
	cf.DernierEncaisse = dernier.encaisse / 1000
	if dernier.notifie != nil {
		cf.DernierNotifie = *dernier.notifie
		cf.DernierNotifieCalcule = dernier.notifieCalcule
	}
	if nComparable > 0 && sommeNotifie > 0 {
		cf.EcartMoyenPct = (sommeNotifie - sommeEncaisse) / sommeNotifie * 100
	}

	var notifiePts, encaissePts []pointControleFiscal
	for _, l := range lignes {
		e := l.encaisse / 1000
		encaissePts = append(encaissePts, pointControleFiscal{Annee: float64(l.annee), Encaisse: &e})
		if l.notifie != nil {
			notifiePts = append(notifiePts, pointControleFiscal{Annee: float64(l.annee), Notifie: l.notifie, NotifieCalcule: l.notifieCalcule})
		}
	}
	cf.SVG = dessinerControleFiscal(notifiePts, encaissePts, lignes[0].annee, dernier.annee)
	return cf, nil
}

func dessinerControleFiscal(notifiePts, encaissePts []pointControleFiscal, anDebut, anFin int) template.HTML {
	const w, h, ml, mr, mt, mb = 720.0, 260.0, 34.0, 62.0, 14.0, 26.0
	maxVal := 0.0
	for _, p := range encaissePts {
		if p.Encaisse != nil && *p.Encaisse > maxVal {
			maxVal = *p.Encaisse
		}
	}
	for _, p := range notifiePts {
		if p.Notifie != nil && *p.Notifie > maxVal {
			maxVal = *p.Notifie
		}
	}
	maxVal *= 1.15
	x := func(annee float64) float64 { return ml + (w-ml-mr)*(annee-float64(anDebut))/float64(anFin-anDebut) }
	y := func(v float64) float64 { return mt + (h-mt-mb)*(1-v/maxVal) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="courbe controle-fiscal" viewBox="0 0 %.0f %.0f" role="img" `+
		`aria-label="Résultats du contrôle fiscal, notifié et encaissé, %d à %d">`, w, h, anDebut, anFin)
	for _, palier := range []float64{0, 5, 10, 15} {
		if palier > maxVal {
			continue
		}
		fmt.Fprintf(&b, `<line class="grille" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, ml, y(palier), w-mr, y(palier))
		fmt.Fprintf(&b, `<text class="et" x="%.1f" y="%.1f">%d Md€</text>`, ml-6, y(palier)+3, int(palier))
	}

	// Le notifié se dessine en segments contigus séparés — jamais une droite
	// reliant deux années dont la valeur intermédiaire est inconnue.
	traceSegments(&b, notifiePts, "notifie", func(p pointControleFiscal) *float64 { return p.Notifie }, x, y)
	traceSegments(&b, encaissePts, "encaisse", func(p pointControleFiscal) *float64 { return p.Encaisse }, x, y)

	if len(notifiePts) > 0 {
		dernier := notifiePts[len(notifiePts)-1]
		cl := "pt-notifie"
		if dernier.NotifieCalcule {
			cl += " calcule"
		}
		fmt.Fprintf(&b, `<circle class="%s" cx="%.1f" cy="%.1f" r="2.6"/>`, cl, x(dernier.Annee), y(*dernier.Notifie))
		fmt.Fprintf(&b, `<text class="lbl-notifie" x="%.1f" y="%.1f">Notifié</text>`, x(dernier.Annee)+5, y(*dernier.Notifie)+3)
	}
	if len(encaissePts) > 0 {
		dernier := encaissePts[len(encaissePts)-1]
		fmt.Fprintf(&b, `<circle class="pt-encaisse" cx="%.1f" cy="%.1f" r="2.6"/>`, x(dernier.Annee), y(*dernier.Encaisse))
		fmt.Fprintf(&b, `<text class="lbl-encaisse" x="%.1f" y="%.1f">Encaissé</text>`, x(dernier.Annee)+5, y(*dernier.Encaisse)+3)
	}

	fmt.Fprintf(&b, `<text class="an" x="%.1f" y="%.1f">%d</text>`, ml, h-8, anDebut)
	fmt.Fprintf(&b, `<text class="an fin" x="%.1f" y="%.1f">%d</text>`, w-mr, h-8, anFin)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// traceSegments : dessine une ligne par groupe d'années consécutives (pas
// de saut) — une année manquante (ex. 2022-2023 pour le notifié) coupe le
// tracé plutôt que d'être comblée par une interpolation visuelle.
func traceSegments(b *strings.Builder, pts []pointControleFiscal, classe string,
	valeur func(pointControleFiscal) *float64, x func(float64) float64, y func(float64) float64) {
	var segment []pointControleFiscal
	flush := func() {
		if len(segment) < 2 {
			segment = nil
			return
		}
		var coords []string
		for _, p := range segment {
			coords = append(coords, fmt.Sprintf("%.2f,%.2f", x(p.Annee), y(*valeur(p))))
		}
		fmt.Fprintf(b, `<polyline class="ligne-%s" points="%s"/>`, classe, strings.Join(coords, " "))
		segment = nil
	}
	anneePrec := 0.0
	for i, p := range pts {
		if i > 0 && p.Annee-anneePrec > 1 {
			flush()
		}
		segment = append(segment, p)
		anneePrec = p.Annee
	}
	flush()
}
