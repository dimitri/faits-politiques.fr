-- +goose Up
-- Les déclarations à la Haute Autorité pour la transparence de la vie publique.
--
-- La HATVP publie en open data 13 687 déclarants et leurs déclarations : les
-- déclarations d'intérêts (DI, DIA et modificatives) et les déclarations de
-- situation patrimoniale (DSP). C'est la seule source publique qui donne le
-- PARCOURS d'un responsable — activités professionnelles des cinq dernières
-- années, mandats détenus, rémunérations déclarées année par année — là où le
-- RNE n'a aucune profondeur historique (D-025).
--
-- Ce qui n'est pas publié l'est visiblement : la source remplace les champs
-- couverts par le secret (adresse, téléphone, certains montants) par la
-- mention « [Données non publiées] ». Elle est transcrite telle quelle dans
-- `non_publie` plutôt que traduite en NULL : « non publié » et « néant » sont
-- deux faits différents, et les confondre ferait dire à la base ce que la
-- source ne dit pas.
CREATE TABLE core.declaration (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  uuid              text NOT NULL UNIQUE,
  person_id         bigint REFERENCES core.person(id) ON DELETE SET NULL,
  -- Nom, prénom et date de naissance sont conservés même quand la personne est
  -- rapprochée : ils viennent de la source, et servent à vérifier le
  -- rapprochement plutôt qu'à devoir lui faire confiance.
  nom               text NOT NULL,
  prenom            text NOT NULL,
  date_naissance    date,
  type_declaration  text NOT NULL,
  date_depot        timestamptz,
  type_mandat       text,
  label_organe      text,
  qualite           text,
  date_debut_mandat date,
  date_fin_mandat   date,
  source_id         bigint NOT NULL REFERENCES raw.source(id),
  provenance        core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX declaration_person_idx ON core.declaration (person_id)
  WHERE person_id IS NOT NULL;
CREATE INDEX declaration_type_idx ON core.declaration (type_declaration);
CREATE INDEX declaration_nom_idx ON core.declaration (core.f_unaccent(lower(nom)));

COMMENT ON TABLE core.declaration IS
  'Déclaration déposée auprès de la HATVP. DI = intérêts, DSP = situation '
  'patrimoniale, les variantes en A et M sont les versions annexes et '
  'modificatives.';

-- Un seul tableau pour tous les blocs de la déclaration, et non quinze tables.
--
-- Le format de la HATVP compte une quinzaine de blocs pour les intérêts
-- (activités professionnelles, mandats, participations, bénévolat…) et autant
-- pour le patrimoine (immeubles, comptes, assurances-vie, véhicules, passif…).
-- Leur donner chacun sa table exigerait de décider ce qui compte dans chacun,
-- c'est-à-dire d'interpréter. `bloc` porte le nom d'élément de la source, et
-- rien n'est perdu.
CREATE TABLE core.declaration_item (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  declaration_id bigint NOT NULL REFERENCES core.declaration(id) ON DELETE CASCADE,
  bloc           text NOT NULL,
  description    text,
  employeur      text,
  commentaire    text,
  -- Les montants sont publiés par année dans les blocs de rémunération.
  annee          integer,
  montant        numeric,
  -- La source a écrit « [Données non publiées] » là où un montant était attendu.
  non_publie     boolean NOT NULL DEFAULT false
);

CREATE INDEX declaration_item_decl_idx ON core.declaration_item (declaration_id);
CREATE INDEX declaration_item_bloc_idx ON core.declaration_item (bloc);

COMMENT ON COLUMN core.declaration_item.bloc IS
  'Nom d''élément du format HATVP (activProfCinqDerniereDto, mandatElectifDto, '
  'immeubleDto, comptesBancaireDto…), conservé sans regroupement.';

-- Le dirigeant d'un groupement, publié par BANATIC avec le reste de sa fiche.
-- C'est la réponse à « qui décide, si ce n'est le maire » : le président d'un
-- EPCI arbitre des budgets souvent supérieurs à ceux de ses communes membres,
-- et n'est élu par personne directement.
ALTER TABLE core.epci
  ADD COLUMN president_civilite text,
  ADD COLUMN president_nom      text,
  ADD COLUMN president_prenom   text,
  ADD COLUMN nb_delegues        integer;

-- +goose Down
ALTER TABLE core.epci
  DROP COLUMN president_civilite, DROP COLUMN president_nom,
  DROP COLUMN president_prenom, DROP COLUMN nb_delegues;
DROP TABLE core.declaration_item;
DROP TABLE core.declaration;
