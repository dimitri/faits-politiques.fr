-- +goose Up
-- Nomenclature des nuances, millésime 2020.
--
-- Les 24 codes ci-dessous sont ceux présents dans les fichiers de résultats des
-- 15 mars et 28 juin 2020, relevés dans les données et non devinés.
--
-- LNC N'EST PAS UNE NUANCE. Le code signifie « nuance non communiquée » et
-- couvre les communes situées sous le seuil d'attribution. Il apparaît 10 889
-- fois, dans des communes dont la médiane est de 1 219 inscrits et le maximum
-- de 4 447 — contre 4 975 de médiane pour les communes réellement nuancées. Il
-- est donc transcrit comme une ABSENCE de nuance, jamais comme une valeur.
--
-- Le seuil d'attribution a changé entre 2020 et 2026 : c'est précisément ce que
-- `circulaire_millesime` sert à distinguer. Comparer une nuance de 2020 à une
-- nuance de 2026 suppose de vérifier que la commune franchissait le seuil aux
-- deux dates.
INSERT INTO ref.nuance_politique (code, circulaire_millesime, libelle) VALUES
  ('LEXG', 2020, 'Liste d''extrême gauche'),
  ('LCOM', 2020, 'Liste du Parti communiste français'),
  ('LFI',  2020, 'Liste La France insoumise'),
  ('LSOC', 2020, 'Liste du Parti socialiste'),
  ('LRDG', 2020, 'Liste du Parti radical de gauche'),
  ('LVEC', 2020, 'Liste Europe Écologie-Les Verts'),
  ('LECO', 2020, 'Liste écologiste'),
  ('LDVG', 2020, 'Liste divers gauche'),
  ('LUG',  2020, 'Liste d''union de la gauche'),
  ('LREM', 2020, 'Liste La République en marche'),
  ('LMDM', 2020, 'Liste du Mouvement démocrate'),
  ('LUDI', 2020, 'Liste de l''Union des démocrates et indépendants'),
  ('LDVC', 2020, 'Liste divers centre'),
  ('LUC',  2020, 'Liste d''union du centre'),
  ('LLR',  2020, 'Liste Les Républicains'),
  ('LUD',  2020, 'Liste d''union de la droite'),
  ('LDVD', 2020, 'Liste divers droite'),
  ('LDLF', 2020, 'Liste Debout la France'),
  ('LRN',  2020, 'Liste du Rassemblement National'),
  ('LEXD', 2020, 'Liste d''extrême droite'),
  ('LREG', 2020, 'Liste régionaliste'),
  ('LGJ',  2020, 'Liste Gilets jaunes'),
  ('LNC',  2020, 'Nuance non communiquée — commune sous le seuil d''attribution'),
  ('LDIV', 2020, 'Liste divers')
ON CONFLICT (code, circulaire_millesime) DO NOTHING;

-- +goose Down
DELETE FROM ref.nuance_politique WHERE circulaire_millesime = 2020;
