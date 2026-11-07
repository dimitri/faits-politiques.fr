package sitegen

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Comment les groupes votent ensemble : la proximité des votes de groupe à groupe,
// à l'Assemblée nationale et au Parlement européen (eurodéputés français).
//
// Ce que la page mesure : à quel point deux groupes votent dans le même sens, sur
// les scrutins où les deux ont voté. Elle ne mesure ni une idéologie, ni un
// accord de fond, ni une alliance — un même vote peut venir d'une consigne, d'une
// coalition, d'un calendrier ou d'un amendement déposé par l'un et soutenu par
// l'autre. Aucun ordre gauche-droite n'y est produit (docs/charte-graphique.md :
// pas d'hémicycle, pas d'axe) : les groupes sont rangés par le dendrogramme, dont
// l'ordre des feuilles ne veut rien dire.
//
// Les groupes sont ceux de la source, transcrits sur chaque bulletin (D-014) :
// mv.scrutin_groupe_vote. Le Sénat n'en est pas : ses bulletins ne portent aucun
// groupe, et ce site ne le reconstitue pas depuis les appartenances.
//
// Le protocole est docs/pre-enregistrement-003.md ; tout paramètre qui change
// un chiffre est dans les constantes ci-dessous, et dans le JSON publié.
const (
	vgDebut       = "2024-07-18" // ouverture de la 17e législature (groupes constitués)
	vgMinVotants  = 3            // exprimés (pour + contre + abstentions) pour qu'un groupe ait une position
	vgMinScrutins = 100          // scrutins où un groupe a une position pour entrer dans l'analyse
	vgMinPaire    = 30           // scrutins communs pour qu'une paire ait un chiffre
	vgReplicats   = 1000         // rééchantillonnages pour l'intervalle de confiance
	vgGraine      = 20240718     // graine fixe : le même calcul redonne les mêmes chiffres
	vgVersion     = "votes-groupes-v1"
)

// vgFusionSlugs : un groupe qui change de nom en cours de législature reste un seul
// groupe (l'UDR devient « Union des droites pour la République » en septembre 2025).
var vgFusionSlugs = map[string]string{
	"udr-17": "union-des-droites-pour-la-republique-17",
}

// vgEcartes : les groupes qui ne sont pas une position politique commune. Un
// non-inscrit n'est pas un groupe ; le site ne les compare pas (D-020 : laisser
// explicitement hors analyse plutôt que rattacher d'office).
func vgNonInscrit(slug string) bool {
	return strings.HasPrefix(slug, "non-inscrit") || slug == "ep-ni"
}

// vgLigne : une ligne de mv.scrutin_groupe_vote rattachée à son scrutin.
type vgLigne struct {
	Scrutin           int64
	Institution, Jour string
	TypeVote          string
	Slug, Court, Nom  string
	Position          string
	N                 int
}

// vgGroupeMeta : un groupe tel que la source le nomme.
type vgGroupeMeta struct {
	Slug, Court, Nom string
}

// vgCompte : pour, contre, abstentions d'un groupe sur un scrutin.
type vgCompte struct{ P, C, A int }

func (c vgCompte) exprimes() int { return c.P + c.C + c.A }

// vgBase : tous les décomptes d'une assemblée sur la fenêtre.
type vgBase struct {
	Jours    []string       // jours de séance, triés
	Scrutins []int64        // identifiants, triés
	JourDe   []int          // JourDe[s] : l'indice du jour du scrutin s
	TypeDe   []string       // TypeDe[s] : le type du scrutin
	Groupes  []vgGroupeMeta // triés par libellé court
	Cpt      [][]vgCompte   // Cpt[g][s]
}

// vgMatrices : ce que le calcul lit — une position majoritaire et un score par
// groupe et par scrutin. -1 (et NaN) quand le groupe n'a pas de position.
type vgMatrices struct {
	Score [][]float64
	Maj   [][]int8 // 0 pour, 1 contre, 2 abstention, -1 aucune
}

