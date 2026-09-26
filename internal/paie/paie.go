// Package paie charge le barème d'un bulletin de paie — taux, plafonds,
// réduction générale — et, pour chaque prélèvement, l'organisme qui le reçoit
// et le budget dont il relève. Les vues de la migration 0085 recalculent un
// bulletin d'exemple et suivent chaque euro jusqu'à son destinataire.
// Voir docs/cotisations-et-droits.md § 3 bis et D-060.
package paie

import (
	"context"
	"fmt"
	"html"
	"os"
	"regexp"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "paie-v1"

const Millesime = "2026-01-01"

var SourceBaremes = archive.Source{
	Slug: "bareme-paie-2026", Label: "Barème de paie au 1er janvier 2026 : taux, plafond, réduction générale, destinataires",
	Publisher:  "DILA (service-public.fr) ; INSEE (comptes de la Nation)",
	Tier:       "PRIMARY_OFFICIAL",
	Licence:    "Pages publiques de l'administration, citées avec lien ; les taux sont des éléments de droit",
	ReuseClass: "ATTRIBUTION",
	Attribution: "Sources : service-public.fr (Entreprendre), fiches F24542, A17906, A15386 ; " +
		"Insee, Administrations publiques en 2025 (périmètre des administrations de sécurité sociale)",
	Cadence: "annuelle (1er janvier), avec changements en cours d'année",
	Notes: "Le barème de référence de l'URSSAF et Légifrance refusent l'accès automatisé : ils sont cités, " +
		"pas archivés. Les taux sans page archivée portent leur fondement en texte (décret, article, " +
		"barème URSSAF) et un document_id NULL. Les taux propres à un employeur (accidents du travail, " +
		"versement mobilité) ou à un foyer (impôt à la source) ne sont pas des taux de barème : ils " +
		"figurent dans les hypothèses du bulletin d'exemple.",
}

// pages archivées, et ce que chacune doit contenir pour être retenue.
var pages = []struct {
	cle, url string
	attendu  []string
}{
	{"rgdu", "https://entreprendre.service-public.gouv.fr/vosdroits/F24542",
		[]string{"T min = 0,0200", "T delta = 0,3781", "T delta = 0,3821", "21 876,40", "1,75", "6,01 %", "0,49 %", "arrondi à quatre décimales"}},
	{"ags", "https://entreprendre.service-public.gouv.fr/actualites/A17906", []string{"0,25 %"}},
	{"pass", "https://entreprendre.service-public.gouv.fr/actualites/A15386", []string{"48 060 €", "4 005 €"}},
	{"asso", "https://www.insee.fr/fr/statistiques/8988833",
		[]string{"le régime d'indemnisation du chômage", "(ARRCO, AGIRC", "(CADES)"}},
}

var (
	reBalises = regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<[^>]+>`)
	reBlancs  = regexp.MustCompile(`\s+`)
)

func texte(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	t := html.UnescapeString(reBalises.ReplaceAllString(string(b), " "))
	t = strings.NewReplacer(" ", " ", " ", " ", "’", "'").Replace(t)
	return reBlancs.ReplaceAllString(t, " "), nil
}

type organisme struct {
	code, nom, statut, budget, sousSecteur, texteBudget, fondement, joTitre, doc string
}

// Le classement budgétaire de chaque destinataire. Sous-secteur de
// comptabilité nationale : renseigné quand une source le dit.
var organismes = []organisme{
	{"DGFIP", "Direction générale des finances publiques", "administration de l'État", "ETAT", "S1311",
		"loi de finances (budget général de l'État)",
		"L'impôt sur le revenu, prélevé à la source par l'employeur et reversé à la DGFiP, est une recette du budget général de l'État.", "", ""},
	{"CNAM", "Caisse nationale de l'assurance maladie", "établissement public national à caractère administratif", "SECU_LFSS", "S1314",
		"loi de financement de la sécurité sociale",
		"Branche maladie du régime général (code de la sécurité sociale, art. L. 200-2) ; régime obligatoire de base, dans le champ de la LFSS.", "", "asso"},
	{"ATMP", "Branche accidents du travail et maladies professionnelles", "branche du régime général gérée par la CNAM", "SECU_LFSS", "S1314",
		"loi de financement de la sécurité sociale",
		"Branche AT-MP du régime général (CSS, art. L. 200-2) ; objectif de dépenses voté en LFSS.", "", "asso"},
	{"CNAV", "Caisse nationale d'assurance vieillesse", "établissement public national à caractère administratif", "SECU_LFSS", "S1314",
		"loi de financement de la sécurité sociale",
		"Branche vieillesse du régime général (CSS, art. L. 200-2).", "", "asso"},
	{"CNAF", "Caisse nationale des allocations familiales", "établissement public national à caractère administratif", "SECU_LFSS", "S1314",
		"loi de financement de la sécurité sociale",
		"Branche famille du régime général (CSS, art. L. 200-2).", "", "asso"},
	{"CNSA", "Caisse nationale de solidarité pour l'autonomie", "établissement public national à caractère administratif", "SECU_LFSS", "S1314",
		"loi de financement de la sécurité sociale",
		"Gestionnaire de la branche autonomie, cinquième branche de la sécurité sociale depuis la loi du 7 août 2020.", "", "asso"},
	{"CADES", "Caisse d'amortissement de la dette sociale", "établissement public national à caractère administratif", "SECU_LFSS", "S1314",
		"budget propre ; son objectif d'amortissement est voté en loi de financement de la sécurité sociale",
		"Rattachée aux administrations de sécurité sociale en comptabilité nationale ; reçoit la CRDS (ordonnance n° 96-50 du 24 janvier 1996).", "", "asso"},
	{"CSG", "Caisses de sécurité sociale bénéficiaires de la CSG", "impôt affecté : répartition fixée par la loi", "SECU_LFSS", "S1314",
		"loi de financement de la sécurité sociale",
		"La CSG est un impôt ; son produit est réparti par la loi entre organismes de sécurité sociale (CSS, art. L. 131-8 et L. 136-8). La clé change presque chaque année et n'est pas détaillée ici.", "", "asso"},
	{"UNEDIC", "Unédic (assurance chômage)", "association paritaire", "SECU_PARITAIRE", "S1314",
		"budget voté par son conseil d'administration paritaire, hors loi de financement ; règles d'indemnisation fixées par décret",
		"Le régime d'assurance chômage est compté parmi les administrations de sécurité sociale en comptabilité nationale, mais n'est pas dans le champ de la LFSS.", "", "asso"},
	{"AGIRC_ARRCO", "Agirc-Arrco (retraite complémentaire)", "fédération paritaire d'institutions de retraite complémentaire", "SECU_PARITAIRE", "S1314",
		"accords nationaux interprofessionnels et conseil d'administration paritaire, hors loi de financement",
		"Les régimes complémentaires obligatoires sont comptés parmi les administrations de sécurité sociale, hors champ de la LFSS.", "", "asso"},
	{"AGS", "AGS (garantie des salaires)", "association d'employeurs de droit privé", "PRIVE", "",
		"budget de l'association, taux fixé par son conseil d'administration",
		"Association créée par les organisations d'employeurs pour garantir les salaires en cas de procédure collective (code du travail, art. L. 3253-14). Son classement en comptabilité nationale n'est pas établi ici.", "", "ags"},
	{"FRANCE_COMPETENCES", "France compétences", "établissement public national à caractère administratif", "OPERATEUR_ETAT", "S1311",
		"budget propre voté par son conseil d'administration, sous tutelle de l'État",
		"Organisme divers d'administration centrale : inscrit à l'annexe 1 de l'arrêté du 29 août 2023 fixant la liste des ODAC.",
		"Arrêté du 29 août 2023 fixant la liste des organismes divers d'administration centrale%", ""},
	{"SOLDE_TA", "Établissements de formation choisis par l'employeur", "affectation choisie par l'employeur, versée via la Caisse des dépôts", "AUTRE", "",
		"aucun budget unique : lycées, universités, écoles, organismes habilités",
		"Le solde de la taxe d'apprentissage est réparti par l'employeur entre établissements habilités (code du travail, art. L. 6241-5).", "", ""},
	{"FNAL", "Fonds national d'aide au logement", "fonds sans personnalité morale", "FONDS_ETAT", "",
		"conseil de gestion placé sous l'autorité du ministre chargé du logement ; gestion financière par la Caisse des dépôts ; abondé par le programme 109 du budget de l'État",
		"Code de la construction et de l'habitation, art. R. 811-1 et suivants : le fonds finance les aides personnelles au logement versées par les CAF.", "", ""},
	{"AGFPN", "Association de gestion du fonds paritaire national", "association paritaire de droit privé", "PRIVE", "",
		"budget de l'association paritaire",
		"Reçoit la contribution au dialogue social qui finance les organisations syndicales et patronales (code du travail, art. L. 2135-10 et L. 2135-15).", "", ""},
	{"AOM", "Autorités organisatrices de la mobilité", "collectivités et groupements", "COLLECTIVITE", "S1313",
		"budget de la collectivité ou du syndicat de transport",
		"Versement mobilité dû par les employeurs d'au moins onze salariés dans le ressort d'une AOM qui l'a institué (code général des collectivités territoriales, art. L. 2333-64). Absent du bulletin d'exemple.", "", ""},
}

type parametre struct {
	code, libelle         string
	valeur                string
	unite, fondement, doc string
}

var parametres = []parametre{
	{"PMSS", "Plafond mensuel de la sécurité sociale", "4005", "EUR", "Arrêté fixant le plafond de la sécurité sociale pour 2026", "pass"},
	{"PASS", "Plafond annuel de la sécurité sociale", "48060", "EUR", "Arrêté fixant le plafond de la sécurité sociale pour 2026", "pass"},
	{"SMIC_HORAIRE", "Smic horaire brut", "12.02", "EUR", "Décret de revalorisation du Smic au 1er janvier 2026 ; repris par la fiche F24542", "rgdu"},
	{"SMIC_ANNUEL", "Smic calculé pour un an (1 820 heures)", "21876.40", "EUR", "Fiche F24542", "rgdu"},
	{"ABATTEMENT_CSG", "Abattement pour frais professionnels sur l'assiette CSG-CRDS", "1.75", "PCT", "CSS, art. L. 136-1-2", ""},
	{"RGDU_TMIN", "Réduction générale : Tmin", "0.0200", "COEF", "Décret n° 2025-1446 du 31 décembre 2025 ; fiche F24542", "rgdu"},
	{"RGDU_TDELTA_MOINS_50", "Réduction générale : Tdelta, moins de 50 salariés (Fnal 0,10 %)", "0.3781", "COEF", "Décret n° 2025-1446 ; fiche F24542", "rgdu"},
	{"RGDU_TDELTA_50_ET_PLUS", "Réduction générale : Tdelta, 50 salariés et plus (Fnal 0,50 %)", "0.3821", "COEF", "Décret n° 2025-1446 ; fiche F24542", "rgdu"},
	{"RGDU_P", "Réduction générale : exposant P", "1.75", "COEF", "Décret n° 2025-1446 ; fiche F24542", "rgdu"},
	{"RGDU_PLAFOND_IRC", "Réduction générale : part imputable sur la retraite complémentaire", "6.01", "PCT", "Fiche F24542 ; imputation = réduction × 0,0601 / coefficient maximal", "rgdu"},
	{"RGDU_PLAFOND_ATMP", "Réduction générale : part imputable sur les accidents du travail", "0.49", "PCT", "CSS, art. D. 241-2-4 ; fiche F24542", "rgdu"},
	{"TRIMESTRE_HEURES_SMIC", "Salaire validant un trimestre de retraite de base, en heures de Smic", "150", "HEURE", "CSS, art. R. 351-9", ""},
	{"AGIRC_ARRCO_TAUX_CALCUL_T1", "Agirc-Arrco : taux de calcul des points, tranche 1", "6.20", "PCT", "Accord national interprofessionnel du 17 novembre 2017 ; taux d'appel 127 %", ""},
	{"AGIRC_ARRCO_PRIX_ACHAT", "Agirc-Arrco : prix d'achat du point (salaire de référence)", "20.1877", "EUR", "Agirc-Arrco, paramètres 2026", ""},
	{"AGIRC_ARRCO_VALEUR_SERVICE", "Agirc-Arrco : valeur de service du point", "1.4386", "EUR", "Agirc-Arrco, valeur fixée au 1er novembre 2024, non revalorisée au 1er novembre 2025", ""},
}

type taux struct {
	code, part, libelle, rubrique string
	ordre                         int
	assiette, taux                string // taux vide : variable
	effMin                        int
	effMax                        int // 0 : sans limite
	organisme, nature             string
	deductible                    bool
	rgduGroupe, rgduTaux          string
	fondement, doc                string
}

const baremeURSSAF = "barème URSSAF des taux de cotisations au 1er janvier 2026"

var baremes = []taux{
	{"MALADIE", "EMPLOYEUR", "Sécurité sociale – maladie, maternité, invalidité, décès", "Santé", 10, "BRUT", "13.00", 0, 0, "CNAM", "MIXTE", true, "URSSAF", "13.00",
		"Taux unique depuis le 1er janvier 2026 : le taux réduit est intégré à la réduction générale ; " + baremeURSSAF, "rgdu"},
	{"ATMP", "EMPLOYEUR", "Accidents du travail – maladies professionnelles", "Accidents du travail – maladies professionnelles", 20, "BRUT", "", 0, 0, "ATMP", "DIFFERE", true, "URSSAF", "0.49",
		"Taux notifié à chaque établissement par la Carsat (CSS, art. D. 242-6-1 et suivants)", ""},
	{"VIEILLESSE_PLAFONNEE", "SALARIE", "Sécurité sociale – vieillesse plafonnée", "Retraite", 30, "TRANCHE_1", "6.90", 0, 0, "CNAV", "DIFFERE", true, "", "", baremeURSSAF, ""},
	{"VIEILLESSE_PLAFONNEE", "EMPLOYEUR", "Sécurité sociale – vieillesse plafonnée", "Retraite", 30, "TRANCHE_1", "8.55", 0, 0, "CNAV", "DIFFERE", true, "URSSAF", "8.55", baremeURSSAF, ""},
	{"VIEILLESSE_DEPLAFONNEE", "SALARIE", "Sécurité sociale – vieillesse déplafonnée", "Retraite", 31, "BRUT", "0.40", 0, 0, "CNAV", "DIFFERE", true, "", "", baremeURSSAF, ""},
	{"VIEILLESSE_DEPLAFONNEE", "EMPLOYEUR", "Sécurité sociale – vieillesse déplafonnée", "Retraite", 31, "BRUT", "2.11", 0, 0, "CNAV", "DIFFERE", true, "URSSAF", "2.11",
		"Décret n° 2025-1446 du 31 décembre 2025, art. 1er : 2,02 % → 2,11 %, en échange d'une baisse du taux AT-MP", ""},
	{"AGIRC_ARRCO_T1", "SALARIE", "Retraite complémentaire Agirc-Arrco – tranche 1", "Retraite", 32, "TRANCHE_1", "3.15", 0, 0, "AGIRC_ARRCO", "DIFFERE", true, "", "",
		"ANI du 17 novembre 2017 : 7,87 % (taux de calcul 6,20 % × taux d'appel 127 %), réparti 60/40", ""},
	{"AGIRC_ARRCO_T1", "EMPLOYEUR", "Retraite complémentaire Agirc-Arrco – tranche 1", "Retraite", 32, "TRANCHE_1", "4.72", 0, 0, "AGIRC_ARRCO", "DIFFERE", true, "IRC", "4.72",
		"ANI du 17 novembre 2017", ""},
	{"CEG_T1", "SALARIE", "Contribution d'équilibre général – tranche 1", "Retraite", 33, "TRANCHE_1", "0.86", 0, 0, "AGIRC_ARRCO", "DIFFERE", true, "", "",
		"ANI du 17 novembre 2017 : 2,15 %, n'ouvre aucun point", ""},
	{"CEG_T1", "EMPLOYEUR", "Contribution d'équilibre général – tranche 1", "Retraite", 33, "TRANCHE_1", "1.29", 0, 0, "AGIRC_ARRCO", "DIFFERE", true, "IRC", "1.29",
		"ANI du 17 novembre 2017", ""},
	{"FAMILLE", "EMPLOYEUR", "Allocations familiales", "Famille", 40, "BRUT", "5.25", 0, 0, "CNAF", "SOLIDARITE", true, "URSSAF", "5.25",
		"Taux unique depuis le 1er janvier 2026 (taux réduit intégré à la réduction générale) ; " + baremeURSSAF, "rgdu"},
	{"CHOMAGE", "EMPLOYEUR", "Assurance chômage", "Assurance chômage", 50, "BRUT", "4.00", 0, 0, "UNEDIC", "DIFFERE", true, "URSSAF", "4.00",
		"4,00 % depuis le 1er mai 2025, hors bonus-malus ; fiche F24542", "rgdu"},
	{"AGS", "EMPLOYEUR", "Garantie des salaires (AGS)", "Assurance chômage", 51, "BRUT", "0.25", 0, 0, "AGS", "DIFFERE", true, "", "",
		"Décision du conseil d'administration de l'AGS, maintien au 1er janvier 2026", "ags"},
	{"CSA", "EMPLOYEUR", "Contribution solidarité autonomie", "Autres contributions dues par l'employeur", 60, "BRUT", "0.30", 0, 0, "CNSA", "SOLIDARITE", true, "URSSAF", "0.30",
		"CSS, art. L. 137-40", ""},
	{"FNAL", "EMPLOYEUR", "Fonds national d'aide au logement", "Autres contributions dues par l'employeur", 61, "TRANCHE_1", "0.10", 0, 50, "FNAL", "SOLIDARITE", true, "URSSAF", "0.10",
		"CSS, art. L. 834-1 : 0,10 % sur la part plafonnée sous 50 salariés ; fiche F24542", "rgdu"},
	{"FNAL", "EMPLOYEUR", "Fonds national d'aide au logement", "Autres contributions dues par l'employeur", 61, "BRUT", "0.50", 50, 0, "FNAL", "SOLIDARITE", true, "URSSAF", "0.50",
		"CSS, art. L. 834-1 : 0,50 % sur la totalité à partir de 50 salariés", ""},
	{"FORMATION", "EMPLOYEUR", "Contribution à la formation professionnelle", "Autres contributions dues par l'employeur", 62, "BRUT", "0.55", 0, 11, "FRANCE_COMPETENCES", "IMPOT", true, "", "",
		"Code du travail, art. L. 6331-1 : 0,55 % sous 11 salariés", ""},
	{"FORMATION", "EMPLOYEUR", "Contribution à la formation professionnelle", "Autres contributions dues par l'employeur", 62, "BRUT", "1.00", 11, 0, "FRANCE_COMPETENCES", "IMPOT", true, "", "",
		"Code du travail, art. L. 6331-3 : 1 % à partir de 11 salariés", ""},
	{"TAXE_APPRENTISSAGE", "EMPLOYEUR", "Taxe d'apprentissage – part principale", "Autres contributions dues par l'employeur", 63, "BRUT", "0.59", 0, 0, "FRANCE_COMPETENCES", "IMPOT", true, "", "",
		"Code du travail, art. L. 6241-2 : 0,68 % dont 0,59 % pour le financement de l'apprentissage", ""},
	{"TAXE_APPRENTISSAGE_SOLDE", "EMPLOYEUR", "Taxe d'apprentissage – solde", "Autres contributions dues par l'employeur", 64, "BRUT", "0.09", 0, 0, "SOLDE_TA", "IMPOT", true, "", "",
		"Code du travail, art. L. 6241-2 et L. 6241-5 : 0,09 % affecté par l'employeur", ""},
	{"DIALOGUE_SOCIAL", "EMPLOYEUR", "Contribution au dialogue social", "Autres contributions dues par l'employeur", 65, "BRUT", "0.016", 0, 0, "AGFPN", "IMPOT", true, "", "",
		"Code du travail, art. L. 2135-10", ""},
	{"CSG_DEDUCTIBLE", "SALARIE", "CSG déductible de l'impôt sur le revenu", "CSG et CRDS", 80, "BRUT_ABATTU", "6.80", 0, 0, "CSG", "IMPOT", true, "", "",
		"CSS, art. L. 136-8 : 9,20 % sur les revenus d'activité, dont 6,80 points déductibles", ""},
	{"CSG_NON_DEDUCTIBLE", "SALARIE", "CSG non déductible de l'impôt sur le revenu", "CSG et CRDS", 81, "BRUT_ABATTU", "2.40", 0, 0, "CSG", "IMPOT", false, "", "",
		"CSS, art. L. 136-8 ; code général des impôts, art. 154 quinquies", ""},
	{"CRDS", "SALARIE", "CRDS non déductible de l'impôt sur le revenu", "CSG et CRDS", 82, "BRUT_ABATTU", "0.50", 0, 0, "CADES", "IMPOT", false, "", "",
		"Ordonnance n° 96-50 du 24 janvier 1996, art. 14", ""},
}

// Le bulletin d'exemple de la note. Personnes et entreprise factices.
var cas = []struct {
	cas, brut          string
	effectif           int
	atmp, pas, descrip string
}{
	{"technicienne-2500", "2500.00", 20, "1.50", "2.6",
		"Technicienne d'atelier non-cadre, CDI temps plein (151,67 h), 2 500 € brut, entreprise de 20 salariés de métropole hors Alsace-Moselle ; " +
			"dispensée de la mutuelle d'entreprise (couverte par celle de son conjoint) ; commune sans versement mobilité ; " +
			"taux AT-MP notifié 1,50 % et taux d'impôt personnalisé 2,6 % : hypothèses."},
}

func nul(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceBaremes)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	stats, err := charger(ctx, pool, arch, srcID, runID)
	if err != nil {
		err = fmt.Errorf("%s : %w", SourceBaremes.Slug, err)
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	arch.EndRun(ctx, runID, "SUCCESS", stats, "")
	fmt.Printf("  %-34s %v\n", SourceBaremes.Slug, stats)
	return nil
}

func charger(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, srcID, runID int64) (map[string]any, error) {
	docs := map[string]any{"": nil}
	for _, p := range pages {
		f, err := arch.Fetch(ctx, srcID, runID, p.url, ".html")
		if err != nil {
			return nil, err
		}
		t, err := texte(f.Path)
		if err != nil {
			return nil, err
		}
		// Une page qui ne dit plus ce qu'on lui fait dire ne peut pas fonder une ligne.
		for _, a := range p.attendu {
			if !strings.Contains(t, a) {
				return nil, fmt.Errorf("%s : « %s » absent de la page", p.url, a)
			}
		}
		docs[p.cle] = f.DocumentID
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM ref.bulletin_cas`); err != nil {
		return nil, err
	}
	// ref.organisme_social en upsert, pas en DELETE+INSERT : ref.taux_cotisation
	// (converti plus bas en MERGE, donc plus jamais vidé) porte une FK sur son
	// code, et un DELETE de la table entière échouerait tant que des taux la
	// référencent encore.
	for _, o := range organismes {
		var jo any
		if o.joTitre != "" {
			var id string
			if err := tx.QueryRow(ctx, `SELECT id FROM jo.texte WHERE titre_complet ILIKE $1 ORDER BY date_texte DESC LIMIT 1`, o.joTitre).Scan(&id); err != nil {
				return nil, fmt.Errorf("%s : texte du JO introuvable (%s) : %w", o.code, o.joTitre, err)
			}
			jo = id
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ref.organisme_social
			(code, nom, statut, budget, sous_secteur, texte_budget, fondement, jo_texte_id, source_id, document_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (code) DO UPDATE SET nom = EXCLUDED.nom, statut = EXCLUDED.statut, budget = EXCLUDED.budget,
			  sous_secteur = EXCLUDED.sous_secteur, texte_budget = EXCLUDED.texte_budget, fondement = EXCLUDED.fondement,
			  jo_texte_id = EXCLUDED.jo_texte_id, source_id = EXCLUDED.source_id, document_id = EXCLUDED.document_id`,
			o.code, o.nom, o.statut, o.budget, nul(o.sousSecteur), o.texteBudget, o.fondement, jo, srcID, docs[o.doc]); err != nil {
			return nil, fmt.Errorf("organisme %s : %w", o.code, err)
		}
	}
	// MERGE plutôt que DELETE+COPY, sur les deux tables : ce connecteur en est
	// l'unique propriétaire, et l'ancien DELETE (table entière) payait le prix
	// des triggers RI à chaque republication du barème, changement ou non.
	rows := [][]any{}
	for _, p := range parametres {
		rows = append(rows, []any{Millesime, p.code, p.libelle, p.valeur, p.unite, p.fondement, srcID, docs[p.doc]})
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_parametre_social (
			millesime date, code text, libelle text, valeur numeric, unite text, fondement text,
			source_id bigint, document_id bigint
		) ON COMMIT DROP`); err != nil {
		return nil, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_parametre_social"},
		[]string{"millesime", "code", "libelle", "valeur", "unite", "fondement", "source_id", "document_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO ref.parametre_social AS tgt
		USING tmp_parametre_social AS src
		ON tgt.millesime = src.millesime AND tgt.code = src.code
		WHEN MATCHED AND (tgt.libelle, tgt.valeur, tgt.unite, tgt.fondement, tgt.source_id, tgt.document_id)
		                  IS DISTINCT FROM
		                  (src.libelle, src.valeur, src.unite, src.fondement, src.source_id, src.document_id) THEN
		    UPDATE SET libelle = src.libelle, valeur = src.valeur, unite = src.unite, fondement = src.fondement,
		               source_id = src.source_id, document_id = src.document_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (millesime, code, libelle, valeur, unite, fondement, source_id, document_id)
		    VALUES (src.millesime, src.code, src.libelle, src.valeur, src.unite, src.fondement, src.source_id, src.document_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return nil, err
	}

	rows = rows[:0]
	for _, t := range baremes {
		var effMax any
		if t.effMax > 0 {
			effMax = t.effMax
		}
		rows = append(rows, []any{Millesime, t.code, t.part, t.libelle, t.rubrique, t.ordre, t.assiette, nul(t.taux), t.taux == "",
			t.effMin, effMax, t.organisme, t.nature, t.deductible, nul(t.rgduGroupe), nul(t.rgduTaux), t.fondement, srcID, docs[t.doc]})
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_taux_cotisation (
			millesime date, code text, part text, libelle text, rubrique text, ordre smallint, assiette text,
			taux numeric, variable boolean, effectif_min int, effectif_max int, organisme text, nature_droit text,
			deductible_ir boolean, rgdu_groupe text, rgdu_taux numeric, fondement text, source_id bigint,
			document_id bigint
		) ON COMMIT DROP`); err != nil {
		return nil, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_taux_cotisation"},
		[]string{"millesime", "code", "part", "libelle", "rubrique", "ordre", "assiette", "taux", "variable",
			"effectif_min", "effectif_max", "organisme", "nature_droit", "deductible_ir", "rgdu_groupe", "rgdu_taux",
			"fondement", "source_id", "document_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO ref.taux_cotisation AS tgt
		USING tmp_taux_cotisation AS src
		ON tgt.millesime = src.millesime AND tgt.code = src.code AND tgt.part = src.part
		   AND tgt.effectif_min = src.effectif_min
		WHEN MATCHED AND (tgt.libelle, tgt.rubrique, tgt.ordre, tgt.assiette, tgt.taux, tgt.variable,
		                   tgt.effectif_max, tgt.organisme, tgt.nature_droit, tgt.deductible_ir, tgt.rgdu_groupe,
		                   tgt.rgdu_taux, tgt.fondement, tgt.source_id, tgt.document_id)
		                  IS DISTINCT FROM
		                  (src.libelle, src.rubrique, src.ordre, src.assiette, src.taux, src.variable,
		                   src.effectif_max, src.organisme, src.nature_droit, src.deductible_ir, src.rgdu_groupe,
		                   src.rgdu_taux, src.fondement, src.source_id, src.document_id) THEN
		    UPDATE SET libelle = src.libelle, rubrique = src.rubrique, ordre = src.ordre, assiette = src.assiette,
		               taux = src.taux, variable = src.variable, effectif_max = src.effectif_max,
		               organisme = src.organisme, nature_droit = src.nature_droit, deductible_ir = src.deductible_ir,
		               rgdu_groupe = src.rgdu_groupe, rgdu_taux = src.rgdu_taux, fondement = src.fondement,
		               source_id = src.source_id, document_id = src.document_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (millesime, code, part, libelle, rubrique, ordre, assiette, taux, variable, effectif_min,
		            effectif_max, organisme, nature_droit, deductible_ir, rgdu_groupe, rgdu_taux, fondement,
		            source_id, document_id)
		    VALUES (src.millesime, src.code, src.part, src.libelle, src.rubrique, src.ordre, src.assiette, src.taux,
		            src.variable, src.effectif_min, src.effectif_max, src.organisme, src.nature_droit,
		            src.deductible_ir, src.rgdu_groupe, src.rgdu_taux, src.fondement, src.source_id, src.document_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return nil, err
	}

	for _, c := range cas {
		if _, err := tx.Exec(ctx, `INSERT INTO ref.bulletin_cas (cas, millesime, brut, effectif, taux_atmp, taux_pas, description)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`, c.cas, Millesime, c.brut, c.effectif, c.atmp, c.pas, c.descrip); err != nil {
			return nil, err
		}
	}
	return map[string]any{"organismes": len(organismes), "parametres": len(parametres), "taux": len(baremes), "cas": len(cas)},
		tx.Commit(ctx)
}
