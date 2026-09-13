-- +goose Up
-- Les actes nominatifs du Journal officiel.
--
-- Les nominations dans les corps d'État, les entrées au Gouvernement, les
-- missions temporaires confiées à des parlementaires sont des actes PUBLICS et
-- NOMINATIFS. La DILA les diffuse en open data sous Licence Ouverte. C'est la
-- seule source ouverte qui documente la carrière publique d'un responsable :
-- le RNE n'a aucune profondeur, la HATVP ne couvre que cinq ans, et les
-- annuaires d'anciens élèves sont des fichiers d'association privée.
--
-- CE QUI REND CE CHANTIER DIFFICILE, et qu'il faut écrire avant d'écrire une
-- ligne de code :
--
--  1. Le format ne porte AUCUN élément dédié aux personnes. Ni <PERSONNE>, ni
--     <NOM>, ni <NAISSANCE>. Les noms apparaissent en prose libre, sous deux
--     graphies dans le même fichier : « GOULARD (Guillaume) » dans le titre,
--     « Guillaume GOULARD » dans le corps.
--  2. Le JO ne publie PAS de date de naissance dans les actes de nomination —
--     vérifié sur 57 actes, zéro occurrence. Notre meilleur discriminant,
--     disponible sur 99,99 % de nos personnes, est donc inutilisable ici.
--  3. 41,9 % des élus de notre base ont un homonyme exact (nom + prénom) parmi
--     les 515 000 personnes, et 13 couples sont même dupliqués À L'INTÉRIEUR
--     des parlementaires en exercice.
--
-- Conséquence assumée : l'appariement N'EST PAS automatique. Les mentions sont
-- extraites et stockées avec leur contexte ; le lien vers une personne reste
-- NULL jusqu'à confirmation. Une fiche ne doit jamais afficher une nomination
-- sur la foi d'un nom qui se ressemble.
CREATE TABLE core.acte_jo (
  id            text PRIMARY KEY,
  nature        text,
  numero        text,
  nor           text,
  date_publi    date,
  date_texte    date,
  titre         text,
  titre_complet text,
  ministere     text,
  -- Le texte de l'acte, débarrassé de son balisage. Conservé parce que c'est
  -- là que les noms se trouvent, et pour que toute réextraction ultérieure se
  -- fasse sans retélécharger.
  contenu       text,
  -- Vrai quand le titre annonce une nomination, un détachement, une
  -- intégration, une mission ou une cessation de fonctions.
  nominatif     boolean NOT NULL DEFAULT false,
  source_id     bigint NOT NULL REFERENCES raw.source(id),
  provenance    core.provenance NOT NULL DEFAULT 'OFFICIAL',
  charge_le     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX acte_jo_date_idx ON core.acte_jo (date_publi);
CREATE INDEX acte_jo_nominatif_idx ON core.acte_jo (nominatif) WHERE nominatif;
CREATE INDEX acte_jo_titre_trgm ON core.acte_jo
  USING gin (core.f_unaccent(titre_complet) gin_trgm_ops);

COMMENT ON TABLE core.acte_jo IS
  'Actes du Journal officiel. Chargés par upsert sur l''identifiant : les '
  'archives incrémentales de la DILA contiennent des rééditions de fiches '
  'anciennes, pas seulement le JO du jour.';

-- Une mention de personne repérée dans un acte. Ce n'est PAS une nomination
-- attribuée : c'est un nom lu dans un texte, avec ce qui l'entoure.
CREATE TABLE core.acte_jo_mention (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  acte_id     text NOT NULL REFERENCES core.acte_jo(id) ON DELETE CASCADE,
  nom         text NOT NULL,
  prenom      text,
  -- Les quelques mots qui suivent le nom. C'est là que se trouve la qualité —
  -- « conseiller d'Etat », « députée », « préfet de » — présente dans 67 % des
  -- actes et seul substitut à la date de naissance pour lever un doute.
  contexte    text,
  -- Où le nom a été lu : TITRE ou CORPS. Les deux graphies diffèrent.
  origine     text NOT NULL CHECK (origine IN ('TITRE','CORPS')),
  -- Rattachement à une personne. NULL tant qu'il n'est pas confirmé.
  person_id   bigint REFERENCES core.person(id) ON DELETE SET NULL,
  -- CANDIDAT : un seul nom correspond en base, sans confirmation contextuelle.
  -- AMBIGU   : plusieurs personnes portent ce nom.
  -- CONFIRME : une décision humaine ou un indice contextuel fort l'a établi.
  -- ABSENT   : aucune personne de notre base ne porte ce nom.
  statut      text NOT NULL CHECK (statut IN ('CANDIDAT','AMBIGU','CONFIRME','ABSENT')),
  homonymes   integer NOT NULL DEFAULT 0,
  method_version text NOT NULL
);

CREATE INDEX acte_jo_mention_acte_idx ON core.acte_jo_mention (acte_id);
CREATE INDEX acte_jo_mention_person_idx ON core.acte_jo_mention (person_id)
  WHERE person_id IS NOT NULL;
CREATE INDEX acte_jo_mention_statut_idx ON core.acte_jo_mention (statut);

COMMENT ON COLUMN core.acte_jo_mention.statut IS
  'CANDIDAT n''est pas CONFIRME. Rien ne doit être publié sur la foi d''un '
  'CANDIDAT : c''est une piste à vérifier, pas un fait établi.';

-- +goose Down
DROP TABLE core.acte_jo_mention;
DROP TABLE core.acte_jo;
