// Package archive tient l'archive scellée des documents primaires.
//
// Règle absolue (docs/perimetre.md §2.5) : un document est identifié par
// l'empreinte de ses octets, n'est jamais écrasé, et toute récupération est
// datée. Localement l'archive est un répertoire ; en production ce sera un
// object storage, avec la même convention de nommage.
package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Archive struct {
	Root string
	Pool *pgxpool.Pool
	// Etape : le nom (internal/ingest.Source.Nom) de l'étape du catalogue
	// pour le compte de laquelle tourne cet appel — écrit dans
	// raw.source.etape par EnsureSource, pour que « fpctl list deps »
	// calcule les octets à télécharger par étape sans liste tenue à la
	// main. Vide pour un appel hors du catalogue (tests, scripts ponctuels)
	// : EnsureSource laisse alors la colonne NULL plutôt que d'écrire une
	// chaîne vide. Une étape qui appelle plusieurs connecteurs (download)
	// partage la même valeur pour tous : c'est le but, elles comptent
	// ensemble. En concurrence (le socle parlementaire, internal/pipeline),
	// chaque étape reçoit sa PROPRE copie de l'Archive plutôt que de muter
	// ce champ sur un pointeur partagé — voir registreParlement,
	// internal/ingest/ingest.go.
	Etape string
}

// Source décrit un flux, avec sa licence : elle alimente le bandeau
// d'attribution et la classe de réutilisation.
type Source struct {
	Slug        string
	Label       string
	Publisher   string
	Tier        string
	Licence     string
	ReuseClass  string
	Attribution string
	Cadence     string
	Notes       string
}

// DownloadTarget nomme une URL qu'un connecteur récupère, sans la récupérer —
// pour qu'un appelant qui n'est PAS ce connecteur (la récupération
// concurrente de fpctl build, voir internal/ingest.PrefetchAll) puisse
// récupérer d'avance tout ce dont plusieurs connecteurs auront besoin, sans
// dupliquer la liste de leurs URL. Chaque connecteur qui en a expose sa
// propre DownloadTargets() : c'est elle, jamais une seconde liste, que son
// propre Ingest/Download parcourt.
type DownloadTarget struct {
	Nom    string
	Source Source
	URL    string
	Ext    string
}

func (a *Archive) EnsureSource(ctx context.Context, s Source) (int64, error) {
	// nil (colonne NULL), pas "" : un appel hors du catalogue (a.Etape
	// jamais renseigné) ne doit pas se lire comme une étape nommée "".
	var etape any
	if a.Etape != "" {
		etape = a.Etape
	}
	var id int64
	err := a.Pool.QueryRow(ctx, `
		INSERT INTO raw.source (slug, label, publisher, tier, licence, reuse_class,
		                        attribution_text, expected_cadence, notes, etape)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (slug) DO UPDATE SET label = EXCLUDED.label, etape = EXCLUDED.etape
		RETURNING id`,
		s.Slug, s.Label, s.Publisher, s.Tier, s.Licence, s.ReuseClass,
		s.Attribution, s.Cadence, s.Notes, etape).Scan(&id)
	return id, err
}

// Fetched décrit le résultat d'une récupération.
type Fetched struct {
	DocumentID  int64
	RetrievalID int64
	Path        string
	SHA256      string
	Cached      bool
}

// prefetchKey — voir WithPrefetched.
type prefetchKey struct{}

// WithPrefetched attache à ctx un ensemble de fichiers déjà récupérés,
// indexés par URL : un Fetch/FetchEntetes ultérieur pour l'une de ces URL
// réutilise les octets déjà sur disque plutôt que de les retélécharger.
//
// Sert aux commandes (fpctl build) qui récupèrent d'abord, en une seule
// vague concurrente, tout ce dont la chaîne de préalables aura besoin —
// plutôt qu'un connecteur après l'autre, chacun attendant son tour dans le
// graphe de dépendances avant même de commencer son propre téléchargement.
// Une nouvelle ligne raw.retrieval est quand même écrite à chaque appel
// (voir Fetch) : ce qui est sauté est le GET, jamais l'attestation qu'un
// document a été vu à cette date, pour CE connecteur et CETTE exécution.
func WithPrefetched(ctx context.Context, files map[string]*Fetched) context.Context {
	return context.WithValue(ctx, prefetchKey{}, files)
}

func prefetched(ctx context.Context, url string) (*Fetched, bool) {
	m, ok := ctx.Value(prefetchKey{}).(map[string]*Fetched)
	if !ok {
		return nil, false
	}
	f, ok := m[url]
	return f, ok
}

