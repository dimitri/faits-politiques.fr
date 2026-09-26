package fiscalite

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceDECP = archive.Source{
	Slug: "decp-consolidees", Label: "Données essentielles de la commande publique, consolidées (format tabulaire)",
	Publisher: "Acheteurs publics (arrêtés du 22 mars 2019 et du 22 décembre 2022), consolidation Colibre / decp.info",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : données essentielles de la commande publique, consolidées par decp.info (data.gouv.fr)",
	Cadence:     "quotidienne",
	Notes: "Données déclarées par chaque acheteur au moment de l'attribution ; consolidation de plusieurs " +
		"flux (plus exhaustive que le fichier du ministère des Finances). Obligation de publication depuis " +
		"2019, variable selon les acheteurs : les marchés antérieurs et une partie des marchés de défense " +
		"et de sécurité n'y figurent pas. Le montant d'un accord-cadre est son MAXIMUM ; un marché à " +
		"plusieurs titulaires répète son montant sur chaque ligne. Seule la dernière version de chaque " +
		"marché (donneesActuelles) est lue.",
}

const urlDECP = "https://www.data.gouv.fr/api/1/datasets/donnees-essentielles-de-la-commande-publique-consolidees-format-tabulaire/"

// Les groupes suivis dans les marchés publics. nom : motif sur la dénomination
// du titulaire ; objet : motif sur l'objet du marché, pour les achats de leurs
// produits via un revendeur. Les noms de groupe reprennent ceux de la sélection
// des filiales, pour que les vues se rejoignent.
var groupesMarches = []struct {
	groupe     string
	nom, objet string
	sirensHors []string // sociétés du groupe hors de la table des filiales (partenaires)
}{
	{"Microsoft Corporation", `\bmicrosoft\b`, `\bmicrosoft\b|\bazure\b|office ?365|\bm365\b`, nil},
	{"Alphabet Inc.", `\bgoogle\b`, `\bgoogle\b`, nil},
	// « AWS » seul désigne aussi Avenue Web Systèmes, éditeur de plateformes de
	// marchés publics, et « Amazon » seul des sociétés guyanaises nommées
	// d'après le fleuve : on exige la dénomination d'une société du groupe.
	{"Amazon.com Inc.", `\bamazon (web services|eu|france|online|data services|digital)\b`, `amazon web services|\bamazon (eu|business)\b`, nil},
	{"Oracle Corporation", `\boracle\b`, `\boracle\b`, nil},
	{"IBM", `\bibm\b|international business machines`, `\bibm\b`, nil},
	{"Salesforce Inc.", `\bsalesforce\b`, `\bsalesforce\b`, nil},
	{"Cisco Systems Inc.", `\bcisco\b`, `\bcisco\b`, nil},
	{"Apple Inc.", `\bapple\b`, `\bipad\b|\bmacbook\b|\bimac\b|\bapple\b`, nil},
	{"Palantir Technologies Inc.", `\bpalantir\b`, `\bpalantir\b`, nil},
	{"McKinsey & Company", `\bmckinsey\b`, `\bmckinsey\b`, nil},
	{"Accenture plc", `\baccenture\b`, `\baccenture\b`, nil},
	// Bleu n'est pas un groupe étranger : coentreprise d'Orange et de
	// Capgemini qui exploite sous licence les technologies de Microsoft. Suivie
	// à part, jamais agrégée à Microsoft.
	{"Bleu (Orange-Capgemini, technologies Microsoft)", "", "", []string{"953440591"}},
	// Capgemini est un groupe FRANÇAIS (société de tête à Paris) : suivi pour ses
	// marchés publics et son rôle dans les offres « cloud de confiance », jamais
	// compté parmi les groupes étrangers.
	{"Capgemini SE (groupe français)", `\bcapgemini\b|\bsogeti\b`, `\bcapgemini\b`,
		[]string{"330703844", "328781786", "479766842", "479766800", "444495774", "652025792", "434325973", "487607574"}},
	// S3NS : coentreprise de Thales et de Google Cloud, même logique que Bleu.
	{"S3NS (Thales-Google Cloud)", `\bs3ns\b`, `\bs3ns\b`, nil},
}

