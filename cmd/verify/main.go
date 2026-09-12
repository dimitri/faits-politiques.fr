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
