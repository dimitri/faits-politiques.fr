-- +goose Up
-- Les élections municipales : transcrire ce que la source publie.
--
-- La nuance politique ne figure PAS au Répertoire national des élus (D-022) :
-- le fichier des maires porte quatorze colonnes, aucune politique. Elle est
-- attribuée par les préfectures aux LISTES CANDIDATES, et publiée uniquement
-- dans les fichiers de résultats du ministère de l'Intérieur.
--
-- D'où cette table : une ligne par liste candidate dans une commune, à un tour
-- donné, telle que le ministère la publie. Rien n'y est calculé. La « couleur »
-- d'une commune, elle, est une déduction — elle vit dans derived (voir plus
-- bas), jamais ici.
-- Nomenclature des nuances, millésime 2026.
--
-- Les 25 codes ci-dessous sont EXACTEMENT ceux présents dans les fichiers des
-- 15 et 22 mars 2026 : la liste est relevée dans les données, pas devinée. Les
-- libellés sont les développements usuels de la nomenclature préfectorale.
--
-- `famille` reste NULL, et ce n'est pas un oubli : regrouper LDVD, LLR et LUD
-- sous « la droite » serait une décision éditoriale. Elle a sa place dans
-- core.nuance_party_link, avec son motif et son drapeau `aggregatable`, pas
-- dans une nomenclature de référence.
INSERT INTO ref.nuance_politique (code, circulaire_millesime, libelle) VALUES
  ('LEXG', 2026, 'Liste d''extrême gauche'),
  ('LCOM', 2026, 'Liste du Parti communiste français'),
  ('LFI',  2026, 'Liste La France insoumise'),
  ('LSOC', 2026, 'Liste du Parti socialiste'),
  ('LVEC', 2026, 'Liste Les Écologistes'),
  ('LECO', 2026, 'Liste écologiste'),
  ('LDVG', 2026, 'Liste divers gauche'),
  ('LUG',  2026, 'Liste d''union de la gauche'),
  ('LDVC', 2026, 'Liste divers centre'),
  ('LUC',  2026, 'Liste d''union du centre'),
  ('LMDM', 2026, 'Liste du Mouvement démocrate'),
  ('LREN', 2026, 'Liste Renaissance'),
  ('LHOR', 2026, 'Liste Horizons'),
  ('LUDI', 2026, 'Liste de l''Union des démocrates et indépendants'),
  ('LLR',  2026, 'Liste Les Républicains'),
  ('LDVD', 2026, 'Liste divers droite'),
  ('LUD',  2026, 'Liste d''union de la droite'),
  ('LUDR', 2026, 'Liste de l''Union des droites pour la République'),
  ('LRN',  2026, 'Liste du Rassemblement National'),
  ('LREC', 2026, 'Liste Reconquête'),
  ('LUXD', 2026, 'Liste d''union de l''extrême droite'),
  ('LEXD', 2026, 'Liste d''extrême droite'),
  ('LDSV', 2026, 'Liste divers souverainiste'),
  ('LREG', 2026, 'Liste régionaliste'),
  ('LDIV', 2026, 'Liste divers')
ON CONFLICT (code, circulaire_millesime) DO NOTHING;

CREATE TABLE core.municipal_list (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  scrutin_annee  integer NOT NULL,
  tour           smallint NOT NULL CHECK (tour IN (1, 2)),
  commune_code   text NOT NULL,
  cog_millesime  integer NOT NULL,
  -- Numéro de panneau : l'ordre officiel de la liste sur le document. C'est la
  -- seule clé stable dont on dispose pour distinguer deux listes d'une commune.
  panneau        integer NOT NULL,
  nuance_code    text,
  circulaire_millesime integer,
  libelle        text NOT NULL,
  nom_candidat   text,
  prenom_candidat text,
  voix           integer,
  sieges_cm      integer,
  sieges_cc      integer,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  provenance     core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (scrutin_annee, tour, commune_code, panneau),
  FOREIGN KEY (commune_code, cog_millesime)
    REFERENCES ref.commune(code_insee, cog_millesime),
  FOREIGN KEY (nuance_code, circulaire_millesime)
    REFERENCES ref.nuance_politique(code, circulaire_millesime),
  -- Une nuance n'existe qu'avec son millésime de circulaire : la nomenclature
  -- change à chaque scrutin, et un code sans millésime ne veut rien dire.
  CONSTRAINT municipal_list_nuance_millesimee
    CHECK ((nuance_code IS NULL) = (circulaire_millesime IS NULL))
);

