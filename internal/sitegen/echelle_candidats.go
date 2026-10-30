package sitegen

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// L'échelle des candidats : les partis des candidats déclarés, placés sur deux
// axes — gauche-droite en abscisse, une position mesurée en ordonnée.
//
// Ce site ne place personne. Les positions sont celles que des experts donnent
// au PARTI du candidat (Chapel Hill Expert Survey 2024, moyenne d'appréciations,
// de 0 à 10) : celles du parti, pas du candidat, et pas les nôtres. Un candidat
// dont l'organisation n'est pas mesurée n'est pas placé, et le dit — ce n'est
// pas un jugement.
//
// L'abscisse est « lrgen », la position générale gauche-droite : 0 à gauche, 10 à
// droite, telle que les experts l'entendent dans le pays considéré (voir
// referentiels.csv : la comparer d'un pays à l'autre n'a qu'un sens limité).
//
// Cinq ordonnées possibles, chacune avec le sens que la source lui donne : l'axe se
// lit toujours de bas en haut, du 0 au 10 de l'enquête. Un score élevé ne veut
// jamais dire « plus » d'une qualité : il dit ce que l'enquête appelle 10, et
// chaque axe porte le libellé de ses deux extrémités.

type axeDef struct {
	ID, Title, Dim, Bas, Haut, Tint, Dix, Note string
}

var axesEchelle = []axeDef{
	{"ecologie", "Écologie", "environment", "L'environnement d'abord", "La croissance d'abord", "vert", "la croissance d'abord",
		"0 : le parti soutient la protection de l'environnement même au prix de la croissance ; 10 : la croissance même au prix de l'environnement. " +
			"Au contact de l'axe, l'environnement d'abord."},
	{"immigration", "Immigration", "immigrate_policy", "Une politique libérale", "Une politique restrictive", "ocre", "une politique restrictive",
		"0 : le parti soutient fortement une politique libérale d'immigration ; 10 : une politique restrictive. C'est la position affichée du parti, pas ses décisions."},
	{"valeurs", "Valeurs", "galtan", "Libertaire, postmatérialiste", "Traditionnel, autoritaire", "ocre", "traditionnel, autoritaire",
		"Axe GAL-TAN, sur les libertés, les droits et les mœurs : 0, libertaire ou postmatérialiste ; 10, traditionnel ou autoritaire."},
	{"executif", "Pouvoir de l'exécutif", "executive_power", "L'exécutif contraint", "L'exécutif sans contrainte", "ocre", "l'exécutif sans contrainte",
		"Question posée pour la première fois en 2024 : 0, les dirigeants de l'exécutif doivent être contraints dans leurs actions ; 10, ils doivent pouvoir agir sans contrainte."},
	{"justice", "Indépendance de la justice", "judicial_independence", "Une justice indépendante", "Le gouvernement l'influence", "ocre", "le gouvernement doit pouvoir influencer la justice",
		"Question posée pour la première fois en 2024 : 0, la justice doit être indépendante ; 10, le gouvernement doit pouvoir exercer une influence sur elle."},
}

// Le plan est calculé pour une largeur et une hauteur de référence : les avatars sont
// écartés les uns des autres dans ces dimensions, puis tout est exprimé en
// pourcentages du plan. La page impose une largeur minimale au plan pour que les
// recouvrements ne reviennent pas.
const (
	planWidth, planHeight = 980.0, 440.0
	planMarginX           = 46.0
	planMarginY           = 42.0
	avatarRadius          = 27.0
)

// EchelleCandidat : un candidat non placé.
type EchelleCandidat struct {
	Slug, Name, FirstName, Organization, Initials string
	Portrait                                      *Media
}

// EchellePoint : un candidat placé sur un axe.
type EchellePoint struct {
	EchelleCandidat
	Left, Top       float64 // le centre de l'avatar, en % du plan, depuis le haut
	DotLeft, DotTop float64 // la position mesurée, en % du plan
	Displaced       bool    // l'avatar a été écarté de sa position mesurée
	Color           string  // la couleur de l'axe gauche-droite à cette abscisse
	Label           string  // le texte de l'infobulle
	LR, Value       float64 // les deux mesures, de 0 à 10
}

// EchelleAxis : un plan complet, prêt à dessiner.
type EchelleAxis struct {
	Root, ID, Title, Bas, Haut, Tint, Note string
	Points                                 []EchellePoint
}

// EchelleColumn : une colonne du tableau des mesures — ce que veut dire le 10.
type EchelleColumn struct{ Title, Ten string }

// EchelleRow : un candidat placé, toutes ses mesures dans l'ordre de Columns.
type EchelleRow struct {
	EchelleCandidat
	Values []string
}

// EchelleCandidats : ce que lisent l'accueil et la page 2027.
type EchelleCandidats struct {
	Root     string
	Axes     []EchelleAxis
	Columns  []EchelleColumn
	Rows     []EchelleRow
	Unplaced []EchelleCandidat
	Total    int
	Placed   int
}

// colorLR : le gauche-droite habituel en France — rouge à gauche, bleu à droite, un
// violet entre les deux. Une aide de lecture, jamais la seule : la position porte
// l'information.
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

