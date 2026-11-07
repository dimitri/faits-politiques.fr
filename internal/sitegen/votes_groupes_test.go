package sitegen

import (
	"fmt"
	"math"
	"testing"
)

func TestVgMajoritaire(t *testing.T) {
	for _, c := range []struct {
		in   vgCompte
		want int8
	}{
		{vgCompte{5, 3, 1}, 0},
		{vgCompte{1, 5, 0}, 1},
		{vgCompte{0, 1, 3}, 2},
		{vgCompte{4, 4, 1}, -1}, // égalité en tête : pas une position
		{vgCompte{2, 2, 2}, -1},
	} {
		if got := vgMajoritaire(c.in); got != c.want {
			t.Errorf("vgMajoritaire(%+v) = %d, attendu %d", c.in, got, c.want)
		}
	}
}

func TestVgIndexPaire(t *testing.T) {
	n := 6
	vu := map[int]bool{}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			k := vgIndexPaire(n, i, j)
			if k != vgIndexPaire(n, j, i) {
				t.Fatalf("la paire (%d,%d) n'est pas symétrique", i, j)
			}
			if k < 0 || k >= n*(n-1)/2 || vu[k] {
				t.Fatalf("indice %d invalide ou en double pour (%d,%d)", k, i, j)
			}
			vu[k] = true
		}
	}
}

// synthetique : trois groupes sur 120 scrutins répartis en 24 jours. A et B votent presque
// pareil ; C est à l'opposé de A.
func synthetique() *vgBase {
	b := &vgBase{Groupes: []vgGroupeMeta{{"a", "A", "Groupe A"}, {"b", "B", "Groupe B"}, {"c", "C", "Groupe C"}}}
	b.Cpt = make([][]vgCompte, 3)
	for s := 0; s < 120; s++ {
		b.Scrutins = append(b.Scrutins, int64(s+1))
		b.JourDe = append(b.JourDe, s/5)
		b.TypeDe = append(b.TypeDe, "")
	}
	for j := 0; j < 24; j++ {
		b.Jours = append(b.Jours, fmt.Sprintf("2025-01-%02d", j+1))
	}
	for g := range b.Cpt {
		b.Cpt[g] = make([]vgCompte, 120)
	}
	for s := 0; s < 120; s++ {
		pour := s%3 != 0 // A pour deux fois sur trois
		fort := 3 + s%4
		a := vgCompte{P: fort + 10, C: 1}
		if !pour {
			a = vgCompte{P: 1, C: fort + 10}
		}
		bb := a
		if s%10 == 0 { // B diverge de temps en temps
			bb = vgCompte{P: a.C, C: a.P}
		}
		c := vgCompte{P: a.C, C: a.P, A: s % 2}
		b.Cpt[0][s], b.Cpt[1][s], b.Cpt[2][s] = a, bb, c
	}
	return b
}

func pearsonNaif(x, y []float64) float64 {
	var n, sx, sy float64
	for i := range x {
		n++
		sx += x[i]
		sy += y[i]
	}
	mx, my := sx/n, sy/n
	var sxy, sxx, syy float64
	for i := range x {
		sxy += (x[i] - mx) * (y[i] - my)
		sxx += (x[i] - mx) * (x[i] - mx)
		syy += (y[i] - my) * (y[i] - my)
	}
	return sxy / math.Sqrt(sxx*syy)
}

func TestVgCalculCorrelation(t *testing.T) {
	b := synthetique()
	m := vgMatricesDe(b)
	res := vgCalcule(b, m)
	n := 3
	for _, c := range [][2]int{{0, 1}, {0, 2}, {1, 2}} {
		want := pearsonNaif(m.Score[c[0]], m.Score[c[1]])
		p := res.paire(n, c[0], c[1])
		if !p.OK || math.Abs(p.R-want) > 1e-9 {
			t.Errorf("paire %v : r = %v (ok=%v), attendu %v", c, p.R, p.OK, want)
		}
		if p.N != 120 {
			t.Errorf("paire %v : %d scrutins communs, attendu 120", c, p.N)
		}
		if !(p.Bas <= p.R && p.R <= p.Haut) {
			t.Errorf("paire %v : r = %v hors de son intervalle [%v ; %v]", c, p.R, p.Bas, p.Haut)
		}
	}
	if r := res.paire(n, 0, 2).R; r > -0.9 {
		t.Errorf("A et C votent à l'opposé : r = %v", r)
	}
	if res.Voisin[0] != 1 || res.Voisin[1] != 0 {
		t.Errorf("voisins = %v : A et B doivent être mutuellement les plus proches", res.Voisin)
	}
	if res.Stabilite[0] < 0.9 {
		t.Errorf("stabilité du voisin de A = %v, attendue élevée", res.Stabilite[0])
	}
}

