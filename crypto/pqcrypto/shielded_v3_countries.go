package pqcrypto

import (
	"strings"
	"unicode"
)

// stampCountryNames is the ISO 3166-1 country and territory name set. Keeping
// it in the client library makes stamp validation deterministic and offline.
// Common short forms are accepted below, but stamps always store the canonical
// display name from this list.
const stampCountryNames = `
Afghanistan
Albania
Algeria
American Samoa
Andorra
Angola
Anguilla
Antarctica
Antigua and Barbuda
Argentina
Armenia
Aruba
Australia
Austria
Azerbaijan
Bahamas
Bahrain
Bangladesh
Barbados
Belarus
Belgium
Belize
Benin
Bermuda
Bhutan
Bolivia
Bonaire, Sint Eustatius and Saba
Bosnia and Herzegovina
Botswana
Bouvet Island
Brazil
British Indian Ocean Territory
Brunei Darussalam
Bulgaria
Burkina Faso
Burundi
Cabo Verde
Cambodia
Cameroon
Canada
Cayman Islands
Central African Republic
Chad
Chile
China
Christmas Island
Cocos (Keeling) Islands
Colombia
Comoros
Congo
Congo, The Democratic Republic of the
Cook Islands
Costa Rica
Croatia
Cuba
Curaçao
Cyprus
Czechia
Côte d'Ivoire
Denmark
Djibouti
Dominica
Dominican Republic
Ecuador
Egypt
El Salvador
Equatorial Guinea
Eritrea
Estonia
Eswatini
Ethiopia
Falkland Islands (Malvinas)
Faroe Islands
Fiji
Finland
France
French Guiana
French Polynesia
French Southern Territories
Gabon
Gambia
Georgia
Germany
Ghana
Gibraltar
Greece
Greenland
Grenada
Guadeloupe
Guam
Guatemala
Guernsey
Guinea
Guinea-Bissau
Guyana
Haiti
Heard Island and McDonald Islands
Holy See
Honduras
Hong Kong
Hungary
Iceland
India
Indonesia
Iran
Iraq
Ireland
Isle of Man
Israel
Italy
Jamaica
Japan
Jersey
Jordan
Kazakhstan
Kenya
Kiribati
Korea, North
Korea, South
Kuwait
Kyrgyzstan
Laos
Latvia
Lebanon
Lesotho
Liberia
Libya
Liechtenstein
Lithuania
Luxembourg
Macao
Madagascar
Malawi
Malaysia
Maldives
Mali
Malta
Marshall Islands
Martinique
Mauritania
Mauritius
Mayotte
Mexico
Micronesia
Moldova
Monaco
Mongolia
Montenegro
Montserrat
Morocco
Mozambique
Myanmar
Namibia
Nauru
Nepal
Netherlands
New Caledonia
New Zealand
Nicaragua
Niger
Nigeria
Niue
Norfolk Island
North Macedonia
Northern Mariana Islands
Norway
Oman
Pakistan
Palau
Palestine
Panama
Papua New Guinea
Paraguay
Peru
Philippines
Pitcairn
Poland
Portugal
Puerto Rico
Qatar
Réunion
Romania
Russia
Rwanda
Saint Barthélemy
Saint Helena, Ascension and Tristan da Cunha
Saint Kitts and Nevis
Saint Lucia
Saint Martin (French part)
Saint Pierre and Miquelon
Saint Vincent and the Grenadines
Samoa
San Marino
Sao Tome and Principe
Saudi Arabia
Senegal
Serbia
Seychelles
Sierra Leone
Singapore
Sint Maarten (Dutch part)
Slovakia
Slovenia
Solomon Islands
Somalia
South Africa
South Georgia and the South Sandwich Islands
South Sudan
Spain
Sri Lanka
Sudan
Suriname
Svalbard and Jan Mayen
Sweden
Switzerland
Syria
Taiwan
Tajikistan
Tanzania
Thailand
Timor-Leste
Togo
Tokelau
Tonga
Trinidad and Tobago
Tunisia
Türkiye
Turkmenistan
Turks and Caicos Islands
Tuvalu
Uganda
Ukraine
United Arab Emirates
United Kingdom
United States
United States Minor Outlying Islands
Uruguay
Uzbekistan
Vanuatu
Venezuela
Vietnam
Virgin Islands, British
Virgin Islands, U.S.
Wallis and Futuna
Western Sahara
Yemen
Zambia
Zimbabwe
Åland Islands
`

var stampCountryCanonical = func() map[string]string {
	result := make(map[string]string, 270)
	for _, name := range strings.Split(stampCountryNames, "\n") {
		name = strings.TrimSpace(name)
		if name != "" {
			result[normalizeStampCountry(name)] = name
		}
	}
	for alias, canonical := range map[string]string{
		"burma": "Myanmar", "cape verde": "Cabo Verde", "congo brazzaville": "Congo",
		"congo kinshasa": "Congo, The Democratic Republic of the", "democratic republic of the congo": "Congo, The Democratic Republic of the",
		"dr congo": "Congo, The Democratic Republic of the", "iran": "Iran", "iran (islamic republic of)": "Iran",
		"ivory coast": "Côte d'Ivoire", "korea": "Korea, South", "north korea": "Korea, North",
		"republic of korea": "Korea, South", "south korea": "Korea, South", "lao pdr": "Laos",
		"micronesia, federated states of": "Micronesia", "moldova, republic of": "Moldova",
		"palestinian territories": "Palestine", "palestine, state of": "Palestine",
		"russia": "Russia", "russian federation": "Russia", "swaziland": "Eswatini",
		"syria": "Syria", "syrian arab republic": "Syria", "taiwan, province of china": "Taiwan",
		"tanzania, united republic of": "Tanzania", "the gambia": "Gambia", "the netherlands": "Netherlands",
		"turkey": "Türkiye", "u.s. virgin islands": "Virgin Islands, U.S.",
		"united states of america": "United States", "venezuela, bolivarian republic of": "Venezuela",
		"vietnam": "Vietnam", "viet nam": "Vietnam", "bolivia, plurinational state of": "Bolivia",
		"brunei": "Brunei Darussalam", "czech republic": "Czechia", "holy see (vatican city state)": "Holy See",
		"macao": "Macao", "north macedonia": "North Macedonia", "moldova": "Moldova",
	} {
		result[normalizeStampCountry(alias)] = canonical
	}
	return result
}()

func normalizeStampCountry(value string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsSpace(r) {
			space = b.Len() > 0
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// StampCountries returns a copy of the supported ISO country and territory
// names for wallet interfaces.
func StampCountries() []string {
	var names []string
	for _, name := range strings.Split(stampCountryNames, "\n") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// CanonicalStampCountry validates a user-supplied country name and returns its
// canonical stored spelling. Unknown values must never be stamped.
func CanonicalStampCountry(value string) (string, bool) {
	name, ok := stampCountryCanonical[normalizeStampCountry(value)]
	return name, ok
}