// vgMatricesDe applique la règle de position : au moins vgMinVotants exprimés, et
// une position majoritaire UNIQUE (une égalité n'est pas une position). Les absents ne
// sont jamais une position : ils n'entrent nulle part.
func vgMatricesDe(b *vgBase) vgMatrices {
	m := vgMatrices{Score: make([][]float64, len(b.Groupes)), Maj: make([][]int8, len(b.Groupes))}
	for g := range b.Groupes {
		m.Score[g] = make([]float64, len(b.Scrutins))
		m.Maj[g] = make([]int8, len(b.Scrutins))
		for s, c := range b.Cpt[g] {
			e := c.exprimes()
			if e < vgMinVotants {
				m.Score[g][s], m.Maj[g][s] = math.NaN(), -1
				continue
			}
			m.Score[g][s] = float64(c.P-c.C) / float64(e)
			m.Maj[g][s] = vgMajoritaire(c)
		}
	}
	return m
}

// vgMajoritaire : 0 pour, 1 contre, 2 abstention ; -1 si deux positions sont à égalité en tête.
func vgMajoritaire(c vgCompte) int8 {
	v := [3]int{c.P, c.C, c.A}
	best, n := -1, 0
	var idx int8 = -1
	for i, x := range v {
		switch {
		case x > best:
			best, n, idx = x, 1, int8(i)
		case x == best:
			n++
		}
	}
	if n != 1 {
		return -1
	}
	return idx
}

// vgSomme : les sommes d'une paire sur un jour (ou sur l'échantillon entier).
// Le coefficient de Pearson et l'accord s'en déduisent sans repasser sur les scrutins,
// ce qui rend le rééchantillonnage par jour presque gratuit.
type vgSomme struct {
	n, sx, sy, sxx, syy, sxy float64
	accord, commun           float64
}

func (a *vgSomme) ajoute(b vgSomme, poids float64) {
	a.n += poids * b.n
	a.sx += poids * b.sx
	a.sy += poids * b.sy
	a.sxx += poids * b.sxx
	a.syy += poids * b.syy
	a.sxy += poids * b.sxy
	a.accord += poids * b.accord
	a.commun += poids * b.commun
}

// r : le coefficient de corrélation de Pearson entre les deux scores.
func (a vgSomme) r() (float64, bool) {
	if a.n < vgMinPaire {
		return math.NaN(), false
	}
	vx := a.n*a.sxx - a.sx*a.sx
	vy := a.n*a.syy - a.sy*a.sy
	if vx <= 0 || vy <= 0 {
		return math.NaN(), false
	}
	return (a.n*a.sxy - a.sx*a.sy) / math.Sqrt(vx*vy), true
}

// acc : la part des scrutins communs où les deux groupes ont la même position majoritaire.
func (a vgSomme) acc() (float64, bool) {
	if a.commun < vgMinPaire {
		return math.NaN(), false
	}
	return a.accord / a.commun, true
}

// vgIndexPaire : l'indice de la paire (i<j) parmi n(n-1)/2.
func vgIndexPaire(n, i, j int) int {
	if i > j {
		i, j = j, i
	}
	return i*n - i*(i+1)/2 + (j - i - 1)
}

// vgSommesParJour : pour chaque paire et chaque jour de séance, les sommes ci-dessus.
func vgSommesParJour(b *vgBase, m vgMatrices) [][]vgSomme {
	n := len(b.Groupes)
	out := make([][]vgSomme, n*(n-1)/2)
	for p := range out {
		out[p] = make([]vgSomme, len(b.Jours))
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			p := vgIndexPaire(n, i, j)
			for s := range b.Scrutins {
				x, y := m.Score[i][s], m.Score[j][s]
				if math.IsNaN(x) || math.IsNaN(y) {
					continue
				}
				d := &out[p][b.JourDe[s]]
				d.n++
				d.sx += x
				d.sy += y
				d.sxx += x * x
				d.syy += y * y
				d.sxy += x * y
				if m.Maj[i][s] >= 0 && m.Maj[j][s] >= 0 {
					d.commun++
					if m.Maj[i][s] == m.Maj[j][s] {
						d.accord++
					}
				}
			}
		}
	}
	return out
}

// vgPaire : les chiffres d'une paire de groupes.
type vgPaire struct {
	I, J                 int
	N                    int // scrutins où les deux ont une position
	R, Bas, Haut         float64
	Acc, AccBas, AccHaut float64
	OK                   bool
}

