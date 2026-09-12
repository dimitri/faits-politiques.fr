package an

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type scrutin struct {
	UID         flexStr `json:"uid"`
	Numero      flexStr `json:"numero"`
	Legislature flexStr `json:"legislature"`
	DateScrutin flexStr `json:"dateScrutin"`
	TypeVote    struct {
		CodeTypeVote    flexStr `json:"codeTypeVote"`
		LibelleTypeVote flexStr `json:"libelleTypeVote"`
	} `json:"typeVote"`
	Sort struct {
		Code flexStr `json:"code"`
	} `json:"sort"`
	Titre string `json:"titre"`
	Objet struct {
		Libelle string `json:"libelle"`
	} `json:"objet"`
	ModePublicationDesVotes flexStr `json:"modePublicationDesVotes"`
	SyntheseVote            struct {
		NombreVotants flexStr `json:"nombreVotants"`
		Decompte      struct {
			Pour        flexStr `json:"pour"`
			Contre      flexStr `json:"contre"`
			Abstentions flexStr `json:"abstentions"`
		} `json:"decompte"`
	} `json:"syntheseVote"`
	VentilationVotes struct {
		Organe struct {
			Groupes struct {
				Groupe json.RawMessage `json:"groupe"`
			} `json:"groupes"`
		} `json:"organe"`
	} `json:"ventilationVotes"`
	MiseAuPoint json.RawMessage `json:"miseAuPoint"`
}

type groupeVote struct {
	OrganeRef flexStr `json:"organeRef"`
	Vote      struct {
		PositionMajoritaire flexStr                    `json:"positionMajoritaire"`
		DecompteNominatif   map[string]json.RawMessage `json:"decompteNominatif"`
		DecompteVoix        map[string]flexStr         `json:"decompteVoix"`
	} `json:"vote"`
}

type votant struct {
	ActeurRef     flexStr `json:"acteurRef"`
	ParDelegation flexStr `json:"parDelegation"`
}

// Correspondance entre les catégories du décompte AN et les positions du modèle.
// ABSENT et NON_VOTING ne sont pas des positions politiques : ce sont des
// données manquantes, et aucun calcul de proximité ne doit les traiter autrement.
var positionOf = map[string]string{
	"pours":                 "FOR",
	"contres":               "AGAINST",
	"abstentions":           "ABSTAIN",
	"nonVotants":            "ABSENT",
	"nonVotantsVolontaires": "NON_VOTING",
}

func votantsIn(raw json.RawMessage) []votant {
	if len(raw) == 0 {
		return nil
	}
	var box struct {
		Votant json.RawMessage `json:"votant"`
	}
	if err := json.Unmarshal(raw, &box); err != nil {
		return nil
	}
	var out []votant
	for _, e := range asSlice(box.Votant) {
		var v votant
		if err := json.Unmarshal(e, &v); err == nil && v.ActeurRef != "" {
			out = append(out, v)
		}
	}
	return out
}

type ballotRow struct {
	scrutinID  int64
	personID   int64
	orgID      *int64
	position   string
	delegation bool
	rectifiee  *string
}

