-- +goose Up
-- Le fonctionnement électoral français : qui est élu, par qui, pour combien de
-- temps.
--
-- Ce n'est pas une donnée ouverte : c'est du DROIT. Aucun jeu de données ne
-- publie « le mandat municipal dure six ans » — cela s'écrit dans le code
-- électoral et le code général des collectivités territoriales. La table
-- transcrit donc la règle, avec l'article qui la fonde, et non un fichier
-- téléchargé.
--
-- Elle sert à deux choses que rien d'autre ne permet :
--
--  1. Savoir si un mandat observé est complet ou en cours, sans quoi comparer
--     deux mandatures compare des durées différentes.
--  2. Distinguer les élus AU SUFFRAGE DIRECT de ceux désignés par d'autres
--     élus. Un président d'intercommunalité, un sénateur, un président de
--     département ne sont choisis par aucun électeur directement — et c'est un
--     fait politique de premier ordre quand on documente qui décide.
CREATE TABLE ref.type_election (
  code            text PRIMARY KEY,
  libelle         text NOT NULL,
  -- Le mandat qu'elle pourvoit, quand il correspond à un type suivi.
  mandate_type    core.mandate_type,
  duree_ans       integer NOT NULL,
  -- DIRECT   : les électeurs inscrits votent pour ce mandat.
  -- INDIRECT : le titulaire est désigné par d'autres élus.
  suffrage        text NOT NULL CHECK (suffrage IN ('DIRECT','INDIRECT')),
  -- Qui vote. Pour un suffrage indirect, c'est ce qui compte.
  corps_electoral text NOT NULL,
  mode_scrutin    text NOT NULL,
  -- L'article qui fonde la règle. Sans lui, la ligne est une affirmation sans
  -- source, ce que ce projet n'admet nulle part.
  fondement       text NOT NULL,
  renouvellement  text
);

COMMENT ON TABLE ref.type_election IS
  'Règles du droit électoral, transcrites avec leur fondement. Ce n''est pas '
  'un jeu de données : aucune source ouverte ne publie la durée d''un mandat.';

