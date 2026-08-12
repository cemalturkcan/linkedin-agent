package resumes

import (
	"regexp"
	"strings"
)

const DefaultLanguage = "en"

var languageSuffix = regexp.MustCompile(`[-_]([a-zA-Z]{2})$`)

var isoLanguages = index(strings.Fields(`
aa ab ae af ak am an ar as av ay az ba be bg bh bi bm bn bo br bs ca ce ch co cr cs cu cv cy
da de dv dz ee el en eo es et eu fa ff fi fj fo fr fy ga gd gl gn gu gv ha he hi ho hr ht hu
hy hz ia id ie ig ii ik io is it iu ja jv ka kg ki kj kk kl km kn ko kr ks ku kv kw ky la lb
lg li ln lo lt lu lv mg mh mi mk ml mn mr ms mt my na nb nd ne ng nl nn no nr nv ny oc oj om
or os pa pi pl ps pt qu rm rn ro ru rw sa sc sd se sg sh si sk sl sm sn so sq sr ss st su sv
sw ta te tg th ti tk tl tn to tr ts tt tw ty ug uk ur uz ve vi vo wa wo xh yi yo za zh zu
`))

func index(codes []string) map[string]struct{} {
	known := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		known[code] = struct{}{}
	}
	return known
}

var documentWords = map[string]struct{}{"cv": {}}

func isLanguage(candidate string) bool {
	lowered := strings.ToLower(candidate)
	if _, names := documentWords[lowered]; names {
		return false
	}
	_, known := isoLanguages[lowered]
	return known
}

func languageOfSuffix(label string) string {
	match := languageSuffix.FindStringSubmatch(label)
	if match == nil {
		return ""
	}
	code := strings.ToLower(match[1])
	if !isLanguage(code) {
		return ""
	}
	return code
}

func withoutLanguage(label string) string {
	if languageOfSuffix(label) == "" {
		return label
	}
	return label[:len(label)-3]
}

func languageOfFile(stem, folderLanguage string) string {
	if folderLanguage != "" {
		return folderLanguage
	}
	if isLanguage(stem) {
		return strings.ToLower(stem)
	}
	if suffix := languageOfSuffix(stem); suffix != "" {
		return suffix
	}
	return DefaultLanguage
}
