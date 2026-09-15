package numerique

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/fiscalite"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const urlDECP = "https://www.data.gouv.fr/api/1/datasets/donnees-essentielles-de-la-commande-publique-consolidees-format-tabulaire/"

// Les produits reconnus dans l'objet d'un marché, du plus spécifique au plus
// général (un marché « Azure et Oracle » compte pour Microsoft). « AWS » seul
// n'est retenu qu'avec un mot du Cloud : le sigle désigne aussi Avenue Web
// Systèmes, éditeur de plateformes de marchés publics. Les offres françaises
// d'hébergement sont reconnues aussi, pour comparer.
var produits = []struct {
	nom   string
	motif *regexp.Regexp
}{
	{"Microsoft", regexp.MustCompile(`(?i)\bmicrosoft\b|\bazure\b|office ?365|\bm365\b|\bwindows\b`)},
	{"Amazon Web Services", regexp.MustCompile(`(?i)amazon web services|\baws\b.*\b(cloud|h[ée]bergement)|\b(cloud|h[ée]bergement)\b.*\baws\b`)},
	{"Google", regexp.MustCompile(`(?i)\bgoogle\b`)},
	{"Oracle", regexp.MustCompile(`(?i)\boracle\b`)},
	{"SAP", regexp.MustCompile(`\bSAP\b`)}, // en capitales : « sap » minuscule est souvent un fragment
	{"VMware (Broadcom)", regexp.MustCompile(`(?i)\bvmware\b|\bbroadcom\b`)},
	{"IBM", regexp.MustCompile(`(?i)\bibm\b`)},
	{"Adobe", regexp.MustCompile(`(?i)\badobe\b`)},
	{"Salesforce", regexp.MustCompile(`(?i)\bsalesforce\b`)},
	{"Cisco", regexp.MustCompile(`(?i)\bcisco\b`)},
	{"ServiceNow", regexp.MustCompile(`(?i)\bservicenow\b`)},
	{"Palantir", regexp.MustCompile(`(?i)\bpalantir\b`)},
	{"Apple", regexp.MustCompile(`(?i)\bapple\b|\bipad\b|\bmacbook\b|\bimac\b`)},
	{"OVHcloud", regexp.MustCompile(`(?i)\bovh(cloud)?\b`)},
	{"Outscale", regexp.MustCompile(`(?i)\boutscale\b`)},
	{"Scaleway", regexp.MustCompile(`(?i)\bscaleway\b`)},
	{"Logiciel libre", regexp.MustCompile(`(?i)logiciels? libres?|\bopen ?source\b|\blinux\b|libre ?office`)},
}

var reHebergement = regexp.MustCompile(`(?i)\b(cloud|nuage|iaas|paas|saas|h[ée]bergement|datacenter|data ?center|centre de donn[ée]es)\b`)

func IngestMarches(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, arch, fiscalite.SourceDECP, func(srcID, runID int64) (map[string]any, error) {
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
		vus := map[string]bool{}
		lus, actuels, nommes := 0, 0, 0
		err = lireCSVFlux(f.Path, func(col map[string]int, rec []string) error {
			if lus == 0 {
				for _, c := range []string{"uid", "titulaire_id", "titulaire_typeIdentifiant", "titulaire_nom", "acheteur_id",
					"acheteur_nom", "acheteur_categorie", "objet", "codeCPV", "nature", "techniques", "dateNotification",
					"montant", "montant_rationalise", "montant_anomalie", "donneesActuelles"} {
					if _, ok := col[c]; !ok {
						return fmt.Errorf("colonne %q absente", c)
					}
				}
			}
			lus++
			v := func(k string) string { return rec[col[k]] }
			if v("donneesActuelles") != "true" {
				return nil
			}
			actuels++
			// Logiciels (48), services informatiques (72), matériel informatique
			// (302 : ordinateurs, périphériques, pièces).
			cpv := v("codeCPV")
			if !(strings.HasPrefix(cpv, "48") || strings.HasPrefix(cpv, "72") || strings.HasPrefix(cpv, "302")) {
				return nil
			}
			tid := v("titulaire_id")
			k := v("uid") + "|" + tid
			if vus[k] {
				return nil
			}
			vus[k] = true
			ttype := v("titulaire_typeIdentifiant")
			siren := ""
			if strings.EqualFold(ttype, "SIRET") && len(tid) >= 9 && estChiffres(tid[:9]) {
				siren = tid[:9]
			}
			objet := v("objet")
			var produit any
			for _, p := range produits {
				if p.motif.MatchString(objet) {
					produit = p.nom
					nommes++
					break
				}
			}
			// Stockage (72317), hébergement de sites (724, 72415) ou mot du
			// Cloud dans l'objet.
			heberge := strings.HasPrefix(cpv, "72317") || strings.HasPrefix(cpv, "724") || reHebergement.MatchString(objet)
			var date any
			if d, err := time.Parse("2006-01-02", v("dateNotification")); err == nil {
				date = d
			}
			lignes = append(lignes, []any{v("uid"), tid, nul(ttype), nul(v("titulaire_nom")), nul(siren),
				nul(v("acheteur_id")), nul(v("acheteur_nom")), nul(v("acheteur_categorie")), nul(objet), cpv,
				nul(v("nature")), nul(v("techniques")), date, nombre(v("montant")), nombre(v("montant_rationalise")),
				nul(v("montant_anomalie")), produit, heberge, f.DocumentID})
			return nil
		})
		if err != nil {
			return nil, err
		}
		if len(lignes) < 50000 {
			return nil, fmt.Errorf("%d marchés informatiques seulement sur %d marchés actuels", len(lignes), actuels)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM core.marche_numerique`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "marche_numerique"},
			[]string{"uid", "titulaire_id", "titulaire_type_id", "titulaire_nom", "siren", "acheteur_id", "acheteur_nom",
				"acheteur_categorie", "objet", "code_cpv", "nature", "techniques", "date_notification", "montant_eur",
				"montant_rationalise", "montant_anomalie", "produit_nomme", "hebergement", "document_id"},
			pgx.CopyFromRows(lignes)); err != nil {
			return nil, err
		}
		return map[string]any{"lignes_lues": lus, "marches_actuels": actuels, "informatiques": len(lignes),
			"produit_nomme": nommes}, tx.Commit(ctx)
	})
}

func estChiffres(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func nombre(s string) any {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "nan") {
		return nil
	}
	return s
}
