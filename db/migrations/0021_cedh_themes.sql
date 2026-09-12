-- +goose Up
-- Regroupements d'articles de la Convention.
--
-- Ce sont des REGROUPEMENTS ÉDITORIAUX, publiés et contestables. La Cour ne
-- classe pas ses arrêts par thème : elle statue article par article.
INSERT INTO ref.cedh_theme (code, libelle, articles, definition) VALUES
 ('VIE_INTEGRITE','Vie et intégrité physique','{2,3}',
  'Article 2 — droit à la vie. Article 3 — interdiction de la torture et des traitements '
  'inhumains ou dégradants.'),
 ('LIBERTE','Liberté et sûreté','{5}',
  'Article 5 — droit à la liberté et à la sûreté : arrestation, garde à vue, détention.'),
 ('PROCES','Procès équitable et recours','{6,13}',
  'Article 6 — droit à un procès équitable, y compris le délai raisonnable. Article 13 — '
  'droit à un recours effectif. C''est le contentieux le plus nombreux contre la France.'),
 ('VIE_PRIVEE','Vie privée et familiale','{8}',
  'Article 8 — respect de la vie privée et familiale, du domicile et de la correspondance.'),
 ('EXPRESSION','Expression, réunion, association','{10,11}',
  'Article 10 — liberté d''expression. Article 11 — liberté de réunion et d''association.'),
 ('DISCRIMINATION','Non-discrimination','{14}',
  'Article 14 — interdiction de la discrimination dans la jouissance des autres droits.'),
 ('FORCES_ORDRE','Articles le plus souvent engagés par l''action des forces de l''ordre','{2,3,5}',
  'REGROUPEMENT ÉDITORIAL ET IMPARFAIT. Les articles 2, 3 et 5 sont ceux que l''action des '
  'forces de l''ordre engage le plus souvent. Mais un arrêt rendu sur ces articles ne '
  'concerne PAS nécessairement la police — ils couvrent aussi les conditions de détention, '
  'l''éloignement d''étrangers, ou les obligations de l''État en matière de protection. '
  'Et inversement, une action policière peut engager d''autres articles. Ce filtre est une '
  'aide à la lecture, pas une catégorie juridique.'),
 ('PROCEDURE','Articles de procédure, pas des droits substantiels','{29,35,41,46}',
  'Article 41 — satisfaction équitable, c''est-à-dire l''indemnisation. Articles 29 et 35 — '
  'recevabilité. Article 46 — exécution des arrêts. Leur présence dans le dispositif d''un '
  'arrêt ne dit rien du droit qui a été violé : ils sont exclus des regroupements '
  'thématiques.');

-- +goose Down
DELETE FROM ref.cedh_theme;
