package sitegen

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// VGGroupe : un groupe comparé, avec le nombre de scrutins où il a une position.
type VGGroupe struct {
	Court, Nom, URL string
	Scrutins        string
}

// VGVoisin : le groupe le plus proche d'un groupe, avec son intervalle et sa stabilité.
type VGVoisin struct {
	Groupe, Voisin, R, Intervalle, Accord, N string
	Stabilite                                int // en %
}

// VGEcarte : un groupe laissé hors de l'analyse, avec sa raison.
type VGEcarte struct{ Court, Nom, Raison string }

// VGFusion : une étape de la classification, lisible sans le dessin.
type VGFusion struct{ Distance, Entre string }

// VGAssemblee : tout ce que la page montre d'une assemblée.
type VGAssemblee struct {
	ID, Titre, Source        string
	Scrutins, Jours, Groupes string
	Debut, Fin               string
	Matrice, Arbre           template.HTML
	Groupe                   []VGGroupe
	Voisins                  []VGVoisin
	Fusions                  []VGFusion
	Ecartes                  []VGEcarte
	EcartesTxt               string
	Proche, Loin             string
	ProcheDetail, LoinDetail string
	StablesN, StablesTotal   int
	Intervalle               string
}

// VGCroisement : la comparaison d'une assemblée à l'autre.
type VGCroisement struct {
	Figure   template.HTML
	Familles []VGFamille
	Lignes   []VGLigneCroisement
}
type VGFamille struct{ Nom, AN, PE string }
type VGLigneCroisement struct{ Paire, AN, PE string }

// VGMotions : la bande des motions de censure.
type VGMotions struct {
	Table template.HTML
	N     int
}

// VGExemple : un scrutin réel, décompté groupe par groupe, pour lire un score.
type VGExemple struct {
	Slug, Objet, Date string
	Lignes            []VGExempleLigne
}
type VGExempleLigne struct {
	Groupe                              string
	Pour, Contre, Abstentions, Exprimes int
	Score, Position                     string
}

// VotesGroupes : la page.
type VotesGroupes struct {
	Debut, Fin, Version string
	Assemblees          []*VGAssemblee
	Croisement          *VGCroisement
	Motions             *VGMotions
	Exemple             *VGExemple
	Param               []VGParam
	JSON                string

	an, pe *vgAnalyse // pour le fichier de données
}
type VGParam struct{ Nom, Valeur, Pourquoi string }

// vgFamilles : le rapprochement, fait par ce site, d'un parti avec le groupe où ses élus
// siègent dans chaque assemblée. Ce n'est pas une équivalence : un groupe européen
// réunit souvent plusieurs partis français (Renew : Renaissance, MoDem, Horizons ;
// S&D : PS et Place publique), et l'on compare ici la délégation française. Le choix est
// écrit dans docs/pre-enregistrement-003.md avant le calcul.
var vgFamilles = []VGFamille{
	{"LFI", "la-france-insoumise-nouveau-front-populaire-17", "ep-gue-ngl"},
	{"PS", "socialistes-et-apparentes-17", "ep-sd"},
	{"Écologistes", "ecologiste-et-social-17", "ep-green-efa"},
	{"Renaissance", "ensemble-pour-la-republique-17", "ep-renew"},
	{"LR", "droite-republicaine-17", "ep-epp"},
	{"RN", "rassemblement-national-17", "ep-pfe"},
}

type vgAnalyse struct {
	Base    *vgBase
	Mat     vgMatrices
	Res     *vgResultat
	Arbre   []vgNoeud
	Ordre   []int
	Ecartes []vgEcarte
}

func vgAnalyser(lignes []vgLigne, institution string) *vgAnalyse {
	b, ecartes := vgConstruit(lignes, institution)
	a := &vgAnalyse{Base: b, Ecartes: ecartes}
	if len(b.Groupes) < 2 {
		return a
	}
	a.Mat = vgMatricesDe(b)
	a.Res = vgCalcule(b, a.Mat)
	n := len(b.Groupes)
	a.Arbre = vgClassifie(n, func(i, j int) float64 { return 1 - a.Res.paire(n, i, j).R })
	a.Ordre = vgOrdre(n, a.Arbre, func(i int) string { return b.Groupes[i].Court })
	return a
}

func (a *vgAnalyse) indice(slug string) int {
	for i, g := range a.Base.Groupes {
		if g.Slug == slug {
			return i
		}
	}
	return -1
}

