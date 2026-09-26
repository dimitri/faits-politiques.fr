-- +goose Up

-- mv.person_dernier_vote : les 100 derniers votes de chaque personne, avec
-- de quoi les afficher (objet, date, résultat) sans rejoindre core.scrutin à
-- la construction — ce que loadVotesBulk (internal/sitegen/load.go)
-- calculait par une fenêtre SQL sur la totalité de core.ballot/core.scrutin
-- à chaque construction (déjà optimisé par rapport à une requête par
-- personne, mais toujours un passage complet sur le fait brut).
--
-- 100, pas les 60 que la page affiche aujourd'hui (voir loadVotesBulk,
-- limit) : une marge, pour qu'une page qui voudrait en montrer un peu plus
-- demain n'ait pas besoin d'une nouvelle migration.
CREATE MATERIALIZED VIEW mv.person_dernier_vote AS
  SELECT person_id, rang, scrutin_slug, objet, date_txt, position, rectifiee, resultat
    FROM (
      SELECT b.person_id,
             row_number() OVER (PARTITION BY b.person_id
                                 ORDER BY s.date_seance DESC, s.numero DESC) AS rang,
             s.slug                                              AS scrutin_slug,
             s.objet,
             to_char(s.date_seance,'DD/MM/YYYY')                  AS date_txt,
             coalesce(b.position_rectifiee, b.position)::text      AS position,
             b.position_rectifiee IS NOT NULL                      AS rectifiee,
             coalesce(s.resultat,'')                                AS resultat
        FROM core.ballot b
        JOIN core.scrutin s ON s.id = b.scrutin_id
    ) x
   WHERE rang <= 100;

CREATE UNIQUE INDEX person_dernier_vote_pk ON mv.person_dernier_vote (person_id, rang);

COMMENT ON MATERIALIZED VIEW mv.person_dernier_vote IS
  'Les 100 derniers votes de chaque personne, prêts à afficher — remplace '
  'la fenêtre SQL sur ballot/scrutin que loadVotesBulk (internal/sitegen/'
  'load.go) refaisait à chaque construction. Rafraîchie par '
  'internal/matview, jamais par une requête applicative.';

-- +goose Down
DROP MATERIALIZED VIEW mv.person_dernier_vote;
