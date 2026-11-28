package seoanalysis

import "strings"

func set(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[Fold(w)] = true
	}
	return m
}

var functionWordsByLang = map[string]map[string]bool{
	"en": set(`a an the and or but nor so yet for of in on at to from by with without about above below into onto over under
		after before between through during is are was were be been being am do does did have has had i you he she it we they me
		him her us them my your his its our their this that these those what which who whom whose how why when where all any
		both each few more most other some such no not only own same than too very can will just should now as if then there
		here also up down out off again further once`),
	"fr": set(`le la les l un une des du de d au aux à a et ou mais donc or ni car ce cet cette ces mon ma mes ton ta tes son sa
		ses notre nos votre vos leur leurs je tu il elle on nous vous ils elles me te se lui y en qui que qu quoi dont où
		est sont était être avoir a ont été par pour sur sous dans avec sans chez entre vers selon pendant depuis plus moins
		très trop peu tout tous toute toutes ne pas ni si comme aussi bien c s n j m t`),
	"ar": set(`في من إلى على عن مع هذا هذه ذلك تلك التي الذي الذين و أو ثم لكن لا لم لن هو هي هم هن أن إن كان كانت قد ما`),
}

func functionWords(locale string) map[string]bool {
	if m, ok := functionWordsByLang[locale]; ok {
		return m
	}
	return functionWordsByLang["en"]
}

// transition words and phrases (abridged lists).
var transitionWords = map[string][]string{
	"en": {"accordingly", "additionally", "afterward", "afterwards", "also", "although", "altogether", "another", "because",
		"besides", "consequently", "conversely", "finally", "first", "firstly", "for example", "for instance", "furthermore",
		"hence", "however", "in addition", "in conclusion", "in contrast", "in fact", "in other words", "in short", "indeed",
		"instead", "lastly", "likewise", "meanwhile", "moreover", "nevertheless", "next", "nonetheless", "otherwise",
		"overall", "rather", "secondly", "similarly", "since", "so that", "specifically", "still", "subsequently", "then",
		"therefore", "thus", "to summarize", "ultimately", "unlike", "whereas", "while", "as a result", "on the other hand",
		"above all", "after all", "as well as", "even though", "in particular", "not only", "such as", "that is why"},
	"fr": {"ainsi", "alors", "après", "au contraire", "aussi", "autrement dit", "bref", "car", "cependant", "certes", "c'est pourquoi",
		"d'abord", "d'ailleurs", "de plus", "de même", "donc", "en bref", "en conclusion", "en effet", "en outre", "en particulier",
		"en revanche", "en somme", "enfin", "ensuite", "finalement", "malgré", "néanmoins", "notamment", "or", "par ailleurs",
		"par conséquent", "par contre", "par exemple", "pourtant", "premièrement", "puis", "quant à", "toutefois", "tout d'abord",
		"bien que", "afin de", "afin que", "grâce à", "c'est-à-dire", "de sorte que", "en résumé", "à condition que",
		"au final", "deuxièmement", "également", "en fait", "de ce fait", "pour conclure", "si bien que", "tandis que"},
}

// HasTransition reports whether a sentence holds a transition word.
func hasTransition(sentence, locale string) bool {
	list, ok := transitionWords[locale]
	if !ok {
		return false
	}
	s := " " + strings.Join(strings.Fields(Fold(sentence)), " ") + " "
	s = strings.NewReplacer(",", " ", ";", " ", ":", " ", ".", " ").Replace(s)
	s = " " + strings.Join(strings.Fields(s), " ") + " "
	for _, t := range list {
		if strings.Contains(s, " "+Fold(t)+" ") {
			return true
		}
	}
	return false
}

var (
	enBe          = set(`am is are was were be been being get gets got gotten`)
	enIrregular   = set(`made done seen given taken written known shown found built sold bought brought kept held left lost paid sent spent told thought caught taught chosen broken spoken driven eaten fallen forgotten frozen grown hidden ridden risen shaken stolen sworn torn thrown woken worn worn born put set cut hit hurt read run won`)
	frEtre        = set(`suis es est sommes êtes sont étais était étions étiez étaient serai seras sera serons serez seront serais serait seraient fus fut furent sois soit soient été être`)
	frParticiple  = []string{"é", "ée", "és", "ées"}
	frIrregular   = set(`fait faite faits faites pris prise prises mis mise mises dit dite dits dites écrit écrite écrits écrites vendu vendue vendus vendues conçu conçue conçus conçues produit produite produits produites construit construite construits construites ouvert ouverte ouverts ouvertes offert offerte offerts offertes couvert couverte couverts couvertes fini finie finis finies choisi choisie choisis choisies rempli remplie remplis remplies garanti garantie garantis garanties vu vue vus vues connu connue connus connues reçu reçue reçus reçues tenu tenue tenus tenues fourni fournie fournis fournies`)
	frNotPassiveA = set(`très bien plus moins pas trop aussi déjà souvent toujours`)
)

// isPassive is a rule-of-thumb passive voice detection: a form of "to be"
// (être) followed, within two words, by a past participle.
func isPassive(sentence, locale string) bool {
	words := Words(sentence)
	for i, w := range words {
		lw := strings.ToLower(w)
		switch locale {
		case "en":
			if !enBe[lw] {
				continue
			}
			for j := i + 1; j < len(words) && j <= i+2; j++ {
				n := strings.ToLower(words[j])
				if (strings.HasSuffix(n, "ed") && len(n) > 4) || enIrregular[n] {
					return true
				}
			}
		case "fr":
			if !frEtre[Fold(lw)] && !frEtre[lw] {
				continue
			}
			for j := i + 1; j < len(words) && j <= i+2; j++ {
				n := strings.ToLower(words[j])
				if frNotPassiveA[n] {
					continue
				}
				if len([]rune(n)) < 4 {
					break
				}
				if frIrregular[Fold(n)] {
					return true
				}
				for _, suf := range frParticiple {
					if strings.HasSuffix(n, suf) {
						return true
					}
				}
				break
			}
		}
	}
	return false
}
