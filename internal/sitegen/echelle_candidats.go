package sitegen

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// L'échelle des candidats de l'accueil : les partis des candidats déclarés, placés
// sur deux axes — gauche-droite en abscisse, environnement en ordonnée.
//
// Ce site ne place personne. Les deux positions sont celles que des experts
// donnent au PARTI du candidat (Chapel Hill Expert Survey 2024, moyenne
// d'appréciations, de 0 à 10) : celle du parti, pas du candidat, et pas la nôtre.
// Un candidat dont l'organisation n'est pas mesurée n'est pas placé, et le dit —
// ce n'est pas un jugement.
//
//   - l'abscisse est « lrgen », la position générale gauche-droite : 0 à gauche,
//     10 à droite ;
//   - l'ordonnée est « environment » : 0, le parti soutient la protection de
//     l'environnement même au prix de la croissance (en bas, au contact de l'axe) ;
//     10, il soutient la croissance même au prix de l'environnement (en haut).

// Le plan est calculé pour une largeur et une hauteur de référence : les
// avatars sont écartés les uns des autres dans ces dimensions, puis tout est
// exprimé en pourcentages du plan. La page impose une largeur minimale au plan
// pour que les recouvrements ne reviennent pas.
const (
	planWidth, planHeight = 980.0, 440.0
	planMarginX           = 46.0
	planMarginY           = 42.0
	avatarRadius          = 27.0
)

// EchelleCandidat : un candidat sur la page d'accueil.
type EchelleCandidat struct {
	Slug, Name, FirstName, Organization, Initials string
	Portrait                                      *Media
	Placed                                        bool
	LR, Env                                       float64 // 0 à 10, tels que publiés
	Left, Top                                     float64 // le centre de l'avatar, en % du plan, depuis le haut
	DotLeft, DotTop                               float64 // la position mesurée, en % du plan
	Displaced                                     bool    // l'avatar a été écarté de sa position mesurée
	Color                                         string  // la couleur de l'axe gauche-droite à cette abscisse
	Label                                         string  // le texte de l'infobulle
}

// EchelleCandidats : ce que lit l'accueil.
type EchelleCandidats struct {
	Placed, Unplaced []EchelleCandidat
	Total            int
}

// colorLR : le gauche-droite habituel en France — rouge à gauche, bleu à droite,
// un violet entre les deux. Une aide de lecture, jamais la seule : la position
// porte l'information.
func colorLR(lr float64) string {
	type rgb struct{ r, g, b float64 }
	left, mid, right := rgb{214, 57, 47}, rgb{126, 91, 166}, rgb{36, 86, 168}
	t := math.Max(0, math.Min(1, lr/10))
	a, b, u := left, mid, t*2
	if t > 0.5 {
		a, b, u = mid, right, (t-0.5)*2
	}
	mix := func(x, y float64) int { return int(math.Round(x + (y-x)*u)) }
	return fmt.Sprintf("#%02X%02X%02X", mix(a.r, b.r), mix(a.g, b.g), mix(a.b, b.b))
}

func initialsOf(first, last string) string {
	f := []rune(strings.TrimSpace(first))
	l := []rune(strings.TrimSpace(last))
	var s string
	if len(f) > 0 {
		s += string(f[0])
	}
	if len(l) > 0 {
		s += string(l[0])
	}
	return strings.ToUpper(s)
}

