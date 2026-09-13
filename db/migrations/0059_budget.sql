-- +goose Up
-- Le budget de l'État et celui de la Sécurité sociale.
--
-- Ce schéma existe pour rendre IMPOSSIBLES quatre confusions qui traversent tout
-- le débat budgétaire français. Elles ne sont pas des subtilités d'expert : les
-- ignorer produit des phrases fausses avec des chiffres justes.
--
--  1. TROIS COMPTABILITÉS coexistent et ne donnent pas les mêmes nombres pour la
--     même année. Budgétaire (encaissement / décaissement, c'est la loi de
--     finances), générale (droits constatés, patrimoniale), nationale (SEC 2010,
--     périmètre consolidé, c'est Maastricht). Le solde de la loi de finances et
--     le déficit public ne sont pas le même objet.
--  2. TROIS PÉRIMÈTRES circulent côté social — régime général seul, régime
--     général + FSV, tous régimes obligatoires de base — et les écarts se
--     comptent en milliards. S'y ajoute un quatrième, la protection sociale au
--     sens de la DREES et d'Eurostat, qui inclut l'assurance chômage et les
--     retraites complémentaires que la LFSS ne couvre pas.
--  3. « VOTÉ » N'EST PAS UN ÉTAT STABLE. Le solde de la LFSS 2026 vaut −17,5 Md€
--     au dépôt et −19,4 Md€ à l'adoption : le Parlement a dégradé le solde qu'on
--     lui demandait de redresser. Sans colonne `stade`, ces deux chiffres justes
--     se contredisent dans la base.
--  4. L'ONDAM EST UN OBJECTIF, PAS UN PLAFOND. La LOLF donne à l'État des crédits
--     limitatifs : les dépasser est illégal. La LFSS fixe des objectifs de
--     dépenses, révisables l'année suivante. « ONDAM respecté » et « dépense
--     tenue » ne sont pas la même affirmation.
--
-- D'où trois référentiels que TOUTE valeur budgétaire doit référencer. Ce sont
-- des clés étrangères, pas des commentaires : une valeur sans périmètre ne peut
-- pas entrer.

CREATE TABLE ref.budget_comptabilite (
  code       text PRIMARY KEY,
  label      text NOT NULL,
  regle      text NOT NULL,
  definition text NOT NULL
);

COMMENT ON TABLE ref.budget_comptabilite IS
  'Les trois règles de comptabilité publique. Deux valeurs de comptabilités '
  'différentes ne s''additionnent pas et ne se comparent pas, même pour la même '
  'année et le même périmètre.';

INSERT INTO ref.budget_comptabilite (code, label, regle, definition) VALUES
  ('BUDGETAIRE', 'Comptabilité budgétaire',
   'Encaissement / décaissement, autorisations d''engagement et crédits de paiement',
   'Celle de la loi de finances, des situations mensuelles et de la loi de '
   'règlement. Elle répond à « le Parlement a autorisé X, l''administration a '
   'dépensé Y ».'),
  ('GENERALE', 'Comptabilité générale',
   'Droits constatés, patrimoniale, avec bilan',
   'Celle des balances des comptes de l''État et des comptes des régimes. Elle '
   'répond à « voici ce que l''État possède et ce qu''il doit ».'),
  ('NATIONALE', 'Comptabilité nationale (SEC 2010)',
   'Droits constatés, périmètre des administrations publiques consolidé',
   'Celle de l''INSEE et d''Eurostat. Elle répond à « voici le déficit public au '
   'sens des critères européens », et c''est la SEULE qui permette de poser '
   'l''État et la Sécurité sociale côte à côte. Elle ne connaît que l''exécuté, '
   'retraité, et à dix-huit mois de délai pour les comptes définitifs.');

CREATE TABLE ref.budget_perimetre (
  code       text PRIMARY KEY,
  label      text NOT NULL,
  definition text NOT NULL,
  -- Vrai quand le périmètre inclut l'assurance chômage et les retraites
  -- complémentaires. C'est la ligne de partage entre ce que la LFSS couvre et
  -- ce que les comptes nationaux mesurent, et elle vaut des dizaines de
  -- milliards : la dette nette de l'Unédic était de 59,6 Md€ fin 2024.
  hors_lfss  boolean NOT NULL
);

COMMENT ON TABLE ref.budget_perimetre IS
  'Le périmètre d''une valeur budgétaire. Trois définitions circulent côté '
  'social et les écarts se comptent en milliards ; aucune valeur n''est chargée '
  'sans la sienne.';

INSERT INTO ref.budget_perimetre (code, label, definition, hors_lfss) VALUES
  ('ETAT_BUDGET_GENERAL', 'État — budget général',
   'Le budget général de l''État, hors comptes spéciaux et budgets annexes. '
   'C''est le périmètre des situations mensuelles budgétaires.', false),
  ('APU_S13', 'Toutes administrations publiques (S13)',
   'État, organismes divers, collectivités et sécurité sociale, après '
   'consolidation des flux entre eux. Périmètre de Maastricht.', true),
  ('APU_S1311', 'Administration centrale (S1311)',
   'L''État et les organismes divers d''administration centrale. Ce n''est pas '
   'le budget général : les opérateurs y sont inclus.', false),
  ('APU_S1313', 'Administrations publiques locales (S1313)',
   'Communes, départements, régions, groupements et satellites.', false),
  ('APU_S1314', 'Administrations de sécurité sociale (S1314)',
   'Régimes obligatoires, MAIS AUSSI assurance chômage et retraites '
   'complémentaires. Plus large que la LFSS.', true),
  ('PROTECTION_SOCIALE', 'Protection sociale (DREES / ESSPROS)',
   'Champ de la protection sociale au sens des comptes de la DREES et '
   'd''ESSPROS : toutes prestations, tous régimes, chômage et retraites '
   'complémentaires compris. Plus large que la LFSS.', true),
  ('REGIME_GENERAL', 'Régime général seul',
   'Les cinq branches du régime général, sans le Fonds de solidarité '
   'vieillesse.', false),
  ('REGIME_GENERAL_FSV', 'Régime général + FSV',
   'Le périmètre le plus souvent cité dans la presse sous le nom de « déficit '
   'de la Sécu ».', false),
  ('TOUS_REGIMES_BASE', 'Tous régimes obligatoires de base',
   'Le périmètre des tableaux d''équilibre votés en loi de financement.', false),
  ('SECTEUR_PRIVE_URSSAF', 'Secteur privé, champ URSSAF',
   'Employeurs du secteur privé relevant du recouvrement URSSAF, France '
   'entière. N''inclut ni la fonction publique ni le régime agricole.', false);

CREATE TABLE ref.budget_stade (
  code       text PRIMARY KEY,
  label      text NOT NULL,
  definition text NOT NULL,
  -- L'ordre chronologique du cycle budgétaire, pour trier sans coder l'ordre
  -- dans les requêtes.
  rang       smallint NOT NULL UNIQUE
);

COMMENT ON TABLE ref.budget_stade IS
  'À quel moment du cycle budgétaire une valeur a été arrêtée. Deux chiffres '
  'justes se contredisent si l''on ignore leur stade : le solde de la LFSS 2026 '
  'valait −17,5 Md€ au dépôt et −19,4 Md€ à l''adoption.';

INSERT INTO ref.budget_stade (code, label, definition, rang) VALUES
  ('DEPOT', 'Dépôt du projet',
   'Les chiffres du projet de loi tel que le Gouvernement le dépose.', 1),
  ('ADOPTION', 'Adoption définitive',
   'Les chiffres de la loi telle qu''elle est promulguée, après amendements et, '
   'le cas échéant, censure partielle du Conseil constitutionnel.', 2),
  ('REVISION', 'Révision en cours d''année',
   'Loi de finances rectificative, loi de financement rectificative, ou '
   'révision de prévision publiée par une commission des comptes.', 3),
  ('EXECUTION', 'Exécution constatée',
   'Ce qui a réellement été encaissé et décaissé, ou constaté en droits. Une '
   'exécution n''est définitive qu''après clôture et, pour la comptabilité '
   'nationale, après révision.', 4);

-- ---------------------------------------------------------------------------
-- Les comptes de la protection sociale (DREES), série depuis 1959.
--
-- C'est la meilleure série longue du champ social en données ouvertes — et pour
-- cause : il n'y en a pas d'autre. Une recherche « comptes de la sécurité
-- sociale » sur data.gouv.fr renvoie zéro jeu de données, et le seul jeu
-- rattaché à la LFSS, les REPSS, est gelé depuis janvier 2022.
--
-- Deux hiérarchies se croisent dans cette table, et c'est le piège principal :
-- `ps_niveau` va de 0 (total des prestations) à 4 (prestation élémentaire), et
-- `si_niveau` de 0 (tous régimes) à 2 (organisme). SOMMER TOUTES LES LIGNES
-- COMPTE CHAQUE EURO PLUSIEURS FOIS. Toute requête doit fixer un niveau sur
-- chacun des deux axes ; la vue derived.protection_sociale_total le fait.
CREATE TABLE core.protection_sociale (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee          smallint NOT NULL CHECK (annee BETWEEN 1959 AND 2100),
  -- Axe « prestation » : le quoi.
  ps_niveau      smallint NOT NULL CHECK (ps_niveau BETWEEN 0 AND 4),
  ps_code        text NOT NULL,
  ps_libelle     text NOT NULL,
  risque         text NOT NULL,
  -- Axe « financeur » : le qui. si_code porte le SECTEUR INSTITUTIONNEL au sens
  -- du SEC 2010 — S13111 pour l'État, S13141 pour le régime général — ce qui
  -- permet de ventiler la protection sociale entre ses financeurs. C'est la
  -- colonne la plus utile de la table et la plus facile à jeter par mégarde.
  si_niveau      smallint NOT NULL CHECK (si_niveau BETWEEN 0 AND 2),
  si_code        text NOT NULL,
  si_nom         text NOT NULL,
  regime         text NOT NULL,
  valeur_meur    double precision NOT NULL,
  perimetre      text NOT NULL REFERENCES ref.budget_perimetre(code),
  comptabilite   text NOT NULL REFERENCES ref.budget_comptabilite(code),
  stade          text NOT NULL REFERENCES ref.budget_stade(code),
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  document_id    bigint NOT NULL REFERENCES raw.document(id),
  -- Le jeu publie une ligne par croisement ; deux lignes identiques signaleraient
  -- un dépliage fautif du chargement, pas une donnée.
  UNIQUE (annee, ps_code, si_code, regime)
);

COMMENT ON TABLE core.protection_sociale IS
  'Comptes de la protection sociale (DREES), 1959 →. Périmètre PROTECTION '
  'SOCIALE : plus large que la LFSS, assurance chômage et retraites '
  'complémentaires comprises. Table HIÉRARCHIQUE sur deux axes : sommer toutes '
  'les lignes compte chaque euro plusieurs fois.';
COMMENT ON COLUMN core.protection_sociale.si_code IS
  'Secteur institutionnel SEC 2010 du financeur (S13111 = État, S13141 = régime '
  'général). C''est par cette colonne que la protection sociale se ventile '
  'entre ses financeurs.';

CREATE INDEX protection_sociale_annee_idx ON core.protection_sociale (annee);
CREATE INDEX protection_sociale_niveaux_idx ON core.protection_sociale (ps_niveau, si_niveau, annee);

-- ---------------------------------------------------------------------------
-- L'exécution mensuelle du budget de l'État.
--
-- La source est publiée PIVOTÉE : vingt-six lignes de postes, et une colonne par
-- date d'arrêté. Elle est dépliée ici en (poste, date, valeur), parce qu'une
-- table qui gagne une colonne chaque mois n'est pas un modèle de données.
--
-- Le titre du jeu annonce « les exercices 2013 à nos jours ». Le schéma publié
-- n'en porte que trente et un arrêtés, de janvier 2024 à juillet 2026. L'écart
-- entre le titre et le contenu est un fait de la source : la table stocke ce
-- qu'elle a reçu, et le contrôle de fraîcheur de cmd/verify dira si cela bouge.
CREATE TABLE core.execution_etat (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  -- La date d'ARRÊTÉ de la situation mensuelle : les montants sont cumulés
  -- depuis le 1er janvier de l'exercice, ils ne sont pas mensuels.
  date_arrete    date NOT NULL,
  exercice       smallint NOT NULL,
  niveau         smallint NOT NULL,
  categorie      text NOT NULL,
  sous_categorie text NOT NULL,
  ligne          text NOT NULL,
  -- NULL quand la source ne publie rien pour ce poste à cette date. Jamais zéro :
  -- un poste non renseigné et un poste à zéro euro sont deux faits différents.
  montant_eur    double precision,
  perimetre      text NOT NULL REFERENCES ref.budget_perimetre(code),
  comptabilite   text NOT NULL REFERENCES ref.budget_comptabilite(code),
  stade          text NOT NULL REFERENCES ref.budget_stade(code),
  source_id      bigint NOT NULL REFERENCES raw.source(id),
  document_id    bigint NOT NULL REFERENCES raw.document(id),
  UNIQUE (date_arrete, categorie, sous_categorie, ligne)
);

COMMENT ON TABLE core.execution_etat IS
  'Situations mensuelles budgétaires de l''État, dépliées depuis une table '
  'pivotée. Montants CUMULÉS depuis le 1er janvier de l''exercice, en '
  'comptabilité BUDGÉTAIRE : non comparables au déficit public.';
COMMENT ON COLUMN core.execution_etat.montant_eur IS
  'NULL si la source ne publie pas de valeur pour ce poste à cette date. '
  'Jamais zéro par défaut : une absence n''est pas un montant nul.';

CREATE INDEX execution_etat_date_idx ON core.execution_etat (date_arrete);
CREATE INDEX execution_etat_ligne_idx ON core.execution_etat (ligne, date_arrete);

-- ---------------------------------------------------------------------------
-- Les exonérations de cotisations, mesure par mesure (URSSAF).
--
-- C'est la matière chiffrée du deuxième canal entre les deux budgets : l'État
-- allège les cotisations et compense — pour l'essentiel via la TVA affectée.
-- 2,63 Md€ d'exonérations restent officiellement non compensées en 2026, et la
-- non-compensation décidée en décembre 2023 ampute les recettes de l'Unédic de
-- 12,05 Md€ sur 2023-2026.
--
-- Un allègement n'est pas une dépense de l'État au sens budgétaire : il ne
-- figure dans aucun crédit. Il diminue une recette sociale, et l'État en
-- rembourse tout ou partie. C'est pour cela que le montant est ici et non dans
-- core.execution_etat.
CREATE TABLE core.exoneration_cotisation (
  id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  annee              smallint NOT NULL,
  code_grande_categorie text NOT NULL,
  grande_categorie   text NOT NULL,
  code_categorie     text NOT NULL,
  categorie          text NOT NULL,
  code_mesure        text NOT NULL,
  mesure             text NOT NULL,
  montant_eur        double precision,
  perimetre          text NOT NULL REFERENCES ref.budget_perimetre(code),
  source_id          bigint NOT NULL REFERENCES raw.source(id),
  document_id        bigint NOT NULL REFERENCES raw.document(id),
  UNIQUE (annee, code_mesure)
);

COMMENT ON TABLE core.exoneration_cotisation IS
  'Exonérations de cotisations sociales par mesure (URSSAF), champ secteur '
  'privé France entière. Ne dit RIEN de la compensation : une exonération '
  'compensée et une exonération non compensée y figurent à l''identique.';

-- ---------------------------------------------------------------------------
-- La masse salariale du secteur privé : l'assiette des cotisations.
--
-- C'est le dénominateur de tout le reste. Un montant d'exonérations qui augmente
-- pendant que la masse salariale augmente autant ne dit pas la même chose qu'un
-- montant qui augmente seul.
CREATE TABLE core.masse_salariale (
  annee            smallint NOT NULL,
  trimestre        smallint NOT NULL CHECK (trimestre BETWEEN 1 AND 4),
  dernier_jour     date NOT NULL,
  -- Deux mesures coexistent, selon le délai de prise en compte des déclarations
  -- tardives (50 ou 60 jours). Les deux sont publiées ; les garder toutes deux
  -- évite d'avoir à choisir à la place du lecteur.
  brut_50j_eur     double precision,
  brut_60j_eur     double precision,
  cvs_50j_eur      double precision,
  cvs_60j_eur      double precision,
  perimetre        text NOT NULL REFERENCES ref.budget_perimetre(code),
  source_id        bigint NOT NULL REFERENCES raw.source(id),
  document_id      bigint NOT NULL REFERENCES raw.document(id),
  PRIMARY KEY (annee, trimestre)
);

COMMENT ON COLUMN core.masse_salariale.cvs_50j_eur IS
  'Corrigé des variations saisonnières. À utiliser pour comparer deux '
  'trimestres, jamais pour un total annuel — la correction déforme les niveaux.';

-- ---------------------------------------------------------------------------
-- Les lois financières : la LISTE, pas les chiffres.
--
-- Au volume concerné — une loi de finances et une loi de financement par an — un
-- parseur de texte de loi coûterait plus cher qu'il ne rapporte, et serait plus
-- fragile. C'est le patron de ref.pdr_proclamation : la liste des textes est
-- semée et relue pièce par pièce, les CHIFFRES viennent d'un document scellé.
--
-- Cette migration ne sème donc AUCUN montant. core.solde_vote reste vide tant
-- qu'un connecteur n'aura pas extrait les tableaux d'équilibre du texte publié
-- au Journal officiel. Une table vide se voit ; un chiffre saisi à la main dans
-- une migration ne se voit plus jamais.
CREATE TABLE ref.loi_financiere (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  -- LFI : loi de finances initiale. LFR : rectificative. LFSS : loi de
  -- financement de la sécurité sociale. LFSSR : rectificative. LACSS : loi
  -- d'approbation des comptes, créée par la loi organique du 14 mars 2022.
  type         text NOT NULL CHECK (type IN ('LFI', 'LFR', 'LFSS', 'LFSSR', 'LACSS')),
  -- L'exercice sur lequel porte la loi, pas l'année de sa promulgation : la
  -- LFSS pour 2026 est promulguée le 30 décembre 2025.
  exercice     smallint NOT NULL,
  numero       text NOT NULL UNIQUE,
  date_texte   date NOT NULL,
  -- La publication au Journal officiel fait foi ; une URL peut mourir.
  jorf         text NOT NULL,
  legifrance_url text NOT NULL,
  notes        text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (type, exercice, date_texte)
);

COMMENT ON TABLE ref.loi_financiere IS
  'Les lois de finances et de financement : QUEL texte fait foi, pas ce qu''il '
  'contient. Les montants votés vivent dans core.solde_vote et viennent du '
  'document scellé, jamais d''une migration.';

CREATE TABLE core.solde_vote (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  loi_id        bigint NOT NULL REFERENCES ref.loi_financiere(id) ON DELETE CASCADE,
  -- Le stade est OBLIGATOIRE : c'est toute la raison d'être de cette table.
  stade         text NOT NULL REFERENCES ref.budget_stade(code),
  perimetre     text NOT NULL REFERENCES ref.budget_perimetre(code),
  comptabilite  text NOT NULL REFERENCES ref.budget_comptabilite(code),
  branche       text,
  recettes_meur double precision,
  depenses_meur double precision,
  solde_meur    double precision,
  -- Vrai pour l'ONDAM, et pour lui seul. L'objectif national de dépenses
  -- d'assurance maladie n'est PAS un plafond : un dépassement n'est pas une
  -- irrégularité, et une page qui affiche « objectif tenu » sans le dire induit
  -- en erreur. La colonne existe pour que l'affichage n'ait pas le choix.
  est_objectif  boolean NOT NULL DEFAULT false,
  source_id     bigint NOT NULL REFERENCES raw.source(id),
  document_id   bigint REFERENCES raw.document(id),
  UNIQUE (loi_id, stade, perimetre, branche)
);

COMMENT ON COLUMN core.solde_vote.est_objectif IS
  'Vrai pour l''ONDAM : un objectif de dépenses, révisable, dont le dépassement '
  'n''est pas une irrégularité. Faux pour les crédits limitatifs de l''État, '
  'dont le dépassement est illégal. La distinction est juridique, pas '
  'rédactionnelle.';

-- La liste des lois. Numéros, dates et références au Journal officiel relus un
-- par un ; aucun montant.
INSERT INTO ref.loi_financiere (type, exercice, numero, date_texte, jorf, legifrance_url, notes) VALUES
  ('LFSS', 2026, '2025-1329', '2025-12-30',
   'JORF n°0303 du 31 décembre 2025',
   'https://www.legifrance.gouv.fr/jorf/id/JORFTEXT000051939031',
   'Adoptée le 16 décembre 2025, promulguée après censure partielle du Conseil '
   'constitutionnel. Le solde voté est passé de −17,5 Md€ au dépôt à −19,4 Md€ '
   'à l''adoption.'),
  ('LFI', 2026, '2026-127', '2026-02-14',
   'JORF n°0038 du 14 février 2026',
   'https://www.legifrance.gouv.fr/jorf/id/JORFTEXT000052110584',
   'Promulguée en février faute d''adoption avant le 31 décembre ; une loi '
   'spéciale a assuré la continuité en janvier.');

-- ---------------------------------------------------------------------------
-- Ce que la comptabilité nationale permet, et qu'aucune autre source ne permet.
--
-- Les séries sont chargées par internal/macro dans core.macro_value ; cette vue
-- les remet en forme par sous-secteur et leur attache explicitement leur
-- périmètre et leur comptabilité, pour qu'aucune requête n'ait à les deviner.
CREATE VIEW derived.budget_sous_secteur AS
  SELECT v.annee,
         s.secteur,
         p.label AS perimetre_label,
         'NATIONALE'::text AS comptabilite,
         'EXECUTION'::text AS stade,
         max(v.valeur) FILTER (WHERE s.agregat = 'depense') AS depenses_meur,
         max(v.valeur) FILTER (WHERE s.agregat = 'recette') AS recettes_meur,
         max(v.valeur) FILTER (WHERE s.agregat = 'solde')   AS solde_meur,
         'sous-secteur-v1'::text AS method_version
    FROM core.macro_value v
    JOIN LATERAL (
      SELECT split_part(v.serie_code, '.', 1) AS agregat,
             split_part(v.serie_code, '.', 2) AS secteur) s ON true
    JOIN ref.budget_perimetre p ON p.code = 'APU_' || s.secteur
   WHERE v.serie_code ~ '^(depense|recette|solde)\.S13'
   GROUP BY 1, 2, 3;

COMMENT ON VIEW derived.budget_sous_secteur IS
  'Dépenses, recettes et solde par sous-secteur des administrations publiques, '
  'en comptabilité nationale. Le seul endroit où l''État et la Sécurité sociale '
  'se mesurent sur la même règle. Ne donne jamais un chiffre VOTÉ : la '
  'comptabilité nationale ne connaît que l''exécuté.';

-- Le total des prestations de protection sociale, un niveau fixé sur chacun des
-- deux axes. Cette vue existe pour qu'on n'ait pas à se rappeler qu'une somme
-- naïve sur core.protection_sociale compte chaque euro cinq fois.
CREATE VIEW derived.protection_sociale_total AS
  SELECT annee,
         risque,
         sum(valeur_meur) AS prestations_meur,
         'total-niveau1-v1'::text AS method_version
    FROM core.protection_sociale
   WHERE ps_niveau = 1 AND si_niveau = 0
   GROUP BY 1, 2;

COMMENT ON VIEW derived.protection_sociale_total IS
  'Prestations par risque, tous régimes. ps_niveau = 1 et si_niveau = 0 : un '
  'seul niveau sur chaque axe, sans quoi les sous-totaux s''ajoutent aux totaux.';

-- ---------------------------------------------------------------------------
-- Le rapprochement qui REFUSE d'aligner ce qui ne se compare pas.
--
-- Une vue de rapprochement naïve mettrait côte à côte le solde voté en LFSS et
-- le besoin de financement des administrations de sécurité sociale, et ferait
-- croire à un écart d'exécution. Les deux nombres ne sont ni dans la même
-- comptabilité, ni sur le même périmètre : leur différence ne mesure rien.
--
-- Celle-ci n'apparie donc que des couples de MÊME comptabilité et de MÊME
-- périmètre, et publie ce qu'elle compare.
CREATE VIEW derived.budget_rapprochement AS
  SELECT l.exercice,
         l.type      AS loi,
         v.stade,
         v.perimetre,
         v.comptabilite,
         v.branche,
         v.solde_meur AS solde_loi_meur,
         b.solde_meur AS solde_comptes_meur,
         CASE WHEN v.solde_meur IS NOT NULL AND b.solde_meur IS NOT NULL
              THEN b.solde_meur - v.solde_meur END AS ecart_meur,
         'rapprochement-v1'::text AS method_version
    FROM core.solde_vote v
    JOIN ref.loi_financiere l ON l.id = v.loi_id
    LEFT JOIN derived.budget_sous_secteur b
           ON b.annee = l.exercice
          AND b.comptabilite = v.comptabilite
          AND 'APU_' || b.secteur = v.perimetre;

COMMENT ON VIEW derived.budget_rapprochement IS
  'Rapproche un solde voté et un solde constaté UNIQUEMENT lorsque la '
  'comptabilité et le périmètre coïncident. Un écart nul de lignes n''est pas '
  'un défaut : c''est la source qui dit que ces deux chiffres ne se comparent pas.';

-- +goose Down
DROP VIEW derived.budget_rapprochement;
DROP VIEW derived.protection_sociale_total;
DROP VIEW derived.budget_sous_secteur;
DROP TABLE core.solde_vote;
DROP TABLE ref.loi_financiere;
DROP TABLE core.masse_salariale;
DROP TABLE core.exoneration_cotisation;
DROP TABLE core.execution_etat;
DROP TABLE core.protection_sociale;
DROP TABLE ref.budget_stade;
DROP TABLE ref.budget_perimetre;
DROP TABLE ref.budget_comptabilite;
