-- +goose Up
-- La population par âge, pour pouvoir rapporter un résultat à autre chose qu'aux
-- inscrits.
--
-- Un score présidentiel est presque toujours cité en pourcentage des suffrages
-- exprimés. C'est le dénominateur le plus flatteur : il exclut l'abstention, les
-- blancs, les nuls, et les non-inscrits. Le dénominateur le plus large — les
-- personnes en âge de voter — raconte une autre histoire, et il faut pouvoir
-- calculer les deux sans arbitrer.
--
-- Cette table transcrit le fichier INSEE de population par âge détaillé au
-- 1er janvier. Rien n'y est interpolé ni agrégé : l'interpolation à la date d'un
-- scrutin est un CALCUL, elle vit dans derived (voir plus bas).
CREATE TABLE core.population_age (
  -- METRO = France métropolitaine, FRANCE = France entière (DOM compris).
  -- Les deux champs coexistent dans le fichier source et ne couvrent pas les
  -- mêmes années : la série France entière ne commence qu'en 1991. Les mélanger
  -- produirait une rupture de série invisible.
  champ       text NOT NULL CHECK (champ IN ('METRO', 'FRANCE')),
  annee       integer NOT NULL,
  -- Âge en années révolues au 1er janvier.
  age         smallint NOT NULL CHECK (age >= 0),
  -- La dernière ligne de chaque feuille INSEE n'est pas un âge mais une TRANCHE
  -- OUVERTE : « 100 ou plus » en 1965, « 105 ou plus » en 2022. Un filtre naïf
  -- sur « l'âge est un entier » la laisse tomber, et la population disparaît
  -- silencieusement — quelques milliers de personnes en 1965, une trentaine de
  -- milliers en 2022. L'erreur est petite mais elle est systématique, toujours
  -- dans le même sens, et invisible. D'où ce drapeau : la ligne est chargée avec
  -- l'âge plancher de sa tranche, et le drapeau dit qu'elle en agrège d'autres.
  age_ouvert  boolean NOT NULL DEFAULT false,
  population  bigint NOT NULL CHECK (population >= 0),
  source_id   bigint NOT NULL REFERENCES raw.source(id),
  provenance  core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (champ, annee, age)
);

COMMENT ON TABLE core.population_age IS
  'Population au 1er janvier par âge révolu (INSEE, séries longues). Compte '
  'TOUS les résidents, étrangers compris : ce n''est pas le corps électoral. '
  'La ligne age_ouvert = true agrège tous les âges supérieurs : sommer sur '
  'age >= seuil reste juste, sommer sur age = valeur ne l''est pas.';

-- L'âge légal de vote, qui n'a pas toujours été 18 ans.
--
-- La loi du 5 juillet 1974 abaisse la majorité de 21 à 18 ans. Elle est donc
-- POSTÉRIEURE au scrutin des 5 et 19 mai 1974 : l'élection de Valéry Giscard
-- d'Estaing est la dernière à 21 ans. C'est le genre d'erreur d'un an qui fausse
-- silencieusement une série de soixante ans, et la seule protection est de
-- l'écrire dans une table plutôt que dans un commentaire de code.
CREATE TABLE ref.age_legal_vote (
  validity    daterange NOT NULL,
  age_minimum smallint NOT NULL CHECK (age_minimum > 0),
  fondement   text NOT NULL,
  EXCLUDE USING gist (validity WITH &&)
);

INSERT INTO ref.age_legal_vote (validity, age_minimum, fondement) VALUES
  ('[1958-10-04,1974-07-05)', 21, 'Ordonnance n° 58-1067 et droit antérieur : majorité électorale à 21 ans'),
  ('[1974-07-05,)',           18, 'Loi n° 74-631 du 5 juillet 1974 fixant à dix-huit ans l''âge de la majorité');

-- Le corps électoral potentiel à la date de chaque scrutin proclamé.
--
-- C'est une VUE et non une table : la valeur se recalcule à chaque lecture à
-- partir de core.population_age et de ref.age_legal_vote. La doctrine de derived
-- demande une method_version et une empreinte des entrées ; ici l'empreinte est
-- la vue elle-même, puisqu'elle ne peut pas diverger de ses entrées.
--
-- Méthode : somme des âges au-dessus du seuil légal aux deux 1er janvier qui
-- encadrent le scrutin, puis interpolation linéaire à la date exacte du tour.
-- L'interpolation est grossière — la population ne varie pas linéairement — mais
-- elle est explicite, reproductible, et l'erreur qu'elle introduit est de l'ordre
-- du pour mille, très en dessous de l'écart entre ce dénominateur et le corps
-- électoral réel.
CREATE VIEW derived.pdr_corps_electoral AS
WITH base AS (
  SELECT p.annee, p.tour, p.date_scrutin,
         (SELECT a.age_minimum FROM ref.age_legal_vote a
           WHERE a.validity @> p.date_scrutin) AS age_minimum,
         -- Le champ le plus large disponible pour l'année : France entière si la
         -- série existe, métropole sinon. La colonne est exposée pour que la
         -- rupture de série reste visible à la lecture.
         (SELECT CASE WHEN EXISTS (SELECT 1 FROM core.population_age f
                                    WHERE f.champ = 'FRANCE' AND f.annee = extract(year FROM p.date_scrutin))
                 THEN 'FRANCE' ELSE 'METRO' END) AS champ
    FROM ref.pdr_proclamation p
)
SELECT b.annee,
       b.tour,
       b.date_scrutin,
       b.age_minimum,
       b.champ,
       p0.pop AS pop_1er_janvier,
       p1.pop AS pop_1er_janvier_suivant,
       -- Interpolation à la date du scrutin.
       round(p0.pop + (p1.pop - p0.pop)
             * ((b.date_scrutin - make_date(extract(year FROM b.date_scrutin)::int, 1, 1))::numeric / 365.25)
            )::bigint AS population_en_age_de_voter,
       'corps-electoral-v1-interpolation-lineaire'::text AS method_version
  FROM base b
  JOIN LATERAL (SELECT sum(population) AS pop FROM core.population_age
                 WHERE champ = b.champ AND annee = extract(year FROM b.date_scrutin)
                   AND age >= b.age_minimum) p0 ON true
  JOIN LATERAL (SELECT sum(population) AS pop FROM core.population_age
                 WHERE champ = b.champ AND annee = extract(year FROM b.date_scrutin) + 1
                   AND age >= b.age_minimum) p1 ON true
 WHERE p0.pop IS NOT NULL AND p1.pop IS NOT NULL;

COMMENT ON VIEW derived.pdr_corps_electoral IS
  'Population en âge de voter à la date de chaque tour proclamé. Inclut les '
  'résidents étrangers, exclut les Français de l''étranger : c''est la VAP au '
  'sens international, pas le corps électoral. Un scrutin dont les deux années '
  'encadrantes ne sont pas chargées n''apparaît pas — jamais d''extrapolation.';

-- +goose Down
DROP VIEW derived.pdr_corps_electoral;
DROP TABLE ref.age_legal_vote;
DROP TABLE core.population_age;
