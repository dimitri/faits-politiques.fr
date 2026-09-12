-- +goose Up
-- L'élection présidentielle, telle que le Conseil constitutionnel la proclame.
--
-- Le chiffre officiel d'une élection présidentielle n'est pas celui du ministère
-- de l'Intérieur le soir du scrutin : c'est celui que le Conseil constitutionnel
-- arrête dans sa décision de proclamation, après avoir tranché les réclamations
-- et, le cas échéant, ANNULÉ les suffrages de certains bureaux. Les deux nombres
-- diffèrent. Seul le second fait foi.
--
-- D'où la séparation en deux tables :
--   ref.pdr_proclamation  : la LISTE des décisions — ce qu'on est allé lire.
--   core.pdr_resultat     : les CHIFFRES — ce qu'on y a lu, transcrit mécaniquement.
--
-- La liste est semée ici parce qu'elle ne change pas : onze scrutins, onze
-- décisions, identifiées par leur numéro et leur publication au Journal officiel.
-- Un connecteur qui irait « découvrir » ces onze références serait une machine
-- compliquée pour un résultat figé — et introduirait le risque de charger la
-- mauvaise décision. Les chiffres, eux, sont extraits par le connecteur à partir
-- du document scellé : ils ne sont jamais écrits à la main dans une migration.

CREATE TABLE ref.pdr_proclamation (
  annee            integer NOT NULL,
  -- Seuls les seconds tours sont semés : ce sont eux qui proclament un élu.
  -- La colonne existe pour que les déclarations de premier tour puissent être
  -- ajoutées plus tard sans changer la clé.
  tour             smallint NOT NULL CHECK (tour IN (1, 2)),
  date_scrutin     date NOT NULL,
  decision_numero  text NOT NULL UNIQUE,
  decision_date    date NOT NULL,
  decision_url     text NOT NULL,
  -- La publication au JO : c'est elle qui fait foi, la page web n'en est que la
  -- reproduction. Un lien peut mourir, une référence au Journal officiel non.
  jorf             text NOT NULL,
  ecli             text,
  created_at       timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, tour),
  -- Une décision ne peut pas précéder le scrutin qu'elle proclame.
  CONSTRAINT proclamation_apres_scrutin CHECK (decision_date >= date_scrutin)
);

COMMENT ON TABLE ref.pdr_proclamation IS
  'Les décisions de proclamation des résultats de l''élection présidentielle. '
  'Liste close et vérifiée pièce par pièce : elle dit QUEL document fait foi, '
  'pas ce qu''il contient.';

INSERT INTO ref.pdr_proclamation
  (annee, tour, date_scrutin, decision_numero, decision_date, decision_url, jorf, ecli) VALUES
  (1965, 2, '1965-12-19', '65-10 PDR',   '1965-12-28',
   'https://www.conseil-constitutionnel.fr/decision/1965/6510pdr.htm',
   'Journal officiel du 30 décembre 1965, page 11916', NULL),
  (1969, 2, '1969-06-15', '69-22 PDR',   '1969-06-19',
   'https://www.conseil-constitutionnel.fr/decision/1969/6922pdr.htm',
   'Journal officiel du 20 juin 1969, page 6212', NULL),
  (1974, 2, '1974-05-19', '74-32 PDR',   '1974-05-24',
   'https://www.conseil-constitutionnel.fr/decision/1974/7432PDR.htm',
   'Journal officiel du 25 mai 1974, page 5669', NULL),
  (1981, 2, '1981-05-10', '81-47 PDR',   '1981-05-15',
   'https://www.conseil-constitutionnel.fr/decision/1981/8147pdr.htm',
   'Journal officiel du 16 mai 1981, page 1467', NULL),
  (1988, 2, '1988-05-08', '88-60 PDR',   '1988-05-11',
   'https://www.conseil-constitutionnel.fr/decision/1988/8860PDR.htm',
   'Journal officiel du 12 mai 1988, page 7036', NULL),
  (1995, 2, '1995-05-07', '95-81 PDR',   '1995-05-12',
   'https://www.conseil-constitutionnel.fr/decision/1995/9581PDR.htm',
   'Journal officiel du 14 mai 1995, page 8149', NULL),
  (2002, 2, '2002-05-05', '2002-111 PDR', '2002-05-08',
   'https://www.conseil-constitutionnel.fr/decision/2002/2002111PDR.htm',
   'Journal officiel du 10 mai 2002, page 9084', NULL),
  (2007, 2, '2007-05-06', '2007-141 PDR', '2007-05-10',
   'https://www.conseil-constitutionnel.fr/decision/2007/2007141pdr.htm',
   'Journal officiel du 11 mai 2007, page 8452, texte n° 1', NULL),
  (2012, 2, '2012-05-06', '2012-154 PDR', '2012-05-10',
   'https://www.conseil-constitutionnel.fr/decision/2012/2012154PDR.htm',
   'Journal officiel du 11 mai 2012, page 8997, texte n° 1', NULL),
  (2017, 2, '2017-05-07', '2017-171 PDR', '2017-05-10',
   'https://www.conseil-constitutionnel.fr/decision/2017/2017171PDR.htm',
   'JORF n° 0110 du 11 mai 2017, texte n° 1', 'ECLI:FR:CC:2017:2017.171.PDR'),
  (2022, 2, '2022-04-24', '2022-197 PDR', '2022-04-27',
   'https://www.conseil-constitutionnel.fr/decision/2022/2022197PDR.htm',
   'JORF n° 0099 du 28 avril 2022, texte n° 1', 'ECLI:FR:CC:2022:2022.197.PDR');

