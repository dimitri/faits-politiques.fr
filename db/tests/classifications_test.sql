-- Garanties de la migration 0013 : classifications tierces de partis, auteurs de
-- textes, échelles et codage de sens.
--
--   psql -v ON_ERROR_STOP=1 -d <db> -f db/tests/classifications_test.sql

BEGIN;

DO $$
DECLARE
  ok int := 0;
  ko text[] := '{}';

  v_src     bigint;
  v_partiA  bigint;
  v_setP    bigint;   -- PopuList
  v_setC    bigint;   -- CHES
  v_lin     bigint; v_rev bigint;
  v_scr1    bigint; v_scr2 bigint;
  v_texte   bigint; v_dossier bigint;
  v_person  bigint;
  v_n       int;
BEGIN
  ------------------------------------------------------------------
  -- Jeu de données minimal
  ------------------------------------------------------------------
  INSERT INTO raw.source (slug, label, publisher, tier, licence, reuse_class, attribution_text)
  VALUES ('zzt-populist','PopuList v4.0','PopuList','PRIMARY_OFFICIAL','CC BY 4.0','ATTRIBUTION','PopuList')
  RETURNING id INTO v_src;

  INSERT INTO core.organization (slug, kind, name) VALUES ('zzt-parti-a','PARTY','Parti A')
    RETURNING id INTO v_partiA;
  INSERT INTO core.person (slug, family_name, given_name) VALUES ('zzt-p-1','Un','Depute')
    RETURNING id INTO v_person;

  INSERT INTO core.dossier (slug, institution, source_uid, titre)
  VALUES ('zzt-d-1','ASSEMBLEE_NATIONALE','D1','Dossier') RETURNING id INTO v_dossier;
  INSERT INTO core.texte (slug, dossier_id, institution, source_uid, kind, titre)
  VALUES ('zzt-t-1', v_dossier, 'ASSEMBLEE_NATIONALE','T1','PROPOSITION_DE_LOI','Texte')
  RETURNING id INTO v_texte;

  INSERT INTO core.scrutin (slug, institution, source_uid, date_seance, granularite, objet)
  VALUES ('zzt-s-1','ASSEMBLEE_NATIONALE','S1','2025-03-01','INDIVIDUAL','Objet 1')
  RETURNING id INTO v_scr1;
  INSERT INTO core.scrutin (slug, institution, source_uid, date_seance, granularite, objet)
  VALUES ('zzt-s-2','ASSEMBLEE_NATIONALE','S2','2025-04-01','INDIVIDUAL','Objet 2')
  RETURNING id INTO v_scr2;

  INSERT INTO core.mapping_lineage (slug, label, kind, listed)
  VALUES ('zzt-codage-reference','Codage de référence','REFERENCE', true) RETURNING id INTO v_lin;
  INSERT INTO core.mapping_revision (lineage_id, revision)
  VALUES (v_lin, 1) RETURNING id INTO v_rev;

  ------------------------------------------------------------------
  -- 1. Identifiants Party Facts : le rapprochement passe par un identifiant,
  --    jamais par le nom du parti.
  ------------------------------------------------------------------
  INSERT INTO core.organization_identifier (organization_id, scheme, value)
  VALUES (v_partiA, 'PARTYFACTS', '1234');
  INSERT INTO core.organization_identifier (organization_id, scheme, value)
  VALUES (v_partiA, 'CNCCFP', 'P-987');
  ok := ok + 1;

  ------------------------------------------------------------------
  -- 2. Une classification est catégorielle OU numérique, jamais les deux :
  --    « far right » et « 8,2 sur galtan » ne sont pas le même objet.
  ------------------------------------------------------------------
  INSERT INTO ref.classification_set (slug, provider, version, label, vocabulary, source_id)
  VALUES ('populist-v4','PopuList','v4.0','PopuList v4.0',
          'far-left / far-right / populist / eurosceptic — vocabulaire anglophone, '
          'non traduit en « extrême gauche / extrême droite »', v_src)
  RETURNING id INTO v_setP;

  INSERT INTO ref.classification_set (slug, provider, version, label, vocabulary)
  VALUES ('ches-2024','CHES','2024','Chapel Hill Expert Survey 2024',
          'scores continus : lrgen, lrecon, galtan')
  RETURNING id INTO v_setC;

  INSERT INTO core.party_classification (classification_set_id, party_id, category)
  VALUES (v_setP, v_partiA, 'far-right');
  INSERT INTO core.party_classification (classification_set_id, party_id, dimension, value, period_year)
  VALUES (v_setC, v_partiA, 'galtan', 8.2, 2024);
  ok := ok + 1;

  BEGIN
    INSERT INTO core.party_classification (classification_set_id, party_id, category, dimension, value)
    VALUES (v_setC, v_partiA, 'far-right', 'lrgen', 9.1);
    ko := ko || 'classification à la fois catégorielle et numérique acceptée'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 3. Deux référentiels peuvent classer le même parti : la divergence est
  --    exposée, jamais moyennée.
  ------------------------------------------------------------------
  INSERT INTO core.party_classification (classification_set_id, party_id, category)
  VALUES (v_setC, v_partiA, 'far-right');

  SELECT nb_referentiels INTO v_n FROM core.party_classification_divergence
   WHERE party_id = v_partiA AND category = 'far-right';
  IF v_n = 2 THEN ok := ok + 1;
  ELSE ko := ko || format('divergence : %s référentiels au lieu de 2', v_n)::text;
  END IF;

  ------------------------------------------------------------------
  -- 4. Auteurs de textes : personne OU organisation.
  ------------------------------------------------------------------
  INSERT INTO core.texte_author (texte_id, person_id, role) VALUES (v_texte, v_person, 'AUTEUR');
  ok := ok + 1;

  BEGIN
    INSERT INTO core.texte_author (texte_id, person_id, organization_id, role)
    VALUES (v_texte, v_person, v_partiA, 'AUTEUR');
    ko := ko || 'auteur de texte à la fois personne et organisation'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 5. Codage de sens : justification obligatoire.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.scale_coding (mapping_revision_id, scale_code, scrutin_id, direction)
    VALUES (v_rev, 'INSTITUTIONS', v_scr1, 1);
    ko := ko || 'codage de sens sans justification accepté'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  INSERT INTO core.scale_coding
    (mapping_revision_id, scale_code, scrutin_id, direction, rationale_code, relu_par)
  VALUES (v_rev, 'INSTITUTIONS', v_scr1, 1, 'OBJET_UNIQUE_EXPLICITE', '{relecteur-a,relecteur-b}');
  ok := ok + 1;

  ------------------------------------------------------------------
  -- 6. Un codage désigne exactement un objet, et un seul par échelle et révision.
  ------------------------------------------------------------------
  BEGIN
    INSERT INTO core.scale_coding
      (mapping_revision_id, scale_code, scrutin_id, texte_id, direction, rationale_code)
    VALUES (v_rev, 'INSTITUTIONS', v_scr2, v_texte, 1, 'OBJET_UNIQUE_EXPLICITE');
    ko := ko || 'codage désignant deux objets accepté'::text;
  EXCEPTION WHEN check_violation THEN ok := ok + 1;
  END;

  BEGIN
    INSERT INTO core.scale_coding
      (mapping_revision_id, scale_code, scrutin_id, direction, rationale_code)
    VALUES (v_rev, 'INSTITUTIONS', v_scr1, -1, 'EXPOSE_DES_MOTIFS');
    ko := ko || 'deux codages du même objet sur la même échelle'::text;
  EXCEPTION WHEN unique_violation THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 7. Un objet composite est affiché mais jamais scoré. C'est le cas courant
  --    d'un texte, assemblage de dispositions de sens opposés.
  ------------------------------------------------------------------
  INSERT INTO core.scale_coding
    (mapping_revision_id, scale_code, texte_id, direction, rationale_code, composite)
  VALUES (v_rev, 'INSTITUTIONS', v_texte, 1, 'DISPOSITIONS_CONTRAIRES', true);

  SELECT count(*) INTO v_n FROM core.scale_coding WHERE mapping_revision_id = v_rev;
  IF v_n <> 2 THEN ko := ko || format('%s codages au lieu de 2', v_n)::text; END IF;

  SELECT count(*) INTO v_n FROM core.scale_coding_scorable WHERE mapping_revision_id = v_rev;
  IF v_n = 1 THEN ok := ok + 1;
  ELSE ko := ko || format('chemin de score : %s codages au lieu de 1', v_n)::text;
  END IF;

  ------------------------------------------------------------------
  -- 8. Le gel des révisions s'applique au codage comme aux rattachements :
  --    on publie une nouvelle révision, on ne réécrit pas un codage cité.
  ------------------------------------------------------------------
  UPDATE core.mapping_revision
     SET frozen_at = now(), content_hash = sha256('codage'::bytea) WHERE id = v_rev;

  BEGIN
    INSERT INTO core.scale_coding
      (mapping_revision_id, scale_code, scrutin_id, direction, rationale_code)
    VALUES (v_rev, 'INSTITUTIONS', v_scr2, 1, 'OBJET_UNIQUE_EXPLICITE');
    ko := ko || 'codage ajouté dans une révision gelée'::text;
  EXCEPTION WHEN raise_exception THEN ok := ok + 1;
  END;

  BEGIN
    UPDATE core.scale_coding SET direction = -1 WHERE mapping_revision_id = v_rev;
    ko := ko || 'codage modifié dans une révision gelée'::text;
  EXCEPTION WHEN raise_exception THEN ok := ok + 1;
  END;

  ------------------------------------------------------------------
  -- 9. Les pôles d'une échelle sont décrits, pas jugés : le champ existe et
  --    est obligatoire des deux côtés.
  ------------------------------------------------------------------
  SELECT count(*) INTO v_n FROM ref.scale
   WHERE code = 'INSTITUTIONS' AND length(pole_negatif) > 0 AND length(pole_positif) > 0;
  IF v_n = 1 THEN ok := ok + 1;
  ELSE ko := ko || 'échelle INSTITUTIONS sans description de ses pôles'::text;
  END IF;

  ------------------------------------------------------------------
  IF array_length(ko, 1) IS NULL THEN
    RAISE NOTICE '% garanties vérifiées — OK', ok;
  ELSE
    RAISE EXCEPTION 'Garanties non tenues : %', array_to_string(ko, ' | ');
  END IF;
END $$;

ROLLBACK;
