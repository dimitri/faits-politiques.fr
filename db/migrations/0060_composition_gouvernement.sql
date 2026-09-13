-- +goose Up
-- La composition des gouvernements, telle que les décrets la publient.
--
-- POURQUOI CETTE TABLE EXISTE. Les services du Premier ministre publient en
-- données ouvertes « Composition des gouvernements de la Vème République », et
-- ce jeu s'ARRÊTE EN 2014 : dernière mise à jour le 18 juin 2014, aucun
-- successeur au catalogue. Notre base croyait donc Manuel Valls encore Premier
-- ministre en septembre 2026.
--
-- Les autres pistes ont été essayées et écartées :
--   * Légifrance répond HTTP 403 derrière une protection anti-robot, et son API
--     exige un compte PISTE ;
--   * l'annuaire de service-public.fr donne les ministères d'aujourd'hui, pas
--     l'historique des ministres ;
--   * l'open data de l'Assemblée ne connaît que les ministres qui furent
--     députés, et il y mêle les « parlementaires en mission », qui ne sont pas
--     membres du Gouvernement — 398 lignes sur 1 103.
--
-- Reste la source de droit : le décret publié au Journal officiel. Il est
-- diffusé par la DILA en open data, et sa prose est réglée parce que c'est du
-- droit — la même phrase depuis 1959.
--
-- CE QUE CETTE TABLE EST, ET N'EST PAS. Elle contient ce que le décret DIT :
-- une civilité, un prénom, un patronyme en capitales, une fonction, un
-- portefeuille. Elle ne contient pas une personne de notre base. Le
-- rapprochement est une opération distincte, et son résultat reste CANDIDAT
-- tant qu'un identifiant ne l'a pas confirmé — 41,9 % de nos élus ont un
-- homonyme exact en nom et prénom.
CREATE TABLE core.gouvernement_membre (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  acte_id      text NOT NULL REFERENCES core.acte_jo(id) ON DELETE CASCADE,
  -- La date du DÉCRET, pas celle de sa publication : un décret du 12 octobre
  -- paru le 13 nomme à compter du 12. L'écart est d'un ou deux jours, et il
  -- suffit à faire naître des gouvernements de quarante-huit heures.
  date_effet   date NOT NULL,
  rang         smallint NOT NULL,
  -- NOMINATION ou CESSATION. Un même décret porte souvent les deux : il met fin
  -- aux fonctions des uns et nomme les autres.
  sens         text NOT NULL CHECK (sens IN ('NOMINATION', 'CESSATION')),
  fonction     text NOT NULL CHECK (fonction IN (
                 'PREMIER_MINISTRE', 'MINISTRE_ETAT', 'MINISTRE',
                 'MINISTRE_DELEGUE', 'SECRETAIRE_ETAT', 'HAUT_COMMISSAIRE')),
  civilite     text NOT NULL,
  prenom       text NOT NULL,
  -- Le patronyme tel que le Journal officiel l'écrit : EN CAPITALES, particule
  -- comprise (« de MONTCHALIN », « Le HÉNANFF »). C'est cette convention
  -- typographique qui rend la découpe possible ; la normaliser ici ferait
  -- perdre l'information qui a servi à l'extraire.
  nom          text NOT NULL,
  -- « auprès du ministre de l'intérieur » : le rattachement d'un ministre
  -- délégué ou d'un secrétaire d'État. NULL pour un ministre de plein exercice.
  rattachement text,
  portefeuille text,
  -- Le rapprochement avec core.person, et son degré de certitude. Même échelle
  -- que core.acte_jo_mention : rien de CANDIDAT ne doit être publié comme un
  -- fait sans le dire.
  person_id    bigint REFERENCES core.person(id) ON DELETE SET NULL,
  statut       text NOT NULL DEFAULT 'ABSENT'
                 CHECK (statut IN ('CONFIRME', 'CANDIDAT', 'AMBIGU', 'ABSENT')),
  homonymes    smallint NOT NULL DEFAULT 0,
  method_version text NOT NULL,
  -- Le nom NORMALISÉ, calculé une fois et stocké.
  --
  -- Les décrets écrivent le patronyme tantôt en capitales, tantôt en casse
  -- ordinaire — « Manuel VALLS » et « Manuel Valls » la même année — et la
  -- continuité d'un mandat se suit sur cette clé, pas sur le texte brut. La
  -- calculer à la volée dans la vue coûtait seize secondes pour mille deux cent
  -- soixante-deux lignes : elle est recalculée pour chaque comparaison, et il y
  -- en a autant que de couples. Stockée et indexée, la vue répond en un instant.
  --
  -- core.f_unaccent est l'enveloppe IMMUTABLE qui rend cela possible :
  -- unaccent() seul n'est pas réputé stable, et PostgreSQL refuserait la
  -- colonne générée comme l'index.
  cle_nom      text GENERATED ALWAYS AS (core.f_unaccent(lower(nom))) STORED,
  cle_prenom   text GENERATED ALWAYS AS (core.f_unaccent(lower(prenom))) STORED,
  -- Un décret ne nomme pas deux fois la même personne à la même fonction.
  UNIQUE (acte_id, sens, fonction, nom, prenom, portefeuille)
);