// loadVotesGroupes calcule la page. Nil, sans erreur, quand la base n'a aucun décompte
// de groupe sur la fenêtre (construction partielle, ou périmètre CI sans ces tables).
func loadVotesGroupes(ctx context.Context, pool *pgxpool.Pool, root string) (*VotesGroupes, error) {
	lignes, err := vgChargeLignes(ctx, pool)
	if err != nil {
		return nil, err
	}
	if len(lignes) == 0 {
		return nil, nil
	}
	an := vgAnalyser(lignes, "ASSEMBLEE_NATIONALE")
	pe := vgAnalyser(lignes, "PARLEMENT_EUROPEEN")
	if an.Res == nil || pe.Res == nil {
		return nil, nil
	}
	v := &VotesGroupes{Version: vgVersion, an: an, pe: pe}
	v.Debut = vgDateFr(vgDebut)
	var fin string
	for _, l := range lignes {
		if l.Jour > fin {
			fin = l.Jour
		}
	}
	v.Fin = vgDateFr(fin)
	v.Assemblees = []*VGAssemblee{
		vgPresente(an, "an", "Assemblée nationale", "le groupe sous lequel chaque député a voté, tel que l'Assemblée le publie sur chaque scrutin", root, true),
		vgPresente(pe, "pe", "Parlement européen, délégation française", "le groupe européen de chaque eurodéputé français, tel que le Parlement européen le publie sur chaque scrutin", root, false),
	}
	v.Croisement = vgCroise(an, pe)
	if m, err := vgMotionsCensure(ctx, pool, lignes, an, root); err != nil {
		return nil, err
	} else {
		v.Motions = m
	}
	v.Exemple = vgExemple(ctx, pool, an)
	v.Param = []VGParam{
		{"Début", vgDebut, "ouverture de la 17e législature : les groupes sont constitués"},
		{"Position d'un groupe", fmt.Sprintf("au moins %d exprimés", vgMinVotants), "en dessous, la majorité de deux ou trois élus n'est pas celle d'un groupe"},
		{"Groupe retenu", fmt.Sprintf("au moins %d scrutins avec une position", vgMinScrutins), "moins, aucune paire n'aurait de chiffre fiable"},
		{"Paire retenue", fmt.Sprintf("au moins %d scrutins communs", vgMinPaire), "moins, le coefficient n'est pas calculé"},
		{"Intervalle", fmt.Sprintf("95 %%, %s rééchantillonnages", Count(vgReplicats)), "par jour de séance entier, pour ne pas compter vingt fois un même texte"},
		{"Graine", fmt.Sprint(vgGraine), "fixe : le calcul redonne exactement les mêmes chiffres"},
		{"Version de la méthode", vgVersion, "écrite dans le fichier de données"},
	}
	v.JSON = root + "/qui-decide/votes-ensemble/donnees.json"
	return v, nil
}

