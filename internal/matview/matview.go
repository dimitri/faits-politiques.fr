// Package matview tient le catalogue des matérialisations Postgres du
// schéma mv (voir db/migrations/0152_matview_ballot.sql) et sait décider,
// avant chaque REFRESH, s'il y a réellement quelque chose à refaire.
//
// La politique tient en trois étages, du disque à l'affichage :
//
//  1. octets archivés (raw.document, via raw.source.etape — internal/
//     archive.Archive.Etape) : ce qui a été téléchargé.
//  2. données en base (core/ref/geo) : ce que l'ingestion en a tiré —
//     mesuré par internal/checksum.Section, DÉJÀ utilisée pour le cache de
//     construction (core.section_checksum, internal/sitegen/cache.go).
//  3. matvue (mv.*) : ce qu'une agrégation en a calculé — mesurée ici,
//     mv.etat, par le même principe (une empreinte des tables source, plus
//     une empreinte du SELECT qui définit la matvue).
//
// internal/sitegen ne doit plus jamais recalculer lui-même une agrégation qu'une
// matvue de ce paquet couvre : un simple SELECT dans le schéma mv, jamais
// un GROUP BY sur une table brute à la construction (voir la revue qui a
// mené à ce paquet — internal/sitegen/scrutins.go, groupBreakdown, un GROUP BY sur
// la totalité de core.ballot À CHAQUE CONSTRUCTION).
package matview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/checksum"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Definition décrit une matvue du schéma mv : son nom, les tables dont son
// SELECT dépend (pour internal/checksum.Section — le signal « faut-il la
// refaire »), et ce SELECT lui-même (pour le sql_hash, et pour qu'il vive
// une seule fois dans le code Go — la migration qui CREATE la matvue en
// porte une copie littérale, à garder synchronisée : voir le commentaire
// de chaque migration mv.*).
type Definition struct {
	Nom    string   // "scrutin_groupe_vote" — sans le schéma
	Tables []string // tables qualifiées dont dépend le SELECT (checksum.Section)
	SQL    string   // le SELECT qui définit la matvue, pour le sql_hash
}

// QualifieNom : mv.<nom>, la forme utilisée dans REFRESH/SELECT.
func (d Definition) QualifieNom() string { return "mv." + d.Nom }

