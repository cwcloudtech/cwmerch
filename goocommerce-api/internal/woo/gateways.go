package woo

import (
	"context"
	"sort"

	"goocommerce-api/internal/php"
)

// Gateway is an offline payment method WooCommerce ships with (bacs, cheque,
// cod). Settings live in the option woocommerce_<id>_settings, serialized.
type Gateway struct {
	ID          string            `json:"id"`
	MethodTitle string            `json:"methodTitle"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Enabled     bool              `json:"enabled"`
	Order       int               `json:"order"`
	Settings    map[string]string `json:"settings"`
	// Accounts is BACS' bank accounts (option woocommerce_bacs_accounts).
	Accounts []BankAccount `json:"accounts,omitempty"`
}

// BankAccount is one BACS account shown in the payment instructions.
type BankAccount struct {
	AccountName   string `json:"accountName"`
	AccountNumber string `json:"accountNumber"`
	BankName      string `json:"bankName"`
	SortCode      string `json:"sortCode"`
	IBAN          string `json:"iban"`
	BIC           string `json:"bic"`
}

var gatewayDefaults = map[string]map[string]string{
	"bacs": {"enabled": "no", "title": "Direct bank transfer",
		"description":  "Make your payment directly into our bank account. Please use your Order ID as the payment reference. Your order will not be shipped until the funds have cleared in our account.",
		"instructions": "", "account_details": ""},
	"cheque": {"enabled": "no", "title": "Check payments",
		"description":  "Please send a check to Store Name, Store Street, Store Town, Store State / County, Store Postcode.",
		"instructions": ""},
	"cod": {"enabled": "no", "title": "Cash on delivery", "description": "Pay with cash upon delivery.",
		"instructions": "Pay with cash upon delivery.", "enable_for_methods": "", "enable_for_virtual": "yes"},
}

var gatewayMethodTitles = map[string]string{"bacs": "Direct bank transfer", "cheque": "Check payments", "cod": "Cash on delivery"}

// GatewayIDs are the gateways goocommerce runs (in WooCommerce's default order).
var GatewayIDs = []string{"bacs", "cheque", "cod"}

// Gateways reads every gateway with its settings.
func (s *Store) Gateways(ctx context.Context) []Gateway {
	order := s.OptionArray(ctx, "woocommerce_gateway_order")
	out := []Gateway{}
	for i, id := range GatewayIDs {
		settings := map[string]string{}
		for k, v := range gatewayDefaults[id] {
			settings[k] = v
		}
		stored := s.OptionArray(ctx, "woocommerce_"+id+"_settings")
		for _, k := range stored.Keys {
			if _, isArr := stored.Get(k).(*php.Array); isArr {
				continue
			}
			settings[php.ToString(k)] = stored.String(k)
		}
		g := Gateway{ID: id, MethodTitle: gatewayMethodTitles[id], Title: settings["title"],
			Description: settings["description"], Enabled: settings["enabled"] == "yes", Order: i, Settings: settings}
		if order.Has(id) {
			g.Order = int(parseFloat(order.String(id)))
		}
		if id == "bacs" {
			g.Accounts = []BankAccount{}
			for _, a := range s.OptionArray(ctx, "woocommerce_bacs_accounts").List() {
				arr, ok := a.(*php.Array)
				if !ok {
					continue
				}
				g.Accounts = append(g.Accounts, BankAccount{AccountName: arr.String("account_name"), AccountNumber: arr.String("account_number"),
					BankName: arr.String("bank_name"), SortCode: arr.String("sort_code"), IBAN: arr.String("iban"), BIC: arr.String("bic")})
			}
		}
		out = append(out, g)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// Gateway is one gateway by id.
func (s *Store) Gateway(ctx context.Context, id string) (Gateway, bool) {
	for _, g := range s.Gateways(ctx) {
		if g.ID == id {
			return g, true
		}
	}
	return Gateway{}, false
}

// GatewayUpdate is the admin's edit of a gateway.
type GatewayUpdate struct {
	Enabled  *bool             `json:"enabled"`
	Order    *int              `json:"order"`
	Settings map[string]string `json:"settings"`
	Accounts []BankAccount     `json:"accounts"`
}

// SaveGateway writes a gateway's settings the way WooCommerce stores them.
func (s *Store) SaveGateway(ctx context.Context, id string, u GatewayUpdate) (Gateway, error) {
	if _, ok := gatewayDefaults[id]; !ok {
		return Gateway{}, ErrNotFound
	}
	stored := s.OptionArray(ctx, "woocommerce_"+id+"_settings")
	for k, v := range gatewayDefaults[id] {
		if !stored.Has(k) {
			stored.Set(k, v)
		}
	}
	for k, v := range u.Settings {
		if k == "enabled" {
			continue
		}
		stored.Set(k, v)
	}
	if u.Enabled != nil {
		stored.Set("enabled", yesNo(*u.Enabled))
	}
	if err := s.SetOption(ctx, "woocommerce_"+id+"_settings", php.Marshal(stored)); err != nil {
		return Gateway{}, err
	}
	if id == "bacs" && u.Accounts != nil {
		list := php.NewArray()
		for _, a := range u.Accounts {
			list.Append(php.NewArray().Set("account_name", a.AccountName).Set("account_number", a.AccountNumber).
				Set("bank_name", a.BankName).Set("sort_code", a.SortCode).Set("iban", a.IBAN).Set("bic", a.BIC))
		}
		if err := s.SetOption(ctx, "woocommerce_bacs_accounts", php.Marshal(list)); err != nil {
			return Gateway{}, err
		}
	}
	if u.Order != nil {
		order := s.OptionArray(ctx, "woocommerce_gateway_order")
		order.Set(id, int64(*u.Order))
		if err := s.SetOption(ctx, "woocommerce_gateway_order", php.Marshal(order)); err != nil {
			return Gateway{}, err
		}
	}
	g, _ := s.Gateway(ctx, id)
	return g, nil
}

// PaymentInstructions is what the "order received" page and the email say
// for an offline gateway (BACS lists the bank accounts).
func (s *Store) PaymentInstructions(ctx context.Context, gatewayID string) string {
	g, ok := s.Gateway(ctx, gatewayID)
	if !ok {
		return ""
	}
	text := g.Settings["instructions"]
	if gatewayID == "bacs" {
		for _, a := range g.Accounts {
			line := ""
			add := func(label, v string) {
				if v != "" {
					line += "\n" + label + ": " + v
				}
			}
			if a.AccountName != "" || a.BankName != "" {
				line += "\n" + a.AccountName
				if a.BankName != "" {
					line += " — " + a.BankName
				}
			}
			add("Account number", a.AccountNumber)
			add("Sort code", a.SortCode)
			add("IBAN", a.IBAN)
			add("BIC", a.BIC)
			text += "\n" + line
		}
	}
	return text
}
