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
const bufferSize = 4096

// stream relie une table cible à son canal d'alimentation. pgx.CopyFrom bloque
// jusqu'à épuisement de la source, d'où une goroutine et une connexion par
// table.
type stream struct {
	table   pgx.Identifier
	columns []string
	rows    chan []any
	n       int64
	err     error
}

func newStream(schema, table string, columns ...string) *stream {
	return &stream{
		table:   pgx.Identifier{schema, table},
		columns: columns,
		rows:    make(chan []any, bufferSize),
	}
}

// source adapte un canal à l'interface que pgx attend. Elle ne met JAMAIS tout
// en mémoire : pgx tire une ligne à la fois, au rythme du réseau.
type source struct {
	c       chan []any
	current []any
	n       *int64
}

func (s *source) Next() bool {
	v, ok := <-s.c
	if !ok {
		return false
	}
	s.current = v
	atomic.AddInt64(s.n, 1)
	return true
}
func (s *source) Values() ([]any, error) { return s.current, nil }
func (s *source) Err() error             { return nil }

// start ouvre une connexion dédiée et y lance un COPY qui durera toute la
// traversée de l'archive.
func (f *stream) start(ctx context.Context, pool *pgxpool.Pool, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := pool.Acquire(ctx)
		if err != nil {
			f.err = err
			// Le canal doit être vidé malgré tout, sinon le lecteur de
			// l'archive se bloque sur un canal plein et le programme fige.
			for range f.rows {
			}
			return
		}
		defer conn.Release()
		_, f.err = conn.CopyFrom(ctx, f.table, f.columns, &source{c: f.rows, n: &f.n})
		if f.err != nil {
			for range f.rows {
			}
		}
	}()
}

// Les deux natures de fichier, décodées séparément : elles n'ont en commun que
// l'identifiant et la date de publication.
type summaryJO struct {
	ID              string `xml:"ID"`
	Nature          string `xml:"NATURE"`
	Title           string `xml:"TITRE"`
	Num             string `xml:"NUM"`
	PublicationDate string `xml:"DATE_PUBLI"`
	Links           []struct {
		TextID    string `xml:"idtxt,attr"`
		TextTitle string `xml:"titretxt,attr"`
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

	f, err := arch.Fetch(ctx, srcID, runID, fullArchiveURL, ".tar.gz")
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
	viewsStart := time.Now()
	for _, v := range []string{"jo.recherche_texte", "jo.recherche_bloc"} {
		if _, err := pool.Exec(ctx, "REFRESH MATERIALIZED VIEW "+v); err != nil {
			return fail(fmt.Errorf("rafraîchissement de %s : %w", v, err))
		}
	}
	viewsSeconds := int64(time.Since(viewsStart).Seconds())

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"fichiers": stats["fichiers"], "sommaires": stats["sommaire"],
		"liens": stats["lien"], "actes": stats["texte"], "blocs": stats["bloc"],
		"secondes": stats["secondes"], "recherche_s": viewsSeconds}, "")
	logs.Notice(fmt.Sprintf("official gazette: %s read in %ds",
		logs.Plural(int(stats["fichiers"]), "file"), stats["secondes"]))
	logs.Notice(fmt.Sprintf("%s, %s, %s, %s", logs.Plural(int(stats["sommaire"]), "summary"),
		logs.Plural(int(stats["lien"]), "link"), logs.Plural(int(stats["texte"]), "act"),
		logs.Plural(int(stats["bloc"]), "block")))
	logs.Notice(fmt.Sprintf("search views refreshed in %ds", viewsSeconds))
	return nil
}

// CopierArchive traverse l'archive une fois et alimente quatre tables en
// parallèle. Elle rend le nombre de lignes écrites par table.
func CopierArchive(ctx context.Context, pool *pgxpool.Pool, path, schema string) (map[string]int64, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	sSummary := newStream(schema, "sommaire", "id", "nature", "titre", "num", "date_publi")
	sLink := newStream(schema, "lien", "sommaire_id", "texte_id", "titre", "ordre")
	sText := newStream(schema, "texte", "id", "nature", "num", "nor", "date_publi",
		"date_texte", "titre", "titre_complet", "ministere", "origine_publi")
	sBlock := newStream(schema, "bloc", "texte_id", "ordre", "section", "article_id",
		"article_num", "contenu")
	streams := []*stream{sSummary, sLink, sText, sBlock}

	var wg sync.WaitGroup
	for _, s := range streams {
		s.start(ctx, pool, &wg)
	}

	// Instrumentation : JORF_ETAPE borne le travail pour mesurer où passe le
	// temps. walk = décompression et parcours seuls ; meta = plus le décodage
	// des métadonnées ; texte = plus l'écriture des actes ; vide = tout.
	stage := os.Getenv("JORF_ETAPE")

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
	workerCount := runtime.NumCPU()
	if v := os.Getenv("JORF_OUVRIERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			workerCount = n
		}
	}
	type task struct {
		base string
		raw  []byte
	}
	tasks := make(chan task, workerCount*8)
	var wgWorkers sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wgWorkers.Add(1)
		go func() {
			defer wgWorkers.Done()
			for t := range tasks {
				analyze(t.base, t.raw, stage, sSummary, sLink, sText, sBlock)
			}
		}()
	}

	start := time.Now()
	var fileCount int64
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
		fileCount++

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
		if stage == "walk" {
			continue
		}
		tasks <- task{base: base, raw: raw}
	}
	close(tasks)
	wgWorkers.Wait()
	for _, s := range streams {
		close(s.rows)
	}
	wg.Wait()

	out := map[string]int64{"fichiers": fileCount, "secondes": int64(time.Since(start).Seconds())}
	for _, s := range streams {
		if s.err != nil {
			return out, fmt.Errorf("%s : %w", s.table.Sanitize(), s.err)
		}
		out[s.table[len(s.table)-1]] = s.n
	}
	return out, nil
}

