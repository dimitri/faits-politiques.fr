package sitegen

import (
	"context"
	"encoding/csv"
	"html/template"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// L'élection présidentielle de 2027 : ce que le site peut dire de chaque
// candidat déclaré, et surtout ce qu'il ne peut pas.
//
// Quatre angles, chacun avec sa couverture propre — et elles sont très
// inégales, ce qui est la première chose à montrer :
//
//	VOTES    ce que la personne a voté à l'Assemblée, au Sénat, au Parlement européen
//	COMPTES  ce que son parti déclare à la CNCCFP
//	TERRAIN  où son parti présente des listes aux municipales, et ses scores
//	INTERETS ce qu'elle déclare à la HATVP
//
// Aucune synthèse, aucun classement, aucune note. Les quatre restent séparés :
// les mêler suggérerait des liens que la donnée n'établit pas.
type Angle struct {
	Present bool
	Reason  string
}

type Candidate2027 struct {
	*Candidate
	Votes    Angle
	Accounts Angle
	Terrain  Angle
	Interest Angle

	For, Against, Abstention, Expressed int
	NuanceCode                          string
	NuanceReason                        string
	MunicipalitiesLists                 int
	SeatsCM                             int
	VotesMunicipal                      int64
	FiscalYears                         int
	LastYear                            int
	TotalCharges                        float64
	CountInterest                       int
	Overview                            Map
	Page                                *PageMap
}

type Stats2027 struct {
	Candidates   []*Candidate2027
	Defs         template.HTML
	WithVotes    int
	WithTerrain  int
	WithAccounts int
	WithInterest int
}

func loadNuanceParties(path string) (map[string][2]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][2]string{}, nil
		}
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	out := map[string][2]string{}
	for i, r := range rows {
		if i == 0 || len(r) < 4 {
			continue
		}
		out[strings.TrimSpace(r[0])] = [2]string{strings.TrimSpace(r[1]), r[3]}
	}
	return out, nil
}

