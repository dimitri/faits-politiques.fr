-- +goose Up
-- La dépense moyenne par élève ou étudiant (RERS, tableau 10.05) — citée
-- depuis la version 1 de docs/education-donnees.md (§ 1, "environ
-- 8 450 €/an en primaire") sans jamais être chargée, faute de jeu de
-- données ouvert structuré identifié à l'époque (§ 6, "Non chargé, et
-- pourquoi"). RERS Interactif (rers.depp.education.fr) sert en réalité
-- chaque tableau en CSV à une URL prévisible
-- (/data/<édition>/<thème>/<sous-thème>/<figure>/data.csv), simplement non
-- documentée ni liée depuis les pages HTML du site — trouvée en rejouant le
-- trafic réseau de l'application JavaScript (voir internal/education/depense_eleve.go).
CREATE TABLE core.education_depense_eleve (
  id                      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee                   integer NOT NULL,
  niveau                  text NOT NULL, -- premier_degre | second_degre | superieur | ensemble
  provisoire              boolean NOT NULL DEFAULT false,
  depense_euros_constants numeric NOT NULL,
  prix_annee              integer NOT NULL, -- année de base des euros constants (ex. 2024)
  edition_rers            integer NOT NULL, -- millésime de l'édition RERS Interactif interrogée
  source_id               bigint NOT NULL REFERENCES raw.source(id),
  created_at              timestamptz NOT NULL DEFAULT now(),
  UNIQUE (annee, niveau)
);

-- +goose Down
DROP TABLE core.education_depense_eleve;