// Fetch télécharge une URL si son contenu n'est pas déjà dans l'archive.
// Un document déjà présent n'est pas retéléchargé mais une nouvelle
// récupération est enregistrée : elle atteste que le document était encore en
// ligne à cette date.
func (a *Archive) Fetch(ctx context.Context, sourceID int64, runID int64, url, ext string) (*Fetched, error) {
	return a.FetchEntetes(ctx, sourceID, runID, url, ext, nil)
}

// FetchEntetes est Fetch avec des en-têtes HTTP supplémentaires — pour les
// API qui exigent une clé (Banque de France Webstat). La clé passe dans un
// en-tête et JAMAIS dans l'URL : raw.retrieval conserve l'URL telle quelle,
// et une clé qui y figurerait serait publiée avec la provenance. Les en-têtes
// ne sont pas archivés ; seule compte l'empreinte des octets reçus.
func (a *Archive) FetchEntetes(ctx context.Context, sourceID int64, runID int64, url, ext string, entetes http.Header) (*Fetched, error) {
	var last error
	for attempt := 1; attempt <= 3; attempt++ {
		f, err := a.fetchOnce(ctx, sourceID, runID, url, ext, entetes, nil, "")
		if err == nil {
			return f, nil
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt*3) * time.Second):
		}
	}
	return nil, last
}

// FetchSession télécharge avec un client fourni — typiquement porteur d'un
// cookie de session, pour un export qui dépend d'une recherche faite juste
// avant (registre européen des aides d'État). L'URL d'export seule ne dit pas
// ce qui a été exporté : urlArchivee, enregistrée à sa place dans
// raw.retrieval, porte la description de la recherche en fragment (#...),
// jamais envoyé au serveur.
func (a *Archive) FetchSession(ctx context.Context, sourceID int64, runID int64, url, urlArchivee, ext string, client *http.Client) (*Fetched, error) {
	var last error
	for attempt := 1; attempt <= 3; attempt++ {
		f, err := a.fetchOnce(ctx, sourceID, runID, url, ext, nil, client, urlArchivee)
		if err == nil {
			return f, nil
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt*5) * time.Second):
		}
	}
	return nil, last
}

// retenuePrecedente : ce qu'a laissé le dernier fetch RÉUSSI (2xx ou 304)
// sur cette URL EXACTE — de quoi construire une requête conditionnelle, et,
// si le serveur répond 304, de quoi renvoyer le même Fetched qu'avant sans
// rien retélécharger.
type retenuePrecedente struct {
	documentID   int64
	sha256       string
	path         string
	byteSize     int64
	etag         string
	lastModified time.Time
}

// derniereRetenue relit raw.retrieval/raw.document pour url — jamais
// urlArchivee, qui peut différer (voir FetchSession) : c'est ce qui est
// vraiment envoyé au serveur qui doit correspondre à l'etag qu'on lui
// renvoie. nil, sans erreur, si cette URL n'a encore jamais été récupérée
// avec succès.
func (a *Archive) derniereRetenue(ctx context.Context, url string) (*retenuePrecedente, error) {
	var p retenuePrecedente
	var etag *string
	var storageKey *string
	var lastModified *time.Time
	err := a.Pool.QueryRow(ctx, `
		SELECT d.id, encode(d.sha256,'hex'), d.storage_key, d.byte_size, r.etag, r.last_modified
		  FROM raw.retrieval r JOIN raw.document d ON d.id = r.document_id
		 WHERE r.url = $1 AND r.document_id IS NOT NULL
		 ORDER BY r.fetched_at DESC LIMIT 1`, url).
		Scan(&p.documentID, &p.sha256, &storageKey, &p.byteSize, &etag, &lastModified)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if etag != nil {
		p.etag = *etag
	}
	if lastModified != nil {
		p.lastModified = *lastModified
	}
	if storageKey != nil {
		p.path = filepath.Join(a.Root, *storageKey)
	}
	return &p, nil
}

// nullableString renvoie primary si non vide, sinon fallback si non vide,
// sinon nil — pour qu'une colonne texte nullable reçoive NULL plutôt qu'une
// chaîne vide quand aucun en-tête ne l'a fournie.
func nullableString(primary, fallback string) any {
	if primary != "" {
		return primary
	}
	if fallback != "" {
		return fallback
	}
	return nil
}

