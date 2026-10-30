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

func TestSpreadAvatarsNeverOverlap(t *testing.T) {
	// Trois partis très proches (le cas réel : Renaissance, Horizons, Les Républicains).
	list := []EchelleCandidat{
		{LR: 6.3, Env: 5.7}, {LR: 6.6, Env: 6.3}, {LR: 6.4, Env: 5.9}, {LR: 0.8, Env: 3.3},
	}
	spreadAvatars(list)
	for i := range list {
		for j := i + 1; j < len(list); j++ {
			dx := (list[i].Left - list[j].Left) / 100 * planWidth
			dy := (list[i].Top - list[j].Top) / 100 * planHeight
			if d := math.Hypot(dx, dy); d < 2*avatarRadius {
				t.Errorf("avatars %d et %d se recouvrent (distance %.1f px)", i, j, d)
			}
		}
	}
	if list[3].Displaced {
		t.Errorf("un avatar isolé ne doit pas bouger")
	}
	if !list[0].Displaced && !list[1].Displaced && !list[2].Displaced {
		t.Errorf("trois avatars quasi confondus doivent être écartés")
	}
}