COMMENT ON TABLE core.gouvernement_membre IS
  'Membres du Gouvernement cités par les décrets publiés au Journal officiel. '
  'Ce que le décret DIT, pas qui est la personne : le rapprochement avec '
  'core.person porte son statut, et CANDIDAT n''est pas un fait.';
COMMENT ON COLUMN core.gouvernement_membre.statut IS
  'CONFIRME : rapproché par un identifiant. CANDIDAT : un seul porteur du nom, '
  'mais rien ne prouve que ce soit lui. AMBIGU : plusieurs porteurs. ABSENT : '
  'personne de ce nom en base.';

CREATE INDEX gouvernement_membre_date_idx ON core.gouvernement_membre (date_effet);
CREATE INDEX gouvernement_membre_person_idx ON core.gouvernement_membre (person_id)
  WHERE person_id IS NOT NULL;
CREATE INDEX gouvernement_membre_nom_idx ON core.gouvernement_membre (nom, prenom);
CREATE INDEX gouvernement_membre_cle_idx ON core.gouvernement_membre (cle_nom, cle_prenom);
CREATE INDEX gouvernement_membre_sens_idx ON core.gouvernement_membre (sens, date_effet);

-- ---------------------------------------------------------------------------
-- Les périodes de fonction, DÉDUITES des décrets.
--
-- Un décret nomme à une date ; il ne dit pas jusqu'à quand. La fin d'une
-- fonction se déduit de trois événements, et le premier qui survient l'emporte :
--
--   1. un décret de CESSATION qui nomme explicitement la personne ;
--   2. un décret de composition COMPLET qui ne la reprend pas — un gouvernement
--      nouvellement nommé remplace le précédent en entier ;
--   3. rien : la fonction est en cours, et la période reste ouverte.
--
-- Le point 2 demande de distinguer un décret de composition complet d'un simple
-- remaniement. Le critère retenu est le nombre de ministres de plein exercice
-- nommés : un gouvernement en compte plus de dix, un remaniement moins. Il est
-- imparfait, et c'est pourquoi il est ici, dans `derived`, avec sa
-- `method_version` — pas dans `core`, où il passerait pour un fait.
CREATE VIEW derived.mandat_ministeriel AS
  WITH complets AS (
    -- Les décrets qui recomposent un gouvernement entier : ils remplacent le
    -- précédent, et qui n'y figure plus n'est plus en fonction. Une vingtaine
    -- de lignes, donc une CTE matérialisée sans dommage.
    SELECT acte_id, date_effet
      FROM core.gouvernement_membre
     WHERE sens = 'NOMINATION' AND fonction IN ('MINISTRE', 'MINISTRE_ETAT')
     GROUP BY 1, 2 HAVING count(*) >= 10
  )
  -- Les sous-requêtes corrélées interrogent la TABLE, jamais une CTE.
  -- PostgreSQL matérialise une CTE référencée plusieurs fois, et une CTE
  -- matérialisée n'a pas d'index : la même vue écrite sur des CTE mettait
  -- seize secondes à rendre mille deux cent soixante-deux lignes, en
  -- reparcourant mille trois cent soixante-dix lignes pour chacune.
  SELECT DISTINCT ON (n.cle_nom, n.cle_prenom, n.fonction, f.validity,
                      coalesce(n.rattachement, ''), coalesce(n.portefeuille, ''))
         n.id,
         n.person_id,
         n.statut,
         n.prenom,
         n.nom,
         n.fonction,
         n.rattachement,
         n.portefeuille,
         f.validity,
         n.acte_id,
         'composition-jo-v2'::text AS method_version
    FROM core.gouvernement_membre n
    CROSS JOIN LATERAL (
      SELECT daterange(n.date_effet, CASE WHEN n.fonction = 'PREMIER_MINISTRE' THEN
               -- Un Premier ministre reste en fonction jusqu'à ce qu'un autre
               -- soit nommé, ou qu'il soit explicitement démis. Le décret qui
               -- nomme SES ministres, deux jours après le sien, ne le reprend
               -- évidemment pas — l'appliquer lui donnait des mandats de
               -- quarante-huit heures, Manuel Valls compris.
               least(
                 (SELECT min(p.date_effet) FROM core.gouvernement_membre p
                   WHERE p.sens = 'NOMINATION' AND p.fonction = 'PREMIER_MINISTRE'
                     AND p.date_effet > n.date_effet),
                 (SELECT min(c.date_effet) FROM core.gouvernement_membre c
                   WHERE c.sens = 'CESSATION' AND c.cle_nom = n.cle_nom
                     AND c.cle_prenom = n.cle_prenom AND c.date_effet > n.date_effet))
             ELSE
               least(
                 (SELECT min(c.date_effet) FROM core.gouvernement_membre c
                   WHERE c.sens = 'CESSATION' AND c.cle_nom = n.cle_nom
                     AND c.cle_prenom = n.cle_prenom AND c.date_effet > n.date_effet),
                 (SELECT min(k.date_effet) FROM complets k
                   WHERE k.date_effet > n.date_effet
                     AND NOT EXISTS (SELECT 1 FROM core.gouvernement_membre r
                                      WHERE r.acte_id = k.acte_id AND r.sens = 'NOMINATION'
                                        AND r.cle_nom = n.cle_nom
                                        AND r.cle_prenom = n.cle_prenom)),
                 -- Une NOUVELLE nomination de la même personne à la même
                 -- fonction clôt la précédente. Sans cette borne, un ministre
                 -- reconduit de gouvernement en gouvernement accumulait autant
                 -- de périodes ouvertes que de reconductions : Jean-Noël Barrot
                 -- était trois fois ministre des affaires étrangères en même
                 -- temps.
                 (SELECT min(r.date_effet) FROM core.gouvernement_membre r
                   WHERE r.sens = 'NOMINATION' AND r.cle_nom = n.cle_nom
                     AND r.cle_prenom = n.cle_prenom AND r.fonction = n.fonction
                     AND r.date_effet > n.date_effet))
             END, '[)') AS validity) f
   WHERE n.sens = 'NOMINATION' AND NOT isempty(f.validity)
   -- Une même nomination peut être citée deux fois : par le même décret, quand
   -- le texte répète une personne, et par deux décrets du même jour — le Journal
   -- officiel publie parfois « Décrets du 2 octobre 1990 » au pluriel, et les
   -- rectificatifs reprennent le dispositif. Deux périodes strictement
   -- identiques ne sont pas deux mandats.
   --
   -- Ce qui n'est PAS réduit : deux périodes qui se chevauchent avec des
   -- portefeuilles ou des rattachements différents. Cumuler deux portefeuilles
   -- existe, et c'est un fait à garder.
   ORDER BY n.cle_nom, n.cle_prenom, n.fonction, f.validity,
            coalesce(n.rattachement, ''), coalesce(n.portefeuille, ''),
            n.acte_id, n.id;

COMMENT ON VIEW derived.mandat_ministeriel IS
  'Périodes de fonction ministérielle DÉDUITES des décrets : un décret nomme, il '
  'ne dit pas jusqu''à quand. Pour un Premier ministre, la fin est la nomination '
  'du suivant ; pour les autres, la première cessation nominative ou le premier '
  'gouvernement complet qui ne les reprend pas.';

-- +goose Down
DROP VIEW derived.mandat_ministeriel;
DROP TABLE core.gouvernement_membre;
