-- +goose Up
-- Qui paie, via quel mécanisme fiscal — pas seulement quel niveau de
-- collectivité reçoit. OFGL (fiscalité directe locale, REI/DGFiP), agrégats
-- nationaux calculés côté serveur (group_by), pas les ~35 millions de lignes
-- par commune × variable × année : seul le total nourrit la question posée
-- (docs/collectivites-donnees.md). Chargement volontairement partiel — voir
-- le commentaire de internal/communes/fiscalite_locale.go pour ce qui est
-- exclu et pourquoi (IFER, TH, TEOM, surtaxes GEMAPI/TSE/CHAMBRE).
CREATE TABLE core.fiscalite_directe_locale (
  annee            integer NOT NULL,
  dispositif       text NOT NULL,   -- 'FB', 'FNB', 'CFE', 'TASCOM' (codes REI)
  categorie_payeur text NOT NULL CHECK (categorie_payeur IN ('MENAGES','ENTREPRISES')),
  destinataire     text NOT NULL CHECK (destinataire IN ('COMMUNE','GFP')),
  montant_eur      numeric NOT NULL,
  source_id        bigint NOT NULL REFERENCES raw.source(id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (annee, dispositif, destinataire)
);

COMMENT ON TABLE core.fiscalite_directe_locale IS
  'Produit réel de quatre impôts locaux (REI/DGFiP via OFGL), par millésime, '
  'dispositif fiscal et destinataire (commune ou intercommunalité) — le montant '
  'exact reçu par les collectivités du bloc communal, catégorisé ménages '
  '(foncier bâti + non bâti) ou entreprises (CFE + TASCOM). Un seul var REI '
  'par ligne, jamais un sous-total ET son détail additionnés (le REI publie '
  'des variables imbriquées — ex. P33 et P33_1+P33_2 — qui ne s''additionnent '
  'pas entre elles).';

-- +goose Down
DROP TABLE core.fiscalite_directe_locale;