// vgResultat : tout ce que le calcul rend pour une assemblée.
type vgResultat struct {
	Paires    []vgPaire // indexées par vgIndexPaire
	Voisin    []int     // le plus proche de chaque groupe (-1 s'il n'a aucune paire)
	Stabilite []float64 // part des rééchantillonnages où ce voisin reste le plus proche
}

func (r *vgResultat) paire(n, i, j int) vgPaire { return r.Paires[vgIndexPaire(n, i, j)] }

// vgCalcule : les chiffres de chaque paire sur l'échantillon entier, puis leur intervalle
// de confiance à 95 % par rééchantillonnage des JOURS DE SÉANCE (pas des scrutins :
// les scrutins d'un même jour, souvent d'un même texte, ne sont pas indépendants —
// docs/perimetre.md § 5.2 demande de corriger cette redondance).
func vgCalcule(b *vgBase, m vgMatrices) *vgResultat {
	n := len(b.Groupes)
	par := vgSommesParJour(b, m)
	nj := len(b.Jours)
	np := len(par)
	res := &vgResultat{Paires: make([]vgPaire, np), Voisin: make([]int, n), Stabilite: make([]float64, n)}

	total := make([]vgSomme, np)
	for p := range par {
		for d := range par[p] {
			total[p].ajoute(par[p][d], 1)
		}
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			p := vgIndexPaire(n, i, j)
			r, ok := total[p].r()
			a, _ := total[p].acc()
			res.Paires[p] = vgPaire{I: i, J: j, N: int(total[p].n), R: r, Acc: a, OK: ok}
		}
	}
	res.Voisin = vgVoisins(n, func(i, j int) float64 { return res.Paires[vgIndexPaire(n, i, j)].R })

	if nj == 0 {
		return res
	}
	rng := rand.New(rand.NewPCG(vgGraine, vgGraine))
	rs := make([][]float64, np)
	as := make([][]float64, np)
	tient := make([]int, n)
	poids := make([]float64, nj)
	for rep := 0; rep < vgReplicats; rep++ {
		for d := range poids {
			poids[d] = 0
		}
		for k := 0; k < nj; k++ {
			poids[rng.IntN(nj)]++
		}
		rr := make([]float64, np)
		for p := range par {
			var s vgSomme
			for d, w := range poids {
				if w > 0 {
					s.ajoute(par[p][d], w)
				}
			}
			r, ok := s.r()
			if ok {
				rs[p] = append(rs[p], r)
			}
			rr[p] = r
			if a, ok := s.acc(); ok {
				as[p] = append(as[p], a)
			}
		}
		v := vgVoisins(n, func(i, j int) float64 { return rr[vgIndexPaire(n, i, j)] })
		for i := range v {
			if v[i] == res.Voisin[i] && v[i] >= 0 {
				tient[i]++
			}
		}
	}
	for p := range res.Paires {
		res.Paires[p].Bas, res.Paires[p].Haut = vgPercentiles(rs[p])
		res.Paires[p].AccBas, res.Paires[p].AccHaut = vgPercentiles(as[p])
	}
	for i := range tient {
		res.Stabilite[i] = float64(tient[i]) / float64(vgReplicats)
	}
	return res
}

// vgVoisins : pour chaque groupe, le groupe dont la valeur de la paire est la plus haute
// (-1 si aucune paire n'a de valeur). À égalité, l'indice le plus bas — le calcul reste déterministe.
func vgVoisins(n int, val func(i, j int) float64) []int {
	out := make([]int, n)
	for i := 0; i < n; i++ {
		out[i] = -1
		best := math.Inf(-1)
		for j := 0; j < n; j++ {
			if j == i {
				continue
			}
			v := val(i, j)
			if math.IsNaN(v) {
				continue
			}
			if v > best {
				best, out[i] = v, j
			}
		}
	}
	return out
}