func load2027(ctx context.Context, pool *pgxpool.Pool, candidates []*Candidate,
	dataDir string) (*Stats2027, error) {

	nuances, err := loadNuanceParties(dataDir + "/nuance-partis.csv")
	if err != nil {
		return nil, err
	}
	vign, err := setOutlines(ctx, pool, "DEPARTEMENT", toleranceOverview)
	if err != nil {
		return nil, err
	}
	end, err := setOutlines(ctx, pool, "DEPARTEMENT", toleranceFull)
	if err != nil {
		return nil, err
	}
	st := &Stats2027{Defs: vign.Defs}

	// Les déclarations HATVP de tous les candidats en une requête, plutôt
	// qu'une par candidat — un dizaine de candidats déclarés, mais le même
	// patron N+1 qu'ailleurs dans ce fichier.
	var idsPeople []int64
	for _, c := range candidates {
		if c.Person != nil {
			idsPeople = append(idsPeople, c.Person.ID)
		}
	}
	countInterestPerPerson := map[int64]int{}
	if len(idsPeople) > 0 {
		rows, err := pool.Query(ctx, `
			SELECT d.person_id, count(*) FROM core.declaration_item i
			JOIN core.declaration d ON d.id = i.declaration_id
			WHERE d.person_id = ANY($1)
			GROUP BY d.person_id`, idsPeople)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var pid int64
			var n int
			if err := rows.Scan(&pid, &n); err != nil {
				rows.Close()
				return nil, err
			}
			countInterestPerPerson[pid] = n
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	// Le terrain municipal se calcule PAR NUANCE, pas par candidat : deux
	// candidats de la même nuance partagent exactement le même résultat.
	// terrainDeNuance mémoïse les deux requêtes (l'agrégat, la carte par
	// département) la première fois qu'une nuance est rencontrée.
	type terrainNuance struct {
		municipalities, seats int
		votes                 *int64
		cells                 []CellMap
	}
	terrainCache := map[string]*terrainNuance{}
	terrainOfNuance := func(nuanceCode string) (*terrainNuance, error) {
		if t, ok := terrainCache[nuanceCode]; ok {
			return t, nil
		}
		t := &terrainNuance{}
		terrainCache[nuanceCode] = t
		if err := pool.QueryRow(ctx, `
			SELECT count(DISTINCT commune_code), coalesce(sum(sieges_cm),0), sum(voix)
			FROM core.municipal_list
			WHERE scrutin_annee=2026 AND tour=1 AND nuance_code=$1`, nuanceCode).
			Scan(&t.municipalities, &t.seats, &t.votes); err != nil {
			return nil, err
		}
		if t.municipalities == 0 {
			return t, nil
		}
		rows, err := pool.Query(ctx, `
			WITH v AS (
			  SELECT c.code_departement dep, max(c.nom_clair) nom,
			         sum(ml.voix) FILTER (WHERE ml.nuance_code=$1) vx, sum(ml.voix) tot
			  FROM core.municipal_list ml
			  JOIN ref.commune c ON c.code_insee=ml.commune_code AND c.cog_millesime=ml.cog_millesime
			  WHERE ml.scrutin_annee=2026 AND ml.tour=1 AND ml.voix>0
			    AND ml.nuance_code IS NOT NULL AND ml.nuance_code<>''
			  GROUP BY 1)
			SELECT dep, nom, CASE WHEN vx IS NULL THEN NULL ELSE 100.0*vx/tot END FROM v`, nuanceCode)
		if err != nil {
			return t, nil // cohérent avec l'ancien code : une erreur ici n'empêchait pas le reste
		}
		for rows.Next() {
			var cc CellMap
			var v *float64
			if rows.Scan(&cc.Code, &cc.Name, &v) != nil {
				break
			}
			if v == nil {
				cc.Absent = true
			} else {
				cc.Value = *v
			}
			t.cells = append(t.cells, cc)
		}
		rows.Close()
		return t, nil
	}

	for _, c := range candidates {
		k := &Candidate2027{Candidate: c}

		// 1. Les votes — seulement si la personne a siégé.
		if c.Person != nil && c.Person.HasVotes {
			k.Votes = Angle{Present: true}
			k.For, k.Against = c.Person.For, c.Person.Against
			k.Abstention, k.Expressed = c.Person.Abstention, c.Person.Expressed
			st.WithVotes++
		} else {
			k.Votes = Angle{Reason: "n'a pas siégé dans une assemblée couverte par ce site"}
		}

		// 2. Les comptes du parti.
		if c.Org != nil && c.Org.HasAccounts {
			k.Accounts = Angle{Present: true}
			k.FiscalYears = len(c.Org.FiscalYears)
			st.WithAccounts++
		} else {
			k.Accounts = Angle{Reason: "aucun compte CNCCFP rattaché à son organisation"}
		}

		// 3. Le terrain — nécessite une nuance propre au parti. Mémoïsé par
		// nuance (terrainDeNuance) : plusieurs candidats partagent souvent
		// la même nuance, et refaire les deux requêtes pour chacun referait
		// exactement le même calcul.
		nu, ok := nuances[c.OrganizationSlug]
		if ok && nu[0] != "" {
			k.NuanceCode = nu[0]
			t, err := terrainOfNuance(nu[0])
			if err != nil {
				return nil, err
			}
			if t.municipalities > 0 {
				k.Terrain = Angle{Present: true}
				k.MunicipalitiesLists, k.SeatsCM = t.municipalities, t.seats
				if t.votes != nil {
					k.VotesMunicipal = *t.votes
				}
				st.WithTerrain++

				fmtPct := func(v float64) string { return Decimal(v, 1) + " %" }
				k.Overview = overview(vign, t.cells, "part des voix nuancées", fmtPct)
				ranks := ranking(t.cells, vign.Noms, fmtPct)
				k.Page = &PageMap{
					Slug:  strings.ToLower(nu[0]),
					Title: "Municipales 2026 — voix de la nuance " + nu[0],
					Question: "Où les listes que le ministère de l'Intérieur range sous " +
						"cette nuance ont-elles recueilli des voix ?",
					Source: "Ministère de l'Intérieur, municipales 2026, premier tour",
					Note: "Une nuance n'est pas une adhésion : c'est un rangement " +
						"administratif décidé par la préfecture, liste par liste. " +
						"82,7 % des sièges nuancés portent d'ailleurs une nuance " +
						"« divers », qui ne nomme aucun parti.",
					Section:         "Présidentielle 2027",
					SectionURL:      "2027",
					SectionIndexURL: "2027",
					Map:             full(end, t.cells, "part des voix nuancées", fmtPct),
					Summary:         summarizeRanking(ranks),
					Ranking:         ranks,
				}
			}
		}
		if !k.Terrain.Present {
			r := "aucune nuance du ministère de l'Intérieur ne désigne cette organisation"
			if ok && nu[1] != "" {
				r = nu[1]
			}
			k.Terrain = Angle{Reason: r}
		}

		// 4. Les intérêts déclarés.
		if c.Person != nil {
			k.CountInterest = countInterestPerPerson[c.Person.ID]
		}
		if k.CountInterest > 0 {
			k.Interest = Angle{Present: true}
			st.WithInterest++
		} else {
			k.Interest = Angle{Reason: "aucune déclaration HATVP rattachée"}
		}

		st.Candidates = append(st.Candidates, k)
	}
	return st, nil
}
