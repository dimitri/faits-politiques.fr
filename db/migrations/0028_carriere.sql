-- +goose Up
-- La carrière politique d'une personne, et son contexte.
--
-- Une fiche de candidat n'a de sens que si l'on voit la suite des mandats, et
-- sous quelle présidence chacun s'est déroulé. La présidence n'imputait rien
-- jusqu'ici parce qu'elle ne vivait que dans un CSV lu au moment du rendu :
-- impossible d'écrire une requête « quels mandats sous Hollande ». Elle entre
-- donc en base, comme un mandat parmi les autres.
--
-- Ce n'est PAS une imputation. Un ministre n'est pas responsable des actes du
-- président, ni l'inverse. La concomitance est un repère chronologique.
ALTER TYPE core.mandate_type ADD VALUE IF NOT EXISTS 'PRESIDENT_REPUBLIQUE';

-- +goose StatementBegin
DO $$ BEGIN
  -- Les intérims du président du Sénat comptent comme des périodes de la
  -- fonction : les omettre laisserait des trous dans la chronologie, et un
  -- mandat tombant dans un trou n'aurait pas de contexte du tout.
  NULL;
END $$;
-- +goose StatementEnd

-- +goose Down
-- PostgreSQL ne sait pas retirer une valeur d'un type énuméré. La descente
-- laisse donc la valeur en place : la signaler vaut mieux que la contourner
-- par une reconstruction du type et de toutes ses colonnes.