// vgPercentiles : les bornes 2,5 % et 97,5 % ; NaN si l'échantillon est trop pauvre.
func vgPercentiles(v []float64) (float64, float64) {
	if len(v) < vgReplicats/2 {
		return math.NaN(), math.NaN()
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	at := func(q float64) float64 { return s[int(math.Floor(q*float64(len(s)-1)))] }
	return at(0.025), at(0.975)
}

// vgNoeud : une fusion de la classification hiérarchique. Les feuilles sont les
// groupes (0..n-1), les nœuds internes n, n+1, ...
type vgNoeud struct {
	A, B    int
	Hauteur float64
	Taille  int
}

// vgClassifie : classification ascendante hiérarchique à lien moyen (UPGMA) sur la distance
// 1 − r. Une paire sans corrélation vaut distance 1 (aucune ressemblance mesurée).
func vgClassifie(n int, dist func(i, j int) float64) []vgNoeud {
	d := make([][]float64, n)
	for i := range d {
		d[i] = make([]float64, n)
		for j := range d[i] {
			if i != j {
				v := dist(i, j)
				if math.IsNaN(v) {
					v = 1
				}
				d[i][j] = v
			}
		}
	}
	membres := make([][]int, n)
	id := make([]int, n)
	for i := 0; i < n; i++ {
		membres[i], id[i] = []int{i}, i
	}
	actifs := make([]bool, n)
	for i := range actifs {
		actifs[i] = true
	}
	var out []vgNoeud
	moy := func(x, y []int) float64 {
		var s float64
		for _, a := range x {
			for _, b := range y {
				s += d[a][b]
			}
		}
		return s / float64(len(x)*len(y))
	}
	for k := 0; k < n-1; k++ {
		bi, bj, best := -1, -1, math.Inf(1)
		for i := 0; i < n; i++ {
			if !actifs[i] {
				continue
			}
			for j := i + 1; j < n; j++ {
				if !actifs[j] {
					continue
				}
				if v := moy(membres[i], membres[j]); v < best-1e-12 {
					bi, bj, best = i, j, v
				}
			}
		}
		out = append(out, vgNoeud{A: id[bi], B: id[bj], Hauteur: best, Taille: len(membres[bi]) + len(membres[bj])})
		membres[bi] = append(membres[bi], membres[bj]...)
		id[bi] = n + k
		actifs[bj] = false
	}
	return out
}

// vgOrdre : l'ordre des feuilles du dendrogramme. À chaque fusion, la branche la plus
// grosse d'abord, puis l'ordre alphabétique : un choix déterministe qui n'a aucun
// sens politique, et la page le dit.
func vgOrdre(n int, arbre []vgNoeud, nom func(int) string) []int {
	if n == 0 {
		return nil
	}
	if n == 1 {
		return []int{0}
	}
	taille := func(x int) int {
		if x < n {
			return 1
		}
		return arbre[x-n].Taille
	}
	premier := func(x int) string { // le plus petit libellé de la branche
		var best string
		var walk func(int)
		walk = func(y int) {
			if y < n {
				if best == "" || KeySort(nom(y)) < KeySort(best) {
					best = nom(y)
				}
				return
			}
			walk(arbre[y-n].A)
			walk(arbre[y-n].B)
		}
		walk(x)
		return best
	}
	var feuilles func(int) []int
	feuilles = func(x int) []int {
		if x < n {
			return []int{x}
		}
		a, b := arbre[x-n].A, arbre[x-n].B
		if taille(a) < taille(b) || (taille(a) == taille(b) && KeySort(premier(a)) > KeySort(premier(b))) {
			a, b = b, a
		}
		return append(feuilles(a), feuilles(b)...)
	}
	return feuilles(n + len(arbre) - 1)
}

// vgChargeLignes lit les décomptes de groupe des deux assemblées, depuis la matvue
// (jamais un GROUP BY sur core.ballot — voir internal/matview).
func vgChargeLignes(ctx context.Context, pool *pgxpool.Pool) ([]vgLigne, error) {
	rows, err := pool.Query(ctx, `
		SELECT s.id, s.institution::text, to_char(s.date_seance, 'YYYY-MM-DD'),
		       coalesce(s.type_vote, ''), v.organisation_slug, v.organisation_nom,
		       o.name, v.position, v.nombre_votes
		  FROM mv.scrutin_groupe_vote v
		  JOIN core.scrutin s ON s.id = v.scrutin_id
		  JOIN core.organization o ON o.id = v.organization_id
		 WHERE s.date_seance >= $1::date
		   AND s.institution IN ('ASSEMBLEE_NATIONALE', 'PARLEMENT_EUROPEEN')`, vgDebut)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []vgLigne
	for rows.Next() {
		var l vgLigne
		if err := rows.Scan(&l.Scrutin, &l.Institution, &l.Jour, &l.TypeVote, &l.Slug, &l.Court, &l.Nom, &l.Position, &l.N); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// vgConstruit regroupe les lignes d'une assemblée en vgBase. Rend aussi la liste des
// groupes écartés, avec la raison — jamais une disparition silencieuse.
func vgConstruit(lignes []vgLigne, institution string) (*vgBase, []vgEcarte) {
	type cle struct {
		sc   int64
		slug string
	}
	cpt := map[cle]*vgCompte{}
	meta := map[string]vgGroupeMeta{}
	jour := map[int64]string{}
	typeVote := map[int64]string{}
	for _, l := range lignes {
		if l.Institution != institution {
			continue
		}
		slug := l.Slug
		if f, ok := vgFusionSlugs[slug]; ok {
			slug = f
		}
		if _, ok := meta[slug]; !ok || slug == l.Slug {
			meta[slug] = vgGroupeMeta{Slug: slug, Court: l.Court, Nom: l.Nom}
		}
		jour[l.Scrutin], typeVote[l.Scrutin] = l.Jour, l.TypeVote
		c := cpt[cle{l.Scrutin, slug}]
		if c == nil {
			c = &vgCompte{}
			cpt[cle{l.Scrutin, slug}] = c
		}
		switch l.Position {
		case "FOR":
			c.P += l.N
		case "AGAINST":
			c.C += l.N
		case "ABSTAIN":
			c.A += l.N
		}
	}
	b := &vgBase{}
	for sc := range jour {
		b.Scrutins = append(b.Scrutins, sc)
	}
	sort.Slice(b.Scrutins, func(i, j int) bool { return b.Scrutins[i] < b.Scrutins[j] })
	jours := map[string]int{}
	for _, sc := range b.Scrutins {
		jours[jour[sc]] = 0
	}
	for j := range jours {
		b.Jours = append(b.Jours, j)
	}
	sort.Strings(b.Jours)
	for i, j := range b.Jours {
		jours[j] = i
	}
	for _, sc := range b.Scrutins {
		b.JourDe = append(b.JourDe, jours[jour[sc]])
		b.TypeDe = append(b.TypeDe, typeVote[sc])
	}

	var ecartes []vgEcarte
	var slugs []string
	for s := range meta {
		slugs = append(slugs, s)
	}
	sort.Slice(slugs, func(i, j int) bool { return KeySort(meta[slugs[i]].Court) < KeySort(meta[slugs[j]].Court) })
	for _, s := range slugs {
		cs := make([]vgCompte, len(b.Scrutins))
		avec := 0
		for k, sc := range b.Scrutins {
			if c := cpt[cle{sc, s}]; c != nil {
				cs[k] = *c
				if c.exprimes() >= vgMinVotants {
					avec++
				}
			}
		}
		switch {
		case vgNonInscrit(s):
			ecartes = append(ecartes, vgEcarte{meta[s], "non-inscrits : ce n'est pas un groupe, ils ne votent pas ensemble", avec})
		case avec < vgMinScrutins:
			ecartes = append(ecartes, vgEcarte{meta[s], fmt.Sprintf("moins de %d scrutins où le groupe compte au moins %d votants", vgMinScrutins, vgMinVotants), avec})
		default:
			b.Groupes = append(b.Groupes, meta[s])
			b.Cpt = append(b.Cpt, cs)
		}
	}
	return b, ecartes
}

// vgEcarte : un groupe laissé hors de l'analyse, et pourquoi.
type vgEcarte struct {
	vgGroupeMeta
	Raison   string
	Scrutins int
}

// dateCourteFr : « 18 juillet 2024 ».
func vgDateFr(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return dateDayFr(t)
}
