-- +goose Up
-- L'écart de prix entre les départements d'outre-mer et la France
-- métropolitaine — Insee, enquête de comparaison spatiale des prix (ECSP),
-- dernière édition 2022 (Insee Première n° 1958, juillet 2023), vérifiée
-- directement (fichier Excel joint à la publication). Une enquête
-- ponctuelle, pas une série annuelle : les millésimes disponibles sont
-- 1985, 1992, 2010, 2015 et 2022 pour l'indice général, cette table ne
-- retient que les trois derniers, seuls comparables entre eux selon
-- l'Insee elle-même (méthode harmonisée depuis 2010).
CREATE TABLE core.ecart_prix_dom (
  id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  territoire      text NOT NULL,
  annee           int NOT NULL,
  fisher_general_pct     double precision,
  fisher_alimentaire_pct double precision,
  source_id       bigint NOT NULL REFERENCES raw.source(id),
  UNIQUE (territoire, annee)
);

COMMENT ON TABLE core.ecart_prix_dom IS
  'Écart de prix (indice de Fisher, moyenne géométrique de deux paniers de '
  'consommation) entre chaque DOM et la France métropolitaine = 0 %. '
  'fisher_alimentaire_pct isole les produits alimentaires et boissons non '
  'alcoolisées, toujours nettement supérieur à l''écart général — les deux ne '
  'doivent jamais être confondus (voir docs/outre-mer-donnees.md).';

-- +goose Down
DROP TABLE core.ecart_prix_dom;
