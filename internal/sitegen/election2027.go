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
	Raison  string
}

type Candidat2027 struct {
	*Candidat
	Votes    Angle
	Comptes  Angle
	Terrain  Angle
	Interets Angle

	Pour, Contre, Abstention, Exprimes int
	NuanceCode                         string
	NuanceRaison                       string
	CommunesListes                     int
	SiegesCM                           int
	VoixMunicipales                    int64
	Exercices                          int
	DerniereAnnee                      int
	TotalCharges                       float64
	NbInterets                         int
	Apercu                             Carte
	Page                               *PageCarte
}

type Stats2027 struct {
	Candidats    []*Candidat2027
	Defs         template.HTML
	AvecVotes    int
	AvecTerrain  int
	AvecComptes  int
	AvecInterets int
}

func loadNuancePartis(path string) (map[string][2]string, error) {
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

func load2027(ctx context.Context, pool *pgxpool.Pool, candidats []*Candidat,
	dataDir string) (*Stats2027, error) {

	nuances, err := loadNuancePartis(dataDir + "/nuance-partis.csv")
	if err != nil {
		return nil, err
	}
	vign, err := jeuContours(ctx, pool, "DEPARTEMENT", tolApercu)
	if err != nil {
		return nil, err
	}
	fin, err := jeuContours(ctx, pool, "DEPARTEMENT", tolPleine)
	if err != nil {
		return nil, err
	}
	st := &Stats2027{Defs: vign.Defs}

	for _, c := range candidats {
		k := &Candidat2027{Candidat: c}

		// 1. Les votes — seulement si la personne a siégé.
		if c.Person != nil && c.Person.HasVotes {
			k.Votes = Angle{Present: true}
			k.Pour, k.Contre = c.Person.Pour, c.Person.Contre
			k.Abstention, k.Exprimes = c.Person.Abstention, c.Person.Exprimes
			st.AvecVotes++
		} else {
			k.Votes = Angle{Raison: "n'a pas siégé dans une assemblée couverte par ce site"}
		}

		// 2. Les comptes du parti.
		if c.Org != nil && c.Org.HasComptes {
			k.Comptes = Angle{Present: true}
			k.Exercices = len(c.Org.Exercices)
			st.AvecComptes++
		} else {
			k.Comptes = Angle{Raison: "aucun compte CNCCFP rattaché à son organisation"}
		}

		// 3. Le terrain — nécessite une nuance propre au parti.
		nu, ok := nuances[c.OrganisationSlug]
		if ok && nu[0] != "" {
			k.NuanceCode = nu[0]
			var communes, sieges int
			var voix *int64
			err := pool.QueryRow(ctx, `
				SELECT count(DISTINCT commune_code), coalesce(sum(sieges_cm),0), sum(voix)
				FROM core.municipal_list
				WHERE scrutin_annee=2026 AND tour=1 AND nuance_code=$1`, nu[0]).
				Scan(&communes, &sieges, &voix)
			if err == nil && communes > 0 {
				k.Terrain = Angle{Present: true}
				k.CommunesListes, k.SiegesCM = communes, sieges
				if voix != nil {
					k.VoixMunicipales = *voix
				}
				st.AvecTerrain++

				rows, err := pool.Query(ctx, `
					WITH v AS (
					  SELECT c.code_departement dep, max(c.nom_clair) nom,
					         sum(ml.voix) FILTER (WHERE ml.nuance_code=$1) vx, sum(ml.voix) tot
					  FROM core.municipal_list ml
					  JOIN ref.commune c ON c.code_insee=ml.commune_code AND c.cog_millesime=ml.cog_millesime
					  WHERE ml.scrutin_annee=2026 AND ml.tour=1 AND ml.voix>0
					    AND ml.nuance_code IS NOT NULL AND ml.nuance_code<>''
					  GROUP BY 1)
					SELECT dep, nom, CASE WHEN vx IS NULL THEN NULL ELSE 100.0*vx/tot END FROM v`, nu[0])
				if err == nil {
					var cases []CaseCarte
					for rows.Next() {
						var cc CaseCarte
						var v *float64
						if rows.Scan(&cc.Code, &cc.Nom, &v) != nil {
							break
						}
						if v == nil {
							cc.Absent = true
						} else {
							cc.Valeur = *v
						}
						cases = append(cases, cc)
					}
					rows.Close()
					fmtPct := func(v float64) string { return Decimal(v, 1) + " %" }
					k.Apercu = apercu(vign, cases, "part des voix nuancées", fmtPct)
					rangs := classement(cases, vign.Noms, fmtPct)
					k.Page = &PageCarte{
						Slug:  strings.ToLower(nu[0]),
						Titre: "Municipales 2026 — voix de la nuance " + nu[0],
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
						Carte:           pleine(fin, cases, "part des voix nuancées", fmtPct),
						Resume:          resumerClassement(rangs),
						Classement:      rangs,
					}
				}
			}
		}
		if !k.Terrain.Present {
			r := "aucune nuance du ministère de l'Intérieur ne désigne cette organisation"
			if ok && nu[1] != "" {
				r = nu[1]
			}
			k.Terrain = Angle{Raison: r}
		}

		// 4. Les intérêts déclarés.
		if c.Person != nil {
			_ = pool.QueryRow(ctx, `
				SELECT count(*) FROM core.declaration_item i
				JOIN core.declaration d ON d.id=i.declaration_id
				WHERE d.person_id=$1`, c.Person.ID).Scan(&k.NbInterets)
		}
		if k.NbInterets > 0 {
			k.Interets = Angle{Present: true}
			st.AvecInterets++
		} else {
			k.Interets = Angle{Raison: "aucune déclaration HATVP rattachée"}
		}

		st.Candidats = append(st.Candidats, k)
	}
	return st, nil
}
