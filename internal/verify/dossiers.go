package verify

// Contrôles des faits des dossiers et de la structure commune (D-066,
// perimetre.md § 2.8).
func init() {
	checks = append(checks, checksDossiers...)
}

var checksDossiers = []check{
	{
		name:  "dossiers : les faits sourcés de tous les dossiers sont chargés",
		query: `SELECT count(*) FROM ref.fait_dossier`,
		min:   170,
	},
	{
		name:  "dossiers : au moins vingt-huit dossiers ont des faits",
		query: `SELECT count(DISTINCT dossier) FROM ref.fait_dossier`,
		min:   28,
	},
	{
		// Chaque dossier doit dire dans quel cadre ses chiffres s'inscrivent.
		name: "dossiers : chaque dossier qui a des faits a au moins un texte ou un fait de cadre",
		query: `SELECT count(*) FROM (SELECT dossier FROM ref.fait_dossier GROUP BY dossier
		         HAVING count(*) FILTER (WHERE section = 'CADRE') = 0) t`,
	},
	{
		name: "dossiers : chaque fait non « presse » a une preuve archivée",
		query: `SELECT count(*) FROM ref.fait_dossier
		         WHERE qualite <> 'PRESSE' AND document_id IS NULL AND jo_texte_id IS NULL AND intervention_slug IS NULL`,
	},
	{
		// Le connecteur des comptes rendus recharge sa table : une prise de
		// parole citée qui disparaîtrait laisserait une citation sans preuve.
		name: "dossiers : chaque prise de parole citée existe dans les comptes rendus chargés",
		query: `SELECT count(*) FROM ref.fait_dossier f
		         WHERE f.intervention_slug IS NOT NULL
		           AND NOT EXISTS (SELECT 1 FROM core.intervention i WHERE i.slug = f.intervention_slug)`,
	},
	{
		// Un élu ou un ministre cité en séance a une fiche : sinon le lien
		// écrit dans le dossier serait mort.
		name: "dossiers : chaque personne citée en séance a une fiche sur le site",
		query: `SELECT count(*) FROM derived.fait_dossier_personne p JOIN ref.fait_dossier f ON f.id = p.fait_id
		         WHERE f.intervention_slug IS NOT NULL AND NOT p.a_une_fiche`,
	},
	{
		name:  "dossiers : les deux tables de faits partagent l'échelle de qualité (aucun « ENTREPRISE »)",
		query: `SELECT count(*) FROM ref.fait_multinationale WHERE qualite NOT IN (SELECT code FROM ref.qualite_fait)`,
	},
	{
		name:  "acteurs : les acteurs français du numérique sont chargés",
		query: `SELECT count(*) FROM ref.acteur_numerique`,
		min:   20,
	},
	{
		name: "acteurs : chaque acteur est une unité légale active",
		query: `SELECT count(*) FROM ref.acteur_numerique a LEFT JOIN ref.unite_legale u USING (siren)
		         WHERE u.siren IS NULL OR u.etat_administratif <> 'A'`,
	},
	{
		name:  "termes : chaque dossier suivi dans les débats a au moins une expression",
		query: `SELECT count(DISTINCT dossier) FROM ref.dossier_terme`,
		min:   15,
	},
	{
		name:  "missions : les dossiers verticaux suivent au moins quinze missions de l'État",
		query: `SELECT count(*) FROM ref.dossier_mission`,
		min:   15,
	},
	{
		// Le chargement refuse un motif sans correspondance ; ce contrôle
		// rattrape un rechargement du budget qui renommerait une mission.
		name: "missions : chaque mission suivie a des crédits dans chaque exercice chargé",
		query: `SELECT count(*) FROM ref.dossier_mission m
		         CROSS JOIN (SELECT DISTINCT exercice FROM core.budget_programme) e
		         WHERE NOT EXISTS (SELECT 1 FROM derived.dossier_budget_programme b
		                           WHERE b.dossier = m.dossier AND b.mission = m.libelle AND b.exercice = e.exercice)`,
	},
	{
		// Un tableau de crédits ne vaut que s'il couvre la mission entière :
		// la somme par programme doit retomber sur le total de la mission.
		name: "missions : les crédits par programme d'un dossier retombent sur le total de la mission",
		query: `SELECT count(*) FROM (
		         SELECT m.dossier, m.libelle, b.exercice, sum(b.credit_paiement) AS cp
		         FROM ref.dossier_mission m JOIN core.budget_programme b ON b.mission_libelle ~ m.motif
		         GROUP BY 1, 2, 3) t
		         JOIN (SELECT dossier, mission, exercice, sum(credit_paiement) AS cp
		               FROM derived.dossier_budget_programme GROUP BY 1, 2, 3) v
		           ON v.dossier = t.dossier AND v.mission = t.libelle AND v.exercice = t.exercice
		         WHERE abs(v.cp - t.cp) > 1`,
	},
}