// loadEchelleCandidats lit les scores CHES de chaque organisation et place les
// candidats dont l'organisation est mesurée sur chacun des cinq axes.
func loadEchelleCandidats(ctx context.Context, pool *pgxpool.Pool, root string, cands []*Candidate) (*EchelleCandidats, error) {
	dims := []string{"lrgen"}
	for _, a := range axesEchelle {
		dims = append(dims, a.Dim)
	}
	rows, err := pool.Query(ctx, `
		SELECT o.name, pc.dimension, pc.value::float8
		FROM core.party_classification pc
		JOIN ref.classification_set cs ON cs.id = pc.classification_set_id
		JOIN core.organization o ON o.id = pc.party_id
		WHERE cs.slug = 'ches-2024' AND pc.dimension = ANY($1)`, dims)
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

	out := &EchelleCandidats{Root: root, Total: len(cands)}
	out.Columns = append(out.Columns, EchelleColumn{"Gauche-droite", "la droite"})
	for _, a := range axesEchelle {
		out.Columns = append(out.Columns, EchelleColumn{a.Title, a.Dix})
	}

	type placed struct {
		c      EchelleCandidat
		org    string
		scores map[string]float64
	}
	var list []placed
	for _, c := range cands {
		e := EchelleCandidat{Slug: c.Slug, Name: c.Name, FirstName: c.FirstName, Organization: c.Organization,
			Portrait: c.Portrait, Initials: initialsOf(c.FirstName, c.Name)}
		var s map[string]float64
		if c.Org != nil && c.Org.CHESName != "" {
			s = scores[c.Org.CHESName]
		}
		complete := s != nil
		for _, d := range dims {
			if _, ok := s[d]; !ok {
				complete = false
			}
		}
		if complete {
			list = append(list, placed{e, c.Organization, s})
		} else {
			out.Unplaced = append(out.Unplaced, e)
		}
	}
	byName := func(a, b EchelleCandidat) bool {
		return KeySort(a.Name+" "+a.FirstName) < KeySort(b.Name+" "+b.FirstName)
	}
	sort.Slice(list, func(i, j int) bool { return byName(list[i].c, list[j].c) })
	sort.Slice(out.Unplaced, func(i, j int) bool { return byName(out.Unplaced[i], out.Unplaced[j]) })
	out.Placed = len(list)

	for _, p := range list {
		row := EchelleRow{EchelleCandidat: p.c, Values: []string{Decimal(p.scores["lrgen"], 1)}}
		for _, a := range axesEchelle {
			row.Values = append(row.Values, Decimal(p.scores[a.Dim], 1))
		}
		out.Rows = append(out.Rows, row)
	}
	for _, a := range axesEchelle {
		axis := EchelleAxis{Root: root, ID: a.ID, Title: a.Title, Bas: a.Bas, Haut: a.Haut, Tint: a.Tint, Note: a.Note}
		xs, ys := make([]float64, len(list)), make([]float64, len(list))
		for i, p := range list {
			xs[i], ys[i] = p.scores["lrgen"], p.scores[a.Dim]
		}
		for i, pl := range layoutPoints(xs, ys) {
			p := list[i]
			axis.Points = append(axis.Points, EchellePoint{
				EchelleCandidat: p.c,
				Left:            pl.left, Top: pl.top, DotLeft: pl.dotLeft, DotTop: pl.dotTop, Displaced: pl.displaced,
				Color: colorLR(xs[i]), LR: xs[i], Value: ys[i],
				Label: fmt.Sprintf("%s %s, %s — gauche-droite %s ; %s %s", p.c.FirstName, p.c.Name, p.org,
					Decimal(xs[i], 1), strings.ToLower(a.Title), Decimal(ys[i], 1)),
			})
		}
		out.Axes = append(out.Axes, axis)
	}
	return out, nil
}

type placement struct {
	left, top, dotLeft, dotTop float64
	displaced                  bool
}

// layoutPoints place chaque candidat à l'abscisse xs[i] et à l'ordonnée ys[i] (de 0 à
// 10), écarte les avatars qui se recouvriraient, puis exprime les positions en
// pourcentages du plan. La position mesurée garde son point ; un trait la relie à
// l'avatar quand il a bougé. Le calcul est déterministe : mêmes entrées, même plan.
func layoutPoints(xs, ys []float64) []placement {
	type pt struct{ tx, ty, x, y float64 }
	pts := make([]pt, len(xs))
	for i := range xs {
		x := planMarginX + xs[i]/10*(planWidth-2*planMarginX)
		y := planMarginY + ys[i]/10*(planHeight-2*planMarginY) // y vers le haut : 0, au contact de l'axe
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
	out := make([]placement, len(pts))
	for i, p := range pts {
		out[i] = placement{
			left: p.x / planWidth * 100, top: 100 - p.y/planHeight*100,
			dotLeft: p.tx / planWidth * 100, dotTop: 100 - p.ty/planHeight*100,
			displaced: math.Hypot(p.x-p.tx, p.y-p.ty) > avatarRadius*0.6,
		}
	}
	return out
}