// analyze décode un fichier et pousse ses lignes vers les flux. Elle est
// appelée depuis plusieurs goroutines : elle ne partage rien, les canaux
// faisant la synchronisation.
func analyze(base string, raw []byte, stage string, sSummary, sLink, sText, sBlock *stream) {
	switch {
	case strings.HasPrefix(base, "JORFCONT"):
		var s summaryJO
		if err := decodeInto(raw, &s); err != nil || s.ID == "" {
			return
		}
		sSummary.rows <- []any{s.ID, nullIfEmpty(s.Nature), nullIfEmpty(s.Title), nullIfEmpty(s.Num), dateJO(s.PublicationDate)}
		for i, l := range s.Links {
			if l.TextID == "" {
				continue
			}
			sLink.rows <- []any{s.ID, l.TextID, nullIfEmpty(l.TextTitle), i + 1}
		}

	case strings.HasPrefix(base, "JORFTEXT"):
		t, err := decode(raw)
		if err != nil || t.ID == "" {
			return
		}
		if stage == "meta" {
			return
		}
		sText.rows <- []any{t.ID, nullIfEmpty(t.Nature), nullIfEmpty(t.Num), nullIfEmpty(t.NOR),
			dateJO(t.PublicationDate), dateJO(t.TextDate), nullIfEmpty(t.Title), nullIfEmpty(t.FullTitle),
			nullIfEmpty(t.Ministry), nullIfEmpty(t.PublicationOrigin)}
		if stage == "texte" {
			return
		}
		for i, b := range textBlocks(raw) {
			sBlock.rows <- []any{t.ID, i + 1, b.section, nullIfEmpty(b.articleID), nullIfEmpty(b.articleNum), b.content}
		}
	}
}

func decodeInto(raw []byte, v any) error {
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	return d.Decode(v)
}

// textBlock est un morceau de texte de l'acte, avec l'endroit d'où il vient.
//
// La SECTION importe autant que le contenu. Le Journal officiel range son texte
// en <NOTICE> (à qui s'adresse le texte), <VISAS> (les fondements juridiques),
// <STRUCT><ARTICLE> (le dispositif, seul à édicter quelque chose), <ABRO> (les
// abrogations) et <SM> (les signataires). Les confondre ferait entrer dans le
// dispositif des phrases que l'acte n'édicte pas — l'extraction de readBody()
// les écarte justement, mais pour archiver il vaut mieux tout garder ET dire
// d'où ça vient : c'est `raw`, on n'y jette rien.
type textBlock struct {
	section    string
	articleID  string
	articleNum string
	content    string
}

// textBlocks parcourt l'acte et rend tous ses <CONTENU>, en notant la section
// qui les porte et, dans le dispositif, l'article auquel ils appartiennent.
//
// Le parcours est à PROFONDEUR LIBRE : la DILA place <BLOC_TEXTUEL> directement
// sous <TEXTE> dans ses livraisons quotidiennes et sous <STRUCT><ARTICLE> dans
// sa base complète. Présumer de l'un a déjà vidé deux cents décrets en silence.
func textBlocks(raw []byte) []textBlock {
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity

	var out []textBlock
	var stack []string
	var artID, artNum string
	articleField := ""

	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch v := tok.(type) {
		case xml.StartElement:
			name := v.Name.Local
			switch name {
			case "ARTICLE":
				artID, artNum = "", ""
			case "ID", "NUM":
				// Ces deux balises existent aussi au niveau du texte ; on ne
				// les lit comme identifiants d'article que DANS un article.
				if inArticle(stack) {
					articleField = name
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
				out = append(out, textBlock{
					section:    section(stack),
					articleID:  artID,
					articleNum: artNum,
					content:    txt,
				})
				continue // DecodeElement a déjà consommé la balise fermante
			}
			stack = append(stack, name)
		case xml.CharData:
			if articleField == "" {
				continue
			}
			s := strings.TrimSpace(string(v))
			if s == "" {
				continue
			}
			if articleField == "ID" {
				artID = s
			} else {
				artNum = s
			}
		case xml.EndElement:
			articleField = ""
			if n := len(stack); n > 0 {
				stack = stack[:n-1]
			}
		}
	}
	return out
}

// section rend la balise significative la plus proche : celle qui dit à quoi
// sert le texte qu'elle contient.
func section(stack []string) string {
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i] {
		case "NOTICE", "VISAS", "ABRO", "RECT", "SM", "TP", "BLOC_TEXTUEL", "SIGNATAIRES", "NOTA":
			if stack[i] == "BLOC_TEXTUEL" {
				return "DISPOSITIF"
			}
			return stack[i]
		}
	}
	return "AUTRE"
}

func inArticle(stack []string) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == "ARTICLE" {
			return true
		}
	}
	return false
}
