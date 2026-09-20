package sitegen

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Le Sénat publie autrement que l'Assemblée, et la page doit le dire avant
// toute chose : le dump Dosleg donne la date, l'objet, les décomptes et le
// relevé NOMINATIF de chaque scrutin, mais ni résultat officiel, ni type de
// vote, ni groupe des votants, ni référence vers le texte débattu.
//
// C'est l'inverse de ce que le site annonçait jusqu'ici (« les scrutins y sont
// publiés par groupe, pas par sénateur ») : les votes sont individuels, et
// c'est le GROUPE qui manque.
type StatsSenat struct {
	Scrutins, Votes, Senateurs int
	Debut, Fin                 string
	Derniers                   []FluxLigne
	Senateurs2                 []*Person
}

// loadSenat lit mv.scrutin_vote_nominal (internal/matview) au lieu de
// core.ballot directement — le JOIN sur mv.scrutin (institution) reste
// applicatif, mais porte sur une table de quelques dizaines de milliers de
// lignes, pas sur le fait 4,9 millions de lignes.
func loadSenat(ctx context.Context, pool *pgxpool.Pool, persons map[string]*Person) (*StatsSenat, error) {
	st := &StatsSenat{}
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM mv.scrutin WHERE institution='SENAT'),
		       (SELECT count(*) FROM mv.scrutin_vote_nominal mv JOIN mv.scrutin s ON s.id=mv.scrutin_id
		         WHERE s.institution='SENAT'),
		       (SELECT count(DISTINCT mv.person_slug) FROM mv.scrutin_vote_nominal mv
		         JOIN mv.scrutin s ON s.id=mv.scrutin_id WHERE s.institution='SENAT'),
		       coalesce((SELECT to_char(min(date_seance),'DD/MM/YYYY') FROM mv.scrutin
		         WHERE institution='SENAT'),''),
		       coalesce((SELECT to_char(max(date_seance),'DD/MM/YYYY') FROM mv.scrutin
		         WHERE institution='SENAT'),'')`).
		Scan(&st.Scrutins, &st.Votes, &st.Senateurs, &st.Debut, &st.Fin); err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT slug, objet, to_char(date_seance,'DD/MM/YYYY'),
		       coalesce(nb_pour,0), coalesce(nb_contre,0), coalesce(nb_abstentions,0)
		FROM mv.scrutin WHERE institution='SENAT'
		ORDER BY date_seance DESC, numero DESC LIMIT 40`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var f FluxLigne
		if err := rows.Scan(&f.Slug, &f.Objet, &f.Date, &f.Pour, &f.Contre, &f.Abstentions); err != nil {
			rows.Close()
			return nil, err
		}
		f.Objet, _ = TitreCourt(f.Objet)
		f.Exprimes = f.Pour + f.Contre + f.Abstentions
		// Le Sénat ne publie pas de résultat : on ne le déduit pas du décompte.
		f.Resultat = ""
		st.Derniers = append(st.Derniers, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	srows, err := pool.Query(ctx, `
		SELECT DISTINCT mv.person_slug FROM mv.scrutin_vote_nominal mv
		JOIN mv.scrutin s ON s.id = mv.scrutin_id
		WHERE s.institution='SENAT'`)
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var slug string
		if err := srows.Scan(&slug); err != nil {
			return nil, err
		}
		if p, ok := persons[slug]; ok {
			st.Senateurs2 = append(st.Senateurs2, p)
		}
	}
	trierPersonnes(st.Senateurs2)
	return st, srows.Err()
}
