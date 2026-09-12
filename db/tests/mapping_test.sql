-- Garanties de la migration 0011 : lignées et révisions de cartes de rattachement,
-- unicité de l'attribution EXACT, étanchéité du chemin d'agrégation local.
--
--   psql -v ON_ERROR_STOP=1 -d <db> -f db/tests/mapping_test.sql

BEGIN;

DO $$
DECLARE
  ok int := 0;
  ko text[] := '{}';

  v_ref_lin  bigint; v_ref_rev bigint;
  v_com_lin  bigint; v_com_rev bigint;
  v_partiA   bigint; v_partiB  bigint;
  v_person   bigint; v_mandat  bigint;
  v_prereg   bigint;
  v_n        int;
BEGIN
  ------------------------------------------------------------------
  -- Jeu de données minimal
  ------------------------------------------------------------------
  INSERT INTO core.organization (slug, kind, name) VALUES ('zzt-parti-a','PARTY','Parti A')
    RETURNING id INTO v_partiA;
  INSERT INTO core.organization (slug, kind, name) VALUES ('zzt-parti-b','PARTY','Parti B')
    RETURNING id INTO v_partiB;
  INSERT INTO ref.nuance_politique (code, circulaire_millesime, libelle)
    VALUES ('ZZTX', 2026, 'Liste Parti A');
  INSERT INTO ref.nuance_politique (code, circulaire_millesime, libelle)
    VALUES ('ZZTD', 2026, 'Liste divers droite');

  INSERT INTO core.mapping_lineage (slug, label, kind, listed)
    VALUES ('zzt-reference','Carte de référence','REFERENCE', true) RETURNING id INTO v_ref_lin;
  INSERT INTO core.mapping_revision (lineage_id, revision)
    VALUES (v_ref_lin, 1) RETURNING id INTO v_ref_rev;

  ------------------------------------------------------------------
  -- 1. Une seule carte de référence.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.mapping_lineage (slug, label, kind, listed)
    VALUES ('zzt-reference-bis','Seconde référence','REFERENCE', true);
    ko := ko || 'deuxième carte de référence acceptée'::text;
  EXCEPTION WHEN unique_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 2. La référence est listée et ne porte pas de jeton d'édition anonyme.
  ------------------------------------------------------------------
  BEGIN
    UPDATE core.mapping_lineage SET edit_token_hash = sha256('t'::bytea) WHERE id = v_ref_lin;
    ko := ko || 'jeton d''édition anonyme sur la carte de référence'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 3. Une carte communautaire se crée sans compte, par jeton de capacité,
  --    et n'est pas listée par défaut.
  ------------------------------------------------------------------
  INSERT INTO core.mapping_lineage (slug, label, kind, edit_token_hash)
  VALUES ('zzt-carte-tiers','Carte d''un contradicteur','COMMUNITY', sha256('jeton'::bytea))
  RETURNING id INTO v_com_lin;

  SELECT count(*) INTO v_n FROM core.mapping_lineage WHERE id = v_com_lin AND NOT listed;
  IF v_n = 1 THEN ok := ok + 1;
  ELSE ko := ko || 'carte communautaire listée par défaut'::text;
  END IF;

  INSERT INTO core.mapping_revision (lineage_id, revision)
    VALUES (v_com_lin, 1) RETURNING id INTO v_com_rev;

  ------------------------------------------------------------------
  -- 4. Une seule tête modifiable par lignée.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.mapping_revision (lineage_id, revision) VALUES (v_com_lin, 2);
    ko := ko || 'deux têtes modifiables sur une même lignée'::text;
  EXCEPTION WHEN unique_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 5. Une attribution EXACT exige un code de justification.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.nuance_party_link
      (mapping_revision_id, nuance_code, circulaire_millesime, party_id, qualification)
    VALUES (v_ref_rev, 'ZZTX', 2026, v_partiA, 'EXACT');
    ko := ko || 'attribution EXACT sans code de justification'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  INSERT INTO core.nuance_party_link
    (mapping_revision_id, nuance_code, circulaire_millesime, party_id, qualification, rationale_code)
  VALUES (v_ref_rev, 'ZZTX', 2026, v_partiA, 'EXACT', 'LIBELLE_NOMME_LE_PARTI');
  ok := ok + 1;

  ------------------------------------------------------------------
  -- 6. Une nuance ne désigne exactement qu'un parti par révision,
  --    mais une nuance de famille large peut en recouvrir plusieurs.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.nuance_party_link
      (mapping_revision_id, nuance_code, circulaire_millesime, party_id, qualification, rationale_code)
    VALUES (v_ref_rev, 'ZZTX', 2026, v_partiB, 'EXACT', 'USAGE_CONSTANT');
    ko := ko || 'deux attributions EXACT pour la même nuance'::text;
  EXCEPTION WHEN unique_violation THEN ok := ok + 1;
  END;

  INSERT INTO core.nuance_party_link
    (mapping_revision_id, nuance_code, circulaire_millesime, party_id, qualification, rationale_code)
  VALUES (v_ref_rev, 'ZZTD', 2026, v_partiA, 'BROADER', 'FAMILLE_PLUS_LARGE');
  INSERT INTO core.nuance_party_link
    (mapping_revision_id, nuance_code, circulaire_millesime, party_id, qualification, rationale_code)
  VALUES (v_ref_rev, 'ZZTD', 2026, v_partiB, 'BROADER', 'FAMILLE_PLUS_LARGE');
  ok := ok + 1;

  ------------------------------------------------------------------
  -- 7. NOT_MAPPABLE interdit de désigner un parti.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.nuance_party_link
      (mapping_revision_id, nuance_code, circulaire_millesime, party_id, qualification)
    VALUES (v_com_rev, 'ZZTD', 2026, v_partiA, 'NOT_MAPPABLE');
    ko := ko || 'NOT_MAPPABLE avec un parti accepté'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 8. Seul EXACT alimente le chemin d'agrégation.
  ------------------------------------------------------------------
  SELECT count(*) INTO v_n FROM core.nuance_party_exact WHERE mapping_revision_id = v_ref_rev;
  IF v_n = 1 THEN ok := ok + 1;
  ELSE ko := ko || format('chemin d''agrégation : %s lignes au lieu de 1', v_n)::text;
  END IF;

  ------------------------------------------------------------------
  -- 9. Une commune n'est rattachée à un parti que par une nuance EXACT.
  ------------------------------------------------------------------
  INSERT INTO ref.commune (code_insee, cog_millesime, nom, code_departement, code_region)
    VALUES ('99999', 2026, 'Commune test', '62', '32');
  INSERT INTO core.person (slug, family_name, given_name)
    VALUES ('zzt-maire-t','T','Maire') RETURNING id INTO v_person;
  INSERT INTO core.mandate (person_id, mandate_type, commune_code, validity)
    VALUES (v_person, 'MAIRE', '99999', daterange('2020-07-01','2026-03-22'))
    RETURNING id INTO v_mandat;

  INSERT INTO core.nuance_assignment (mandate_id, nuance_code, circulaire_millesime)
    VALUES (v_mandat, 'ZZTD', 2026);
  SELECT count(*) INTO v_n FROM core.commune_party
   WHERE mapping_revision_id = v_ref_rev AND commune_code = '99999';
  IF v_n = 0 THEN ok := ok + 1;
  ELSE ko := ko || 'commune attribuée via une nuance BROADER'::text;
  END IF;

  UPDATE core.nuance_assignment SET nuance_code = 'ZZTX' WHERE mandate_id = v_mandat;
  SELECT count(*) INTO v_n FROM core.commune_party
   WHERE mapping_revision_id = v_ref_rev AND commune_code = '99999';
  IF v_n = 1 THEN ok := ok + 1;
  ELSE ko := ko || format('commune non attribuée malgré une nuance EXACT (%s)', v_n)::text;
  END IF;

  ------------------------------------------------------------------
  -- 10. Un gel sans empreinte est refusé : une version citée doit être identifiable.
  ------------------------------------------------------------------
  BEGIN
    UPDATE core.mapping_revision SET frozen_at = now() WHERE id = v_ref_rev;
    ko := ko || 'gel sans empreinte accepté'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 11. Un résultat publié ne peut citer qu'une révision gelée.
  ------------------------------------------------------------------
  INSERT INTO core.preregistration (slug, title, document_path, content_hash, method_version, population_def, sealed_at)
  VALUES ('zzt-pre-001','P','docs/p.md', sha256('p'::bytea),'v1','{}'::jsonb, now())
  RETURNING id INTO v_prereg;

  BEGIN
    INSERT INTO derived.comparison_run
      (slug, indicator_code, period_year, matching_vars, method_version,
       n_communes, n_sans_nuance, taux_exclusion, result, preregistration_id, mapping_revision_id)
    VALUES ('zzt-run-1','ofgl.dette_par_hab',2025,'{strate}','v1',10,2,0.2,'{}'::jsonb, v_prereg, v_ref_rev);
    ko := ko || 'résultat citant une révision non gelée'::text;
  EXCEPTION WHEN raise_exception THEN ok := ok + 1;
  END;

  UPDATE core.mapping_revision
     SET frozen_at = now(), content_hash = sha256('carte'::bytea) WHERE id = v_ref_rev;

  INSERT INTO derived.comparison_run
    (slug, indicator_code, period_year, matching_vars, method_version,
     n_communes, n_sans_nuance, taux_exclusion, result, preregistration_id, mapping_revision_id)
  VALUES ('zzt-run-2','ofgl.dette_par_hab',2025,'{strate}','v1',10,2,0.2,'{}'::jsonb, v_prereg, v_ref_rev);
  ok := ok + 1;

  ------------------------------------------------------------------
  -- 12. Une révision gelée est en lecture seule : on crée une révision, on ne
  --     réécrit pas. La tête de la carte du contradicteur reste modifiable.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.nuance_party_link
      (mapping_revision_id, nuance_code, circulaire_millesime, party_id, qualification, rationale_code)
    VALUES (v_ref_rev, 'ZZTD', 2026, v_partiB, 'EXACT', 'USAGE_CONSTANT');
    ko := ko || 'écriture acceptée dans une révision gelée'::text;
  EXCEPTION WHEN raise_exception THEN ok := ok + 1;
  END;

  BEGIN
    DELETE FROM core.nuance_party_link WHERE mapping_revision_id = v_ref_rev AND nuance_code = 'ZZTX';
    ko := ko || 'suppression acceptée dans une révision gelée'::text;
  EXCEPTION WHEN raise_exception THEN ok := ok + 1;
  END;

  -- Le contradicteur attribue la même nuance à un autre parti : c'est son droit,
  -- c'est tracé, et les deux cartes deviennent comparables ligne à ligne.
  INSERT INTO core.nuance_party_link
    (mapping_revision_id, nuance_code, circulaire_millesime, party_id, qualification,
     rationale_code, rationale_note)
  VALUES (v_com_rev, 'ZZTD', 2026, v_partiB, 'EXACT', 'ARBITRAIRE_ASSUME',
          'Le contradicteur estime cette nuance attribuable.');
  ok := ok + 1;

  -- Une nouvelle révision peut suivre une révision gelée.
  INSERT INTO core.mapping_revision (lineage_id, revision) VALUES (v_ref_lin, 2);
  ok := ok + 1;

  ------------------------------------------------------------------
  -- 13. Successions : une dissolution n'a pas de successeur.
  ------------------------------------------------------------------
  INSERT INTO core.organization_succession
    (predecessor_id, successor_id, succession_type, effective_date)
  VALUES (v_partiA, v_partiB, 'RENOMMAGE', '2018-06-01');
  ok := ok + 1;

  BEGIN
    INSERT INTO core.organization_succession
      (predecessor_id, successor_id, succession_type, effective_date)
    VALUES (v_partiB, v_partiA, 'DISSOLUTION', '2020-01-01');
    ko := ko || 'dissolution avec successeur acceptée'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  IF array_length(ko, 1) IS NULL THEN
    RAISE NOTICE '% garanties vérifiées — OK', ok;
  ELSE
    RAISE EXCEPTION 'Garanties non tenues : %', array_to_string(ko, ' | ');
  END IF;
END $$;

ROLLBACK;
