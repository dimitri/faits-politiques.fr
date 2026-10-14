package verify

// Contrôles du financement des SDIS (docs/securite-civile-donnees.md),
// chargés par internal/securitecivile.IngestSDIS dans core.collectivite_budget
// (niveau='SDIS') et core.sdis.
func init() {
	checks = append(checks, checksSDIS...)
}

var checksSDIS = []check{
	{
		name:    "des comptes de SDIS sont chargés",
		query:   `SELECT count(*) FROM core.collectivite_budget WHERE niveau = 'SDIS'`,
		min:     10000,
		sources: []string{"sdis"},
	},
	{
		name:    "la dimension SDIS est peuplée",
		query:   `SELECT count(*) FROM core.sdis`,
		min:     90,
		sources: []string{"sdis"},
	},
	// Chaque SDIS connu de la dimension doit avoir un département de
	// rattachement — sans lui, aucune carte par département n'est possible.
	{
		name:    "chaque SDIS a un département de rattachement",
		query:   `SELECT count(*) FROM core.sdis WHERE code_departement IS NULL OR code_departement = ''`,
		sources: []string{"sdis"},
	},
	// Continuité : le dernier exercice chargé doit compter un nombre de SDIS
	// proche du total connu (core.sdis) — un effondrement signalerait un
	// filtre cbudg ou un schéma de colonnes cassé plutôt qu'une vraie
	// disparition de services d'incendie et de secours.
	{
		name: "le dernier exercice SDIS couvre la quasi-totalité des services connus",
		query: `SELECT count(*) FROM (
		          SELECT 1
		            FROM (SELECT max(exercice) AS dernier FROM core.collectivite_budget WHERE niveau = 'SDIS') m
		           WHERE (SELECT count(DISTINCT code) FROM core.collectivite_budget
		                   WHERE niveau = 'SDIS' AND exercice = m.dernier)
		                 < (SELECT count(*) FROM core.sdis) - 10
		        ) t`,
		sources: []string{"sdis"},
	},
	// Borne de plausibilité, pas une identité comptable : core.collectivite_
	// budget (niveau SDIS) ne charge que 10 agrégats sur la soixantaine que
	// publie l'OFGL, donc participations + concours de l'État ne peuvent
	// qu'être INFÉRIEURS ou égaux aux recettes totales (il manque entre
	// autres les ventes, produits de cession, reprises sur provisions).
	// Un dépassement signalerait un agrégat mal mappé (deux fois compté),
	// pas une vraie recette.
	{
		name: "SDIS : participations des collectivités + concours de l'État ne dépassent jamais les recettes totales",
		query: `SELECT count(*) FROM (
		          SELECT code, exercice,
		                 sum(montant) FILTER (WHERE indicator_code IN (
		                   'sdis.participation_departements', 'sdis.participation_communes',
		                   'sdis.participation_groupements', 'sdis.participation_regions',
		                   'sdis.concours_etat')) AS finance,
		                 max(montant) FILTER (WHERE indicator_code = 'sdis.recettes_totales') AS total
		            FROM core.collectivite_budget
		           WHERE niveau = 'SDIS'
		           GROUP BY code, exercice
		        ) t
		       WHERE t.total IS NOT NULL AND t.finance > t.total + 1`,
		sources: []string{"sdis"},
	},
	// Ordre de grandeur national connu (vérifié manuellement sur le fichier
	// OFGL au moment du chargement : 2,85 Md€ en 2023, 2,96 Md€ en 2024) —
	// une valeur hors de [1 Md€, 6 Md€] trahirait une erreur d'unité ou un
	// filtre cbudg oublié (les budgets annexes dédoubleraient la somme).
	{
		name: "SDIS : la participation départementale nationale reste dans un ordre de grandeur plausible",
		query: `SELECT count(*) FROM (
		          SELECT exercice, sum(montant) AS total
		            FROM core.collectivite_budget
		           WHERE niveau = 'SDIS' AND indicator_code = 'sdis.participation_departements'
		           GROUP BY exercice
		        ) t
		       WHERE t.total NOT BETWEEN 1e9 AND 6e9`,
		sources: []string{"sdis"},
	},
}
