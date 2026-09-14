-- +goose Up
-- Quatre séries demandées pour approfondir les notes chômage et retraite :
-- la prime d'activité (qui manquait à core.minima_sociaux_effectif), le taux
-- de remplacement à la retraite, les demandes d'asile (Ofpra), et le ratio
-- cotisants/retraités qui mesure directement la pression démographique sur
-- un système par répartition.

-- La prime d'activité a remplacé le RSA activité au 1er janvier 2016 : une
-- table à part plutôt qu'un nouveau dispositif_code dans
-- core.minima_sociaux_effectif, pour que deux connecteurs indépendants
-- (chacun avec son propre DELETE) ne puissent jamais s'effacer l'un l'autre.
CREATE TABLE core.prime_activite_effectif (
  annee      smallint NOT NULL PRIMARY KEY,
  effectif   integer  NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.prime_activite_effectif IS
  'Nombre d''allocataires de la prime d''activité au 31 décembre, France, '
  'depuis 2016 (Drees, même jeu de données que core.minima_sociaux_effectif '
  'mais fichier distinct : « RSA et prime d''activité — données nationales »).';

-- Le taux de remplacement : la part du revenu d'avant la retraite que la
-- pension remplace, par cohorte de départ et par caractéristique (sexe, durée
-- de carrière, groupe de niveau de vie…). Publié en quantiles, pas en
-- moyenne : un taux médian de 85 % masque une dispersion large que les cinq
-- colonnes de quantile rendent visible.
CREATE TABLE core.taux_remplacement_retraite (
  premiere_annee_retraite smallint NOT NULL,
  caracteristique         text NOT NULL,
  categorie               text NOT NULL,
  sexe                    text NOT NULL,
  revenu_reference        text NOT NULL,
  taux_q10                numeric,
  taux_q25                numeric,
  taux_q50                numeric,
  taux_q75                numeric,
  taux_q90                numeric,
  part_taux_inferieur_100 numeric,
  source_id               bigint NOT NULL REFERENCES raw.source(id),
  created_at              timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (premiere_annee_retraite, caracteristique, categorie, sexe, revenu_reference)
);

COMMENT ON TABLE core.taux_remplacement_retraite IS
  'Répartition (quantiles à 10, 25, 50, 75, 90 %) du taux de remplacement '
  '(pension / revenu d''activité antérieur, en %) par cohorte de première '
  'année pleine de retraite et par caractéristique du retraité (Drees). '
  '100 = pension égale au revenu d''avant la retraite ; au-delà, la retraite '
  'rapporte plus que l''activité ne rapportait (cas rares mais réels aux '
  'quantiles hauts).';

-- Les demandes d'asile : un flux administratif distinct de l'immigration au
-- sens du recensement (core.population_immigree_origine) et des titres de
-- séjour (core.titre_sejour_stock) — toute personne qui dépose une demande
-- n'obtient pas un titre, et l'obtention d'un titre de séjour n'implique pas
-- une demande d'asile préalable.
CREATE TABLE core.demande_asile_ofpra (
  annee              smallint NOT NULL,
  niveau             text NOT NULL CHECK (niveau IN ('TOTAL', 'CONTINENT', 'NATIONALITE')),
  continent          text,
  code_pays          text,
  nationalite        text,
  premiere_demande   integer,
  reexamen           integer,
  reouverture         integer,
  mineurs_accompagnants_premiere_demande integer,
  mineurs_accompagnants_reexamen         integer,
  mineurs_accompagnants_reouverture      integer,
  femmes_premiere_demande integer,
  femmes_reexamen         integer,
  femmes_reouverture      integer,
  source_id          bigint NOT NULL REFERENCES raw.source(id),
  created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX demande_asile_ofpra_uniq ON core.demande_asile_ofpra
  (annee, niveau, coalesce(continent, ''), coalesce(code_pays, ''), coalesce(nationalite, ''));

COMMENT ON TABLE core.demande_asile_ofpra IS
  'Demandes d''asile et de statut d''apatride déposées devant l''Ofpra, par '
  'continent et nationalité, 2021-2025. « premiere_demande » ne compte que '
  'les nouvelles demandes ; reexamen et reouverture sont des demandes '
  'antérieures rouvertes, à ne pas additionner à la première demande sans le '
  'dire.';

-- Le ratio cotisants/retraités : la mesure la plus directement lisible de la
-- pression démographique sur un système de retraite par répartition (docs/
-- cotisations-et-droits.md § 2). Tous régimes confondus.
CREATE TABLE core.cotisants_retraites_ratio (
  annee               smallint NOT NULL PRIMARY KEY,
  cotisants_millions  numeric NOT NULL,
  retraites_millions  numeric NOT NULL,
  ratio_demographique numeric NOT NULL,
  source_id           bigint NOT NULL REFERENCES raw.source(id),
  created_at          timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.cotisants_retraites_ratio IS
  'Nombre de cotisants et de retraités de droit direct, tous régimes, et leur '
  'rapport démographique (cotisants / retraités), France, 2004-2023 (Insee, '
  'sources Drees EACR/EIR/modèle ANCETRE). Rupture de série en 2020 (effectifs '
  'de retraités résidant à l''étranger revus à la baisse).';

-- +goose Down
DROP TABLE core.cotisants_retraites_ratio;
DROP TABLE core.demande_asile_ofpra;
DROP TABLE core.taux_remplacement_retraite;
DROP TABLE core.prime_activite_effectif;
