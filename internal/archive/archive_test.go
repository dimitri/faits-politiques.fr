package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func tempFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "dl-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func content(t *testing.T, f *os.File) []byte {
	t.Helper()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func expectedBody(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

func TestDownloadBodySuccess(t *testing.T) {
	body := expectedBody(5000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write(body)
	}))
	defer srv.Close()

	tmp := tempFile(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	status, _, announcedSize, err := downloadBody(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("statut = %d, attendu 200", status)
	}
	if announcedSize != int64(len(body)) {
		t.Fatalf("annonce = %d, attendu %d", announcedSize, len(body))
	}
	if got := content(t, tmp); string(got) != string(body) {
		t.Fatalf("contenu reçu différent (%d octets, attendu %d)", len(got), len(body))
	}
}

func TestDownloadBodyInterruptionAndResume(t *testing.T) {
	body := expectedBody(10000)
	threshold := 4000 // la première réponse s'arrête net après ce nombre d'octets
	var attempts int
	var receivedRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			// Coupure en cours de corps : Content-Length annoncé pour la
			// taille complète, mais la connexion ferme après seuil octets —
			// io.Copy doit voir une erreur, pas une fin de flux propre.
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.WriteHeader(http.StatusOK)
			w.Write(body[:threshold])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					conn.Close()
				}
			}
			return
		}
		receivedRange = r.Header.Get("Range")
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Write(body)
			return
		}
		var start int
		fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(body[start:])
	}))
	defer srv.Close()

	tmp := tempFile(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	status, _, announcedSize, err := downloadBody(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if status != http.StatusPartialContent {
		t.Fatalf("statut final = %d, attendu 206 (reprise)", status)
	}
	if announcedSize != int64(len(body)) {
		t.Fatalf("annonce = %d, attendu %d", announcedSize, len(body))
	}
	if receivedRange != fmt.Sprintf("bytes=%d-", threshold) {
		t.Fatalf("Range reçu = %q, attendu bytes=%d-", receivedRange, threshold)
	}
	if got := content(t, tmp); string(got) != string(body) {
		t.Fatalf("contenu assemblé différent (%d octets, attendu %d) — doublon ou trou possible", len(got), len(body))
	}
	if attempts != 2 {
		t.Fatalf("tentatives = %d, attendu 2", attempts)
	}
}

// Un serveur qui coupe le flux toutes les quelques centaines d'octets mais
// avance à chaque reprise ne doit pas épuiser le budget de tentatives : seules
// les tentatives sans aucun octet comptent (run build-full du 2 novembre 2026,
// flux coupé par data.assemblee-nationale.fr après 113 Mo).
func TestDownloadBodyProgressDoesNotExhaustBudget(t *testing.T) {
	old := delaiReprise
	delaiReprise = 0
	defer func() { delaiReprise = old }()

	body := expectedBody(3000)
	const morceau = 100 // 30 coupures : bien au-delà de maxAttempts (8)
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		start := 0
		if rh := r.Header.Get("Range"); rh != "" {
			fmt.Sscanf(rh, "bytes=%d-", &start)
		}
		end := start + morceau
		if end > len(body) {
			end = len(body)
		}
		if start > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)-start))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.WriteHeader(http.StatusOK)
		}
		w.Write(body[start:end])
		if end < len(body) {
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					conn.Close()
				}
			}
		}
	}))
	defer srv.Close()

	tmp := tempFile(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if _, _, _, err := downloadBody(context.Background(), srv.Client(), req, tmp); err != nil {
		t.Fatalf("après %d tentatives : %v", attempts, err)
	}
	if got := content(t, tmp); string(got) != string(body) {
		t.Fatalf("contenu assemblé différent (%d octets, attendu %d)", len(got), len(body))
	}
	if attempts <= 8 {
		t.Fatalf("seulement %d tentatives : le test ne dépasse pas le budget", attempts)
	}
}