func vgPresente(a *vgAnalyse, id, titre, source, root string, lien bool) *VGAssemblee {
	b := a.Base
	n := len(b.Groupes)
	out := &VGAssemblee{ID: id, Titre: titre, Source: source,
		Scrutins: Count(len(b.Scrutins)), Jours: Count(len(b.Jours)), Groupes: Count(n)}
	if len(b.Jours) > 0 {
		out.Debut, out.Fin = vgDateFr(b.Jours[0]), vgDateFr(b.Jours[len(b.Jours)-1])
	}
	labels := make([]string, n)
	for i, g := range b.Groupes {
		labels[i] = g.Court
	}
	scr := make([]int, n)
	for g := range b.Groupes {
		for _, m := range a.Mat.Maj[g] {
			if m >= 0 {
				scr[g]++
			}
		}
	}
	for _, g := range a.Ordre {
		url := ""
		if lien {
			url = root + "/groupe/" + b.Groupes[g].Slug + "/"
		}
		out.Groupe = append(out.Groupe, VGGroupe{Court: b.Groupes[g].Court, Nom: b.Groupes[g].Nom, URL: url, Scrutins: Count(scr[g])})
	}
	out.Matrice = vgMatriceHTML(a)
	out.Arbre = vgArbreSVG(labels, a.Ordre, a.Arbre, fmt.Sprintf("Classification hiérarchique des %d groupes — %s", n, titre))
	// Les fusions, dans l'ordre, pour qui ne voit pas le dessin.
	nom := func(x int) string { return vgNomNoeud(x, n, a.Arbre, labels, a.Ordre) }
	for _, f := range a.Arbre {
		out.Fusions = append(out.Fusions, VGFusion{Distance: Decimal(f.Hauteur, 2), Entre: nom(f.A) + " + " + nom(f.B)})
	}
	// Le voisin le plus proche de chaque groupe, dans l'ordre alphabétique.
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(x, y int) bool { return KeySort(labels[idx[x]]) < KeySort(labels[idx[y]]) })
	for _, i := range idx {
		j := a.Res.Voisin[i]
		if j < 0 {
			continue
		}
		p := a.Res.paire(n, i, j)
		st := int(math.Round(100 * a.Res.Stabilite[i]))
		out.Voisins = append(out.Voisins, VGVoisin{Groupe: labels[i], Voisin: labels[j],
			R: Decimal(p.R, 2), Intervalle: vgIntervalle(p.Bas, p.Haut), Accord: vgPct(p.Acc), N: Count(p.N), Stabilite: st})
		out.StablesTotal++
		if st >= 90 {
			out.StablesN++
		}
	}
	// La paire la plus proche et la plus éloignée : des faits sur la matrice, pas un palmarès.
	var proche, loin *vgPaire
	for k := range a.Res.Paires {
		p := &a.Res.Paires[k]
		if !p.OK {
			continue
		}
		if proche == nil || p.R > proche.R {
			proche = p
		}
		if loin == nil || p.R < loin.R {
			loin = p
		}
	}
	if proche != nil {
		out.Proche = labels[proche.I] + " – " + labels[proche.J]
		out.ProcheDetail = "r = " + Decimal(proche.R, 2) + " " + vgIntervalle(proche.Bas, proche.Haut)
		out.Loin = labels[loin.I] + " – " + labels[loin.J]
		out.LoinDetail = "r = " + Decimal(loin.R, 2) + " " + vgIntervalle(loin.Bas, loin.Haut)
	}
	switch len(a.Ecartes) {
	case 0:
		out.EcartesTxt = "aucun écarté"
	case 1:
		out.EcartesTxt = "1 écarté, avec sa raison (plus bas)"
	default:
		out.EcartesTxt = fmt.Sprintf("%d écartés, avec leur raison (plus bas)", len(a.Ecartes))
	}
	for _, e := range a.Ecartes {
		out.Ecartes = append(out.Ecartes, VGEcarte{Court: e.Court, Nom: e.Nom, Raison: e.Raison + " (" + Count(e.Scrutins) + " scrutins avec une position)"})
	}
	return out
}

func vgIntervalle(bas, haut float64) string {
	if math.IsNaN(bas) || math.IsNaN(haut) {
		return ""
	}
	return "[" + Decimal(bas, 2) + " ; " + Decimal(haut, 2) + "]"
}

func vgPct(x float64) string {
	if math.IsNaN(x) {
		return "—"
	}
	return Count(int(math.Round(100*x))) + " %"
}

// vgNomNoeud : « A + B » d'une branche, les groupes dans l'ordre du dendrogramme.
func vgNomNoeud(x, n int, arbre []vgNoeud, labels []string, ordre []int) string {
	if x < n {
		return labels[x]
	}
	pos := map[int]int{}
	for k, g := range ordre {
		pos[g] = k
	}
	var feuilles []int
	var walk func(int)
	walk = func(y int) {
		if y < n {
			feuilles = append(feuilles, y)
			return
		}
		walk(arbre[y-n].A)
		walk(arbre[y-n].B)
	}
	walk(x)
	sort.Slice(feuilles, func(i, j int) bool { return pos[feuilles[i]] < pos[feuilles[j]] })
	var s []string
	for _, f := range feuilles {
		s = append(s, labels[f])
	}
	return "{" + strings.Join(s, ", ") + "}"
}

