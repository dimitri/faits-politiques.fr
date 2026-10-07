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
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Archive struct {
	Root string
	Pool *pgxpool.Pool
	// Step : le nom (internal/ingest.Source.Nom) de l'étape du catalogue
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
	Step string
}

// Source décrit un flux, avec sa licence : elle alimente le bandeau
// d'attribution et la classe de réutilisation.
type Source struct {
	Slug        string
	Label       string
	Publisher   string
	Tier        string
	License     string
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
	Name   string
	Source Source
	URL    string
	Ext    string
}

func (a *Archive) EnsureSource(ctx context.Context, s Source) (int64, error) {
	// nil (colonne NULL), pas "" : un appel hors du catalogue (a.Step
	// jamais renseigné) ne doit pas se lire comme une étape nommée "".
	var step any
	if a.Step != "" {
		step = a.Step
	}
	var id int64
	err := a.Pool.QueryRow(ctx, `
		INSERT INTO raw.source (slug, label, publisher, tier, licence, reuse_class,
		                        attribution_text, expected_cadence, notes, etape)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (slug) DO UPDATE SET label = EXCLUDED.label, etape = EXCLUDED.etape
		RETURNING id`,
		s.Slug, s.Label, s.Publisher, s.Tier, s.License, s.ReuseClass,
		s.Attribution, s.Cadence, s.Notes, step).Scan(&id)
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
func (a *Archive) FetchEntetes(ctx context.Context, sourceID int64, runID int64, url, ext string, headers http.Header) (*Fetched, error) {
	var last error
	for attempt := 1; attempt <= 3; attempt++ {
		f, err := a.fetchOnce(ctx, sourceID, runID, url, ext, headers, nil, "")
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
// ce qui a été exporté : archivedURL, enregistrée à sa place dans
// raw.retrieval, porte la description de la recherche en fragment (#...),
// jamais envoyé au serveur.
func (a *Archive) FetchSession(ctx context.Context, sourceID int64, runID int64, url, archivedURL, ext string, client *http.Client) (*Fetched, error) {
	var last error
	for attempt := 1; attempt <= 3; attempt++ {
		f, err := a.fetchOnce(ctx, sourceID, runID, url, ext, nil, client, archivedURL)
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

// previousRetrieval : ce qu'a laissé le dernier fetch RÉUSSI (2xx ou 304)
// sur cette URL EXACTE — de quoi construire une requête conditionnelle, et,
// si le serveur répond 304, de quoi renvoyer le même Fetched qu'avant sans
// rien retélécharger.
type previousRetrieval struct {
	documentID   int64
	sha256       string
	path         string
	byteSize     int64
	etag         string
	lastModified time.Time
}

// lookupPreviousRetrieval relit raw.retrieval/raw.document pour url — jamais
// archivedURL, qui peut différer (voir FetchSession) : c'est ce qui est
// vraiment envoyé au serveur qui doit correspondre à l'etag qu'on lui
// renvoie. nil, sans erreur, si cette URL n'a encore jamais été récupérée
// avec succès.
func (a *Archive) lookupPreviousRetrieval(ctx context.Context, url string) (*previousRetrieval, error) {
	var p previousRetrieval
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
	// Le fichier peut avoir disparu de a.Root sans que raw.retrieval le
	// sache — une archive reconstituée partiellement, un nettoyage, un
	// worktree différent de celui où ce document a été scellé. Un 304 sans
	// fichier derrière laisserait fetchOnce renvoyer un Fetched.Path
	// introuvable à l'appelant, qui échouerait bien plus tard et bien
	// moins clairement en essayant de le lire. nil ici, comme s'il n'y
	// avait jamais eu de precedent : aucun en-tête conditionnel n'est
	// envoyé, un GET normal a lieu et réécrit le fichier à sa place.
	if p.path == "" {
		return nil, nil
	}
	if _, err := os.Stat(p.path); err != nil {
		return nil, nil
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

// resumableStatus : les statuts qu'une nouvelle tentative peut corriger —
// jamais un autre 4xx (qui ne changera pas en réessayant), jamais un succès
// ou un 304. Découvert sur recherche-entreprises.api.gouv.fr (429 : un
// ingest complet y multiplie les requêtes, une par SIREN, depuis une même
// adresse IP, en quelques minutes — invisible en développement, où les
// requêtes s'étalent sur des jours d'essais successifs, mais systématique
// en CI) ; 502/504 ajoutés après un échec réel en CI sur
// data.assemblee-nationale.fr (un relais CDN, « x-cdn-pop » dans ses
// en-têtes, qui renvoie 206/Accept-Ranges normalement mais a renvoyé un 502
// ponctuel sur un fichier de 296 Mo).
func resumableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// wait attend delay (ou l'annulation de ctx), puis le double pour la
// prochaine fois, plafonné à 60s — le même rythme pour une panne réseau et
// pour un statut reprenable, jamais deux politiques de repli différentes
// dans la même fonction.
func wait(ctx context.Context, delay *time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(*delay):
	}
	*delay *= 2
	if *delay > 60*time.Second {
		*delay = 60 * time.Second
	}
	return true
}

// totalFromContentRange extrait le total après le "/" d'un en-tête
// Content-Range ("bytes 0-100/310464306") — -1 si absent ou illisible,
// jamais une erreur : seule la vérification de taille en fin de
// téléchargement en dépend, pas la reprise elle-même.
func totalFromContentRange(cr string) int64 {
	i := strings.LastIndexByte(cr, '/')
	if i < 0 || cr[i+1:] == "*" {
		return -1
	}
	n, err := strconv.ParseInt(cr[i+1:], 10, 64)
	if err != nil {
		return -1
	}
	return n
}

// downloadBody récupère le corps de req dans tmp (déjà ouvert en
// écriture, par l'appelant), avec reprise : une panne réseau, une coupure en
// cours de corps, ou un statut de resumableStatus ne redémarrent pas le
// téléchargement à zéro — la tentative suivante reprend par Range après les
// octets déjà écrits dans tmp. Si le serveur ignore Range (renvoie 200 au
// lieu de 206) ou renvoie 416 (nos octets ne correspondent plus à ce qu'il a
// — rare, contenu changé entre deux tentatives), tmp est tronqué et tout
// redémarre, aussi rarement que ce soit en pratique.
//
// Ne décide jamais elle-même si le statut final est un succès : rendu tel
// quel (statut, en-têtes, taille annoncée) à l'appelant, qui connaît déjà la
// sémantique à appliquer (304, 2xx, autre). Le SHA256 ne se calcule PAS ici
// : fetchOnce relit tmp au complet une fois le transfert terminé — correct
// quel que soit le nombre de tentatives qu'il a fallu, jamais un état de
// hachage à recoller entre elles.
func downloadBody(ctx context.Context, client *http.Client, req *http.Request, tmp *os.File) (status int, headers http.Header, announcedSize int64, err error) {
	const maxAttempts = 8
	delay := 2 * time.Second
	var written int64
	announcedSize = -1
	restart := func() error {
		if err := tmp.Truncate(0); err != nil {
			return err
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return err
		}
		written, announcedSize = 0, -1
		return nil
	}
	for attempt := 1; ; attempt++ {
		r := req.Clone(ctx)
		if written > 0 {
			r.Header.Set("Range", fmt.Sprintf("bytes=%d-", written))
		}
		resp, errReq := client.Do(r)
		if errReq != nil {
			if ctx.Err() != nil || attempt >= maxAttempts {
				return 0, nil, announcedSize, errReq
			}
			logs.Notice(fmt.Sprintf("%s : %s, nouvelle tentative dans %s (%d/%d)",
				req.URL.String(), errReq, delay, attempt, maxAttempts))
			if !wait(ctx, &delay) {
				return 0, nil, announcedSize, ctx.Err()
			}
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK:
			if written > 0 {
				// Range ignoré (ou une autre route a répondu) : impossible
				// de recoller un début et une suite qui ne viennent pas du
				// même corps — on repart de zéro pour de vrai.
				if err := restart(); err != nil {
					resp.Body.Close()
					return 0, nil, announcedSize, err
				}
			}
			if resp.ContentLength >= 0 {
				announcedSize = resp.ContentLength
			}
		case http.StatusPartialContent:
			if announcedSize < 0 {
				announcedSize = totalFromContentRange(resp.Header.Get("Content-Range"))
			}
		case http.StatusRequestedRangeNotSatisfiable:
			resp.Body.Close()
			if err := restart(); err != nil {
				return 0, nil, announcedSize, err
			}
			if attempt >= maxAttempts {
				return 0, nil, announcedSize, fmt.Errorf("%s : HTTP 416 persistant", req.URL.String())
			}
			continue
		default:
			if resumableStatus(resp.StatusCode) {
				waitTime := delay
				if ra := resp.Header.Get("Retry-After"); ra != "" {
					if s, err := strconv.Atoi(ra); err == nil && s >= 0 {
						waitTime = time.Duration(s) * time.Second
					}
				}
				resp.Body.Close()
				if attempt >= maxAttempts {
					// Budget épuisé : rendu tel quel, fetchOnce traite ce
					// statut comme n'importe quel échec terminal.
					return resp.StatusCode, resp.Header, announcedSize, nil
				}
				logs.Notice(fmt.Sprintf("%s : HTTP %d, nouvelle tentative dans %s (%d/%d)",
					req.URL.String(), resp.StatusCode, waitTime, attempt, maxAttempts))
				select {
				case <-ctx.Done():
					return 0, nil, announcedSize, ctx.Err()
				case <-time.After(waitTime):
				}
				delay *= 2
				if delay > 60*time.Second {
					delay = 60 * time.Second
				}
				continue
			}
			// Statut terminal non reprenable (304, 2xx hors 200/206, un
			// autre 4xx...) : aucun corps qui nous concerne ici (304 n'en a
			// pas ; un 4xx porte une page d'erreur, pas le document) —
			// rendu tel quel, jamais de copie.
			return resp.StatusCode, resp.Header, announcedSize, nil
		}

		n, errCopy := io.Copy(tmp, resp.Body)
		resp.Body.Close()
		written += n
		if errCopy != nil {
			if ctx.Err() != nil {
				return 0, nil, announcedSize, ctx.Err()
			}
			if attempt >= maxAttempts {
				return 0, nil, announcedSize, fmt.Errorf("%s : %w (après %s reçus)",
					req.URL.String(), errCopy, readableSize(written))
			}
			logs.Notice(fmt.Sprintf("%s : coupure après %s, reprise dans %s (%d/%d)",
				req.URL.String(), readableSize(written), delay, attempt, maxAttempts))
			if !wait(ctx, &delay) {
				return 0, nil, announcedSize, ctx.Err()
			}
			continue
		}
		if announcedSize >= 0 && written != announcedSize {
			// Le corps s'est terminé SANS erreur réseau mais plus court
			// qu'annoncé : certains relais ferment la connexion proprement
			// sans la signaler comme une panne — jamais un fichier tronqué
			// scellé comme bon, traité comme une coupure ordinaire.
			if attempt >= maxAttempts {
				return 0, nil, announcedSize, fmt.Errorf("%s : %d octets reçus sur %d annoncés",
					req.URL.String(), written, announcedSize)
			}
			logs.Notice(fmt.Sprintf("%s : %s reçus sur %s annoncés, reprise (%d/%d)",
				req.URL.String(), readableSize(written), readableSize(announcedSize), attempt, maxAttempts))
			continue
		}
		return resp.StatusCode, resp.Header, announcedSize, nil
	}
}

func (a *Archive) fetchOnce(ctx context.Context, sourceID int64, runID int64, url, ext string, headers http.Header, client *http.Client, archivedURL string) (*Fetched, error) {
	if archivedURL == "" {
		archivedURL = url
	}
	if pf, ok := prefetched(ctx, url); ok {
		var retID int64
		if err := a.Pool.QueryRow(ctx, `
			INSERT INTO raw.retrieval (source_id, fetch_run_id, url, http_status, document_id)
			VALUES ($1,$2,$3,200,$4) RETURNING id`,
			sourceID, runID, archivedURL, pf.DocumentID).Scan(&retID); err != nil {
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
	for k, vs := range headers {
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
	previous, err := a.lookupPreviousRetrieval(ctx, url)
	if err != nil {
		return nil, err
	}
	if previous != nil {
		if previous.etag != "" {
			req.Header.Set("If-None-Match", previous.etag)
		}
		if !previous.lastModified.IsZero() {
			req.Header.Set("If-Modified-Since", previous.lastModified.UTC().Format(http.TimeFormat))
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
		logs.Notice(fmt.Sprintf("downloading %s (%s)", url, readableSize(taille)))
	} else {
		logs.Notice("downloading " + url)
	}
	status, headers, announcedSize, err := downloadBody(ctx, client, req, tmp)
	if err != nil {
		tmp.Close()
		return nil, err
	}

	// 304 : le serveur confirme que previous.documentID est toujours le bon
	// document, sans en renvoyer les octets — c'est tout le gain de la
	// requête conditionnelle ci-dessus. raw.retrieval garde quand même une
	// ligne (avec CE document_id, migration 0178) : ce qui est sauté est le
	// GET, jamais l'attestation qu'une source a été vue à cette date pour
	// CE run, même principe que le chemin déjà-préchargé plus haut.
	if status == http.StatusNotModified && previous != nil {
		tmp.Close()
		var lastModified any
		if !previous.lastModified.IsZero() {
			lastModified = previous.lastModified
		}
		var retID int64
		if err := a.Pool.QueryRow(ctx, `
			INSERT INTO raw.retrieval (source_id, fetch_run_id, url, http_status, document_id, etag, last_modified)
			VALUES ($1,$2,$3,304,$4,$5,$6) RETURNING id`,
			sourceID, runID, archivedURL, previous.documentID,
			nullableString(headers.Get("ETag"), previous.etag), lastModified).Scan(&retID); err != nil {
			return nil, err
		}
		logs.Notice(fmt.Sprintf("downloaded %s: %s, sha256 %s (unchanged)",
			url, readableSize(previous.byteSize), previous.sha256[:12]))
		return &Fetched{DocumentID: previous.documentID, RetrievalID: retID,
			Path: previous.path, SHA256: previous.sha256, Cached: true}, nil
	}

	if status < 200 || status > 299 {
		tmp.Close()
		_, _ = a.Pool.Exec(ctx, `
			INSERT INTO raw.retrieval (source_id, fetch_run_id, url, http_status)
			VALUES ($1,$2,$3,$4)`, sourceID, runID, archivedURL, status)
		return nil, fmt.Errorf("%s : HTTP %d", url, status)
	}

	// L'empreinte se calcule en relisant tmp au complet, jamais au fil de
	// l'écriture (un sha256.New() posé en io.MultiWriter avec la première
	// tentative, comme avant cette fonction) : downloadBody peut avoir
	// tronqué et réécrit tmp depuis le début en cours de route (reprise
	// refusée par le serveur, 416) — un hash entamé sur la PREMIÈRE tentative
	// ne vaudrait plus rien après un tel redémarrage. Relire une fois, après
	// coup, est correct quel que soit le nombre de tentatives qu'il a
	// fallu, et ne coûte qu'une lecture disque face à un re-téléchargement
	// réseau que la reprise cherche justement à éviter.
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		tmp.Close()
		return nil, err
	}
	h := sha256.New()
	n, err := io.Copy(h, tmp)
	tmp.Close()
	if err != nil {
		return nil, err
	}

	// Un téléchargement tronqué ne doit JAMAIS être scellé : on archiverait
	// l'empreinte d'un fichier corrompu, et toute vérification ultérieure
	// porterait sur une donnée fausse en croyant l'avoir authentifiée. Garde
	// redondante avec la vérification déjà faite dans downloadBody —
	// celle-ci porte sur ce qui a vraiment atterri sur le disque.
	if announcedSize >= 0 && n != announcedSize {
		return nil, fmt.Errorf("%s : %d octets reçus sur %d annoncés", url, n, announcedSize)
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
	state := "archived"
	if cached {
		state = "unchanged"
	}
	logs.Notice(fmt.Sprintf("downloaded %s: %s, sha256 %s (%s)", url, readableSize(n), sum[:12], state))

	var docID int64
	err = a.Pool.QueryRow(ctx, `
		INSERT INTO raw.document (sha256, storage_key, content_type, byte_size)
		VALUES (decode($1,'hex'), $2, $3, $4)
		ON CONFLICT (sha256) DO UPDATE SET storage_key = raw.document.storage_key
		RETURNING id`,
		sum, key, headers.Get("Content-Type"), n).Scan(&docID)
	if err != nil {
		return nil, err
	}

	var lastModified any
	if t, err := http.ParseTime(headers.Get("Last-Modified")); err == nil {
		lastModified = t
	}
	var retID int64
	err = a.Pool.QueryRow(ctx, `
		INSERT INTO raw.retrieval (source_id, fetch_run_id, url, http_status, document_id, etag, last_modified)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		sourceID, runID, archivedURL, status, docID, nullableString(headers.Get("ETag"), ""), lastModified).Scan(&retID)
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
func headContentLength(ctx context.Context, client *http.Client, url string, headers http.Header) int64 {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return -1
	}
	req.Header = headers.Clone()
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

// readableSize formate un nombre d'octets pour un NOTICE — même échelle
// (Ko/Mo/Go) que cmd/fpctl/list.go, dupliquée plutôt que partagée : deux
// lignes, pas la peine d'exporter un paquet utilitaire pour ça.
func readableSize(bytes int64) string {
	const unit = 1024.0
	v := float64(bytes)
	for _, suffix := range []string{"B", "KB", "MB", "GB", "TB"} {
		if v < unit {
			return fmt.Sprintf("%.1f %s", v, suffix)
		}
		v /= unit
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
