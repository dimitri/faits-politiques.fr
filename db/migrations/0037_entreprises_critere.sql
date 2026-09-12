-- +goose Up
-- Remplacer une liste par une règle.
--
-- La première version suivait « le CAC 40 ». Or le CAC 40 est un indice
-- PROPRIÉTAIRE d'Euronext : sa composition est arrêtée par un comité d'indice,
-- révisée trimestriellement, et publiée sous les conditions d'Euronext. Aucune
-- autorité publique française ne la publie, et elle n'existe dans aucun jeu de
-- données ouvert.
--
-- Une liste de sociétés écrite à la main était donc soit une copie d'un produit
-- commercial, soit une reconstitution de mémoire. La seconde a été essayée :
-- elle contenait des erreurs — TotalEnergies y figurait avec 7,02 Md€ de
-- chiffre d'affaires, qui est son compte SOCIAL, alors que son compte
-- CONSOLIDÉ déposé la même année affiche 214,6 Md€.
--
-- La règle remplace la liste : « les sociétés ayant déposé un compte consolidé
-- dont le chiffre d'affaires dépasse un seuil, pour un exercice donné ». Elle
-- s'écrit en une requête, le lecteur peut la rejouer, et elle ne dépend d'aucun
-- comité privé.
ALTER TABLE core.entreprise RENAME COLUMN indice TO critere;

COMMENT ON COLUMN core.entreprise.critere IS
  'Règle de sélection ayant fait entrer cette société, et non appartenance à '
  'un indice. Reproductible par requête sur la base ouverte de l''INPI.';

COMMENT ON COLUMN core.entreprise.verification IS
  'Exercice et chiffre d''affaires ayant servi à retenir ce SIREN. Le '
  'rattachement d''une marque à un SIREN ne se devine pas : trois résolutions '
  'automatiques ont produit de mauvaises entités avant d''en arriver là (D-034).';

-- Le LEI, identifiant mondial d'entité juridique. GLEIF le publie en CC0 avec
-- l'identifiant de registre national déclaré par l'entité — pour la France, le
-- SIREN. C'est le seul pont ouvert entre les référentiels financiers
-- européens (ESMA) et le registre du commerce français.
ALTER TABLE core.entreprise ADD COLUMN IF NOT EXISTS lei text;

CREATE UNIQUE INDEX IF NOT EXISTS entreprise_lei_idx
  ON core.entreprise (lei) WHERE lei IS NOT NULL;

COMMENT ON COLUMN core.entreprise.lei IS
  'Legal Entity Identifier (GLEIF, CC0). Le champ registeredAs de GLEIF porte '
  'le SIREN déclaré par l''entité : c''est un appariement par identifiant, '
  'jamais par nom.';

-- +goose Down
DROP INDEX IF EXISTS entreprise_lei_idx;
ALTER TABLE core.entreprise DROP COLUMN IF EXISTS lei;
ALTER TABLE core.entreprise RENAME COLUMN critere TO indice;
