-- +goose Up
-- Le rattachement d'un dossier législatif à la loi promulguée qui en est
-- sortie — sur une ÉGALITÉ EXACTE entre deux clés publiées, jamais un
-- rapprochement de titres (voir internal/carto/themes.go pour le même
-- principe appliqué aux thèmes de scrutin, et pourquoi une similarité de
-- titre y est explicitement écartée).
--
-- La clé : chaque dossier de l'Assemblée qui aboutit à une loi porte, dans
-- son propre flux `an.dossierParlementaire` (déjà scellé dans raw.record),
-- une étape « PROM » dont la sous-étape « PROM-PUB » donne codeLoi
-- (« 2024-1017 »), la référence NOR (« EAEJ2333583L ») et la date JO. Le
-- Journal officiel publie CETTE MÊME référence NOR sur la loi elle-même
-- (jo.texte.nor). Les deux se rejoignent donc sans deviner : vérifié sur
-- plusieurs dizaines de dossiers avant d'écrire cette migration, 100 %
-- d'égalités exactes, aucune approximation.
CREATE TABLE core.dossier_promulgation (
  dossier_id          bigint PRIMARY KEY REFERENCES core.dossier(id) ON DELETE CASCADE,
  code_loi            text NOT NULL,
  reference_nor       text NOT NULL,
  numero_jo           text,
  date_jo             date,
  date_promulgation   date,
  jo_texte_id         text REFERENCES jo.texte(id),
  UNIQUE (reference_nor)
);

COMMENT ON TABLE core.dossier_promulgation IS
  'Un dossier législatif -> la loi qui en est sortie, par égalité exacte de '
  'référence NOR entre le flux dossierParlementaire de l''Assemblée '
  '(raw.record) et le Journal officiel (jo.texte.nor). jo_texte_id est NULL '
  'quand la loi existe côté Assemblée mais n''a pas (encore) été retrouvée '
  'dans le corpus JORF chargé — pas une absence de loi, une absence de '
  'jointure.';

-- +goose Down
DROP TABLE core.dossier_promulgation;
