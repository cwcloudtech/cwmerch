package woo

import (
	"sort"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// isoCountries is every ISO 3166-1 alpha-2 code WooCommerce sells to, grouped
// by WooCommerce's continent codes (used by shipping zones).
var continentCountries = map[string]string{
	"AF": "AO BF BI BJ BW CD CF CG CI CM CV DJ DZ EG EH ER ET GA GH GM GN GQ GW KE KM LR LS LY MA MG ML MR MU MW MZ NA NE NG RE RW SC SD SH SL SN SO SS ST SZ TD TG TN TZ UG YT ZA ZM ZW",
	"AN": "AQ BV GS HM TF",
	"AS": "AE AF AM AZ BD BH BN BT CC CN CX CY GE HK ID IL IN IO IQ IR JO JP KG KH KP KR KW KZ LA LB LK MM MN MO MV MY NP OM PH PK PS QA SA SG SY TH TJ TL TM TR TW UZ VN YE",
	"EU": "AD AL AT AX BA BE BG BY CH CZ DE DK EE ES FI FO FR GB GG GI GR HR HU IE IM IS IT JE LI LT LU LV MC MD ME MK MT NL NO PL PT RO RS RU SE SI SJ SK SM UA VA XK",
	"NA": "AG AI AW BB BL BM BQ BS BZ CA CR CU CW DM DO GD GL GP GT HN HT JM KN KY LC MF MQ MS MX NI PA PM PR SV SX TC TT US VC VG VI",
	"OC": "AS AU CK FJ FM GU KI MH MP NC NF NR NU NZ PF PG PN PW SB TK TO TV UM VU WF WS",
	"SA": "AR BO BR CL CO EC FK GF GY PE PY SR UY VE",
}

var continentNames = map[string]string{"AF": "Africa", "AN": "Antarctica", "AS": "Asia", "EU": "Europe",
	"NA": "North America", "OC": "Oceania", "SA": "South America"}

// State is a WooCommerce state/province.
type State struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Country is a country with its states (WooCommerce lists states for a few).
type Country struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Continent string  `json:"continent"`
	States    []State `json:"states"`
}

