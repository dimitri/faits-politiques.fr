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

func fichierTemp(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "dl-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func contenu(t *testing.T, f *os.File) []byte {
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

func corpsAttendu(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

func TestTelechargerCorpsSucces(t *testing.T) {
	corps := corpsAttendu(5000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(corps)))
		w.Write(corps)
	}))
	defer srv.Close()

	tmp := fichierTemp(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	statut, _, annonce, err := telechargerCorps(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if statut != http.StatusOK {
		t.Fatalf("statut = %d, attendu 200", statut)
	}
	if annonce != int64(len(corps)) {
		t.Fatalf("annonce = %d, attendu %d", annonce, len(corps))
	}
	if got := contenu(t, tmp); string(got) != string(corps) {
		t.Fatalf("contenu reçu différent (%d octets, attendu %d)", len(got), len(corps))
	}
}

func TestTelechargerCorpsCoupureEtReprise(t *testing.T) {
	corps := corpsAttendu(10000)
	seuil := 4000 // la première réponse s'arrête net après ce nombre d'octets
	var tentatives int
	var rangeRecue string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tentatives++
		if tentatives == 1 {
			// Coupure en cours de corps : Content-Length annoncé pour la
			// taille complète, mais la connexion ferme après seuil octets —
			// io.Copy doit voir une erreur, pas une fin de flux propre.
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(corps)))
			w.WriteHeader(http.StatusOK)
			w.Write(corps[:seuil])
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
		rangeRecue = r.Header.Get("Range")
		rang := r.Header.Get("Range")
		if rang == "" {
			w.Write(corps)
			return
		}
		var debut int
		fmt.Sscanf(rang, "bytes=%d-", &debut)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", debut, len(corps)-1, len(corps)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(corps[debut:])
	}))
	defer srv.Close()

	tmp := fichierTemp(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	statut, _, annonce, err := telechargerCorps(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if statut != http.StatusPartialContent {
		t.Fatalf("statut final = %d, attendu 206 (reprise)", statut)
	}
	if annonce != int64(len(corps)) {
		t.Fatalf("annonce = %d, attendu %d", annonce, len(corps))
	}
	if rangeRecue != fmt.Sprintf("bytes=%d-", seuil) {
		t.Fatalf("Range reçu = %q, attendu bytes=%d-", rangeRecue, seuil)
	}
	if got := contenu(t, tmp); string(got) != string(corps) {
		t.Fatalf("contenu assemblé différent (%d octets, attendu %d) — doublon ou trou possible", len(got), len(corps))
	}
	if tentatives != 2 {
		t.Fatalf("tentatives = %d, attendu 2", tentatives)
	}
}

func TestTelechargerCorpsRangeIgnoreRedemarre(t *testing.T) {
	corps := corpsAttendu(3000)
	var tentatives int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tentatives++
		if tentatives == 1 {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(corps)))
			w.WriteHeader(http.StatusOK)
			w.Write(corps[:1000])
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					conn.Close()
				}
			}
			return
		}
		// Un serveur qui ignore Range et renvoie TOUJOURS 200 complet.
		w.Write(corps)
	}))
	defer srv.Close()

	tmp := fichierTemp(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	statut, _, _, err := telechargerCorps(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if statut != http.StatusOK {
		t.Fatalf("statut = %d, attendu 200", statut)
	}
	got := contenu(t, tmp)
	if string(got) != string(corps) {
		t.Fatalf("contenu = %d octets, attendu %d (pas de doublon après redémarrage)", len(got), len(corps))
	}
}

func TestTelechargerCorps502PuisSucces(t *testing.T) {
	corps := corpsAttendu(500)
	var tentatives int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tentatives++
		if tentatives == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write(corps)
	}))
	defer srv.Close()

	tmp := fichierTemp(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	statut, _, _, err := telechargerCorps(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if statut != http.StatusOK {
		t.Fatalf("statut = %d, attendu 200 après reprise sur 502", statut)
	}
	if got := contenu(t, tmp); string(got) != string(corps) {
		t.Fatalf("contenu différent après reprise sur 502")
	}
	if tentatives != 2 {
		t.Fatalf("tentatives = %d, attendu 2 (502 puis succès)", tentatives)
	}
}

func TestTelechargerCorps404PasDeReprise(t *testing.T) {
	var tentatives int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tentatives++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmp := fichierTemp(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	statut, _, _, err := telechargerCorps(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if statut != http.StatusNotFound {
		t.Fatalf("statut = %d, attendu 404", statut)
	}
	if tentatives != 1 {
		t.Fatalf("tentatives = %d, attendu 1 (un 404 ne se retente jamais)", tentatives)
	}
}

func TestTelechargerCorps429RetryAfter(t *testing.T) {
	corps := corpsAttendu(200)
	var tentatives int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tentatives++
		if tentatives == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write(corps)
	}))
	defer srv.Close()

	tmp := fichierTemp(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	statut, _, _, err := telechargerCorps(context.Background(), srv.Client(), req, tmp)
	if err != nil {
		t.Fatalf("inattendu : %v", err)
	}
	if statut != http.StatusOK {
		t.Fatalf("statut = %d, attendu 200 après 429", statut)
	}
	if tentatives != 2 {
		t.Fatalf("tentatives = %d, attendu 2", tentatives)
	}
}

func TestTotalDepuisContentRange(t *testing.T) {
	cas := []struct {
		entete  string
		attendu int64
	}{
		{"bytes 0-100/310464306", 310464306},
		{"bytes 500-999/*", -1},
		{"", -1},
		{"n'importe quoi", -1},
	}
	for _, c := range cas {
		if got := totalDepuisContentRange(c.entete); got != c.attendu {
			t.Errorf("totalDepuisContentRange(%q) = %d, attendu %d", c.entete, got, c.attendu)
		}
	}
}

func TestEmpreinteApresReprise(t *testing.T) {
	// Vérifie que le SHA256 recalculé après coup (fetchOnce) correspond
	// bien au contenu assemblé par plusieurs tentatives, pas seulement que
	// les octets sont corrects : la régression la plus probable d'un
	// hachage "recollé" entre tentatives serait une empreinte fausse sur un
	// contenu par ailleurs correct.
	corps := corpsAttendu(8000)
	var tentatives int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tentatives++
		if tentatives == 1 {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(corps)))
			w.WriteHeader(http.StatusOK)
			w.Write(corps[:3000])
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					conn.Close()
				}
			}
			return
		}
		rang := r.Header.Get("Range")
		var debut int
		fmt.Sscanf(rang, "bytes=%d-", &debut)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", debut, len(corps)-1, len(corps)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(corps[debut:])
	}))
	defer srv.Close()

	tmp := fichierTemp(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if _, _, _, err := telechargerCorps(context.Background(), srv.Client(), req, tmp); err != nil {
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
	attendu := sha256.Sum256(corps)
	if got != hex.EncodeToString(attendu[:]) {
		t.Fatalf("empreinte = %s, attendue %s", got, hex.EncodeToString(attendu[:]))
	}
}
