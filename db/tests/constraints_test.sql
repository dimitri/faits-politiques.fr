-- Tests des garanties structurelles du schéma.
-- Ces contraintes sont la traduction en SQL des principes de docs/perimetre.md.
-- Si l'une d'elles cesse de mordre, une règle éditoriale devient une convention
-- que le code peut violer silencieusement — c'est exactement ce qu'on veut éviter.
--
--   psql -v ON_ERROR_STOP=1 -d <db> -f db/tests/constraints_test.sql

-- +goose NO TRANSACTION
BEGIN;

-- +goose StatementBegin
DO $$
DECLARE
  ok  int := 0;
  ko  text[] := '{}';

  v_person   bigint;
  v_person2  bigint;
  v_groupe   bigint;
  v_groupe2  bigint;
  v_parti    bigint;
  v_parti2   bigint;
  v_scr_grp  bigint;
  v_scr_ind  bigint;
  v_doc      bigint;
  v_ret      bigint;
  v_src      bigint;
  v_amdt     bigint;
  v_n        int;
BEGIN
  ------------------------------------------------------------------
  -- Jeu de données minimal
  ------------------------------------------------------------------
  INSERT INTO raw.source (slug, label, publisher, tier, licence, reuse_class, attribution_text)
  VALUES ('zzt-an-scrutins','AN — Scrutins','Assemblée nationale','PRIMARY_OFFICIAL',
          'Licence Ouverte 2.0', 'ATTRIBUTION', 'Source : Assemblée nationale')
  RETURNING id INTO v_src;

  INSERT INTO raw.document (sha256, storage_key, content_type, byte_size)
  VALUES (sha256('doc'::bytea), 'raw/an/2026/09/11/x.xml', 'application/xml', 10)
  RETURNING id INTO v_doc;

  INSERT INTO raw.retrieval (source_id, url, http_status, document_id)
  VALUES (v_src, 'https://data.assemblee-nationale.fr/x.xml', 200, v_doc)
  RETURNING id INTO v_ret;

  INSERT INTO core.person (slug, family_name, given_name)
  VALUES ('zzt-dupont-jean','Dupont','Jean') RETURNING id INTO v_person;
  INSERT INTO core.person (slug, family_name, given_name)
  VALUES ('zzt-martin-claude','Martin','Claude') RETURNING id INTO v_person2;

  INSERT INTO core.organization (slug, kind, name) VALUES
    ('zzt-groupe-a','PARLIAMENTARY_GROUP','Groupe A') RETURNING id INTO v_groupe;
  INSERT INTO core.organization (slug, kind, name) VALUES
    ('zzt-groupe-b','PARLIAMENTARY_GROUP','Groupe B') RETURNING id INTO v_groupe2;
  INSERT INTO core.organization (slug, kind, name) VALUES
    ('zzt-parti-a','PARTY','Parti A') RETURNING id INTO v_parti;
  INSERT INTO core.organization (slug, kind, name) VALUES
    ('zzt-parti-b','PARTY','Parti B') RETURNING id INTO v_parti2;

  INSERT INTO core.scrutin (slug, institution, source_uid, date_seance, granularite, objet)
  VALUES ('zzt-senat-1','SENAT','S1','2026-03-01','GROUP','Objet Sénat')
  RETURNING id INTO v_scr_grp;

  INSERT INTO core.scrutin (slug, institution, source_uid, date_seance, granularite, objet)
  VALUES ('zzt-an-1','ASSEMBLEE_NATIONALE','A1','2026-03-01','INDIVIDUAL','Objet AN')
  RETURNING id INTO v_scr_ind;

  INSERT INTO core.amendement (slug, institution, source_uid, numero)
  VALUES ('zzt-amdt-1','ASSEMBLEE_NATIONALE','AM1','1')
  RETURNING id INTO v_amdt;

  ------------------------------------------------------------------
  -- 1. Une source restreinte est ingérable mais reste hors du périmètre
  --    redistribuable : la restriction est tracée, jamais dissoute.
  --    Une absence de licence explicite vaut RESTRICTED, pas autorisation.
  ------------------------------------------------------------------
  INSERT INTO raw.source (slug, label, publisher, tier, licence, reuse_class, attribution_text)
  VALUES ('zzt-ches','CHES 2024','Chapel Hill Expert Survey','PRIMARY_OFFICIAL',
          'Aucune licence explicite publiée', 'RESTRICTED', 'CHES 2024');
  INSERT INTO raw.source (slug, label, publisher, tier, licence, reuse_class, attribution_text)
  VALUES ('zzt-populist','PopuList v4.0','PopuList','PRIMARY_OFFICIAL',
          'CC BY 4.0', 'ATTRIBUTION', 'PopuList v4.0');

  SELECT count(*) INTO v_n FROM raw.source_redistribuable WHERE slug IN ('zzt-ches','zzt-populist','zzt-an-scrutins');
  IF v_n = 2 THEN ok := ok + 1;
  ELSE ko := ko || format('périmètre redistribuable : %s sources au lieu de 2', v_n)::text;
  END IF;

  -- commercial_use est calculée depuis reuse_class : on ne peut pas la forcer.
  BEGIN
    INSERT INTO raw.source (slug, label, publisher, tier, licence, reuse_class,
                            commercial_use, attribution_text)
    VALUES ('zzt-bidon','Bidon','X','SECONDARY_PRESS','CC BY-NC-SA','NON_COMMERCIAL', true, 'x');
    ko := ko || 'commercial_use forcée malgré une source NC'::text;
  EXCEPTION WHEN generated_always THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 2. Un vote nominatif est impossible sur un scrutin de granularité GROUP.
  --    (perimetre.md §2.4 — ne jamais projeter un vote de groupe sur un individu)
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.ballot (scrutin_id, person_id, position)
    VALUES (v_scr_grp, v_person, 'FOR');
    ko := ko || 'vote nominatif accepté sur un scrutin GROUP'::text;
  EXCEPTION WHEN foreign_key_violation THEN ok := ok + 1;
  END;

  -- ... et reste possible sur un scrutin INDIVIDUAL.
  INSERT INTO core.ballot (scrutin_id, person_id, position)
  VALUES (v_scr_ind, v_person, 'AGAINST');
  ok := ok + 1;

  ------------------------------------------------------------------
  -- 3. Symétriquement : pas de position de groupe sur un scrutin INDIVIDUAL.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.ballot_group (scrutin_id, organization_id, position)
    VALUES (v_scr_ind, v_groupe, 'FOR');
    ko := ko || 'position de groupe acceptée sur un scrutin INDIVIDUAL'::text;
  EXCEPTION WHEN foreign_key_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 4. Une mise au point sans date (ou l'inverse) est rejetée.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.ballot (scrutin_id, person_id, position, position_rectifiee)
    VALUES (v_scr_ind, v_person2, 'FOR', 'AGAINST');
    ko := ko || 'mise au point sans date acceptée'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 5. Une preuve porte exactement un sujet.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.evidence (document_id, retrieval_id, tier)
    VALUES (v_doc, v_ret, 'PRIMARY_OFFICIAL');
    ko := ko || 'preuve sans sujet acceptée'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  INSERT INTO core.evidence (scrutin_id, document_id, retrieval_id, tier)
  VALUES (v_scr_ind, v_doc, v_ret, 'PRIMARY_OFFICIAL');
  ok := ok + 1;

  ------------------------------------------------------------------
  -- 6. Mandats : pas de chevauchement du même type pour la même personne.
  ------------------------------------------------------------------
  INSERT INTO core.mandate (person_id, mandate_type, institution, validity)
  VALUES (v_person, 'DEPUTE', 'ASSEMBLEE_NATIONALE', daterange('2022-06-22','2024-06-09'));
  BEGIN
    INSERT INTO core.mandate (person_id, mandate_type, institution, validity)
    VALUES (v_person, 'DEPUTE', 'ASSEMBLEE_NATIONALE', daterange('2023-01-01','2025-01-01'));
    ko := ko || 'mandats de même type chevauchants acceptés'::text;
  EXCEPTION WHEN exclusion_violation THEN ok := ok + 1;
  END;

  -- Un mandat local et un mandat national simultanés restent possibles.
  INSERT INTO core.mandate (person_id, mandate_type, commune_code, validity)
  VALUES (v_person, 'MAIRE', '75056', daterange('2023-01-01', NULL));
  ok := ok + 1;

  -- Un mandat communal sans code commune est rejeté.
  BEGIN
    INSERT INTO core.mandate (person_id, mandate_type, validity)
    VALUES (v_person2, 'MAIRE', daterange('2026-03-22', NULL));
    ko := ko || 'mandat de maire sans commune accepté'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 7. Appartenances : un seul groupe parlementaire à la fois,
  --    mais plusieurs partis simultanés restent permis.
  ------------------------------------------------------------------
  INSERT INTO core.affiliation (person_id, organization_id, organization_kind, validity)
  VALUES (v_person, v_groupe, 'PARLIAMENTARY_GROUP', daterange('2022-06-22','2024-01-01'));
  BEGIN
    INSERT INTO core.affiliation (person_id, organization_id, organization_kind, validity)
    VALUES (v_person, v_groupe2, 'PARLIAMENTARY_GROUP', daterange('2023-06-01','2025-01-01'));
    ko := ko || 'deux groupes parlementaires simultanés acceptés'::text;
  EXCEPTION WHEN exclusion_violation THEN ok := ok + 1;
  END;

  -- Succession sans chevauchement : permise.
  INSERT INTO core.affiliation (person_id, organization_id, organization_kind, validity)
  VALUES (v_person, v_groupe2, 'PARLIAMENTARY_GROUP', daterange('2024-01-01','2025-01-01'));
  ok := ok + 1;

  -- Double appartenance partisane : permise.
  INSERT INTO core.affiliation (person_id, organization_id, organization_kind, validity)
  VALUES (v_person, v_parti,  'PARTY', daterange('2020-01-01', NULL));
  INSERT INTO core.affiliation (person_id, organization_id, organization_kind, validity)
  VALUES (v_person, v_parti2, 'PARTY', daterange('2021-01-01', NULL));
  ok := ok + 1;

  -- Le kind déclaré doit correspondre à celui de l'organisation.
  BEGIN
    INSERT INTO core.affiliation (person_id, organization_id, organization_kind, validity)
    VALUES (v_person2, v_parti, 'PARLIAMENTARY_GROUP', daterange('2022-01-01', NULL));
    ko := ko || 'kind d''appartenance incohérent accepté'::text;
  EXCEPTION WHEN foreign_key_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 8. Une attribution UNRESOLVED ne peut pas désigner une organisation.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.amendement_attribution (amendement_id, organization_id, attribution, method_version)
    VALUES (v_amdt, v_groupe, 'UNRESOLVED', 'v1');
    ko := ko || 'attribution UNRESOLVED avec organisation acceptée'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 9. Un auteur d'amendement est une personne OU une organisation.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.amendement_author (amendement_id, person_id, organization_id, role)
    VALUES (v_amdt, v_person, v_groupe, 'AUTEUR');
    ko := ko || 'auteur à la fois personne et organisation accepté'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 10. Un résumé marqué HUMAN_VERIFIED ne peut pas être de provenance IA.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.scrutin_resume (scrutin_id, titre_neutre, description, ne_dit_pas,
                                     provenance, verification)
    VALUES (v_scr_ind, 't', 'd', 'n', 'AI_EXTRACTED', 'HUMAN_VERIFIED');
    ko := ko || 'résumé IA marqué vérifié humainement accepté'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 11. Une récupération HTTP en échec ne peut pas porter de document.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO raw.retrieval (source_id, url, http_status, document_id)
    VALUES (v_src, 'https://x/404', 404, v_doc);
    ko := ko || 'document attaché à une récupération en échec'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 12. Une contestation close doit porter une réponse.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.contestation (public_ref, page_path, submitter_kind, body, status)
    VALUES ('zzt-C-1','/scrutin/an-1','ELU','Contestation', 'ACCEPTED');
    ko := ko || 'contestation close sans réponse acceptée'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  IF array_length(ko, 1) IS NULL THEN
    RAISE NOTICE '% garanties vérifiées — OK', ok;
  ELSE
    RAISE EXCEPTION 'Garanties non tenues : %', array_to_string(ko, ' | ');
  END IF;
END $$;
-- +goose StatementEnd

ROLLBACK;
