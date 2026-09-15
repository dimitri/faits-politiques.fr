-- +goose Up
-- Souveraineté numérique de l'État : le contexte, les règles, les contrôles et
-- la situation que montrent les chiffres publics. Voir
-- docs/souverainete-numerique.md et D-065.
--
-- Quatre sources, jamais additionnées entre elles :
--   core.qualification_secnumcloud  les services Cloud qualifiés par l'ANSSI
--                                   (catalogue officiel, daté) ;
--   core.sanction_cnil              la liste des sanctions de la CNIL, telle que
--                                   publiée : l'organisme n'y est désigné que par
--                                   sa catégorie, la CNIL retirant les noms à
--                                   l'expiration du délai de publicité ;
--   core.marche_numerique           tous les marchés publics informatiques des
--                                   données essentielles (codes CPV 48, 72,
--                                   302), pas seulement ceux des groupes suivis ;
--   core.sill_logiciel              le socle interministériel de logiciels libres.
-- Les textes, constats et déclarations du dossier sont des faits de
-- ref.fait_dossier (migration 0098), comme ceux des autres dossiers.

CREATE TABLE core.qualification_secnumcloud (
  fournisseur        text NOT NULL,          -- tel qu'écrit au catalogue
  service            text NOT NULL,
  saas               boolean NOT NULL,
  paas               boolean NOT NULL,
  caas               boolean NOT NULL,
  iaas               boolean NOT NULL,
  date_debut         date NOT NULL,
  date_fin           date NOT NULL,
  decision           text,                   -- numéro de la décision de qualification
  catalogue_du       date NOT NULL,          -- date de mise à jour du catalogue lu
  -- Société française exploitante, quand elle est identifiée sans ambiguïté
  -- (dénomination Sirene) ; NULL sinon.
  siren              text CHECK (siren ~ '^[0-9]{9}$'),
  -- Technologie d'un groupe extra-européen opérée sous licence (S3NS : Google
  -- Cloud). La qualification porte sur l'opérateur, pas sur l'éditeur.
  technologie_tierce text,
  document_id        bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (fournisseur, service, catalogue_du),
  CHECK (saas OR paas OR caas OR iaas),
  CHECK (date_fin > date_debut)
);

COMMENT ON TABLE core.qualification_secnumcloud IS
  'Services Cloud (location d''ordinateurs dans des salles serveurs) qualifiés SecNumCloud (catalogue ANSSI, section 3.3.1). Une qualification '
  'porte sur un service, pas sur la société : un marché passé avec un fournisseur qualifié n''utilise pas '
  'forcément son service qualifié.';

CREATE TABLE core.sanction_cnil (
  rang             integer NOT NULL,          -- ordre dans la liste publiée
  date_decision    date NOT NULL,
  organisme        text NOT NULL,             -- catégorie publiée (« SOCIÉTÉ DE SUPPORT LOGISTIQUE »)
  manquements      text,
  sanction         text NOT NULL,             -- libellé publié
  -- Somme des montants du libellé (« 60 et 40 millions » = 100 M€) ; NULL
  -- pour un avertissement, un rappel à l'ordre ou un montant non publié.
  montant_eur      numeric,
  deliberation_url text,
  public           boolean NOT NULL,          -- l'organisme sanctionné est une personne publique (catégorie publiée)
  procedure_simplifiee boolean NOT NULL,
  document_id      bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (rang, document_id)
);

COMMENT ON TABLE core.sanction_cnil IS
  'Sanctions de la CNIL (liste publiée sur cnil.fr). Aucune ré-identification : quand la CNIL a retiré le nom de '
  'l''organisme à l''expiration du délai de publicité, ce projet ne le rétablit pas (D-065).';