// vgMatriceHTML : la matrice des corrélations, en tableau (lisible au clavier, par un lecteur
// d'écran, et sans JavaScript). La teinte ne porte que l'intensité ; le signe et la valeur
// sont écrits dans chaque case.
func vgMatriceHTML(a *vgAnalyse) template.HTML {
	b := a.Base
	n := len(b.Groupes)
	var s strings.Builder
	s.WriteString(`<div class="vg-defile"><table class="vg-matrice"><caption class="sr">Corrélation des votes, groupe par groupe</caption><thead><tr><td></td>`)
	for _, j := range a.Ordre {
		fmt.Fprintf(&s, `<th scope="col"><span>%s</span></th>`, template.HTMLEscapeString(b.Groupes[j].Court))
	}
	s.WriteString(`</tr></thead><tbody>`)
	for _, i := range a.Ordre {
		fmt.Fprintf(&s, `<tr><th scope="row">%s</th>`, template.HTMLEscapeString(b.Groupes[i].Court))
		for _, j := range a.Ordre {
			if i == j {
				s.WriteString(`<td class="diag" aria-label="le groupe avec lui-même">·</td>`)
				continue
			}
			p := a.Res.paire(n, i, j)
			if !p.OK {
				s.WriteString(`<td class="nd" title="moins de 30 scrutins communs">·</td>`)
				continue
			}
			cl := "pos"
			if p.R < 0 {
				cl = "neg"
			}
			titre := fmt.Sprintf("%s × %s : r = %s %s · accord %s · %s scrutins communs",
				b.Groupes[i].Court, b.Groupes[j].Court, Decimal(p.R, 2), vgIntervalle(p.Bas, p.Haut), vgPct(p.Acc), Count(p.N))
			fmt.Fprintf(&s, `<td class="%s" style="--v:%.2f" title="%s">%s</td>`,
				cl, math.Abs(p.R), template.HTMLEscapeString(titre), Decimal(p.R, 2))
		}
		s.WriteString(`</tr>`)
	}
	s.WriteString(`</tbody></table></div>`)
	return template.HTML(s.String())
}

