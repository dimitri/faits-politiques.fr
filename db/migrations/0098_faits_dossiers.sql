-- +goose Up
-- Une structure commune à tous les dossiers (D-066) : contexte, enjeux, cadre,
-- contrôles et évaluations, situation chiffrée. Chaque affirmation d'un dossier
-- qui ne sort pas d'une table de données est un fait, rangé ici avec sa
-- section, sa qualité et sa preuve.
--
--   ref.qualite_fait        l'échelle unique de qualité (OFFICIEL, DECLARATIF,
--                           PRESSE), commune aux faits de l'évasion fiscale ;
--   ref.nature_montant      la liste unique des natures de montant ;
--   ref.fait_dossier        les faits de tous les dossiers, reliés à la fiche
--                           de la personne citée quand elle existe ;
--   ref.dossier_terme       les mots par lesquels un dossier est repéré dans les
--                           débats de l'Assemblée nationale ;
--   ref.acteur_numerique    les acteurs français du numérique nommés par le
--                           dossier souveraineté, avec le fondement de leur
--                           rattachement.

CREATE TABLE ref.qualite_fait (
  code    text PRIMARY KEY,
  libelle text NOT NULL,
  sens    text NOT NULL
);

INSERT INTO ref.qualite_fait VALUES
  ('OFFICIEL',   'officiel',   'Établi par une institution (texte, décision, constat d''enquête), ou propos dont une institution atteste qu''il a été tenu.'),
  ('DECLARATIF', 'déclaratif', 'Propos ou chiffre d''une partie (élu en séance, ministre, entreprise, association, responsable administratif) : il est établi que le propos a été tenu, pas qu''il est exact.'),
  ('PRESSE',     'presse',     'Révélé par un média, non confirmé par une institution ; cité, non archivé.');

CREATE TABLE ref.nature_montant (
  code    text PRIMARY KEY,
  libelle text NOT NULL,
  sens    text NOT NULL
);

INSERT INTO ref.nature_montant VALUES
  ('PLAFOND',          'maximum',              'Montant maximal d''un accord-cadre : jamais une dépense.'),
  ('ESTIME',           'estimé',               'Montant prévisionnel d''un marché ou d''un projet.'),
  ('COMMANDE',         'commandé',             'Commandes émises sur un marché, sans preuve de paiement.'),
  ('VENTES',           'ventes',               'Ventes déclarées par un intermédiaire (centrale d''achat).'),
  ('PAYE',             'payé',                 'Somme effectivement versée.'),
  ('DEPENSE',          'dépense',              'Dépense constatée en comptabilité publique.'),
  ('AIDE',             'aide',                 'Aide publique, en nominal ou en équivalent-subvention selon la source.'),
  ('AMENDE',           'amende',               'Sanction pécuniaire.'),
  ('IMPOT',            'impôt',                'Droits réclamés ou payés.'),
  ('CHIFFRE_AFFAIRES', 'chiffre d''affaires',  'Chiffre d''affaires publié.');

-- Les faits de l'évasion fiscale rejoignent l'échelle commune : le communiqué
-- d'une entreprise est une déclaration de partie, comme celui d'une association.
ALTER TABLE ref.fait_multinationale DROP CONSTRAINT fait_multinationale_qualite_check;
ALTER TABLE ref.fait_multinationale DROP CONSTRAINT fait_multinationale_nature_montant_check;
UPDATE ref.fait_multinationale SET qualite = 'DECLARATIF' WHERE qualite = 'ENTREPRISE';
ALTER TABLE ref.fait_multinationale
  ADD CONSTRAINT fait_multinationale_qualite_fkey FOREIGN KEY (qualite) REFERENCES ref.qualite_fait(code),
  ADD CONSTRAINT fait_multinationale_nature_montant_fkey FOREIGN KEY (nature_montant) REFERENCES ref.nature_montant(code);

