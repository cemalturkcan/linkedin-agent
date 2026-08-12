package postings

import (
	"regexp"
	"strings"
)

var (
	legalEntity = regexp.MustCompile(
		`\b(inc|llc|ltd|limited|gmbh|mbh|ag|bv|nv|ab|oy|aps|sa|sas|srl|spa|plc|pvt|corp|` +
			`corporation|a\s?s|sti)\b`,
	)
	titleNoise = regexp.MustCompile(
		`\b(m w d|m f d|f m d|m w x|w m d|d f m|all genders|remote|hybrid|onsite|on site|` +
			`full time|part time|fulltime|parttime|contract|freelance|intern|internship|urgent|` +
			`hiring|we are hiring)\b`,
	)
	bracketed  = regexp.MustCompile(`[(\[{][^)\]}]*[)\]}]`)
	nonWord    = regexp.MustCompile(`[^a-z0-9]+`)
	manySpaces = regexp.MustCompile(`\s+`)

	diacritics = strings.NewReplacer(
		"ı", "i", "İ", "i", "ş", "s", "Ş", "s", "ğ", "g", "Ğ", "g",
		"ü", "u", "Ü", "u", "ö", "o", "Ö", "o", "ç", "c", "Ç", "c",
		"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a", "ā", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e", "ē", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i", "ī", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ø", "o", "ō", "o",
		"ú", "u", "ù", "u", "û", "u", "ū", "u",
		"ñ", "n", "ń", "n", "ç", "c", "ć", "c", "č", "c", "š", "s", "ž", "z",
		"ý", "y", "ÿ", "y", "ß", "ss", "æ", "ae", "œ", "oe", "ł", "l",
	)
)

func fold(value string) string {
	return strings.ToLower(diacritics.Replace(value))
}

func CompanyKey(company string) string {
	folded := strings.ReplaceAll(strings.ReplaceAll(fold(company), ".", " "), ",", " ")
	cleaned := nonWord.ReplaceAllString(folded, " ")
	cleaned = legalEntity.ReplaceAllString(cleaned, " ")
	return strings.TrimSpace(manySpaces.ReplaceAllString(cleaned, " "))
}

func TitleKey(title string) string {
	folded := bracketed.ReplaceAllString(fold(title), " ")
	cleaned := nonWord.ReplaceAllString(folded, " ")
	cleaned = titleNoise.ReplaceAllString(cleaned, " ")
	return strings.TrimSpace(manySpaces.ReplaceAllString(cleaned, " "))
}

func DupeKey(company, title string) string {
	left := CompanyKey(company)
	right := TitleKey(title)
	if left == "" || right == "" {
		return ""
	}
	return left + "|" + right
}

func PlaceKey(name string) string {
	return strings.TrimSpace(manySpaces.ReplaceAllString(fold(name), " "))
}
