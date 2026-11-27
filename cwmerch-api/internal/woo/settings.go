package woo

import (
	"context"
	"strings"
)

// Address is a billing or shipping address (email only on billing).
type Address struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Company   string `json:"company"`
	Address1  string `json:"address1"`
	Address2  string `json:"address2"`
	City      string `json:"city"`
	State     string `json:"state"`
	Postcode  string `json:"postcode"`
	Country   string `json:"country"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone"`
}

// GatewaySummary is a payment method offered at checkout.
type GatewaySummary struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Type           string `json:"type"`
	PublishableKey string `json:"publishableKey,omitempty"`
}

// StoreAddress is the shop's own address.
type StoreAddress struct {
	Address1 string `json:"address1"`
	Address2 string `json:"address2"`
	City     string `json:"city"`
	Postcode string `json:"postcode"`
	Country  string `json:"country"`
	State    string `json:"state"`
}

// ShopSettings is what the storefront needs to render prices and checkout.
type ShopSettings struct {
	Name                 string           `json:"name"`
	Description          string           `json:"description"`
	Currency             string           `json:"currency"`
	CurrencySymbol       string           `json:"currencySymbol"`
	CurrencyPosition     string           `json:"currencyPosition"`
	ThousandSeparator    string           `json:"thousandSeparator"`
	DecimalSeparator     string           `json:"decimalSeparator"`
	Decimals             int              `json:"decimals"`
	WeightUnit           string           `json:"weightUnit"`
	DimensionUnit        string           `json:"dimensionUnit"`
	CalcTaxes            bool             `json:"calcTaxes"`
	PricesIncludeTax     bool             `json:"pricesIncludeTax"`
	TaxDisplayShop       string           `json:"taxDisplayShop"`
	TaxDisplayCart       string           `json:"taxDisplayCart"`
	TaxBasedOn           string           `json:"taxBasedOn"`
	EnableCoupons        bool             `json:"enableCoupons"`
	EnableReviews        bool             `json:"enableReviews"`
	EnableReviewRating   bool             `json:"enableReviewRating"`
	ReviewRatingRequired bool             `json:"reviewRatingRequired"`
	VerifiedOwnersOnly   bool             `json:"verifiedOwnersOnly"`
	GuestCheckout        bool             `json:"guestCheckout"`
	SignupAtCheckout     bool             `json:"signupAtCheckout"`
	SignupOnMyAccount    bool             `json:"signupOnMyAccount"`
	ManageStock          bool             `json:"manageStock"`
	DefaultCountry       string           `json:"defaultCountry"`
	DefaultState         string           `json:"defaultState"`
	AllowedCountries     []string         `json:"allowedCountries"`
	ShipToCountries      []string         `json:"shipToCountries"`
	ShippingEnabled      bool             `json:"shippingEnabled"`
	StoreAddress         StoreAddress     `json:"storeAddress"`
	PaymentGateways      []GatewaySummary `json:"paymentGateways"`
	ContactEnabled       bool             `json:"contactEnabled"`
	OIDCProviders        []string         `json:"oidcProviders"`
	ShopEnabled          bool             `json:"shopEnabled"`
	// PhoneConfirmation: no online payment partner is configured, so an
	// order is confirmed by phone (the phone number is required at checkout).
	PhoneConfirmation   bool       `json:"phoneConfirmation"`
	ClosedMessage       string     `json:"closedMessage"`
	RegistrationEnabled bool       `json:"registrationEnabled"`
	Languages           []string   `json:"languages"`
	Permalinks          Permalinks `json:"permalinks"`
	DefaultLanguage     string     `json:"defaultLanguage"`
	Version             string     `json:"version"`
}

// Decimals is the shop's price precision.
func (s *Store) Decimals(ctx context.Context) int {
	return s.OptionInt(ctx, "woocommerce_price_num_decimals", 2)
}

// Currency is the shop currency code.
func (s *Store) Currency(ctx context.Context) string {
	return s.OptionOr(ctx, "woocommerce_currency", "EUR")
}

