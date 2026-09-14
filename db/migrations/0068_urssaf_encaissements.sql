-- +goose Up
-- Les montants réellement encaissés par les Urssaf, catégorie par catégorie,
-- région par région — le complément qui manquait à core.exoneration_cotisation
-- (ce que l'État allège) et core.masse_salariale (l'assiette) : ce qui est
-- effectivement PERÇU.
--
-- Trois millésimes seulement (2020-2022), et le jeu n'a plus été mis à jour
-- depuis le 5 juillet 2023 — voir docs/budget-donnees.md § 4.2, où il est déjà
-- signalé « dormant ». Chargé quand même : trois années exactes valent mieux
-- que zéro, et une table vide dirait moins que ces 525 lignes datées.
--
-- Le niveau géographique est la RÉGION (par Urssaf régionale, y compris les
-- CGSS d'outre-mer), pas le département : c'est la maille à laquelle les
-- caisses elles-mêmes existent. Aucun jeu URSSAF ne publie les encaissements
-- (par opposition à l'assiette, la masse salariale) à une maille plus fine.
CREATE TABLE core.encaissement_urssaf (
  id                         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee                      smallint NOT NULL,
  organisme                  text NOT NULL,
  region                     text NOT NULL,
  code_region                text NOT NULL,
  -- Six catégories, dont une seule répond à « les entreprises » : le secteur
  -- privé hors grandes entreprises nationales, et les grandes entreprises
  -- nationales elles-mêmes (SNCF, RATP…, qui cotisent à part). Secteur public,
  -- travailleurs indépendants et particuliers employeurs ne sont pas des
  -- entreprises ; les cotisations sur revenus de remplacement (chômage,
  -- retraite) ne sont pas des cotisations D'entreprise, elles sont prélevées
  -- SUR un revenu de remplacement. La colonne `categorie_entreprise` évite
  -- d'avoir à relire ce commentaire à chaque requête.
  categorie                  text NOT NULL,
  categorie_detaillee        text NOT NULL,
  categorie_entreprise       boolean NOT NULL,
  montant_eur                double precision NOT NULL,
  perimetre                  text NOT NULL REFERENCES ref.budget_perimetre(code),
  source_id                  bigint NOT NULL REFERENCES raw.source(id),
  document_id                bigint NOT NULL REFERENCES raw.document(id),
  UNIQUE (annee, organisme, categorie_detaillee)
);

COMMENT ON TABLE core.encaissement_urssaf IS
  'Encaissements annuels des Urssaf par région et catégorie (Urssaf Caisse '
  'nationale), 2020-2022 seulement — jeu non mis à jour depuis juillet 2023. '
  'Niveau géographique : la région (maille des caisses elles-mêmes), pas le '
  'département. Filtrer sur categorie_entreprise pour isoler les cotisations '
  'versées PAR des entreprises des autres encaissements (secteur public, '
  'indépendants, particuliers employeurs, revenus de remplacement).';

-- +goose Down
DROP TABLE core.encaissement_urssaf;