// Catalogue : les matvues connues. Une seule entrée aujourd'hui
// (scrutin_groupe_vote, le premier étage du chantier) — chaque nouvelle
// matvue s'y ajoute, jamais ailleurs : fpctl list matviews et
// ActualiserToutes n'ont besoin de connaître qu'elle.
var Catalogue = []Definition{
	{
		Nom:    "scrutin_groupe_vote",
		Tables: []string{"core.ballot", "core.organization"},
		SQL: `SELECT b.scrutin_id,
	     o.id                                             AS organization_id,
	     coalesce(o.short_name, o.name)                   AS organisation_nom,
	     o.slug                                            AS organisation_slug,
	     coalesce(b.position_rectifiee, b.position)::text AS position,
	     count(*)::int                                    AS nombre_votes
	FROM core.ballot b
	JOIN core.organization o ON o.id = b.organization_id
	GROUP BY 1, 2, 3, 4, 5`,
	},
	{
		Nom:    "scrutin_vote_nominal",
		Tables: []string{"core.ballot", "core.person", "core.organization"},
		SQL: `SELECT b.scrutin_id,
	     p.id                                                 AS person_id,
	     p.slug                                              AS person_slug,
	     p.family_name                                       AS person_family_name,
	     p.given_name                                        AS person_given_name,
	     o.id                                                 AS organization_id,
	     coalesce(o.short_name, o.name, '')                  AS organisation_nom,
	     coalesce(o.slug, '')                                AS organisation_slug,
	     coalesce(b.position_rectifiee, b.position)::text    AS position,
	     b.position_rectifiee IS NOT NULL                    AS rectifiee
	FROM core.ballot b
	JOIN core.person p ON p.id = b.person_id
	LEFT JOIN core.organization o ON o.id = b.organization_id`,
	},
	{
		Nom:    "person_dernier_vote",
		Tables: []string{"core.ballot", "core.scrutin"},
		SQL: `SELECT person_id, rang, scrutin_slug, objet, date_txt, position, rectifiee, resultat
	FROM (
	  SELECT b.person_id,
	         row_number() OVER (PARTITION BY b.person_id
	                             ORDER BY s.date_seance DESC, s.numero DESC) AS rang,
	         s.slug                                              AS scrutin_slug,
	         s.objet,
	         to_char(s.date_seance,'DD/MM/YYYY')                  AS date_txt,
	         coalesce(b.position_rectifiee, b.position)::text    AS position,
	         b.position_rectifiee IS NOT NULL                    AS rectifiee,
	         coalesce(s.resultat,'')                              AS resultat
	    FROM core.ballot b
	    JOIN core.scrutin s ON s.id = b.scrutin_id
	) x
	WHERE rang <= 100`,
	},
	// Les six matvues de loadTerritoires (internal/sitegen/territoires.go).
	// dept_population D'ABORD dans ce fichier : Registre.Ajouter (registre,
	// plus bas) panique si une matvue est déclarée avant celle qu'elle cite
	// dans Tables — les quatre suivantes la lisent par SELECT (une matvue
	// construite sur une autre, voir la migration 0157), donc leur place ici
	// doit rester après. L'ORDRE D'EXÉCUTION réel, lui, ne dépend plus de
	// cette place : registre() lit Tables et construit le graphe de
	// dépendances que REFRESH ne cascade jamais tout seul.
	{
		Nom:    "dept_population",
		Tables: []string{"core.commune_indicator", "ref.commune"},
		SQL: `SELECT c.code_departement,
	     max(c.nom_clair)  AS nom_departement,
	     d.period_year,
	     sum(d.value)       AS population
	FROM core.commune_indicator d
	JOIN ref.commune c ON c.code_insee = d.commune_code AND c.cog_millesime = d.cog_millesime
	WHERE d.indicator_code = 'ofgl.population_totale'
	GROUP BY c.code_departement, d.period_year`,
	},
	{
		Nom:    "dept_indicateur_communal",
		Tables: []string{"core.commune_indicator", "ref.commune"},
		SQL: `SELECT c.code_departement,
	     max(c.nom_clair)                                       AS nom_departement,
	     d.indicator_code,
	     d.period_year,
	     sum(d.value * p.value) / nullif(sum(p.value), 0)        AS valeur_par_hab
	FROM core.commune_indicator d
	JOIN core.commune_indicator p ON p.commune_code = d.commune_code
	 AND p.period_year = d.period_year AND p.indicator_code = 'ofgl.population_totale'
	JOIN ref.commune c ON c.code_insee = d.commune_code AND c.cog_millesime = d.cog_millesime
	WHERE d.indicator_code IN ('ofgl.dette_par_hab', 'ofgl.investissement_par_hab',
	                            'ofgl.epargne_brute_par_hab', 'ofgl.masse_salariale_par_hab')
	GROUP BY c.code_departement, d.indicator_code, d.period_year`,
	},
	{
		Nom:    "dept_association_densite",
		Tables: []string{"ref.commune", "core.association", "mv.dept_population"},
		SQL: `SELECT c.code_departement,
	     max(c.nom_clair)                                           AS nom_departement,
	     1000.0 * count(a.rna_id) / nullif(max(pop.population), 0)  AS pour_mille
	FROM ref.commune c
	JOIN mv.dept_population pop ON pop.code_departement = c.code_departement AND pop.period_year = 2023
	LEFT JOIN core.association a ON a.commune_code = c.code_insee
	GROUP BY c.code_departement`,
	},
	{
		Nom:    "dept_medecin_generaliste",
		Tables: []string{"core.medecin_secteur_effectif", "mv.dept_population"},
		SQL: `SELECT m.dep,
	     max(m.libelle_departement)                                  AS nom_departement,
	     100000.0 * sum(m.effectif) / nullif(max(pop.population), 0) AS pour_100k
	FROM (SELECT *, CASE WHEN code_departement ~ '^[0-9]$'
	                 THEN '0' || code_departement ELSE code_departement END AS dep
	        FROM core.medecin_secteur_effectif) m
	JOIN mv.dept_population pop ON pop.code_departement = m.dep AND pop.period_year = 2023
	WHERE m.annee = 2024 AND m.code_departement <> '999'
	  AND m.profession_sante IN
	    ('Médecins généralistes (hors médecins à expertise particulière - MEP)',
	     'Médecins généralistes à expertise particulière (MEP)')
	GROUP BY m.dep`,
	},
	{
		Nom:    "dept_part_partisane",
		Tables: []string{"core.municipal_list", "ref.commune"},
		SQL: `WITH s AS (
	  SELECT c.code_departement AS dep, max(c.nom_clair) AS nom, ml.nuance_code AS nc, sum(ml.sieges_cm) AS sg
	    FROM core.municipal_list ml
	    JOIN ref.commune c ON c.code_insee = ml.commune_code AND c.cog_millesime = ml.cog_millesime
	   WHERE ml.scrutin_annee = 2026 AND ml.sieges_cm > 0
	     AND ml.nuance_code IS NOT NULL AND ml.nuance_code <> ''
	   GROUP BY 1, 3)
	SELECT dep,
	     max(nom)                                                      AS nom_departement,
	     100.0 * coalesce(sum(sg) FILTER (WHERE nc IN
	       ('LLR','LRN','LSOC','LFI','LCOM','LVEC','LUDR','LUXD','LEXD','LECO')), 0) / sum(sg) AS pct
	FROM s
	GROUP BY 1`,
	},
	{
		Nom:    "collectivite_budget_pivot",
		Tables: []string{"core.collectivite_budget"},
		SQL: `SELECT niveau, code, exercice,
	     max(nom)                                    AS nom,
	     max(population)                              AS population,
	     jsonb_object_agg(indicator_code, montant)     AS totaux,
	     jsonb_object_agg(indicator_code, euros_par_hab) AS par_hab
	FROM core.collectivite_budget
	WHERE niveau IN ('REGION', 'DEPARTEMENT')
	GROUP BY niveau, code, exercice`,
	},
	{
		Nom:    "epci",
		Tables: []string{"core.epci", "core.epci_competence"},
		SQL: `SELECT e.siren, e.nom, e.nature_juridique,
	     coalesce(e.code_departement, '')                                          AS code_departement,
	     coalesce(e.population_totale, 0)                                          AS population,
	     coalesce(e.nb_membres, 0)                                                  AS nb_membres,
	     trim(coalesce(e.president_prenom, '') || ' ' || coalesce(e.president_nom, '')) AS president,
	     (SELECT count(*) FROM core.epci_competence x WHERE x.epci_siren = e.siren)  AS nb_competences
	FROM core.epci e
	WHERE e.nature_juridique = ANY(ARRAY['CC','CA','CU','METRO','MET69','EPT'])`,
	},
	{
		Nom:    "epci_budget_exercice",
		Tables: []string{"core.collectivite_budget"},
		SQL: `SELECT code AS siren, exercice,
	     jsonb_object_agg(indicator_code, euros_par_hab) AS par_hab
	FROM core.collectivite_budget
	WHERE niveau = 'GROUPEMENT'
	GROUP BY code, exercice`,
	},
	{
		Nom:    "dept_budget_commune",
		Tables: []string{"core.commune_indicator", "ref.commune"},
		SQL: `SELECT rc.code_departement,
	     f.period_year,
	     coalesce(sum(f.value*p.value) FILTER (WHERE f.indicator_code='ofgl.fonctionnement_par_hab'),0)::float8 AS fonctionnement,
	     coalesce(sum(f.value*p.value) FILTER (WHERE f.indicator_code='ofgl.investissement_par_hab'),0)::float8 AS investissement
	FROM core.commune_indicator f
	JOIN core.commune_indicator p ON p.commune_code=f.commune_code AND p.period_year=f.period_year
	 AND p.indicator_code='ofgl.population_totale'
	JOIN ref.commune rc ON rc.code_insee=f.commune_code AND rc.cog_millesime=f.cog_millesime
	WHERE f.indicator_code IN ('ofgl.fonctionnement_par_hab','ofgl.investissement_par_hab')
	GROUP BY rc.code_departement, f.period_year`,
	},
	{
		Nom:    "dept_budget_epci",
		Tables: []string{"core.collectivite_budget", "core.epci"},
		SQL: `SELECT e.code_departement,
	     b.exercice,
	     coalesce(sum(b.montant) FILTER (WHERE b.indicator_code='ofgl.fonctionnement_par_hab'),0)::float8 AS fonctionnement,
	     coalesce(sum(b.montant) FILTER (WHERE b.indicator_code='ofgl.investissement_par_hab'),0)::float8 AS investissement
	FROM core.collectivite_budget b JOIN core.epci e ON e.siren=b.code
	WHERE b.niveau='GROUPEMENT' AND e.code_departement IS NOT NULL
	GROUP BY e.code_departement, b.exercice`,
	},
	{
		Nom:    "securite_dept_annee",
		Tables: []string{"core.commune_delinquance", "ref.commune"},
		SQL: `SELECT d.indicateur_code,
	     c.code_departement,
	     max(c.nom_clair) AS nom_departement,
	     d.annee,
	     sum(d.nombre)     AS nombre,
	     sum(d.population) AS population
	FROM core.commune_delinquance d
	JOIN ref.commune c ON c.code_insee = d.commune_code AND c.cog_millesime = d.cog_millesime
	WHERE d.diffuse
	GROUP BY d.indicateur_code, c.code_departement, d.annee`,
	},
	// person_actif D'ABORD dans ce fichier, même raison que dept_population
	// plus haut : mandate_actif et affiliation_actif la lisent par SELECT.
	{
		Nom:    "person_actif",
		Tables: []string{"core.person", "core.ballot", "core.mandate"},
		SQL: `SELECT p.id, p.slug, p.given_name, p.family_name
	FROM core.person p
	WHERE EXISTS (SELECT 1 FROM core.ballot b WHERE b.person_id = p.id)
	   OR EXISTS (SELECT 1 FROM core.mandate m WHERE m.person_id = p.id
	               AND m.mandate_type IN ('DEPUTE','SENATEUR','DEPUTE_EUROPEEN',
	                                      'MINISTRE','PRESIDENT_REPUBLIQUE'))`,
	},
	{
		Nom:    "mandate_actif",
		Tables: []string{"core.mandate", "mv.person_actif"},
		SQL: `SELECT m.person_id, m.mandate_type, m.constituency, m.role, m.portefeuille,
	     m.validity, m.commune_code
	FROM core.mandate m
	WHERE m.person_id IN (SELECT id FROM mv.person_actif)`,
	},
	{
		Nom:    "affiliation_actif",
		Tables: []string{"core.affiliation", "core.organization", "mv.person_actif"},
		SQL: `SELECT a.person_id, o.name AS organization_nom, a.organization_kind, a.declared_via, a.validity
	FROM core.affiliation a
	JOIN core.organization o ON o.id = a.organization_id
	WHERE a.person_id IN (SELECT id FROM mv.person_actif)`,
	},
	// Décompte sur la position D'ORIGINE (pas position_rectifiee) : distinct
	// de mv.scrutin_vote_nominal à dessein, voir loadPersons (internal/
	// sitegen/load.go) — confondre les deux changerait silencieusement le
	// décompte de 1 612 bulletins rectifiés.
	{
		Nom:    "person_position_brute",
		Tables: []string{"core.ballot"},
		SQL: `SELECT person_id, position::text AS position, count(*)::int AS nombre_votes
	FROM core.ballot
	GROUP BY 1, 2`,
	},
	// Président/vice-présidents des conseils régionaux/départementaux/
	// communautaires : un rôle très spécifique parmi 617 196 mandats — 13 044
	// lignes en poste, moins de 3% de la table.
	{
		Nom:    "mandat_executif_local",
		Tables: []string{"core.mandate", "core.person"},
		SQL: `SELECT m.constituency, m.role, p.slug AS person_slug,
	     p.given_name AS person_given_name, p.family_name AS person_family_name,
	     lower(m.validity) AS depuis
	FROM core.mandate m JOIN core.person p ON p.id = m.person_id
	WHERE m.role LIKE '%résident%conseil%' AND m.constituency IS NOT NULL
	  AND upper(m.validity) IS NULL`,
	},
	// Effectifs des conseils régionaux/départementaux (par territoire) et
	// communautaires (total national) : trois GROUP BY que collectivites.go
	// refaisait sur la totalité de core.mandate à chaque construction.
	{
		Nom:    "mandat_local_compte",
		Tables: []string{"core.mandate"},
		SQL: `SELECT mandate_type::text AS mandate_type, left(constituency,2) AS code_territoire,
	     count(*)::int AS nombre_elus
	FROM core.mandate
	WHERE mandate_type IN ('CONSEILLER_REGIONAL','CONSEILLER_DEPARTEMENTAL')
	  AND constituency IS NOT NULL AND upper(validity) IS NULL
	GROUP BY 1, 2
	UNION ALL
	SELECT mandate_type::text, NULL, count(*)::int
	FROM core.mandate
	WHERE mandate_type = 'CONSEILLER_COMMUNAUTAIRE' AND upper(validity) IS NULL
	GROUP BY 1`,
	},
	// Un count(*) par source, page /sources (chargerStatsGlobalesSources
	// et la boucle Enregistrements de sources.go) — remplace un JOIN à
	// quatre tables filtré par slug, rejoué une fois par source (166 fois),
	// sur la totalité de raw.record (489 Mo, 177 026 lignes).
	{
		Nom:    "source_enregistrements",
		Tables: []string{"raw.record", "raw.document", "raw.retrieval", "raw.source"},
		SQL: `SELECT s.slug AS source_slug, count(*)::int AS nombre_enregistrements
	FROM raw.record rec
	JOIN raw.document d ON d.id = rec.document_id
	JOIN raw.retrieval r ON r.document_id = d.id
	JOIN raw.source s ON s.id = r.source_id
	GROUP BY s.slug`,
	},
	{
		Nom:    "commune_association_count",
		Tables: []string{"core.association"},
		SQL: `SELECT commune_code, count(*) AS nombre_associations
	FROM core.association
	WHERE commune_code IS NOT NULL
	GROUP BY commune_code`,
	},
	{
		Nom:    "population_nationale_annee",
		Tables: []string{"core.population_historique_commune"},
		SQL: `SELECT annee, sum(population) AS population
	FROM core.population_historique_commune
	GROUP BY annee`,
	},
	{
		Nom:    "dept_rsa",
		Tables: []string{"core.prestation_solidarite", "mv.dept_population"},
		SQL: `SELECT p.code_geo,
	     p.nom_geo,
	     p.mois,
	     1000.0 * p.valeur / nullif(pop.population, 0) AS pour_mille
	FROM core.prestation_solidarite p
	JOIN mv.dept_population pop ON pop.code_departement = p.code_geo AND pop.period_year = 2023
	WHERE p.niveau = 'DEPARTEMENT' AND p.serie = 'RSA_beneficiaires'
	  AND p.mois = (SELECT max(mois) FROM core.prestation_solidarite
	                 WHERE serie = 'RSA_beneficiaires' AND niveau = 'DEPARTEMENT')`,
	},
}

