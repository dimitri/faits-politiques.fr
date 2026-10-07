// Package balisage convertit un fragment balisé en texte lisible.
//
// Il existe parce que la réponse honnête à « est-ce qu'on parse ou est-ce
// qu'on bricole » était, à quatre endroits de ce dépôt, « on bricole ».
// `regexp.MustCompile("<[^>]+>")` paraît suffisant jusqu'au jour où le
// document contient `<p title="a>b">` : l'expression s'arrête au premier `>`,
// laisse `b">` dans le texte, et personne ne le voit. Une expression régulière
// ne sait pas ce qu'est une balise ; elle sait reconnaître une forme.
//
// Ici, le découpage est fait par `xml.Decoder`, qui est un vrai analyseur
// lexical : il connaît les attributs, les guillemets, les entités et les
// sections CDATA. Il est réglé en mode tolérant, parce que les fragments
// rencontrés sont du HTML enfermé dans du XML — balises non fermées, `<br>`
// solitaires, entités de traitement de texte :
//
//	Strict = false     n'échoue pas sur une balise mal fermée
//	AutoClose          ferme <br>, <img>, <hr> et leurs semblables
//	Entity = HTMLEntity connaît &nbsp;, &oelig;, &eacute; et les 250 autres
//
// Et si malgré tout le fragment est indécodable, la fonction rend le texte
// obtenu jusque-là plutôt que rien : un document tronqué reste préférable à un
// document absent, à condition que ce soit dit.
package balisage

import (
	"encoding/xml"
	"html"
	"regexp"
	"strings"
)

// Les balises qui séparent deux idées. Tout le reste — <em>, <span>, <a> —
// disparaît sans laisser d'espace : `<span>M</span>esdames` doit rendre
// « Mesdames » et non « M esdames ». Cette erreur-là a été commise, et publiée.
var blockElements = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "tr": true, "table": true,
	"blockquote": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "ul": true, "ol": true, "section": true,
	"article": true, "header": true, "footer": true, "hr": true,
}

// Les éléments dont le CONTENU n'est pas du texte à lire.
var silent = map[string]bool{"script": true, "style": true, "head": true}

var spaces = regexp.MustCompile(`[ \t\x{00a0}]+`)

// Text rend le contenu textuel d'un fragment balisé, les blocs séparés par
// des retours à la ligne.
func Text(fragment string) string {
	var b strings.Builder
	d := xml.NewDecoder(strings.NewReader("<fp-racine>" + fragment + "</fp-racine>"))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity

	silentDepth := 0
	for {
		t, err := d.Token()
		if err != nil {
			break // fin du fragment, ou fragment indécodable : on garde l'acquis
		}
		switch v := t.(type) {
		case xml.StartElement:
			name := strings.ToLower(v.Name.Local)
			if silent[name] {
				silentDepth++
			}
			if blockElements[name] {
				b.WriteString("\n")
			}
		case xml.EndElement:
			name := strings.ToLower(v.Name.Local)
			if silent[name] && silentDepth > 0 {
				silentDepth--
			}
			if blockElements[name] {
				b.WriteString("\n")
			}
		case xml.CharData:
			if silentDepth == 0 {
				b.Write(v)
			}
		}
	}
	return clean(b.String())
}

// clean normalise les blancs sans souder les lignes : les espaces d'une
// même ligne sont réduits à un, les lignes vides disparaissent.
func clean(s string) string {
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, l := range lines {
		l = strings.TrimSpace(spaces.ReplaceAllString(l, " "))
		if l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// Line rend le même texte sur une seule ligne. Pour les champs courts — un
// intitulé, un motif de déport — où le découpage en blocs n'apporte rien.
func Line(fragment string) string {
	return strings.Join(strings.Fields(Text(fragment)), " ")
}
