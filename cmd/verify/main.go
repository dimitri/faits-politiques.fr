// Commande verify : contrôles de cohérence exécutés avant toute publication.
//
// Ils portent sur les DONNÉES chargées, là où db/tests/ porte sur les règles du
// schéma. Un écart signale une erreur d'ingestion : publier des chiffres qui ne
// concordent pas avec le relevé officiel serait la pire faute possible pour un
// outil de vérification.
package main

import (
	"context"
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
}

func main() {
	ctx := context.Background()
	pool, err := store.Open(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
		os.Exit(1)
	}
	fmt.Println("\ncohérence vérifiée")
}
