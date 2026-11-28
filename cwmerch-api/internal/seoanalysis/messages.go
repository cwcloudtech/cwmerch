package seoanalysis

import "fmt"

type msg struct {
	title string
	texts map[string]string
}

// message is an assessment's title and text in the language (fmt verbs filled
// with args), falling back to English.
func message(lang, id, key string, args ...any) (string, string) {
	m, ok := catalog[lang][id]
	if !ok {
		m = catalog["en"][id]
	}
	t, ok := m.texts[key]
	if !ok {
		t = catalog["en"][id].texts[key]
	}
	if len(args) > 0 {
		t = fmt.Sprintf(t, args...)
	}
	return m.title, t
}

var catalog = map[string]map[string]msg{
	"en": {
		"keyphraseLength": {"Keyphrase length", map[string]string{
			"none":    "No focus keyphrase was set for this page. Set a keyphrase in order to calculate your SEO score.",
			"good":    "Good job!",
			"long":    "The keyphrase is %d words long. Try to make it shorter.",
			"tooLong": "The keyphrase is %d words long. That's way more than the recommended maximum of 4 words. Make it shorter!"}},
		"keyphraseIntroduction": {"Keyphrase in introduction", map[string]string{
			"good":            "Well done!",
			"notSameSentence": "Your keyphrase or its synonyms appear in the first paragraph, but not within one sentence. Fix that!",
			"bad":             "Your keyphrase or its synonyms do not appear in the first paragraph. Make sure the topic is clear immediately."}},
		"keyphraseDensity": {"Keyphrase density", map[string]string{
			"none": "The focus keyphrase was found 0 times. Focus on your keyphrase!",
			"low":  "The focus keyphrase was found %d times (%s%%). That's less than recommended (0.5%%). Focus on your keyphrase!",
			"high": "The focus keyphrase was found %d times (%s%%). That's more than recommended (3%%). Don't overoptimize!",
			"good": "The focus keyphrase was found %d times (%s%%). This is great!"}},
		"metaDescriptionKeyphrase": {"Keyphrase in meta description", map[string]string{
			"none":    "The meta description has been specified, but it does not contain the keyphrase. Fix that!",
			"good":    "Keyphrase or synonym appear in the meta description. Well done!",
			"tooMany": "The meta description contains the keyphrase %d times, which is over the advised maximum of 2 times. Limit that!"}},
		"titleKeyphrase": {"Keyphrase in SEO title", map[string]string{
			"good":       "The exact match of the focus keyphrase appears at the beginning of the SEO title. Good job!",
			"notAtStart": "The SEO title contains the keyphrase, but not at the beginning. Move it to the beginning of the title.",
			"bad":        "Not all the words of your keyphrase appear in the SEO title. Use the exact match of your keyphrase in the SEO title."}},
		"slugKeyphrase": {"Keyphrase in slug", map[string]string{
			"good": "Great work!",
			"bad":  "(Part of) your keyphrase does not appear in the slug. Change that!"}},
		"subheadingsKeyphrase": {"Keyphrase in subheading", map[string]string{
			"none":    "Use more keyphrases or synonyms in your H2 and H3 subheadings!",
			"few":     "%d of your %d subheadings reflect the topic of your copy. Use the keyphrase in more of them.",
			"tooMany": "%d of your %d subheadings contain the keyphrase. Don't overoptimize!",
			"good":    "%d of your %d higher-level subheadings reflect the topic of your copy. Good job!"}},
		"imageKeyphrase": {"Image keyphrase", map[string]string{
			"none":    "Images on this page do not have alt attributes with at least half of the words from your keyphrase. Fix that!",
			"few":     "Out of %d images, only %d have alt attributes that reflect the topic. Add your keyphrase to more of them.",
			"tooMany": "%d out of %d images have alt attributes with the keyphrase. That's a bit much: only include it when it fits the image.",
			"good":    "Good job! %d of %d images have alt attributes with the keyphrase."}},
		"previouslyUsedKeyphrase": {"Previously used keyphrase", map[string]string{
			"good": "You've not used this keyphrase before, very good.",
			"once": "You've used this keyphrase before (on \"%s\"). Do not use your focus keyphrase more than once.",
			"many": "You've used this keyphrase %d times before (%s). Do not use your focus keyphrase more than once."}},
		"competingLinks": {"Competing links", map[string]string{
			"bad": "You're linking to another page with the words you want this page to rank for (%d link(s)). Don't do that!"}},
		"metaDescriptionLength": {"Meta description length", map[string]string{
			"none":  "No meta description has been specified. Search engines will display copy from the page instead. Make sure to write one!",
			"short": "The meta description is too short (%d characters, up to 156 available). Use the space!",
			"long":  "The meta description is over 156 characters (%d). Search engines will cut it. Reduce the length!",
			"good":  "Well done! (%d characters)"}},
		"titleWidth": {"SEO title width", map[string]string{
			"none":  "Please create an SEO title.",
			"short": "The SEO title is too short. Use the space to add keyphrase variations or create compelling call-to-action copy.",
			"long":  "The SEO title is wider than the viewable limit. Try to make it shorter.",
			"good":  "Good job!"}},
		"textLength": {"Text length", map[string]string{
			"good":     "The text contains %d words. Good job!",
			"short":    "The text contains %d words. That's below the recommended minimum of %d words. Add more content.",
			"tooShort": "The text contains %d words. That's far below the recommended minimum of %d words. Add more content."}},
		"internalLinks": {"Internal links", map[string]string{
			"good": "You have enough internal links (%d). Good job!",
			"none": "No internal links appear in this page. Make sure to add some!"}},
		"outboundLinks": {"Outbound links", map[string]string{
			"good":        "Good job! (%d outbound link(s))",
			"none":        "No outbound links appear in this page. Add some!",
			"allNofollow": "All outbound links on this page are nofollowed. Add some normal links."}},
		"images": {"Images", map[string]string{
			"good": "Good job! (%d image(s))",
			"none": "No images appear on this page. Add some!"}},
		"singleH1": {"Single title", map[string]string{
			"bad": "The text contains %d H1 heading(s): the page's name already is its H1. Use H2 subheadings instead."}},
		"shortDescription": {"Product short description", map[string]string{
			"none":  "Please provide a short description for this product.",
			"short": "The short description is %d words long: make it at least 20 words.",
			"long":  "The short description is %d words long: keep it under 50 words.",
			"good":  "Your short description has a good length (%d words)."}},
		"sku": {"SKU", map[string]string{
			"good": "Your product has a SKU, good job!",
			"none": "Your product is missing a SKU. Include it if you can, as it will help search engines understand your content."}},
		"productIdentifiers": {"Product identifier", map[string]string{
			"good": "Your product has a GTIN / UPC / EAN / ISBN identifier, good job!",
			"none": "Your product is missing an identifier (GTIN, UPC, EAN or ISBN). Include it if you can, as it will help search engines."}},
		"textPresence": {"Not enough content", map[string]string{
			"bad": "The text has %d words: please add some content to enable a good analysis."}},
		"subheadingDistribution": {"Subheading distribution", map[string]string{
			"shortText": "You are not using any subheadings, but your text is short enough and probably doesn't need them.",
			"none":      "You are not using any subheadings, although your text is rather long. Try and add some subheadings.",
			"long":      "A section of your text is longer than 300 words (%d) and is not separated by any subheadings. Add subheadings to improve readability.",
			"good":      "Great job!"}},
		"paragraphLength": {"Paragraph length", map[string]string{
			"good": "None of the paragraphs are too long. Great job!",
			"long": "%d of the paragraphs contain more than the recommended maximum of 150 words. Shorten your paragraphs!"}},
		"sentenceLength": {"Sentence length", map[string]string{
			"good": "Great!",
			"long": "%s%% of the sentences contain more than 20 words, which is more than the recommended maximum of 25%%. Try to shorten the sentences."}},
		"consecutiveSentences": {"Consecutive sentences", map[string]string{
			"good": "There is enough variety in your sentences. That's great!",
			"bad":  "The text contains %d instance(s) of 3 or more consecutive sentences starting with the same word. Try to mix things up!"}},
		"passiveVoice": {"Passive voice", map[string]string{
			"good": "You are not using too much passive voice. That's great!",
			"high": "%s%% of the sentences contain passive voice, which is more than the recommended maximum of 10%%. Try to use their active counterparts."}},
		"transitionWords": {"Transition words", map[string]string{
			"good": "Well done!",
			"few":  "Only %s%% of the sentences contain transition words, which is not enough. Use more of them."}},
		"fleschReadingEase": {"Flesch Reading Ease", map[string]string{
			"veryEasy":        "The copy scores %s in the test, which is considered very easy to read. Good job!",
			"easy":            "The copy scores %s in the test, which is considered easy to read. Good job!",
			"ok":              "The copy scores %s in the test, which is considered ok to read. Good job!",
			"fairlyDifficult": "The copy scores %s in the test, which is considered fairly difficult to read. Try to make shorter sentences to improve readability.",
			"difficult":       "The copy scores %s in the test, which is considered difficult to read. Try to make shorter sentences, using less difficult words.",
			"veryDifficult":   "The copy scores %s in the test, which is considered very difficult to read. Try to make shorter sentences, using less difficult words."}},
	},
	"fr": {
		"keyphraseLength": {"Longueur de la requête cible", map[string]string{
			"none":    "Aucune requête cible n'a été définie pour cette page. Définissez-en une pour calculer votre score SEO.",
			"good":    "Bon travail !",
			"long":    "La requête cible contient %d mots. Essayez de la raccourcir.",
			"tooLong": "La requête cible contient %d mots, bien plus que le maximum recommandé de 4 mots. Raccourcissez-la !"}},
		"keyphraseIntroduction": {"Requête dans l'introduction", map[string]string{
			"good":            "Bien joué !",
			"notSameSentence": "Votre requête apparaît dans le premier paragraphe, mais pas dans une même phrase. Corrigez cela !",
			"bad":             "Votre requête n'apparaît pas dans le premier paragraphe. Assurez-vous que le sujet soit clair immédiatement."}},
		"keyphraseDensity": {"Densité de la requête", map[string]string{
			"none": "La requête cible a été trouvée 0 fois. Concentrez-vous sur votre requête !",
			"low":  "La requête cible a été trouvée %d fois (%s %%). C'est moins que recommandé (0,5 %%). Concentrez-vous sur votre requête !",
			"high": "La requête cible a été trouvée %d fois (%s %%). C'est plus que recommandé (3 %%). Ne sur-optimisez pas !",
			"good": "La requête cible a été trouvée %d fois (%s %%). C'est parfait !"}},
		"metaDescriptionKeyphrase": {"Requête dans la méta description", map[string]string{
			"none":    "La méta description a été définie, mais elle ne contient pas la requête. Corrigez cela !",
			"good":    "La requête apparaît dans la méta description. Bien joué !",
			"tooMany": "La méta description contient %d fois la requête, au-delà du maximum conseillé de 2. Limitez-la !"}},
		"titleKeyphrase": {"Requête dans le méta titre", map[string]string{
			"good":       "La requête cible exacte apparaît au début du méta titre. Bon travail !",
			"notAtStart": "Le méta titre contient la requête, mais pas au début. Déplacez-la au début du titre.",
			"bad":        "Tous les mots de votre requête n'apparaissent pas dans le méta titre. Utilisez la requête exacte dans le titre."}},
		"slugKeyphrase": {"Requête dans le slug", map[string]string{
			"good": "Excellent travail !",
			"bad":  "(Une partie de) votre requête n'apparaît pas dans le slug. Changez cela !"}},
		"subheadingsKeyphrase": {"Requête dans les intertitres", map[string]string{
			"none":    "Utilisez davantage votre requête ou ses synonymes dans vos intertitres H2 et H3 !",
			"few":     "%d de vos %d intertitres reflètent le sujet de votre texte. Utilisez la requête dans davantage d'entre eux.",
			"tooMany": "%d de vos %d intertitres contiennent la requête. Ne sur-optimisez pas !",
			"good":    "%d de vos %d intertitres reflètent le sujet de votre texte. Bon travail !"}},
		"imageKeyphrase": {"Requête dans les images", map[string]string{
			"none":    "Les images de cette page n'ont pas d'attribut alt contenant votre requête. Corrigez cela !",
			"few":     "Sur %d images, seules %d ont un attribut alt qui reflète le sujet. Ajoutez votre requête à davantage d'entre elles.",
			"tooMany": "%d images sur %d ont la requête dans leur attribut alt. C'est un peu trop : ne l'utilisez que quand elle décrit l'image.",
			"good":    "Bon travail ! %d images sur %d ont la requête dans leur attribut alt."}},
		"previouslyUsedKeyphrase": {"Requête déjà utilisée", map[string]string{
			"good": "Vous n'avez jamais utilisé cette requête, très bien.",
			"once": "Vous avez déjà utilisé cette requête (sur « %s »). N'utilisez pas votre requête cible plus d'une fois.",
			"many": "Vous avez déjà utilisé cette requête %d fois (%s). N'utilisez pas votre requête cible plus d'une fois."}},
		"competingLinks": {"Liens concurrents", map[string]string{
			"bad": "Vous faites un lien vers une autre page avec les mots pour lesquels cette page doit se positionner (%d lien(s)). Évitez-le !"}},
		"metaDescriptionLength": {"Longueur de la méta description", map[string]string{
			"none":  "Aucune méta description n'a été définie. Les moteurs de recherche afficheront un extrait de la page. Rédigez-en une !",
			"short": "La méta description est trop courte (%d caractères, jusqu'à 156 disponibles). Utilisez l'espace !",
			"long":  "La méta description dépasse 156 caractères (%d). Les moteurs de recherche la couperont. Raccourcissez-la !",
			"good":  "Bien joué ! (%d caractères)"}},
		"titleWidth": {"Largeur du méta titre", map[string]string{
			"none":  "Veuillez créer un méta titre.",
			"short": "Le méta titre est trop court. Utilisez l'espace pour ajouter des variantes de la requête ou un appel à l'action.",
			"long":  "Le méta titre dépasse la largeur affichable. Essayez de le raccourcir.",
			"good":  "Bon travail !"}},
		"textLength": {"Longueur du texte", map[string]string{
			"good":     "Le texte contient %d mots. Bon travail !",
			"short":    "Le texte contient %d mots, en dessous du minimum recommandé de %d mots. Ajoutez du contenu.",
			"tooShort": "Le texte contient %d mots, très en dessous du minimum recommandé de %d mots. Ajoutez du contenu."}},
		"internalLinks": {"Liens internes", map[string]string{
			"good": "Vous avez suffisamment de liens internes (%d). Bon travail !",
			"none": "Aucun lien interne n'apparaît sur cette page. Ajoutez-en !"}},
		"outboundLinks": {"Liens sortants", map[string]string{
			"good":        "Bon travail ! (%d lien(s) sortant(s))",
			"none":        "Aucun lien sortant n'apparaît sur cette page. Ajoutez-en !",
			"allNofollow": "Tous les liens sortants de cette page sont en nofollow. Ajoutez des liens normaux."}},
		"images": {"Images", map[string]string{
			"good": "Bon travail ! (%d image(s))",
			"none": "Aucune image n'apparaît sur cette page. Ajoutez-en !"}},
		"singleH1": {"Titre unique", map[string]string{
			"bad": "Le texte contient %d titre(s) H1 : le nom de la page est déjà son H1. Utilisez plutôt des intertitres H2."}},
		"shortDescription": {"Description courte du produit", map[string]string{
			"none":  "Veuillez rédiger une description courte pour ce produit.",
			"short": "La description courte fait %d mots : visez au moins 20 mots.",
			"long":  "La description courte fait %d mots : restez sous 50 mots.",
			"good":  "Votre description courte a une bonne longueur (%d mots)."}},
		"sku": {"UGS (SKU)", map[string]string{
			"good": "Votre produit a une UGS, bon travail !",
			"none": "Votre produit n'a pas d'UGS. Ajoutez-la si possible : elle aide les moteurs de recherche à comprendre votre contenu."}},
		"productIdentifiers": {"Identifiant produit", map[string]string{
			"good": "Votre produit a un identifiant GTIN / UPC / EAN / ISBN, bon travail !",
			"none": "Votre produit n'a pas d'identifiant (GTIN, UPC, EAN ou ISBN). Ajoutez-le si possible, il aide les moteurs de recherche."}},
		"textPresence": {"Pas assez de contenu", map[string]string{
			"bad": "Le texte contient %d mots : ajoutez du contenu pour permettre une bonne analyse."}},
		"subheadingDistribution": {"Répartition des intertitres", map[string]string{
			"shortText": "Vous n'utilisez pas d'intertitres, mais votre texte est assez court pour ne probablement pas en avoir besoin.",
			"none":      "Vous n'utilisez pas d'intertitres alors que votre texte est plutôt long. Ajoutez-en.",
			"long":      "Une section de votre texte dépasse 300 mots (%d) sans intertitre. Ajoutez des intertitres pour améliorer la lisibilité.",
			"good":      "Excellent travail !"}},
		"paragraphLength": {"Longueur des paragraphes", map[string]string{
			"good": "Aucun paragraphe n'est trop long. Excellent travail !",
			"long": "%d paragraphe(s) dépasse(nt) le maximum recommandé de 150 mots. Raccourcissez vos paragraphes !"}},
		"sentenceLength": {"Longueur des phrases", map[string]string{
			"good": "Parfait !",
			"long": "%s %% des phrases contiennent plus de 20 mots, au-delà du maximum recommandé de 25 %%. Essayez de raccourcir vos phrases."}},
		"consecutiveSentences": {"Phrases consécutives", map[string]string{
			"good": "Vos phrases sont suffisamment variées. Parfait !",
			"bad":  "Le texte contient %d série(s) d'au moins 3 phrases consécutives commençant par le même mot. Variez-les !"}},
		"passiveVoice": {"Voix passive", map[string]string{
			"good": "Vous n'utilisez pas trop la voix passive. Parfait !",
			"high": "%s %% des phrases sont à la voix passive, au-delà du maximum recommandé de 10 %%. Préférez la voix active."}},
		"transitionWords": {"Mots de transition", map[string]string{
			"good": "Bien joué !",
			"few":  "Seules %s %% des phrases contiennent des mots de transition, ce n'est pas assez. Utilisez-en davantage."}},
		"fleschReadingEase": {"Facilité de lecture Flesch", map[string]string{
			"veryEasy":        "Le texte obtient %s au test, ce qui le rend très facile à lire. Bon travail !",
			"easy":            "Le texte obtient %s au test, ce qui le rend facile à lire. Bon travail !",
			"ok":              "Le texte obtient %s au test, ce qui le rend correct à lire. Bon travail !",
			"fairlyDifficult": "Le texte obtient %s au test, ce qui le rend assez difficile à lire. Faites des phrases plus courtes.",
			"difficult":       "Le texte obtient %s au test, ce qui le rend difficile à lire. Faites des phrases plus courtes avec des mots plus simples.",
			"veryDifficult":   "Le texte obtient %s au test, ce qui le rend très difficile à lire. Faites des phrases plus courtes avec des mots plus simples."}},
	},
	"ar": {
		"keyphraseLength": {"طول العبارة المفتاحية", map[string]string{
			"none":    "لم يتم تحديد عبارة مفتاحية لهذه الصفحة. حدّد واحدة لحساب نتيجة تحسين محركات البحث.",
			"good":    "عمل جيد!",
			"long":    "العبارة المفتاحية تتكون من %d كلمات. حاول اختصارها.",
			"tooLong": "العبارة المفتاحية تتكون من %d كلمات، أكثر بكثير من الحد الأقصى الموصى به (4 كلمات). اختصرها!"}},
		"keyphraseIntroduction": {"العبارة المفتاحية في المقدمة", map[string]string{
			"good":            "أحسنت!",
			"notSameSentence": "تظهر العبارة المفتاحية في الفقرة الأولى ولكن ليس في جملة واحدة. صحّح ذلك!",
			"bad":             "لا تظهر العبارة المفتاحية في الفقرة الأولى. تأكد من وضوح الموضوع من البداية."}},
		"keyphraseDensity": {"كثافة العبارة المفتاحية", map[string]string{
			"none": "لم يتم العثور على العبارة المفتاحية. ركّز على عبارتك!",
			"low":  "تم العثور على العبارة المفتاحية %d مرات (%s٪)، أقل من الموصى به (0.5٪).",
			"high": "تم العثور على العبارة المفتاحية %d مرات (%s٪)، أكثر من الموصى به (3٪). لا تبالغ في التحسين!",
			"good": "تم العثور على العبارة المفتاحية %d مرات (%s٪). ممتاز!"}},
		"metaDescriptionKeyphrase": {"العبارة المفتاحية في الوصف التعريفي", map[string]string{
			"none":    "الوصف التعريفي لا يحتوي على العبارة المفتاحية. صحّح ذلك!",
			"good":    "تظهر العبارة المفتاحية في الوصف التعريفي. أحسنت!",
			"tooMany": "يحتوي الوصف التعريفي على العبارة المفتاحية %d مرات، أكثر من الحد الأقصى (مرتان)."}},
		"titleKeyphrase": {"العبارة المفتاحية في عنوان SEO", map[string]string{
			"good":       "تظهر العبارة المفتاحية في بداية عنوان SEO. عمل جيد!",
			"notAtStart": "يحتوي العنوان على العبارة المفتاحية ولكن ليس في بدايته. انقلها إلى البداية.",
			"bad":        "لا تظهر كل كلمات العبارة المفتاحية في عنوان SEO. استخدمها كما هي في العنوان."}},
		"slugKeyphrase": {"العبارة المفتاحية في الرابط", map[string]string{
			"good": "عمل رائع!",
			"bad":  "العبارة المفتاحية (أو جزء منها) لا تظهر في الرابط. غيّر ذلك!"}},
		"subheadingsKeyphrase": {"العبارة المفتاحية في العناوين الفرعية", map[string]string{
			"none":    "استخدم العبارة المفتاحية أكثر في عناوين H2 وH3!",
			"few":     "%d من %d عناوين فرعية تعكس الموضوع. استخدم العبارة في المزيد منها.",
			"tooMany": "%d من %d عناوين فرعية تحتوي على العبارة المفتاحية. لا تبالغ في التحسين!",
			"good":    "%d من %d عناوين فرعية تعكس موضوع النص. عمل جيد!"}},
		"imageKeyphrase": {"العبارة المفتاحية في الصور", map[string]string{
			"none":    "لا تحتوي الصور على نص بديل (alt) يتضمن العبارة المفتاحية. صحّح ذلك!",
			"few":     "من بين %d صور، %d فقط لها نص بديل يعكس الموضوع.",
			"tooMany": "%d من %d صور تحتوي على العبارة المفتاحية في النص البديل. هذا كثير بعض الشيء.",
			"good":    "عمل جيد! %d من %d صور تحتوي على العبارة المفتاحية في النص البديل."}},
		"previouslyUsedKeyphrase": {"عبارة مفتاحية مستخدمة سابقًا", map[string]string{
			"good": "لم تستخدم هذه العبارة من قبل، جيد جدًا.",
			"once": "سبق أن استخدمت هذه العبارة (في «%s»). لا تستخدم العبارة المفتاحية أكثر من مرة.",
			"many": "سبق أن استخدمت هذه العبارة %d مرات (%s). لا تستخدمها أكثر من مرة."}},
		"competingLinks": {"روابط منافسة", map[string]string{
			"bad": "أنت تربط صفحة أخرى بالكلمات التي تريد أن تتصدر بها هذه الصفحة (%d رابط). تجنّب ذلك!"}},
		"metaDescriptionLength": {"طول الوصف التعريفي", map[string]string{
			"none":  "لم يتم تحديد وصف تعريفي. ستعرض محركات البحث نصًا من الصفحة. اكتب واحدًا!",
			"short": "الوصف التعريفي قصير جدًا (%d حرفًا، حتى 156 متاحة).",
			"long":  "الوصف التعريفي يتجاوز 156 حرفًا (%d). ستقصّه محركات البحث.",
			"good":  "أحسنت! (%d حرفًا)"}},
		"titleWidth": {"عرض عنوان SEO", map[string]string{
			"none":  "يرجى إنشاء عنوان SEO.",
			"short": "عنوان SEO قصير جدًا. استخدم المساحة لإضافة صيغ من العبارة أو دعوة لاتخاذ إجراء.",
			"long":  "عنوان SEO أعرض من الحد المرئي. حاول اختصاره.",
			"good":  "عمل جيد!"}},
		"textLength": {"طول النص", map[string]string{
			"good":     "يحتوي النص على %d كلمة. عمل جيد!",
			"short":    "يحتوي النص على %d كلمة، أقل من الحد الأدنى الموصى به (%d). أضف المزيد من المحتوى.",
			"tooShort": "يحتوي النص على %d كلمة، أقل بكثير من الحد الأدنى الموصى به (%d). أضف المزيد من المحتوى."}},
		"internalLinks": {"الروابط الداخلية", map[string]string{
			"good": "لديك روابط داخلية كافية (%d). عمل جيد!",
			"none": "لا توجد روابط داخلية في هذه الصفحة. أضف بعضها!"}},
		"outboundLinks": {"الروابط الخارجية", map[string]string{
			"good":        "عمل جيد! (%d رابط خارجي)",
			"none":        "لا توجد روابط خارجية في هذه الصفحة. أضف بعضها!",
			"allNofollow": "كل الروابط الخارجية في هذه الصفحة nofollow. أضف روابط عادية."}},
		"images": {"الصور", map[string]string{
			"good": "عمل جيد! (%d صورة)",
			"none": "لا توجد صور في هذه الصفحة. أضف بعضها!"}},
		"singleH1": {"عنوان واحد", map[string]string{
			"bad": "يحتوي النص على %d عنوان H1: اسم الصفحة هو بالفعل عنوانها H1. استخدم عناوين H2."}},
		"shortDescription": {"الوصف القصير للمنتج", map[string]string{
			"none":  "يرجى كتابة وصف قصير لهذا المنتج.",
			"short": "الوصف القصير %d كلمة: اجعله 20 كلمة على الأقل.",
			"long":  "الوصف القصير %d كلمة: اجعله أقل من 50 كلمة.",
			"good":  "طول الوصف القصير جيد (%d كلمة)."}},
		"sku": {"رمز المنتج (SKU)", map[string]string{
			"good": "لمنتجك رمز SKU، عمل جيد!",
			"none": "منتجك بدون رمز SKU. أضفه إن أمكن لمساعدة محركات البحث."}},
		"productIdentifiers": {"معرّف المنتج", map[string]string{
			"good": "لمنتجك معرّف GTIN / UPC / EAN / ISBN، عمل جيد!",
			"none": "منتجك بدون معرّف (GTIN أو UPC أو EAN أو ISBN). أضفه إن أمكن."}},
		"textPresence": {"محتوى غير كافٍ", map[string]string{
			"bad": "يحتوي النص على %d كلمة: أضف محتوى لتحليل أفضل."}},
		"subheadingDistribution": {"توزيع العناوين الفرعية", map[string]string{
			"shortText": "لا تستخدم عناوين فرعية، لكن نصك قصير بما يكفي.",
			"none":      "لا تستخدم عناوين فرعية رغم أن نصك طويل. أضف بعضها.",
			"long":      "قسم من نصك يتجاوز 300 كلمة (%d) دون عنوان فرعي.",
			"good":      "عمل رائع!"}},
		"paragraphLength": {"طول الفقرات", map[string]string{
			"good": "لا توجد فقرات طويلة جدًا. عمل رائع!",
			"long": "%d فقرة تتجاوز الحد الأقصى الموصى به (150 كلمة). اختصر فقراتك!"}},
		"sentenceLength": {"طول الجمل", map[string]string{
			"good": "ممتاز!",
			"long": "%s٪ من الجمل تتجاوز 20 كلمة، أكثر من الحد الأقصى الموصى به (25٪)."}},
		"consecutiveSentences": {"الجمل المتتالية", map[string]string{
			"good": "جملك متنوعة بما يكفي. ممتاز!",
			"bad":  "يحتوي النص على %d حالة من 3 جمل متتالية أو أكثر تبدأ بنفس الكلمة. نوّع!"}},
	},
}