func IngestMarches(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	// Les SIREN des sociétés de chaque groupe, depuis la table des filiales.
	sirens := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT DISTINCT siren, groupe FROM core.filiale_groupe_etranger WHERE origine = 'SELECTION'`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var s, g string
		if err := rows.Scan(&s, &g); err != nil {
			return err
		}
		sirens[s] = g
	}
	rows.Close()
	if len(sirens) < 40 {
		return fmt.Errorf("sélection des filiales absente (charger -only=fiscalite-filiales)")
	}
	type motifs struct {
		groupe     string
		nom, objet *regexp.Regexp
	}
	var ms []motifs
	suivis := map[string]bool{}
	for _, g := range groupesMarches {
		suivis[g.groupe] = true
		m := motifs{groupe: g.groupe}
		if g.nom != "" {
			m.nom = regexp.MustCompile("(?i)" + g.nom)
		}
		if g.objet != "" {
			m.objet = regexp.MustCompile("(?i)" + g.objet)
		}
		ms = append(ms, m)
		for _, s := range g.sirensHors {
			sirens[s] = g.groupe
		}
	}

	return executer(ctx, arch, SourceDECP, func(srcID, runID int64) (map[string]any, error) {
		// L'URL du fichier du jour est lue dans la fiche du jeu de données.
		fiche, err := arch.Fetch(ctx, srcID, runID, urlDECP, ".json")
		if err != nil {
			return nil, err
		}
		urlCSV, err := ressourceDataGouv(fiche.Path, "decp.csv")
		if err != nil {
			return nil, err
		}
		f, err := arch.Fetch(ctx, srcID, runID, urlCSV, ".csv")
		if err != nil {
			return nil, err
		}
		var lignes [][]any
		lus, actuelles := 0, 0
		vus := map[string]bool{}
		par := map[string]int{}
		err = lireCSVFlux(f.Path, func(col map[string]int, rec []string) error {
			if lus == 0 {
				if err := exigerColonnes(col, "uid", "titulaire_id", "titulaire_typeIdentifiant", "titulaire_nom",
					"acheteur_id", "acheteur_nom", "objet", "montant", "nature", "techniques", "procedure", "codeCPV",
					"dateNotification", "dureeMois", "donneesActuelles", "montant_rationalise", "montant_anomalie",
					"acheteur_categorie", "sourceDataset"); err != nil {
					return err
				}
			}
			lus++
			v := func(k string) string { return rec[col[k]] }
			if v("donneesActuelles") != "true" {
				return nil
			}
			actuelles++
			tid, ttype := v("titulaire_id"), v("titulaire_typeIdentifiant")
			siren := ""
			if ttype == "SIRET" && len(tid) >= 9 {
				siren = tid[:9]
			}
			ajoute := func(groupe, corr string) {
				k := v("uid") + "|" + tid + "|" + groupe
				if vus[k] {
					return
				}
				vus[k] = true
				par[corr]++
				var date any
				if d, err := time.Parse("2006-01-02", v("dateNotification")); err == nil {
					date = d
				}
				lignes = append(lignes, []any{v("uid"), tid, nul(ttype), nul(v("titulaire_nom")), nul(siren), groupe, corr,
					nul(v("acheteur_id")), nul(v("acheteur_nom")), nul(v("acheteur_categorie")), nul(v("objet")),
					nul(v("nature")), nul(v("techniques")), nul(v("procedure")), nul(v("codeCPV")), date,
					nombre(v("dureeMois")), nombre(v("montant")), nombre(v("montant_rationalise")), nul(v("montant_anomalie")),
					nul(v("sourceDataset")), f.DocumentID})
			}
			// Du plus sûr au moins sûr ; un marché n'est rattaché qu'une fois
			// par groupe.
			if g, ok := sirens[siren]; ok && siren != "" && suivis[g] {
				ajoute(g, "SIREN")
				return nil
			}
			for _, m := range ms {
				if m.nom != nil && m.nom.MatchString(v("titulaire_nom")) {
					ajoute(m.groupe, "NOM")
					return nil
				}
			}
			for _, m := range ms {
				if m.objet != nil && m.objet.MatchString(v("objet")) {
					ajoute(m.groupe, "OBJET")
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if actuelles < 500000 {
			return nil, fmt.Errorf("%d marchés actuels seulement sur %d lignes", actuelles, lus)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE tmp_marche_public_cible (
				uid text, titulaire_id text, titulaire_type_id text, titulaire_nom text, siren text,
				groupe text, correspondance text, acheteur_id text, acheteur_nom text, acheteur_categorie text,
				objet text, nature text, techniques text, procedure text, code_cpv text,
				date_notification date, duree_mois numeric, montant_eur numeric, montant_rationalise numeric,
				montant_anomalie text, source_decp text, document_id bigint
			) ON COMMIT DROP`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_marche_public_cible"},
			[]string{"uid", "titulaire_id", "titulaire_type_id", "titulaire_nom", "siren", "groupe", "correspondance",
				"acheteur_id", "acheteur_nom", "acheteur_categorie", "objet", "nature", "techniques", "procedure", "code_cpv",
				"date_notification", "duree_mois", "montant_eur", "montant_rationalise", "montant_anomalie", "source_decp", "document_id"},
			pgx.CopyFromRows(lignes)); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			MERGE INTO core.marche_public_cible AS tgt
			USING tmp_marche_public_cible AS src
			     ON tgt.uid = src.uid AND tgt.titulaire_id = src.titulaire_id AND tgt.groupe = src.groupe
			WHEN MATCHED AND (tgt.titulaire_type_id, tgt.titulaire_nom, tgt.siren, tgt.correspondance,
			                   tgt.acheteur_id, tgt.acheteur_nom, tgt.acheteur_categorie, tgt.objet, tgt.nature,
			                   tgt.techniques, tgt.procedure, tgt.code_cpv, tgt.date_notification, tgt.duree_mois,
			                   tgt.montant_eur, tgt.montant_rationalise, tgt.montant_anomalie, tgt.source_decp,
			                   tgt.document_id)
			     IS DISTINCT FROM (src.titulaire_type_id, src.titulaire_nom, src.siren, src.correspondance,
			                        src.acheteur_id, src.acheteur_nom, src.acheteur_categorie, src.objet, src.nature,
			                        src.techniques, src.procedure, src.code_cpv, src.date_notification, src.duree_mois,
			                        src.montant_eur, src.montant_rationalise, src.montant_anomalie, src.source_decp,
			                        src.document_id)
			THEN UPDATE SET titulaire_type_id = src.titulaire_type_id, titulaire_nom = src.titulaire_nom,
			     siren = src.siren, correspondance = src.correspondance, acheteur_id = src.acheteur_id,
			     acheteur_nom = src.acheteur_nom, acheteur_categorie = src.acheteur_categorie, objet = src.objet,
			     nature = src.nature, techniques = src.techniques, procedure = src.procedure, code_cpv = src.code_cpv,
			     date_notification = src.date_notification, duree_mois = src.duree_mois, montant_eur = src.montant_eur,
			     montant_rationalise = src.montant_rationalise, montant_anomalie = src.montant_anomalie,
			     source_decp = src.source_decp, document_id = src.document_id
			WHEN NOT MATCHED BY TARGET THEN
			     INSERT (uid, titulaire_id, titulaire_type_id, titulaire_nom, siren, groupe, correspondance,
			             acheteur_id, acheteur_nom, acheteur_categorie, objet, nature, techniques, procedure, code_cpv,
			             date_notification, duree_mois, montant_eur, montant_rationalise, montant_anomalie,
			             source_decp, document_id)
			     VALUES (src.uid, src.titulaire_id, src.titulaire_type_id, src.titulaire_nom, src.siren, src.groupe,
			             src.correspondance, src.acheteur_id, src.acheteur_nom, src.acheteur_categorie, src.objet,
			             src.nature, src.techniques, src.procedure, src.code_cpv, src.date_notification, src.duree_mois,
			             src.montant_eur, src.montant_rationalise, src.montant_anomalie, src.source_decp, src.document_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
			return nil, fmt.Errorf("fusion marche_public_cible : %w", err)
		}
		return map[string]any{"lignes_lues": lus, "marches_actuels": actuelles, "retenues": len(lignes),
			"par_siren": par["SIREN"], "par_nom": par["NOM"], "par_objet": par["OBJET"]}, tx.Commit(ctx)
	})
}

func nombre(s string) any {
	s = strings.TrimSpace(s)
	if s == "" || s == "nan" || s == "NaN" {
		return nil
	}
	return s
}
