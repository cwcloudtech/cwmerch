package woo

import (
	"context"
	"strings"

	"cwmerch-api/internal/php"
)

// EmailDef is one of WooCommerce's transactional emails.
type EmailDef struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Customer    bool   `json:"customerEmail"`
	Subject     map[string]string
	Heading     map[string]string
	Intro       map[string]string
}

// EmailDefs mirrors WooCommerce's WC_Email_* classes and their default texts
// (English and French), with the placeholders {site_title}, {order_number},
// {order_date}, {customer_first_name}.
var EmailDefs = []EmailDef{
	{ID: "new_order", Title: "New order", Description: "Sent to the shop when a new order is received.",
		Subject: map[string]string{"en": "[{site_title}]: New order #{order_number}", "fr": "[{site_title}] : Nouvelle commande n°{order_number}"},
		Heading: map[string]string{"en": "New Order: #{order_number}", "fr": "Nouvelle commande : n°{order_number}"},
		Intro:   map[string]string{"en": "You've received the following order from {customer_full_name}:", "fr": "Vous avez reçu la commande suivante de {customer_full_name} :"}},
	{ID: "cancelled_order", Title: "Cancelled order", Description: "Sent to the shop when an order is cancelled.",
		Subject: map[string]string{"en": "[{site_title}]: Order #{order_number} has been cancelled", "fr": "[{site_title}] : La commande n°{order_number} a été annulée"},
		Heading: map[string]string{"en": "Order Cancelled: #{order_number}", "fr": "Commande annulée : n°{order_number}"},
		Intro:   map[string]string{"en": "Notification to let you know — order #{order_number} belonging to {customer_full_name} has been cancelled:", "fr": "Pour information, la commande n°{order_number} de {customer_full_name} a été annulée :"}},
	{ID: "failed_order", Title: "Failed order", Description: "Sent to the shop when a pending or on-hold order fails.",
		Subject: map[string]string{"en": "[{site_title}]: Order #{order_number} has failed", "fr": "[{site_title}] : La commande n°{order_number} a échoué"},
		Heading: map[string]string{"en": "Order Failed: #{order_number}", "fr": "Commande échouée : n°{order_number}"},
		Intro:   map[string]string{"en": "Payment for order #{order_number} from {customer_full_name} has failed. The order was as follows:", "fr": "Le paiement de la commande n°{order_number} de {customer_full_name} a échoué. La commande était la suivante :"}},
	{ID: "customer_on_hold_order", Title: "Order on-hold", Description: "Sent to the customer when an order is placed on hold.", Customer: true,
		Subject: map[string]string{"en": "Your {site_title} order has been received!", "fr": "Votre commande {site_title} a été reçue !"},
		Heading: map[string]string{"en": "Thank you for your order", "fr": "Merci pour votre commande"},
		Intro:   map[string]string{"en": "Thanks for your order. It's on-hold until we confirm that payment has been received.", "fr": "Merci pour votre commande. Elle est en attente jusqu'à ce que nous confirmions la réception du paiement."}},
	{ID: "customer_processing_order", Title: "Processing order", Description: "Sent to the customer after payment, with the order details.", Customer: true,
		Subject: map[string]string{"en": "Your {site_title} order has been received!", "fr": "Votre commande {site_title} a été reçue !"},
		Heading: map[string]string{"en": "Thank you for your order", "fr": "Merci pour votre commande"},
		Intro:   map[string]string{"en": "Just to let you know — we've received your order #{order_number}, and it is now being processed:", "fr": "Pour information, nous avons bien reçu votre commande n°{order_number}, elle est maintenant en cours de traitement :"}},
	{ID: "customer_completed_order", Title: "Completed order", Description: "Sent to the customer when the order is marked completed.", Customer: true,
		Subject: map[string]string{"en": "Your {site_title} order is now complete", "fr": "Votre commande {site_title} est maintenant terminée"},
		Heading: map[string]string{"en": "Thanks for shopping with us", "fr": "Merci pour vos achats"},
		Intro:   map[string]string{"en": "We have finished processing your order.", "fr": "Nous avons terminé le traitement de votre commande."}},
	{ID: "customer_refunded_order", Title: "Refunded order", Description: "Sent to the customer when the order is refunded.", Customer: true,
		Subject: map[string]string{"en": "Your {site_title} order #{order_number} has been refunded", "fr": "Votre commande {site_title} n°{order_number} a été remboursée"},
		Heading: map[string]string{"en": "Order Refunded: {order_number}", "fr": "Commande remboursée : {order_number}"},
		Intro:   map[string]string{"en": "Your order on {site_title} has been refunded. There are more details below for your reference:", "fr": "Votre commande sur {site_title} a été remboursée. Vous trouverez plus de détails ci-dessous :"}},
	{ID: "customer_partially_refunded_order", Title: "Partially refunded order", Description: "Sent to the customer when the order is partially refunded.", Customer: true,
		Subject: map[string]string{"en": "Your {site_title} order #{order_number} has been partially refunded", "fr": "Votre commande {site_title} n°{order_number} a été partiellement remboursée"},
		Heading: map[string]string{"en": "Partial Refund: Order {order_number}", "fr": "Remboursement partiel : commande {order_number}"},
		Intro:   map[string]string{"en": "Your order on {site_title} has been partially refunded. There are more details below for your reference:", "fr": "Votre commande sur {site_title} a été partiellement remboursée. Vous trouverez plus de détails ci-dessous :"}},
	{ID: "customer_invoice", Title: "Customer invoice / Order details", Description: "Sent manually to the customer with the order details (and payment instructions).", Customer: true,
		Subject: map[string]string{"en": "Details for order #{order_number} on {site_title}", "fr": "Détails de la commande n°{order_number} sur {site_title}"},
		Heading: map[string]string{"en": "Details for order #{order_number}", "fr": "Détails de la commande n°{order_number}"},
		Intro:   map[string]string{"en": "Here are the details of your order placed on {order_date}:", "fr": "Voici les détails de votre commande passée le {order_date} :"}},
	{ID: "customer_note", Title: "Customer note", Description: "Sent to the customer when a note for them is added to the order.", Customer: true,
		Subject: map[string]string{"en": "Note added to your {site_title} order from {order_date}", "fr": "Note ajoutée à votre commande {site_title} du {order_date}"},
		Heading: map[string]string{"en": "A note has been added to your order", "fr": "Une note a été ajoutée à votre commande"},
		Intro:   map[string]string{"en": "The following note has been added to your order:", "fr": "La note suivante a été ajoutée à votre commande :"}},
	{ID: "customer_new_account", Title: "New account", Description: "Sent to the customer when they create an account.", Customer: true,
		Subject: map[string]string{"en": "Your {site_title} account has been created!", "fr": "Votre compte {site_title} a été créé !"},
		Heading: map[string]string{"en": "Welcome to {site_title}", "fr": "Bienvenue sur {site_title}"},
		Intro:   map[string]string{"en": "Thanks for creating an account on {site_title}. You can access your account area to view orders, change your password, and more.", "fr": "Merci d'avoir créé un compte sur {site_title}. Vous pouvez accéder à votre espace client pour consulter vos commandes, modifier votre mot de passe, etc."}},
}

