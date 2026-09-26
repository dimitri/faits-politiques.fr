-- +goose Up
-- Débloque un manque documenté comme structurel au § 9 du dossier eau :
-- l'eau est un budget ANNEXE (nomenclature M49/M49A), que les données OFGL
-- déjà chargées (core.commune_indicator, budget PRINCIPAL uniquement) ne
-- couvrent pas. L'OFGL publie en réalité les budgets annexes séparément,
-- avec leur propre nomenclature — vérifié directement sur son API
-- (data.ofgl.fr, jeux ofgl-base-communes et ofgl-base-gfp), pas supposé
-- absent après le seul examen d'internal/communes/ofgl.go.
--
-- Deux agrégats seulement (dépenses et recettes totales du budget annexe),
-- pas les 41 agrégats détaillés que l'API publie : un chargement plus fin
-- serait possible, mais hors de la proportion de ce chantier. Le champ
-- nom_budget (ex. "EAU-AUTRECHE", "ASST-AUTRECHE", "SPANC CCRAPC") laisse
-- deviner eau/assainissement pour beaucoup de lignes mais pas toutes, dans
-- un texte libre non structuré par l'OFGL — jamais recatégorisé ici.
CREATE TABLE core.budget_annexe_eau (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  type_collectivite text NOT NULL CHECK (type_collectivite IN ('COMMUNE', 'EPCI')),
  code              text NOT NULL,  -- code Insee (commune) ou Siren (EPCI)
  nom_collectivite  text NOT NULL,
  nom_budget        text NOT NULL, -- libellé du budget annexe (lbudg OFGL), texte libre
  nomenclature      text NOT NULL CHECK (nomenclature IN ('M49', 'M49A')),
  annee             integer NOT NULL,
  agregat           text NOT NULL CHECK (agregat IN ('DEPENSES_TOTALES', 'RECETTES_TOTALES')),
  montant_eur       numeric NOT NULL,
  source_id         bigint NOT NULL REFERENCES raw.source(id),
  created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX budget_annexe_eau_code_idx ON core.budget_annexe_eau (code, annee);
CREATE INDEX budget_annexe_eau_type_annee_idx ON core.budget_annexe_eau (type_collectivite, annee);

COMMENT ON TABLE core.budget_annexe_eau IS
  'Budgets annexes eau/assainissement (nomenclature M49 et M49A), communes et EPCI, '
  '2018-2025 — OFGL (data.ofgl.fr, jeux ofgl-base-communes et ofgl-base-gfp). Seuls deux '
  'agrégats sont chargés (dépenses totales, recettes totales) sur les 41 que l''API '
  'publie. M49 et M49A sont la même nomenclature comptable, seule la présentation '
  'diffère selon la taille de la collectivité (M49A = présentation abrégée) : les deux '
  'sont chargés ensemble, jamais l''un sans l''autre. Une collectivité qui délègue '
  'entièrement son service à un opérateur privé n''a pas de budget annexe M49 ou en a '
  'un réduit : cette table ne capture que ce qui transite par le budget public, pas le '
  'chiffre d''affaires de l''opérateur.';

-- +goose Down
DROP TABLE core.budget_annexe_eau;
