package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"encoding/json"

	"github.com/go-chi/chi/v5"

	"cwmerch-api/internal/authtoken"
	"cwmerch-api/internal/middleware"
	"cwmerch-api/internal/models"
	"cwmerch-api/internal/payments"
	"cwmerch-api/internal/seo"
	"cwmerch-api/internal/store"
	"cwmerch-api/internal/utils"
	"cwmerch-api/internal/woo"
	"cwmerch-api/internal/wooemail"
)

// ShopHandler serves the public storefront and the customer's account data.
type ShopHandler struct {
	auditor
	woo            *woo.Store
	users          *store.UserStore
	mailer         *wooemail.Mailer
	seo            *seo.Renderer
	jwtSecret      string
	activationMode string
	uiBaseURL      string
	maxUploadBytes int64
	oidcProviders  func() []string
	contactEnabled bool
	payments       *payments.Service
}

// WithPayments wires the online payment providers.
func (h *ShopHandler) WithPayments(p *payments.Service) *ShopHandler {
	h.payments = p
	return h
}

func NewShopHandler(w *woo.Store, users *store.UserStore, mailer *wooemail.Mailer, renderer *seo.Renderer, audit *store.AuditStore,
	jwtSecret, activationMode, uiBaseURL string, oidcProviders func() []string, contactEnabled bool) *ShopHandler {
	return &ShopHandler{auditor: newAuditor(audit), woo: w, users: users, mailer: mailer, seo: renderer, jwtSecret: jwtSecret,
		activationMode: activationMode, uiBaseURL: uiBaseURL, oidcProviders: oidcProviders, contactEnabled: contactEnabled}
}

// writeWooError maps woo refusals to their status and i18n code.
func writeWooError(w http.ResponseWriter, err error) { writeWooErrorRC(w, err) }

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", CodeInvalidRequestBody)
		return false
	}
	return true
}

func idParam(r *http.Request, name string) uint64 { return woo.ParseID(chi.URLParam(r, name)) }

func queryInt(r *http.Request, name string, def int) int { return rcInt(r, name, def) }

func queryFloat(r *http.Request, name string) *float64 {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	return &f
}

func queryFlag(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func queryDate(r *http.Request, name string) *time.Time {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil
	}
	return &t
}

func callerID(r *http.Request) uint64 {
	id, _ := middleware.UserIDFromContext(r.Context())
	return woo.ParseID(id)
}

// Settings is GET /v1/shop/settings.
func (h *ShopHandler) Settings(w http.ResponseWriter, r *http.Request) {
	set := h.woo.ShopSettings(r.Context())
	set.ContactEnabled = h.contactEnabled || h.woo.CWCloudSettings(r.Context()).ContactMode != "disabled"
	if h.oidcProviders != nil {
		set.OIDCProviders = h.oidcProviders()
	}
	online := 0
	if h.payments != nil {
		for _, g := range h.payments.Gateways(r.Context(), set.Currency) {
			set.PaymentGateways = append(set.PaymentGateways, woo.GatewaySummary{ID: g.ID, Title: g.Title, Description: g.Description,
				Type: g.Type, PublishableKey: g.PublishableKey})
			online++
		}
	}
	set.PhoneConfirmation = online == 0
	writeJSON(w, http.StatusOK, set)
}