func normalizeScrutins(ctx context.Context, pool *pgxpool.Pool,
	personByUID map[string]int64, orgByUID map[string]int64) (int, int, error) {

	var legID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO core.legislature (institution, numero, validity)
		VALUES ('ASSEMBLEE_NATIONALE', 17, daterange('2024-07-07', NULL))
		ON CONFLICT (institution, numero) DO UPDATE SET numero = EXCLUDED.numero
		RETURNING id`).Scan(&legID); err != nil {
		return 0, 0, err
	}

	rows, err := pool.Query(ctx,
		`SELECT DISTINCT ON (natural_key) payload FROM raw.record
		  WHERE record_type = 'an.scrutin'
		  ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return 0, 0, err
	}
	var all []scrutin
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return 0, 0, err
		}
		var s scrutin
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		all = append(all, s)
	}
	rows.Close()

	var ballots []ballotRow
	nScrutins := 0

	for _, s := range all {
		// La granularité décrit ce que la SOURCE publie, jamais ce qu'on
		// aimerait avoir (docs/perimetre.md §2.4).
		gran := "GROUP"
		if strings.EqualFold(s.ModePublicationDesVotes.String(), "DecompteNominatif") {
			gran = "INDIVIDUAL"
		}
		sort := strings.ToUpper(s.Sort.Code.String())
		switch sort {
		case "ADOPTÉ", "ADOPTE":
			sort = "ADOPTE"
		case "REJETÉ", "REJETE":
			sort = "REJETE"
		default:
			sort = ""
		}
		objet := s.Objet.Libelle
		if objet == "" {
			objet = s.Titre
		}

		var sid int64
		err := pool.QueryRow(ctx, `
			INSERT INTO core.scrutin
			  (slug, institution, legislature_id, source_uid, numero, date_seance,
			   granularite, objet, type_vote, resultat, nb_votants, nb_pour, nb_contre, nb_abstentions)
			VALUES ($1,'ASSEMBLEE_NATIONALE',$2,$3,$4,$5::date,$6,$7,$8,NULLIF($9,''),$10,$11,$12,$13)
			ON CONFLICT (institution, source_uid) DO UPDATE SET objet = EXCLUDED.objet
			RETURNING id`,
			"an-17-"+s.Numero.String(), legID, s.UID.String(), s.Numero.String(),
			s.DateScrutin.String(), gran, objet, s.TypeVote.LibelleTypeVote.String(), sort,
			s.SyntheseVote.NombreVotants.Int(), s.SyntheseVote.Decompte.Pour.Int(),
			s.SyntheseVote.Decompte.Contre.Int(), s.SyntheseVote.Decompte.Abstentions.Int(),
		).Scan(&sid)
		if err != nil {
			return 0, 0, fmt.Errorf("scrutin %s : %w", s.UID, err)
		}
		nScrutins++

		// Mises au point : à l'AN, un député peut corriger son vote APRÈS le
		// scrutin. Cela modifie le relevé nominatif sans modifier le résultat
		// officiel — les deux sont stockés séparément.
		rectif := map[string]string{}
		if len(s.MiseAuPoint) > 0 {
			var mp map[string]json.RawMessage
			if json.Unmarshal(s.MiseAuPoint, &mp) == nil {
				for cat, pos := range positionOf {
					for _, v := range votantsIn(mp[cat]) {
						rectif[v.ActeurRef.String()] = pos
					}
				}
			}
		}

		seen := map[int64]bool{}
		for _, graw := range asSlice(s.VentilationVotes.Organe.Groupes.Groupe) {
			var g groupeVote
			if err := json.Unmarshal(graw, &g); err != nil {
				continue
			}
			if gran == "GROUP" {
				if err := insertGroupBallot(ctx, pool, sid, g, orgByUID); err != nil {
					return 0, 0, err
				}
				continue
			}
			// Le groupe sous lequel la source enregistre ce vote, ce jour-là.
			var orgID *int64
			if id, ok := orgByUID[g.OrganeRef.String()]; ok {
				orgID = &id
			}
			for cat, pos := range positionOf {
				for _, v := range votantsIn(g.Vote.DecompteNominatif[cat]) {
					pid, ok := personByUID[v.ActeurRef.String()]
					if !ok || seen[pid] {
						continue
					}
					seen[pid] = true
					b := ballotRow{
						scrutinID:  sid,
						personID:   pid,
						orgID:      orgID,
						position:   pos,
						delegation: v.ParDelegation.String() == "true",
					}
					if r, ok := rectif[v.ActeurRef.String()]; ok && r != pos {
						b.rectifiee = &r
					}
					ballots = append(ballots, b)
				}
			}
		}
	}

	if _, err := pool.Exec(ctx, `TRUNCATE core.ballot`); err != nil {
		return 0, 0, err
	}
	n, err := pool.CopyFrom(ctx,
		pgx.Identifier{"core", "ballot"},
		[]string{"scrutin_id", "person_id", "organization_id", "position",
			"par_delegation", "position_rectifiee", "rectifiee_le"},
		pgx.CopyFromSlice(len(ballots), func(i int) ([]any, error) {
			b := ballots[i]
			var rect, date any
			if b.rectifiee != nil {
				rect = *b.rectifiee
				date = "1970-01-01" // date de mise au point non publiée dans ce flux
			}
			var org any
			if b.orgID != nil {
				org = *b.orgID
			}
			return []any{b.scrutinID, b.personID, org, b.position, b.delegation, rect, date}, nil
		}))
	return nScrutins, int(n), err
}

func insertGroupBallot(ctx context.Context, pool *pgxpool.Pool, sid int64,
	g groupeVote, orgByUID map[string]int64) error {
	orgID, ok := orgByUID[g.OrganeRef.String()]
	if !ok {
		return nil
	}
	pos := map[string]string{
		"pour": "FOR", "contre": "AGAINST", "abstention": "ABSTAIN",
	}[strings.ToLower(g.Vote.PositionMajoritaire.String())]
	if pos == "" {
		pos = "NON_VOTING"
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO core.ballot_group
		  (scrutin_id, organization_id, position, nb_pour, nb_contre, nb_abstentions)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
		sid, orgID, pos,
		g.Vote.DecompteVoix["pour"].Int(),
		g.Vote.DecompteVoix["contre"].Int(),
		g.Vote.DecompteVoix["abstentions"].Int())
	return err
}