-- Les chiffres nationaux du tour, transcrits depuis la décision.
--
-- blancs et nuls sont NULLABLES, et c'est le fait le plus important de cette
-- table : avant 2017 le Conseil constitutionnel ne publiait NI l'un NI l'autre.
-- Il donnait inscrits, votants et suffrages exprimés, rien d'autre. En 2017 il
-- ajoute les bulletins blancs seuls ; en 2022 seulement, blancs et nuls séparés.
-- Écrire un zéro à la place d'un NULL, ou répartir la différence, serait inventer
-- une donnée que la source ne porte pas.
CREATE TABLE core.pdr_resultat (
  annee        integer NOT NULL,
  tour         smallint NOT NULL,
  inscrits     bigint NOT NULL CHECK (inscrits > 0),
  votants      bigint NOT NULL CHECK (votants > 0),
  blancs       bigint CHECK (blancs >= 0),
  nuls         bigint CHECK (nuls >= 0),
  exprimes     bigint NOT NULL CHECK (exprimes > 0),
  -- La seule quantité que la source ne donne jamais directement : votants moins
  -- suffrages exprimés. Colonne générée, donc impossible à contredire, et
  -- visiblement dérivée — on ne peut pas la confondre avec un chiffre proclamé.
  non_exprimes bigint GENERATED ALWAYS AS (votants - exprimes) STORED,
  source_id    bigint NOT NULL REFERENCES raw.source(id),
  document_id  bigint NOT NULL REFERENCES raw.document(id),
  provenance   core.provenance NOT NULL DEFAULT 'OFFICIAL',
  created_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, tour),
  FOREIGN KEY (annee, tour) REFERENCES ref.pdr_proclamation(annee, tour),
  CONSTRAINT votants_sous_inscrits CHECK (votants <= inscrits),
  CONSTRAINT exprimes_sous_votants CHECK (exprimes <= votants),
  -- Quand le détail est publié, il doit refermer l'identité comptable.
  CONSTRAINT detail_blancs_nuls_coherent CHECK (
    blancs IS NULL OR nuls IS NULL OR blancs + nuls = votants - exprimes)
);

COMMENT ON COLUMN core.pdr_resultat.blancs IS
  'NULL signifie « non publié par la décision », jamais « zéro ». Le Conseil '
  'constitutionnel ne publie les bulletins blancs qu''à partir de 2017, et les '
  'nuls séparément qu''à partir de 2022.';

-- Les voix par candidat, telles que la décision les énumère.
CREATE TABLE core.pdr_voix (
  annee       integer NOT NULL,
  tour        smallint NOT NULL,
  -- Le nom TEL QUE la décision l'écrit. Aucun rattachement à core.person ici :
  -- relier « Charles de Gaulle » à une fiche est une décision éditoriale, elle
  -- a sa place dans une table de liaison, pas dans la transcription.
  candidat    text NOT NULL,
  voix        bigint NOT NULL CHECK (voix >= 0),
  elu         boolean NOT NULL DEFAULT false,
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, tour, candidat),
  FOREIGN KEY (annee, tour) REFERENCES core.pdr_resultat(annee, tour) ON DELETE CASCADE
);

-- Un seul élu par tour proclamé.
CREATE UNIQUE INDEX pdr_voix_un_seul_elu ON core.pdr_voix (annee, tour) WHERE elu;

COMMENT ON TABLE core.pdr_voix IS
  'Voix obtenues par candidat au tour proclamé. Le connecteur refuse de charger '
  'un tour dont la somme des voix ne redonne pas exactement les suffrages '
  'exprimés : une transcription qui ne boucle pas est une transcription fausse.';

-- +goose Down
DROP TABLE core.pdr_voix;
DROP TABLE core.pdr_resultat;
DROP TABLE ref.pdr_proclamation;
