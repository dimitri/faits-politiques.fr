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

	// Granularité/résultat/objet calculés une fois par scrutin, en Go, avant
	// toute écriture — un seul aller-retour COPY + INSERT...SELECT...
	// RETURNING pour les 8 000+ scrutins d'une législature plutôt qu'un
	// QueryRow chacun (le blocage que ce commentaire répare : voir la
	// session qui l'a introduit). Aucune contrainte d'exclusion sur
	// core.scrutin — (institution, source_uid) est une clé unique ordinaire
	// — donc aucune dépendance à l'ordre des lignes, contrairement à
	// copierMandats plus haut dans ce paquet.
	lignes := make([]ligneScrutin, 0, len(all))
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
		lignes = append(lignes, ligneScrutin{
			uid: s.UID.String(), slug: "an-17-" + s.Numero.String(), numero: s.Numero.String(),
			date: s.DateScrutin.String(), gran: gran, objet: objet,
			typeVote: s.TypeVote.LibelleTypeVote.String(), resultat: sort,
			votants: s.SyntheseVote.NombreVotants.Int(), pour: s.SyntheseVote.Decompte.Pour.Int(),
			contre: s.SyntheseVote.Decompte.Contre.Int(), abstentions: s.SyntheseVote.Decompte.Abstentions.Int(),
		})
	}

	sidByUID, err := copierScrutins(ctx, pool, legID, lignes)
	if err != nil {
		return 0, 0, err
	}
	nScrutins := len(sidByUID)

	var ballots []ballotRow
	for _, s := range all {
		sid, ok := sidByUID[s.UID.String()]
		if !ok {
			continue // anomalie déjà signalée par copierScrutins, jamais ici en double
		}
		gran := "GROUP"
		if strings.EqualFold(s.ModePublicationDesVotes.String(), "DecompteNominatif") {
			gran = "INDIVIDUAL"
		}

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

	// TRUNCATE core.ballot était écrit ici, sans portée. Il a détruit les
	// 1 970 025 votes du Parlement européen — quatrième fois qu'une remise à
	// zéro déborde son périmètre dans ce projet, et la première où un TRUNCATE
	// subsistait alors que le connecteur voisin porte un commentaire
	// expliquant pourquoi il n'en faut pas.
	//
	// Un connecteur ne détruit QUE ce qu'il produit. La portée est ici celle de
	// l'Assemblée, et elle est exprimée par une jointure sur l'institution du
	// scrutin, jamais par la table entière.
	if _, err := pool.Exec(ctx, `
		DELETE FROM core.ballot b USING core.scrutin s
		 WHERE s.id = b.scrutin_id AND s.institution = 'ASSEMBLEE_NATIONALE'`); err != nil {
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

// ligneScrutin : une ligne prête pour tmp_scrutin (copierScrutins).
type ligneScrutin struct {
	uid, slug, numero, date, gran, objet, typeVote, resultat string
	votants, pour, contre, abstentions                       int
}

// copierScrutins upserte tous les scrutins d'un coup — une table temporaire,
// une copie, un WITH...INSERT...RETURNING joint sur cette même table pour
// retrouver l'id attribué à chaque source_uid, exactement le patron déjà
// suivi par normalizeOrganes (internal/an/normalize.go) pour la même raison.
func copierScrutins(ctx context.Context, pool *pgxpool.Pool, legID int64, lignes []ligneScrutin) (map[string]int64, error) {
	if len(lignes) == 0 {
		return map[string]int64{}, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_scrutin (
			uid text, slug text, numero text, date_seance text, gran text, objet text,
			type_vote text, resultat text, votants int, pour int, contre int, abstentions int
		) ON COMMIT DROP`); err != nil {
		return nil, err
	}
	rows := make([][]any, len(lignes))
	for i, l := range lignes {
		rows[i] = []any{l.uid, l.slug, l.numero, l.date, l.gran, l.objet, l.typeVote,
			nullable(l.resultat), l.votants, l.pour, l.contre, l.abstentions}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_scrutin"},
		[]string{"uid", "slug", "numero", "date_seance", "gran", "objet", "type_vote",
			"resultat", "votants", "pour", "contre", "abstentions"},
		pgx.CopyFromRows(rows)); err != nil {
		return nil, err
	}
	res, err := tx.Query(ctx, `
		WITH upsert AS (
			INSERT INTO core.scrutin
			  (slug, institution, legislature_id, source_uid, numero, date_seance,
			   granularite, objet, type_vote, resultat, nb_votants, nb_pour, nb_contre, nb_abstentions)
			SELECT slug, 'ASSEMBLEE_NATIONALE'::core.institution, $1, uid, numero, date_seance::date,
			       gran::core.scrutin_granularite, objet, type_vote, resultat, votants, pour, contre, abstentions
			  FROM tmp_scrutin
			ON CONFLICT (institution, source_uid) DO UPDATE SET objet = EXCLUDED.objet
			RETURNING id, source_uid
		)
		SELECT source_uid, id FROM upsert`, legID)
	if err != nil {
		return nil, fmt.Errorf("scrutins : %w", err)
	}
	sidByUID := map[string]int64{}
	for res.Next() {
		var uid string
		var id int64
		if err := res.Scan(&uid, &id); err != nil {
			res.Close()
			return nil, err
		}
		sidByUID[uid] = id
	}
	res.Close()
	if err := res.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return sidByUID, nil
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