CREATE TABLE ref.fait_dossier (
  id             text PRIMARY KEY,
  dossier        text NOT NULL,              -- nom du document docs/<dossier>.md
  -- L'ordre commun des dossiers (perimetre.md § 2.8).
  section        text NOT NULL CHECK (section IN ('CONTEXTE','ENJEUX','CADRE','CONTROLE','SITUATION')),
  theme          text,                       -- sous-partie libre, pour grouper à l'affichage
  type           text NOT NULL CHECK (type IN ('TEXTE','CONSTAT','EVALUATION','DECLARATION','CONTRAT','SANCTION',
                                               'RECOMMANDATION','AIDE','DONNEE')),
  date_fait      date,
  auteur         text NOT NULL,              -- institution, ou fonction de la personne à la date
  person_id      bigint REFERENCES core.person(id),  -- la personne citée, pour lier sa fiche
  groupe         text,
  intitule       text NOT NULL,
  montant_eur    numeric,
  nature_montant text REFERENCES ref.nature_montant(code),
  constat        text NOT NULL,              -- ce que la source établit, ou ce que la personne a dit
  source_url     text NOT NULL,
  page           text,
  qualite        text NOT NULL REFERENCES ref.qualite_fait(code),
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  document_id    bigint REFERENCES raw.document(id),
  jo_texte_id    text REFERENCES jo.texte(id),
  -- Prise de parole à l'Assemblée : identifiant stable du paragraphe (slug de
  -- core.intervention), sans clé étrangère — le connecteur des comptes rendus
  -- recharge la table entière, et une clé bloquerait son DELETE.
  intervention_slug text,
  CHECK (montant_eur IS NULL OR nature_montant IS NOT NULL),
  -- Une preuve archivée : document scellé, texte du JORF ou compte rendu chargé.
  CHECK (qualite = 'PRESSE' OR document_id IS NOT NULL OR jo_texte_id IS NOT NULL OR intervention_slug IS NOT NULL),
  -- Une prise de parole en séance est une déclaration de son auteur.
  CHECK (intervention_slug IS NULL OR (qualite = 'DECLARATIF' AND person_id IS NOT NULL))
);

CREATE INDEX fait_dossier_dossier_idx ON ref.fait_dossier (dossier, section);

COMMENT ON TABLE ref.fait_dossier IS
  'Faits des dossiers documentaires, rangés dans l''ordre commun (contexte, enjeux, cadre, contrôles, situation). '
  'person_id relie la personne citée à sa fiche ; un propos tenu en séance est DECLARATIF.';

-- La fiche d'une personne existe sur le site quand elle a un mandat national ou
-- un vote chargé (cmd/build, loadPersons) : la vue le dit, pour que les dossiers
-- ne produisent pas de lien mort.
CREATE VIEW derived.fait_dossier_personne AS
SELECT f.id AS fait_id, f.dossier, p.id AS person_id, p.slug, p.given_name || ' ' || p.family_name AS nom,
       (EXISTS (SELECT 1 FROM core.ballot b WHERE b.person_id = p.id)
        OR EXISTS (SELECT 1 FROM core.mandate m WHERE m.person_id = p.id
                    AND m.mandate_type IN ('DEPUTE','SENATEUR','DEPUTE_EUROPEEN','MINISTRE','PRESIDENT_REPUBLIQUE'))) AS a_une_fiche,
       '/depute/' || p.slug || '/' AS chemin_fiche,
       'fait-dossier-personne-v1'::text AS method_version
FROM ref.fait_dossier f JOIN core.person p ON p.id = f.person_id;

CREATE TABLE ref.dossier_terme (
  dossier text NOT NULL,
  libelle text NOT NULL,                     -- l'expression telle qu'affichée
  motif   text NOT NULL,                     -- expression régulière PostgreSQL, insensible à la casse
  PRIMARY KEY (dossier, libelle)
);

-- Les prises de parole de l'Assemblée qui emploient les mots d'un dossier, par
-- trimestre. Pour situer la place du sujet dans le débat, jamais pour attribuer
-- une position : une mention n'est ni un soutien ni une critique.
CREATE VIEW derived.dossier_mentions_an AS
SELECT t.dossier, t.libelle,
       extract(year FROM i.date_seance)::int    AS annee,
       extract(quarter FROM i.date_seance)::int AS trimestre,
       count(*)                                 AS interventions,
       count(DISTINCT i.person_id)              AS orateurs,
       min(i.date_seance)                       AS premiere,
       max(i.date_seance)                       AS derniere,
       'dossier-mentions-an-v1'::text           AS method_version
FROM ref.dossier_terme t
JOIN core.intervention i ON i.institution = 'ASSEMBLEE_NATIONALE' AND i.contenu ~* t.motif
GROUP BY t.dossier, t.libelle, 3, 4;

COMMENT ON VIEW derived.dossier_mentions_an IS
  'Interventions en séance publique de l''Assemblée employant les mots d''un dossier. Couverture : les comptes '
  'rendus chargés (depuis juillet 2024). Une mention ne dit pas la position de l''orateur.';

