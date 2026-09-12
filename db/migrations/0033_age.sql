-- +goose Up
-- L'âge d'une personne, calculé et non stocké.
--
-- Un âge n'est pas une donnée : c'est une différence entre une date de
-- naissance et un instant. Le stocker obligerait à le recalculer, et une fiche
-- publiée il y a huit mois annoncerait un âge faux. La vue le recalcule à
-- chaque lecture, donc à chaque génération du site — c'est exactement ce que
-- veut dire « son âge au moment de la publication ».
--
-- Les personnes sans date de naissance renvoient NULL, jamais zéro ni une
-- estimation. Une fiche doit pouvoir dire « date de naissance non publiée »
-- plutôt qu'afficher un âge inventé.
CREATE VIEW core.personne_age AS
  SELECT p.id AS person_id,
         p.slug,
         p.given_name,
         p.family_name,
         p.birth_date,
         CASE WHEN p.birth_date IS NOT NULL
              -- age() puis extract(year) : la soustraction de dates donnerait
              -- des jours, et diviser par 365,25 se trompe d'un an autour de
              -- l'anniversaire.
              THEN extract(year FROM age(CURRENT_DATE, p.birth_date))::integer
         END AS age,
         -- Le jour où l'âge affiché deviendra faux. Utile pour dater une page
         -- statique : au-delà, la fiche doit être régénérée.
         CASE WHEN p.birth_date IS NOT NULL
              THEN (p.birth_date
                    + ((extract(year FROM age(CURRENT_DATE, p.birth_date))::integer + 1)
                       * interval '1 year'))::date
         END AS age_valable_jusqu_au
    FROM core.person p;

COMMENT ON VIEW core.personne_age IS
  'Âge recalculé à chaque lecture. NULL quand la date de naissance n''est pas '
  'publiée : une fiche doit dire qu''elle ne sait pas, pas inventer.';

-- +goose Down
DROP VIEW core.personne_age;
