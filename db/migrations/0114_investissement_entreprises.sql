-- +goose Up
-- Investissement et dividendes des entreprises non financières, et la
-- répartition entre micro-entreprises, PME, ETI et grandes entreprises.
--
-- Deux systèmes statistiques distincts, à ne jamais mélanger dans une même
-- somme : la comptabilité nationale (Insee, comptes des sociétés non
-- financières, secteur S11) donne des FLUX macro-économiques annuels ou
-- trimestriels, tous secteurs et toutes tailles d'entreprise confondus ;
-- Ésane (Insee, statistiques structurelles d'entreprises) donne, une fois
-- par an, la répartition par catégorie d'entreprise (micro-entreprises,
-- PME hors micro, ETI, grandes entreprises) au sens de la loi de
-- modernisation de l'économie de 2008 — mais sans distinguer, à cette
-- maille, l'investissement productif de l'achat de titres financiers.

-- Comptes des sociétés non financières (S11), séries trimestrielles CVS de
-- la Banque de données macro-économiques (BDM) de l'Insee. Les deux séries
-- portent le même champ (France, sociétés non financières), la même unité
-- (millions d'euros courants) et la même fréquence : directement
-- comparables terme à terme.
CREATE TABLE core.flux_financier_snf (
  serie      text NOT NULL,             -- 'dividendes' ou 'fbcf' (investissement productif)
  trimestre  text NOT NULL,             -- 'AAAA-Qn'
  valeur_meur numeric NOT NULL,
  source_id  bigint NOT NULL REFERENCES raw.source(id),
  PRIMARY KEY (serie, trimestre)
);
ALTER TABLE core.flux_financier_snf
  ADD CONSTRAINT flux_financier_snf_serie_check CHECK (serie IN ('dividendes', 'fbcf'));

-- Le système productif par catégorie d'entreprise (Ésane), publication
-- annuelle. Une ligne par catégorie et par année publiée : la série
-- s'allonge d'une ligne chaque année plutôt que d'écraser la précédente,
-- pour permettre une comparaison dans le temps si plusieurs millésimes sont
-- un jour chargés.
CREATE TABLE core.entreprise_categorie (
  annee                    integer NOT NULL,
  categorie                text NOT NULL,   -- MIC, PME, ETI, GE
  nb_entreprises           integer,
  effectif_etp_milliers    numeric,
  chiffre_affaires_meur    numeric,
  valeur_ajoutee_meur      numeric,
  taux_investissement_pct  numeric,         -- investissement corporel / valeur ajoutée
  immobilisations_par_salarie_eur numeric,  -- immobilisations corporelles par salarié ETP
  source_id                bigint NOT NULL REFERENCES raw.source(id),
  PRIMARY KEY (annee, categorie)
);
ALTER TABLE core.entreprise_categorie
  ADD CONSTRAINT entreprise_categorie_categorie_check CHECK (categorie IN ('MIC', 'PME', 'ETI', 'GE'));

-- +goose Down
DROP TABLE core.entreprise_categorie;
DROP TABLE core.flux_financier_snf;