CREATE TABLE core.marche_numerique (
  uid                  text NOT NULL,
  titulaire_id         text NOT NULL,
  titulaire_type_id    text,
  titulaire_nom        text,
  siren                text CHECK (siren ~ '^[0-9]{9}$'),
  acheteur_id          text,
  acheteur_nom         text,
  acheteur_categorie   text,                 -- classement du consolidateur (État, Commune, Établissement hospitalier…)
  objet                text,
  code_cpv             text NOT NULL,
  nature               text,
  techniques           text,
  date_notification    date,
  montant_eur          numeric,              -- montant ou MAXIMUM d'accord-cadre
  montant_rationalise  numeric,
  montant_anomalie     text,
  -- Éditeur ou service nommé dans l'objet (« licences Microsoft », « Azure ») :
  -- le seul endroit où les données essentielles disent ce qui est acheté
  -- quand le titulaire est un revendeur. NULL : aucun produit reconnu.
  produit_nomme        text,
  -- L'objet ou le code CPV désigne de l'hébergement ou un service Cloud.
  hebergement          boolean NOT NULL,
  document_id          bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (uid, titulaire_id)
);

CREATE INDEX marche_numerique_siren_idx ON core.marche_numerique (siren);

COMMENT ON TABLE core.marche_numerique IS
  'Marchés publics informatiques (DECP consolidées, codes CPV 48, 72 et 302, dernière version). Le titulaire est '
  'souvent un revendeur ou un intégrateur : sa nationalité n''est pas celle de l''éditeur du logiciel acheté.';

