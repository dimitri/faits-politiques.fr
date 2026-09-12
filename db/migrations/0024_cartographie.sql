-- +goose Up
-- Cartographie éditoriale : relier un parti à ce qui le nomme ailleurs.
--
-- Un même parti existe en base sous plusieurs lignes, parce que plusieurs
-- sources le nomment chacune à sa façon et qu'aucune ne cite l'identifiant de
-- l'autre :
--
--   registre CNCCFP   « Rassemblement national »          code 40
--   CHES 2024         « RN »                              code 610
--   organes de l'AN   « Rassemblement national »          PO761239  (le parti)
--   organes de l'AN   « Rassemblement national »          PO845401  (le groupe)
--
-- core.organization_identifier ne peut pas les réunir : (scheme, value) y est
-- UNIQUE, et c'est une bonne chose — un identifiant désigne une chose et une
-- seule. Dire « ces quatre lignes parlent du même parti » n'est pas un fait
-- publié, c'est une décision. Elle vit donc ici, dans une révision de
-- cartographie versionnée, gelable et forkable, comme les autres décisions de
-- rattachement (core.party_group_link, core.nuance_party_link).
--
-- Le parti canonique est l'entrée du registre CNCCFP : c'est la seule liste
-- centrale et officielle des partis politiques français.
CREATE TABLE core.party_referential_link (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  mapping_revision_id bigint NOT NULL
                        REFERENCES core.mapping_revision(id) ON DELETE CASCADE,
  party_id            bigint NOT NULL,
  party_kind          core.organization_kind NOT NULL DEFAULT 'PARTY'
                        CHECK (party_kind = 'PARTY'),
  referential_id      bigint NOT NULL,
  referential_kind    core.organization_kind NOT NULL DEFAULT 'PARTY'
                        CHECK (referential_kind = 'PARTY'),
  scheme              text NOT NULL
                        CHECK (scheme IN ('CHES', 'POPULIST', 'PARTYFACTS',
                                          'PARLGOV', 'MANIFESTO', 'AN_ORGANE')),
  rationale_code      text REFERENCES ref.rationale_code(code),
  created_at          timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (party_id, party_kind)
    REFERENCES core.organization(id, kind),
  FOREIGN KEY (referential_id, referential_kind)
    REFERENCES core.organization(id, kind),
  -- Un référentiel ne nomme qu'une fois le même parti, et une entrée de
  -- référentiel ne vaut que pour un parti.
  CONSTRAINT party_referential_link_pas_de_boucle
    CHECK (party_id <> referential_id)
);

CREATE UNIQUE INDEX party_referential_link_unicite
  ON core.party_referential_link (mapping_revision_id, scheme, referential_id);
CREATE UNIQUE INDEX party_referential_link_un_seul_par_schema
  ON core.party_referential_link (mapping_revision_id, scheme, party_id);
CREATE INDEX party_referential_link_revision_idx
  ON core.party_referential_link (mapping_revision_id);

-- +goose StatementBegin
CREATE TRIGGER party_referential_link_respecte_le_gel
  BEFORE INSERT OR UPDATE OR DELETE ON core.party_referential_link
  FOR EACH ROW EXECUTE FUNCTION core.assert_revision_not_frozen();
-- +goose StatementEnd

COMMENT ON TABLE core.party_referential_link IS
  'Décision éditoriale : cette entrée de référentiel tiers désigne ce parti. '
  'Versionnée, gelable, forkable. Jamais un rapprochement de noms calculé.';

-- Motifs manquants pour ce type de rattachement.
INSERT INTO ref.rationale_code (code, label) VALUES
  ('DENOMINATION_IDENTIQUE',
   'La dénomination publiée par le référentiel est identique à celle du registre'),
  ('SIGLE_OFFICIEL',
   'Le référentiel emploie le sigle officiel du parti'),
  ('SUCCESSION_DOCUMENTEE',
   'Le référentiel nomme un prédécesseur dont la succession est documentée')
ON CONFLICT (code) DO NOTHING;

-- +goose Down
DROP TABLE core.party_referential_link;
DELETE FROM ref.rationale_code
 WHERE code IN ('DENOMINATION_IDENTIQUE', 'SIGLE_OFFICIEL', 'SUCCESSION_DOCUMENTEE');