// EmailDefByID finds a definition.
func EmailDefByID(id string) (EmailDef, bool) {
	for _, d := range EmailDefs {
		if d.ID == id {
			return d, true
		}
	}
	return EmailDef{}, false
}

// EmailSettings is one email's configuration (option woocommerce_<id>_settings).
type EmailSettings struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	CustomerEmail     bool   `json:"customerEmail"`
	Enabled           bool   `json:"enabled"`
	Recipient         string `json:"recipient"`
	Subject           string `json:"subject"`
	Heading           string `json:"heading"`
	AdditionalContent string `json:"additionalContent"`
	EmailType         string `json:"emailType"`
	DefaultSubject    string `json:"defaultSubject"`
	DefaultHeading    string `json:"defaultHeading"`
}

// Lang is the shop language for default texts ("fr" or "en").
func (s *Store) Lang(ctx context.Context) string {
	if strings.HasPrefix(strings.ToLower(s.OptionOr(ctx, "WPLANG", "")), "fr") {
		return "fr"
	}
	if strings.HasPrefix(strings.ToLower(s.Option(ctx, "cwmerch_language")), "fr") {
		return "fr"
	}
	return "en"
}

// EmailSettingsList is every email with its settings.
func (s *Store) EmailSettingsList(ctx context.Context) []EmailSettings {
	out := []EmailSettings{}
	for _, d := range EmailDefs {
		out = append(out, s.EmailSetting(ctx, d.ID))
	}
	return out
}

// EmailSetting reads one email's settings.
func (s *Store) EmailSetting(ctx context.Context, id string) EmailSettings {
	d, _ := EmailDefByID(id)
	lang := s.Lang(ctx)
	arr := s.OptionArray(ctx, "woocommerce_"+id+"_settings")
	es := EmailSettings{ID: id, Title: d.Title, Description: d.Description, CustomerEmail: d.Customer,
		Enabled: arr.String("enabled") != "no", Recipient: arr.String("recipient"), Subject: arr.String("subject"),
		Heading: arr.String("heading"), AdditionalContent: arr.String("additional_content"), EmailType: orDefault(arr.String("email_type"), "html"),
		DefaultSubject: d.Subject[lang], DefaultHeading: d.Heading[lang]}
	if !d.Customer && es.Recipient == "" {
		es.Recipient = s.Option(ctx, "admin_email")
	}
	return es
}

// SaveEmailSetting writes one email's settings.
func (s *Store) SaveEmailSetting(ctx context.Context, id string, in EmailSettings) (EmailSettings, error) {
	if _, ok := EmailDefByID(id); !ok {
		return EmailSettings{}, ErrNotFound
	}
	key := "woocommerce_" + id + "_settings"
	arr := s.OptionArray(ctx, key)
	arr.Set("enabled", yesNo(in.Enabled)).Set("subject", in.Subject).Set("heading", in.Heading).
		Set("additional_content", in.AdditionalContent).Set("email_type", orDefault(in.EmailType, "html"))
	if d, _ := EmailDefByID(id); !d.Customer {
		arr.Set("recipient", in.Recipient)
	}
	if err := s.SetOption(ctx, key, php.Marshal(arr)); err != nil {
		return EmailSettings{}, err
	}
	return s.EmailSetting(ctx, id), nil
}