// splitCountry reads "FR" or "US:CA" (country:state), WooCommerce's default_country.
func splitCountry(v string) (string, string) {
	parts := strings.SplitN(v, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return v, ""
}

func (s *Store) countryList(ctx context.Context, mode, specific, except string) []string {
	all := []string{}
	for _, c := range AllCountries("en") {
		all = append(all, c.Code)
	}
	switch mode {
	case "specific":
		return s.OptionArray(ctx, specific).StringList()
	case "all_except":
		excluded := map[string]bool{}
		for _, c := range s.OptionArray(ctx, except).StringList() {
			excluded[c] = true
		}
		out := []string{}
		for _, c := range all {
			if !excluded[c] {
				out = append(out, c)
			}
		}
		return out
	}
	return all
}

// AllowedCountries is where the shop sells (woocommerce_allowed_countries).
func (s *Store) AllowedCountries(ctx context.Context) []string {
	return s.countryList(ctx, s.OptionOr(ctx, "woocommerce_allowed_countries", "all"),
		"woocommerce_specific_allowed_countries", "woocommerce_all_except_countries")
}

// ShipToCountries is where the shop ships ("" = same as selling countries,
// "disabled" = no shipping).
func (s *Store) ShipToCountries(ctx context.Context) []string {
	switch s.Option(ctx, "woocommerce_ship_to_countries") {
	case "", "all_selling":
		return s.AllowedCountries(ctx)
	case "disabled":
		return []string{}
	case "specific":
		return s.OptionArray(ctx, "woocommerce_specific_ship_to_countries").StringList()
	}
	return s.countryList(ctx, "all", "", "")
}

// ShopSettings reads the storefront settings from WooCommerce's options.
func (s *Store) ShopSettings(ctx context.Context) ShopSettings {
	country, state := splitCountry(s.OptionOr(ctx, "woocommerce_default_country", "FR"))
	currency := s.Currency(ctx)
	calc := s.OptionBool(ctx, "woocommerce_calc_taxes", false)
	set := ShopSettings{
		Name:                 s.Option(ctx, "blogname"),
		Description:          s.Option(ctx, "blogdescription"),
		Currency:             currency,
		CurrencySymbol:       CurrencySymbol(currency),
		CurrencyPosition:     s.OptionOr(ctx, "woocommerce_currency_pos", "left"),
		ThousandSeparator:    s.OptionOr(ctx, "woocommerce_price_thousand_sep", ","),
		DecimalSeparator:     s.OptionOr(ctx, "woocommerce_price_decimal_sep", "."),
		Decimals:             s.Decimals(ctx),
		WeightUnit:           s.OptionOr(ctx, "woocommerce_weight_unit", "kg"),
		DimensionUnit:        s.OptionOr(ctx, "woocommerce_dimension_unit", "cm"),
		CalcTaxes:            calc,
		PricesIncludeTax:     calc && s.OptionBool(ctx, "woocommerce_prices_include_tax", false),
		TaxDisplayShop:       s.OptionOr(ctx, "woocommerce_tax_display_shop", "excl"),
		TaxDisplayCart:       s.OptionOr(ctx, "woocommerce_tax_display_cart", "excl"),
		TaxBasedOn:           s.OptionOr(ctx, "woocommerce_tax_based_on", "shipping"),
		EnableCoupons:        s.OptionBool(ctx, "woocommerce_enable_coupons", true),
		EnableReviews:        s.OptionBool(ctx, "woocommerce_enable_reviews", true),
		EnableReviewRating:   s.OptionBool(ctx, "woocommerce_enable_review_rating", true),
		ReviewRatingRequired: s.OptionBool(ctx, "woocommerce_review_rating_required", true),
		VerifiedOwnersOnly:   s.OptionBool(ctx, "woocommerce_review_rating_verification_required", false),
		GuestCheckout:        s.OptionBool(ctx, "woocommerce_enable_guest_checkout", true),
		SignupAtCheckout:     s.OptionBool(ctx, "woocommerce_enable_signup_and_login_from_checkout", true),
		SignupOnMyAccount:    s.OptionBool(ctx, "woocommerce_enable_myaccount_registration", true),
		ManageStock:          s.OptionBool(ctx, "woocommerce_manage_stock", true),
		DefaultCountry:       country,
		DefaultState:         state,
		AllowedCountries:     s.AllowedCountries(ctx),
		ShipToCountries:      s.ShipToCountries(ctx),
		StoreAddress: StoreAddress{
			Address1: s.Option(ctx, "woocommerce_store_address"), Address2: s.Option(ctx, "woocommerce_store_address_2"),
			City: s.Option(ctx, "woocommerce_store_city"), Postcode: s.Option(ctx, "woocommerce_store_postcode"),
			Country: country, State: state,
		},
		PaymentGateways: []GatewaySummary{},
		OIDCProviders:   []string{},
	}
	sc := s.ShopConfig(ctx)
	set.ShopEnabled, set.ClosedMessage, set.RegistrationEnabled = sc.ShopEnabled, sc.ClosedMessage, sc.RegistrationEnabled
	set.Languages, set.DefaultLanguage = sc.Languages, sc.DefaultLanguage
	set.Permalinks = s.Permalinks(ctx)
	if !calc {
		set.TaxDisplayShop, set.TaxDisplayCart = "excl", "excl"
	}
	set.ShippingEnabled = s.Option(ctx, "woocommerce_ship_to_countries") != "disabled"
	for _, g := range s.Gateways(ctx) {
		if g.Enabled {
			set.PaymentGateways = append(set.PaymentGateways, GatewaySummary{ID: g.ID, Title: g.Title, Description: g.Description, Type: "offline"})
		}
	}
	return set
}

// SettingField describes one editable WooCommerce option for the admin form.
type SettingField struct {
	Name    string            `json:"name"`
	Group   string            `json:"group"`
	Type    string            `json:"type"` // text, number, select, multiselect, checkbox, textarea, country
	Label   string            `json:"label"`
	Default string            `json:"default,omitempty"`
	Choices map[string]string `json:"choices,omitempty"`
}

var yesNoField = "checkbox"

// SettingFields is the whitelist of options PUT /v1/admin/settings may write,
// grouped like WooCommerce's settings tabs.
func SettingFields() []SettingField {
	currencies := map[string]string{}
	for _, c := range CurrencyCodes() {
		currencies[c] = c + " (" + CurrencySymbol(c) + ")"
	}
	return []SettingField{
		{Name: "blogname", Group: "general", Type: "text", Label: "Shop name"},
		{Name: "blogdescription", Group: "general", Type: "text", Label: "Tagline"},
		{Name: "admin_email", Group: "general", Type: "text", Label: "Administration email"},
		{Name: "woocommerce_store_address", Group: "general", Type: "text", Label: "Address line 1"},
		{Name: "woocommerce_store_address_2", Group: "general", Type: "text", Label: "Address line 2"},
		{Name: "woocommerce_store_city", Group: "general", Type: "text", Label: "City"},
		{Name: "woocommerce_default_country", Group: "general", Type: "country", Label: "Country / State"},
		{Name: "woocommerce_store_postcode", Group: "general", Type: "text", Label: "Postcode / ZIP"},
		{Name: "woocommerce_allowed_countries", Group: "general", Type: "select", Label: "Selling location(s)", Default: "all",
			Choices: map[string]string{"all": "Sell to all countries", "all_except": "Sell to all countries, except for…", "specific": "Sell to specific countries"}},
		{Name: "woocommerce_all_except_countries", Group: "general", Type: "multiselect", Label: "Sell to all countries, except for…"},
		{Name: "woocommerce_specific_allowed_countries", Group: "general", Type: "multiselect", Label: "Sell to specific countries"},
		{Name: "woocommerce_ship_to_countries", Group: "general", Type: "select", Label: "Shipping location(s)",
			Choices: map[string]string{"": "Ship to all countries you sell to", "all": "Ship to all countries", "specific": "Ship to specific countries only", "disabled": "Disable shipping & shipping calculations"}},
		{Name: "woocommerce_specific_ship_to_countries", Group: "general", Type: "multiselect", Label: "Ship to specific countries"},
		{Name: "woocommerce_default_customer_address", Group: "general", Type: "select", Label: "Default customer location", Default: "base",
			Choices: map[string]string{"": "No location by default", "base": "Shop country/region"}},
		{Name: "woocommerce_calc_taxes", Group: "general", Type: yesNoField, Label: "Enable tax rates and calculations"},
		{Name: "woocommerce_enable_coupons", Group: "general", Type: yesNoField, Label: "Enable the use of coupon codes"},
		{Name: "woocommerce_calc_discounts_sequentially", Group: "general", Type: yesNoField, Label: "Calculate coupon discounts sequentially"},
		{Name: "woocommerce_currency", Group: "general", Type: "select", Label: "Currency", Choices: currencies},
		{Name: "woocommerce_currency_pos", Group: "general", Type: "select", Label: "Currency position",
			Choices: map[string]string{"left": "Left", "right": "Right", "left_space": "Left with space", "right_space": "Right with space"}},
		{Name: "woocommerce_price_thousand_sep", Group: "general", Type: "text", Label: "Thousand separator"},
		{Name: "woocommerce_price_decimal_sep", Group: "general", Type: "text", Label: "Decimal separator"},
		{Name: "woocommerce_price_num_decimals", Group: "general", Type: "number", Label: "Number of decimals"},

		{Name: "woocommerce_weight_unit", Group: "products", Type: "select", Label: "Weight unit",
			Choices: map[string]string{"kg": "kg", "g": "g", "lbs": "lbs", "oz": "oz"}},
		{Name: "woocommerce_dimension_unit", Group: "products", Type: "select", Label: "Dimensions unit",
			Choices: map[string]string{"m": "m", "cm": "cm", "mm": "mm", "in": "in", "yd": "yd"}},
		{Name: "woocommerce_enable_reviews", Group: "products", Type: yesNoField, Label: "Enable product reviews"},
		{Name: "woocommerce_review_rating_verification_label", Group: "products", Type: yesNoField, Label: "Show \"verified owner\" label on customer reviews"},
		{Name: "woocommerce_review_rating_verification_required", Group: "products", Type: yesNoField, Label: "Reviews can only be left by \"verified owners\""},
		{Name: "woocommerce_enable_review_rating", Group: "products", Type: yesNoField, Label: "Enable star rating on reviews"},
		{Name: "woocommerce_review_rating_required", Group: "products", Type: yesNoField, Label: "Star ratings should be required, not optional"},
		{Name: "woocommerce_manage_stock", Group: "products", Type: yesNoField, Label: "Enable stock management"},
		{Name: "woocommerce_hold_stock_minutes", Group: "products", Type: "number", Label: "Hold stock (minutes)"},
		{Name: "woocommerce_notify_low_stock", Group: "products", Type: yesNoField, Label: "Enable low stock notifications"},
		{Name: "woocommerce_notify_no_stock", Group: "products", Type: yesNoField, Label: "Enable out of stock notifications"},
		{Name: "woocommerce_stock_email_recipient", Group: "products", Type: "text", Label: "Notification recipient(s)"},
		{Name: "woocommerce_notify_low_stock_amount", Group: "products", Type: "number", Label: "Low stock threshold"},
		{Name: "woocommerce_notify_no_stock_amount", Group: "products", Type: "number", Label: "Out of stock threshold"},
		{Name: "woocommerce_hide_out_of_stock_items", Group: "products", Type: yesNoField, Label: "Hide out of stock items from the catalog"},
		{Name: "woocommerce_stock_format", Group: "products", Type: "select", Label: "Stock display format",
			Choices: map[string]string{"": "Always show quantity remaining in stock", "low_amount": "Only show quantity remaining in stock when low", "no_amount": "Never show quantity remaining in stock"}},
		{Name: "woocommerce_downloads_require_login", Group: "products", Type: yesNoField, Label: "Downloads require login"},
		{Name: "woocommerce_downloads_grant_access_after_payment", Group: "products", Type: yesNoField, Label: "Grant access to downloadable products after payment"},

		{Name: "woocommerce_prices_include_tax", Group: "tax", Type: "select", Label: "Prices entered with tax",
			Choices: map[string]string{"yes": "Yes, I will enter prices inclusive of tax", "no": "No, I will enter prices exclusive of tax"}},
		{Name: "woocommerce_tax_based_on", Group: "tax", Type: "select", Label: "Calculate tax based on",
			Choices: map[string]string{"shipping": "Customer shipping address", "billing": "Customer billing address", "base": "Shop base address"}},
		{Name: "woocommerce_shipping_tax_class", Group: "tax", Type: "text", Label: "Shipping tax class (inherit = based on cart items)"},
		{Name: "woocommerce_tax_round_at_subtotal", Group: "tax", Type: yesNoField, Label: "Round tax at subtotal level, instead of rounding per line"},
		{Name: "woocommerce_tax_display_shop", Group: "tax", Type: "select", Label: "Display prices in the shop",
			Choices: map[string]string{"incl": "Including tax", "excl": "Excluding tax"}},
		{Name: "woocommerce_tax_display_cart", Group: "tax", Type: "select", Label: "Display prices during cart and checkout",
			Choices: map[string]string{"incl": "Including tax", "excl": "Excluding tax"}},
		{Name: "woocommerce_tax_total_display", Group: "tax", Type: "select", Label: "Display tax totals",
			Choices: map[string]string{"single": "As a single total", "itemized": "Itemized"}},

		{Name: "woocommerce_enable_shipping_calc", Group: "shipping", Type: yesNoField, Label: "Enable the shipping calculator on the cart page"},
		{Name: "woocommerce_shipping_cost_requires_address", Group: "shipping", Type: yesNoField, Label: "Hide shipping costs until an address is entered"},
		{Name: "woocommerce_ship_to_destination", Group: "shipping", Type: "select", Label: "Shipping destination",
			Choices: map[string]string{"shipping": "Default to customer shipping address", "billing": "Default to customer billing address", "billing_only": "Force shipping to the customer billing address"}},

		{Name: "woocommerce_enable_guest_checkout", Group: "accounts", Type: yesNoField, Label: "Allow customers to place orders without an account"},
		{Name: "woocommerce_enable_checkout_login_reminder", Group: "accounts", Type: yesNoField, Label: "Allow customers to log into an existing account during checkout"},
		{Name: "woocommerce_enable_signup_and_login_from_checkout", Group: "accounts", Type: yesNoField, Label: "Allow customers to create an account during checkout"},
		{Name: "woocommerce_enable_myaccount_registration", Group: "accounts", Type: yesNoField, Label: "Allow customers to create an account on the \"My account\" page"},
		{Name: "woocommerce_registration_generate_password", Group: "accounts", Type: yesNoField, Label: "Send password setup link"},
		{Name: "woocommerce_erasure_request_removes_order_data", Group: "accounts", Type: yesNoField, Label: "Remove personal data from orders on request"},
		{Name: "woocommerce_registration_privacy_policy_text", Group: "accounts", Type: "textarea", Label: "Registration privacy policy"},
		{Name: "woocommerce_checkout_privacy_policy_text", Group: "accounts", Type: "textarea", Label: "Checkout privacy policy"},

		{Name: "woocommerce_email_from_name", Group: "emails", Type: "text", Label: "\"From\" name"},
		{Name: "woocommerce_email_from_address", Group: "emails", Type: "text", Label: "\"From\" address"},
		{Name: "woocommerce_email_header_image", Group: "emails", Type: "text", Label: "Header image URL"},
		{Name: "woocommerce_email_footer_text", Group: "emails", Type: "textarea", Label: "Footer text"},
		{Name: "woocommerce_email_base_color", Group: "emails", Type: "text", Label: "Base colour"},
		{Name: "woocommerce_email_background_color", Group: "emails", Type: "text", Label: "Background colour"},
		{Name: "woocommerce_email_body_background_color", Group: "emails", Type: "text", Label: "Body background colour"},
		{Name: "woocommerce_email_text_color", Group: "emails", Type: "text", Label: "Body text colour"},

		{Name: "woocommerce_terms_page_id", Group: "advanced", Type: "number", Label: "Terms and conditions page id"},
		{Name: "woocommerce_checkout_terms_and_conditions_checkbox_text", Group: "advanced", Type: "text", Label: "Terms and conditions checkbox text"},
		{Name: "woocommerce_custom_orders_table_enabled", Group: "advanced", Type: yesNoField, Label: "Order storage: high-performance order storage (HPOS)"},
		{Name: "woocommerce_custom_orders_table_data_sync_enabled", Group: "advanced", Type: yesNoField, Label: "Enable compatibility mode (synchronize orders between HPOS and posts)"},
		{Name: "timezone_string", Group: "advanced", Type: "text", Label: "Timezone (e.g. Europe/Paris)"},
	}
}

// multiselectOptions are stored PHP-serialized by WooCommerce.
func isSerializedSetting(f SettingField) bool { return f.Type == "multiselect" }

// AdminSettings reads every whitelisted option.
func (s *Store) AdminSettings(ctx context.Context) map[string]any {
	out := map[string]any{}
	for _, f := range SettingFields() {
		if isSerializedSetting(f) {
			out[f.Name] = s.OptionArray(ctx, f.Name).StringList()
			continue
		}
		out[f.Name] = s.OptionOr(ctx, f.Name, f.Default)
	}
	return out
}

// SaveAdminSettings writes the given whitelisted options; unknown names are refused.
func (s *Store) SaveAdminSettings(ctx context.Context, values map[string]any) ([]string, error) {
	fields := map[string]SettingField{}
	for _, f := range SettingFields() {
		fields[f.Name] = f
	}
	changed := []string{}
	for name, raw := range values {
		f, ok := fields[name]
		if !ok {
			return nil, refuse(400, "errors.unknownSetting", "Unknown setting: "+name)
		}
		var value string
		switch v := raw.(type) {
		case []any:
			list := []string{}
			for _, item := range v {
				list = append(list, toString(item))
			}
			value = serializeStrings(list)
		case bool:
			value = yesNo(v)
		default:
			value = toString(v)
			if f.Type == yesNoField && (value == "true" || value == "1") {
				value = "yes"
			} else if f.Type == yesNoField && (value == "false" || value == "0" || value == "") {
				value = "no"
			}
		}
		if err := s.SetOption(ctx, name, value); err != nil {
			return nil, err
		}
		changed = append(changed, name)
	}
	s.ForgetOptions()
	return changed, nil
}
