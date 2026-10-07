package sitegen

import (
	"context"
	"html/template"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Les cartes départementales : ce que le site sait de chaque département, en
// agrégeant des données communales.
//
// C'est aujourd'hui la donnée la mieux couverte du site — 34 869 communes sur
// 34 875 pour les finances, la totalité pour la délinquance — très loin devant
// la donnée politique. L'ordre des cartes suit cette couverture.
//
// Ancienne page /territoires/, fusionnée dans /collectivites/ : ces cartes
// agrègent des données communales, comme les tableaux régions/départements/
// EPCI de collectivites.go agrègent des budgets — une même page, un même
// niveau de lecture (« l'étage entre la commune et l'État »), plutôt que deux
// pages qui se renvoyaient l'une à l'autre pour dire la même chose.
type MapTerritory struct {
	Slug, Title, Question, Source, Note string
	Overview                            Map
	Page                                PageMap
}

type StatsTerritories struct {
	Maps                        []MapTerritory
	Defs                        template.HTML
	Municipalities, Departments int
	Year                        int
}

func loadTerritories(ctx context.Context, pool *pgxpool.Pool) (*StatsTerritories, error) {
	// Deux jeux de tracés : la vignette de l'index, grossière et partagée par
	// toutes les cartes de la grille ; le tracé fin, réservé aux pages de
	// détail où une seule carte s'affiche.
	vign, err := setOutlines(ctx, pool, "DEPARTEMENT", toleranceOverview)
	if err != nil {
		return nil, err
	}
	end2, err := setOutlines(ctx, pool, "DEPARTEMENT", toleranceFull)
	if err != nil {
		return nil, err
	}
	st := &StatsTerritories{Departments: len(vign.Codes), Year: 2023, Defs: vign.Defs}

	// poser fabrique d'un coup la vignette, la carte pleine et le classement.
	poser := func(c MapTerritory, cells []CellMap, unit string,
		format func(float64) string) MapTerritory {
		c.Overview = overview(vign, cells, unit, format)
		ranks := ranking(cells, vign.Noms, format)
		c.Page = PageMap{
			Slug: c.Slug, Title: c.Title, Question: c.Question,
			Source: c.Source, Note: c.Note,
			Section: "Collectivités", SectionURL: "collectivites/carte", SectionIndexURL: "collectivites",
			Map:     full(end2, cells, unit, format),
			Summary: summarizeRanking(ranks),
			Ranking: ranks,
		}
		return c
	}
	_ = pool.QueryRow(ctx, `SELECT count(DISTINCT commune_code) FROM mv.commune_indicator_dernier`).
		Scan(&st.Municipalities)

	eur := func(v float64) string { return Count(int(v+0.5)) + "\u202f€" }
	for1000 := func(v float64) string { return Decimal(v, 1) + " ‰" }
	pct := func(v float64) string { return Decimal(v, 0) + " %" }

	// Indicateurs financiers : moyenne pondérée par la population, déjà
	// calculée dans mv.dept_indicateur_communal (internal/matview) — plus le
	// double JOIN sur core.commune_indicator que cette fermeture refaisait
	// une fois par indicateur, à chaque construction.
	end := func(code string) ([]CellMap, error) {
		rows, err := pool.Query(ctx, `
			SELECT code_departement, nom_departement, valeur_par_hab
			FROM mv.dept_indicateur_communal
			WHERE indicator_code=$1 AND period_year=2023`, code)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []CellMap
		for rows.Next() {
			var cc CellMap
			var v *float64
			if err := rows.Scan(&cc.Code, &cc.Name, &v); err != nil {
				return nil, err
			}
			if v == nil {
				cc.Absent = true
			} else {
				cc.Value = *v
			}
			out = append(out, cc)
		}
		return out, rows.Err()
	}

	for _, d := range []struct{ slug, code, title, question, note string }{
		{"dette", "ofgl.dette_par_hab", "Dette des communes par habitant",
			"Combien chaque commune doit-elle, rapporté à ses habitants ?",
			"C'est la dette des communes seules : ni celle du département, ni celle de la région, ni celle de l'État."},
		{"investissement", "ofgl.investissement_par_hab", "Investissement communal par habitant",
			"Combien les communes dépensent-elles en équipement ?",
			"L'investissement d'une année n'est pas un rythme : une seule opération lourde suffit à faire bouger un petit territoire."},
		{"epargne", "ofgl.epargne_brute_par_hab", "Épargne brute par habitant",
			"Ce qui reste du fonctionnement pour investir et rembourser.",
			"Une épargne négative signale un budget de fonctionnement déficitaire."},
		{"masse-salariale", "ofgl.masse_salariale_par_hab", "Masse salariale communale par habitant",
			"Ce que coûte le personnel communal, par habitant.",
			"Elle dépend d'abord de ce que la commune gère en propre plutôt que par une intercommunalité : deux territoires ne sont pas comparables sans savoir qui fait quoi."},
	} {
		cells, err := end(d.code)
		if err != nil {
			return nil, err
		}
		st.Maps = append(st.Maps, poser(MapTerritory{
			Slug: d.slug, Title: d.title, Question: d.question, Note: d.note,
			Source: "OFGL / DGCL, exercice 2023",
		}, cells, "€ par habitant", eur))
	}

	// Densité associative : un fait sur la vie locale, sans jugement possible
	// — mv.dept_association_densite (internal/matview) remplace le calcul.
	rows, err := pool.Query(ctx, `
		SELECT code_departement, nom_departement, pour_mille
		FROM mv.dept_association_densite`)
	if err == nil {
		var cells []CellMap
		for rows.Next() {
			var cc CellMap
			var v *float64
			if err := rows.Scan(&cc.Code, &cc.Name, &v); err != nil {
				break
			}
			if v == nil {
				cc.Absent = true
			} else {
				cc.Value = *v
			}
			cells = append(cells, cc)
		}
		rows.Close()
		st.Maps = append(st.Maps, poser(MapTerritory{
			Slug: "associations", Title: "Associations pour 1 000 habitants",
			Question: "Où la vie associative déclarée est-elle la plus dense ?",
			Note:     "Le répertoire recense les associations déclarées depuis 1901 et n'enregistre pas toujours les dissolutions : le compte penche vers le haut, surtout dans les départements anciens.",
			Source:   "RNA — répertoire national des associations",
		}, cells, "pour 1 000 habitants", for1000))
	}

	// Médecins généralistes pour 100 000 habitants — la densité de l'offre de
	// soins de premier recours, département par département. Le RPPS a été
	// écarté pour cette carte : seuls 56 % des généralistes y ont une commune
	// d'exercice identifiable (remplaçants sans structure fixe, notamment),
	// une lacune de source, pas un bug de jointure. La Cnam (démographie par
	// secteur conventionnel, déjà chargée pour docs/sante-donnees.md § 2)
	// couvre les 101 départements sans exception — le code déjà NOT NULL en
	// base, par construction administrative (un médecin conventionné est
	// nécessairement rattaché à une caisse départementale). En contrepartie,
	// elle ne compte que les généralistes libéraux conventionnés : ni les
	// salariés hospitaliers, ni les non-conventionnés (0,8 % de l'ensemble
	// des médecins, cf. § 2) — un champ plus étroit mais mesuré partout.
	// Le pseudo-département « 999 » (« FRANCE » / « Tout département ») porte
	// des totaux nationaux déjà agrégés : l'exclure évite de les additionner
	// aux départements réels et de tripler le compte.
	// lpad(code,2,'0') tronquerait les codes DOM à 3 chiffres ("971" -> "97") :
	// PostgreSQL réduit une chaîne déjà plus longue que la cible au lieu de la
	// laisser telle quelle. Un CASE, pas lpad, pour ne padder que les codes à
	// un seul chiffre.
	// mv.dept_medecin_generaliste (internal/matview) remplace le calcul.
	mgrows, err := pool.Query(ctx, `
		SELECT dep, nom_departement, pour_100k
		FROM mv.dept_medecin_generaliste`)
	if err != nil {
		return nil, err
	}
	var cellsMG []CellMap
	for mgrows.Next() {
		var cc CellMap
		var v *float64
		if err := mgrows.Scan(&cc.Code, &cc.Name, &v); err != nil {
			mgrows.Close()
			return nil, err
		}
		if v == nil {
			cc.Absent = true
		} else {
			cc.Value = *v
		}
		cellsMG = append(cellsMG, cc)
	}
	mgrows.Close()
	if err := mgrows.Err(); err != nil {
		return nil, err
	}
	if len(cellsMG) > 0 {
		st.Maps = append(st.Maps, poser(MapTerritory{
			Slug: "medecins-generalistes", Title: "Médecins généralistes pour 100 000 habitants",
			Question: "Où l'offre de médecine générale est-elle la plus dense ?",
			Note: "Ne compte que les généralistes libéraux conventionnés — 55 546 en 2024, " +
				"les seuls dont cette source situe systématiquement le département d'exercice. " +
				"Les généralistes salariés (hôpital, centre de santé) et les 0,8 % non " +
				"conventionnés en sont absents. La carte compte une présence identifiable, pas " +
				"la disponibilité réelle : un désert médical peut aussi être un territoire où les " +
				"généralistes recensés n'ont plus de créneaux libres.",
			Source: "Cnam, démographie par secteur conventionnel, 2024 ; OFGL, population 2023",
		}, cellsMG, "généralistes pour 100 000 hab.", func(v float64) string { return Decimal(v, 0) }))
	}

	// Part des sièges municipaux dont la nuance nomme un parti. C'est la carte
	// qui dit pourquoi une « carte des partis » n'existe pas.
	// mv.dept_part_partisane (internal/matview) remplace le calcul.
	prows, err := pool.Query(ctx, `
		SELECT dep, nom_departement, pct FROM mv.dept_part_partisane`)
	if err == nil {
		var cells []CellMap
		for prows.Next() {
			var cc CellMap
			var v *float64
			if err := prows.Scan(&cc.Code, &cc.Name, &v); err != nil {
				break
			}
			if v == nil {
				cc.Absent = true
			} else {
				cc.Value = *v
			}
			cells = append(cells, cc)
		}
		prows.Close()
		st.Maps = append(st.Maps, poser(MapTerritory{
			Slug: "part-partisane", Title: "Sièges municipaux dont la nuance nomme un parti",
			Question: "Les élections municipales sont-elles des élections de partis ?",
			Note:     "Non, très majoritairement : 82,7 % des sièges nuancés portent une nuance « divers », que le ministère de l'Intérieur refuse d'attribuer à un parti. Treize départements sont à zéro.",
			Source:   "Ministère de l'Intérieur, municipales 2026",
		}, cells, "part des sièges", pct))
	}
	// Le RSA par habitant, département par département — core.prestation_solidarite,
	// chargé pour lui-même (RSA, PPA, AAH, ASS, aides au logement, tous mensuels
	// depuis 2017), mais jamais encore cartographié. Le RSA est le foyer, pas la
	// personne : c'est ainsi que la CNAF elle-même compte ses allocataires.
	// mv.dept_rsa (internal/matview) remplace le calcul.
	rsrows, err := pool.Query(ctx, `
		SELECT code_geo, nom_geo, to_char(mois,'YYYY-MM'), pour_mille
		FROM mv.dept_rsa`)
	if err != nil {
		return nil, err
	}
	var cellsRSA []CellMap
	var monthRSA string
	for rsrows.Next() {
		var cc CellMap
		var month string
		var v *float64
		if err := rsrows.Scan(&cc.Code, &cc.Name, &month, &v); err != nil {
			rsrows.Close()
			return nil, err
		}
		monthRSA = month
		if v == nil {
			cc.Absent = true
		} else {
			cc.Value = *v
		}
		cellsRSA = append(cellsRSA, cc)
	}
	rsrows.Close()
	if err := rsrows.Err(); err != nil {
		return nil, err
	}
	if len(cellsRSA) > 0 {
		st.Maps = append(st.Maps, poser(MapTerritory{
			Slug: "rsa", Title: "Foyers au RSA pour 1 000 habitants",
			Question: "Où le revenu de solidarité active est-il le plus versé ?",
			Note: "C'est le FOYER allocataire qui est compté, pas chaque personne couverte : un " +
				"foyer avec enfants compte pour un. Le dénominateur (population 2023) et le " +
				"numérateur (" + monthRSA + ") ne sont pas de la même date, faute d'une " +
				"population plus récente en base.",
			Source: "CNAF, données ouvertes sur les prestations de solidarité",
		}, cellsRSA, "‰ habitants", for1000))
	}

	for i := range st.Maps {
		for _, other := range st.Maps {
			if other.Slug != st.Maps[i].Slug {
				st.Maps[i].Page.Neighboring = append(st.Maps[i].Page.Neighboring,
					LinkMap{Slug: other.Slug, Title: other.Title})
			}
		}
	}
	return st, nil
}
