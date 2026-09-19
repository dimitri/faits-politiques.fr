-- +goose Up
-- Le trafic des ports français, par port et par année, 2000-2025 (SDES,
-- ministère de la Transition écologique — data.statistiques.developpement-
-- durable.gouv.fr, vérifié directement : 1541 lignes, 42 ports, deux sens
-- de circulation par port et par année). HAROPA (Le Havre, Rouen, Paris)
-- apparaît comme une entité unique depuis la fusion administrative de 2021
-- — les années antérieures pour Le Havre seul restent dans la table sous
-- son propre nom, jamais recalculées a posteriori sous "HAROPA".
CREATE TABLE core.trafic_portuaire (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  locode         text NOT NULL,
  port           text NOT NULL,
  facade         text,
  region         text,
  mouvement      text NOT NULL CHECK (mouvement IN ('Entree', 'Sortie')),
  annee          int NOT NULL,
  tonnage_tot    bigint,
  vracs_liquides bigint,
  vracs_solides  bigint,
  cont_tot       bigint,
  evp_tot        bigint,
  roro_tot       bigint,
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  UNIQUE (locode, mouvement, annee)
);

CREATE INDEX trafic_portuaire_port_idx ON core.trafic_portuaire (port, annee);

COMMENT ON TABLE core.trafic_portuaire IS
  'Trafic maritime de marchandises par port français, 2000-2025 (SDES). '
  'tonnage_tot = marchandises + tare (poids des contenants) ; les sous-totaux '
  '(vracs, conteneurs, roulier) sont des marchandises seules, ne totalisent pas '
  'exactement tonnage_tot — voir docs/ports-donnees.md.';

-- La comparaison européenne : six ports nommés (Eurostat, mar_go_aa), pas
-- tous les ports du continent — même logique de sélection nommée que
-- core.indicateur_mondial pour les comparaisons internationales déjà
-- chargées (SIPRI, PIB des blocs).
CREATE TABLE core.trafic_portuaire_europe (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code_port         text NOT NULL,
  port_label        text NOT NULL,
  pays_code         text NOT NULL,
  annee             int NOT NULL,
  tonnage_milliers  double precision NOT NULL,
  source_id         bigint NOT NULL REFERENCES raw.source(id),
  UNIQUE (code_port, annee)
);

COMMENT ON TABLE core.trafic_portuaire_europe IS
  'Trafic total (sens confondus) de six ports, France et rang nord-européen '
  '(Eurostat mar_go_aa, direct=TOTAL, unit=milliers de tonnes), 1997-2025.';

-- +goose Down
DROP TABLE core.trafic_portuaire_europe;
DROP TABLE core.trafic_portuaire;
