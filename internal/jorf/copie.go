package jorf

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Chargement de la base complète du Journal officiel par le protocole COPY.
//
// LE PROBLÈME. L'archive fait 1,1 Go compressé et contient 1,24 million de
// fichiers XML de deux natures : les SOMMAIRES (JORFCONT), qui décrivent un
// numéro du Journal officiel et listent les textes qu'il contient, et les
// TEXTES (JORFTEXT), qui portent l'acte lui-même et ses articles. Un chargement
// ligne à ligne est hors de question ; COPY est la seule voie raisonnable.
//
// Or COPY monopolise une connexion : on ne peut pas alimenter plusieurs tables
// en même temps sur la même. Trois façons d'en sortir, et la troisième est la
// bonne :
//
//  1. UNE SEULE TABLE, large, avec un discriminant de nature. Un seul flux, le
//     débit maximal — mais on écrit dans la base une forme qu'on devra
//     démêler ensuite, et toutes les lignes portent toutes les colonnes.
//  2. PLUSIEURS PASSES, une par table. Chaque passe est simple ; chacune coûte
//     une décompression complète de l'archive. Le prix est linéaire en nombre
//     de tables.
//  3. UNE SEULE TRAVERSÉE, PLUSIEURS FLUX COPY EN PARALLÈLE, une connexion par
//     table cible. C'est ce qui est fait ici.
//
// ET L'ORDRE DES COMMIT ? Il n'existe pas, parce qu'il n'y a rien à ordonner :
// les tables d'atterrissage n'ont ni clé étrangère ni index. Un lien vers un
// texte qui n'est pas encore chargé n'est pas une violation, c'est une ligne.
// La structure référentielle — les clés, les contraintes, les rejets — est
// bâtie APRÈS, par des INSERT ... SELECT à l'intérieur de la base, où l'ordre
// des dépendances se lit dans le SQL et non dans l'ordonnancement de goroutines.
//
// C'est le même principe que partout ailleurs dans ce dépôt : `raw` accueille
// ce que la source dit, `core` accueille ce qui a été vérifié.

// Le tampon entre le lecteur de l'archive et les écrivains. Il absorbe les
// à-coups : un fichier de sommaire est minuscule, un texte de loi de finances
// pèse plusieurs mégaoctets, et les deux arrivent dans le même flux.
const tampon = 4096

// flux relie une table cible à son canal d'alimentation. pgx.CopyFrom bloque
// jusqu'à épuisement de la source, d'où une goroutine et une connexion par
// table.
type flux struct {
	table    pgx.Identifier
	colonnes []string
	lignes   chan []any
	n        int64
	err      error
}

func nouveauFlux(schema, table string, colonnes ...string) *flux {
	return &flux{
		table:    pgx.Identifier{schema, table},
		colonnes: colonnes,
		lignes:   make(chan []any, tampon),
	}
}

// source adapte un canal à l'interface que pgx attend. Elle ne met JAMAIS tout
// en mémoire : pgx tire une ligne à la fois, au rythme du réseau.
type source struct {
	c       chan []any
	courant []any
	n       *int64
}

func (s *source) Next() bool {
	v, ok := <-s.c
	if !ok {
		return false
	}
	s.courant = v
	atomic.AddInt64(s.n, 1)
	return true
}
func (s *source) Values() ([]any, error) { return s.courant, nil }
func (s *source) Err() error             { return nil }

// demarrer ouvre une connexion dédiée et y lance un COPY qui durera toute la
// traversée de l'archive.
func (f *flux) demarrer(ctx context.Context, pool *pgxpool.Pool, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := pool.Acquire(ctx)
		if err != nil {
			f.err = err
			// Le canal doit être vidé malgré tout, sinon le lecteur de
			// l'archive se bloque sur un canal plein et le programme fige.
			for range f.lignes {
			}
			return
		}
		defer conn.Release()
		_, f.err = conn.CopyFrom(ctx, f.table, f.colonnes, &source{c: f.lignes, n: &f.n})
		if f.err != nil {
			for range f.lignes {
			}
		}
	}()
}

// Les deux natures de fichier, décodées séparément : elles n'ont en commun que
// l'identifiant et la date de publication.
type sommaireJO struct {
	ID        string `xml:"ID"`
	Nature    string `xml:"NATURE"`
	Titre     string `xml:"TITRE"`
	Num       string `xml:"NUM"`
	DatePubli string `xml:"DATE_PUBLI"`
	Liens     []struct {
		IDTxt    string `xml:"idtxt,attr"`
		TitreTxt string `xml:"titretxt,attr"`
	} `xml:"STRUCTURE_TXT>LIEN_TXT"`
}

