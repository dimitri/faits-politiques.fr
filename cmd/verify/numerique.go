package main

// Contrôles du dossier sur la souveraineté numérique
// (docs/souverainete-numerique.md).
func init() {
	checks = append(checks, checksNumerique...)
}

var checksNumerique = []check{
	{
		name: "SecNumCloud : le dernier catalogue de l'ANSSI compte au moins 15 services qualifiés",
		query: `SELECT count(*) FROM core.qualification_secnumcloud
		         WHERE catalogue_du = (SELECT max(catalogue_du) FROM core.qualification_secnumcloud)`,
		min: 15,
	},
	{
		// Une qualification expirée à la date du catalogue n'y figurerait plus :
		// sa présence trahirait une date mal lue (colonnes décalées).
		name: "SecNumCloud : aucune qualification échue à la date du catalogue",
		query: `SELECT count(*) FROM core.qualification_secnumcloud
		         WHERE date_fin < catalogue_du OR date_debut > catalogue_du`,
	},
	{
		// La CNIL publie le total des amendes de l'année dans son bilan annuel
		// (486 839 500 € pour 2025, publié le 9 février 2026). La somme des
		// montants lus dans la liste doit le retrouver à 0,5 % près : les
		// liquidations d'astreinte n'y sont pas comptées, et un libellé mal lu
		// (« 60 et 40 millions ») ferait un écart de plusieurs millions.
		name: "CNIL : les amendes 2025 lues retrouvent le total du bilan de la CNIL à 0,5 % près",
		query: `SELECT count(*) FROM (SELECT sum(montant_eur) s FROM core.sanction_cnil
		         WHERE date_decision >= '2025-01-01' AND date_decision < '2026-01-01') t
		         WHERE s IS NULL OR abs(s - 486839500) > 0.005 * 486839500`,
	},
	{
		name:  "CNIL : chaque année de 2011 à 2025 a au moins une sanction",
		query: `SELECT count(DISTINCT extract(year FROM date_decision)) FROM core.sanction_cnil WHERE date_decision < '2026-01-01'`,
		min:   15,
	},
	{
		// Aucune ré-identification (D-065) : la table ne contient que les
		// catégories publiées. Un nom de groupe suivi y trahirait un ajout.
		name: "CNIL : aucun organisme ré-identifié dans la liste chargée",
		query: `SELECT count(*) FROM core.sanction_cnil
		         WHERE organisme ~* '\m(google|amazon|microsoft|apple|facebook|meta|oracle|ibm|palantir)\M'`,
	},
	{
		name:  "SILL : le socle interministériel de logiciels libres est chargé",
		query: `SELECT count(*) FROM core.sill_logiciel`,
		min:   300,
	},
	{
		name:  "faits : les textes, constats et déclarations du dossier souveraineté sont chargés",
		query: `SELECT count(*) FROM ref.fait_dossier WHERE dossier = 'souverainete-numerique'`,
		min:   40,
	},
	{
		name: "contexte : les trois décrets d'attributions relus dans le corpus JORF sont chargés",
		query: `SELECT count(*) FROM ref.fait_dossier
		         WHERE dossier = 'souverainete-numerique' AND section = 'CONTEXTE' AND jo_texte_id IS NOT NULL`,
		min: 3,
	},
	{
		name:  "marchés informatiques : l'ensemble des DECP est lu, pas seulement les groupes suivis",
		query: `SELECT count(DISTINCT uid) FROM core.marche_numerique`,
		min:   50000,
	},
	{
		// Chaque marché est rangé dans un seul rattachement : la somme des
		// lignes de la vue doit retrouver le nombre de marchés distincts.
		name: "marchés informatiques : la vue par titulaire compte chaque marché une fois",
		query: `SELECT abs((SELECT sum(marches) FROM derived.marche_numerique_titulaire)
		                 - (SELECT count(DISTINCT uid) FROM core.marche_numerique))::int`,
	},
	{
		name: "marchés informatiques : la vue par produit compte chaque marché une fois",
		query: `SELECT abs((SELECT sum(marches) FROM derived.marche_numerique_produit)
		                 - (SELECT count(DISTINCT uid) FROM core.marche_numerique))::int`,
	},
	{
		// Les marchés de la sélection de l'évasion fiscale rattachés par SIREN
		// sont des marchés informatiques quand leur CPV l'est : les deux tables
		// doivent se recouper.
		name: "marchés informatiques : les marchés d'Oracle rattachés par SIREN dans le dossier fiscal sont retrouvés",
		query: `SELECT count(*) FROM core.marche_public_cible c
		         WHERE c.groupe = 'Oracle Corporation' AND c.correspondance = 'SIREN'
		           AND (c.code_cpv LIKE '48%' OR c.code_cpv LIKE '72%' OR c.code_cpv LIKE '302%')
		           AND NOT EXISTS (SELECT 1 FROM core.marche_numerique m WHERE m.uid = c.uid AND m.titulaire_id = c.titulaire_id)`,
	},
}