// Countries is GET /v1/shop/countries: where the shop sells, with states.
func (h *ShopHandler) Countries(w http.ResponseWriter, r *http.Request) {
	allowed := map[string]bool{}
	for _, c := range h.woo.AllowedCountries(r.Context()) {
		allowed[c] = true
	}
	out := []woo.Country{}
	for _, c := range woo.AllCountries(r.URL.Query().Get("lang")) {
		if allowed[c.Code] {
			out = append(out, c)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// AllCountries is GET /v1/countries: every country ({iso, name}).
func (h *ShopHandler) AllCountries(w http.ResponseWriter, r *http.Request) {
	type row struct {
		ISO       string      `json:"iso"`
		Code      string      `json:"code"`
		Name      string      `json:"name"`
		Continent string      `json:"continent"`
		States    []woo.State `json:"states"`
	}
	out := []row{}
	for _, c := range woo.AllCountries(r.URL.Query().Get("lang")) {
		out = append(out, row{ISO: c.Code, Code: c.Code, Name: c.Name, Continent: c.Continent, States: c.States})
	}
	writeJSON(w, http.StatusOK, out)
}

// Currencies is GET /v1/currencies.
func (h *ShopHandler) Currencies(w http.ResponseWriter, r *http.Request) {
	type row struct {
		Code   string `json:"code"`
		Symbol string `json:"symbol"`
	}
	out := []row{}
	for _, c := range woo.CurrencyCodes() {
		out = append(out, row{Code: c, Symbol: woo.CurrencySymbol(c)})
	}
	writeJSON(w, http.StatusOK, out)
}

func productQuery(r *http.Request, public bool) woo.ProductQuery {
	q := r.URL.Query()
	return woo.ProductQuery{Search: strings.TrimSpace(q.Get("search")), Category: q.Get("category"), Tag: q.Get("tag"),
		MinPrice: queryFloat(r, "minPrice"), MaxPrice: queryFloat(r, "maxPrice"), OnSale: queryFlag(r, "onSale"),
		Featured: queryFlag(r, "featured"), Status: q.Get("status"), Type: q.Get("type"), StockStatus: q.Get("stockStatus"),
		OrderBy: q.Get("orderby"), Order: q.Get("order"), Include: woo.ParseIDList(q.Get("include")), Exclude: woo.ParseIDList(q.Get("exclude")),
		Page: queryInt(r, "page", 1), PerPage: queryInt(r, "perPage", 20), Public: public}
}

// displayPrices turns prices into the shop's display mode (incl./excl. tax),
// so the storefront shows what WooCommerce would.
func (h *ShopHandler) displayPrices(r *http.Request, products []woo.Product) {
	h.woo.ApplyDisplayPrices(r.Context(), products)
}

// Products is GET /v1/shop/products.
func (h *ShopHandler) Products(w http.ResponseWriter, r *http.Request) {
	res, err := h.woo.Products(r.Context(), productQuery(r, true))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.displayPrices(r, res.Items)
	writeJSON(w, http.StatusOK, res)
}

// Product is GET /v1/shop/products/{idOrSlug}.
func (h *ShopHandler) Product(w http.ResponseWriter, r *http.Request) {
	p, err := h.woo.PublicProduct(r.Context(), chi.URLParam(r, "idOrSlug"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	list := []woo.Product{p}
	h.displayPrices(r, list)
	writeJSON(w, http.StatusOK, list[0])
}

// Categories is GET /v1/shop/categories.
func (h *ShopHandler) Categories(w http.ResponseWriter, r *http.Request) {
	terms, err := h.woo.Terms(r.Context(), woo.TaxCategory)
	if err != nil {
		writeWooError(w, err)
		return
	}
	if queryFlag(r, "hideEmpty") {
		kept := []woo.Term{}
		for _, t := range terms {
			if t.Count > 0 {
				kept = append(kept, t)
			}
		}
		terms = kept
	}
	writeJSON(w, http.StatusOK, terms)
}

// Tags is GET /v1/shop/tags.
func (h *ShopHandler) Tags(w http.ResponseWriter, r *http.Request) {
	terms, err := h.woo.Terms(r.Context(), woo.TaxTag)
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, terms)
}

func (h *ShopHandler) cartFor(r *http.Request, req woo.CartRequest) (woo.Cart, error) {
	if uid := callerID(r); uid != 0 {
		req.UserID = uid
		if req.Email == "" {
			if u, err := h.users.FindByID(r.Context(), strconv.FormatUint(uid, 10)); err == nil {
				req.Email = u.Email
			}
		}
	}
	return h.woo.ComputeCart(r.Context(), req)
}

// Cart is POST /v1/shop/cart: prices a cart.
func (h *ShopHandler) Cart(w http.ResponseWriter, r *http.Request) {
	var req woo.CartRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	cart, err := h.cartFor(r, req)
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cart)
}

type checkoutPayload struct {
	Items                  []woo.CartLine `json:"items"`
	Coupons                []string       `json:"coupons"`
	ShippingMethod         string         `json:"shippingMethod"`
	Billing                woo.Address    `json:"billing"`
	Shipping               woo.Address    `json:"shipping"`
	ShipToDifferentAddress bool           `json:"shipToDifferentAddress"`
	PaymentMethod          string         `json:"paymentMethod"`
	CustomerNote           string         `json:"customerNote"`
	CreateAccount          bool           `json:"createAccount"`
	AccountPassword        string         `json:"accountPassword"`
}

type checkoutResponse struct {
	Order               woo.Order         `json:"order"`
	PaymentInstructions string            `json:"paymentInstructions"`
	Redirect            string            `json:"redirect"`
	Token               string            `json:"token,omitempty"`
	Payment             *payments.Session `json:"payment,omitempty"`
	// Confirmation is set when the order will be confirmed by phone (no
	// online payment partner configured).
	Confirmation *phoneConfirmation `json:"confirmation,omitempty"`
}

type phoneConfirmation struct {
	OrderNumber string `json:"orderNumber"`
	Phone       string `json:"phone"`
	Message     string `json:"message"`
}

// validPhone accepts international/local formats with at least 8 digits.
func validPhone(p string) bool {
	digits := 0
	for _, r := range p {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '+' || r == ' ' || r == '.' || r == '-' || r == '(' || r == ')':
		default:
			return false
		}
	}
	return digits >= 8 && digits <= 15
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return strings.Trim(host, "[]")
}

func validateAddress(a woo.Address, billing bool) string {
	missing := []string{}
	if strings.TrimSpace(a.FirstName) == "" {
		missing = append(missing, "firstName")
	}
	if strings.TrimSpace(a.LastName) == "" {
		missing = append(missing, "lastName")
	}
	if strings.TrimSpace(a.Address1) == "" {
		missing = append(missing, "address1")
	}
	if strings.TrimSpace(a.City) == "" {
		missing = append(missing, "city")
	}
	if strings.TrimSpace(a.Country) == "" {
		missing = append(missing, "country")
	}
	if billing && !strings.Contains(a.Email, "@") {
		missing = append(missing, "email")
	}
	return strings.Join(missing, ", ")
}

// Checkout is POST /v1/shop/checkout: places an order.
func (h *ShopHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	var p checkoutPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	ctx := r.Context()
	set := h.woo.ShopSettings(ctx)
	onlineCount := 0
	if h.payments != nil {
		for _, g := range h.payments.Gateways(ctx, set.Currency) {
			set.PaymentGateways = append(set.PaymentGateways, woo.GatewaySummary{ID: g.ID, Title: g.Title, Type: g.Type})
			onlineCount++
		}
	}
	// No payment partner: the order is confirmed by phone, so the number is required.
	phoneConfirm := onlineCount == 0
	if phoneConfirm && !validPhone(strings.TrimSpace(p.Billing.Phone)) {
		writeError(w, http.StatusBadRequest, "A valid phone number is required to confirm your order", "errors.phoneRequired")
		return
	}
	uid := callerID(r)
	if !p.ShipToDifferentAddress {
		email := p.Shipping.Email
		p.Shipping = p.Billing
		p.Shipping.Email = email
	}
	if missing := validateAddress(p.Billing, true); missing != "" {
		writeError(w, http.StatusBadRequest, "Missing billing fields: "+missing, "errors.checkoutMissingFields")
		return
	}
	var token string
	if uid == 0 && p.CreateAccount {
		if !set.SignupAtCheckout {
			writeError(w, http.StatusBadRequest, "Creating an account during checkout is disabled", "errors.signupDisabled")
			return
		}
		if ok, code := utils.IsPasswordValid(p.AccountPassword); !ok {
			writeInvalidPassword(w, code)
			return
		}
		hashed, err := hashPassword(p.AccountPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), CodeInternal)
			return
		}
		user, err := h.users.CreateWithMode(ctx, strings.TrimSpace(p.Billing.Email), hashed, p.Billing.FirstName, p.Billing.LastName, models.ActivationModeAuto)
		if err != nil {
			if errors.Is(err, store.ErrDuplicateEmail) {
				writeError(w, http.StatusBadRequest, "An account is already registered with your email address. Please log in.", CodeDuplicateEmail)
				return
			}
			writeStoreError(w, err)
			return
		}
		uid = woo.ParseID(user.ID)
		token, _ = authtoken.Generate(h.jwtSecret, user.ID)
		h.recordAs(ctx, user.ID, models.AuditAccountRegistered, models.AuditSubjectUser, user.ID, map[string]any{"email": user.Email, "via": "checkout"})
		if h.mailer != nil {
			go h.mailer.SendNewAccount(ctx, user.Email, user.Name, user.Login, h.uiBaseURL+"/my-account")
		}
	}
	if uid == 0 && !set.GuestCheckout {
		writeError(w, http.StatusUnauthorized, "You must be logged in to checkout.", "errors.loginRequiredForCheckout")
		return
	}
	billingLoc := woo.Location{Country: p.Billing.Country, State: p.Billing.State, Postcode: p.Billing.Postcode, City: p.Billing.City}
	cart, err := h.woo.ComputeCart(ctx, woo.CartRequest{Items: p.Items, Coupons: p.Coupons, ShippingMethod: p.ShippingMethod,
		Customer: woo.Location{Country: p.Shipping.Country, State: p.Shipping.State, Postcode: p.Shipping.Postcode, City: p.Shipping.City},
		Email:    p.Billing.Email, UserID: uid, Billing: &billingLoc})
	if err != nil {
		writeWooError(w, err)
		return
	}
	if cart.HasErrors() {
		writeJSON(w, http.StatusConflict, map[string]any{"message": cart.Notices[0].Message, "i18n_code": "errors.cartInvalid", "cart": cart})
		return
	}
	if cart.NeedsShipping && cart.ChosenShipping == nil {
		writeError(w, http.StatusBadRequest, "Please choose a shipping method", "errors.shippingRequired")
		return
	}
	if cart.NeedsPayment && !(phoneConfirm && p.PaymentMethod == "") {
		ok := false
		for _, g := range set.PaymentGateways {
			if g.ID == p.PaymentMethod {
				ok = true
			}
		}
		if !ok {
			writeError(w, http.StatusBadRequest, "Invalid payment method", "errors.invalidPaymentMethod")
			return
		}
	}
	online := h.payments != nil && h.payments.IsOnline(p.PaymentMethod) && cart.NeedsPayment
	paymentTitle := ""
	if online {
		paymentTitle = h.payments.Title(ctx, p.PaymentMethod)
	}
	if phoneConfirm && p.PaymentMethod == "" {
		paymentTitle = map[string]string{"fr": "Confirmation par téléphone", "ar": "تأكيد عبر الهاتف"}[h.woo.Lang(ctx)]
		if paymentTitle == "" {
			paymentTitle = "Confirmation by phone"
		}
	}
	order, err := h.woo.CreateOrder(ctx, woo.NewOrderInput{Cart: cart, Billing: p.Billing, Shipping: p.Shipping, PaymentMethod: p.PaymentMethod,
		PaymentTitle: paymentTitle, CustomerID: uid, CustomerNote: p.CustomerNote, CreatedVia: "checkout", IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	if err != nil {
		writeWooError(w, err)
		return
	}
	if online {
		// Online payment: the order stays pending until the provider confirms.
		sess, err := h.payments.Start(ctx, order)
		if err != nil {
			_, _ = h.woo.SetStatus(ctx, order.ID, "failed", "The payment could not be started.", false)
			writeWooError(w, err)
			return
		}
		if uid != 0 {
			h.saveCustomerAddresses(r, uid, p.Billing, p.Shipping)
		}
		h.recordAs(ctx, strconv.FormatUint(uid, 10), models.AuditOrderPlaced, models.AuditSubjectOrder, strconv.FormatUint(order.ID, 10),
			map[string]any{"total": order.Total, "currency": order.Currency, "email": order.Billing.Email, "payment": order.PaymentMethod})
		writeJSON(w, http.StatusCreated, checkoutResponse{Order: order, Payment: &sess, Token: token,
			Redirect: fmt.Sprintf("/checkout/payment/%d?key=%s", order.ID, order.OrderKey)})
		return
	}
	// WooCommerce's offline gateways: BACS/cheque leave the order on hold
	// until the money arrives; cash on delivery goes straight to processing.
	next := "on-hold"
	note := ""
	if (p.PaymentMethod == "cod" || !cart.NeedsPayment) && !phoneConfirm {
		next = "processing"
	}
	if phoneConfirm {
		// Placed on hold until the shop calls the customer back.
		next, note = "on-hold", "To confirm by phone with the customer at "+strings.TrimSpace(p.Billing.Phone)+"."
	}
	if order, err = h.woo.SetStatus(ctx, order.ID, next, note, false); err != nil {
		writeWooError(w, err)
		return
	}
	if uid != 0 {
		h.saveCustomerAddresses(r, uid, p.Billing, p.Shipping)
	}
	h.recordAs(ctx, strconv.FormatUint(uid, 10), models.AuditOrderPlaced, models.AuditSubjectOrder, strconv.FormatUint(order.ID, 10),
		map[string]any{"total": order.Total, "currency": order.Currency, "email": order.Billing.Email, "payment": order.PaymentMethod})
	resp := checkoutResponse{Order: order, PaymentInstructions: h.woo.PaymentInstructions(ctx, order.PaymentMethod),
		Redirect: fmt.Sprintf("/checkout/order-received/%d?key=%s", order.ID, order.OrderKey), Token: token}
	if phoneConfirm {
		msg := "Your order number is %s. We will call you at %s to confirm it."
		switch h.woo.Lang(ctx) {
		case "fr":
			msg = "Votre numéro de commande est %s. Nous vous appellerons au %s pour la confirmer."
		case "ar":
			msg = "رقم طلبك هو %s. سنتصل بك على الرقم %s لتأكيده."
		}
		phone := strings.TrimSpace(p.Billing.Phone)
		resp.Confirmation = &phoneConfirmation{OrderNumber: order.Number, Phone: phone, Message: fmt.Sprintf(msg, order.Number, phone)}
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *ShopHandler) saveCustomerAddresses(r *http.Request, uid uint64, billing, shipping woo.Address) {
	values := map[string]string{}
	for k, v := range woo.AddressMeta(billing, "billing_") {
		values[k] = v
	}
	ship := woo.AddressMeta(shipping, "shipping_")
	delete(ship, "shipping_email")
	for k, v := range ship {
		values[k] = v
	}
	_ = h.users.SetUserMetas(r.Context(), strconv.FormatUint(uid, 10), values)
	_ = h.woo.UpsertCustomerLookup(r.Context(), uid)
}

// OrderReceived is GET /v1/shop/orders/{id}?key=: the thank-you page.
func (h *ShopHandler) OrderReceived(w http.ResponseWriter, r *http.Request) {
	o, err := h.woo.OrderByKey(r.Context(), idParam(r, "id"), r.URL.Query().Get("key"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	o.CustomerIPAddress, o.CustomerUserAgent = "", ""
	o.PaymentInstructions = h.woo.PaymentInstructions(r.Context(), o.PaymentMethod)
	writeJSON(w, http.StatusOK, o)
}

// Appearance is GET /v1/shop/appearance.
func (h *ShopHandler) Appearance(w http.ResponseWriter, r *http.Request) {
	a, err := h.woo.Appearance(r.Context(), false)
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// Themes is GET /v1/shop/themes.
func (h *ShopHandler) Themes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, woo.Themes)
}

// CustomCSS is GET /v1/shop/custom.css.
func (h *ShopHandler) CustomCSS(w http.ResponseWriter, r *http.Request) {
	css, _, err := h.woo.AppearanceCSS(r.Context())
	if err != nil {
		writeWooError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(css))
}

// Media is GET /v1/media/*: uploaded files when cwmerch owns the uploads folder.
func (h *ShopHandler) Media(w http.ResponseWriter, r *http.Request) {
	path, ok := h.woo.MediaFile(chi.URLParam(r, "*"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasSuffix(strings.ToLower(path), ".svg") {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	}
	http.ServeFile(w, r, path)
}

// SEOHTML is GET /v1/seo/html?path=: index.html with the page's head.
func (h *ShopHandler) SEOHTML(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if orig := r.Header.Get("X-Original-URI"); path == "" && orig != "" {
		path = orig
	}
	if path == "" {
		path = "/"
	}
	tpl, err := h.seo.Template(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error(), "errors.seoTemplateUnavailable")
		return
	}
	head := h.seo.Page(r.Context(), path)
	status := http.StatusOK
	if kind, _ := h.woo.Permalinks(r.Context()).RouteKind(path); kind == "product" && head.NoIndex && strings.Contains(head.Title, "Not found") {
		status = http.StatusNotFound
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(seo.Inject(tpl, head)))
}

// Sitemap is GET /sitemap.xml.
func (h *ShopHandler) Sitemap(w http.ResponseWriter, r *http.Request) {
	out, err := h.seo.Sitemap(r.Context())
	if err != nil {
		writeWooError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write(out)
}

// Robots is GET /robots.txt.
func (h *ShopHandler) Robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(h.seo.Robots()))
}

// ------------------------------------------------------------------ account

type addressesPayload struct {
	Billing  woo.Address `json:"billing"`
	Shipping woo.Address `json:"shipping"`
}

var addressKeys = []string{"first_name", "last_name", "company", "address_1", "address_2", "city", "state", "postcode", "country", "email", "phone"}

// MyAddresses is GET /v1/users/me/addresses.
func (h *ShopHandler) MyAddresses(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserIDFromContext(r.Context())
	keys := []string{}
	for _, k := range addressKeys {
		keys = append(keys, "billing_"+k, "shipping_"+k)
	}
	meta, err := h.users.UserMeta(r.Context(), uid, keys...)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	ship := woo.AddressFromMeta(meta, "shipping_")
	ship.Email = ""
	writeJSON(w, http.StatusOK, addressesPayload{Billing: woo.AddressFromMeta(meta, "billing_"), Shipping: ship})
}

// UpdateMyAddresses is PUT /v1/users/me/addresses.
func (h *ShopHandler) UpdateMyAddresses(w http.ResponseWriter, r *http.Request) {
	var p addressesPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	uid := callerID(r)
	h.saveCustomerAddresses(r, uid, p.Billing, p.Shipping)
	h.record(r, models.AuditAddressesUpdated, models.AuditSubjectUser, strconv.FormatUint(uid, 10), nil)
	h.MyAddresses(w, r)
}

func (h *ShopHandler) me(r *http.Request) (models.User, error) {
	uid, _ := middleware.UserIDFromContext(r.Context())
	return h.users.FindByID(r.Context(), uid)
}

// MyOrders is GET /v1/users/me/orders.
func (h *ShopHandler) MyOrders(w http.ResponseWriter, r *http.Request) {
	u, err := h.me(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	res, err := h.woo.Orders(r.Context(), woo.OrderQuery{CustomerID: woo.ParseID(u.ID), Status: r.URL.Query().Get("status"),
		Page: queryInt(r, "page", 1), PerPage: queryInt(r, "perPage", 20)})
	if err != nil {
		writeWooError(w, err)
		return
	}
	for i := range res.Items {
		res.Items[i].CustomerIPAddress, res.Items[i].CustomerUserAgent = "", ""
	}
	writeJSON(w, http.StatusOK, res)
}

// MyOrder is GET /v1/users/me/orders/{id}.
func (h *ShopHandler) MyOrder(w http.ResponseWriter, r *http.Request) {
	u, err := h.me(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	o, err := h.woo.Order(r.Context(), idParam(r, "id"))
	if err != nil || o.CustomerID != woo.ParseID(u.ID) {
		writeError(w, http.StatusNotFound, "Order not found", CodeNotFound)
		return
	}
	o.CustomerIPAddress, o.CustomerUserAgent = "", ""
	notes, _ := h.woo.Notes(r.Context(), o.ID, true)
	writeJSON(w, http.StatusOK, map[string]any{"order": o, "notes": notes, "paymentInstructions": o.PaymentInstructions})
}

// MyDownloads is GET /v1/users/me/downloads.
func (h *ShopHandler) MyDownloads(w http.ResponseWriter, r *http.Request) {
	u, err := h.me(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	list, err := h.woo.Downloads(r.Context(), woo.ParseID(u.ID), u.Email)
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// Download is GET /v1/users/me/downloads/{productId}/{downloadId}: counts the
// download and redirects to the file.
func (h *ShopHandler) Download(w http.ResponseWriter, r *http.Request) {
	u, err := h.me(r)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	file, err := h.woo.UseDownload(r.Context(), woo.ParseID(u.ID), u.Email, idParam(r, "productId"), chi.URLParam(r, "downloadId"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": file})
}
