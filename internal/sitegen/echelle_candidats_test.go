package sitegen

import (
	"math"
	"testing"
)

func TestColorLR(t *testing.T) {
	if got := colorLR(0); got != "#D6392F" {
		t.Errorf("la gauche est rouge : %s", got)
	}
	if got := colorLR(10); got != "#2456A8" {
		t.Errorf("la droite est bleue : %s", got)
	}
	if got := colorLR(-3); got != colorLR(0) {
		t.Errorf("hors échelle, on borne : %s", got)
	}
}

func TestLayoutPointsNeverOverlap(t *testing.T) {
	// Trois partis très proches (le cas réel : Renaissance, Horizons, Les Républicains),
	// et un parti isolé.
	xs := []float64{6.3, 6.6, 6.4, 0.8}
	ys := []float64{5.7, 6.3, 5.9, 3.3}
	pl := layoutPoints(xs, ys)
	for i := range pl {
		for j := i + 1; j < len(pl); j++ {
			dx := (pl[i].left - pl[j].left) / 100 * planWidth
			dy := (pl[i].top - pl[j].top) / 100 * planHeight
			if d := math.Hypot(dx, dy); d < 2*avatarRadius {
				t.Errorf("avatars %d et %d se recouvrent (distance %.1f px)", i, j, d)
			}
		}
	}
	if pl[3].displaced {
		t.Errorf("un avatar isolé ne doit pas bouger")
	}
	if !pl[0].displaced && !pl[1].displaced && !pl[2].displaced {
		t.Errorf("trois avatars quasi confondus doivent être écartés")
	}
}

// Chaque axe se lit de bas en haut, du 0 au 10 de l'enquête : le plus grand score est
// toujours le plus haut, jamais l'inverse.
func TestLayoutPointsHigherScoreIsHigher(t *testing.T) {
	pl := layoutPoints([]float64{2, 8}, []float64{2.0, 8.0})
	if !(pl[1].dotTop < pl[0].dotTop) {
		t.Errorf("le score 8 doit être au-dessus du score 2 (top en %% : %.1f contre %.1f)", pl[1].dotTop, pl[0].dotTop)
	}
}

func TestAxesHaveBothEnds(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range axesEchelle {
		if a.Bas == "" || a.Haut == "" || a.Dix == "" || a.Note == "" {
			t.Errorf("l'axe %q doit nommer ses deux extrémités et dire ce que veut dire 10", a.ID)
		}
		if seen[a.ID] {
			t.Errorf("l'axe %q est en double", a.ID)
		}
		seen[a.ID] = true
	}
}
