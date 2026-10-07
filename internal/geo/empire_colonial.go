package geo

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceEmpireColonial = archive.Source{
	Slug: "cshapes-empire-colonial", Label: "CShapes 2.0 — contours historiques, entités de l'empire colonial français",
	Publisher: "ETH Zürich (International Conflict Research)", Tier: "PRIMARY_OFFICIAL",
	Licence: "CC BY-NC-SA 4.0", ReuseClass: "OPEN",
	Attribution: "Source : CShapes 2.0, icr.ethz.ch/data/cshapes",
	Cadence:     "ponctuelle",
	Notes: "Le panel démarre au 1er janvier 1886 pour toutes les entités : les changements " +
		"de frontière antérieurs (dont la conquête de l'Algérie, 1830) n'y sont pas visibles. " +
		"Les dates de rattachement et le régime juridique (colonie, protectorat, mandat) " +
		"viennent d'une vérification séparée, territoire par territoire, sur Wikidata — voir " +
		"le commentaire de la migration 0128.",
}

const urlCShapes = "https://icr.ethz.ch/data/cshapes/CShapes-2.0.csv"

// territoireColonial décrit un territoire de l'empire colonial français :
// la géométrie et la date de fin viennent de CShapes (cshapesNom identifie
// la ligne à sélectionner par son nom de pays actuel et sa date de fin de
// période — la dernière période sous administration française selon ce
// jeu, pas nécessairement le jour exact de l'indépendance politique, voir
// noteIndependance) ; le reste est vérifié à la main, territoire par
// territoire (migration 0128).
type territoireColonial struct {
	territoire, region, regime         string
	anneeRattachement                  int
	noteRattachement                   string
	dateIndependance, noteIndependance string
	cshapesNom, cshapesFinPeriode      string
}