INSERT INTO ref.type_election
  (code, libelle, mandate_type, duree_ans, suffrage, corps_electoral, mode_scrutin, fondement, renouvellement) VALUES
  ('PRESIDENTIELLE', 'Élection présidentielle', 'PRESIDENT_REPUBLIQUE', 5, 'DIRECT',
   'Électeurs inscrits sur les listes électorales',
   'Scrutin uninominal majoritaire à deux tours',
   'Constitution, art. 6 et 7 ; durée portée de 7 à 5 ans par la loi constitutionnelle du 2 octobre 2000',
   'Intégral'),

  ('LEGISLATIVE', 'Élections législatives', 'DEPUTE', 5, 'DIRECT',
   'Électeurs inscrits, par circonscription',
   'Scrutin uninominal majoritaire à deux tours',
   'Code électoral, art. L.121 et L.123',
   'Intégral, sauf dissolution'),

  ('SENATORIALE', 'Élections sénatoriales', 'SENATEUR', 6, 'INDIRECT',
   'Grands électeurs : députés, conseillers régionaux et départementaux, et surtout délégués des conseils municipaux — environ 95 % du collège',
   'Majoritaire à deux tours dans les départements élisant 1 ou 2 sénateurs, proportionnel au-delà',
   'Code électoral, art. L.O.276 et L.280',
   'Par moitié tous les trois ans'),

  ('EUROPEENNE', 'Élections européennes', 'DEPUTE_EUROPEEN', 5, 'DIRECT',
   'Électeurs inscrits, circonscription nationale unique depuis 2019',
   'Proportionnel à la plus forte moyenne, seuil de 5 %',
   'Loi n° 77-729 du 7 juillet 1977, modifiée par la loi du 25 juin 2018',
   'Intégral'),

  ('MUNICIPALE', 'Élections municipales', 'CONSEILLER_MUNICIPAL', 6, 'DIRECT',
   'Électeurs inscrits dans la commune',
   'Scrutin de liste à deux tours avec prime majoritaire dans les communes de 1 000 habitants et plus ; scrutin plurinominal majoritaire en deçà',
   'Code électoral, art. L.227, L.252 et L.260',
   'Intégral'),

  ('MAIRE', 'Élection du maire', 'MAIRE', 6, 'INDIRECT',
   'Conseil municipal',
   'Scrutin secret à la majorité absolue aux deux premiers tours, relative au troisième',
   'CGCT, art. L.2122-4 et L.2122-7',
   'À chaque renouvellement du conseil'),

  ('COMMUNAUTAIRE', 'Conseillers communautaires', 'CONSEILLER_COMMUNAUTAIRE', 6, 'DIRECT',
   'Électeurs inscrits dans la commune, par fléchage sur la liste municipale (communes de 1 000 habitants et plus)',
   'Fléchage sur le bulletin municipal ; désignation par le conseil municipal dans les communes de moins de 1 000 habitants',
   'Code électoral, art. L.273-6 et L.273-11',
   'Intégral'),

  ('PRESIDENCE_EPCI', 'Président d''intercommunalité', NULL, 6, 'INDIRECT',
   'Conseil communautaire — donc aucun électeur directement',
   'Scrutin secret à la majorité absolue',
   'CGCT, art. L.5211-2 renvoyant à L.2122-7',
   'À chaque renouvellement du conseil communautaire'),

  ('DEPARTEMENTALE', 'Élections départementales', 'CONSEILLER_DEPARTEMENTAL', 6, 'DIRECT',
   'Électeurs inscrits dans le canton',
   'Scrutin binominal mixte majoritaire à deux tours, une femme et un homme par binôme',
   'Code électoral, art. L.191 et L.192, loi du 17 mai 2013',
   'Intégral depuis 2015 ; auparavant par moitié tous les trois ans'),

  ('PRESIDENCE_DEPARTEMENT', 'Président du conseil départemental', NULL, 6, 'INDIRECT',
   'Conseil départemental',
   'Scrutin secret à la majorité absolue aux deux premiers tours',
   'CGCT, art. L.3122-1',
   'À chaque renouvellement du conseil'),

  ('REGIONALE', 'Élections régionales', 'CONSEILLER_REGIONAL', 6, 'DIRECT',
   'Électeurs inscrits dans la région, sections départementales',
   'Scrutin de liste à deux tours avec prime majoritaire de 25 % des sièges',
   'Code électoral, art. L.336 et L.338',
   'Intégral'),

  ('PRESIDENCE_REGION', 'Président du conseil régional', NULL, 6, 'INDIRECT',
   'Conseil régional',
   'Scrutin secret à la majorité absolue aux deux premiers tours',
   'CGCT, art. L.4133-1',
   'À chaque renouvellement du conseil');

-- Les scrutins qui ont réellement eu lieu. Une règle dit la durée théorique ;
-- seules les dates disent ce qui s'est passé — dissolutions, reports, et la
-- réforme de 2015 qui a fait passer les départementales de la moitié au tout.
CREATE TABLE ref.scrutin_national (
  type_election text NOT NULL REFERENCES ref.type_election(code),
  annee         integer NOT NULL,
  tour1         date,
  tour2         date,
  note          text,
  PRIMARY KEY (type_election, annee)
);

COMMENT ON TABLE ref.scrutin_national IS
  'Dates des scrutins effectivement tenus. À compléter au fil du chargement des '
  'résultats : chaque jeu de résultats du ministère de l''Intérieur porte sa date.';

-- Seuls les scrutins dont le projet détient les résultats sont inscrits ici.
-- Les autres seront ajoutés quand leurs résultats le seront : une date sans
-- données derrière n'aurait aucun usage.
INSERT INTO ref.scrutin_national (type_election, annee, tour1, tour2, note) VALUES
  ('MUNICIPALE', 2020, '2020-03-15', '2020-06-28',
   'Second tour reporté de trois mois par la loi du 23 mars 2020 (épidémie de covid-19)'),
  ('MUNICIPALE', 2026, '2026-03-15', '2026-03-22', NULL);

-- +goose Down
DROP TABLE ref.scrutin_national;
DROP TABLE ref.type_election;