// IngestComplet télécharge la base complète du Journal officiel, la scelle, et
// la charge en entier dans le schéma jo.
//
// Idempotent : les tables sont vidées avant chargement, et l'archive n'est
// téléchargée qu'une fois — archive.Fetch la reconnaît à son empreinte.
func IngestComplet(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, Source)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	f, err := arch.Fetch(ctx, srcID, runID, baseGlobale, ".tar.gz")
	if err != nil {
		return fail(err)
	}

	// TOUT CE QUI VÉRIFIE PART LE TEMPS DU CHARGEMENT.
	//
	// La clé étrangère, parce que les quatre tables sont remplies en parallèle
	// et qu'un bloc peut arriver avant son acte : la vérifier au fil de l'eau
	// imposerait un ordre entre les flux, donc de les sérialiser. La vérifier à
	// la fin coûte quatre secondes et contrôle tout d'un coup.
	//
	// Les index de recherche, eux, n'ont rien à retirer : ils ne sont pas sur
	// ces tables. Ils portent sur des VUES MATÉRIALISÉES, rafraîchies après le
	// chargement (migration 0063). C'est ce qui permet à COPY d'écrire à plein
	// débit sans qu'aucun index ne soit maintenu ligne à ligne — mesuré, un GIN
	// présent pendant l'insertion coûte 8 % de plus que le même construit en
	// bloc à la fin.
	for _, q := range []string{
		`ALTER TABLE jo.bloc DROP CONSTRAINT IF EXISTS bloc_texte_fk`,
		`TRUNCATE jo.sommaire, jo.lien, jo.texte, jo.bloc`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fail(err)
		}
	}

	stats, err := CopierArchive(ctx, pool, f.Path, "jo")
	if err != nil {
		return fail(err)
	}

	if _, err := pool.Exec(ctx, `
		ALTER TABLE jo.bloc ADD CONSTRAINT bloc_texte_fk
		  FOREIGN KEY (texte_id) REFERENCES jo.texte(id) ON DELETE CASCADE`); err != nil {
		return fail(fmt.Errorf("remise de la clé étrangère : %w", err))
	}

	// Le vecteur de recherche est calculé ici, en rafraîchissant les vues.
	// L'expression `to_tsvector('fr', …)` n'est écrite nulle part dans ce
	// fichier : elle est dans la définition des vues, pour qu'il n'en existe
	// qu'une seule version. Écrite deux fois, elle finirait par différer — et un
	// vecteur calculé avec une configuration puis interrogé avec une autre ne
	// rend rien, sans erreur.
	//
	// maintenance_work_mem est relevé explicitement : la valeur par défaut est
	// de 64 Mo, et la construction d'un index GIN s'en accommode mal à mesure
	// que le corpus grandit.
	debutVues := time.Now()
	if _, err := pool.Exec(ctx, `SET maintenance_work_mem = '1GB'`); err != nil {
		return fail(err)
	}
	for _, v := range []string{"jo.recherche_texte", "jo.recherche_bloc"} {
		if _, err := pool.Exec(ctx, "REFRESH MATERIALIZED VIEW "+v); err != nil {
			return fail(fmt.Errorf("rafraîchissement de %s : %w", v, err))
		}
	}
	vues := int64(time.Since(debutVues).Seconds())

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"fichiers": stats["fichiers"], "sommaires": stats["sommaire"],
		"liens": stats["lien"], "actes": stats["texte"], "blocs": stats["bloc"],
		"secondes": stats["secondes"], "recherche_s": vues}, "")
	logs.Notice(fmt.Sprintf("official gazette: %s read in %ds",
		logs.Plural(int(stats["fichiers"]), "file"), stats["secondes"]))
	logs.Notice(fmt.Sprintf("%s, %s, %s, %s", logs.Plural(int(stats["sommaire"]), "summary"),
		logs.Plural(int(stats["lien"]), "link"), logs.Plural(int(stats["texte"]), "act"),
		logs.Plural(int(stats["bloc"]), "block")))
	logs.Notice(fmt.Sprintf("search views refreshed in %ds", vues))
	return nil
}

