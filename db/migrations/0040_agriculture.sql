-- +goose Up
-- Nourrir ses habitants : ce que les bilans alimentaires permettent d'établir.
--
-- La question posée est simple et la réponse ne l'est pas : « est-ce que le pays
-- arrive à nourrir ses habitants ? » Un bilan alimentaire y répond par un
-- rapport — ce qui est produit sur ce qui est consommé — et par une
-- décomposition : ce qui va directement aux humains, ce qui va au bétail, ce qui
-- est exporté.
--
-- La FAO publie ces bilans pour tous les pays sous CC BY 4.0, avec les mêmes
-- conventions partout : production, importations, exportations, disponibilité
-- intérieure, alimentation humaine, alimentation animale, et l'apport en
-- calories par habitant et par jour.
--
-- CE QU'UN TAUX D'AUTO-APPROVISIONNEMENT NE DIT PAS. Produire plus de céréales
-- qu'on n'en consomme ne veut pas dire qu'on pourrait se nourrir seul : les
-- systèmes de production dépendent d'intrants importés — engrais azotés,
-- tourteaux de soja, carburant — que le bilan ne compte pas. Un pays peut
-- afficher 176 % d'auto-approvisionnement céréalier et dépendre entièrement de
-- l'étranger pour le produire.
CREATE TABLE ref.produit_alimentaire (
  code    text PRIMARY KEY,
  libelle text NOT NULL,
  -- Les agrégats de la FAO : « Grand Total », « Animal Products »,
  -- « Vegetal Products » et les groupes de produits. Ils ne s'additionnent pas
  -- entre eux — un agrégat contient déjà ses composants.
  agregat boolean NOT NULL DEFAULT false
);

CREATE TABLE core.bilan_alimentaire (
  produit_code text NOT NULL REFERENCES ref.produit_alimentaire(code),
  -- L'élément du bilan, conservé dans les termes de la source :
  -- Production, Import quantity, Export quantity, Domestic supply quantity,
  -- Food, Feed, Food supply (kcal/capita/day)…
  element      text NOT NULL,
  annee        integer NOT NULL,
  valeur       numeric NOT NULL,
  unite        text NOT NULL,
  source_id    bigint NOT NULL REFERENCES raw.source(id),
  provenance   core.provenance NOT NULL DEFAULT 'OFFICIAL',
  PRIMARY KEY (produit_code, element, annee)
);

CREATE INDEX bilan_alimentaire_element_idx ON core.bilan_alimentaire (element, annee);

COMMENT ON TABLE core.bilan_alimentaire IS
  'Bilans alimentaires de la FAO pour la France. Les unités diffèrent d''un '
  'élément à l''autre (milliers de tonnes, kcal/habitant/jour) : ne jamais '
  'additionner sans vérifier l''unité.';

-- Surfaces, emploi agricole et nombre d'exploitations. Séparés du bilan parce
-- qu'ils ne portent pas sur un produit mais sur l'appareil de production.
CREATE TABLE core.agriculture_indicateur (
  code       text NOT NULL,
  annee      integer NOT NULL,
  valeur     numeric NOT NULL,
  unite      text NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  provenance core.provenance NOT NULL DEFAULT 'OFFICIAL',
  PRIMARY KEY (code, annee)
);

COMMENT ON TABLE core.agriculture_indicateur IS
  'Surface agricole, emploi agricole : ce avec quoi on produit, par opposition '
  'à ce qu''on produit.';

-- Le taux d'auto-approvisionnement, calculé et non stocké : production rapportée
-- à la disponibilité intérieure. Au-dessus de 100 %, le pays produit plus qu'il
-- ne consomme du produit considéré.
CREATE VIEW derived.autonomie_alimentaire AS
  SELECT p.produit_code,
         r.libelle,
         p.annee,
         p.valeur AS production,
         d.valeur AS disponibilite_interieure,
         CASE WHEN d.valeur > 0 THEN round(100 * p.valeur / d.valeur, 1) END AS taux_pct,
         f.valeur AS alimentation_humaine,
         a.valeur AS alimentation_animale,
         -- Part de la disponibilité qui passe par le bétail avant d'arriver —
         -- ou pas — dans une assiette. C'est la question « direct ou indirect ».
         CASE WHEN coalesce(f.valeur,0) + coalesce(a.valeur,0) > 0
              THEN round(100 * coalesce(a.valeur,0)
                         / (coalesce(f.valeur,0) + coalesce(a.valeur,0)), 1) END AS part_animale_pct
    FROM core.bilan_alimentaire p
    JOIN ref.produit_alimentaire r ON r.code = p.produit_code
    LEFT JOIN core.bilan_alimentaire d
      ON d.produit_code = p.produit_code AND d.annee = p.annee
     AND d.element = 'Domestic supply quantity'
    LEFT JOIN core.bilan_alimentaire f
      ON f.produit_code = p.produit_code AND f.annee = p.annee AND f.element = 'Food'
    LEFT JOIN core.bilan_alimentaire a
      ON a.produit_code = p.produit_code AND a.annee = p.annee AND a.element = 'Feed'
   WHERE p.element = 'Production';

COMMENT ON VIEW derived.autonomie_alimentaire IS
  'Taux d''auto-approvisionnement par produit. Ne mesure PAS l''autonomie : les '
  'intrants importés qui ont servi à produire ne sont pas comptés.';

-- +goose Down
DROP VIEW derived.autonomie_alimentaire;
DROP TABLE core.agriculture_indicateur;
DROP TABLE core.bilan_alimentaire;
DROP TABLE ref.produit_alimentaire;
