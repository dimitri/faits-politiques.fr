-- +goose Up
-- La DGF (dotation globale de fonctionnement) : le principal transfert de
-- l'État vers les collectivités, celui qui répond directement à « d'où vient
-- l'argent » (docs/collectivites-donnees.md). Publiée par l'OFGL dans le même
-- agrégat déjà utilisé pour dette/investissement/fonctionnement/masse
-- salariale/épargne : un seul indicateur de plus dans un connecteur déjà en
-- place (internal/communes/ofgl.go), pas un nouveau chargement.
-- Recettes totales et impôts et taxes complètent la DGF pour répondre à
-- « d'où vient l'argent » (docs/collectivites-donnees.md § 1) : la part de
-- fiscalité propre vs la part de transferts de l'État ne se lit qu'en
-- comparant ces trois agrégats entre eux, jamais un seul isolément.
INSERT INTO ref.indicator (code, label, unit, producer, formula, caveat, competence_code, comparable) VALUES
('ofgl.dgf_par_hab','Dotation globale de fonctionnement par habitant','EUR_PAR_HABITANT','OFGL',
 'dotation_globale_de_fonctionnement / population_municipale',
 'La DGF se décompose en dotation forfaitaire, de solidarité rurale/urbaine et '
 'd''intercommunalité selon le niveau — cet agrégat OFGL est leur somme, pas le détail par '
 'composante. Une baisse peut venir d''un écrêtement de péréquation, pas d''une décision '
 'ponctuelle contre la commune.', NULL, true),
('ofgl.recettes_totales_par_hab','Recettes totales par habitant','EUR_PAR_HABITANT','OFGL',
 'recettes_totales / population_municipale',
 'Additionne fonctionnement et investissement, dotations et fiscalité propre : un total, pas '
 'une décomposition — croiser avec ofgl.impots_taxes_par_hab et ofgl.dgf_par_hab pour '
 'distinguer l''origine des recettes.', NULL, true),
('ofgl.impots_taxes_par_hab','Impôts et taxes par habitant','EUR_PAR_HABITANT','OFGL',
 'impots_et_taxes / population_municipale',
 'La fiscalité propre (taxe foncière, part de CFE/CVAE reversée...), à ne pas confondre avec '
 'les dotations de l''État (ofgl.dgf_par_hab) : deux origines différentes de la même colonne '
 '« recettes ».', NULL, true);

-- +goose Down
DELETE FROM ref.indicator WHERE code IN
  ('ofgl.dgf_par_hab','ofgl.recettes_totales_par_hab','ofgl.impots_taxes_par_hab');