// CopierArchive traverse l'archive une fois et alimente quatre tables en
// parallèle. Elle rend le nombre de lignes écrites par table.
func CopierArchive(ctx context.Context, pool *pgxpool.Pool, chemin, schema string) (map[string]int64, error) {
	fh, err := os.Open(chemin)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	fSommaire := nouveauFlux(schema, "sommaire", "id", "nature", "titre", "num", "date_publi")
	fLien := nouveauFlux(schema, "lien", "sommaire_id", "texte_id", "titre", "ordre")
	fTexte := nouveauFlux(schema, "texte", "id", "nature", "num", "nor", "date_publi",
		"date_texte", "titre", "titre_complet", "ministere", "origine_publi")
	fBloc := nouveauFlux(schema, "bloc", "texte_id", "ordre", "section", "article_id",
		"article_num", "contenu")
	flux := []*flux{fSommaire, fLien, fTexte, fBloc}

	var wg sync.WaitGroup
	for _, f := range flux {
		f.demarrer(ctx, pool, &wg)
	}

	// Instrumentation : JORF_ETAPE borne le travail pour mesurer où passe le
	// temps. walk = décompression et parcours seuls ; meta = plus le décodage
	// des métadonnées ; texte = plus l'écriture des actes ; vide = tout.
	etape := os.Getenv("JORF_ETAPE")

	// L'ANALYSE EST RÉPARTIE SUR UN POOL. La mesure a montré où passe le temps :
	// la décompression et le parcours de l'archive coûtent 117 s sur 1 497, et
	// encoding/xml le reste. Sur ce processeur, la seule tokenisation — boucler
	// sur d.Token() sans rien construire — plafonne à 20 Mo/s et alloue 2,6
	// millions de fois pour 17,6 Mo, soit une allocation tous les sept octets.
	//
	// Le parcours de l'archive, lui, ne se parallélise pas : gzip est un flux
	// strictement séquentiel. L'analyse, si — et c'est elle qui domine. Le
	// lecteur ne fait donc plus que lire et distribuer ; N ouvriers décodent.
	//
	// L'ORDRE DE SORTIE N'EST PLUS GARANTI, et c'est sans conséquence : les
	// tables d'atterrissage n'ont ni clé étrangère ni index, et une ligne de
	// lien qui précède le texte qu'elle désigne n'est pas une violation. C'est
	// la même propriété qui dispensait déjà d'ordonner les COMMIT.
	nOuvriers := runtime.NumCPU()
	if v := os.Getenv("JORF_OUVRIERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			nOuvriers = n
		}
	}
	type tache struct {
		base string
		raw  []byte
	}
	taches := make(chan tache, nOuvriers*8)
	var wgO sync.WaitGroup
	for i := 0; i < nOuvriers; i++ {
		wgO.Add(1)
		go func() {
			defer wgO.Done()
			for t := range taches {
				analyser(t.base, t.raw, etape, fSommaire, fLien, fTexte, fBloc)
			}
		}()
	}

	debut := time.Now()
	var nFichiers int64
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg || !strings.HasSuffix(h.Name, ".xml") {
			continue
		}
		raw, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("lecture de %s : %w", h.Name, err)
		}
		nFichiers++

		// Le routage se fait sur le NOM DE BASE, pas sur le chemin. Un texte est
		// rangé DANS le répertoire de son sommaire :
		//
		//   …/JORFCONT000000016676/JORFTEXT000000339141.xml
		//
		// Un `strings.Contains(h.Name, "JORFCONT")` est donc vrai pour les deux,
		// et il l'était d'abord : les 1 236 284 textes sont partis dans la table
		// des sommaires, où ils se sont décodés sans erreur — un JORFTEXT a lui
		// aussi une balise <ID>. Seul le compteur resté à zéro l'a dit.
		base := h.Name
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		if etape == "walk" {
			continue
		}
		taches <- tache{base: base, raw: raw}
	}
	close(taches)
	wgO.Wait()
	for _, f := range flux {
		close(f.lignes)
	}
	wg.Wait()

	out := map[string]int64{"fichiers": nFichiers, "secondes": int64(time.Since(debut).Seconds())}
	for _, f := range flux {
		if f.err != nil {
			return out, fmt.Errorf("%s : %w", f.table.Sanitize(), f.err)
		}
		out[f.table[len(f.table)-1]] = f.n
	}
	return out, nil
}

