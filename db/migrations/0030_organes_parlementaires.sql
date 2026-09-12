-- +goose Up
-- Les organes du Parlement, et les mandats qui y sont exercés.
--
-- 29 702 mandats de l'Assemblée dormaient dans raw.record, dont quatre types
-- seulement étaient normalisés (ASSEMBLEE, MINISTERE, GP, PARPOL). Les 23 000
-- autres décrivent où un député travaille réellement : commissions
-- permanentes, commissions mixtes paritaires, missions d'information,
-- délégations, groupes d'études, groupes d'amitié, organismes
-- extraparlementaires, bureau de l'Assemblée.
--
-- Ils remontent à 1988, alors que le reste du jeu ne couvre que la législature
-- en cours. C'est la seule profondeur historique dont le projet dispose.
ALTER TYPE core.organization_kind ADD VALUE IF NOT EXISTS 'PARLIAMENTARY_BODY';

-- +goose StatementBegin
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                 WHERE table_schema = 'core' AND table_name = 'organization'
                   AND column_name = 'organ_type') THEN
    -- Le code de l'Assemblée est conservé tel quel. Ranger un groupe d'amitié,
    -- une commission permanente et une mission d'information sous un même
    -- libellé serait une décision éditoriale ; la source les distingue, la base
    -- aussi.
    ALTER TABLE core.organization ADD COLUMN organ_type text;
    COMMENT ON COLUMN core.organization.organ_type IS
      'codeType publié par l''Assemblée (COMPER, GA, GE, DELEG, CMP…), '
      'conservé sans regroupement.';
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE core.organization DROP COLUMN IF EXISTS organ_type;
