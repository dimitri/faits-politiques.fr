-- +goose Up
-- La date de décès, et ce qu'elle change à l'âge.
--
-- Sans elle, la vue core.personne_age fait vieillir les morts : un sénateur né
-- en 1930 et décédé en 2001 se verrait attribuer 96 ans. L'âge d'une personne
-- décédée est son âge AU DÉCÈS, et il ne bouge plus.
--
-- Le Sénat publie la date de décès de ses anciens membres dans ODSEN_GENERAL.
-- C'est la seule source du projet qui le fasse ; les autres fichiers d'élus
-- décrivent des vivants.
ALTER TABLE core.person ADD COLUMN IF NOT EXISTS death_date date;

COMMENT ON COLUMN core.person.death_date IS
  'Date de décès, quand la source la publie. Jamais déduite d''une absence de '
  'mandat : cesser d''être élu n''est pas mourir.';

-- +goose StatementBegin
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'person_deces_apres_naissance') THEN
    ALTER TABLE core.person ADD CONSTRAINT person_deces_apres_naissance
      CHECK (death_date IS NULL OR birth_date IS NULL OR death_date >= birth_date);
  END IF;
END $$;
-- +goose StatementEnd

DROP VIEW IF EXISTS core.personne_age;

CREATE VIEW core.personne_age AS
  SELECT p.id AS person_id,
         p.slug,
         p.given_name,
         p.family_name,
         p.birth_date,
         p.death_date,
         -- L'âge se calcule à la date du décès quand il y en a une, à
         -- aujourd'hui sinon. Un âge qui continue d'augmenter après la mort
         -- serait une erreur visible par n'importe quel lecteur.
         CASE WHEN p.birth_date IS NOT NULL
              THEN extract(year FROM age(coalesce(p.death_date, CURRENT_DATE),
                                         p.birth_date))::integer
         END AS age,
         p.death_date IS NOT NULL AS decedee,
         -- Le jour où l'âge affiché deviendra faux. Pour une personne décédée,
         -- il ne le deviendra jamais : la colonne reste nulle.
         CASE WHEN p.birth_date IS NOT NULL AND p.death_date IS NULL
              THEN (p.birth_date
                    + ((extract(year FROM age(CURRENT_DATE, p.birth_date))::integer + 1)
                       * interval '1 year'))::date
         END AS age_valable_jusqu_au
    FROM core.person p;

COMMENT ON VIEW core.personne_age IS
  'Âge recalculé à chaque lecture : à la date du décès pour une personne '
  'décédée, à aujourd''hui sinon. NULL quand la date de naissance n''est pas '
  'publiée — une fiche doit dire qu''elle ne sait pas, pas inventer.';

-- +goose Down
DROP VIEW core.personne_age;
ALTER TABLE core.person DROP CONSTRAINT IF EXISTS person_deces_apres_naissance;
ALTER TABLE core.person DROP COLUMN IF EXISTS death_date;