// loadEchelleCandidats lit les deux scores CHES de chaque organisation et place
// les candidats dont l'organisation est mesurée.
func loadEchelleCandidats(ctx context.Context, pool *pgxpool.Pool, cands []*Candidate) (*EchelleCandidats, error) {
	rows, err := pool.Query(ctx, `
		SELECT o.name, pc.dimension, pc.value::float8
		FROM core.party_classification pc
		JOIN ref.classification_set cs ON cs.id = pc.classification_set_id
		JOIN core.organization o ON o.id = pc.party_id
		WHERE cs.slug = 'ches-2024' AND pc.dimension IN ('lrgen', 'environment')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	scores := map[string]map[string]float64{}
	for rows.Next() {
		var name, dim string
		var v float64
		if err := rows.Scan(&name, &dim, &v); err != nil {
			return nil, err
		}
		if scores[name] == nil {
			scores[name] = map[string]float64{}
		}
		scores[name][dim] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := &EchelleCandidats{Total: len(cands)}
	for _, c := range cands {
		e := EchelleCandidat{Slug: c.Slug, Name: c.Name, FirstName: c.FirstName, Organization: c.Organization,
			Portrait: c.Portrait, Initials: initialsOf(c.FirstName, c.Name)}
		if c.Org != nil && c.Org.CHESName != "" {
			s := scores[c.Org.CHESName]
			if lr, ok := s["lrgen"]; ok {
				if env, ok := s["environment"]; ok {
					e.Placed, e.LR, e.Env = true, lr, env
					e.Color = colorLR(lr)
					e.Label = fmt.Sprintf("%s %s, %s — gauche-droite %s, environnement %s",
						c.FirstName, c.Name, c.Organization, Decimal(lr, 1), Decimal(env, 1))
				}
			}
		}
		if e.Placed {
			out.Placed = append(out.Placed, e)
		} else {
			out.Unplaced = append(out.Unplaced, e)
		}
	}
	for _, l := range [][]EchelleCandidat{out.Placed, out.Unplaced} {
		sort.Slice(l, func(i, j int) bool {
			return KeySort(l[i].Name+" "+l[i].FirstName) < KeySort(l[j].Name+" "+l[j].FirstName)
		})
	}
	spreadAvatars(out.Placed)
	return out, nil
}

// spreadAvatars écarte les avatars qui se recouvriraient, puis exprime les
// positions en pourcentages du plan. La position mesurée garde son point ; un
// trait la relie à l'avatar quand il a bougé.
func spreadAvatars(list []EchelleCandidat) {
	type pt struct{ tx, ty, x, y float64 }
	pts := make([]pt, len(list))
	for i, e := range list {
		x := planMarginX + e.LR/10*(planWidth-2*planMarginX)
		y := planMarginY + e.Env/10*(planHeight-2*planMarginY) // y vers le haut : 0, au contact de l'axe
		pts[i] = pt{x, y, x, y}
	}
	minDist := 2*avatarRadius + 20 // la place de l'étiquette sous chaque avatar
	for iter := 0; iter < 600; iter++ {
		moved := false
		for i := range pts {
			for j := i + 1; j < len(pts); j++ {
				dx, dy := pts[j].x-pts[i].x, pts[j].y-pts[i].y
				d := math.Hypot(dx, dy)
				if d >= minDist {
					continue
				}
				if d < 1e-6 { // même point : on les sépare sur l'horizontale
					dx, dy, d = 1, 0, 1
				}
				push := (minDist - d) / 2
				pts[i].x -= dx / d * push
				pts[i].y -= dy / d * push
				pts[j].x += dx / d * push
				pts[j].y += dy / d * push
				moved = true
			}
		}
		for i := range pts { // un ressort faible les rappelle vers leur mesure
			pts[i].x += (pts[i].tx - pts[i].x) * 0.015
			pts[i].y += (pts[i].ty - pts[i].y) * 0.015
			pts[i].x = math.Max(avatarRadius, math.Min(planWidth-avatarRadius, pts[i].x))
			pts[i].y = math.Max(avatarRadius, math.Min(planHeight-avatarRadius, pts[i].y))
		}
		if !moved && iter > 20 {
			break
		}
	}
	for i := range list {
		list[i].Left = pts[i].x / planWidth * 100
		list[i].Top = 100 - pts[i].y/planHeight*100
		list[i].DotLeft = pts[i].tx / planWidth * 100
		list[i].DotTop = 100 - pts[i].ty/planHeight*100
		list[i].Displaced = math.Hypot(pts[i].x-pts[i].tx, pts[i].y-pts[i].ty) > avatarRadius*0.6
	}
}