var territoiresColoniaux = []territoireColonial{
	{"Algérie", "Afrique du Nord", "colonie", 1830,
		"Conquête engagée avec la prise d'Alger (5 juillet 1830) ; statut de colonie, puis, à partir de 1848, un ensemble de départements français.",
		"1962-07-05", "Indépendance proclamée à l'issue des accords d'Évian, après le référendum d'autodétermination du 1er juillet 1962.",
		"Algeria", "1962-07-04"},
	{"Tunisie", "Afrique du Nord", "protectorat", 1881,
		"Protectorat établi par le traité du Bardo (12 mai 1881).",
		"1956-03-20", "Indépendance reconnue par le protocole franco-tunisien du 20 mars 1956.",
		"Tunisia", "1955-12-31"},
	{"Maroc", "Afrique du Nord", "protectorat", 1912,
		"Protectorat établi par le traité de Fès (30 mars 1912).",
		"1956-03-02", "Indépendance reconnue par la déclaration commune franco-marocaine du 2 mars 1956 (la zone espagnole du Nord et Tanger suivent séparément la même année).",
		"Morocco", "1956-03-01"},
	{"Sénégal", "Afrique de l'Ouest", "colonie", 1854,
		"Expansion coloniale à partir des comptoirs de Saint-Louis et Gorée sous le gouverneur Faidherbe, à partir de 1854.",
		"1960-08-20", "Indépendance proclamée le 20 juin 1960 dans le cadre de la Fédération du Mali (avec le Soudan français), pleinement autonome après la rupture de cette fédération le 20 août 1960.",
		"Senegal", "1959-04-03"},
	{"Mali (Soudan français)", "Afrique de l'Ouest", "colonie", 1880,
		"Colonie du Soudan français, organisée à partir des conquêtes menées depuis le Sénégal dans les années 1880.",
		"1960-09-22", "République du Mali proclamée le 22 septembre 1960, après la rupture de la Fédération du Mali.",
		"Mali", "1960-06-19"},
	{"Guinée", "Afrique de l'Ouest", "colonie", 1894,
		"Colonie distincte détachée du Sénégal en 1894.",
		"1958-10-02", "Seul territoire d'Afrique à voter « non » au référendum du 28 septembre 1958 sur la Communauté française — indépendance immédiate.",
		"Guinea", "1958-10-01"},
	{"Côte d'Ivoire", "Afrique de l'Ouest", "colonie", 1893,
		"Colonie créée en 1893.",
		"1960-08-07", "Indépendance le 7 août 1960.",
		"Cote D'Ivoire", "1960-08-06"},
	{"Dahomey (Bénin)", "Afrique de l'Ouest", "colonie", 1894,
		"Protectorat puis colonie à partir de 1894, après la chute du royaume d'Abomey.",
		"1960-08-01", "Indépendance le 1er août 1960.",
		"Benin", "1960-07-31"},
	{"Haute-Volta (Burkina Faso)", "Afrique de l'Ouest", "colonie", 1919,
		"Colonie créée en 1919 par démembrement de colonies voisines, supprimée en 1932 puis recréée en 1947.",
		"1960-08-05", "Indépendance le 5 août 1960.",
		"Burkina Faso (Upper Volta)", "1960-08-04"},
	{"Niger", "Afrique de l'Ouest", "colonie", 1922,
		"Colonie distincte à partir de 1922 (territoire militaire depuis 1900).",
		"1960-08-03", "Indépendance le 3 août 1960.",
		"Niger", "1960-08-02"},
	{"Mauritanie", "Afrique de l'Ouest", "colonie", 1904,
		"Protectorat puis colonie à partir de 1904, rattachée à l'Afrique-Occidentale française.",
		"1960-11-28", "Indépendance le 28 novembre 1960.",
		"Mauritania", "1960-11-27"},
	{"Gabon", "Afrique équatoriale", "colonie", 1886,
		"Colonie distincte au sein de l'Afrique-Équatoriale française à partir de 1886 (présence française sur l'estuaire depuis 1839).",
		"1960-08-17", "Indépendance le 17 août 1960.",
		"Gabon", "1960-08-16"},
	{"Congo (Moyen-Congo)", "Afrique équatoriale", "colonie", 1891,
		"Colonie du Moyen-Congo, au sein de l'Afrique-Équatoriale française.",
		"1960-08-15", "Indépendance le 15 août 1960 (République du Congo).",
		"Congo", "1960-08-14"},
	{"Tchad", "Afrique équatoriale", "colonie", 1900,
		"Colonie distincte à partir de 1900, rattachée à l'Afrique-Équatoriale française en 1910.",
		"1960-08-11", "Indépendance le 11 août 1960.",
		"Chad", "1960-08-10"},
	{"Oubangui-Chari (Centrafrique)", "Afrique équatoriale", "colonie", 1906,
		"Colonie distincte à partir de 1906, au sein de l'Afrique-Équatoriale française.",
		"1960-08-13", "Indépendance le 13 août 1960 (République centrafricaine).",
		"Central African Republic", "1960-08-12"},
	{"Cameroun", "Afrique équatoriale", "mandat puis tutelle", 1919,
		"Mandat de la Société des Nations confié à la France en 1919 (ancienne colonie allemande), converti en tutelle des Nations unies en 1946.",
		"1960-01-01", "Indépendance le 1er janvier 1960.",
		"Cameroon", "1959-12-31"},
	{"Madagascar", "Afrique de l'Est et océan Indien", "colonie", 1896,
		"Annexion en 1896, après un protectorat imposé en 1885 et la déposition de la reine Ranavalona III en 1897.",
		"1960-06-26", "Indépendance le 26 juin 1960.",
		"Madagascar (Malagasy)", "1960-06-25"},
	{"Comores", "Afrique de l'Est et océan Indien", "colonie", 1886,
		"Protectorat à partir de 1886, colonie rattachée administrativement à Madagascar de 1912 à 1946.",
		"1975-07-06", "Indépendance proclamée unilatéralement le 6 juillet 1975 par trois des quatre îles ; Mayotte, ayant voté contre en 1974, reste française.",
		"Comoros", "1975-07-05"},
	{"Djibouti", "Afrique de l'Est et océan Indien", "colonie", 1896,
		"Côte française des Somalis, colonie à partir de 1896 (premiers points d'appui dès 1862).",
		"1977-06-27", "Indépendance le 27 juin 1977 — dernier territoire africain à y accéder.",
		"Djibouti", "1977-06-26"},
	{"Cambodge", "Asie du Sud-Est", "protectorat", 1863,
		"Protectorat établi en 1863, intégré à l'Union indochinoise en 1887.",
		"1953-11-09", "Indépendance complète reconnue le 9 novembre 1953.",
		"Cambodia (Kampuchea)", "1953-11-08"},
	{"Laos", "Asie du Sud-Est", "protectorat", 1893,
		"Protectorat établi en 1893 (traité franco-siamois), intégré à l'Union indochinoise.",
		"1953-10-22", "Indépendance complète reconnue par le traité franco-laotien du 22 octobre 1953, tout en restant associé à l'Union française jusqu'en 1954.",
		"Laos", "1954-04-30"},
	{"Vietnam (Cochinchine, Annam, Tonkin)", "Asie du Sud-Est", "colonie puis protectorat", 1862,
		"Cochinchine cédée par le traité de Saïgon (1862), colonie française ; protectorats sur l'Annam et le Tonkin en 1883-1884 ; ensemble intégré à l'Union indochinoise en 1887.",
		"1954-07-21", "La fin de l'Indochine française est elle-même une question à plusieurs étapes (proclamation d'indépendance par Hô Chi Minh le 2 septembre 1945, État du Viêt Nam en 1949, partition actée par les accords de Genève du 21 juillet 1954) — traitée en détail dans le dossier dédié aux guerres de décolonisation. La date retenue ici est celle des accords de Genève.",
		"Vietnam (Annam/Cochin China/Tonkin)", "1954-04-30"},
}