-- La nationalité des titulaires des marchés informatiques. « Groupe étranger »
-- ne vaut que pour les sociétés rattachées par GLEIF ou par la sélection
-- nommée : une filiale absente de GLEIF est comptée « société française, groupe
-- étranger non identifié ». Les montants sont dédoublonnés comme dans
-- derived.multinationale_marches et restent des ordres de grandeur (maxima
-- d'accords-cadres).
CREATE VIEW derived.marche_numerique_titulaire AS
WITH g AS (
  SELECT DISTINCT ON (siren) siren, groupe, pays_groupe
  FROM core.filiale_groupe_etranger ORDER BY siren, (origine = 'SELECTION') DESC
), l AS (
  SELECT m.*, coalesce(m.montant_rationalise, m.montant_eur) AS montant,
         (coalesce(m.techniques, '') ILIKE '%accord-cadre%' OR coalesce(m.nature, '') ILIKE '%accord-cadre%') AS accord_cadre,
         CASE
           WHEN m.siren IS NULL AND upper(coalesce(m.titulaire_type_id, '')) IN ('HORS_UE','HORS-UE') THEN 'HORS_UE'
           WHEN m.siren IS NULL AND upper(coalesce(m.titulaire_type_id, '')) IN ('TVA','TVA_INTRACOMMUNAUTAIRE','UE') THEN 'UE_HORS_FRANCE'
           WHEN m.siren IS NULL THEN 'NON_IDENTIFIE'
           WHEN g.pays_groupe = 'US' THEN 'GROUPE_US'
           WHEN g.pays_groupe IN ('AT','BE','BG','HR','CY','CZ','DK','EE','FI','DE','GR','HU','IE','IT','LV','LT','LU',
                                  'MT','NL','PL','PT','RO','SK','SI','ES','SE') THEN 'GROUPE_UE'
           WHEN g.pays_groupe IS NOT NULL THEN 'GROUPE_AUTRE'
           ELSE 'FRANCE_SANS_GROUPE_ETRANGER_CONNU'
         END AS rattachement
  FROM core.marche_numerique m LEFT JOIN g USING (siren)
), u AS (
  SELECT DISTINCT ON (uid) * FROM l ORDER BY uid, titulaire_id
), d AS (
  SELECT DISTINCT ON (rattachement, titulaire_id, date_notification, montant) *
  FROM u WHERE coalesce(montant_anomalie, '') = '' AND montant IS NOT NULL AND NOT accord_cadre
  ORDER BY rattachement, titulaire_id, date_notification, montant
)
SELECT u.rattachement,
       count(*)                                          AS marches,
       count(*) FILTER (WHERE u.acheteur_categorie = 'État') AS dont_etat,
       count(*) FILTER (WHERE u.hebergement)             AS dont_hebergement,
       (SELECT sum(montant) FROM d WHERE d.rattachement = u.rattachement) AS montants_hors_accords_cadres_eur,
       'marche-numerique-titulaire-v1'::text             AS method_version
FROM u GROUP BY u.rattachement;

COMMENT ON VIEW derived.marche_numerique_titulaire IS
  'Marchés informatiques par rattachement du titulaire (un marché compte une fois, premier titulaire). '
  'montants_hors_accords_cadres_eur : marchés simples seulement, dédoublonnés, hors montants suspects.';

-- Ce que les objets des marchés nomment. Rapporté au nombre total de marchés
-- informatiques, pour dire aussi ce que les données ne disent pas.
CREATE VIEW derived.marche_numerique_produit AS
WITH u AS (SELECT DISTINCT ON (uid) * FROM core.marche_numerique ORDER BY uid, titulaire_id)
SELECT coalesce(produit_nomme, '(aucun produit nommé)') AS produit,
       count(*)                                              AS marches,
       count(*) FILTER (WHERE acheteur_categorie = 'État')    AS dont_etat,
       count(*) FILTER (WHERE acheteur_categorie = 'Établissement hospitalier') AS dont_hopitaux,
       count(*) FILTER (WHERE hebergement)                    AS dont_hebergement,
       round(100.0 * count(*) / sum(count(*)) OVER (), 2)     AS pct_marches,
       min(date_notification)                                 AS premiere_notification,
       max(date_notification)                                 AS derniere_notification,
       'marche-numerique-produit-v1'::text                    AS method_version
FROM u GROUP BY 1;

-- Les marchés d'hébergement ou de Cloud et la qualification de leur titulaire.
-- « Titulaire qualifié » : la société détient au moins un service qualifié
-- SecNumCloud en cours à la date du catalogue ; cela ne prouve pas que le
-- marché porte sur ce service.
CREATE VIEW derived.marche_hebergement_qualification AS
WITH q AS (
  SELECT DISTINCT siren FROM core.qualification_secnumcloud
  WHERE siren IS NOT NULL AND catalogue_du = (SELECT max(catalogue_du) FROM core.qualification_secnumcloud)
), u AS (SELECT DISTINCT ON (uid) * FROM core.marche_numerique WHERE hebergement ORDER BY uid, titulaire_id)
SELECT coalesce(u.acheteur_categorie, '(non classé)') AS acheteur_categorie,
       count(*)                                        AS marches_hebergement,
       count(*) FILTER (WHERE q.siren IS NOT NULL)     AS titulaire_qualifie,
       count(*) FILTER (WHERE u.produit_nomme IN ('Microsoft','Amazon Web Services','Google')) AS objet_nomme_hyperscaler_us,
       'marche-hebergement-qualification-v1'::text     AS method_version
FROM u LEFT JOIN q USING (siren)
GROUP BY 1;

-- Le socle interministériel de logiciels libres (SILL) : les logiciels libres
-- que des agents publics recommandent et utilisent, publiés par la DINUM sur
-- code.gouv.fr. Un logiciel référencé n'est pas un logiciel déployé partout :
-- les colonnes comptent les organisations qui se déclarent utilisatrices ou
-- référentes.
CREATE TABLE core.sill_logiciel (
  id                   integer PRIMARY KEY,     -- identifiant du SILL
  nom                  text NOT NULL,
  licence              text,
  reference_depuis     date,
  en_observation       boolean NOT NULL,
  issu_service_public  boolean NOT NULL,        -- développé par un service public français
  contrat_support      boolean NOT NULL,        -- couvert par le marché interministériel de support
  organisations        integer NOT NULL,        -- organisations utilisatrices ou référentes
  utilisateurs         integer NOT NULL,
  referents            integer NOT NULL,
  prestataires         integer NOT NULL,        -- prestataires de service déclarés
  categories           text[],
  document_id          bigint NOT NULL REFERENCES raw.document(id)
);

COMMENT ON TABLE core.sill_logiciel IS
  'Socle interministériel de logiciels libres (code.gouv.fr/sill). Recommandation et usage déclarés par des agents '
  'publics, pas un inventaire des déploiements.';

-- +goose Down
DROP TABLE core.sill_logiciel;
DROP VIEW derived.marche_hebergement_qualification;
DROP VIEW derived.marche_numerique_produit;
DROP VIEW derived.marche_numerique_titulaire;
DROP TABLE core.marche_numerique;
DROP TABLE core.sanction_cnil;
DROP TABLE core.qualification_secnumcloud;
