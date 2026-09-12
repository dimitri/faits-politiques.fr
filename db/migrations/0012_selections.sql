-- +goose Up
-- Un dossier est un FILTRE, pas un texte.
--
-- Il n'y a aucune prose dans ce produit : un dossier est une liste de faits réunis
-- pour être relus facilement dans un contexte donné, sans avoir à consulter tout le
-- corpus. C'est de la donnée structurée, donc il peut être créé et partagé sans
-- compte, exactement comme une carte de rattachement.
--
-- Mais le biais ne disparaît pas : il se déplace. Sans prose, LE FILTRE EST
-- L'ARGUMENT — retenir 12 scrutins sur 900 est un acte rhétorique même sans un mot
-- de commentaire. La garantie centrale n'est donc pas « toute affirmation est citée »
-- mais « toute sélection déclare sa sélectivité ».

CREATE SCHEMA selection;
COMMENT ON SCHEMA selection IS
  'Filtres nommés sur les faits. Aucune prose. Ne référence que core et derived : '
  'aucune clé étrangère ne doit remonter de core ou derived vers ici.';

-- Un gabarit est un filtre PARAMÉTRÉ par sujet. La symétrie entre organisations
-- devient une propriété de construction — une définition unique instanciée pour tous
-- les sujets — au lieu d'une discipline à tenir page par page.
CREATE TABLE selection.template (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug         text NOT NULL UNIQUE,
  label        text NOT NULL,
  subject_kind text NOT NULL CHECK (subject_kind IN
                 ('PARLIAMENTARY_GROUP','PARTY','NUANCE','COMMUNE','PERSON','TOPIC')),
  criteria     jsonb NOT NULL,      -- critères, avec un emplacement pour le sujet
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TYPE selection.pick_mode AS ENUM (
  'CRITERIA',        -- le filtre EST sa définition : reproductible, se met à jour seul
  'MANUAL_SUBSET'    -- choix explicite à l'intérieur d'un univers déclaré
);

CREATE TABLE selection.dossier (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug                text NOT NULL UNIQUE,
  label               text NOT NULL,
  template_id         bigint REFERENCES selection.template(id),
  subject_key         text,
  -- L'univers est TOUJOURS déclaré, même quand la sélection est manuelle : sans lui,
  -- la sélectivité n'est pas calculable et « 12 faits » ne veut rien dire.
  universe_criteria   jsonb NOT NULL,
  pick_mode           selection.pick_mode NOT NULL,
  mapping_revision_id bigint REFERENCES core.mapping_revision(id),
  -- Même mécanisme que pour les cartes : édition par jeton de capacité, seul le haché
  -- est stocké. Aucun compte, aucune identité, aucun profil d'opinion.
  edit_token_hash     bytea,
  listed              boolean NOT NULL DEFAULT false,
  content_hash        bytea,
  frozen_at           timestamptz,
  created_at          timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT edit_token_is_32_bytes
    CHECK (edit_token_hash IS NULL OR octet_length(edit_token_hash) = 32),
  CONSTRAINT gel_coherent CHECK ((frozen_at IS NULL) = (content_hash IS NULL)),
  CONSTRAINT sujet_avec_gabarit CHECK ((template_id IS NULL) = (subject_key IS NULL))
);
CREATE INDEX dossier_template_idx ON selection.dossier (template_id, subject_key);
CREATE INDEX dossier_listed_idx   ON selection.dossier (listed) WHERE listed;

-- Les faits retenus à la main. N'existent qu'en mode MANUAL_SUBSET.
CREATE TABLE selection.dossier_item (
  id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dossier_id           bigint NOT NULL REFERENCES selection.dossier(id) ON DELETE CASCADE,
  ordinal              int NOT NULL,
  scrutin_id           bigint REFERENCES core.scrutin(id),
  amendement_id        bigint REFERENCES core.amendement(id),
  intervention_id      bigint REFERENCES core.intervention(id),
  commune_indicator_id bigint REFERENCES core.commune_indicator(id),
  municipal_tariff_id  bigint REFERENCES core.municipal_tariff(id),
  public_contract_id   bigint REFERENCES core.public_contract(id),
  stat_query_id        bigint REFERENCES derived.stat_query(id),
  comparison_run_id    bigint REFERENCES derived.comparison_run(id),
  UNIQUE (dossier_id, ordinal),
  CONSTRAINT item_designe_exactement_un_fait CHECK (
    num_nonnulls(scrutin_id, amendement_id, intervention_id, commune_indicator_id,
                 municipal_tariff_id, public_contract_id, stat_query_id,
                 comparison_run_id) = 1
  )
);
CREATE INDEX dossier_item_dossier_idx ON selection.dossier_item (dossier_id);

-- +goose StatementBegin
CREATE FUNCTION selection.assert_items_only_if_manual() RETURNS trigger
LANGUAGE plpgsql AS $fn$
DECLARE v_mode selection.pick_mode;
BEGIN
  SELECT pick_mode INTO v_mode FROM selection.dossier WHERE id = NEW.dossier_id;
  IF v_mode <> 'MANUAL_SUBSET' THEN
    RAISE EXCEPTION
      'Dossier en mode % : il se définit par ses critères, pas par une liste de faits.', v_mode;
  END IF;
  RETURN NEW;
END;
$fn$;
-- +goose StatementEnd

CREATE TRIGGER dossier_item_reserve_au_mode_manuel
  BEFORE INSERT OR UPDATE ON selection.dossier_item
  FOR EACH ROW EXECUTE FUNCTION selection.assert_items_only_if_manual();

-- Sélectivité : calculée par le système, jamais saisie par l'auteur, non supprimable.
-- C'est ce qui remplace l'obligation de citation. Un lecteur doit voir « 12 faits
-- retenus sur 214 éligibles » sans que personne n'ait eu à l'écrire.
CREATE TABLE selection.dossier_selectivity (
  dossier_id          bigint PRIMARY KEY REFERENCES selection.dossier(id) ON DELETE CASCADE,
  universe_size       int NOT NULL CHECK (universe_size >= 0),
  selected_size       int NOT NULL CHECK (selected_size >= 0),
  exclusion_breakdown jsonb NOT NULL DEFAULT '{}'::jsonb,  -- par ref.unverifiable_reason
  method_version      text NOT NULL,
  computed_at         timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT selection_incluse_dans_univers CHECK (selected_size <= universe_size)
);

-- Un dossier ne peut être ni gelé ni listé sans sélectivité calculée : le pendant,
-- pour les filtres, du refus de publier une affirmation non citée.
-- +goose StatementBegin
CREATE FUNCTION selection.assert_selectivity_computed() RETURNS trigger
LANGUAGE plpgsql AS $fn$
BEGIN
  IF NEW.frozen_at IS NULL AND NOT NEW.listed THEN RETURN NEW; END IF;
  IF NOT EXISTS (SELECT 1 FROM selection.dossier_selectivity WHERE dossier_id = NEW.id) THEN
    RAISE EXCEPTION
      'Dossier % : sélectivité non calculée. Un filtre ne se publie pas sans déclarer ce qu''il écarte.',
      NEW.slug;
  END IF;
  RETURN NEW;
END;
$fn$;
-- +goose StatementEnd

CREATE TRIGGER dossier_publication_exige_selectivite
  BEFORE INSERT OR UPDATE ON selection.dossier
  FOR EACH ROW EXECUTE FUNCTION selection.assert_selectivity_computed();

-- Rend l'asymétrie visible : pour un gabarit donné, quels sujets ont un filtre et
-- lesquels n'en ont pas. Une page qui n'existerait que pour une organisation saute
-- aux yeux ici.
CREATE VIEW selection.template_coverage AS
SELECT t.slug AS template_slug, t.subject_kind,
       count(d.id)                                     AS instances,
       count(*) FILTER (WHERE d.listed)                AS listees,
       count(*) FILTER (WHERE d.frozen_at IS NOT NULL) AS gelees
FROM selection.template t
LEFT JOIN selection.dossier d ON d.template_id = t.id
GROUP BY t.slug, t.subject_kind;

-- +goose Down
DROP VIEW selection.template_coverage;
DROP TRIGGER dossier_publication_exige_selectivite ON selection.dossier;
DROP FUNCTION selection.assert_selectivity_computed();
DROP TABLE selection.dossier_selectivity;
DROP TRIGGER dossier_item_reserve_au_mode_manuel ON selection.dossier_item;
DROP FUNCTION selection.assert_items_only_if_manual();
DROP TABLE selection.dossier_item;
DROP TABLE selection.dossier;
DROP TYPE selection.pick_mode;
DROP TABLE selection.template;
DROP SCHEMA selection;
