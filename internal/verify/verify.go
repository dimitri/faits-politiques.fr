// Package verify exécute les contrôles de cohérence rejoués avant toute
// publication. Appelé par fpctl (voir cmd/fpctl) : fpctl verify data.
//
// Ils portent sur les DONNÉES chargées, là où db/tests/ porte sur les règles du
// schéma. Un écart signale une erreur d'ingestion : publier des chiffres qui ne
// concordent pas avec le relevé officiel serait la pire faute possible pour un
// outil de vérification.
package verify

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/store"
)

// Une vérification exprime un nombre d'ANOMALIES : elle passe à zéro.
// Les vérifications de volume (min) existent parce qu'un contrôle de
// concordance passe TRIVIALEMENT quand aucune donnée n'est chargée. Une porte
// de publication qui s'ouvre sur une base vide est pire qu'inutile : elle
// donne l'illusion d'avoir vérifié.
type check struct {
	name  string
	query string
	min   int // si > 0 : la requête compte des lignes attendues, pas des anomalies
}

var checks = []check{
	{
		name:  "des scrutins sont chargés",
		query: `SELECT count(*) FROM core.scrutin`,
		min:   1000,
	},
	{
		name:  "des votes nominatifs sont chargés",
		query: `SELECT count(*) FROM core.ballot`,
		min:   100000,
	},
	{
		name: "des scrutins sont comparables au relevé officiel",
		query: `SELECT count(DISTINCT s.id) FROM core.scrutin s
		        JOIN core.ballot b ON b.scrutin_id = s.id
		        WHERE s.granularite = 'INDIVIDUAL'`,
		min: 1000,
	},
	{
		// Ce contrôle aurait détecté un bug réel : une attribution de groupe
		// reconstituée à partir des mandats plaçait les 577 députés dans
		// « Non inscrit », les fichiers de mandats publiés ne portant pas les
		// groupes de la 17e législature.
		// Deux régressions réelles ont détruit des données par cascade : un
		// TRUNCATE sur core.organization emportant les comptes CNCCFP, puis un
		// TRUNCATE sur core.dossier emportant les scrutins et leurs 1,27 million
		// de votes. Ces seuils sont les sondes qui rendent une telle perte
		// impossible à publier.
		name:  "les scrutins du Sénat sont chargés",
		query: `SELECT count(*) FROM core.scrutin WHERE institution = 'SENAT'`,
		min:   4000,
	},
	{
		// Correction d'une affirmation longtemps portée par ce projet : le Sénat
		// publie bien les votes INDIVIDUELS de ses membres.
		name: "les votes nominatifs du Sénat sont chargés",
		query: `SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id = b.scrutin_id
		        WHERE s.institution = 'SENAT'`,
		min: 1000000,
	},
	{
		name: "les dossiers du Sénat portent leur thème officiel",
		query: `SELECT count(*) FROM core.topic_assignment a
		        JOIN ref.topic t ON t.code = a.topic_code AND t.taxonomy_version = 'senat'`,
		min: 10000,
	},
	{
		// Les décomptes publiés incluent les délégations et les non-votants :
		// la somme des positions chargées ne peut qu'être inférieure ou égale.
		// Deux scrutins sur 4 764 comptent une voix « pour » de plus dans le
		// relevé nominatif que dans le décompte publié — 2020/9 et 2024/93. Aucune
		// correction n'est enregistrée dans la table corscr : l'incohérence est
		// dans la source. On la borne à une voix plutôt que de la masquer : un
		// défaut d'ingestion produirait des écarts massifs, pas des écarts d'une
		// unité.
		name: "aucun scrutin du Sénat ne s'écarte de plus d'une voix du total publié",
		query: `SELECT count(*) FROM (
		          SELECT s.id FROM core.scrutin s JOIN core.ballot b ON b.scrutin_id = s.id
		          WHERE s.institution = 'SENAT' AND s.nb_pour IS NOT NULL
		          GROUP BY s.id, s.nb_pour
		          HAVING count(*) FILTER (WHERE b.position='FOR') > s.nb_pour + 1) x`,
	},
	{
		name:  "les scrutins du Parlement européen sont chargés",
		query: `SELECT count(*) FROM core.scrutin WHERE institution = 'PARLEMENT_EUROPEEN'`,
		min:   10000,
	},
	{
		name: "les votes des eurodéputés français sont chargés",
		query: `SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id = b.scrutin_id
		        WHERE s.institution = 'PARLEMENT_EUROPEEN'`,
		min: 500000,
	},
	{
		name: "les scrutins européens portent des thèmes EuroVoc",
		query: `SELECT count(*) FROM core.topic_assignment a
		        JOIN ref.topic t ON t.code = a.topic_code AND t.taxonomy_version = 'eurovoc'`,
		min: 10000,
	},
	{
		// Les décomptes publiés pour un scrutin européen portent sur TOUT le
		// Parlement, alors que seuls les votes français sont chargés. Ce contrôle
		// vérifie donc l'inclusion, pas l'égalité.
		name: "la part française d'un scrutin européen n'excède jamais le total publié",
		query: `SELECT count(*) FROM (
		          SELECT s.id FROM core.scrutin s JOIN core.ballot b ON b.scrutin_id = s.id
		          WHERE s.institution = 'PARLEMENT_EUROPEEN'
		          GROUP BY s.id, s.nb_pour, s.nb_contre, s.nb_abstentions
		          HAVING count(*) FILTER (WHERE b.position='FOR') > s.nb_pour
		              OR count(*) FILTER (WHERE b.position='AGAINST') > s.nb_contre) x`,
	},
	{
		name:  "les comptes publics des partis sont chargés",
		query: `SELECT count(*) FROM core.party_account_line`,
		min:   10000,
	},
	{
		name:  "les classifications tierces sont chargées",
		query: `SELECT count(*) FROM core.party_classification`,
		min:   10,
	},
	{
		name:  "les dossiers législatifs sont chargés",
		query: `SELECT count(*) FROM core.dossier`,
		min:   1000,
	},
	{
		name:  "les dossiers portent un initiateur",
		query: `SELECT count(*) FROM core.dossier_author`,
		min:   1000,
	},
	{
		name:  "les scrutins sont rattachés à leur dossier",
		query: `SELECT count(*) FROM core.scrutin WHERE dossier_id IS NOT NULL`,
		min:   2000,
	},
	{
		name:  "des portraits et logos libres sont disponibles",
		query: `SELECT count(*) FROM core.media`,
		min:   10,
	},
	{
		name: "la ventilation par groupe est plausible",
		query: `SELECT count(*) FROM (
		          SELECT 1 FROM core.ballot b
		          JOIN core.organization o ON o.id = b.organization_id
		          GROUP BY o.id HAVING count(*) > 1000) x`,
		min: 8,
	},
	{
		// Borné aux institutions dont la source publie l'appartenance de groupe.
		// Le dump Dosleg du Sénat ne la contient pas : ses votes n'ont donc pas
		// de groupe, et l'inclure ici ferait échouer un contrôle pour une raison
		// qui n'est pas un défaut de notre ingestion.
		name: "la quasi-totalité des votes porte un groupe, là où la source le publie",
		query: `SELECT (100 * count(*) FILTER (WHERE b.organization_id IS NOT NULL))
		               / greatest(count(*),1)
		        FROM core.ballot b JOIN core.scrutin s ON s.id = b.scrutin_id
		        WHERE s.institution IN ('ASSEMBLEE_NATIONALE','PARLEMENT_EUROPEEN')`,
		min: 95,
	},
	{
		name: "décomptes nominatifs concordants avec le relevé officiel",
		query: `WITH calc AS (
		   SELECT s.id, s.nb_pour, s.nb_contre, s.nb_abstentions,
		          count(*) FILTER (WHERE b.position='FOR')     c_pour,
		          count(*) FILTER (WHERE b.position='AGAINST') c_contre,
		          count(*) FILTER (WHERE b.position='ABSTAIN') c_abst
		   FROM core.scrutin s JOIN core.ballot b ON b.scrutin_id = s.id
		   WHERE s.granularite = 'INDIVIDUAL'
		     -- Borné à l'Assemblée : pour un scrutin européen, le décompte publié
		     -- porte sur tout le Parlement alors que seuls les votes français sont
		     -- chargés. Les comparer serait une erreur de périmètre.
		     AND s.institution = 'ASSEMBLEE_NATIONALE'
		   GROUP BY 1,2,3,4)
		 SELECT count(*) FROM calc
		 WHERE nb_pour <> c_pour OR nb_contre <> c_contre OR nb_abstentions <> c_abst`,
	},
	{
		name: "aucune période de validité vide",
		query: `SELECT (SELECT count(*) FROM core.mandate WHERE isempty(validity))
		      + (SELECT count(*) FROM core.affiliation WHERE isempty(validity))`,
	},
	{
		name: "aucun vote nominatif sur un scrutin de granularité GROUP",
		query: `SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id = b.scrutin_id
		 WHERE s.granularite <> 'INDIVIDUAL'`,
	},
	{
		name:  "aucun scrutin sans objet",
		query: `SELECT count(*) FROM core.scrutin WHERE coalesce(trim(objet),'') = ''`,
	},
	{
		name: "toute personne portant un vote a un identifiant officiel",
		query: `SELECT count(DISTINCT b.person_id) FROM core.ballot b
		 WHERE NOT EXISTS (SELECT 1 FROM core.person_identifier i
		                   WHERE i.person_id = b.person_id
		                     AND i.scheme IN ('AN_ACTEUR','SENAT_MATRICULE','EP_MEP'))`,
	},
	{
		// Borné à l'Assemblée : les scrutins européens viennent de fichiers CSV,
		// scellés eux aussi mais sans passer par raw.record.
		name: "tout scrutin de l'Assemblée porte une source archivée",
		query: `SELECT count(*) FROM core.scrutin s
		 WHERE s.institution = 'ASSEMBLEE_NATIONALE'
		   AND NOT EXISTS (SELECT 1 FROM raw.record r
		                   WHERE r.record_type = 'an.scrutin' AND r.natural_key = s.source_uid)`,
	},
	{
		// Sans ces liens, aucun score de positionnement ne se joint à un vote :
		// c'était l'état de la base avant la cartographie (D-020), et rien ne le
		// signalait.
		name: "la cartographie parti -> groupe est chargée",
		query: `SELECT count(*) FROM core.party_group_link l
		        JOIN core.mapping_revision r ON r.id = l.mapping_revision_id
		        JOIN core.mapping_lineage g ON g.id = r.lineage_id
		        WHERE g.kind = 'REFERENCE'`,
		min: 9,
	},
	{
		// Le contrôle qui compte : la chaîne complète, du score CHES au bulletin.
		// Un maillon rompu n'importe où fait tomber ce chiffre à zéro.
		name: "un score gauche-droite est joignable aux votes",
		query: `SELECT count(DISTINCT g.id)
		        FROM core.party_group_link pgl
		        JOIN core.organization g ON g.id = pgl.group_id
		        JOIN core.party_referential_link prl
		          ON prl.party_id = pgl.party_id AND prl.scheme = 'CHES'
		         AND prl.mapping_revision_id = pgl.mapping_revision_id
		        JOIN core.party_classification pc
		          ON pc.party_id = prl.referential_id AND pc.dimension = 'lrgen'
		        WHERE EXISTS (SELECT 1 FROM core.ballot b WHERE b.organization_id = g.id)`,
		min: 9,
	},
	{
		name:  "les thèmes hérités par la navette sont calculés",
		query: `SELECT count(DISTINCT scrutin_id) FROM derived.scrutin_topic WHERE origine = 'NAVETTE'`,
		min:   2000,
	},
	{
		// Un thème hérité doit rester remontable jusqu'à la loi qui le porte.
		name: "tout thème hérité cite le dossier du Sénat dont il vient",
		query: `SELECT count(*) FROM derived.scrutin_topic st
		        WHERE st.origine = 'NAVETTE'
		          AND NOT EXISTS (SELECT 1 FROM core.dossier d
		                          WHERE d.id = st.via_dossier_id AND d.institution = 'SENAT')`,
	},
	{
		// derived ne doit jamais contredire core : un thème direct est une
		// affirmation de la source, un thème hérité est un calcul. Les deux ne
		// peuvent pas porter sur le même couple.
		name: "aucun scrutin ne porte le même thème en direct et par héritage",
		query: `SELECT count(*) FROM derived.scrutin_topic a
		        JOIN derived.scrutin_topic b
		          ON b.scrutin_id = a.scrutin_id AND b.topic_code = a.topic_code
		         AND b.origine <> a.origine`,
	},
	{
		name:  "le référentiel des communes est chargé",
		query: `SELECT count(*) FROM ref.commune`,
		min:   34000,
	},
	{
		name:  "les maires sont chargés",
		query: `SELECT count(*) FROM core.mandate WHERE mandate_type = 'MAIRE'`,
		min:   30000,
	},
	{
		// Une commune à deux maires simultanés est impossible dans le monde
		// réel. La contrainte d'exclusion du schéma porte sur la PERSONNE et
		// non sur la commune : elle ne verrait pas ce cas.
		name: "aucune commune n'a deux maires sur la même période",
		query: `SELECT count(*) FROM (
		          SELECT commune_code FROM core.mandate
		           WHERE mandate_type = 'MAIRE'
		           GROUP BY commune_code, validity HAVING count(*) > 1) x`,
	},
	{
		name:  "les listes candidates aux municipales sont chargées",
		query: `SELECT count(*) FROM core.municipal_list`,
		min:   40000,
	},
	{
		// Le seuil de nuance est une caractéristique de la source, pas un
		// défaut de chargement : si cette part s'effondrait, c'est notre
		// ingestion qui aurait changé, pas la France.
		name: "la part de communes nuancées reste dans l'ordre de grandeur connu",
		query: `SELECT count(*) FROM derived.commune_couleur
		         WHERE scrutin_annee = 2026 AND statut = 'NUANCEE'`,
		min: 3000,
	},
	{
		name: "toute nuance portée par une liste existe dans la nomenclature",
		query: `SELECT count(*) FROM core.municipal_list m
		         WHERE m.nuance_code IS NOT NULL
		           AND NOT EXISTS (SELECT 1 FROM ref.nuance_politique n
		                           WHERE n.code = m.nuance_code
		                             AND n.circulaire_millesime = m.circulaire_millesime)`,
	},
	{
		// La couleur est une déduction : elle doit toujours pointer la liste
		// dont elle est tirée, sinon elle n'est pas remontable.
		name:  "toute couleur de commune cite la liste dont elle est déduite",
		query: `SELECT count(*) FROM derived.commune_couleur WHERE municipal_list_id IS NULL`,
	},
	{
		name:  "les comptes communaux de l'OFGL sont chargés",
		query: `SELECT count(*) FROM core.commune_indicator WHERE indicator_code LIKE 'ofgl.%'`,
		min:   1000000,
	},
	{
		// Sans plusieurs exercices, la dimension locale se réduit à une photo.
		name:  "les comptes communaux couvrent plusieurs exercices",
		query: `SELECT count(DISTINCT period_year) FROM core.commune_indicator WHERE indicator_code LIKE 'ofgl.%'`,
		min:   7,
	},
	{
		name: "les mandats locaux sont chargés, tous niveaux",
		query: `SELECT count(DISTINCT mandate_type) FROM core.mandate
		         WHERE mandate_type IN ('MAIRE','CONSEILLER_MUNICIPAL','CONSEILLER_COMMUNAUTAIRE',
		                                'CONSEILLER_DEPARTEMENTAL','CONSEILLER_REGIONAL',
		                                'SENATEUR','DEPUTE','DEPUTE_EUROPEEN')`,
		min: 8,
	},
	{
		name:  "les présidences de la République sont chargées",
		query: `SELECT count(*) FROM core.mandate WHERE mandate_type = 'PRESIDENT_REPUBLIQUE'`,
		min:   10,
	},
	{
		// Une frise sans contexte n'est qu'une liste de dates. Si ce chiffre
		// tombe à zéro, c'est que les présidences ou les mandats ont disparu.
		name:  "les mandats se replacent sous une présidence",
		query: `SELECT count(*) FROM core.mandat_contexte`,
		min:   1000,
	},
	{
		// Deux présidences ne peuvent pas se chevaucher : la passation a lieu
		// un jour donné. Un chevauchement signalerait une erreur de saisie
		// dans data/presidents.csv, que rien d'autre n'attraperait.
		name: "aucune présidence n'en chevauche une autre",
		query: `SELECT count(*) FROM core.mandate a JOIN core.mandate b
		          ON a.id < b.id AND a.validity && b.validity
		         WHERE a.mandate_type = 'PRESIDENT_REPUBLIQUE'
		           AND b.mandate_type = 'PRESIDENT_REPUBLIQUE'`,
	},
	{
		name:  "les intercommunalités sont chargées",
		query: `SELECT count(*) FROM core.epci`,
		min:   5000,
	},
	{
		name:  "les compétences transférées sont chargées",
		query: `SELECT count(*) FROM core.epci_competence`,
		min:   50000,
	},
	{
		// Sans adhésions, la vue commune_competence est vide et aucun budget
		// communal ne peut être qualifié.
		name:  "les communes sont rattachées à leurs groupements",
		query: `SELECT count(DISTINCT commune_code) FROM core.epci_membre`,
		min:   30000,
	},
	{
		name: "toute compétence exercée existe dans la nomenclature",
		query: `SELECT count(*) FROM core.epci_competence c
		         WHERE NOT EXISTS (SELECT 1 FROM ref.competence r WHERE r.code = c.competence_code)`,
	},
	{
		// Contrôle né d'une destruction réelle : un DELETE non borné dans le
		// connecteur de l'Assemblée a emporté les dossiers du Sénat et les
		// 17 660 thèmes qui s'y rattachaient. Rien ne l'avait signalé.
		name:  "les dossiers du Sénat survivent à une renormalisation de l'Assemblée",
		query: `SELECT count(*) FROM core.dossier WHERE institution = 'SENAT'`,
		min:   8000,
	},
	{
		name:  "les appartenances aux organes de l'Assemblée sont normalisées",
		query: `SELECT count(*) FROM core.affiliation`,
		min:   25000,
	},
	{
		// La profondeur historique est l'apport de ces appartenances : le reste
		// du jeu de l'Assemblée ne couvre que la législature en cours. Le seuil
		// est calé sur ce qui existe vraiment — 2 431 appartenances antérieures
		// à 2017 — et non sur ce qu'on aurait aimé trouver. Un premier seuil
		// posé au jugé (« 1 000 avant 2000 ») a échoué et a bien fait : il ne
		// restait que 32 lignes avant 2010.
		name:  "les appartenances débordent la législature en cours",
		query: `SELECT count(*) FROM core.affiliation WHERE lower(validity) < '2017-01-01'`,
		min:   2000,
	},
	{
		name:  "les grandes séries nationales sont chargées",
		query: `SELECT count(DISTINCT serie_code) FROM core.macro_value`,
		min:   18,
	},
	{
		name:  "les séries nationales couvrent au moins vingt ans",
		query: `SELECT max(annee) - min(annee) FROM core.macro_value`,
		min:   20,
	},
	{
		// Une série sans définition est un chiffre sans signification : « le
		// chômage » veut dire trois choses selon la source.
		name:  "toute série nationale porte sa définition",
		query: `SELECT count(*) FROM ref.macro_serie WHERE coalesce(definition,'') = ''`,
	},
	{
		name:  "les déclarations HATVP sont chargées",
		query: `SELECT count(*) FROM core.declaration`,
		min:   5000,
	},
	{
		name:  "les présidents d'intercommunalité sont connus",
		query: `SELECT count(*) FROM core.epci WHERE president_nom IS NOT NULL`,
		min:   5000,
	},
	{
		// Contrôle de COUVERTURE et non d'anomalie : 59 personnes n'ont pas de
		// date de naissance publiée — 31 à l'Assemblée, 13 au Parlement
		// européen, 15 présidents créés depuis le fichier éditorial. C'est une
		// lacune connue et documentée, pas un défaut de chargement, et elle ne
		// doit pas bloquer la publication. Ce qui doit la bloquer, c'est une
		// chute de la couverture.
		name:  "les dates de naissance couvrent la quasi-totalité des personnes",
		query: `SELECT count(*) FROM core.person WHERE birth_date IS NOT NULL`,
		min:   500000,
	},
	{
		// Une personne décédée ne doit pas vieillir. Le contrôle vérifie que
		// l'âge calculé s'arrête bien à la date du décès.
		name: "aucune personne décédée ne continue de vieillir",
		query: `SELECT count(*) FROM core.personne_age a
		         WHERE a.decedee
		           AND a.age <> extract(year FROM age(a.death_date, a.birth_date))::integer`,
	},
	{
		name:  "les comptes déposés des sociétés cotées sont chargés",
		query: `SELECT count(*) FROM core.entreprise_compte`,
		min:   300,
	},
	{
		// Le rattachement d'une marque à un SIREN a échoué trois fois en
		// automatique. Chaque ligne doit porter la preuve qui a servi à la
		// retenir, sinon personne ne peut la contester.
		name:  "toute société porte la vérification de son SIREN",
		query: `SELECT count(*) FROM core.entreprise WHERE coalesce(verification,'') = ''`,
	},
	{
		// La sonde générique de complétude d'ingestion.
		//
		// Convention : un connecteur qui lit une entrée bornée enregistre
		// `lignes_recues`, `lignes_chargees`, et un compteur `rejet_*` par
		// motif de rejet. La somme doit refermer exactement. Une ligne qui
		// n'est ni chargée ni rejetée sous un motif nommé s'est évaporée.
		//
		// Un simple ratio ne suffirait pas : le marqueur « NA » de la DREES
		// faisait disparaître 5 % des lignes, ce qu'un seuil de tolérance
		// aurait laissé passer. Ici la seule valeur admise est zéro.
		name: "aucune ligne ne disparaît entre la source et la base",
		// Seul le DERNIER chargement réussi de chaque source est jugé. Un run
		// ancien, défectueux puis corrigé, reste dans raw.fetch_run — c'est
		// voulu, l'archive ne se réécrit pas — mais il ne doit pas bloquer la
		// publication à perpétuité. Ce qui compte est l'état présent des données.
		query: `SELECT count(*) FROM (
		          SELECT DISTINCT ON (r.source_id) r.stats
		            FROM raw.fetch_run r
		           WHERE r.status = 'SUCCESS' AND r.stats ? 'lignes_recues'
		           ORDER BY r.source_id, r.started_at DESC) d
		         WHERE (d.stats->>'lignes_recues')::bigint <> (
		                 (d.stats->>'lignes_chargees')::bigint
		               + coalesce((SELECT sum(value::bigint) FROM jsonb_each_text(d.stats)
		                            WHERE key LIKE 'rejet\_%'), 0))`,
	},
	{
		// Toute commune appartient à exactement un EPCI à fiscalité propre —
		// sauf les quatre îles mono-communales que la loi dispense d'adhérer
		// (Île-de-Bréhat, Île-de-Sein, Ouessant, L'Île-d'Yeu).
		//
		// Cette sonde aurait attrapé la perte de 97 communes : la correspondance
		// SIREN → INSEE ne lisait que l'exercice 2025, incomplet en septembre
		// 2026, et les communes sans compte publié quittaient leur
		// intercommunalité sans erreur. Sur une carte, ç'aurait été 97 trous
		// blancs présentés comme des communes isolées.
		name: "toute commune est dans un seul EPCI à fiscalité propre, sauf les quatre îles exemptées",
		query: `SELECT count(*) FROM ref.commune c
		         WHERE c.cog_millesime = (SELECT max(cog_millesime) FROM ref.commune)
		           AND c.code_insee NOT IN ('22016','29083','29155','85113')
		           AND (SELECT count(*) FROM core.epci_membre m
		                  JOIN core.epci e ON e.siren = m.epci_siren
		                 WHERE m.commune_code = c.code_insee
		                   AND e.nature_juridique IN ('CC','CA','CU','METRO','MET69')) <> 1`,
	},
	{
		// Chaque commune du COG a son contour, pour chaque millésime chargé. Un
		// trou ici, c'est une commune qui disparaît de la carte sans que rien ne
		// le signale. L'inverse est admis pour deux communes : Saint-Pierre et
		// Miquelon-Langlade, que l'IGN dessine et que le COG range parmi les
		// collectivités d'outre-mer, hors du fichier des communes.
		name: "toute commune du COG a son contour IGN, à chaque millésime",
		query: `SELECT count(*) FROM ref.commune c
		         WHERE c.cog_millesime IN (SELECT DISTINCT cog_millesime FROM geo.contour_cog)
		           AND NOT EXISTS (SELECT 1 FROM geo.contour_cog g
		                            WHERE g.niveau = 'COMMUNE' AND g.code = c.code_insee
		                              AND g.cog_millesime = c.cog_millesime)`,
	},
	{
		name: "aucun contour IGN hors COG, sauf Saint-Pierre et Miquelon-Langlade",
		query: `SELECT count(*) FROM geo.contour_cog g
		         WHERE g.niveau = 'COMMUNE' AND g.code NOT IN ('97501','97502')
		           AND NOT EXISTS (SELECT 1 FROM ref.commune c
		                            WHERE c.code_insee = g.code AND c.cog_millesime = g.cog_millesime)`,
	},
	{
		// Pour le millésime courant, les contours intercommunaux suivent
		// BANATIC : chaque EPCI à fiscalité propre a le sien. Un EPCI sans
		// contour, c'est un SIREN dont aucune commune membre n'a été dessinée.
		name: "tout EPCI à fiscalité propre et tout EPT du millésime courant a son contour",
		query: `SELECT count(*) FROM core.epci e
		         WHERE e.nature_juridique IN ('CC','CA','CU','METRO','MET69','EPT')
		           AND EXISTS (SELECT 1 FROM core.epci_membre m WHERE m.epci_siren = e.siren
		                          AND m.cog_millesime = (SELECT max(cog_millesime) FROM ref.commune))
		           AND NOT EXISTS (SELECT 1 FROM geo.contour_cog g
		                            WHERE g.niveau IN ('EPCI','EPT') AND g.code = e.siren
		                              AND g.cog_millesime = (SELECT max(cog_millesime) FROM ref.commune))`,
	},
	{
		// L'IGN publie l'appartenance de chaque commune à son intercommunalité,
		// mais sa couche du millésime courant retarde sur les changements du
		// 1er janvier : en 2026, 45 communes — les fusions de Lévézou (19) et de
		// Thionville Fensch Agglomération (23), et trois communes passées d'un
		// EPCI à un autre. C'est pourquoi le millésime courant se dessine
		// d'après BANATIC. Le plafond borne le retard admis : s'il grandit,
		// l'une des deux sources a changé de nature et il faut savoir laquelle.
		name: "l'IGN et BANATIC s'accordent sur l'EPCI de chaque commune, au retard de l'IGN près",
		query: `SELECT greatest(count(*) - 45, 0) FROM geo.contour_cog c
		         WHERE c.niveau = 'COMMUNE'
		           AND c.cog_millesime = (SELECT max(cog_millesime) FROM ref.commune)
		           AND coalesce((SELECT array_agg(m.epci_siren ORDER BY m.epci_siren)
		                           FROM core.epci_membre m JOIN core.epci e ON e.siren = m.epci_siren
		                          WHERE m.commune_code = c.code AND m.cog_millesime = c.cog_millesime
		                            AND e.nature_juridique IN ('CC','CA','CU','METRO','MET69')), '{}')
		               <> coalesce((SELECT array_agg(s ORDER BY s) FROM unnest(c.codes_siren_epci) s
		                             WHERE s NOT IN (SELECT siren FROM core.epci WHERE nature_juridique = 'EPT')), '{}')`,
	},
	{
		// Le total « ensemble », publié par la Drees, est une moyenne PONDÉRÉE
		// par le nombre de ménages de chaque type — pas leur moyenne arithmétique,
		// les types n'ayant pas le même poids (12 millions de personnes seules,
		// 335 000 couples de quatre enfants ou plus). Repondérer les neuf types
		// retenus par leurs effectifs (core.menage_type_effectif) doit redonner ce
		// total à peu près : 10 % de marge couvrent la couverture incomplète
		// (97 % des ménages, cf. le commentaire de core.menage_type_effectif) et
		// l'écart d'univers entre le recensement et l'enquête ERFS. Un écart plus
		// grand dirait qu'une colonne a été mal reconnue — décalée, feuille
		// modifiée — bien avant que quiconque ne le remarque dans un total agrégé.
		name: "le revenu initial pondéré par effectif recompose le total Drees, à 10 % près",
		query: `SELECT count(*) FROM (
		          SELECT 1
		            FROM (SELECT sum(e.nb_menages * d.revenu_initial_menage) / sum(e.nb_menages) AS pondere
		                    FROM core.menage_type_drees d
		                    JOIN core.menage_type_effectif e ON e.type_menage = d.type_menage AND e.annee = d.annee
		                   WHERE d.type_menage <> 'ensemble') p
		            CROSS JOIN (SELECT revenu_initial_menage AS ensemble
		                          FROM core.menage_type_drees WHERE type_menage = 'ensemble') e
		           WHERE abs(p.pondere - e.ensemble) / e.ensemble > 0.10
		        ) t`,
	},
	{
		// Chaque type de ménage simulé doit avoir son pendant en effectif, sinon
		// la vue derived.socle_universel_simulation les exclut en silence (jointure
		// interne) et le total national sous-compte sans qu'aucune requête ne le
		// signale. Exception assumée : « complexe avec enfants » (ménages
		// multifamiliaux ou avec un tiers hors famille), que la nomenclature Insee
		// utilisée par internal/macro/menages_effectif.go ne dénombre pas
		// séparément — un résidu documenté, pas un oubli.
		name: "chaque type de ménage de la simulation du socle a un effectif",
		query: `SELECT count(*) FROM core.menage_type_drees d
		         WHERE d.type_menage NOT IN ('ensemble', 'complexe_avec_enfants')
		           AND NOT EXISTS (SELECT 1 FROM core.menage_type_effectif e
		                            WHERE e.type_menage = d.type_menage AND e.annee = d.annee)`,
	},
	{
		// Les neuf déciles nationaux du niveau de vie (docs/revenu-universel-
		// microsimulation.md § 6) doivent être strictement croissants : un
		// décile qui redescend dirait qu'une mesure a été lue à la mauvaise
		// ligne du classeur Filosofi.
		name: "les déciles nationaux du niveau de vie sont strictement croissants",
		query: `SELECT count(*) FROM (
		          SELECT annee, decile, niveau_vie_mensuel,
		                 lag(niveau_vie_mensuel) OVER (PARTITION BY annee ORDER BY decile) AS precedent
		            FROM core.filosofi_decile_national
		        ) x WHERE precedent IS NOT NULL AND niveau_vie_mensuel <= precedent`,
	},
	{
		// La reprise de la version 2 (method_version socle-uc-v2-reprise-
		// mediane-plancher) ne doit jamais faire perdre un ménage net : c'est
		// la propriété que le plancher est censé garantir structurellement.
		name:  "la version 2 de la reprise du socle ne fait jamais perdre un ménage net",
		query: `SELECT count(*) FROM derived.socle_universel_simulation WHERE delta_mensuel_menage < 0`,
	},
	{
		// Immigré + non-immigré, et étranger + français, doivent chacun
		// reconstituer le total publié — sinon une des deux catégories a été
		// mal reconnue (docs/immigration-donnees.md).
		name: "population immigrée + non-immigrée reconstitue le total, à chaque croisement d'âge, sexe et emploi",
		query: `SELECT count(*) FROM (
		          SELECT annee, sexe, age_tranche, statut_emploi,
		                 sum(population) FILTER (WHERE categorie IN ('IMMIGRE','NON_IMMIGRE')) AS somme,
		                 max(population) FILTER (WHERE categorie = 'TOTAL') AS total
		            FROM core.population_statut_migratoire WHERE classification = 'IMMIGRATION'
		           GROUP BY 1,2,3,4
		        ) x WHERE total IS NOT NULL AND abs(somme - total) > 1`,
	},
	{
		name: "population étrangère + française reconstitue le total, à chaque croisement d'âge, sexe et emploi",
		query: `SELECT count(*) FROM (
		          SELECT annee, sexe, age_tranche, statut_emploi,
		                 sum(population) FILTER (WHERE categorie IN ('ETRANGER','FRANCAIS')) AS somme,
		                 max(population) FILTER (WHERE categorie = 'TOTAL') AS total
		            FROM core.population_statut_migratoire WHERE classification = 'NATIONALITE'
		           GROUP BY 1,2,3,4
		        ) x WHERE total IS NOT NULL AND abs(somme - total) > 1`,
	},
	{
		name: "les pays de naissance des immigrés se somment au total, par année, sexe et âge",
		query: `SELECT count(*) FROM (
		          SELECT annee, sexe, age_tranche,
		                 sum(population) FILTER (WHERE pays_code <> '_T') AS somme,
		                 max(population) FILTER (WHERE pays_code = '_T') AS total
		            FROM core.population_immigree_origine
		           GROUP BY 1,2,3
		        ) x WHERE total IS NOT NULL AND abs(somme - total) > 1`,
	},
	{
		// Eurostat ne publie la décomposition national/UE27/hors UE27 de
		// façon complète qu'à partir de 2015 pour la France — avant, seul le
		// total est fiable. Le contrôle se limite donc à 2015 et après ; ce
		// n'est pas une tolérance de confort, c'est ce que la source permet.
		name: "national + UE27 + hors UE27 reconstitue le total Eurostat, par dimension et par année depuis 2015",
		query: `SELECT count(*) FROM (
		          SELECT dimension, annee,
		                 sum(population) FILTER (WHERE categorie IN ('NATIONAL','UE27_AUTRE','HORS_UE27')) AS somme,
		                 max(population) FILTER (WHERE categorie = 'TOTAL') AS total
		            FROM core.eurostat_population_migratoire
		           WHERE annee >= 2015
		           GROUP BY 1,2
		        ) x WHERE total IS NOT NULL AND abs(somme - total) > 1`,
	},
	{
		name:  "le stock de titres de séjour couvre au moins dix ans",
		query: `SELECT count(DISTINCT annee) FROM core.titre_sejour_stock`,
		min:   10,
	},
	{
		// Chaque immigré a un pays de naissance connu (34 % ont la
		// nationalité française, mais TOUS ont un pays de naissance) : c'est
		// le fait qui distingue « immigré » d'« étranger » au § 1 de
		// docs/immigration-donnees.md, et il doit rester vrai en base.
		name: "tout immigré recensé a un pays de naissance dans la ventilation par origine",
		query: `SELECT count(*) FROM (
		          SELECT 1 WHERE
		            (SELECT population FROM core.population_statut_migratoire
		              WHERE classification='IMMIGRATION' AND categorie='IMMIGRE'
		                AND age_tranche='TOUS_AGES' AND sexe='TOTAL' AND statut_emploi='TOTAL')
		            <>
		            (SELECT population FROM core.population_immigree_origine
		              WHERE pays_code='_T' AND age_tranche='TOUS_AGES' AND sexe='TOTAL')
		        ) x`,
	},
	{
		// La continuité RMI → RSA (docs/chomage-donnees.md) : le RMI doit
		// être résiduel ou nul à partir de 2010, sans quoi la bascule de
		// juin 2009 a été mal reconnue par le connecteur.
		name: "le RMI est résiduel ou nul à partir de 2010, après la bascule vers le RSA",
		query: `SELECT count(*) FROM core.minima_sociaux_effectif
		         WHERE dispositif_code = 'RMI' AND annee >= 2010 AND effectif > 10000`,
	},
	{
		// L'« Ensemble » publié par la Drees doit rester supérieur ou très
		// proche de la somme des dispositifs qu'elle recense (elle compte des
		// allocations, pas des allocataires dédupliqués — voir le commentaire
		// de la table). Une marge de 200 couvre les arrondis indépendants de
		// chaque dispositif (observés jusqu'à 100 certaines années) ; un écart
		// plus grand dirait qu'un dispositif a été compté deux fois.
		name: "l'ensemble des minima sociaux n'est jamais inférieur à la somme des dispositifs nommés, à 200 près",
		query: `SELECT count(*) FROM (
		          SELECT annee,
		                 sum(effectif) FILTER (WHERE dispositif_code <> 'ENSEMBLE') AS somme,
		                 max(effectif) FILTER (WHERE dispositif_code = 'ENSEMBLE') AS ensemble
		            FROM core.minima_sociaux_effectif GROUP BY annee
		        ) x WHERE ensemble IS NOT NULL AND somme > ensemble + 200`,
	},
	{
		name:  "les effectifs de minima sociaux couvrent au moins trente ans",
		query: `SELECT count(DISTINCT annee) FROM core.minima_sociaux_effectif`,
		min:   30,
	},
	{
		name:  "la population immigrée et étrangère couvre au moins un siècle",
		query: `SELECT max(annee) - min(annee) FROM core.population_historique_nationalite`,
		min:   100,
	},
	{
		// Français de naissance + par acquisition + étrangers doit
		// reconstituer la population totale, chaque millésime — sinon une
		// colonne du classeur Insee a été mal repérée.
		name: "français de naissance + par acquisition + étrangers reconstitue la population totale, à 1 % près",
		query: `SELECT count(*) FROM core.population_historique_nationalite
		         WHERE abs(francais_naissance_milliers + francais_acquisition_milliers + etrangers_milliers
		                    - population_totale_milliers) > population_totale_milliers * 0.01`,
	},
	{
		name:  "les flux migratoires (immigration et naturalisation) couvrent au moins quinze ans chacun",
		query: `SELECT min(n) FROM (SELECT type_flux, count(*) AS n FROM core.flux_migratoire GROUP BY type_flux) x`,
		min:   15,
	},
	{
		name:  "l'âge de départ à la retraite couvre au moins quinze ans",
		query: `SELECT count(DISTINCT annee) FROM core.age_depart_retraite`,
		min:   15,
	},
	{
		// L'âge de départ conjoncturel n'a, historiquement, jamais dépassé 65
		// ans ni été inférieur à 55 : un chiffre hors de ces bornes dirait une
		// colonne décalée (âge des femmes pris pour celui des hommes, etc.).
		name: "l'âge de départ à la retraite reste entre 55 et 65 ans",
		query: `SELECT count(*) FROM core.age_depart_retraite
		         WHERE age_femmes NOT BETWEEN 55 AND 65 OR age_hommes NOT BETWEEN 55 AND 65`,
	},
	{
		name:  "les demandeurs d'emploi inscrits couvrent au moins vingt ans",
		query: `SELECT count(DISTINCT date_mois) FROM core.demandeur_emploi_categorie`,
		min:   240,
	},
	{
		// La catégorie ABC doit être proche de A+B+C : proche, pas égale,
		// parce que la Dares publie ces quatre séries indépendamment (chacune
		// CVS-CJO séparément), avec un arrondi propre à chacune.
		name: "la catégorie ABC des demandeurs d'emploi est proche de A + B + C, à 1 % près",
		query: `SELECT count(*) FROM (
		          SELECT date_mois, champ,
		                 max(effectif) FILTER (WHERE categorie = 'ABC') AS abc,
		                 sum(effectif) FILTER (WHERE categorie IN ('A','B','C')) AS somme
		            FROM core.demandeur_emploi_categorie
		           WHERE categorie IN ('A','B','C','ABC')
		           GROUP BY date_mois, champ
		        ) x WHERE abc IS NOT NULL AND somme IS NOT NULL
		              AND abs(abc - somme) > abc * 0.01`,
	},
	{
		name:  "la prime d'activité couvre au moins neuf ans",
		query: `SELECT count(DISTINCT annee) FROM core.prime_activite_effectif`,
		min:   9,
	},
	{
		// Le rapport démographique publié par l'Insee doit correspondre au
		// rapport cotisants/retraités qu'on peut recalculer soi-même à partir
		// des deux mêmes colonnes — sinon une colonne a été lue à la mauvaise
		// place.
		name: "le ratio cotisants/retraités correspond à cotisants ÷ retraités, à 1 % près",
		query: `SELECT count(*) FROM core.cotisants_retraites_ratio
		         WHERE abs(ratio_demographique - cotisants_millions / retraites_millions) > 0.02`,
	},
	{
		name:  "le ratio cotisants/retraités couvre au moins quinze ans",
		query: `SELECT count(DISTINCT annee) FROM core.cotisants_retraites_ratio`,
		min:   15,
	},
	{
		// Les six continents (dont indéterminé) doivent se sommer au total
		// publié par l'Ofpra pour la même année — sinon une ligne « Total »
		// ou « Continent » a été mal reconnue dans le CSV.
		name: "les demandes d'asile par continent se somment au total, chaque année",
		query: `SELECT count(*) FROM (
		          SELECT annee,
		                 sum(premiere_demande) FILTER (WHERE niveau = 'CONTINENT') AS somme,
		                 max(premiere_demande) FILTER (WHERE niveau = 'TOTAL') AS total
		            FROM core.demande_asile_ofpra
		           GROUP BY annee
		        ) x WHERE total IS NOT NULL AND somme IS NOT NULL AND somme <> total`,
	},
	{
		name:  "les demandes d'asile Ofpra couvrent au moins quatre ans",
		query: `SELECT count(DISTINCT annee) FROM core.demande_asile_ofpra`,
		min:   4,
	},
	{
		// Les quantiles d'un taux de remplacement doivent être croissants
		// (q10 <= q25 <= ... <= q90) : c'est la définition même d'un quantile,
		// pas une propriété empirique qui pourrait être fausse.
		name: "les quantiles du taux de remplacement à la retraite sont croissants",
		query: `SELECT count(*) FROM core.taux_remplacement_retraite
		         WHERE taux_q10 > taux_q25 OR taux_q25 > taux_q50
		            OR taux_q50 > taux_q75 OR taux_q75 > taux_q90`,
	},
	{
		name:  "le budget par mission (PLF) couvre au moins deux exercices",
		query: `SELECT count(DISTINCT exercice) FROM core.budget_programme`,
		min:   2,
	},
	{
		// Les quatre programmes de la mission Défense (144, 146, 178, 212)
		// doivent être présents à chaque exercice chargé — un programme absent
		// signalerait un export PLF partiel plutôt qu'un choix éditorial.
		name: "la mission Défense a ses quatre programmes à chaque exercice chargé",
		query: `SELECT count(*) FROM (
		          SELECT exercice, count(DISTINCT programme_libelle) AS n
		            FROM core.budget_programme WHERE mission_libelle = 'Défense'
		           GROUP BY exercice
		        ) x WHERE n <> 4`,
	},
	{
		// Police nationale et Gendarmerie nationale doivent apparaître ensemble
		// à chaque exercice de la mission Sécurités — les deux budgets ne se
		// substituent jamais l'un à l'autre (docs/securite-police-donnees.md).
		name: "la mission Sécurités porte Police nationale et Gendarmerie nationale à chaque exercice",
		query: `SELECT count(*) FROM (
		          SELECT exercice,
		                 bool_or(programme_libelle = 'Police nationale') AS a_police,
		                 bool_or(programme_libelle = 'Gendarmerie nationale') AS a_gendarmerie
		            FROM core.budget_programme WHERE mission_libelle = 'Sécurités'
		           GROUP BY exercice
		        ) x WHERE NOT (a_police AND a_gendarmerie)`,
	},
	{
		// L'Assemblée nationale et le Sénat doivent apparaître ensemble à
		// chaque exercice de la mission Pouvoirs publics — voir
		// docs/pouvoirs-publics-donnees.md.
		name: "la mission Pouvoirs publics porte l'Assemblée nationale et le Sénat à chaque exercice",
		query: `SELECT count(*) FROM (
		          SELECT exercice,
		                 bool_or(programme_libelle = 'Assemblée nationale') AS a_an,
		                 bool_or(programme_libelle = 'Sénat') AS a_senat
		            FROM core.budget_programme WHERE mission_libelle = 'Pouvoirs publics'
		           GROUP BY exercice
		        ) x WHERE NOT (a_an AND a_senat)`,
	},
	{
		name:  "la dépense environnementale couvre au moins dix ans",
		query: `SELECT count(DISTINCT annee) FROM core.depense_environnementale WHERE purpose_code='TOT_CEP_EP' AND secteur_code='S1'`,
		min:   10,
	},
	{
		// Le secteur S1 (total économie) doit toujours être au moins aussi
		// grand que le plus grand de ses sous-secteurs — sinon la hiérarchie
		// purpose/secteur documentée dans core.depense_environnementale a été
		// mal lue (même piège que SAE Q24, voir docs/sante-donnees.md § 1.1).
		name: "le total économie de la dépense environnementale domine chaque sous-secteur",
		query: `SELECT count(*) FROM (
		          SELECT annee,
		                 max(valeur) FILTER (WHERE secteur_code='S1') AS total,
		                 max(valeur) FILTER (WHERE secteur_code<>'S1') AS sous_secteur
		            FROM core.depense_environnementale
		           WHERE purpose_code='TOT_CEP_EP' AND unite='MIO_EUR'
		           GROUP BY annee
		        ) x WHERE total IS NOT NULL AND sous_secteur IS NOT NULL AND sous_secteur > total`,
	},
	{
		name:  "la certification HAS couvre au moins 300 démarches",
		query: `SELECT count(*) FROM core.certification_has_demarche`,
		min:   300,
	},
	{
		// Un score de chapitre est une moyenne sur 100 — hors bornes, une
		// colonne a été mal lue (virgule décimale, ou pourcentage x100 en trop).
		name:  "les scores de certification HAS restent entre 0 et 100",
		query: `SELECT count(*) FROM core.certification_has_chapitre WHERE score IS NOT NULL AND (score < 0 OR score > 100)`,
	},
	{
		name:  "le taux de pauvreté européen couvre la France sur au moins dix ans",
		query: `SELECT count(DISTINCT annee) FROM core.pauvrete_taux_eu WHERE geo_code = 'FR'`,
		min:   10,
	},
	{
		// Un taux de risque de pauvreté est un pourcentage de population :
		// hors de cette fourchette, une colonne a été mal lue.
		name:  "le taux de pauvreté européen reste une proportion plausible",
		query: `SELECT count(*) FROM core.pauvrete_taux_eu WHERE taux_pct <= 0 OR taux_pct > 50`,
	},
	{
		name:  "l'aide alimentaire couvre les six réseaux du dispositif Insee-Drees",
		query: `SELECT count(DISTINCT association) FROM core.aide_alimentaire`,
		min:   6,
	},
	{
		// Chaque réseau doit porter periode_libelle si et seulement s'il est en
		// CAMPAGNE (Restos du Cœur) — sinon le format de période a été mal
		// reconnu à l'ingestion (voir internal/macro/aide_alimentaire.go).
		name: "periode_libelle n'est renseigné que pour les lignes CAMPAGNE de l'aide alimentaire",
		query: `SELECT count(*) FROM core.aide_alimentaire
		          WHERE (periode_type = 'CAMPAGNE') <> (periode_libelle IS NOT NULL)`,
	},
	{
		name:  "le salaire minimum européen couvre la France sur au moins vingt ans",
		query: `SELECT count(DISTINCT semestre) FROM core.salaire_minimum WHERE geo_code = 'FR'`,
		min:   40, // deux semestres par an
	},
	{
		// Un salaire minimum mensuel plausible : au-delà, une colonne a été
		// mal lue (unité horaire prise pour mensuelle, par exemple).
		name:  "le salaire minimum reste dans une fourchette mensuelle plausible",
		query: `SELECT count(*) FROM core.salaire_minimum WHERE unite = 'EUR' AND (valeur <= 0 OR valeur > 5000)`,
	},
	{
		name: "le PIB Banque mondiale couvre les dix pays de comparaison chaque année depuis 2010",
		query: `SELECT count(*) FROM (
		          SELECT annee, count(DISTINCT pays_code) AS n
		            FROM core.indicateur_mondial
		           WHERE indicateur = 'NY.GDP.MKTP.CD' AND annee >= 2010 AND annee <= 2023
		           GROUP BY annee
		        ) x WHERE n < 10`,
	},
	{
		name:  "les personnels du premier degré couvrent au moins deux rentrées scolaires",
		query: `SELECT count(DISTINCT annee) FROM core.education_personnel_etablissement WHERE degre = 'PREMIER'`,
		min:   2,
	},
	{
		name:  "les effectifs d'élèves du premier degré couvrent au moins quinze rentrées scolaires",
		query: `SELECT count(DISTINCT annee) FROM core.education_effectif_eleves`,
		min:   15,
	},
	{
		// Un secteur qui perd des écoles doit aussi perdre des élèves — sinon
		// une colonne a été échangée avec une autre (déjà arrivé cette session
		// sur d'autres connecteurs DEPP/DREES).
		name: "les effectifs d'élèves suivent le nombre d'écoles, secteur par secteur",
		query: `SELECT count(*) FROM (
		          SELECT secteur,
		                 (max(nombre_ecoles) FILTER (WHERE annee = (SELECT max(annee) FROM core.education_effectif_eleves))
		                    < max(nombre_ecoles) FILTER (WHERE annee = (SELECT min(annee) FROM core.education_effectif_eleves))) AS moins_ecoles,
		                 (max(nombre_eleves) FILTER (WHERE annee = (SELECT max(annee) FROM core.education_effectif_eleves))
		                    < max(nombre_eleves) FILTER (WHERE annee = (SELECT min(annee) FROM core.education_effectif_eleves))) AS moins_eleves
		            FROM core.education_effectif_eleves
		           WHERE secteur IN ('PUBLIC','PRIVE SOUS CONTRAT')
		           GROUP BY secteur
		        ) x WHERE moins_ecoles AND NOT moins_eleves`,
	},
	{
		name:  "le RPPS couvre au moins 300 000 médecins distincts",
		query: `SELECT count(DISTINCT identifiant_pp) FROM core.rpps_professionnel_activite WHERE code_profession = '10'`,
		min:   300000,
	},
	{
		// Une ligne sans identifiant de profession serait invisible à toute
		// analyse par profession — le NOT NULL de la colonne le garantit déjà
		// en base, cette sonde vérifie que le connecteur ne l'a pas contourné
		// avec une chaîne vide.
		name:  "toute ligne RPPS porte un code profession non vide",
		query: `SELECT count(*) FROM core.rpps_professionnel_activite WHERE code_profession = ''`,
	},
	{
		name:  "le PMSI-MCO national couvre au moins cinq exercices",
		query: `SELECT count(DISTINCT annee) FROM core.pmsi_mco_national`,
		min:   5,
	},
	{
		// 'Tous' doit rester la somme exacte des deux types d'hospitalisation
		// — vérifié aussi à l'ingestion (internal/sante/pmsi.go), sonde
		// redondante à dessein pour couvrir une donnée chargée hors connecteur.
		name: "le PMSI-MCO national : Tous égale complète plus ambulatoire, chaque année",
		query: `SELECT count(*) FROM (
		          SELECT annee,
		                 max(nb_sejours) FILTER (WHERE typ_hospit = 'Tous') AS tous,
		                 sum(nb_sejours) FILTER (WHERE typ_hospit <> 'Tous') AS somme
		            FROM core.pmsi_mco_national
		           GROUP BY annee
		        ) x WHERE tous <> somme`,
	},
	{
		name:  "le PMSI-MCO par établissement couvre les régions métropolitaines et ultramarines",
		query: `SELECT count(DISTINCT region) FROM core.pmsi_mco_par_etablissement`,
		min:   15,
	},
	{
		// Le total est chez la même ligne 'Tous' (côté patient) : le nombre de
		// séjours qu'elle porte doit dominer chaque région nommée, sinon la
		// dimension a été mal reconnue (même piège que SAE Q24, aide
		// alimentaire, dépense environnementale — déjà rencontré plusieurs
		// fois cette session).
		name: "le PMSI-MCO par patient : la ligne région=Tous domine chaque région nommée",
		query: `SELECT count(*) FROM (
		          SELECT annee,
		                 max(nb_sejours) FILTER (WHERE region = 'Tous' AND age='Tous' AND sexe='Tous') AS tous,
		                 max(nb_sejours) FILTER (WHERE region <> 'Tous' AND age='Tous' AND sexe='Tous') AS max_region
		            FROM core.pmsi_mco_par_patient
		           GROUP BY annee
		        ) x WHERE tous IS NOT NULL AND max_region IS NOT NULL AND max_region > tous`,
	},
	{
		name:  "le PMSI-SMR couvre au moins cinq exercices",
		query: `SELECT count(DISTINCT annee) FROM core.pmsi_smr_regional`,
		min:   5,
	},
	{
		// Vérifié aussi à l'ingestion (internal/sante/pmsi_smr_had.go) ; sonde
		// redondante à dessein, comme pour le MCO. Ignore les (année, région)
		// où seule la ligne 'Tous' est publiée, sans détail HC/HP — une
		// lacune connue de la source à cette maille, pas une anomalie.
		name: "le PMSI-SMR : nb_jours Tous égale HC plus HP quand le détail existe",
		query: `SELECT count(*) FROM (
		          SELECT annee, region,
		                 max(nb_jours) FILTER (WHERE type_hosp = 'Tous') AS tous,
		                 sum(nb_jours) FILTER (WHERE type_hosp <> 'Tous') AS somme,
		                 count(*) FILTER (WHERE type_hosp IN ('HC','HP')) AS nb_detail
		            FROM core.pmsi_smr_regional
		           GROUP BY annee, region
		        ) x WHERE nb_detail = 2 AND tous <> somme`,
	},
	{
		name:  "le PMSI-HAD couvre au moins cinq exercices",
		query: `SELECT count(DISTINCT annee) FROM core.pmsi_had_regional`,
		min:   5,
	},
	{
		name:  "les honoraires des médecins couvrent au moins dix exercices",
		query: `SELECT count(DISTINCT annee) FROM core.medecin_honoraires`,
		min:   10,
	},
	{
		// 25 à 35 Md€ d'honoraires « Ensemble des médecins » au total national
		// est l'ordre de grandeur connu (Cnam, comptes de la santé) — un
		// exercice hors de cette fourchette signalerait un NS mal décodé (en
		// 0 plutôt qu'en NULL) ou une confusion d'agrégat (code_departement
		// '999' compté en double, voir le commentaire de la migration 0109).
		name: "le total national des honoraires médecins reste dans un ordre de grandeur plausible",
		query: `SELECT count(*) FROM (
		          SELECT annee, hono_sans_depassement_total + depassements_total AS total
		            FROM core.medecin_honoraires
		           WHERE code_region = '99' AND profession_sante = 'Ensemble des médecins'
		        ) x WHERE total NOT BETWEEN 20e9 AND 40e9`,
	},
	{
		name:  "les DECP couvrent au moins deux millions de lignes de marché",
		query: `SELECT count(*) FROM core.public_contract`,
		min:   2000000,
	},
	{
		// source_uid combine marché, titulaire et modification précisément
		// pour rester unique malgré les co-titulaires (jusqu'à 87 observés
		// sur un même marché) — si un doublon apparaît, le dédoublonnage de
		// internal/decp/decp.go a régressé.
		name:  "les DECP n'ont aucun doublon de source_uid",
		query: `SELECT count(*) - count(DISTINCT source_uid) FROM core.public_contract`,
	},
	{
		// Une petite fraction de dates de notification est aberrante dans la
		// source elle-même (des dates de l'an 1 à 3, ~0,01 % des lignes) —
		// connu et documenté (docs/sante-donnees.md et le commentaire de la
		// table), cette sonde s'assure seulement que cette fraction reste
		// marginale plutôt que de devenir majoritaire sans qu'on s'en aperçoive.
		name:  "les DECP : moins de 0,1 % de dates de notification antérieures à 2000",
		query: `SELECT (count(*) FILTER (WHERE date_notification < '2000-01-01') > count(*) / 1000) ::int FROM core.public_contract`,
	},
	{
		// Le total d'ETP d'un établissement ne peut pas être inférieur à ses
		// seuls enseignants — sinon une colonne a été lue à la mauvaise place
		// (déjà arrivé cette session sur d'autres connecteurs DEPP/DREES).
		name: "aucun établissement n'a moins d'enseignants que d'ETP total (second degré)",
		query: `SELECT count(*) FROM core.education_personnel_etablissement
		         WHERE degre = 'SECOND' AND etp_total IS NOT NULL AND etp_enseignants IS NOT NULL
		           AND etp_total < etp_enseignants - 0.5`,
	},
	{
		name:  "le référentiel FINESS couvre au moins 100 000 établissements",
		query: `SELECT count(*) FROM ref.finess_etablissement`,
		min:   100000,
	},
	{
		// nofinesset suit le motif [0-9][0-9A-Z][0-9]{7} de la spécification
		// etalab_cs1100502 — un identifiant hors motif signale un décalage de
		// colonnes dans le fichier plat sans en-tête.
		name:  "tous les identifiants FINESS suivent le format attendu",
		query: `SELECT count(*) FROM ref.finess_etablissement WHERE nofinesset !~ '^[0-9][0-9A-Z][0-9]{7}$'`,
	},
	{
		name:  "les secteurs conventionnels couvrent au moins dix ans",
		query: `SELECT count(DISTINCT annee) FROM core.medecin_secteur_effectif`,
		min:   10,
	},
	{
		// L'agrégat national (région "FRANCE") doit être présent pour "Ensemble
		// des médecins" à chaque millésime — sinon le calcul du § 3 de
		// docs/sante-donnees.md n'a plus de dénominateur national. Trois
		// secteurs avant 2013 (l'Optam n'existait pas), quatre depuis : moins
		// de trois signalerait une vraie lacune, pas la rupture de série connue.
		name: "l'agrégat national des médecins par secteur est présent à chaque exercice",
		query: `SELECT count(*) FROM (
		          SELECT annee FROM core.medecin_secteur_effectif
		           WHERE profession_sante = 'Ensemble des médecins' AND libelle_region = 'FRANCE'
		          GROUP BY annee HAVING count(DISTINCT secteur_code) < 3
		        ) x`,
	},
	{
		name:  "les sept bassins hydrographiques métropolitains sont chargés",
		query: `SELECT count(*) FROM geo.contour_bassin WHERE territoire = 'metropole'`,
		min:   7,
	},
	{
		name:  "les deux bassins d'outre-mer disponibles (Martinique, Mayotte) sont chargés",
		query: `SELECT count(*) FROM geo.contour_bassin WHERE territoire = 'outremer'`,
		min:   2,
	},
	{
		name:  "tous les contours de bassin sont des géométries valides",
		query: `SELECT count(*) FROM geo.contour_bassin WHERE NOT ST_IsValid(geom)`,
	},
	{
		// La somme des 7 bassins métropolitains doit rester dans l'ordre de
		// grandeur de la superficie de la France métropolitaine
		// (543 940 km²) — un écart large signalerait une reprojection
		// Lambert-93/WGS84 ratée. Filtré à la métropole : les bassins
		// d'outre-mer, avec leur propre SRID, n'ont pas leur place dans
		// cette même vérification (voir la probe dédiée juste après).
		name: "la surface totale des bassins métropolitains reste dans l'ordre de grandeur de la métropole",
		query: `SELECT count(*) FROM (
		          SELECT sum(ST_Area(geom::geography)) / 1e6 AS km2 FROM geo.contour_bassin
		          WHERE territoire = 'metropole'
		        ) x WHERE km2 NOT BETWEEN 450000 AND 650000`,
	},
	{
		// Martinique (≈ 1 128 km²) et Mayotte (≈ 374 km²) réunis : un écart
		// large signalerait la même erreur de reprojection que ci-dessus,
		// mais sur les SRID ultramarins (5490, 4471) cette fois.
		name: "la surface des deux bassins d'outre-mer reste dans un ordre de grandeur plausible",
		query: `SELECT count(*) FROM (
		          SELECT sum(ST_Area(geom::geography)) / 1e6 AS km2 FROM geo.contour_bassin
		          WHERE territoire = 'outremer'
		        ) x WHERE km2 NOT BETWEEN 500 AND 3000`,
	},
	{
		name:  "au moins 15 des grands cours d'eau retenus sont chargés",
		query: `SELECT count(DISTINCT nom) FROM geo.cours_eau`,
		min:   15,
	},
	{
		name:  "tous les tronçons de cours d'eau sont des géométries valides",
		query: `SELECT count(*) FROM geo.cours_eau WHERE NOT ST_IsValid(geom)`,
	},
	{
		// L'emprise des tronçons chargés doit rester dans le rectangle de la
		// métropole (Corse comprise) — une valeur hors de cette plage
		// signalerait une reprojection Lambert-93/WGS84 ratée, comme pour
		// les bassins hydrographiques.
		name:  "l'emprise des cours d'eau reste dans le rectangle de la métropole",
		query: `SELECT count(*) FROM geo.cours_eau WHERE NOT (geom && ST_MakeEnvelope(-5.5, 41, 9.7, 51.5, 4326))`,
	},
	{
		name:  "au moins 5 000 sous-bassins versants topographiques sont chargés",
		query: `SELECT count(*) FROM geo.contour_sous_bassin`,
		min:   5000,
	},
	{
		// Chaque sous-bassin doit se rattacher à l'un des 7 grands bassins
		// déjà chargés : un code orphelin signalerait un décalage entre les
		// deux millésimes Sandre.
		name:  "chaque sous-bassin se rattache à un grand bassin connu",
		query: `SELECT count(*) FROM geo.contour_sous_bassin sb
		        WHERE NOT EXISTS (SELECT 1 FROM geo.contour_bassin b WHERE b.code = sb.code_bassin)`,
	},
	{
		name:  "assainissement : au moins 5 000 communes chargées (collectif + non collectif)",
		query: `SELECT count(*) FROM core.service_assainissement`,
		min:   5000,
	},
	{
		name:  "le personnel SAE couvre au moins 3 000 établissements",
		query: `SELECT count(*) FROM core.sae_personnel_fonction`,
		min:   3000,
	},
	{
		// Un établissement ne doit apparaître qu'une fois : le fichier source
		// publie une ligne par discipline PLUS un total (discipline 9999) —
		// seul ce total est chargé (internal/sante/sae.go). Un doublon
		// signalerait qu'une ligne de détail s'est glissée à côté du total.
		name:  "chaque établissement SAE n'apparaît qu'une fois",
		query: `SELECT count(*) FROM (SELECT nofinesset FROM core.sae_personnel_fonction GROUP BY 1 HAVING count(*) > 1) x`,
	},
	{
		// Le total national de personnel non médical hospitalier est de
		// l'ordre du million — un multiple de ce nombre signalerait le retour
		// du bug de double comptage par discipline déjà rencontré une fois.
		name: "le total national de personnel SAE reste dans un ordre de grandeur plausible",
		query: `SELECT count(*) FROM (SELECT sum(etp_total_pnm) AS t FROM core.sae_personnel_fonction) x
		         WHERE t NOT BETWEEN 700000 AND 1500000`,
	},
	{
		// Une tranche de pension ou de chômage manquante décale silencieusement
		// tout calcul de reprise fiscale fondé sur la distribution plutôt que sur
		// la moyenne (docs/revenu-universel-microsimulation.md §3).
		name: "les tranches de pension EIR totalisent 100 %, à 1 point près",
		query: `SELECT count(*) FROM (
		          SELECT annee FROM core.pension_tranche_eir
		          GROUP BY annee HAVING abs(sum(pct_ensemble) - 100) > 1
		        ) t`,
	},
	{
		name: "les tranches d'indemnisation chômage totalisent 100 %, à 1 point près, chaque trimestre",
		query: `SELECT count(*) FROM (
		          SELECT date_reference FROM core.chomage_tranche_unedic
		          GROUP BY date_reference HAVING abs(sum(pct) - 100) > 1
		        ) t`,
	},
	{
		name:  "les onze proclamations présidentielles sont chargées",
		query: `SELECT count(*) FROM core.pdr_resultat`,
		min:   11,
	},
	{
		// Le contrôle qui compte : la somme des voix des candidats doit redonner
		// EXACTEMENT les suffrages exprimés proclamés. Une extraction qui lit un
		// chiffre de travers passe toutes les contraintes de colonne et échoue
		// ici — c'est la seule barrière entre une coquille de parsing et une
		// citation fausse sur le site.
		name: "la somme des voix redonne les suffrages exprimés proclamés",
		query: `SELECT count(*) FROM core.pdr_resultat r
		         WHERE r.exprimes <> (SELECT coalesce(sum(v.voix), 0) FROM core.pdr_voix v
		                               WHERE v.annee = r.annee AND v.tour = r.tour)`,
	},
	{
		// La concordance vaut dans les deux sens : elle authentifie notre
		// extraction, et elle qualifie la source tierce. Le jour où l'écart
		// n'est plus nul, c'est l'un des deux qui a bougé, et il faut savoir
		// lequel avant de publier quoi que ce soit.
		name: "les inscrits compilés par International IDEA concordent avec le Conseil constitutionnel",
		query: `SELECT count(*) FROM core.pdr_resultat r
		         JOIN ref.pdr_proclamation p ON p.annee = r.annee AND p.tour = r.tour
		         JOIN core.turnout_election t
		           ON t.pays_iso3 = 'FRA' AND t.type_scrutin = 'Presidential'
		          AND t.date_scrutin = p.date_scrutin
		        WHERE t.inscrits IS DISTINCT FROM r.inscrits`,
	},
	{
		name:  "le corps électoral est calculable pour chaque tour proclamé",
		query: `SELECT count(*) FROM derived.pdr_corps_electoral`,
		min:   11,
	},
	{
		// Un corps électoral plus petit que le nombre d'inscrits serait absurde :
		// on ne peut pas inscrire plus de gens qu'il n'y en a en âge de voter.
		// Le contraire est normal — les non-inscrits existent.
		name: "aucun scrutin ne compte plus d'inscrits que de personnes en âge de voter",
		query: `SELECT count(*) FROM core.pdr_resultat r
		         JOIN derived.pdr_corps_electoral c USING (annee, tour)
		        WHERE r.inscrits > c.population_en_age_de_voter`,
	},
	{
		name:  "la participation comparée est chargée",
		query: `SELECT count(*) FROM core.turnout_election`,
		min:   3000,
	},
	{
		name:  "la délinquance enregistrée est chargée",
		query: `SELECT count(*) FROM core.commune_delinquance`,
		min:   4000000,
	},
	{
		// Le secret statistique doit rester distinguable d'un zéro. S'il n'y
		// avait plus aucune ligne non diffusée, c'est qu'on l'aurait écrasé.
		name:  "le secret statistique du SSMSI est préservé",
		query: `SELECT count(*) FROM core.commune_delinquance WHERE NOT diffuse`,
		min:   100000,
	},
	{
		name: "aucune ligne non diffusée ne porte de nombre",
		query: `SELECT count(*) FROM core.commune_delinquance
		         WHERE NOT diffuse AND nombre IS NOT NULL`,
	},
	{
		// Sans les deux scrutins, aucune comparaison de mandature n'est
		// possible : c'est ce que D-036 a débloqué.
		name:  "les deux mandatures municipales sont chargées",
		query: `SELECT count(DISTINCT scrutin_annee) FROM derived.commune_couleur`,
		min:   2,
	},
	{
		name: "des communes sont nuancées aux deux scrutins",
		query: `SELECT count(*) FROM derived.commune_couleur a
		        JOIN derived.commune_couleur b ON b.commune_code = a.commune_code
		         AND b.scrutin_annee = 2026 AND b.nuance_code IS NOT NULL
		        WHERE a.scrutin_annee = 2020 AND a.nuance_code IS NOT NULL`,
		min: 1500,
	},
	{
		// Une observation ne peut être rattachée qu'à UNE municipalité : celle
		// en vigueur cette année-là. Deux lignes pour un même triplet
		// signaleraient que la vue a cessé de trancher.
		name: "chaque observation de sécurité n'a qu'une municipalité",
		query: `SELECT count(*) FROM (
		          SELECT commune_code, annee, indicateur_code
		            FROM derived.commune_securite
		           GROUP BY 1,2,3 HAVING count(*) > 1) x`,
	},
	{
		name: "les séries du compte des sociétés sont chargées",
		query: `SELECT count(DISTINCT serie_code) FROM core.macro_value
		         WHERE serie_code IN ('dividendes.verses.snf','impots.revenu.payes.snf',
		                              'remuneration.salaries.snf','ebe.snf','valeur.ajoutee.snf')`,
		min: 5,
	},
	{
		// Ces séries remontent à 1971, soit vingt-quatre ans avant les finances
		// publiques. C'est leur intérêt principal.
		name: "le compte des sociétés remonte avant 1980",
		query: `SELECT count(*) FROM core.macro_value
		         WHERE serie_code = 'dividendes.verses.snf' AND annee < 1980`,
		min: 8,
	},
	{
		// Un sénateur qui est aussi conseiller municipal ne doit pas exister en
		// deux fiches (D-039). La fusion est faite sur nom + date de naissance
		// exacte, avec appariement unique des deux côtés : ce contrôle vérifie
		// qu'il ne reste aucune paire de ce genre à réunir.
		name: "aucun sénateur en double avec une personne déjà connue",
		query: `SELECT count(*)
		          FROM core.person s JOIN core.person o
		            ON core.f_unaccent(lower(s.family_name)) = core.f_unaccent(lower(o.family_name))
		           AND core.f_unaccent(lower(s.given_name))  = core.f_unaccent(lower(o.given_name))
		           AND s.birth_date = o.birth_date AND s.id <> o.id
		         WHERE s.birth_date IS NOT NULL
		           AND EXISTS (SELECT 1 FROM core.person_identifier i
		                        WHERE i.person_id = s.id AND i.scheme = 'SENAT_MATRICULE')
		           AND NOT EXISTS (SELECT 1 FROM core.person_identifier i
		                            WHERE i.person_id = o.id AND i.scheme = 'SENAT_MATRICULE')`,
	},
	{
		// 539 310 heures de parole en deux ans avaient été publiées parce que
		// `stime` avait été lu comme une durée (D-038). Une session de l'Assemblée
		// tient dans l'année : 3 000 heures de débat est déjà généreux.
		name: "le temps de parole tient dans une année de séance",
		query: `SELECT count(*) FROM derived.temps_de_parole
		         WHERE duree_debat_s > 3000 * 3600`,
	},
	{
		// L'Assemblée publie 1 258 mandats de député depuis 1988 ; la contrainte
		// d'exclusion en refusait 1 194 à cause d'un chevauchement de deux jours
		// entre législatures (D-041). Le seuil est bas à dessein : il tombe si
		// le recollement cesse de fonctionner, pas si la source évolue.
		name: "les mandats de député remontent avant 2017",
		query: `SELECT count(*) FROM core.mandate
		         WHERE mandate_type = 'DEPUTE' AND institution = 'ASSEMBLEE_NATIONALE'
		           AND lower(validity) < '2017-01-01'`,
		min: 200,
	},
	{
		// Un acte du Journal officiel sans corps de texte ne peut rien nommer, et
		// le décodeur en a déjà perdu la trace deux fois : rejet du fichier
		// entier par le décodeur strict (D-040), puis chemin figé vers
		// <BLOC_TEXTUEL> qui vidait deux cents décrets sur deux cent quatre
		// (D-049).
		//
		// Ce contrôle a d'abord exigé ZÉRO acte sans corps, puis moins de 5 %.
		// Les deux formulations étaient mauvaises : la première affirmait plus
		// que la source ne promet, la seconde est un pourcentage qui dérive avec
		// la composition du corpus — il est passé à 5,07 % en ajoutant les
		// décrets anciens, et le contrôle a échoué sans que rien ne soit cassé.
		//
		// Le fait réel est daté, pas statistique : la DILA ne publie que les
		// MÉTADONNÉES des actes anciens, et son dernier acte nominatif sans
		// texte date du 4 juin 1997. Au-delà de 1998, un acte sans corps est
		// nécessairement un défaut de lecture — et une régression du décodeur
		// ferait tomber ce contrôle sur les 934 actes concernés, pas sur trois.
		name: "tout acte nominatif postérieur à 1998 porte un corps de texte",
		query: `SELECT count(*) FROM core.acte_jo
		         WHERE nominatif AND contenu IS NULL
		           AND coalesce(date_texte, date_publi) >= '1998-01-01'`,
	},

	// ----------------------------------------------------------------------
	// Budget de l'État et de la Sécurité sociale.
	{
		// Une sonde de concordance passe trivialement sur une table vide : c'est
		// déjà arrivé dans ce dépôt. On compte donc d'abord, on rapproche ensuite.
		// Douze séries — quatre sous-secteurs × dépenses, recettes, solde — dont
		// chacune porte trente et un millésimes depuis 1995.
		name:  "les douze séries par sous-secteur sont chargées",
		query: `SELECT count(*) FROM ref.macro_serie WHERE code ~ '^(depense|recette|solde)\.S13'`,
		min:   12,
	},
	{
		name: "chaque série par sous-secteur a au moins vingt-cinq points",
		query: `SELECT count(*) FROM (
		          SELECT serie_code FROM core.macro_value
		           WHERE serie_code ~ '^(depense|recette|solde)\.S13'
		           GROUP BY 1 HAVING count(*) < 25) x`,
	},
	{
		// ESSPROS remonte à 1990, quatre ans plus tôt que les comptes des
		// administrations publiques. C'est son intérêt : dater le basculement
		// des cotisations vers l'impôt affecté plutôt que de l'affirmer.
		name: "les cinq séries de financement de la protection sociale ont au moins trente points",
		query: `SELECT count(*) FROM (
		          SELECT serie_code FROM core.macro_value
		           WHERE serie_code LIKE 'protection.financement.%'
		           GROUP BY 1 HAVING count(*) < 30) x`,
	},
	{
		// L'identité comptable du SEC 2010 : recettes moins dépenses égale la
		// capacité de financement. Si elle cesse d'être vraie, c'est que trois
		// séries indépendantes ne décrivent plus le même sous-secteur — une
		// dimension a changé de nom chez Eurostat et la requête ramène autre
		// chose sans le dire. La tolérance d'un million d'euros couvre les
		// arrondis de publication ; l'écart observé est de 0,1 M€.
		name: "TE − TR = −B9 pour chaque sous-secteur et chaque année",
		query: `SELECT count(*) FROM derived.budget_sous_secteur
		         WHERE depenses_meur IS NOT NULL AND recettes_meur IS NOT NULL
		           AND solde_meur IS NOT NULL
		           AND abs(depenses_meur - recettes_meur + solde_meur) > 1`,
	},
	{
		// La somme des sous-secteurs dépasse toujours le total : les transferts
		// entre administrations sont comptés une fois chez celle qui verse et
		// une fois chez celle qui reçoit, et le total S13 les consolide. L'écart
		// n'est donc PAS nul par construction — il vaut de 6,2 % à 11,0 % sur
		// 1995-2025, et c'est la mesure des flux internes. La borne de 15 %
		// attrape la disparition d'un sous-secteur, pas la consolidation.
		name: "la somme des sous-secteurs reste à moins de 15 % du total consolidé",
		query: `SELECT count(*) FROM (
		          SELECT annee,
		                 max(depenses_meur) FILTER (WHERE secteur = 'S13')    AS total,
		                 sum(depenses_meur) FILTER (WHERE secteur <> 'S13')   AS somme
		            FROM derived.budget_sous_secteur GROUP BY 1) s
		         WHERE total IS NOT NULL AND somme IS NOT NULL
		           AND (somme - total) / total NOT BETWEEN 0 AND 0.15`,
	},
	{
		name:  "les comptes de la protection sociale couvrent au moins soixante millésimes",
		query: `SELECT count(DISTINCT annee) FROM core.protection_sociale`,
		min:   60,
	},
	{
		// La table croise deux hiérarchies. Le sommet de chacune — total des
		// prestations, tous régimes — doit donner EXACTEMENT une ligne par année.
		// Deux lignes signaleraient un dépliage fautif, et une somme naïve sur
		// la table compterait alors chaque euro deux fois de plus.
		name: "le total des prestations est unique chaque année",
		query: `SELECT count(*) FROM (
		          SELECT annee FROM core.protection_sociale
		           WHERE ps_niveau = 0 AND si_niveau = 0
		           GROUP BY 1 HAVING count(*) <> 1) x`,
	},
	{
		// Le secteur institutionnel du financeur est la colonne la plus utile de
		// la table et la plus facile à perdre : c'est elle qui permet de ventiler
		// la protection sociale entre l'État, le régime général et les autres.
		name: "la protection sociale est ventilée par secteur institutionnel",
		query: `SELECT count(DISTINCT si_code) FROM core.protection_sociale
		         WHERE si_code LIKE 'S13%'`,
		min: 5,
	},
	{
		// Le jeu est publié pivoté, une colonne par arrêté. Un dépliage qui
		// perdrait une colonne ne ferait pas d'erreur : il rendrait un mois de
		// moins. On compte donc les arrêtés.
		name:  "l'exécution de l'État porte au moins vingt arrêtés mensuels",
		query: `SELECT count(DISTINCT date_arrete) FROM core.execution_etat`,
		min:   20,
	},
	{
		// Chaque arrêté doit porter les mêmes postes que les autres. Un poste
		// manquant à une date est le symptôme d'une colonne partiellement lue.
		name: "chaque arrêté mensuel porte le même nombre de postes",
		query: `SELECT count(*) FROM (
		          SELECT date_arrete FROM core.execution_etat GROUP BY 1
		           HAVING count(*) <> (SELECT count(DISTINCT (categorie, sous_categorie, ligne))
		                                 FROM core.execution_etat)) x`,
	},
	{
		name:  "les exonérations de cotisations couvrent au moins vingt millésimes",
		query: `SELECT count(DISTINCT annee) FROM core.exoneration_cotisation`,
		min:   20,
	},
	{
		name:  "la masse salariale couvre au moins cent trimestres",
		query: `SELECT count(*) FROM core.masse_salariale`,
		min:   100,
	},
	{
		// Volontairement PAS dans la sonde de fraîcheur ci-dessous : ce jeu est
		// connu comme dormant depuis juillet 2023 (docs/budget-donnees.md § 4.2)
		// et publié comme tel, pas comme une série à jour. Le seul risque à
		// couvrir est une régression du connecteur, pas l'âge de la source.
		name:  "les encaissements URSSAF couvrent les trois millésimes publiés",
		query: `SELECT count(DISTINCT annee) FROM core.encaissement_urssaf`,
		min:   3,
	},
	{
		// La catégorisation « entreprise » (secteur privé hors GEN + GEN) doit
		// rester une SOUS-PARTIE stricte du total encaissé — sinon le filtre
		// double-compte ou classe une catégorie non-entreprise comme entreprise.
		name: "les encaissements « entreprises » ne dépassent jamais le total encaissé, par région et année",
		query: `SELECT count(*) FROM (
		          SELECT annee, organisme,
		                 sum(montant_eur) FILTER (WHERE categorie_entreprise) AS entreprises,
		                 sum(montant_eur) AS total
		            FROM core.encaissement_urssaf
		           GROUP BY annee, organisme
		          HAVING sum(montant_eur) FILTER (WHERE categorie_entreprise) > sum(montant_eur)
		        ) x`,
	},
	{
		// Fraîcheur. C'est le contrôle qui aurait attrapé les encaissements
		// URSSAF, figés depuis juillet 2023 et pourtant présentés comme courants.
		// Les seuils suivent la cadence réelle de chaque source : les comptes de
		// la protection sociale paraissent avec deux ans de recul, les
		// exonérations avec un peu plus d'un an, la masse salariale au trimestre.
		name: "aucune source budgétaire annuelle n'a pris plus de trois ans de retard",
		query: `SELECT count(*) FROM (
		          SELECT 1 FROM core.protection_sociale
		           HAVING max(annee) < extract(year FROM CURRENT_DATE) - 3
		          UNION ALL
		          SELECT 1 FROM core.exoneration_cotisation
		           HAVING max(annee) < extract(year FROM CURRENT_DATE) - 3
		          UNION ALL
		          SELECT 1 FROM core.masse_salariale
		           HAVING max(dernier_jour) < CURRENT_DATE - INTERVAL '18 months') x`,
	},
	{
		name: "l'exécution de l'État a moins de quinze mois de retard",
		query: `SELECT count(*) FROM (
		          SELECT 1 FROM core.execution_etat
		           HAVING max(date_arrete) < CURRENT_DATE - INTERVAL '15 months') x`,
	},
	{
		// Le schéma doit rendre la faute impossible, pas la signaler. Ce contrôle
		// vérifie que la vue de rapprochement tient sa promesse : elle n'apparie
		// jamais deux valeurs de comptabilités différentes. Comparer un solde de
		// loi de finances et un besoin de financement au sens de Maastricht
		// fabriquerait un « écart d'exécution » qui ne mesure rien.
		name: "le rapprochement n'aligne jamais deux comptabilités différentes",
		query: `SELECT count(*) FROM derived.budget_rapprochement
		         WHERE solde_comptes_meur IS NOT NULL AND comptabilite <> 'NATIONALE'`,
	},
	{
		// La composition des gouvernements n'existait plus en open data après
		// 2014 : le jeu des services du Premier ministre est gelé depuis le
		// 18 juin 2014. Les décrets du Journal officiel comblent le trou ; ce
		// contrôle vérifie qu'ils le comblent vraiment.
		name: "les décrets de gouvernement couvrent 2014-2026",
		query: `SELECT count(DISTINCT date_trunc('year', coalesce(date_texte, date_publi)))
		          FROM core.acte_jo
		         WHERE titre_complet ~* 'composition du gouvernement|nomination du premier ministre'
		           AND coalesce(date_texte, date_publi) >= '2014-01-01'`,
		min: 9,
	},
	{
		// « 2999-01-01 » est la sentinelle que la DILA écrit quand la date d'un
		// texte est inconnue. Prise au mot, elle datait dix-huit décrets de
		// composition du trentième siècle et faussait l'ordre chronologique dont
		// dépend la déduction des périodes ministérielles. Une sentinelle n'est
		// pas une date : elle doit être NULL.
		name: "aucun acte du Journal officiel n'est daté du trentième siècle",
		query: `SELECT count(*) FROM core.acte_jo
		         WHERE date_texte > '2100-01-01' OR date_publi > '2100-01-01'`,
	},
	{
		// Un décret qui compose un gouvernement entier nomme plus de dix
		// ministres de plein exercice. S'il n'en reste aucun de cette taille,
		// c'est que la lecture de la prose a cessé de fonctionner — et elle
		// échouerait en silence, un décret illisible ne produisant pas d'erreur.
		name: "des décrets composent un gouvernement entier",
		query: `SELECT count(*) FROM (
		          SELECT acte_id FROM core.gouvernement_membre
		           WHERE sens = 'NOMINATION' AND fonction IN ('MINISTRE', 'MINISTRE_ETAT')
		           GROUP BY 1 HAVING count(*) >= 10) x`,
		min: 10,
	},
	{
		// Les périodes déduites doivent être ordonnées. Une fin antérieure au
		// début signalerait que l'ordre chronologique des décrets est faux —
		// exactement ce que la sentinelle 2999 provoquait.
		name: "aucune fonction ministérielle ne se termine avant de commencer",
		query: `SELECT count(*) FROM derived.mandat_ministeriel
		         WHERE NOT upper_inf(validity) AND upper(validity) <= lower(validity)`,
	},
	{
		// Le rapprochement d'un nom de décret avec une personne de la base reste
		// CANDIDAT : le décret ne porte aucune date de naissance, et 41,9 % de
		// nos élus ont un homonyme exact. Rien ne doit y être CONFIRME tant
		// qu'un identifiant ne l'a pas établi.
		name: "aucun membre du Gouvernement n'est confirmé sans identifiant",
		query: `SELECT count(*) FROM core.gouvernement_membre g
		         WHERE g.statut = 'CONFIRME'
		           AND NOT EXISTS (SELECT 1 FROM core.person_identifier i
		                            WHERE i.person_id = g.person_id)`,
	},
	{
		// Le corpus du Journal officiel. Un chargement partiel ne lève aucune
		// erreur : les quatre flux COPY sont indépendants, et l'un peut rendre
		// zéro ligne pendant que les autres réussissent — c'est exactement ce
		// qui est arrivé quand le routage se faisait sur le chemin et non sur
		// le nom de base, et seul un compteur resté à zéro l'a dit.
		name:  "le corpus du Journal officiel porte plus d'un million d'actes",
		query: `SELECT count(*) FROM jo.texte`,
		min:   1000000,
	},
	{
		name:  "chaque acte du corpus porte au moins un bloc de texte",
		query: `SELECT count(*) FROM jo.bloc`,
		min:   3000000,
	},
	{
		// La clé étrangère est retirée pendant le chargement en masse et remise
		// ensuite. Si le connecteur échoue entre les deux, elle reste absente —
		// et la table accepterait alors des blocs orphelins sans rien dire.
		name: "la clé étrangère du corpus est bien remise après le chargement",
		query: `SELECT count(*) FROM pg_constraint
		         WHERE conname = 'bloc_texte_fk' AND conrelid = 'jo.bloc'::regclass`,
		min: 1,
	},
	{
		// Le vecteur de recherche vit dans une VUE MATÉRIALISÉE, et non dans une
		// colonne : pg_dump n'en emporte que la définition, et PostgreSQL
		// connaît la dépendance vers jo.bloc. Une vue ne peut pas dériver ligne
		// à ligne — elle est fraîche ou périmée — mais elle peut être PÉRIMÉE,
		// ce qui se voit à un décompte qui ne suit plus la table.
		name: "la vue de recherche du corpus couvre tous les blocs",
		query: `SELECT count(*) FROM jo.bloc
		         WHERE NOT EXISTS (SELECT 1 FROM jo.recherche_bloc r WHERE r.id = jo.bloc.id)`,
	},
	{
		// L'index UNIQUE conditionne REFRESH ... CONCURRENTLY. Sans lui, tout
		// rafraîchissement prend un verrou exclusif et coupe la recherche
		// pendant les quarante-quatre secondes qu'il dure.
		name: "la vue de recherche peut être rafraîchie sans couper le service",
		query: `SELECT count(*) FROM pg_index i
		         WHERE i.indrelid = 'jo.recherche_bloc'::regclass AND i.indisunique`,
		min: 1,
	},
	{
		// La configuration `fr` déaccentue avant de désuffixer. Si elle
		// retombait sur la configuration `french` livrée, « Élysée » et
		// « Elysee » redeviendraient deux lexèmes différents et la recherche
		// perdrait des actes en silence — 60 contre 10 sur les seuls titres.
		name: "la recherche en français ignore les accents",
		query: `SELECT count(*) FROM (
		          SELECT to_tsvector('fr', 'Élysée') = to_tsvector('fr', 'Elysee') AS ok) x
		         WHERE ok`,
		min: 1,
	},
	{
		// Le thésaurus reconnaît par PHRASE : prénom immédiatement suivi du nom.
		// Une requête à un seul terme signalerait qu'un nom n'a pas été analysé
		// comme attendu, et croiserait alors deux mots au lieu d'identifier une
		// personne.
		name:  "toute entrée du thésaurus des élus est une phrase",
		query: `SELECT count(*) FROM ref.elu_recherche WHERE numnode(requete) < 2`,
	},
	{
		// Les trois référentiels sont semés par la migration. S'ils sont vides,
		// aucune valeur budgétaire n'a pu être chargée — et les contrôles de
		// volume ci-dessus l'auraient dit — mais le message serait obscur.
		name: "les référentiels de périmètre, comptabilité et stade sont semés",
		query: `SELECT least(
		          (SELECT count(*) FROM ref.budget_perimetre),
		          (SELECT count(*) FROM ref.budget_comptabilite),
		          (SELECT count(*) FROM ref.budget_stade))`,
		min: 3,
	},
	{
		name:  "Open Damir couvre les douze mois de 2025",
		query: `SELECT count(*) FROM core.remboursement_national WHERE annee = 2025`,
		min:   12,
	},
	{
		// Le montant remboursé mensuel observé varie entre 10,2 et 13,5 Md€ sur
		// 2025 — un mois hors de [5, 20] signalerait un fichier tronqué ou un
		// filtre PRS_REM_TYP mal appliqué (internal/damir/damir.go), pas une
		// variation saisonnière réelle.
		name: "chaque mois Open Damir reste dans un ordre de grandeur plausible",
		query: `SELECT count(*) FROM core.remboursement_national
		         WHERE montant_rembourse NOT BETWEEN 5e9 AND 20e9`,
	},
	{
		// region_code x prs_nat_code est censé être la clé d'agrégation exacte
		// (internal/damir/damir.go) — un doublon signalerait que l'index unique
		// reconstruit après le COPY n'a pas fait son travail.
		name: "Open Damir région×prestation n'a aucun doublon (année, mois, région, prestation)",
		query: `SELECT count(*) - count(DISTINCT (annee, mois, region_code, prs_nat_code))
		          FROM core.remboursement_region_prestation`,
	},
	{
		// La somme des lignes région×prestation d'un mois ne peut pas dépasser
		// le total national du même mois : chaque acte n'est compté qu'une
		// fois dans l'agrégation en flux (agregerFichier), les deux tables
		// partent du même flux filtré PRS_REM_TYP=0.
		name: "Open Damir région×prestation ne dépasse jamais le total national du même mois",
		query: `SELECT count(*) FROM (
		          SELECT r.annee, r.mois, sum(r.montant_rembourse) AS regional, max(n.montant_rembourse) AS national
		            FROM core.remboursement_region_prestation r
		            JOIN core.remboursement_national n USING (annee, mois)
		           GROUP BY r.annee, r.mois
		        ) x WHERE regional > national * 1.001`,
	},
	{
		// code_departement est NOT NULL sur toute la table (démographie Cnam
		// construite par rattachement de caisse, pas par adresse déclarative) :
		// hors la ligne pseudo-département "999" (totaux déjà agrégés), les
		// généralistes doivent couvrir les 101 départements sans exception —
		// c'est précisément ce qui a fait préférer cette source au RPPS pour
		// la carte de densité (cmd/build/territoires.go).
		name: "les généralistes du secteur conventionnel couvrent les 101 départements, chaque exercice depuis 2013",
		query: `SELECT count(*) FROM (
		          SELECT annee, count(DISTINCT code_departement) AS nb
		            FROM core.medecin_secteur_effectif
		           WHERE code_departement <> '999' AND annee >= 2013
		             AND profession_sante IN
		               ('Médecins généralistes (hors médecins à expertise particulière - MEP)',
		                'Médecins généralistes à expertise particulière (MEP)')
		           GROUP BY annee HAVING count(DISTINCT code_departement) < 101
		        ) x`,
	},
	{
		name:  "la nomenclature des régions Open Damir a ses quatorze codes",
		query: `SELECT count(*) FROM ref.damir_region`,
		min:   14,
	},
	{
		// La contrainte de clé étrangère (migration 0107) empêcherait déjà un
		// code non décodé d'entrer ; cette sonde couvre le cas où la
		// contrainte aurait été retirée sans que la donnée le reflète.
		name: "toute région Open Damir se décode dans ref.damir_region",
		query: `SELECT count(DISTINCT r.region_code) FROM core.remboursement_region_prestation r
		         WHERE NOT EXISTS (SELECT 1 FROM ref.damir_region d WHERE d.code = r.region_code)`,
	},
	{
		// La dette FMI (internal/dette/fmi.go) couvre 14 pays de longue date ;
		// Chine, Russie et Arabie saoudite l'ont rejointe pour
		// docs/international-donnees.md § 6 (le seul jeu ouvert qui mesure
		// leur dette avec un concept proche de celui utilisé pour la France).
		name:  "la dette brute FMI couvre les dix-sept pays de comparaison",
		query: `SELECT count(DISTINCT serie) FROM core.dette_observation WHERE serie LIKE 'fmi:GGXWDG_NGDP:%'`,
		min:   17,
	},
	{
		// L'Arabie saoudite est absente par construction (l'OCDE ne la
		// couvre pas pour cet indicateur, internal/international/sante_ocde.go)
		// : neuf pays, pas dix, sans que ce soit une régression.
		name:  "l'espérance de vie OCDE couvre les neuf pays qu'elle publie",
		query: `SELECT count(DISTINCT pays_code) FROM core.indicateur_mondial WHERE indicateur = 'OCDE_ESPERANCE_VIE_NAISSANCE'`,
		min:   9,
	},
	{
		// Une valeur hors de [65, 95] ans signalerait une colonne mal lue
		// (l'export SDMX porte treize dimensions, une confusion d'index est
		// facile) plutôt qu'une espérance de vie réelle.
		name:  "l'espérance de vie OCDE reste dans un intervalle plausible",
		query: `SELECT count(*) FROM core.indicateur_mondial WHERE indicateur = 'OCDE_ESPERANCE_VIE_NAISSANCE' AND valeur NOT BETWEEN 65 AND 95`,
	},
	{
		name:  "la dépense de santé OCDE couvre au moins huit pays",
		query: `SELECT count(DISTINCT pays_code) FROM core.indicateur_mondial WHERE indicateur = 'OCDE_DEPENSE_SANTE_HABITANT'`,
		min:   8,
	},
	{
		name:  "la dépense militaire SIPRI couvre les dix pays de comparaison",
		query: `SELECT count(DISTINCT pays_code) FROM core.indicateur_mondial WHERE indicateur = 'SIPRI_DEPENSE_MILITAIRE_PIB'`,
		min:   10,
	},
	{
		// Un mélange de fraction brute et de pourcentage déjà mis en forme
		// coexiste dans le classeur SIPRI (internal/international/sipri.go) :
		// une valeur hors de [0, 20] signalerait que l'un des deux formats a
		// été mal détecté (facteur 100 appliqué ou non par erreur).
		name:  "la dépense militaire SIPRI reste dans un ordre de grandeur plausible",
		query: `SELECT count(*) FROM core.indicateur_mondial WHERE indicateur = 'SIPRI_DEPENSE_MILITAIRE_PIB' AND valeur NOT BETWEEN 0 AND 20`,
	},
	{
		name:  "la participation électorale IDEA couvre au moins huit pays",
		query: `SELECT count(DISTINCT pays_iso3) FROM core.participation_electorale`,
		min:   8,
	},
	{
		// Un taux hors de [0, 100] signalerait une colonne mal alignée
		// (l'export IDEA a été lu par position d'en-tête, pas par index fixe,
		// mais un changement de nom de colonne romprait silencieusement le
		// mappage sans cette sonde).
		name:  "la participation électorale IDEA reste un pourcentage valide",
		query: `SELECT count(*) FROM core.participation_electorale WHERE taux_participation_inscrits NOT BETWEEN 0 AND 100`,
	},
	{
		// Les huit sous-fonctions COFOG de la protection sociale (GF10.x)
		// doivent sommer exactement le total GF10, chaque année — sinon une
		// sous-fonction a été mal codée ou une nouvelle a été introduite par
		// Eurostat sans être ajoutée à internal/macro/macro.go.
		name: "les sous-fonctions COFOG de la protection sociale somment le total GF10",
		query: `SELECT count(*) FROM (
		          SELECT mv.annee,
		                 max(mv.valeur) FILTER (WHERE rs.cofog = 'GF10') AS total,
		                 sum(mv.valeur) FILTER (WHERE rs.cofog LIKE 'GF10__') AS somme
		            FROM core.macro_value mv JOIN ref.macro_serie rs ON rs.code = mv.serie_code
		           WHERE rs.cofog = 'GF10' OR rs.cofog LIKE 'GF10__'
		           GROUP BY mv.annee
		        ) x WHERE abs(total - somme) > 0.5`,
	},
	{
		name:  "l'APA à domicile couvre au moins cent départements chaque année depuis 2010",
		query: `SELECT count(*) FROM (SELECT annee, count(*) AS nb FROM core.apa_domicile GROUP BY annee HAVING count(*) < 100) x`,
	},
	{
		// Le total France (somme des départements déclarants, donc un
		// plancher : les 'ND' ne sont jamais comptés comme 0) doit rester
		// dans un ordre de grandeur plausible — hors de [1,5] Md€ signalerait
		// une colonne mal lue (séparateur de milliers, décalage d'index).
		name: "le total national APA à domicile reste dans un ordre de grandeur plausible",
		query: `SELECT count(*) FROM (
		          SELECT annee, sum(depenses_total_eur) AS total FROM core.apa_domicile GROUP BY annee
		        ) x WHERE total NOT BETWEEN 1e9 AND 5e9`,
	},
	{
		name:  "InserJeunes couvre au moins six promotions",
		query: `SELECT count(DISTINCT annee_cumul) FROM core.insertion_apprentissage`,
		min:   6,
	},
	{
		name:  "les taux InserJeunes restent des pourcentages valides",
		query: `SELECT count(*) FROM core.insertion_apprentissage WHERE taux_emploi_6_mois NOT BETWEEN 0 AND 100`,
	},
	{
		name:  "la population par département et âge couvre au moins quatre-vingt-dix départements chaque année",
		query: `SELECT count(*) FROM (SELECT annee, count(DISTINCT code_departement) AS nb FROM core.population_age_departement GROUP BY annee HAVING count(DISTINCT code_departement) < 90) x`,
	},
	{
		// Bornes larges mais réelles : 2,66 millions en 1975, 7,3 millions en
		// 2025 (vérifié). Hors de [2, 8] millions signalerait une colonne mal
		// lue (décalage d'index, séparateur de milliers) plutôt qu'un vrai
		// changement démographique.
		name: "la population nationale de 75 ans ou plus reste dans un ordre de grandeur plausible",
		query: `SELECT count(*) FROM (
		          SELECT annee, sum(population) AS total FROM core.population_age_departement
		          WHERE tranche = '75_PLUS' GROUP BY annee
		        ) x WHERE total NOT BETWEEN 2e6 AND 8e6`,
	},
	{
		name: "chaque département porte exactement ses cinq tranches d'âge chaque année",
		query: `SELECT count(*) FROM (
		          SELECT code_departement, annee, count(*) AS nb
		          FROM core.population_age_departement GROUP BY code_departement, annee
		        ) x WHERE nb <> 5`,
	},
	{
		name: "l'emploi total et l'emploi salarié couvrent 1975 à aujourd'hui sans trou",
		query: `SELECT count(*) FROM (
		          SELECT annee FROM core.macro_value WHERE serie_code = 'emploi.total'
		          EXCEPT SELECT annee FROM core.macro_value WHERE serie_code = 'emploi.salarie'
		          UNION
		          SELECT annee FROM core.macro_value WHERE serie_code = 'emploi.salarie'
		          EXCEPT SELECT annee FROM core.macro_value WHERE serie_code = 'emploi.total'
		        ) x`,
	},
	{
		// L'emploi salarié ne peut jamais dépasser l'emploi total (les non-
		// salariés se déduisent des deux par soustraction, jamais négatifs).
		name: "l'emploi salarié ne dépasse jamais l'emploi total, chaque année",
		query: `SELECT count(*) FROM (
		          SELECT t.annee FROM core.macro_value t
		          JOIN core.macro_value s ON s.serie_code = 'emploi.salarie' AND s.annee = t.annee
		          WHERE t.serie_code = 'emploi.total' AND s.valeur > t.valeur
		        ) x`,
	},
	{
		// D5 < D9 < Q99 < Q99,9 < Q99,99, sur les deux colonnes (revenu avant
		// redistribution et niveau de vie) : un seuil plus haut dans la
		// distribution ne peut jamais correspondre à un montant plus bas.
		name: "les seuils de très hauts revenus sont strictement croissants",
		query: `SELECT count(*) FROM (
		          SELECT annee,
		                 revenu_avant_redistribution_eur - lag(revenu_avant_redistribution_eur) OVER w AS d_avant,
		                 niveau_de_vie_eur - lag(niveau_de_vie_eur) OVER w AS d_niveau
		          FROM core.filosofi_haut_revenu
		          WINDOW w AS (PARTITION BY annee ORDER BY array_position(
		            ARRAY['D5','D9','Q99','Q99_9','Q99_99'], seuil))
		        ) x WHERE d_avant IS NOT NULL AND (d_avant <= 0 OR d_niveau <= 0)`,
	},
	{
		name: "les quatre groupes non recouvrants de revenu_part_groupe somment à 100 %, chaque année",
		query: `SELECT count(*) FROM (
		          SELECT annee, sum(part_pct) AS total FROM core.revenu_part_groupe
		          WHERE groupe IN ('90_MODESTES','9_SUIVANTS','0_9_SUIVANTS','0_1_PLUS_AISES')
		          GROUP BY annee
		        ) x WHERE total NOT BETWEEN 99.5 AND 100.5`,
	},
	{
		name: "1_PLUS_AISES égale 0_9_SUIVANTS + 0_1_PLUS_AISES, chaque année",
		query: `SELECT count(*) FROM (
		          SELECT a.annee FROM core.revenu_part_groupe a
		          JOIN core.revenu_part_groupe b ON b.groupe='0_9_SUIVANTS' AND b.annee=a.annee
		          JOIN core.revenu_part_groupe c ON c.groupe='0_1_PLUS_AISES' AND c.annee=a.annee
		          WHERE a.groupe='1_PLUS_AISES' AND abs(a.part_pct - (b.part_pct + c.part_pct)) > 0.15
		        ) x`,
	},
	{
		name: "les tranches de hauts patrimoines sont strictement croissantes, chaque année",
		query: `SELECT count(*) FROM (
		          SELECT annee, seuil_bas_eur - lag(seuil_bas_eur) OVER w AS d
		          FROM core.patrimoine_haut
		          WINDOW w AS (PARTITION BY annee ORDER BY array_position(
		            ARRAY['P90_P95','P95_P99','SUP_P99'], tranche))
		        ) x WHERE d IS NOT NULL AND d <= 0`,
	},
	{
		name:  "la part des ménages ayant hérité ou reçu une donation reste un pourcentage valide",
		query: `SELECT count(*) FROM core.menage_heritage WHERE part_herite_pct NOT BETWEEN 0 AND 100 OR part_donation_pct NOT BETWEEN 0 AND 100`,
	},
	{
		// Chaque catégorie de ménage a bien ses trois tranches d'âge (40-59,
		// 60 ou plus, tous âges) — jamais une tranche manquante en silence.
		name: "chaque catégorie de ménage a ses trois tranches d'âge dans menage_heritage",
		query: `SELECT count(*) FROM (
		          SELECT categorie, count(*) AS nb FROM core.menage_heritage GROUP BY categorie
		        ) x WHERE nb <> 3`,
	},
	{
		// Les ménages aisés en patrimoine ET niveau de vie ont hérité plus
		// souvent que l'ensemble des ménages, à tranche d'âge égale — le
		// point que docs/repartition-richesse-donnees.md § 4 documente : si
		// ce n'était pas vrai dans les données, la note ne pourrait pas
		// l'affirmer.
		name: "les ménages à haut patrimoine et haut niveau de vie ont hérité plus souvent que l'ensemble, à âge égal",
		query: `SELECT count(*) FROM (
		          SELECT h.tranche_age FROM core.menage_heritage h
		          JOIN core.menage_heritage e ON e.categorie='ENSEMBLE' AND e.tranche_age=h.tranche_age
		          WHERE h.categorie='HAUT_PATRIMOINE_ET_NIVEAU_VIE' AND h.part_herite_pct <= e.part_herite_pct
		        ) x`,
	},
	{
		name:  "l'indice de Gini du patrimoine et du niveau de vie reste entre 0 et 1",
		query: `SELECT count(*) FROM core.gini_patrimoine_niveau_vie WHERE indice_patrimoine NOT BETWEEN 0 AND 1 OR indice_niveau_vie NOT BETWEEN 0 AND 1`,
	},
	{
		// Le patrimoine est structurellement plus concentré que le revenu en
		// France (constat documenté par la source elle-même) : l'indice de
		// Gini du patrimoine doit rester supérieur à celui du niveau de vie.
		name:  "l'indice de Gini du patrimoine est supérieur à celui du niveau de vie",
		query: `SELECT count(*) FROM core.gini_patrimoine_niveau_vie WHERE indice_patrimoine <= indice_niveau_vie`,
	},
	{
		// Garde-fou de reconstruction de total : le foncier bâti seul (le
		// plus gros des quatre dispositifs chargés) doit rester dans un
		// ordre de grandeur plausible par année — le repère précis qui
		// aurait détecté la confusion P33/P33_1+P33_2 découverte à
		// l'ingestion (un calcul naïf donnait 21,9 Md€ de CFE intercommunale
		// contre 7,3 Md€ réels : un facteur ~3, que cette fourchette large
		// suffit à attraper si elle se reproduit sur le foncier bâti).
		name: "le produit du foncier bâti (bloc communal) reste dans un ordre de grandeur plausible, chaque année",
		query: `SELECT count(*) FROM (
		          SELECT annee, sum(montant_eur) AS total FROM core.fiscalite_directe_locale
		          WHERE dispositif = 'FB' GROUP BY annee
		        ) x WHERE total NOT BETWEEN 20e9 AND 70e9`,
	},
	{
		// categorie_payeur doit rester cohérent avec le dispositif fiscal —
		// jamais une CFE classée « ménages » ou un foncier classé
		// « entreprises » par une régression future du connecteur.
		name: "categorie_payeur de fiscalite_directe_locale reste cohérent avec le dispositif fiscal",
		query: `SELECT count(*) FROM core.fiscalite_directe_locale
		        WHERE (dispositif IN ('FB','FNB') AND categorie_payeur <> 'MENAGES')
		           OR (dispositif IN ('CFE','TASCOM') AND categorie_payeur <> 'ENTREPRISES')`,
	},
	{
		// Garde-fou contre exactement le piège trouvé à l'inspection avant
		// chargement (p101_1, un taux de conformité microbiologique 0-100,
		// avait failli passer pour le prix de l'eau) : le prix réel doit
		// rester dans un ordre de grandeur plausible pour un service français
		// d'eau potable, pas dans une plage 0-100 qui trahirait un mélange de
		// colonnes si le format SISPEA changeait de nom de colonne demain.
		name: "le prix médian de l'eau potable (SISPEA) reste dans un ordre de grandeur plausible",
		query: `SELECT count(*) FROM (
		          SELECT annee, percentile_cont(0.5) WITHIN GROUP (ORDER BY prix_eur_m3) AS mediane
		          FROM core.service_eau_potable WHERE prix_eur_m3 IS NOT NULL GROUP BY annee
		        ) x WHERE mediane NOT BETWEEN 0.5 AND 6`,
	},
	{
		name: "mode_gestion des services d'eau potable ne contient que des valeurs connues",
		query: `SELECT count(*) FROM core.service_eau_potable
		        WHERE mode_gestion IS NOT NULL AND mode_gestion NOT IN ('REGIE','DELEGATION')`,
	},
	{
		// Une aide négative trahirait une erreur de colonne (par exemple un
		// taux en % lu à la place du montant en €) plutôt qu'une vraie
		// décision d'aide. Un montant à zéro, en revanche, existe réellement
		// dans le fichier Rhin-Meuse : 40 dossiers « Soldé » à 0 € (vérifié
		// à l'inspection, voir SourceAidesRhinMeuse.Notes) — pas une erreur
		// de chargement, donc pas rejeté ici.
		name: "les aides des agences de l'eau n'ont jamais un montant négatif",
		query: `SELECT count(*) FROM core.aide_agence_eau WHERE montant_eur < 0`,
	},
	{
		// Le total annuel Loire-Bretagne (le plus gros des deux bassins
		// chargés) doit rester dans un ordre de grandeur plausible pour une
		// seule agence de l'eau, jamais au niveau des ~2 Md€/an des SIX
		// agences réunies (12e programme national, 2025-2030).
		name: "le total annuel des aides Loire-Bretagne reste dans un ordre de grandeur plausible",
		query: `SELECT count(*) FROM (
		          SELECT annee, sum(montant_eur) AS total FROM core.aide_agence_eau
		          WHERE agence = 'LOIRE_BRETAGNE' GROUP BY annee
		        ) x WHERE total NOT BETWEEN 50e6 AND 700e6`,
	},
	{
		name:  "au moins 35 000 aides Rhin-Meuse sont chargées",
		query: `SELECT count(*) FROM core.aide_agence_eau WHERE agence = 'RHIN_MEUSE'`,
		min:   35000,
	},
	{
		name:  "aide_agence_eau.agence ne contient que des valeurs connues",
		query: `SELECT count(*) FROM core.aide_agence_eau WHERE agence NOT IN ('LOIRE_BRETAGNE','ARTOIS_PICARDIE','RHIN_MEUSE')`,
	},
	{
		// Chaque EPTB/EPAGE affiché sur la carte doit avoir un contour
		// géométrique réel : un contour reconstruit sans membre résolu
		// n'aurait jamais dû être inséré (garde HAVING count(g.geom)>0 côté
		// connecteur) — ce contrôle vérifie que ça reste vrai.
		name: "tout contour EPTB/EPAGE reconstruit couvre au moins un membre",
		query: `SELECT count(*) FROM geo.contour_eptb_epage WHERE nb_membres_resolus = 0`,
	},
	{
		name: "le type EPTB/EPAGE ne contient que des valeurs connues",
		query: `SELECT count(*) FROM core.eptb_epage WHERE type NOT IN ('EPTB','EPAGE','EPTB_EPAGE')`,
	},
	{
		// Le seuil de publication de la DGFiP (plus de 50 redevables) doit
		// rester vrai dans les données chargées : une ligne en dessous
		// trahirait une erreur de colonne ou un fichier différent de celui
		// documenté.
		name: "IFICOM : chaque commune publiée dépasse bien le seuil de 50 redevables",
		query: `SELECT count(*) FROM core.ifi_commune WHERE nombre_redevables <= 50`,
	},
	{
		// Le patrimoine moyen des redevables IFI d'une commune ne peut pas
		// être inférieur au seuil d'assujettissement (1,3 M€) : ce serait la
		// preuve d'une colonne mélangée avec une autre valeur.
		name: "IFICOM : le patrimoine moyen par commune reste au-dessus du seuil d'assujettissement",
		query: `SELECT count(*) FROM core.ifi_commune WHERE patrimoine_moyen_eur < 1300000`,
	},
	{
		// La branche C (industrie manufacturière) est une SOUS-catégorie de
		// B-E (industrie y compris énergie) : elle ne peut jamais la
		// dépasser sans trahir une confusion de colonnes.
		name: "emploi par secteur NACE : l'industrie manufacturière (C) ne dépasse jamais l'industrie entière (B-E)",
		query: `SELECT count(*) FROM (
		          SELECT annee FROM core.emploi_secteur_nace WHERE code_nace='C'
		        ) c JOIN (
		          SELECT annee, emploi_milliers FROM core.emploi_secteur_nace WHERE code_nace='B-E'
		        ) be USING (annee)
		        JOIN (SELECT annee, emploi_milliers FROM core.emploi_secteur_nace WHERE code_nace='C') cc USING (annee)
		        WHERE cc.emploi_milliers > be.emploi_milliers`,
	},
	{
		name:  "emploi par secteur NACE : au moins 40 années chargées (série 1975-2025)",
		query: `SELECT count(DISTINCT annee) FROM core.emploi_secteur_nace`,
		min:   40,
	},
	{
		// Les trois scénarios (bas/central/haut) sont des bornes d'un même
		// intervalle : le scénario bas ne peut jamais dépasser le central,
		// ni le central le haut, sans trahir une inversion de colonnes.
		name: "délocalisations : le scénario bas ne dépasse jamais le central, ni le central le haut (unités légales)",
		query: `SELECT count(*) FROM core.delocalisation_annuelle
		        WHERE unites_legales_bas > unites_legales_central
		           OR unites_legales_central > unites_legales_haut`,
	},
	{
		name: "délocalisations : le scénario bas ne dépasse jamais le central, ni le central le haut (emplois ETP)",
		query: `SELECT count(*) FROM core.delocalisation_annuelle
		        WHERE emplois_etp_bas IS NOT NULL
		          AND (emplois_etp_bas > emplois_etp_central OR emplois_etp_central > emplois_etp_haut)`,
	},
	{
		name:  "délocalisations : les 96 départements métropolitains sont chargés",
		query: `SELECT count(*) FROM core.delocalisation_departement`,
		min:   96,
	},
	{
		// Chaque part est un pourcentage : au-delà de 100, une colonne a été
		// décalée à la lecture du fichier.
		name: "délocalisations : les parts par catégorie socioprofessionnelle restent des pourcentages plausibles",
		query: `SELECT count(*) FROM core.delocalisation_csp
		        WHERE part_champ_general_pct NOT BETWEEN 0 AND 100
		           OR part_postes_delocalises_pct NOT BETWEEN 0 AND 100`,
	},
	{
		// Le total mondial (code_partenaire=0) doit rester la plus grande
		// valeur de son (secteur, code_hs, année) : un partenaire ne peut
		// jamais dépasser le monde entier.
		name: "commerce par partenaire : le total mondial domine chaque partenaire, secteur par secteur",
		query: `SELECT count(*) FROM core.commerce_partenaire_secteur c
		        WHERE c.code_partenaire <> 0 AND c.valeur_usd > (
		          SELECT m.valeur_usd FROM core.commerce_partenaire_secteur m
		          WHERE m.secteur=c.secteur AND m.code_hs=c.code_hs AND m.annee=c.annee AND m.code_partenaire=0)`,
	},
	{
		name:  "commerce par partenaire : les trois secteurs et les deux années sont tous chargés",
		query: `SELECT count(DISTINCT secteur||code_hs||annee) FROM core.commerce_partenaire_secteur`,
		min:   8,
	},
	{
		name:  "fond de carte mondial : au moins 200 pays/territoires chargés",
		query: `SELECT count(*) FROM geo.contour_pays`,
		min:   200,
	},
	{
		// Une part de francophones est un pourcentage : au-delà de 100, une
		// colonne a été décalée à la lecture du fichier.
		name:  "Francophonie : les parts restent des pourcentages plausibles",
		query: `SELECT count(*) FROM core.francophonie_entite WHERE francophone_pct NOT BETWEEN 0 AND 100`,
	},
	{
		// Le nombre de francophones ne peut jamais dépasser la population de
		// l'entité : ce serait la preuve d'une colonne mélangée.
		name: "Francophonie : le nombre de francophones ne dépasse jamais la population",
		query: `SELECT count(*) FROM core.francophonie_entite
		        WHERE francophone_milliers IS NOT NULL AND population_2025_milliers IS NOT NULL
		          AND francophone_milliers > population_2025_milliers`,
	},
	{
		// Reproduit ici la normalisation (accents, apostrophes typographiques)
		// et les six alias appliqués par cmd/build/francophonie.go, pour
		// vérifier le taux de rattachement réel plutôt qu'un plancher
		// arbitraire — si ce nombre baisse, le rendu de la carte a
		// probablement le même problème.
		name: "Francophonie : au moins 100 des 102 pays souverains se rattachent au fond de carte mondial",
		query: `SELECT count(*) FROM core.francophonie_entite f
		        JOIN geo.contour_pays g ON lower(unaccent(replace(replace(g.nom_fr,'''',''),'’',''))) =
		          lower(unaccent(replace(replace(CASE f.entite
		            WHEN 'Cabo Verde' THEN 'Cap-Vert'
		            WHEN 'Centrafrique' THEN 'République centrafricaine'
		            WHEN 'Congo' THEN 'République du Congo'
		            WHEN 'Congo (République démocratique du)' THEN 'République démocratique du Congo'
		            WHEN 'États-Unis d''Amérique' THEN 'États-Unis'
		            WHEN 'Fédération de Russie' THEN 'Russie'
		            ELSE f.entite END, '''', ''), '’', '')))
		        WHERE f.type_entite='pays'`,
		min: 100,
	},
	{
		name:  "Accord de Paris : au moins 190 pays chargés",
		query: `SELECT count(*) FROM core.ratification_accord_paris`,
		min:   190,
	},
	{
		// La ratification ne peut jamais précéder la signature.
		name: "Accord de Paris : aucune ratification antérieure à la signature",
		query: `SELECT count(*) FROM core.ratification_accord_paris
		        WHERE date_ratification IS NOT NULL AND date_signature IS NOT NULL
		          AND date_ratification < date_signature`,
	},
	{
		name:  "Empire colonial : les 22 territoires vérifiés sont chargés",
		query: `SELECT count(*) FROM geo.territoire_colonial`,
		min:   22,
	},
	{
		// L'indépendance ne peut jamais précéder l'année de rattachement —
		// ce serait la preuve d'une ligne mal recopiée entre les deux dates
		// saisies à la main (migration 0128).
		name: "Empire colonial : aucune indépendance antérieure au rattachement",
		query: `SELECT count(*) FROM geo.territoire_colonial
		        WHERE extract(year FROM date_independance) < annee_rattachement`,
	},
	{
		name:  "Seconde Guerre mondiale : la ligne de démarcation est chargée",
		query: `SELECT count(*) FROM geo.ligne_demarcation`,
		min:   1,
	},
	{
		name:  "Justice : les dix directions interrégionales sont chargées",
		query: `SELECT count(DISTINCT direction_interregionale) FROM core.etablissement_penitentiaire`,
		min:   10,
	},
	{
		// La densité (détenus/capacité) ne peut être négative ; une valeur
		// négative signalerait une colonne mal alignée à la lecture.
		name:  "Justice : aucune densité carcérale négative",
		query: `SELECT count(*) FROM core.etablissement_penitentiaire WHERE densite_pct < 0`,
	},
	{
		name:  "SRU : au moins 2000 communes chargées",
		query: `SELECT count(*) FROM core.sru_commune`,
		min:   2000,
	},
	{
		// La carte (cmd/build/logement.go) joint core.sru_commune à
		// geo.contour_cog par code Insee, avec un repli par nom pour les
		// quelques communes nouvelles dont le code diverge entre les deux
		// sources. Si ce repli devient ambigu (plusieurs communes de même
		// nom dans le même département), la jointure duplique des lignes ;
		// si le code du COG change sans mise à jour du repli, elle en perd.
		// Les deux comptes doivent rester strictement égaux.
		name: "SRU : la jointure vers geo.contour_cog ne perd ni ne duplique de commune",
		query: `SELECT count(*) - (
			SELECT count(*) FROM core.sru_commune s
			JOIN geo.contour_cog g ON g.niveau='COMMUNE' AND g.cog_millesime=2026
				AND (g.code=s.code_insee
					OR (upper(unaccent(g.nom))=upper(unaccent(s.commune))
						AND g.code_departement = left(s.code_insee, CASE WHEN left(s.code_insee,2)='97' THEN 3 ELSE 2 END)))
		) FROM core.sru_commune`,
	},
	{
		name:  "Effectifs étudiants : au moins 1000 couples commune/rentrée chargés",
		query: `SELECT count(*) FROM core.effectifs_etudiants_commune`,
		min:   1000,
	},
	{
		name:  "Effort de recherche : au moins 25 années chargées",
		query: `SELECT count(*) FROM core.effort_recherche`,
		min:   25,
	},
	{
		// Le DIRD/PIB français reste dans une fourchette de 1,5 à 3 % sur
		// toute la série connue (1990-2023) ; une valeur hors de cette plage
		// signalerait une colonne mal alignée à la lecture du classeur.
		name:  "Effort de recherche : DIRD/PIB France dans une fourchette plausible",
		query: `SELECT count(*) FROM core.effort_recherche WHERE dird_pib_fr NOT BETWEEN 1.5 AND 3.0`,
	},
	{
		name:  "Ports : au moins 500 tronçons autoroutiers proches des quatre ports chargés",
		query: `SELECT count(*) FROM geo.autoroute_portuaire`,
		min:   500,
	},
	{
		name:  "Ports : au moins 50 tronçons de voie ferrée portuaire chargés",
		query: `SELECT count(*) FROM geo.voie_ferree_portuaire`,
		min:   50,
	},
	{
		name:  "Ports : le report modal est chargé pour les quatre ports suivis",
		query: `SELECT count(*) FROM core.report_modal_port`,
		min:   4,
	},
	{
		// Un pourcentage ne peut jamais dépasser 100, qu'il s'agisse d'un
		// chiffre exact ou d'un plafond déclaré.
		name:  "Ports : aucun pourcentage de report modal hors de 0-100",
		query: `SELECT count(*) FROM core.report_modal_port WHERE part_massifiee_pct NOT BETWEEN 0 AND 100`,
	},
	{
		// Les trois modes doivent sommer à 100 (à l'arrondi près) pour
		// chaque port de la comparaison conteneurs — sinon une valeur a été
		// mal recopiée depuis la source.
		name: "Ports : la répartition modale conteneurs somme à 100 % par port",
		query: `SELECT count(*) FROM core.report_modal_conteneurs
		        WHERE abs(part_fer_pct + part_fleuve_pct + part_route_pct - 100) > 0.2`,
	},
	{
		name:  "Population historique : au moins 15 millésimes chargés",
		query: `SELECT count(DISTINCT annee) FROM core.population_historique_commune`,
		min:   15,
	},
	{
		// La population totale de la France (hors Mayotte) n'est jamais
		// descendue sous 35 M ni montée au-dessus de 65 M sur 1876-1999 :
		// une valeur hors de cette plage signalerait une agrégation erronée
		// (doublon de commune, unité mal lue).
		name: "Population historique : le total national par année reste plausible",
		query: `SELECT count(*) FROM (
		          SELECT annee, sum(population) total FROM core.population_historique_commune GROUP BY annee
		        ) x WHERE total NOT BETWEEN 35000000 AND 65000000`,
	},
	{
		// Le creux démographique de la Première Guerre mondiale (1911→1921)
		// est le fait central du § population du dossier Seconde Guerre
		// mondiale : si la source changeait de sens à ce sujet, la baisse ne
		// serait plus vérifiée.
		name: "Population historique : la population recule bien entre 1911 et 1921",
		query: `SELECT count(*) FROM (
		          SELECT
		            (SELECT sum(population) FROM core.population_historique_commune WHERE annee=1911) p1911,
		            (SELECT sum(population) FROM core.population_historique_commune WHERE annee=1921) p1921
		        ) x WHERE p1921 >= p1911`,
	},
	{
		name:  "Contrôle fiscal : au moins 10 années chargées, 2015-2024",
		query: `SELECT count(*) FROM core.controle_fiscal_resultats WHERE annee BETWEEN 2015 AND 2024`,
		min:   10,
	},
	{
		// Le notifié 2022 et 2023 n'a jamais été retrouvé dans une source
		// primaire : si une valeur apparaissait un jour à cette place, ce
		// serait un ajout à vérifier, pas un chargement silencieux.
		name:  "Contrôle fiscal : le notifié 2022 et 2023 reste bien non chargé",
		query: `SELECT count(*) FROM core.controle_fiscal_resultats WHERE annee IN (2022,2023) AND montant_notifie_m IS NOT NULL`,
	},
	{
		// L'encaissé ne peut jamais dépasser le notifié : on ne recouvre pas
		// plus que ce qui a été mis en recouvrement.
		name:  "Contrôle fiscal : l'encaissé ne dépasse jamais le notifié",
		query: `SELECT count(*) FROM core.controle_fiscal_resultats WHERE montant_notifie_m IS NOT NULL AND montant_encaisse_m > montant_notifie_m`,
	},
	{
		name:  "Outre-mer : l'écart de prix couvre les cinq DOM en 2022",
		query: `SELECT count(*) FROM core.ecart_prix_dom WHERE annee=2022`,
		min:   5,
	},
	{
		name:  "Musées : au moins 1000 musées labellisés Musée de France chargés",
		query: `SELECT count(*) FROM core.musee_france`,
		min:   1000,
	},
	{
		name:  "Revenu agricole : France et Union européenne chargées",
		query: `SELECT count(DISTINCT geo_code) FROM core.revenu_agricole_reel`,
		min:   2,
	},
	{
		name:  "Ports : au moins 40 ports français chargés (SDES)",
		query: `SELECT count(DISTINCT port) FROM core.trafic_portuaire`,
		min:   40,
	},
	{
		// tonnage_tot inclut la tare (poids des contenants) : les
		// sous-totaux de marchandises seules (vracs, conteneurs) ne
		// peuvent jamais le dépasser, sous peine de colonnes inversées.
		name: "Ports : les sous-totaux de marchandises ne dépassent jamais le tonnage total",
		query: `SELECT count(*) FROM core.trafic_portuaire
		        WHERE tonnage_tot IS NOT NULL AND vracs_liquides IS NOT NULL
		          AND vracs_liquides > tonnage_tot`,
	},
	{
		name:  "Ports : la comparaison européenne couvre les six ports nommés",
		query: `SELECT count(DISTINCT code_port) FROM core.trafic_portuaire_europe`,
		min:   6,
	},
	{
		name:  "poids économique mondial : l'UE est chargée aux côtés des dix pays de comparaison",
		query: `SELECT count(DISTINCT indicateur) FROM core.indicateur_mondial WHERE pays_code='EU'`,
		min:   10,
	},
	{
		// Le commerce extra-UE ne peut jamais dépasser le PIB de l'UE — ce
		// serait la preuve d'une confusion entre biens+services et biens
		// seuls, ou entre millions et unités.
		name: "commerce extra-UE : les exportations et importations restent sous le PIB de l'UE",
		query: `SELECT count(*) FROM (
		          SELECT annee, valeur*1e6 AS export_eur FROM core.indicateur_mondial
		          WHERE pays_code='EU' AND indicateur='EU_EXTRA_EXPORT_MEUR'
		        ) e JOIN (
		          SELECT annee, valeur AS pib FROM core.indicateur_mondial
		          WHERE pays_code='EU' AND indicateur='NY.GDP.MKTP.CD'
		        ) g USING (annee)
		        WHERE e.export_eur > g.pib`,
	},
}

