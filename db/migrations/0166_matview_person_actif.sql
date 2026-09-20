-- +goose Up

-- mv.person_actif : les personnes ayant un mandat NATIONAL ou un vote
-- enregistré — exactement le filtre que loadPersons (internal/sitegen/
-- load.go) applique déjà (EXISTS ballot OR EXISTS mandat qualifiant),
-- mais qui scannait pour cela la totalité de core.person (515 374 lignes,
-- dont l'immense majorité de conseillers municipaux RNE sans fiche). Un
-- PRÉ-FILTRE, pas un miroir : la sortie est de deux ordres de grandeur
-- plus petite que core.person.
CREATE MATERIALIZED VIEW mv.person_actif AS
  SELECT p.id, p.slug, p.given_name, p.family_name
    FROM core.person p
   WHERE EXISTS (SELECT 1 FROM core.ballot b WHERE b.person_id = p.id)
      OR EXISTS (SELECT 1 FROM core.mandate m WHERE m.person_id = p.id
                  AND m.mandate_type IN ('DEPUTE','SENATEUR','DEPUTE_EUROPEEN',
                                         'MINISTRE','PRESIDENT_REPUBLIQUE'));

CREATE UNIQUE INDEX person_actif_pk ON mv.person_actif (id);
CREATE UNIQUE INDEX person_actif_slug_idx ON mv.person_actif (slug);

COMMENT ON MATERIALIZED VIEW mv.person_actif IS
  'Personnes avec un mandat national ou un vote enregistré — remplace le '
  'premier SELECT de loadPersons (internal/sitegen/load.go), qui scannait '
  'core.person (515k lignes, surtout des conseillers municipaux sans '
  'fiche) pour un résultat de quelques milliers de lignes.';

-- mv.mandate_actif : l'historique COMPLET de mandats des personnes
-- actives (pas seulement leurs mandats qualifiants — une fiche affiche
-- tout l'historique) — remplace le SELECT sans WHERE sur core.mandate
-- (617 196 lignes, dont 508 788 conseillers municipaux) que
-- loadPersons refaisait, en filtrant seulement APRÈS coup en Go.
CREATE MATERIALIZED VIEW mv.mandate_actif AS
  SELECT m.person_id, m.mandate_type, m.constituency, m.role, m.portefeuille,
         m.validity, m.commune_code
    FROM core.mandate m
   WHERE m.person_id IN (SELECT id FROM mv.person_actif);

CREATE INDEX mandate_actif_person_idx ON mv.mandate_actif (person_id);

COMMENT ON MATERIALIZED VIEW mv.mandate_actif IS
  'Historique complet des mandats des personnes actives (mv.person_actif) '
  '— remplace le SELECT sans filtre sur core.mandate que loadPersons '
  '(internal/sitegen/load.go) refaisait, avant de jeter en Go tout ce qui '
  'n''appartenait pas à une personne active.';

-- mv.affiliation_actif : idem pour les appartenances (groupe, parti) —
-- le nom de l'organisation est déjà joint, internal/sitegen n'a plus
-- besoin de core.organization pour ce seul usage.
CREATE MATERIALIZED VIEW mv.affiliation_actif AS
  SELECT a.person_id, o.name AS organization_nom, a.organization_kind, a.declared_via, a.validity
    FROM core.affiliation a
    JOIN core.organization o ON o.id = a.organization_id
   WHERE a.person_id IN (SELECT id FROM mv.person_actif);

CREATE INDEX affiliation_actif_person_idx ON mv.affiliation_actif (person_id);

COMMENT ON MATERIALIZED VIEW mv.affiliation_actif IS
  'Appartenances (groupe, parti) des personnes actives, nom d''organisation '
  'déjà joint — remplace le SELECT sans filtre sur core.affiliation JOIN '
  'core.organization que loadPersons (internal/sitegen/load.go) refaisait.';

-- +goose Down
DROP MATERIALIZED VIEW mv.affiliation_actif;
DROP MATERIALIZED VIEW mv.mandate_actif;
DROP MATERIALIZED VIEW mv.person_actif;