func (a *Archive) fetchOnce(ctx context.Context, sourceID int64, runID int64, url, ext string, entetes http.Header, client *http.Client, urlArchivee string) (*Fetched, error) {
	if urlArchivee == "" {
		urlArchivee = url
	}
	if pf, ok := prefetched(ctx, url); ok {
		var retID int64
		if err := a.Pool.QueryRow(ctx, `
			INSERT INTO raw.retrieval (source_id, fetch_run_id, url, http_status, document_id)
			VALUES ($1,$2,$3,200,$4) RETURNING id`,
			sourceID, runID, urlArchivee, pf.DocumentID).Scan(&retID); err != nil {
			return nil, err
		}
		return &Fetched{DocumentID: pf.DocumentID, RetrievalID: retID, Path: pf.Path, SHA256: pf.SHA256, Cached: true}, nil
	}
	tmp, err := os.CreateTemp(a.Root, ".dl-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	for k, vs := range entetes {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	// Un appelant qui fournit déjà un User-Agent (contournement d'un pare-feu
	// applicatif qui rejette l'identification par défaut, ex. Drees) garde le
	// sien — le défaut ne s'applique que si l'en-tête est absent.
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "faits-politiques.fr (ingestion open data)")
	}
	// Requête conditionnelle : si un fetch précédent EXACTEMENT sur cette
	// URL a laissé un etag/last_modified, les renvoyer coûte un en-tête et
	// peut éviter tout le corps — un serveur qui les ignore répond
	// simplement 200 comme avant, jamais un risque, seulement un gain
	// possible. Vérifié en direct sur data.assemblee-nationale.fr,
	// data.senat.fr, static.data.gouv.fr et popu-list.github.io (GitHub
	// Pages) : les quatre rendent un vrai 304 à une requête conditionnelle.
	precedent, err := a.derniereRetenue(ctx, url)
	if err != nil {
		return nil, err
	}
	if precedent != nil {
		if precedent.etag != "" {
			req.Header.Set("If-None-Match", precedent.etag)
		}
		if !precedent.lastModified.IsZero() {
			req.Header.Set("If-Modified-Since", precedent.lastModified.UTC().Format(http.TimeFormat))
		}
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	// Un GET peut prendre plusieurs minutes sur un gros fichier (AMO30 fait
	// plusieurs dizaines de Mo) — sans repère avant qu'il ne se termine, ça
	// se lit comme une commande bloquée plutôt que comme un téléchargement en
	// cours. taille annoncée par un HEAD préalable, au mieux : un serveur qui
	// ne le supporte pas ou ne rend pas Content-Length laisse simplement ce
	// champ absent, jamais une raison d'échouer le HEAD ne doit faire
	// échouer le GET qui suit.
	if taille := headContentLength(ctx, client, req.URL.String(), req.Header); taille > 0 {
		logs.Notice(fmt.Sprintf("downloading %s (%s)", url, tailleLisible(taille)))
	} else {
		logs.Notice("downloading " + url)
	}
	resp, err := client.Do(req)
	if err != nil {
		tmp.Close()
		return nil, err
	}
	defer resp.Body.Close()

	// 304 : le serveur confirme que precedent.documentID est toujours le bon
	// document, sans en renvoyer les octets — c'est tout le gain de la
	// requête conditionnelle ci-dessus. raw.retrieval garde quand même une
	// ligne (avec CE document_id, migration 0178) : ce qui est sauté est le
	// GET, jamais l'attestation qu'une source a été vue à cette date pour
	// CE run, même principe que le chemin déjà-préchargé plus haut.
	if resp.StatusCode == http.StatusNotModified && precedent != nil {
		tmp.Close()
		var dernModif any
		if !precedent.lastModified.IsZero() {
			dernModif = precedent.lastModified
		}
		var retID int64
		if err := a.Pool.QueryRow(ctx, `
			INSERT INTO raw.retrieval (source_id, fetch_run_id, url, http_status, document_id, etag, last_modified)
			VALUES ($1,$2,$3,304,$4,$5,$6) RETURNING id`,
			sourceID, runID, urlArchivee, precedent.documentID,
			nullableString(resp.Header.Get("ETag"), precedent.etag), dernModif).Scan(&retID); err != nil {
			return nil, err
		}
		logs.Notice(fmt.Sprintf("downloaded %s: %s, sha256 %s (unchanged)",
			url, tailleLisible(precedent.byteSize), precedent.sha256[:12]))
		return &Fetched{DocumentID: precedent.documentID, RetrievalID: retID,
			Path: precedent.path, SHA256: precedent.sha256, Cached: true}, nil
	}

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	tmp.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = a.Pool.Exec(ctx, `
			INSERT INTO raw.retrieval (source_id, fetch_run_id, url, http_status)
			VALUES ($1,$2,$3,$4)`, sourceID, runID, urlArchivee, resp.StatusCode)
		return nil, fmt.Errorf("%s : HTTP %d", url, resp.StatusCode)
	}

	// Un téléchargement tronqué ne doit JAMAIS être scellé : on archiverait
	// l'empreinte d'un fichier corrompu, et toute vérification ultérieure
	// porterait sur une donnée fausse en croyant l'avoir authentifiée.
	if resp.ContentLength > 0 && n != resp.ContentLength {
		return nil, fmt.Errorf("%s : %d octets reçus sur %d annoncés", url, n, resp.ContentLength)
	}

	sum := hex.EncodeToString(h.Sum(nil))
	now := time.Now().UTC()
	key := filepath.Join(fmt.Sprintf("%04d/%02d/%02d", now.Year(), now.Month(), now.Day()), sum+ext)
	dst := filepath.Join(a.Root, key)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, err
	}

	cached := false
	if _, err := os.Stat(dst); err == nil {
		cached = true // mêmes octets : on ne réécrit rien
	} else if err := os.Rename(tmp.Name(), dst); err != nil {
		return nil, err
	}
	etat := "archived"
	if cached {
		etat = "unchanged"
	}
	logs.Notice(fmt.Sprintf("downloaded %s: %s, sha256 %s (%s)", url, tailleLisible(n), sum[:12], etat))

	var docID int64
	err = a.Pool.QueryRow(ctx, `
		INSERT INTO raw.document (sha256, storage_key, content_type, byte_size)
		VALUES (decode($1,'hex'), $2, $3, $4)
		ON CONFLICT (sha256) DO UPDATE SET storage_key = raw.document.storage_key
		RETURNING id`,
		sum, key, resp.Header.Get("Content-Type"), n).Scan(&docID)
	if err != nil {
		return nil, err
	}

	var dernModif any
	if t, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		dernModif = t
	}
	var retID int64
	err = a.Pool.QueryRow(ctx, `
		INSERT INTO raw.retrieval (source_id, fetch_run_id, url, http_status, document_id, etag, last_modified)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		sourceID, runID, urlArchivee, resp.StatusCode, docID, nullableString(resp.Header.Get("ETag"), ""), dernModif).Scan(&retID)
	if err != nil {
		return nil, err
	}

	return &Fetched{DocumentID: docID, RetrievalID: retID, Path: dst, SHA256: sum, Cached: cached}, nil
}

// headContentLength interroge url en HEAD pour connaître sa taille avant de
// la récupérer en entier — un simple repère affiché dans le NOTICE qui
// précède le GET, jamais une condition d'échec : un serveur qui refuse HEAD,
// ne rend pas Content-Length, ou répond hors 2xx laisse simplement -1,
// silencieusement, le GET qui suit tente sa chance normalement.
func headContentLength(ctx context.Context, client *http.Client, url string, entetes http.Header) int64 {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return -1
	}
	req.Header = entetes.Clone()
	resp, err := client.Do(req)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return -1
	}
	return resp.ContentLength
}

// tailleLisible formate un nombre d'octets pour un NOTICE — même échelle
// (Ko/Mo/Go) que cmd/fpctl/list.go, dupliquée plutôt que partagée : deux
// lignes, pas la peine d'exporter un paquet utilitaire pour ça.
func tailleLisible(octets int64) string {
	const unite = 1024.0
	v := float64(octets)
	for _, suffixe := range []string{"B", "KB", "MB", "GB", "TB"} {
		if v < unite {
			return fmt.Sprintf("%.1f %s", v, suffixe)
		}
		v /= unite
	}
	return fmt.Sprintf("%.1f PB", v)
}

func (a *Archive) StartRun(ctx context.Context, sourceID int64, version string) (int64, error) {
	var id int64
	err := a.Pool.QueryRow(ctx, `
		INSERT INTO raw.fetch_run (source_id, connector_version) VALUES ($1,$2) RETURNING id`,
		sourceID, version).Scan(&id)
	return id, err
}

func (a *Archive) EndRun(ctx context.Context, runID int64, status string, stats map[string]any, errMsg string) {
	_, _ = a.Pool.Exec(ctx, `
		UPDATE raw.fetch_run SET finished_at = now(), status = $2, stats = $3, error = NULLIF($4,'')
		WHERE id = $1`, runID, status, stats, errMsg)
}
