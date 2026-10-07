package sitegen

import (
	"bytes"
	"strings"
)

// Espaces insécables de la typographie française.
//
// La règle : une espace fine insécable AVANT les deux-points, le point-virgule,
// le point d'exclamation, le point d'interrogation ET LE SIGNE POURCENT ; une
// espace insécable après le guillemet ouvrant et avant le guillemet fermant.
// Sans elles, un navigateur coupe la ligne entre le nombre et son signe — une
// colonne de tableau affiche alors « 45,8\n% » — et le texte a l'air cassé.
//
// Pourquoi ici plutôt que dans chaque gabarit : la correction doit aussi porter
// sur les documents Markdown de « Comprendre », sur les libellés venus de la
// base et sur tout ce qui sera écrit demain. Le faire à l'écriture du fichier
// est le seul endroit qui les couvre tous.
//
// Ce qu'elle ne touche JAMAIS :
//
//   - l'intérieur des balises (un attribut href n'est pas du texte) ;
//   - <code>, <pre>, <script>, <style> : y insérer une espace insécable
//     changerait un identifiant, une requête SQL ou un chemin de fichier ;
//   - une ponctuation déjà précédée d'une espace insécable ou d'un &nbsp; ;
//   - « http:// » et les entités HTML, où le deux-points est de la syntaxe.
//
// L'espace fine (U+202F) est utilisée devant « ; : ! ? » et le guillemet
// fermant, l'insécable ordinaire (U+00A0) après le guillemet ouvrant : c'est la
// recommandation de l'Imprimerie nationale, et c'est ce que fait le reste du
// site pour les milliers.
const (
	fine        = " "
	nonbreaking = " "
)

var withoutTypo = map[string]bool{"code": true, "pre": true, "script": true, "style": true}

func fixTypography(html []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(html) + len(html)/64)

	var stack []string // balises ouvertes qu'on ne corrige pas
	i := 0
	for i < len(html) {
		lt := bytes.IndexByte(html[i:], '<')
		if lt < 0 {
			writeText(&out, html[i:], len(stack) == 0)
			break
		}
		writeText(&out, html[i:i+lt], len(stack) == 0)
		i += lt

		gt := bytes.IndexByte(html[i:], '>')
		if gt < 0 {
			out.Write(html[i:])
			break
		}
		tag := html[i : i+gt+1]
		out.Write(tag)
		i += gt + 1

		name, closing := nameTag(tag)
		if !withoutTypo[name] {
			continue
		}
		if closing {
			if n := len(stack); n > 0 && stack[n-1] == name {
				stack = stack[:n-1]
			}
		} else if !bytes.HasSuffix(tag, []byte("/>")) {
			stack = append(stack, name)
		}
	}
	return out.Bytes()
}

func nameTag(b []byte) (string, bool) {
	s := string(b)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	closing := strings.HasPrefix(s, "/")
	s = strings.TrimPrefix(s, "/")
	if j := strings.IndexAny(s, " \t\n/"); j >= 0 {
		s = s[:j]
	}
	return strings.ToLower(s), closing
}

// writeText applique la règle à un fragment de texte pur.
func writeText(out *bytes.Buffer, txt []byte, actif bool) {
	if !actif || len(txt) == 0 {
		out.Write(txt)
		return
	}
	s := string(txt)
	r := []rune(s)
	for k := 0; k < len(r); k++ {
		c := r[k]
		switch c {
		case ':', ';', '!', '?', '%':
			// Une ponctuation collée au mot précédent est déjà correcte en
			// anglais et dans les URL ; on n'agit que sur « mot blanc signe ».
			// Le blanc peut être un retour à la ligne du gabarit : le navigateur
			// le rend comme une espace, et couperait la ligne là.
			if k > 0 && isBlank(r[k-1]) && !inEntity(r, k) {
				removeBlankFinal(out)
				out.WriteString(fine)
			}
		case '«':
			out.WriteRune(c)
			if k+1 < len(r) && isBlank(r[k+1]) {
				out.WriteString(nonbreaking)
				for k+1 < len(r) && isBlank(r[k+1]) {
					k++
				}
			}
			continue
		case '»':
			if k > 0 && isBlank(r[k-1]) {
				removeBlankFinal(out)
				out.WriteString(fine)
			}
		}
		out.WriteRune(c)
	}
}

// inEntity : « &nbsp; » se termine par un point-virgule qui n'est pas de la
// ponctuation. On regarde en arrière jusqu'à une esperluette proche.
func inEntity(r []rune, k int) bool {
	if r[k] != ';' {
		return false
	}
	for j := k - 1; j >= 0 && k-j <= 10; j-- {
		if r[j] == '&' {
			return true
		}
		if r[j] == ' ' || r[j] == '<' {
			return false
		}
	}
	return false
}

func isBlank(c rune) bool { return c == ' ' || c == '\n' || c == '\t' || c == '\r' }

// removeBlankFinal enlève la suite de blancs déjà écrite : un gabarit peut
// avoir coupé sa ligne juste avant la ponctuation, ce qui donne « \n\t\t : ».
func removeBlankFinal(out *bytes.Buffer) {
	b := out.Bytes()
	n := len(b)
	for n > 0 && (b[n-1] == ' ' || b[n-1] == '\n' || b[n-1] == '\t' || b[n-1] == '\r') {
		n--
	}
	out.Truncate(n)
}