// territoireIndochine1954 décrit l'une des deux entités nées de la
// partition du Viêt Nam actée par les accords de Genève (21 juillet 1954) :
// pas un territoire de l'empire colonial français (voir le commentaire de
// la migration 0151 pour pourquoi ce n'est pas une ligne de plus dans
// territoiresColoniaux ci-dessus), mais la même source CShapes, déjà
// téléchargée pour cette dernière — d'où le partage du fichier CSV et du
// run d'archive plutôt qu'une fonction d'ingestion séparée.
type territoireIndochine1954 struct {
	territoire, camp              string
	dateDebut, dateFin            string
	note                          string
	cshapesNom, cshapesFinPeriode string
}

var territoiresIndochine1954 = []territoireIndochine1954{
	{"République démocratique du Viêt Nam (Nord)", "nord", "1954-05-01", "1975-04-30",
		"Proclamée dès 1945 par Hô Chi Minh, reconnue comme la moitié nord du Viêt Nam " +
			"(nord du 17ᵉ parallèle) par les accords de Genève du 21 juillet 1954 — CShapes " +
			"situe le début de cette période au 1er mai 1954, avant la signature du texte. " +
			"Devient l'ensemble du Viêt Nam réunifié à la chute de Saïgon, le 30 avril 1975.",
		"Vietnam, Democratic Republic of", "1975-04-30"},
	{"République du Viêt Nam (Sud)", "sud", "1954-05-01", "1975-04-30",
		"Moitié sud du Viêt Nam (sud du 17ᵉ parallèle) après la partition de Genève, " +
			"héritière de l'État du Viêt Nam proclamé en 1949 ; cesse d'exister à la chute " +
			"de Saïgon, le 30 avril 1975, absorbée par la République démocratique du Viêt Nam.",
		"Vietnam, Republic of", "1975-04-30"},
}

