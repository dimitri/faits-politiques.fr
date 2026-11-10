package jorf

import "testing"

// La DILA ne garde que la dernière archive complète : le nom change à chaque
// rotation (le 13 juillet 2025 a disparu, HTTP 404, quand celle du 8 novembre
// 2026 est parue). On lit l'index plutôt que de figer un nom.
func TestLatestFullArchive(t *testing.T) {
	index := []byte(`<a href="Freemium_jorf_simple_20250713-140000.tar.gz">x</a>
<a href="JORFSIMPLE_20261109-212854.tar.gz">incrément</a>
<a href="Freemium_jorf_simple_20261108-140000.tar.gz">y</a>
<a href="Freemium_Legi_global_20260101-000000.tar.gz">autre fonds</a>`)
	if got := latestFullArchive(index); got != "Freemium_jorf_simple_20261108-140000.tar.gz" {
		t.Errorf("latestFullArchive = %q", got)
	}
	if got := latestFullArchive([]byte(`<a href="JORFSIMPLE_20261109-212854.tar.gz">`)); got != "" {
		t.Errorf("sans archive complète, attendu \"\", reçu %q", got)
	}
}