// TableDirecte : une table de core/ref/geo que internal/sitegen lit
// DIRECTEMENT, sans passer par une matvue — parce qu'aucun pré-agrégat,
// pré-jointure ou pré-filtre ne réduirait ce qu'elle sert : le code Go en
// lit l'essentiel du contenu (« en gros »), pour une table déjà petite.
// Une matvue miroir n'y ajouterait rien — voir la migration 0165, qui en
// supprime trois créées avant que cette distinction soit posée : mv.
// scrutin/mv.macro_value/mv.macro_serie ne faisaient QUE recopier leur
// source, doublant le stockage et le coût de REFRESH pour un gain nul.
//
// Suivies ici EXPLICITEMENT plutôt que devinées : le périmètre d'un dump
// -Fc pour reconstruire le site en CI (voir docs/ci-pipeline.md) est le
// schéma mv EN ENTIER, PLUS ces tables-ci — jamais mv seul. Un export qui
// oublierait une entrée ici ferait échouer fpctl build site en CI sur une
// relation manquante, pas silencieusement : voir Perimetre().
type TableDirecte struct {
	Table  string // schema.table qualifié
	Raison string // pourquoi une matvue n'ajouterait rien ici
}

// TablesDirectes : la liste tenue à la main, dans le même esprit que
// checksum.Sections ou ingestPrealables (cmd/fpctl/build.go) — sous-couvrir
// cette liste ne fait jamais construire une page fausse (l'échec est un
// dump CI incomplet, détecté tout de suite par une relation manquante),
// au pire un export CI trop large.
var TablesDirectes = []TableDirecte{
	{"core.scrutin", "lu par institution/date/slug/dossier_id dans huit fichiers, jamais agrégé — 38 042 lignes"},
	{"core.macro_value", "lu par serie_code dans onze fichiers, jamais agrégé — 1 988 lignes"},
	{"ref.macro_serie", "table de libellés pour macro_value — 61 lignes"},
	{"core.organization", "table de référence (nom, slug, type) largement réutilisée, sans agrégation dominante — 5 650 lignes"},
	// core.mandate/core.person : le sous-ensemble « national » (députés,
	// sénateurs, ministres...) est réductible et sert de base à mv.
	// person_actif/mandate_actif/affiliation_actif ci-dessus. Mais l'écrasante
	// majorité des lignes (conseillers municipaux, communautaires — 605 654
	// sur 617 196 mandats, quasi toutes actives : aucun filtre de validité ne
	// les réduit) sert les pages locales (internal/sitegen/lieux_pages.go,
	// candidat_local.go) qui ont besoin de CHAQUE élu en poste, commune par
	// commune — une matvue n'y changerait rien, ce serait le même volume.
	{"core.mandate", "élus locaux en poste (conseillers municipaux/communautaires) lus wholesale par lieux_pages.go/candidat_local.go — 605 654 lignes actives sur 617 196"},
	{"core.person", "identité des élus locaux, jointe 1:1 sur core.mandate wholesale dans les mêmes fichiers — 515 374 lignes"},
	// Deux tables de contrôle, pas de contenu éditorial : lues sans
	// condition au tout début de main.go (page Sources, cache de
	// construction), avant même qu'-only choisisse quoi ÉCRIRE — donc
	// nécessaires au périmètre CI quelle que soit la section demandée.
	{"raw.source", "métadonnées des flux archivés, page /sources — 166 lignes"},
	{"raw.retrieval", "historique des relevés par source, page /sources — 15 330 lignes"},
	{"raw.document", "un document par relevé, page /sources — 4 482 lignes"},
	{"raw.fetch_run", "dernier statut d'ingestion par source, page /sources — 734 lignes"},
	{"core.section_checksum", "empreintes du cache de construction (internal/sitegen/cache.go) — 2 lignes"},
}

