-- +goose Up
-- L'effort de recherche : dépense intérieure de recherche et développement
-- (DIRD) rapportée au PIB, France et Union européenne (agrégat UE27 sur
-- toute la période), part des entreprises (DIRDE) isolée — Insee, à partir
-- de sources MESR-SIES (France) et OCDE (UE27), vérifié directement
-- (fichier Excel téléchargé, 33 années, 1990-2023). Pour
-- docs/recherche-enseignement-superieur-donnees.md.
CREATE TABLE core.effort_recherche (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee         int NOT NULL UNIQUE,
  dird_pib_fr   double precision,
  dird_pib_ue27 double precision,
  dirde_pib_fr  double precision,
  dirde_pib_ue27 double precision,
  estimation    boolean NOT NULL DEFAULT false,
  source_id     bigint NOT NULL REFERENCES raw.source(id)
);

COMMENT ON TABLE core.effort_recherche IS
  'DIRD/PIB et DIRDE/PIB (part des entreprises), France et UE27, 1990-2023 '
  '(Insee, sources MESR-SIES et OCDE). Le dernier point est une estimation, '
  'marquée comme telle par la source elle-même.';

-- +goose Down
DROP TABLE core.effort_recherche;