// ErrAnomalies signale qu'au moins un contrôle a échoué — déjà détaillé sur
// stderr par Run, jamais un message à répéter par l'appelant.
var ErrAnomalies = errors.New("des anomalies ont été trouvées")

// Run exécute la commande verify. Ne prend aucune option ; args n'existe que
// pour l'uniformité avec les autres commandes routées par fpctl.
func Run(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("verify ne prend aucune option (%q inattendu)", args[0])
	}
	ctx := context.Background()
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	failed := false
	for _, c := range checks {
		var n int
		if err := pool.QueryRow(ctx, c.query).Scan(&n); err != nil {
			fmt.Fprintf(os.Stderr, "  ECHEC  %s : %v\n", c.name, err)
			failed = true
			continue
		}
		switch {
		case c.min > 0 && n < c.min:
			fmt.Fprintf(os.Stderr, "  ECHEC  %s : %d (minimum attendu %d)\n", c.name, n, c.min)
			failed = true
		case c.min == 0 && n != 0:
			fmt.Fprintf(os.Stderr, "  ECHEC  %s : %d anomalie(s)\n", c.name, n)
			failed = true
		default:
			fmt.Printf("  ok     %s\n", c.name)
		}
	}
	if failed {
		fmt.Fprintln(os.Stderr, "\nPublication bloquée : les données chargées ne concordent pas.")
		return ErrAnomalies
	}
	fmt.Println("\ncohérence vérifiée")
	return nil
}