var countryStates = map[string]string{
	"US": "AL:Alabama AK:Alaska AZ:Arizona AR:Arkansas CA:California CO:Colorado CT:Connecticut DE:Delaware DC:District_of_Columbia FL:Florida GA:Georgia HI:Hawaii ID:Idaho IL:Illinois IN:Indiana IA:Iowa KS:Kansas KY:Kentucky LA:Louisiana ME:Maine MD:Maryland MA:Massachusetts MI:Michigan MN:Minnesota MS:Mississippi MO:Missouri MT:Montana NE:Nebraska NV:Nevada NH:New_Hampshire NJ:New_Jersey NM:New_Mexico NY:New_York NC:North_Carolina ND:North_Dakota OH:Ohio OK:Oklahoma OR:Oregon PA:Pennsylvania RI:Rhode_Island SC:South_Carolina SD:South_Dakota TN:Tennessee TX:Texas UT:Utah VT:Vermont VA:Virginia WA:Washington WV:West_Virginia WI:Wisconsin WY:Wyoming AA:Armed_Forces_(AA) AE:Armed_Forces_(AE) AP:Armed_Forces_(AP)",
	"CA": "AB:Alberta BC:British_Columbia MB:Manitoba NB:New_Brunswick NL:Newfoundland_and_Labrador NT:Northwest_Territories NS:Nova_Scotia NU:Nunavut ON:Ontario PE:Prince_Edward_Island QC:Quebec SK:Saskatchewan YT:Yukon_Territory",
	"AU": "ACT:Australian_Capital_Territory NSW:New_South_Wales NT:Northern_Territory QLD:Queensland SA:South_Australia TAS:Tasmania VIC:Victoria WA:Western_Australia",
	"ES": "C:A_Coruña VI:Araba/Álava AB:Albacete A:Alicante AL:Almería O:Asturias AV:Ávila BA:Badajoz PM:Baleares B:Barcelona BU:Burgos CC:Cáceres CA:Cádiz S:Cantabria CS:Castellón CE:Ceuta CR:Ciudad_Real CO:Córdoba CU:Cuenca GI:Girona GR:Granada GU:Guadalajara SS:Gipuzkoa H:Huelva HU:Huesca J:Jaén LO:La_Rioja GC:Las_Palmas LE:León L:Lleida LU:Lugo M:Madrid MA:Málaga ML:Melilla MU:Murcia NA:Navarra OR:Ourense P:Palencia PO:Pontevedra SA:Salamanca TF:Santa_Cruz_de_Tenerife SG:Segovia SE:Sevilla SO:Soria T:Tarragona TE:Teruel TO:Toledo V:Valencia VA:Valladolid BI:Biscay ZA:Zamora Z:Zaragoza",
	"IT": "AG:Agrigento AL:Alessandria AN:Ancona AO:Aosta AR:Arezzo AP:Ascoli_Piceno AT:Asti AV:Avellino BA:Bari BT:Barletta-Andria-Trani BL:Belluno BN:Benevento BG:Bergamo BI:Biella BO:Bologna BZ:Bolzano BS:Brescia BR:Brindisi CA:Cagliari CL:Caltanissetta CB:Campobasso CE:Caserta CT:Catania CZ:Catanzaro CH:Chieti CO:Como CS:Cosenza CR:Cremona KR:Crotone CN:Cuneo EN:Enna FM:Fermo FE:Ferrara FI:Firenze FG:Foggia FC:Forlì-Cesena FR:Frosinone GE:Genova GO:Gorizia GR:Grosseto IM:Imperia IS:Isernia SP:La_Spezia AQ:L'Aquila LT:Latina LE:Lecce LC:Lecco LI:Livorno LO:Lodi LU:Lucca MC:Macerata MN:Mantova MS:Massa-Carrara MT:Matera ME:Messina MI:Milano MO:Modena MB:Monza_e_della_Brianza NA:Napoli NO:Novara NU:Nuoro OR:Oristano PD:Padova PA:Palermo PR:Parma PV:Pavia PG:Perugia PU:Pesaro_e_Urbino PE:Pescara PC:Piacenza PI:Pisa PT:Pistoia PN:Pordenone PZ:Potenza PO:Prato RG:Ragusa RA:Ravenna RC:Reggio_Calabria RE:Reggio_Emilia RI:Rieti RN:Rimini RM:Roma RO:Rovigo SA:Salerno SS:Sassari SV:Savona SI:Siena SR:Siracusa SO:Sondrio SU:Sud_Sardegna TA:Taranto TE:Teramo TR:Terni TO:Torino TP:Trapani TN:Trento TV:Treviso TS:Trieste UD:Udine VA:Varese VE:Venezia VB:Verbano-Cusio-Ossola VC:Vercelli VR:Verona VV:Vibo_Valentia VI:Vicenza VT:Viterbo",
}

var countryIndex map[string]Country

func countryName(code, lang string) string {
	region, err := language.ParseRegion(code)
	if err != nil {
		return code
	}
	tag := language.English
	if t, err := language.Parse(lang); err == nil && lang != "" {
		tag = t
	}
	if name := display.Regions(tag).Name(region); name != "" {
		return name
	}
	return display.English.Regions().Name(region)
}

func init() {
	countryIndex = map[string]Country{}
	for continent, list := range continentCountries {
		for _, code := range strings.Fields(list) {
			c := Country{Code: code, Name: countryName(code, "en"), Continent: continent, States: []State{}}
			for _, pair := range strings.Fields(countryStates[code]) {
				kv := strings.SplitN(pair, ":", 2)
				if len(kv) == 2 {
					c.States = append(c.States, State{Code: kv[0], Name: strings.ReplaceAll(kv[1], "_", " ")})
				}
			}
			countryIndex[code] = c
		}
	}
}

// AllCountries is every country, sorted by name (names in lang when known).
func AllCountries(lang string) []Country {
	out := make([]Country, 0, len(countryIndex))
	for _, c := range countryIndex {
		if lang != "" && lang != "en" {
			c.Name = countryName(c.Code, lang)
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// CountryByCode answers one country.
func CountryByCode(code string) (Country, bool) {
	c, ok := countryIndex[strings.ToUpper(code)]
	return c, ok
}

// ContinentOf is WooCommerce's continent code of a country.
func ContinentOf(code string) string { return countryIndex[strings.ToUpper(code)].Continent }

// Continents lists WooCommerce's continents.
func Continents() []State {
	out := []State{}
	for _, k := range []string{"AF", "AN", "AS", "EU", "NA", "OC", "SA"} {
		out = append(out, State{Code: k, Name: continentNames[k]})
	}
	return out
}

// CountryStateName is a state's display name (code when unknown).
func CountryStateName(country, state string) string {
	for _, st := range countryIndex[strings.ToUpper(country)].States {
		if st.Code == state {
			return st.Name
		}
	}
	return state
}