// vgArbreSVG : le dendrogramme, à l'horizontale. La hauteur d'une fusion est la distance
// 1 − r moyenne entre les deux branches. L'ordre des feuilles est une convention de dessin.
func vgArbreSVG(labels []string, ordre []int, arbre []vgNoeud, aria string) template.HTML {
	n := len(labels)
	if n < 2 || len(arbre) == 0 {
		return ""
	}
	const rowH, labW, w, top, bas = 26.0, 124.0, 640.0, 8.0, 40.0
	var hmax float64
	for _, f := range arbre {
		hmax = math.Max(hmax, f.Hauteur)
	}
	ech := math.Max(1, math.Ceil(hmax*2)/2)
	x0 := labW + 12
	xs := func(h float64) float64 { return x0 + h/ech*(w-x0-34) }
	h := top + float64(n)*rowH + bas

	ypos := map[int]float64{}
	xpos := map[int]float64{}
	for k, g := range ordre {
		ypos[g] = top + (float64(k)+0.5)*rowH
		xpos[g] = xs(0)
	}
	var s strings.Builder
	fmt.Fprintf(&s, `<svg class="vg-arbre" viewBox="0 0 %.0f %.0f" role="img" aria-label="%s">`, w, h, template.HTMLEscapeString(aria))
	for k, f := range arbre {
		id := n + k
		ya, yb := ypos[f.A], ypos[f.B]
		x := xs(f.Hauteur)
		fmt.Fprintf(&s, `<path class="br" d="M%.1f %.1fH%.1fV%.1fH%.1f"/>`, xpos[f.A], ya, x, yb, xpos[f.B])
		fmt.Fprintf(&s, `<text class="ht" x="%.1f" y="%.1f">%s</text>`, x+4, (ya+yb)/2-3, Decimal(f.Hauteur, 2))
		ypos[id], xpos[id] = (ya+yb)/2, x
	}
	for _, g := range ordre {
		fmt.Fprintf(&s, `<text class="fe" x="%.1f" y="%.1f">%s</text>`, labW, ypos[g]+4, template.HTMLEscapeString(labels[g]))
	}
	ay := top + float64(n)*rowH + 6
	fmt.Fprintf(&s, `<line class="ax" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, x0, ay, xs(ech), ay)
	for t := 0.0; t <= ech+1e-9; t += 0.5 {
		fmt.Fprintf(&s, `<line class="ax" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/><text class="tk" x="%.1f" y="%.1f">%s</text>`,
			xs(t), ay, xs(t), ay+4, xs(t), ay+16, Decimal(t, 1))
	}
	fmt.Fprintf(&s, `<text class="tk" x="%.1f" y="%.1f">distance (1 − r) : plus c'est court, plus les groupes votent de la même façon</text>`, x0, ay+32)
	s.WriteString(`</svg>`)
	return template.HTML(s.String())
}

// vgCroise : les paires de familles, dans les deux assemblées.
func vgCroise(an, pe *vgAnalyse) *VGCroisement {
	type lig struct {
		nom  string
		a, p vgPaire
	}
	var fam []VGFamille
	idxA, idxP := map[string]int{}, map[string]int{}
	for _, f := range vgFamilles {
		ia, ip := an.indice(f.AN), pe.indice(f.PE)
		if ia < 0 || ip < 0 {
			continue
		}
		fam = append(fam, f)
		idxA[f.Nom], idxP[f.Nom] = ia, ip
	}
	var lignes []lig
	for x := 0; x < len(fam); x++ {
		for y := x + 1; y < len(fam); y++ {
			pa := an.Res.paire(len(an.Base.Groupes), idxA[fam[x].Nom], idxA[fam[y].Nom])
			pp := pe.Res.paire(len(pe.Base.Groupes), idxP[fam[x].Nom], idxP[fam[y].Nom])
			if !pa.OK || !pp.OK {
				continue
			}
			lignes = append(lignes, lig{fam[x].Nom + " – " + fam[y].Nom, pa, pp})
		}
	}
	sort.SliceStable(lignes, func(i, j int) bool { return lignes[i].a.R > lignes[j].a.R })
	c := &VGCroisement{Familles: fam}
	var fig strings.Builder
	const w, labW, rowH, top, bas = 720.0, 190.0, 28.0, 30.0, 14.0
	h := top + float64(len(lignes))*rowH + bas
	x0, x1 := labW+16, w-24
	xs := func(r float64) float64 { return x0 + (r+1)/2*(x1-x0) }
	fmt.Fprintf(&fig, `<svg class="vg-halt" viewBox="0 0 %.0f %.0f" role="img" aria-label="Corrélation des votes de quinze paires de partis, à l'Assemblée nationale et au Parlement européen">`, w, h)
	for _, t := range []float64{-1, -0.5, 0, 0.5, 1} {
		cl := "gr"
		if t == 0 {
			cl = "zero"
		}
		fmt.Fprintf(&fig, `<line class="%s" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/><text class="tk" x="%.1f" y="%.1f">%s</text>`,
			cl, xs(t), top-6, xs(t), h-bas+4, xs(t), top-12, Decimal(t, 1))
	}
	for k, l := range lignes {
		y := top + (float64(k)+0.5)*rowH
		fmt.Fprintf(&fig, `<text class="pa" x="%.1f" y="%.1f">%s</text>`, labW, y+4, template.HTMLEscapeString(l.nom))
		ya, yp := y-5, y+5
		if !math.IsNaN(l.a.Bas) {
			fmt.Fprintf(&fig, `<line class="ic an" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, xs(l.a.Bas), ya, xs(l.a.Haut), ya)
		}
		if !math.IsNaN(l.p.Bas) {
			fmt.Fprintf(&fig, `<line class="ic pe" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, xs(l.p.Bas), yp, xs(l.p.Haut), yp)
		}
		fmt.Fprintf(&fig, `<circle class="pt an" cx="%.1f" cy="%.1f" r="4.5"><title>%s, Assemblée nationale : r = %s %s</title></circle>`,
			xs(l.a.R), ya, template.HTMLEscapeString(l.nom), Decimal(l.a.R, 2), vgIntervalle(l.a.Bas, l.a.Haut))
		fmt.Fprintf(&fig, `<rect class="pt pe" x="%.1f" y="%.1f" width="8" height="8" transform="rotate(45 %.1f %.1f)"><title>%s, Parlement européen : r = %s %s</title></rect>`,
			xs(l.p.R)-4, yp-4, xs(l.p.R), yp, template.HTMLEscapeString(l.nom), Decimal(l.p.R, 2), vgIntervalle(l.p.Bas, l.p.Haut))
		c.Lignes = append(c.Lignes, VGLigneCroisement{Paire: l.nom,
			AN: Decimal(l.a.R, 2) + " " + vgIntervalle(l.a.Bas, l.a.Haut), PE: Decimal(l.p.R, 2) + " " + vgIntervalle(l.p.Bas, l.p.Haut)})
	}
	fig.WriteString(`</svg>`)
	c.Figure = template.HTML(fig.String())
	// Pour la légende : le groupe de chaque assemblée derrière chaque parti.
	for i := range c.Familles {
		c.Familles[i].AN = an.Base.Groupes[idxA[c.Familles[i].Nom]].Court
		c.Familles[i].PE = pe.Base.Groupes[idxP[c.Familles[i].Nom]].Court
	}
	return c
}

var vgReAlinea = regexp.MustCompile(`alinéa\s+(\d)`)

// vgMotionsCensure : les motions de censure de la fenêtre, groupe par groupe. La procédure
// ne fait voter que les soutiens de la motion : un groupe sans bulletin « pour » n'a
// aucune position enregistrée, ce qui n'est ni un « contre » ni une abstention
// (charte graphique : cinquième état).
func vgMotionsCensure(ctx context.Context, pool *pgxpool.Pool, lignes []vgLigne, an *vgAnalyse, root string) (*VGMotions, error) {
	type motion struct {
		id         int64
		slug, jour string
		article    string
	}
	rows, err := pool.Query(ctx, `
		SELECT id, slug, to_char(date_seance, 'YYYY-MM-DD'), objet
		  FROM core.scrutin
		 WHERE institution = 'ASSEMBLEE_NATIONALE' AND type_vote = 'motion de censure'
		   AND date_seance >= $1::date
		 ORDER BY date_seance, id`, vgDebut)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ms []motion
	for rows.Next() {
		var m motion
		var objet string
		if err := rows.Scan(&m.id, &m.slug, &m.jour, &objet); err != nil {
			return nil, err
		}
		m.article = "49.?"
		if g := vgReAlinea.FindStringSubmatch(objet); g != nil {
			m.article = "49." + g[1]
		}
		ms = append(ms, m)
	}
	if err := rows.Err(); err != nil || len(ms) == 0 {
		return nil, err
	}
	pour := map[int64]map[string]int{}
	for _, l := range lignes {
		if l.Institution != "ASSEMBLEE_NATIONALE" || l.TypeVote != "motion de censure" || l.Position != "FOR" {
			continue
		}
		slug := l.Slug
		if f, ok := vgFusionSlugs[slug]; ok {
			slug = f
		}
		if pour[l.Scrutin] == nil {
			pour[l.Scrutin] = map[string]int{}
		}
		pour[l.Scrutin][slug] += l.N
	}
	var s strings.Builder
	s.WriteString(`<div class="vg-defile"><table class="vg-motions"><caption class="sr">Motions de censure, groupe par groupe</caption><thead><tr><td></td>`)
	for _, m := range ms {
		t, _ := vgJourCourt(m.jour)
		fmt.Fprintf(&s, `<th scope="col"><a href="%s/scrutin/%s/" title="%s, article %s"><span>%s</span></a></th>`,
			root, m.slug, vgDateFr(m.jour), m.article, template.HTMLEscapeString(t+" · "+m.article))
	}
	s.WriteString(`</tr></thead><tbody>`)
	idx := make([]int, len(an.Base.Groupes))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(x, y int) bool {
		return KeySort(an.Base.Groupes[idx[x]].Court) < KeySort(an.Base.Groupes[idx[y]].Court)
	})
	for _, g := range idx {
		gr := an.Base.Groupes[g]
		fmt.Fprintf(&s, `<tr><th scope="row"><a href="%s/groupe/%s/">%s</a></th>`, root, gr.Slug, template.HTMLEscapeString(gr.Court))
		for _, m := range ms {
			// Le nombre exact de bulletins « pour », pas un oui/non : trois députés d'un groupe de
			// cinquante votant la motion ne sont pas une position du groupe, et le lecteur doit le voir.
			if n := pour[m.id][gr.Slug]; n > 0 {
				fmt.Fprintf(&s, `<td><span class="vg-sq pour" role="img" aria-label="%s : %d bulletins pour la motion du %s" title="%d bulletins « pour »">%d</span></td>`,
					template.HTMLEscapeString(gr.Court), n, vgDateFr(m.jour), n, n)
			} else {
				fmt.Fprintf(&s, `<td><span class="vg-sq aucune" role="img" aria-label="%s : aucune position enregistrée sur la motion du %s" title="aucune position enregistrée"></span></td>`,
					template.HTMLEscapeString(gr.Court), vgDateFr(m.jour))
			}
		}
		s.WriteString(`</tr>`)
	}
	s.WriteString(`</tbody></table></div>`)
	return &VGMotions{Table: template.HTML(s.String()), N: len(ms)}, nil
}

