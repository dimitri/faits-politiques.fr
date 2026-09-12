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
		name: "la quasi-totalité des votes porte un groupe",
		query: `SELECT (100 * count(*) FILTER (WHERE organization_id IS NOT NULL))
		               / greatest(count(*),1) FROM core.ballot`,
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
		                     AND i.scheme IN ('AN_ACTEUR','EP_MEP'))`,
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