// Perimetre : le nom qualifié de chaque objet nécessaire pour reconstruire
// le site depuis un dump — toutes les matvues du Catalogue (schéma mv,
// PAS leurs tables source : le contenu déjà calculé suffit, aucun REFRESH
// n'est nécessaire en CI) plus TablesDirectes. Sert à construire la
// commande pg_dump -Fc du chantier CI (« shallow clone ») — voir
// docs/ci-pipeline.md.
func Perimetre() []string {
	out := make([]string, 0, len(Catalogue)+len(TablesDirectes))
	for _, def := range Catalogue {
		out = append(out, def.QualifieNom())
	}
	for _, t := range TablesDirectes {
		out = append(out, t.Table)
	}
	return out
}

func sqlHash(sql string) string {
	h := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(h[:])
}

// Etat : la dernière ligne connue de mv.etat pour une matvue — nil (pas
// d'erreur) si elle n'a jamais été actualisée depuis que cette ligne existe.
type Etat struct {
	DataHash     string
	SQLHash      string
	Lignes       int64
	ActualiseeLe time.Time
}

func lireEtat(ctx context.Context, pool *pgxpool.Pool, nom string) (*Etat, error) {
	var e Etat
	err := pool.QueryRow(ctx,
		`SELECT data_hash, sql_hash, lignes, actualisee_le FROM mv.etat WHERE nom = $1`, nom,
	).Scan(&e.DataHash, &e.SQLHash, &e.Lignes, &e.ActualiseeLe)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

// Actualiser vérifie si def a besoin d'un REFRESH (empreinte des tables
// source ou du SELECT qui la définit différente de mv.etat) et, si oui, le
// fait — REFRESH et mise à jour de mv.etat dans LA MÊME TRANSACTION : un
// crash entre les deux ne laisse jamais mv.etat prétendre une fraîcheur que
// la matvue n'a pas atteinte, ni un REFRESH réel sans trace.
//
// Le calcul de l'empreinte des tables source (internal/checksum.Section)
// N'EST PAS dans cette transaction — il ne modifie rien, et pgcopydp-style
// hashtext() sur 4,9 millions de lignes (core.ballot) prend quelques
// secondes : l'exécuter hors transaction évite de tenir une transaction
// ouverte plus longtemps que nécessaire.
func Actualiser(ctx context.Context, pool *pgxpool.Pool, def Definition) (rafraichie bool, err error) {
	dataHash, err := checksum.Section(ctx, pool, def.Tables)
	if err != nil {
		return false, fmt.Errorf("empreinte des tables de %s : %w", def.Nom, err)
	}
	sqlH := sqlHash(def.SQL)

	actuel, err := lireEtat(ctx, pool, def.Nom)
	if err != nil {
		return false, fmt.Errorf("état de mv.%s : %w", def.Nom, err)
	}
	if actuel != nil && actuel.DataHash == dataHash && actuel.SQLHash == sqlH {
		return false, nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "REFRESH MATERIALIZED VIEW "+def.QualifieNom()); err != nil {
		return false, fmt.Errorf("REFRESH %s : %w", def.QualifieNom(), err)
	}
	var lignes int64
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+def.QualifieNom()).Scan(&lignes); err != nil {
		return false, fmt.Errorf("comptage de %s : %w", def.QualifieNom(), err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO mv.etat (nom, data_hash, sql_hash, lignes, actualisee_le)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (nom) DO UPDATE
		SET data_hash = excluded.data_hash, sql_hash = excluded.sql_hash,
		    lignes = excluded.lignes, actualisee_le = excluded.actualisee_le`,
		def.Nom, dataHash, sqlH, lignes); err != nil {
		return false, fmt.Errorf("mv.etat de %s : %w", def.Nom, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// registre construit le pipeline.Registre du Catalogue pour pool : chaque
// Definition devient une Etape dont les Dependances sont lues directement
// dans Tables — toute entrée qui commence par "mv." y nomme une AUTRE
// matvue du Catalogue (jamais une TableDirecte, qui ne vit pas dans ce
// schéma), donc c'est exactement le nom qu'attend Registre.Ajouter.
// Catalogue reste déclaré dépendance-d'abord (dept_population avant ce qui
// la lit, person_actif avant ce qui le lit) : Ajouter panique sinon, donc
// cette construction est elle-même une vérification que l'ordre du fichier
// reste correct.
//
// pool volontairement absent de pipeline.NouveauRegistre : le journal
// core.pipeline_etape que pipeline.Registre.Executer tient à jour reste
// réservé à l'ingest (internal/ingest), jamais à ce registre-ci — un
// Registre sans pool saute cette écriture (voir pipeline.go, Executer).
func registre(pool *pgxpool.Pool) *pipeline.Registre {
	reg := pipeline.NouveauRegistre(nil)
	for _, def := range Catalogue {
		def := def
		var dependances []string
		for _, t := range def.Tables {
			if nom, ok := strings.CutPrefix(t, "mv."); ok {
				dependances = append(dependances, nom)
			}
		}
		reg.Ajouter(pipeline.Etape{
			Nom:         def.Nom,
			Description: "checking " + def.QualifieNom(),
			Dependances: dependances,
			Executer: func(ctx context.Context, _ pipeline.Results) (any, error) {
				rafraichie, err := Actualiser(ctx, pool, def)
				if err != nil {
					return nil, fmt.Errorf("mv.%s : %w", def.Nom, err)
				}
				if rafraichie {
					logs.Notice(def.QualifieNom() + " refreshed")
				} else {
					logs.Notice(def.QualifieNom() + " already up to date")
				}
				return rafraichie, nil
			},
		})
	}
	return reg
}

// ActualiserToutes actualise chaque matvue du Catalogue, dans l'ordre de
// dépendance déclaré (registre) plutôt que dans l'ordre du fichier tenu à
// la main — une matvue construite sur une autre (mv.dept_population,
// mv.person_actif) est désormais garantie à jour avant que sa dépendante ne
// soit vérifiée, par construction du graphe plutôt que par une place
// correcte dans Catalogue. À la concurrence partagée par défaut (4, celle du
// pool qu'ouvre internal/store.Open) — pour un appelant qui n'a pas de
// raison de s'en écarter. Voir ActualiserToutesConcurrence pour celui qui en
// a une.
func ActualiserToutes(ctx context.Context, pool *pgxpool.Pool) error {
	return ActualiserToutesConcurrence(ctx, pool, 4)
}

// ActualiserToutesConcurrence : comme ActualiserToutes, jusqu'à `concurrence`
// de front plutôt que 4 — la plupart des 25 matvues n'ont aucune dépendance
// entre elles (voir Catalogue), les enchaîner à une concurrence bridée par
// défaut n'avait jamais été qu'un héritage du pool à 4 connexions que
// RunSources/RunSource/RunCategorie/RunTout ouvrent pour tout le reste de
// l'ingestion, jamais une limite propre à cette étape.
//
// L'appelant doit fournir un pool dont MaxConns couvre CETTE concurrence
// (voir internal/store.OpenWithMaxConns, déjà le choix d'internal/sitegen
// pour le même besoin) : demander plus de front que le pool n'a de
// connexions ne fait que mettre des goroutines en attente de la même
// poignée de connexions, sans rien paralléliser de plus.
func ActualiserToutesConcurrence(ctx context.Context, pool *pgxpool.Pool, concurrence int) error {
	reg := registre(pool)
	_, err := reg.Executer(ctx, reg.Noms(), pipeline.Options{Concurrence: concurrence})
	return err
}

// EtatAffiche : une ligne du catalogue, avec son état connu (ou son
// absence) — pour fpctl list matviews.
type EtatAffiche struct {
	Nom          string
	Tables       []string
	Connue       bool
	Lignes       int64
	ActualiseeLe time.Time
}

// Lister renvoie l'état affiché de chaque matvue du Catalogue, dans l'ordre
// de déclaration — lit mv.etat tel quel, ne recalcule aucune empreinte
// (donc ne dit pas si une matvue est PÉRIMÉE, seulement quand et sur
// combien de lignes elle a tourné pour la dernière fois : le recalcul de
// l'empreinte des tables source coûte, sur core.ballot, plusieurs secondes
// — pas le prix d'un simple affichage).
func Lister(ctx context.Context, pool *pgxpool.Pool) ([]EtatAffiche, error) {
	out := make([]EtatAffiche, 0, len(Catalogue))
	for _, def := range Catalogue {
		e, err := lireEtat(ctx, pool, def.Nom)
		if err != nil {
			return nil, fmt.Errorf("état de mv.%s : %w", def.Nom, err)
		}
		ea := EtatAffiche{Nom: def.Nom, Tables: def.Tables}
		if e != nil {
			ea.Connue = true
			ea.Lignes = e.Lignes
			ea.ActualiseeLe = e.ActualiseeLe
		}
		out = append(out, ea)
	}
	return out, nil
}