CREATE INDEX municipal_list_commune_idx ON core.municipal_list (commune_code, scrutin_annee);
CREATE INDEX municipal_list_nuance_idx ON core.municipal_list (nuance_code)
  WHERE nuance_code IS NOT NULL;

COMMENT ON TABLE core.municipal_list IS
  'Listes candidates aux municipales, transcrites des fichiers du ministère de '
  'l''Intérieur. La nuance qualifie la LISTE, jamais une personne.';

-- La couleur d'une commune est une DÉDUCTION, pas une donnée publiée.
--
--   « la couleur d'une commune est la nuance de la liste ayant obtenu le plus
--     de sièges au conseil municipal, au tour où le conseil a été pourvu »
--
-- Vérifiée sur deux cas connus : Nice donne LUXD (liste Ciotti, 52 sièges
-- contre 13), Perpignan donne LRN (liste Aliot, 43 sièges).
--
-- Elle est reproductible mais elle reste une décision, et elle échoue là où le
-- source ne tranche pas : ex aequo en sièges, ou aucune liste nuancée. Ces cas
-- sont marqués, jamais devinés.
CREATE TABLE derived.commune_couleur (
  commune_code   text NOT NULL,
  scrutin_annee  integer NOT NULL,
  nuance_code    text,
  circulaire_millesime integer,
  -- Le tour où le conseil a été pourvu, et la liste retenue.
  tour           smallint NOT NULL,
  municipal_list_id bigint REFERENCES core.municipal_list(id) ON DELETE CASCADE,
  sieges_cm      integer,
  -- NUANCEE       une liste nuancée est arrivée en tête en sièges
  -- SANS_NUANCE   la commune est sous le seuil : aucune liste n'a de nuance
  -- EX_AEQUO      deux listes à égalité de sièges, la règle ne tranche pas
  statut         text NOT NULL CHECK (statut IN ('NUANCEE', 'SANS_NUANCE', 'EX_AEQUO')),
  method_version text NOT NULL,
  computed_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (commune_code, scrutin_annee),
  CONSTRAINT commune_couleur_nuance_ssi_nuancee
    CHECK ((statut = 'NUANCEE') = (nuance_code IS NOT NULL))
);

CREATE INDEX commune_couleur_nuance_idx ON derived.commune_couleur (nuance_code)
  WHERE nuance_code IS NOT NULL;

COMMENT ON TABLE derived.commune_couleur IS
  'Déduction : couleur d''une commune = nuance de la liste majoritaire en '
  'sièges. Recalculable. Le statut dit explicitement quand la règle ne tranche '
  'pas, plutôt que de laisser croire à une absence de donnée.';

-- Population totale publiée par l'OFGL avec chaque compte. Ce n'est PAS la
-- population municipale de l'INSEE : indicateur distinct, libellé distinct.
INSERT INTO ref.indicator (code, label, unit, producer, caveat, comparable) VALUES
  ('ofgl.population_totale', 'Population totale retenue par l''OFGL', 'HABITANTS',
   'Observatoire des finances et de la gestion publique locales',
   'Population de référence utilisée par l''OFGL pour ses ratios par habitant. '
   'Distincte de la population municipale de l''INSEE.', true)
ON CONFLICT (code) DO NOTHING;

-- +goose Down
DROP TABLE derived.commune_couleur;
DROP TABLE core.municipal_list;
DELETE FROM ref.indicator WHERE code = 'ofgl.population_totale';
DELETE FROM ref.nuance_politique WHERE circulaire_millesime = 2026;