// Sans aucun progrès, le budget reste de 8 tentatives.
func TestDownloadBodyNoProgressStillGivesUp(t *testing.T) {
	old := delaiReprise
	delaiReprise = 0
	defer func() { delaiReprise = old }()

	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				conn.Close()
			}
		}
	}))
	defer srv.Close()

	tmp := tempFile(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if _, _, _, err := downloadBody(context.Background(), srv.Client(), req, tmp); err == nil {
		t.Fatal("un serveur qui n'envoie rien doit finir en erreur")
	}
	if attempts != 8 {
		t.Fatalf("tentatives = %d, attendu 8", attempts)
	}
}

func TestDownloadBodyRangeIgnoredRestarts(t *testing.T) {
	body := expectedBody(3000)
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.WriteHeader(http.StatusOK)
			w.Write(body[:1000])
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					conn.Close()
				}
			}
			return
		}
		// Un serveur qui ignore Range et renvoie TOUJOURS 200 complet.
		w.Write(body)
	}))
	defer srv.Close()

	tmp := tempFile(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	status, _, _, err := downloadBody(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("statut = %d, attendu 200", status)
	}
	got := content(t, tmp)
	if string(got) != string(body) {
		t.Fatalf("contenu = %d octets, attendu %d (pas de doublon après redémarrage)", len(got), len(body))
	}
}

func TestDownloadBody502ThenSuccess(t *testing.T) {
	body := expectedBody(500)
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()

	tmp := tempFile(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	status, _, _, err := downloadBody(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("statut = %d, attendu 200 après reprise sur 502", status)
	}
	if got := content(t, tmp); string(got) != string(body) {
		t.Fatalf("contenu différent après reprise sur 502")
	}
	if attempts != 2 {
		t.Fatalf("tentatives = %d, attendu 2 (502 puis succès)", attempts)
	}
}

func TestDownloadBody404NoRetry(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmp := tempFile(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	status, _, _, err := downloadBody(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if status != http.StatusNotFound {
		t.Fatalf("statut = %d, attendu 404", status)
	}
	if attempts != 1 {
		t.Fatalf("tentatives = %d, attendu 1 (un 404 ne se retente jamais)", attempts)
	}
}

func TestDownloadBody429RetryAfter(t *testing.T) {
	body := expectedBody(200)
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()

	tmp := tempFile(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	status, _, _, err := downloadBody(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("statut = %d, attendu 200 après 429", status)
	}
	if attempts != 2 {
		t.Fatalf("tentatives = %d, attendu 2", attempts)
	}
}

func TestTotalFromContentRange(t *testing.T) {
	cases := []struct {
		header   string
		expected int64
	}{
		{"bytes 0-100/310464306", 310464306},
		{"bytes 500-999/*", -1},
		{"", -1},
		{"n'importe quoi", -1},
	}
	for _, c := range cases {
		if got := totalFromContentRange(c.header); got != c.expected {
			t.Errorf("totalDepuisContentRange(%q) = %d, attendu %d", c.header, got, c.expected)
		}
	}
}

func TestChecksumAfterResume(t *testing.T) {
	// Vérifie que le SHA256 recalculé après coup (fetchOnce) correspond
	// bien au contenu assemblé par plusieurs tentatives, pas seulement que
	// les octets sont corrects : la régression la plus probable d'un
	// hachage "recollé" entre tentatives serait une empreinte fausse sur un
	// contenu par ailleurs correct.
	body := expectedBody(8000)
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.WriteHeader(http.StatusOK)
			w.Write(body[:3000])
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					conn.Close()
				}
			}
			return
		}
		rangeHeader := r.Header.Get("Range")
		var start int
		fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(body[start:])
	}))
	defer srv.Close()

	tmp := tempFile(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if _, _, _, err := downloadBody(context.Background(), srv.Client(), req, tmp); err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, tmp); err != nil {
		t.Fatal(err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	expected := sha256.Sum256(body)
	if got != hex.EncodeToString(expected[:]) {
		t.Fatalf("empreinte = %s, attendue %s", got, hex.EncodeToString(expected[:]))
	}
}
