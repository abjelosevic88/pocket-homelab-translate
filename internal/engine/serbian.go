package engine

import (
	"strings"
	"unicode"
)

// Serbian is written in two alphabets that map one to one (Vuk's
// alphabet), so going between them needs no engine.

var cyrToLat = map[rune]string{
	'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D", 'Ђ': "Đ", 'Е': "E", 'Ж': "Ž", 'З': "Z", 'И': "I",
	'Ј': "J", 'К': "K", 'Л': "L", 'Љ': "Lj", 'М': "M", 'Н': "N", 'Њ': "Nj", 'О': "O", 'П': "P", 'Р': "R",
	'С': "S", 'Т': "T", 'Ћ': "Ć", 'У': "U", 'Ф': "F", 'Х': "H", 'Ц': "C", 'Ч': "Č", 'Џ': "Dž", 'Ш': "Š",
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'ђ': "đ", 'е': "e", 'ж': "ž", 'з': "z", 'и': "i",
	'ј': "j", 'к': "k", 'л': "l", 'љ': "lj", 'м': "m", 'н': "n", 'њ': "nj", 'о': "o", 'п': "p", 'р': "r",
	'с': "s", 'т': "t", 'ћ': "ć", 'у': "u", 'ф': "f", 'х': "h", 'ц': "c", 'ч': "č", 'џ': "dž", 'ш': "š",
}

// ToLatin writes Serbian Cyrillic in Latin letters. A capital digraph
// before a capital (an all-caps word) stays all caps: "ЉУБАВ" → "LJUBAV".
func ToLatin(s string) string {
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range rs {
		l, ok := cyrToLat[r]
		if !ok {
			b.WriteRune(r)
			continue
		}
		if len(l) > 1 && unicode.IsUpper(r) {
			upperNext := i+1 < len(rs) && unicode.IsUpper(rs[i+1])
			upperPrev := i > 0 && unicode.IsUpper(rs[i-1])
			if upperNext || (upperPrev && (i+1 >= len(rs) || !unicode.IsLetter(rs[i+1]))) {
				l = strings.ToUpper(l)
			}
		}
		b.WriteString(l)
	}
	return b.String()
}

var latToCyr = map[string]rune{
	"A": 'А', "B": 'Б', "V": 'В', "G": 'Г', "D": 'Д', "Đ": 'Ђ', "E": 'Е', "Ž": 'Ж', "Z": 'З', "I": 'И',
	"J": 'Ј', "K": 'К', "L": 'Л', "M": 'М', "N": 'Н', "O": 'О', "P": 'П', "R": 'Р', "S": 'С', "T": 'Т',
	"Ć": 'Ћ', "U": 'У', "F": 'Ф', "H": 'Х', "C": 'Ц', "Č": 'Ч', "Š": 'Ш',
	"a": 'а', "b": 'б', "v": 'в', "g": 'г', "d": 'д', "đ": 'ђ', "e": 'е', "ž": 'ж', "z": 'з', "i": 'и',
	"j": 'ј', "k": 'к', "l": 'л', "m": 'м', "n": 'н', "o": 'о', "p": 'п', "r": 'р', "s": 'с', "t": 'т',
	"ć": 'ћ', "u": 'у', "f": 'ф', "h": 'х', "c": 'ц', "č": 'ч', "š": 'ш',
	"Lj": 'Љ', "LJ": 'Љ', "lj": 'љ', "Nj": 'Њ', "NJ": 'Њ', "nj": 'њ', "Dž": 'Џ', "DŽ": 'Џ', "dž": 'џ',
}

// ToCyrillic writes Serbian Latin in Cyrillic. Words that look foreign
// (they have q, w, x or y, or are web addresses) are left as they are.
func ToCyrillic(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 2)
	word := []rune{}
	flush := func() {
		if len(word) == 0 {
			return
		}
		b.WriteString(cyrWord(word))
		word = word[:0]
	}
	for _, r := range s {
		if unicode.IsLetter(r) || r == '@' || r == '.' && len(word) > 0 {
			word = append(word, r)
			continue
		}
		flush()
		b.WriteRune(r)
	}
	flush()
	return b.String()
}

func cyrWord(w []rune) string {
	s := string(w)
	trail := ""
	if strings.HasSuffix(s, ".") {
		s, trail = strings.TrimSuffix(s, "."), "."
	}
	if strings.ContainsAny(s, "qwxyQWXY@.") {
		return s + trail
	}
	rs := []rune(s)
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		if i+1 < len(rs) {
			if c, ok := latToCyr[string(rs[i:i+2])]; ok {
				b.WriteRune(c)
				i++
				continue
			}
		}
		if c, ok := latToCyr[string(rs[i])]; ok {
			b.WriteRune(c)
			continue
		}
		b.WriteRune(rs[i])
	}
	return b.String() + trail
}

// IsCyrillic reports whether most letters of s are Cyrillic.
func IsCyrillic(s string) bool {
	cyr, lat := 0, 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyr++
		case unicode.Is(unicode.Latin, r):
			lat++
		}
	}
	return cyr > lat
}

// Script puts text in the alphabet a Serbian code asks for (other codes: as is).
func Script(code, s string) string {
	switch code {
	case "sr-Latn", "cnr":
		if IsCyrillic(s) {
			return ToLatin(s)
		}
	case "sr-Cyrl":
		if !IsCyrillic(s) {
			return ToCyrillic(s)
		}
	}
	return s
}