// analyser décode un fichier et pousse ses lignes vers les flux. Elle est
// appelée depuis plusieurs goroutines : elle ne partage rien, les canaux
// faisant la synchronisation.
func analyser(base string, raw []byte, etape string, fSommaire, fLien, fTexte, fBloc *flux) {
	switch {
	case strings.HasPrefix(base, "JORFCONT"):
		var s sommaireJO
		if err := decoderDans(raw, &s); err != nil || s.ID == "" {
			return
		}
		fSommaire.lignes <- []any{s.ID, nul(s.Nature), nul(s.Titre), nul(s.Num), dateJO(s.DatePubli)}
		for i, l := range s.Liens {
			if l.IDTxt == "" {
				continue
			}
			fLien.lignes <- []any{s.ID, l.IDTxt, nul(l.TitreTxt), i + 1}
		}

	case strings.HasPrefix(base, "JORFTEXT"):
		t, err := decoder(raw)
		if err != nil || t.ID == "" {
			return
		}
		if etape == "meta" {
			return
		}
		fTexte.lignes <- []any{t.ID, nul(t.Nature), nul(t.Num), nul(t.NOR),
			dateJO(t.DatePubli), dateJO(t.DateTexte), nul(t.Titre), nul(t.TitreFull),
			nul(t.Ministere), nul(t.OriginePubli)}
		if etape == "texte" {
			return
		}
		for i, b := range blocs(raw) {
			fBloc.lignes <- []any{t.ID, i + 1, b.section, nul(b.articleID), nul(b.articleNum), b.contenu}
		}
	}
}

func decoderDans(raw []byte, v any) error {
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	return d.Decode(v)
}

// bloc est un morceau de texte de l'acte, avec l'endroit d'où il vient.
//
// La SECTION importe autant que le contenu. Le Journal officiel range son texte
// en <NOTICE> (à qui s'adresse le texte), <VISAS> (les fondements juridiques),
// <STRUCT><ARTICLE> (le dispositif, seul à édicter quelque chose), <ABRO> (les
// abrogations) et <SM> (les signataires). Les confondre ferait entrer dans le
// dispositif des phrases que l'acte n'édicte pas — l'extraction de corps() les
// écarte justement, mais pour archiver il vaut mieux tout garder ET dire d'où
// ça vient : c'est `raw`, on n'y jette rien.
type blocTexte struct {
	section    string
	articleID  string
	articleNum string
	contenu    string
}

// blocs parcourt l'acte et rend tous ses <CONTENU>, en notant la section qui
// les porte et, dans le dispositif, l'article auquel ils appartiennent.
//
// Le parcours est à PROFONDEUR LIBRE : la DILA place <BLOC_TEXTUEL> directement
// sous <TEXTE> dans ses livraisons quotidiennes et sous <STRUCT><ARTICLE> dans
// sa base complète. Présumer de l'un a déjà vidé deux cents décrets en silence.
func blocs(raw []byte) []blocTexte {
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity

	var out []blocTexte
	var pile []string
	var artID, artNum string
	champArticle := ""

	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch v := tok.(type) {
		case xml.StartElement:
			nom := v.Name.Local
			switch nom {
			case "ARTICLE":
				artID, artNum = "", ""
			case "ID", "NUM":
				// Ces deux balises existent aussi au niveau du texte ; on ne
				// les lit comme identifiants d'article que DANS un article.
				if dansArticle(pile) {
					champArticle = nom
				}
			case "CONTENU":
				var inner struct {
					XML string `xml:",innerxml"`
				}
				if err := d.DecodeElement(&inner, &v); err != nil {
					continue
				}
				txt := strings.TrimSpace(inner.XML)
				if txt == "" {
					continue
				}
				out = append(out, blocTexte{
					section:    section(pile),
					articleID:  artID,
					articleNum: artNum,
					contenu:    txt,
				})
				continue // DecodeElement a déjà consommé la balise fermante
			}
			pile = append(pile, nom)
		case xml.CharData:
			if champArticle == "" {
				continue
			}
			s := strings.TrimSpace(string(v))
			if s == "" {
				continue
			}
			if champArticle == "ID" {
				artID = s
			} else {
				artNum = s
			}
		case xml.EndElement:
			champArticle = ""
			if n := len(pile); n > 0 {
				pile = pile[:n-1]
			}
		}
	}
	return out
}

// section rend la balise significative la plus proche : celle qui dit à quoi
// sert le texte qu'elle contient.
func section(pile []string) string {
	for i := len(pile) - 1; i >= 0; i-- {
		switch pile[i] {
		case "NOTICE", "VISAS", "ABRO", "RECT", "SM", "TP", "BLOC_TEXTUEL", "SIGNATAIRES", "NOTA":
			if pile[i] == "BLOC_TEXTUEL" {
				return "DISPOSITIF"
			}
			return pile[i]
		}
	}
	return "AUTRE"
}

func dansArticle(pile []string) bool {
	for i := len(pile) - 1; i >= 0; i-- {
		if pile[i] == "ARTICLE" {
			return true
		}
	}
	return false
}
