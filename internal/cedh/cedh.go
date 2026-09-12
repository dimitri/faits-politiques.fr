// Package cedh ingère les arrêts de la Cour européenne des droits de l'homme
// concernant la France, depuis la base HUDOC.
//
// Principe non négociable : la Cour condamne un ÉTAT, jamais une personne. Rien
// ici ne rattache un arrêt à un responsable politique — ce serait une
// imputation, que la Cour ne formule nulle part et que ce site n'a pas à
// produire.
package cedh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Source = archive.Source{
	Slug: "cedh-hudoc", Label: "CEDH — HUDOC, arrêts concernant la France",
	Publisher:   "Cour européenne des droits de l'homme / Conseil de l'Europe",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Conditions HUDOC, à vérifier ; aucune licence ouverte explicite",
	ReuseClass:  "RESTRICTED",
	Attribution: "Source : Cour européenne des droits de l'homme, base HUDOC",
	Cadence:     "continue",
	Notes: "Chaque arrêt est publié en français ET en anglais sous deux identifiants " +
		"distincts partageant le même numéro de requête : dédoublonner, en préférant " +
		"la version française.",
}

const endpoint = "https://hudoc.echr.coe.int/app/query/results"

// Requête HUDOC : arrêts (JUDGMENTS) dont l'État défendeur est la France.
const requete = `contentsitename:ECHR AND ` +
	`(NOT (doctype=PR OR doctype=HFCOMOLD OR doctype=HECOMOLD)) AND ` +
	`((documentcollectionid2="JUDGMENTS")) AND (respondent="FRA")`

type ligne struct {
	Columns struct {
		ItemID        string `json:"itemid"`
		AppNo         string `json:"appno"`
		DocName       string `json:"docname"`
		DocTypeBranch string `json:"doctypebranch"`
		KPDate        string `json:"kpdate"`
		Article       string `json:"article"`
		Conclusion    string `json:"conclusion"`
		Importance    string `json:"importance"`
	} `json:"columns"`
}

// « Non-violation » contient « violation » : la détection doit donc porter sur
// un début de segment, pas sur une sous-chaîne. Un même arrêt peut retenir une
// violation sur un article et une non-violation sur un autre.
var reViolation = regexp.MustCompile(`(?i)(^|[;,]\s*)violation\b`)

func aRetenuUneViolation(conclusion string) bool {
	for _, seg := range strings.Split(conclusion, ";") {
		s := strings.TrimSpace(seg)
		if s == "" {
			continue
		}
		bas := strings.ToLower(s)
		if strings.HasPrefix(bas, "non-violation") || strings.HasPrefix(bas, "no violation") {
			continue
		}
		if strings.HasPrefix(bas, "violation") || reViolation.MatchString(s) {
			return true
		}
	}
	return false
}

// estFrancais distingue la version française de sa jumelle anglaise : les deux
// existent sous des identifiants différents pour le même numéro de requête.
func estFrancais(docname string) bool {
	return strings.HasPrefix(strings.ToUpper(docname), "AFFAIRE")
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, Source)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "cedh/1")
	if err != nil {
		return err
	}

	hc := &http.Client{Timeout: 90 * time.Second}
	const page = 500
	var toutes []ligne
	for start := 0; ; start += page {
		v := url.Values{
			"query":  {requete},
			"select": {"itemid,appno,docname,doctypebranch,kpdate,article,conclusion,importance"},
			"sort":   {"kpdate Ascending"},
			"start":  {fmt.Sprint(start)},
			"length": {fmt.Sprint(page)},
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+v.Encode(), nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "faits-politiques.fr (ingestion open data)")
		resp, err := hc.Do(req)
		if err != nil {
			arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
			return err
		}
		var out struct {
			ResultCount int     `json:"resultcount"`
			Results     []ligne `json:"results"`
		}
		err = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if err != nil {
			arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
			return err
		}
		toutes = append(toutes, out.Results...)
		if len(out.Results) < page || len(toutes) >= out.ResultCount {
			break
		}
		time.Sleep(600 * time.Millisecond) // ne pas marteler un service public
	}

	// Dédoublonnage par numéro de requête, version française préférée.
	parAppNo := map[string]ligne{}
	for _, l := range toutes {
		c := l.Columns
		if c.ItemID == "" || c.AppNo == "" {
			continue
		}
		cle := c.AppNo + "|" + c.KPDate
		prev, existe := parAppNo[cle]
		if !existe || (estFrancais(c.DocName) && !estFrancais(prev.Columns.DocName)) {
			parAppNo[cle] = l
		}
	}

	if _, err := pool.Exec(ctx, `DELETE FROM core.cedh_arret`); err != nil {
		return err
	}

	nTotal, nViol := 0, 0
	seen := map[string]bool{}
	for _, l := range parAppNo {
		c := l.Columns
		if len(c.KPDate) < 10 {
			continue
		}
		// Un slice Go nil devient NULL en SQL, pas un tableau vide : un arrêt
		// sans article renseigné doit porter un tableau vide, pas une absence.
		articles := []string{}
		for _, a := range strings.Split(c.Article, ";") {
			if a = strings.TrimSpace(a); a != "" && !strings.Contains(a, "-") {
				articles = append(articles, a)
			}
		}
		viol := aRetenuUneViolation(c.Conclusion)
		slug := slugify(c.AppNo + "-" + c.KPDate[:10])
		for i := 2; seen[slug]; i++ {
			slug = fmt.Sprintf("%s-%d", slugify(c.AppNo+"-"+c.KPDate[:10]), i)
		}
		seen[slug] = true

		if _, err := pool.Exec(ctx, `
			INSERT INTO core.cedh_arret
			  (itemid, appno, slug, titre, date_arret, formation, importance,
			   articles, conclusion, violation, url, source_id)
			VALUES ($1,$2,$3,$4,$5::date,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (itemid) DO NOTHING`,
			c.ItemID, c.AppNo, slug, c.DocName, c.KPDate[:10],
			nullifEmpty(c.DocTypeBranch), nullifEmpty(c.Importance),
			articles, c.Conclusion, viol,
			"https://hudoc.echr.coe.int/fre?i="+c.ItemID, srcID); err != nil {
			return fmt.Errorf("arrêt %s : %w", c.ItemID, err)
		}
		nTotal++
		if viol {
			nViol++
		}
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"arrets": nTotal, "avec_violation": nViol}, "")
	fmt.Printf("  CEDH          %d arrêts, dont %d retenant au moins une violation\n", nTotal, nViol)
	return nil
}

func nullifEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