// IngestTerritoireColonial charge la géographie (CShapes 2.0) des
// territoires de l'empire colonial français listés ci-dessus, à leur
// dernière période sous administration française, ainsi que — depuis le
// même fichier CSV — les deux entités de la partition du Viêt Nam de 1954
// (geo.indochine_partition_1954, table séparée, voir la migration 0151).
// Voir docs/empire-colonial-donnees.md et docs/guerres-decolonisation-donnees.md.
func IngestTerritoireColonial(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEmpireColonial)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "empire-colonial-v1")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, urlCShapes, ".csv")
	if err != nil {
		return fail(err)
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer file.Close()

	r := csv.NewReader(file)
	header, err := r.Read()
	if err != nil {
		return fail(fmt.Errorf("en-tête CShapes illisible : %w", err))
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	for _, must := range []string{"cntry_name", "gwedate", "gwsyear", "gweyear", "the_geom"} {
		if _, ok := col[must]; !ok {
			return fail(fmt.Errorf("colonne %q absente du CSV CShapes", must))
		}
	}

	// periode : une ligne CShapes pour un pays — une plage d'années, pas une
	// seule date. Nécessaire pour situer l'empire à une année FIXE (1900,
	// 1920...) plutôt qu'à la seule dernière période avant indépendance :
	// voir chargerExtensionEmpire ci-dessous.
	type periode struct {
		debut, fin int
		geom       string
	}
	geomParPeriode := map[[2]string]string{}
	periodesParPays := map[string][]periode{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(fmt.Errorf("ligne CShapes illisible : %w", err))
		}
		nom := rec[col["cntry_name"]]
		geomParPeriode[[2]string{nom, rec[col["gwedate"]]}] = rec[col["the_geom"]]
		debut, errD := strconv.Atoi(rec[col["gwsyear"]])
		fin, errF := strconv.Atoi(rec[col["gweyear"]])
		if errD == nil && errF == nil {
			periodesParPays[nom] = append(periodesParPays[nom], periode{debut, fin, rec[col["the_geom"]]})
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM geo.territoire_colonial`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM geo.indochine_partition_1954`); err != nil {
		return fail(err)
	}

	var manquants int
	for _, t := range territoiresColoniaux {
		if _, err := time.Parse("2006-01-02", t.dateIndependance); err != nil {
			return fail(fmt.Errorf("%s : date d'indépendance illisible : %w", t.territoire, err))
		}
		ewkt, ok := geomParPeriode[[2]string{t.cshapesNom, t.cshapesFinPeriode}]
		if !ok {
			manquants++
			fmt.Printf("  %s : aucune géométrie CShapes pour %q au %s\n", t.territoire, t.cshapesNom, t.cshapesFinPeriode)
			ewkt = ""
		}
		var geomExpr any
		if ewkt != "" {
			geomExpr = ewkt
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO geo.territoire_colonial
				(territoire, region, regime, annee_rattachement, note_rattachement,
				 date_independance, note_independance, geom, source_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, ST_Multi(ST_GeomFromEWKT($8)), $9)`,
			t.territoire, t.region, t.regime, t.anneeRattachement, t.noteRattachement,
			t.dateIndependance, t.noteIndependance, geomExpr, srcID); err != nil {
			return fail(fmt.Errorf("%s : insertion : %w", t.territoire, err))
		}
	}
	for _, p := range territoiresIndochine1954 {
		for _, d := range []string{p.dateDebut, p.dateFin} {
			if _, err := time.Parse("2006-01-02", d); err != nil {
				return fail(fmt.Errorf("%s : date illisible : %w", p.territoire, err))
			}
		}
		ewkt, ok := geomParPeriode[[2]string{p.cshapesNom, p.cshapesFinPeriode}]
		if !ok {
			manquants++
			fmt.Printf("  %s : aucune géométrie CShapes pour %q au %s\n", p.territoire, p.cshapesNom, p.cshapesFinPeriode)
			ewkt = ""
		}
		var geomExpr any
		if ewkt != "" {
			geomExpr = ewkt
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO geo.indochine_partition_1954
				(territoire, camp, date_debut, date_fin, note, geom, source_id)
			VALUES ($1, $2, $3, $4, $5, ST_Multi(ST_GeomFromEWKT($6)), $7)`,
			p.territoire, p.camp, p.dateDebut, p.dateFin, p.note, geomExpr, srcID); err != nil {
			return fail(fmt.Errorf("%s : insertion : %w", p.territoire, err))
		}
	}
	if manquants > 0 {
		return fail(fmt.Errorf("%d territoires sans géométrie CShapes — vérifier les noms/dates", manquants))
	}

	// L'empire à quatre dates fixes (voir la migration 0154) : pas la
	// dernière période avant indépendance comme ci-dessus, mais la période
	// CShapes qui couvre chaque année repère — un territoire absent d'une
	// carte donnée parce qu'il n'était pas encore français à cette date
	// (anneeRattachement > repère) ou déjà indépendant (repère >= année
	// d'indépendance), jamais parce que la donnée manquerait.
	if _, err := tx.Exec(ctx, `DELETE FROM geo.empire_colonial_extension`); err != nil {
		return fail(err)
	}
	var extensions int
	for _, anneeRepere := range []int{1900, 1920, 1938, 1946} {
		for _, t := range territoiresColoniaux {
			anneeIndep, err := strconv.Atoi(t.dateIndependance[:4])
			if err != nil {
				return fail(fmt.Errorf("%s : année d'indépendance illisible : %w", t.territoire, err))
			}
			if anneeRepere < t.anneeRattachement || anneeRepere >= anneeIndep {
				continue
			}
			var geom string
			for _, per := range periodesParPays[t.cshapesNom] {
				if anneeRepere >= per.debut && anneeRepere <= per.fin {
					geom = per.geom
					break
				}
			}
			if geom == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO geo.empire_colonial_extension (territoire, annee_repere, geom, source_id)
				VALUES ($1, $2, ST_Multi(ST_GeomFromEWKT($3)), $4)`,
				t.territoire, anneeRepere, geom, srcID); err != nil {
				return fail(fmt.Errorf("%s à %d : insertion extension : %w", t.territoire, anneeRepere, err))
			}
			extensions++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"territoires":              len(territoiresColoniaux),
		"indochine_partition_54":   len(territoiresIndochine1954),
		"extension_annees_reperes": extensions,
	}, "")
	fmt.Printf("  Empire colonial français : %d territoires (CShapes 2.0 + Wikidata, vérifiés territoire par territoire)\n", len(territoiresColoniaux))
	fmt.Printf("  Partition de l'Indochine, 1954 : %d entités (CShapes 2.0)\n", len(territoiresIndochine1954))
	fmt.Printf("  Extension à 1900/1920/1938/1946 : %d lignes territoire×année\n", extensions)
	return nil
}
