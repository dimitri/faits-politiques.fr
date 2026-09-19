-- +goose Up
-- Le budget hospitalier historique : la Drees publie, dans le fichier
-- Excel qui accompagne chaque édition de son Panorama « Les établissements
-- de santé » (fiche « La situation économique et financière des hôpitaux
-- publics »), une série 2005-2024 du compte de résultat des hôpitaux
-- publics — vérifié directement (fichier téléchargé et inspecté, pas
-- deviné depuis la seule PDF). Chargées ici : le compte de résultat
-- (Graphique 1, une valeur par indicateur et par année, France entière)
-- et le déficit en % des recettes par catégorie d'établissement
-- (Tableau 1). Les autres feuilles du même fichier (effort
-- d'investissement, capacité d'autofinancement, dotation aux
-- amortissements, surendettement, marge brute) empilent plusieurs
-- sous-tableaux dans une même feuille sans repère structurel — un
-- chargement plus fin y demanderait un découpage par bloc, pas fait ici.
CREATE TABLE core.hopital_public_resultat (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee       smallint NOT NULL,
  indicateur  text NOT NULL CHECK (indicateur IN (
                'RESULTAT_EXPLOITATION', 'RESULTAT_FINANCIER', 'RESULTAT_EXCEPTIONNEL', 'RESULTAT_NET')),
  montant_meur numeric NOT NULL,
  source_id   bigint NOT NULL REFERENCES raw.source(id)
);
CREATE UNIQUE INDEX hopital_public_resultat_uniq ON core.hopital_public_resultat (annee, indicateur);

COMMENT ON TABLE core.hopital_public_resultat IS
  'Compte de résultat des hôpitaux publics, France entière (Drees, Panorama ES, fiche '
  '« situation économique et financière des hôpitaux publics », Graphique 1), 2005-2024, '
  'en millions d''euros. RESULTAT_NET = EXPLOITATION + FINANCIER + EXCEPTIONNEL '
  '(vérifié à l''ingestion, à l''arrondi près).';

CREATE TABLE core.hopital_public_deficit_categorie (
  id                     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee                  smallint NOT NULL,
  categorie              text NOT NULL,
  deficit_pct_recettes   numeric NOT NULL,
  source_id              bigint NOT NULL REFERENCES raw.source(id)
);
CREATE UNIQUE INDEX hopital_public_deficit_categorie_uniq ON core.hopital_public_deficit_categorie (annee, categorie);

COMMENT ON TABLE core.hopital_public_deficit_categorie IS
  'Excédent (positif) ou déficit (négatif) des hôpitaux publics, en % des recettes, par '
  'catégorie d''établissement (Drees, Panorama ES, même fiche, Tableau 1), 2005-2024. '
  '« Ensemble des hôpitaux publics » et les sept catégories (AP-HP, autres CHR, CH '
  'spécialisés en psychiatrie, CH ex-hôpitaux locaux, très grands/grands/moyens/petits '
  'CH) ne forment pas nécessairement une partition stricte qui s''additionne à '
  '« Ensemble » — la note méthodologique de la source ne le garantit pas explicitement, '
  'jamais supposé ici sans vérification.';

-- +goose Down
DROP TABLE core.hopital_public_deficit_categorie;
DROP TABLE core.hopital_public_resultat;