// Le calcul est déterministe : la graine est fixe.
func TestVgDeterministe(t *testing.T) {
	b := synthetique()
	m := vgMatricesDe(b)
	r1, r2 := vgCalcule(b, m), vgCalcule(b, m)
	for k := range r1.Paires {
		a, c := r1.Paires[k], r2.Paires[k]
		if a.Bas != c.Bas || a.Haut != c.Haut || a.R != c.R {
			t.Fatalf("paire %d : deux calculs donnent deux résultats", k)
		}
	}
}

// Un groupe à moins de vgMinVotants exprimés n'a pas de position : il ne compte pas dans la paire.
func TestVgPositionMinimale(t *testing.T) {
	b := synthetique()
	for s := 0; s < 100; s++ {
		b.Cpt[2][s] = vgCompte{P: 1, C: 1} // 2 exprimés
	}
	m := vgMatricesDe(b)
	res := vgCalcule(b, m)
	if p := res.paire(3, 0, 2); p.N != 20 || p.OK {
		t.Errorf("20 scrutins communs : la paire ne doit pas avoir de chiffre (N=%d ok=%v)", p.N, p.OK)
	}
}

func TestVgClassifie(t *testing.T) {
	// Quatre points : 0 et 1 proches (0,1), 2 un peu plus loin, 3 très loin.
	d := [][]float64{
		{0, 0.1, 0.4, 1.0},
		{0.1, 0, 0.5, 1.2},
		{0.4, 0.5, 0, 0.9},
		{1.0, 1.2, 0.9, 0},
	}
	arbre := vgClassifie(4, func(i, j int) float64 { return d[i][j] })
	if len(arbre) != 3 {
		t.Fatalf("%d fusions, attendu 3", len(arbre))
	}
	if arbre[0].A != 0 || arbre[0].B != 1 || math.Abs(arbre[0].Hauteur-0.1) > 1e-9 {
		t.Errorf("première fusion = %+v", arbre[0])
	}
	// {0,1} + 2 : moyenne de 0,4 et 0,5 = 0,45
	if arbre[1].A != 4 || arbre[1].B != 2 || math.Abs(arbre[1].Hauteur-0.45) > 1e-9 {
		t.Errorf("deuxième fusion = %+v", arbre[1])
	}
	// {0,1,2} + 3 : (1,0 + 1,2 + 0,9) / 3 = 1,0333
	if math.Abs(arbre[2].Hauteur-(1.0+1.2+0.9)/3) > 1e-9 || arbre[2].Taille != 4 {
		t.Errorf("troisième fusion = %+v", arbre[2])
	}
	ordre := vgOrdre(4, arbre, func(i int) string { return fmt.Sprint("g", i) })
	if len(ordre) != 4 {
		t.Fatalf("ordre = %v", ordre)
	}
	vu := map[int]bool{}
	for _, g := range ordre {
		vu[g] = true
	}
	if len(vu) != 4 {
		t.Errorf("l'ordre doit être une permutation : %v", ordre)
	}
	// Le dessin garde {0,1} côte à côte.
	pos := map[int]int{}
	for k, g := range ordre {
		pos[g] = k
	}
	if d := pos[0] - pos[1]; d != 1 && d != -1 {
		t.Errorf("0 et 1 doivent être voisins dans l'ordre %v", ordre)
	}
}

// Les deux slugs de l'UDR forment un seul groupe ; un non-inscrit est écarté avec sa raison.
func TestVgConstruit(t *testing.T) {
	var l []vgLigne
	add := func(sc int64, jour, slug, court string, pos string, n int) {
		l = append(l, vgLigne{Scrutin: sc, Institution: "ASSEMBLEE_NATIONALE", Jour: jour, Slug: slug, Court: court, Nom: court, Position: pos, N: n})
	}
	for s := int64(1); s <= 120; s++ {
		jour := fmt.Sprintf("2025-02-%02d", 1+s%20)
		slug := "udr-17"
		if s > 60 {
			slug = "union-des-droites-pour-la-republique-17"
		}
		add(s, jour, slug, "UDR", "FOR", 5)
		add(s, jour, "rassemblement-national-17", "RN", "FOR", 20)
		add(s, jour, "non-inscrit-17", "NI", "AGAINST", 4)
	}
	b, ecartes := vgConstruit(l, "ASSEMBLEE_NATIONALE")
	if len(b.Groupes) != 2 {
		t.Fatalf("groupes = %v : l'UDR doit être un seul groupe", b.Groupes)
	}
	if len(ecartes) != 1 || ecartes[0].Slug != "non-inscrit-17" || ecartes[0].Raison == "" {
		t.Errorf("écartés = %+v", ecartes)
	}
	for g := range b.Groupes {
		if b.Groupes[g].Court == "UDR" {
			s := 0
			for _, c := range b.Cpt[g] {
				if c.P == 5 {
					s++
				}
			}
			if s != 120 {
				t.Errorf("l'UDR compte %d scrutins, attendu 120", s)
			}
		}
	}
}
