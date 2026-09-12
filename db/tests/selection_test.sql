-- Garanties des migrations 0009, 0010 et 0012 : pré-enregistrement, piège CCAS,
-- rattachement d'un exercice à un mandat, et filtres (schéma selection).
--
--   psql -v ON_ERROR_STOP=1 -d <db> -f db/tests/selection_test.sql

BEGIN;

DO $$
DECLARE
  ok int := 0;
  ko text[] := '{}';

  v_fk_count int;
  v_src      bigint;
  v_scrutin  bigint;
  v_prereg   bigint;
  v_person   bigint;
  v_tmpl     bigint;
  v_dcrit    bigint;
  v_dman     bigint;
  v_couv     numeric;
BEGIN
  ------------------------------------------------------------------
  -- 1. Subordination : aucune clé étrangère ne remonte de core/derived
  --    vers selection. Un filtre ne peut jamais modifier un fait.
  ------------------------------------------------------------------
  SELECT count(*) INTO v_fk_count
  FROM pg_constraint c
  JOIN pg_class src ON src.oid = c.conrelid
  JOIN pg_namespace sn ON sn.oid = src.relnamespace
  JOIN pg_class tgt ON tgt.oid = c.confrelid
  JOIN pg_namespace tn ON tn.oid = tgt.relnamespace
  WHERE c.contype = 'f'
    AND sn.nspname IN ('raw','ref','core','derived')
    AND tn.nspname = 'selection';
  IF v_fk_count = 0 THEN ok := ok + 1;
  ELSE ko := ko || format('%s clé(s) étrangère(s) de core/derived vers selection', v_fk_count)::text;
  END IF;

  ------------------------------------------------------------------
  -- Jeu de données minimal
  ------------------------------------------------------------------
  INSERT INTO raw.source (slug, label, publisher, tier, licence, reuse_class, attribution_text)
  VALUES ('zzt-an','AN','Assemblée nationale','PRIMARY_OFFICIAL','Licence Ouverte 2.0', 'ATTRIBUTION', 'AN')
  RETURNING id INTO v_src;

  INSERT INTO core.scrutin (slug, institution, source_uid, date_seance, granularite, objet)
  VALUES ('zzt-an-1','ASSEMBLEE_NATIONALE','A1','2025-03-01','INDIVIDUAL','Objet')
  RETURNING id INTO v_scrutin;

  INSERT INTO selection.template (slug, label, subject_kind, criteria)
  VALUES ('zzt-votes-par-groupe','Votes par groupe','PARLIAMENTARY_GROUP','{"topic":"$subject"}'::jsonb)
  RETURNING id INTO v_tmpl;

  ------------------------------------------------------------------
  -- 2. Un dossier par critères ne peut pas contenir de faits choisis à la main.
  ------------------------------------------------------------------
  INSERT INTO selection.dossier (slug, label, universe_criteria, pick_mode)
  VALUES ('zzt-filtre-criteres','Filtre par critères','{"institution":"AN"}'::jsonb,'CRITERIA')
  RETURNING id INTO v_dcrit;

  BEGIN
    INSERT INTO selection.dossier_item (dossier_id, ordinal, scrutin_id)
    VALUES (v_dcrit, 1, v_scrutin);
    ko := ko || 'fait choisi à la main dans un dossier par critères'::text;
  EXCEPTION WHEN raise_exception THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 3. Un dossier manuel accepte des faits, un par ligne exactement.
  ------------------------------------------------------------------
  INSERT INTO selection.dossier (slug, label, universe_criteria, pick_mode)
  VALUES ('zzt-filtre-manuel','Filtre manuel','{"institution":"AN"}'::jsonb,'MANUAL_SUBSET')
  RETURNING id INTO v_dman;

  INSERT INTO selection.dossier_item (dossier_id, ordinal, scrutin_id) VALUES (v_dman, 1, v_scrutin);
  ok := ok + 1;

  BEGIN
    INSERT INTO selection.dossier_item (dossier_id, ordinal) VALUES (v_dman, 2);
    ko := ko || 'élément de dossier sans fait accepté'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 4. Publication refusée tant que la sélectivité n'est pas calculée.
  --    Sans prose, le filtre EST l'argument : il doit déclarer ce qu'il écarte.
  ------------------------------------------------------------------
  BEGIN
    UPDATE selection.dossier SET listed = true WHERE id = v_dman;
    ko := ko || 'dossier listé sans sélectivité calculée'::text;
  EXCEPTION WHEN raise_exception THEN ok := ok + 1;
  END;

  BEGIN
    UPDATE selection.dossier SET frozen_at = now(), content_hash = sha256('x'::bytea)
     WHERE id = v_dman;
    ko := ko || 'dossier gelé sans sélectivité calculée'::text;
  EXCEPTION WHEN raise_exception THEN ok := ok + 1;
  END;

  -- Une sélection ne peut pas dépasser son univers.
  BEGIN
    INSERT INTO selection.dossier_selectivity (dossier_id, universe_size, selected_size, method_version)
    VALUES (v_dman, 10, 12, 'v1');
    ko := ko || 'sélection plus grande que son univers acceptée'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  INSERT INTO selection.dossier_selectivity (dossier_id, universe_size, selected_size, method_version)
  VALUES (v_dman, 214, 1, 'v1');
  UPDATE selection.dossier SET listed = true WHERE id = v_dman;
  ok := ok + 1;

  ------------------------------------------------------------------
  -- 5. Un sujet ne va pas sans gabarit, et réciproquement.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO selection.dossier (slug, label, universe_criteria, pick_mode, subject_key)
    VALUES ('zzt-orphelin','Sujet sans gabarit','{}'::jsonb,'CRITERIA','zzt-groupe-a');
    ko := ko || 'sujet sans gabarit accepté'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 6. Pré-enregistrement : un calcul ne peut se rattacher qu'à un protocole
  --    scellé, et ne peut pas le précéder.
  ------------------------------------------------------------------
  INSERT INTO core.preregistration (slug, title, document_path, content_hash, method_version, population_def)
  VALUES ('zzt-pre-001','Protocole 001','docs/pre-enregistrement-001.md',
          sha256('protocole'::bytea),'v1','{}'::jsonb)
  RETURNING id INTO v_prereg;

  BEGIN
    INSERT INTO derived.comparison_run
      (slug, indicator_code, period_year, matching_vars, method_version,
       n_communes, n_sans_nuance, taux_exclusion, result, preregistration_id)
    VALUES ('zzt-run-1','ofgl.dette_par_hab',2025,'{strate}','v1',10,2,0.2,'{}'::jsonb, v_prereg);
    ko := ko || 'calcul rattaché à un protocole non scellé'::text;
  EXCEPTION WHEN raise_exception THEN ok := ok + 1;
  END;

  UPDATE core.preregistration SET sealed_at = now() WHERE id = v_prereg;

  BEGIN
    INSERT INTO derived.comparison_run
      (slug, indicator_code, period_year, matching_vars, method_version, computed_at,
       n_communes, n_sans_nuance, taux_exclusion, result, preregistration_id)
    VALUES ('zzt-run-2','ofgl.dette_par_hab',2025,'{strate}','v1', now() - interval '1 day',
            10,2,0.2,'{}'::jsonb, v_prereg);
    ko := ko || 'calcul antérieur au scellement accepté'::text;
  EXCEPTION WHEN raise_exception THEN ok := ok + 1;
  END;

  INSERT INTO derived.comparison_run
    (slug, indicator_code, period_year, matching_vars, method_version,
     n_communes, n_sans_nuance, taux_exclusion, result, preregistration_id)
  VALUES ('zzt-run-3','ofgl.dette_par_hab',2025,'{strate}','v1',10,2,0.2,'{}'::jsonb, v_prereg);
  ok := ok + 1;

  ------------------------------------------------------------------
  -- 7. Piège CCAS : deux périmètres budgétaires coexistent, un doublon non.
  ------------------------------------------------------------------
  INSERT INTO ref.commune (code_insee, cog_millesime, nom, code_departement, code_region, population_municipale)
  VALUES ('99999', 2025, 'Commune test', '62', '32', 26000);

  INSERT INTO core.commune_indicator
    (commune_code, cog_millesime, indicator_code, period_year, value, source_id, budget_scope)
  VALUES ('99999',2025,'dgfip.f5_depenses_par_hab',2024, 120, v_src, 'COMMUNE');
  INSERT INTO core.commune_indicator
    (commune_code, cog_millesime, indicator_code, period_year, value, source_id, budget_scope)
  VALUES ('99999',2025,'dgfip.f5_depenses_par_hab',2024,  45, v_src, 'CCAS');
  ok := ok + 1;

  BEGIN
    INSERT INTO core.commune_indicator
      (commune_code, cog_millesime, indicator_code, period_year, value, source_id, budget_scope)
    VALUES ('99999',2025,'dgfip.f5_depenses_par_hab',2024, 999, v_src, 'COMMUNE');
    ko := ko || 'doublon sur le même périmètre budgétaire accepté'::text;
  EXCEPTION WHEN unique_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 8. Une année d'élection est comptée au prorata du mandat.
  ------------------------------------------------------------------
  INSERT INTO core.person (slug, family_name, given_name)
  VALUES ('zzt-maire-test','Test','Maire') RETURNING id INTO v_person;
  INSERT INTO core.mandate (person_id, mandate_type, commune_code, validity)
  VALUES (v_person, 'MAIRE', '99999', daterange('2024-07-01', NULL));

  SELECT couverture_annee INTO v_couv
  FROM core.commune_indicator_mandate
  WHERE commune_code = '99999' AND period_year = 2024 AND budget_scope = 'COMMUNE';

  IF v_couv BETWEEN 0.49 AND 0.52 THEN ok := ok + 1;
  ELSE ko := ko || format('couverture d''exercice partagé incorrecte : %s', v_couv)::text;
  END IF;

  ------------------------------------------------------------------
  IF array_length(ko, 1) IS NULL THEN
    RAISE NOTICE '% garanties vérifiées — OK', ok;
  ELSE
    RAISE EXCEPTION 'Garanties non tenues : %', array_to_string(ko, ' | ');
  END IF;
END $$;

ROLLBACK;