// vgJourCourt : « 04/12 » et l'année.
func vgJourCourt(iso string) (string, string) {
	if len(iso) != 10 {
		return iso, ""
	}
	return iso[8:10] + "/" + iso[5:7], iso[:4]
}

// vgExemple : le scrutin solennel le plus récent où chaque groupe retenu a une position,
// décompté tel que la source le donne, avec le score qui en découle.
func vgExemple(ctx context.Context, pool *pgxpool.Pool, an *vgAnalyse) *VGExemple {
	b := an.Base
	for s := len(b.Scrutins) - 1; s >= 0; s-- {
		if b.TypeDe[s] != "scrutin public solennel" {
			continue
		}
		ok := true
		for g := range b.Groupes {
			if an.Mat.Maj[g][s] < 0 {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		var slug, objet, jour string
		if err := pool.QueryRow(ctx, `SELECT slug, objet, to_char(date_seance, 'YYYY-MM-DD') FROM core.scrutin WHERE id = $1`, b.Scrutins[s]).Scan(&slug, &objet, &jour); err != nil {
			return nil
		}
		ex := &VGExemple{Slug: slug, Objet: Truncate(objet, 220), Date: vgDateFr(jour)}
		for _, g := range an.Ordre {
			c := b.Cpt[g][s]
			pos := []string{"pour", "contre", "abstention"}[an.Mat.Maj[g][s]]
			ex.Lignes = append(ex.Lignes, VGExempleLigne{Groupe: b.Groupes[g].Court, Pour: c.P, Contre: c.C, Abstentions: c.A,
				Exprimes: c.exprimes(), Score: Decimal(an.Mat.Score[g][s], 2), Position: pos})
		}
		return ex
	}
	return nil
}

// vgDonnees : le fichier publié à côté de la page — paramètres, version de la méthode,
// et chaque chiffre de chaque paire, pour qu'on puisse refaire le calcul ou le contester.
type vgJSONPaire struct {
	A, B                          string
	Scrutins                      int
	R, RBas, RHaut                float64
	Accord, AccordBas, AccordHaut float64
}
type vgJSONAssemblee struct {
	Institution string
	Groupes     []vgGroupeMeta
	Scrutins    int
	Jours       int
	Paires      []vgJSONPaire
}

func vgEcritJSON(out string, an, pe *vgAnalyse) error {
	nul := func(f float64) float64 {
		if math.IsNaN(f) {
			return -2
		}
		return math.Round(f*10000) / 10000
	}
	doc := struct {
		Version, Debut string
		Parametres     map[string]any
		Assemblees     []vgJSONAssemblee
		Note           string
	}{
		Version: vgVersion, Debut: vgDebut,
		Parametres: map[string]any{"votants_min": vgMinVotants, "scrutins_min": vgMinScrutins, "paire_min": vgMinPaire,
			"replicats": vgReplicats, "graine": vgGraine, "bloc": "jour de séance", "intervalle": "95 %, percentiles"},
		Note: "La valeur -2 signifie « non calculé » (moins de 30 scrutins communs).",
	}
	for _, a := range []*vgAnalyse{an, pe} {
		inst := "ASSEMBLEE_NATIONALE"
		if a == pe {
			inst = "PARLEMENT_EUROPEEN"
		}
		ja := vgJSONAssemblee{Institution: inst, Groupes: a.Base.Groupes, Scrutins: len(a.Base.Scrutins), Jours: len(a.Base.Jours)}
		n := len(a.Base.Groupes)
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				p := a.Res.paire(n, i, j)
				ja.Paires = append(ja.Paires, vgJSONPaire{A: a.Base.Groupes[i].Slug, B: a.Base.Groupes[j].Slug, Scrutins: p.N,
					R: nul(p.R), RBas: nul(p.Bas), RHaut: nul(p.Haut), Accord: nul(p.Acc), AccordBas: nul(p.AccBas), AccordHaut: nul(p.AccHaut)})
			}
		}
		doc.Assemblees = append(doc.Assemblees, ja)
	}
	buf, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		return err
	}
	dir := filepath.Join(out, "qui-decide", "votes-ensemble")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "donnees.json"), buf, 0o644)
}