-- Sur toute la période chargée : les orateurs distincts ne s'additionnent pas
-- d'un trimestre à l'autre.
CREATE VIEW derived.dossier_mentions_an_total AS
SELECT t.dossier, t.libelle, count(*) AS interventions, count(DISTINCT i.person_id) AS orateurs,
       min(i.date_seance) AS premiere, max(i.date_seance) AS derniere,
       'dossier-mentions-an-v1'::text AS method_version
FROM ref.dossier_terme t
JOIN core.intervention i ON i.institution = 'ASSEMBLEE_NATIONALE' AND i.contenu ~* t.motif
GROUP BY t.dossier, t.libelle;

CREATE TABLE ref.acteur_numerique (
  siren     text PRIMARY KEY CHECK (siren ~ '^[0-9]{9}$'),
  nom       text NOT NULL,
  -- SEMI_CONDUCTEURS, CLOUD (location d'ordinateurs dans des salles serveurs),
  -- LOGICIEL (édition, dont logiciel libre), IA, POLE (pôle de compétitivité),
  -- FILIERE (association professionnelle ou d'utilisateurs).
  categorie text NOT NULL CHECK (categorie IN ('SEMI_CONDUCTEURS','CLOUD','LOGICIEL','IA','POLE','FILIERE')),
  groupe    text,                            -- groupe ou société de tête, quand elle diffère
  -- Pourquoi la société est rangée là, et d'où vient ce rattachement.
  fondement text NOT NULL,
  qualite   text NOT NULL REFERENCES ref.qualite_fait(code),
  source_url text NOT NULL
);

-- Les acteurs, avec ce que les tables publiques disent d'eux : unité légale
-- (Sirene), services qualifiés SecNumCloud, marchés informatiques dont ils sont
-- titulaires, aides d'État publiées. Aucune de ces colonnes n'est additionnée
-- à une autre.
CREATE VIEW derived.acteur_numerique_fiche AS
SELECT a.siren, a.nom, a.categorie, a.groupe, a.qualite,
       u.denomination, u.categorie_juridique, u.tranche_effectifs, u.annee_effectifs, u.categorie_entreprise,
       u.date_creation, u.etat_administratif,
       (SELECT count(*) FROM core.qualification_secnumcloud q
         WHERE q.siren = a.siren AND q.catalogue_du = (SELECT max(catalogue_du) FROM core.qualification_secnumcloud)) AS services_secnumcloud,
       (SELECT count(DISTINCT m.uid) FROM core.marche_numerique m WHERE m.siren = a.siren) AS marches_informatiques,
       (SELECT count(*) FROM core.aide_nominative n WHERE n.siren = a.siren) AS aides_publiees,
       (SELECT sum(n.montant_esb_eur) FROM core.aide_nominative n WHERE n.siren = a.siren) AS aides_esb_eur,
       'acteur-numerique-fiche-v1'::text AS method_version
FROM ref.acteur_numerique a
LEFT JOIN ref.unite_legale u USING (siren);

-- +goose Down
DROP VIEW derived.acteur_numerique_fiche;
DROP TABLE ref.acteur_numerique;
DROP VIEW derived.dossier_mentions_an_total;
DROP VIEW derived.dossier_mentions_an;
DROP TABLE ref.dossier_terme;
DROP VIEW derived.fait_dossier_personne;
DROP TABLE ref.fait_dossier;
ALTER TABLE ref.fait_multinationale DROP CONSTRAINT fait_multinationale_qualite_fkey;
ALTER TABLE ref.fait_multinationale DROP CONSTRAINT fait_multinationale_nature_montant_fkey;
UPDATE ref.fait_multinationale SET qualite = 'ENTREPRISE' WHERE qualite = 'DECLARATIF';
ALTER TABLE ref.fait_multinationale
  ADD CONSTRAINT fait_multinationale_qualite_check CHECK (qualite IN ('OFFICIEL','PRESSE','ENTREPRISE')),
  ADD CONSTRAINT fait_multinationale_nature_montant_check
    CHECK (nature_montant IN ('PLAFOND','ESTIME','COMMANDE','PAYE','AMENDE','IMPOT','CHIFFRE_AFFAIRES'));
DROP TABLE ref.nature_montant;
DROP TABLE ref.qualite_fait;
