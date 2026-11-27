// cwmerch's API: a lighter Go remake of WooCommerce working on the same
// MySQL/MariaDB database as an existing WordPress/WooCommerce (or alone).
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"

	"cwmerch-api/internal/aidownload"
	"cwmerch-api/internal/config"
	"cwmerch-api/internal/contact"
	"cwmerch-api/internal/db"
	"cwmerch-api/internal/email"
	"cwmerch-api/internal/enrich"
	"cwmerch-api/internal/handlers"
	"cwmerch-api/internal/invoicing"
	"cwmerch-api/internal/metrics"
	"cwmerch-api/internal/middleware"
	"cwmerch-api/internal/models"
	"cwmerch-api/internal/notify"
	"cwmerch-api/internal/oidc"
	"cwmerch-api/internal/payments"
	"cwmerch-api/internal/router"
	"cwmerch-api/internal/secrets"
	"cwmerch-api/internal/seo"
	"cwmerch-api/internal/store"
	"cwmerch-api/internal/telemetry"
	"cwmerch-api/internal/utils"
	"cwmerch-api/internal/woo"
	"cwmerch-api/internal/wooemail"
)

const mfaIssuer = "cwmerch"

func main() {
	cfg := config.Load()
	runtime.GOMAXPROCS(cfg.MaxWorkers)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	tel, err := telemetry.Setup(ctx, telemetry.Config{Endpoint: cfg.OtelEndpoint, Proto: cfg.OtelProto, Version: cfg.Version})
	if err != nil {
		panic(err)
	}
	defer func() { _ = tel.Shutdown(context.Background()) }()

	if utils.IsBlank(cfg.JWTSecret) {
		tel.Logger.Error("JWT_SECRET is required")
		os.Exit(1)
	}

	database, err := db.Connect(ctx, db.Config{Host: cfg.DBHost, Port: cfg.DBPort, User: cfg.DBUser, Password: cfg.DBPassword,
		Name: cfg.DBName, Prefix: cfg.DBPrefix, PoolSize: cfg.DBPoolSize})
	if err != nil {
		tel.Logger.Error("failed to connect to the database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	handlers.SetPasswordAlgo(cfg.PasswordHash)

	users := store.NewUserStore(database)
	sessions := store.NewSessionStore(database)
	apiKeys := store.NewApiKeyStore(database)
	webauthnCreds := store.NewWebAuthnCredentialStore(database)
	audit := store.NewAuditStore(database)
	aiStore := store.NewAIStore(database)
	shop := woo.New(database, cfg.UploadsDir, cfg.UploadsURL, cfg.ShopURL)

	discord := notify.NewDiscord(cfg.DiscordToken)
	discordText := func(text string) {
		if discord.Enabled() {
			go func() { _ = discord.SendText(context.Background(), text) }()
		}
	}
	enricher := enrich.NewGeo(cfg.IPInfoToken).Enrich

	mailer := email.NewSender(cfg.CWCloudAPIURL, cfg.CWCloudAPIKey, cfg.EmailFrom, cfg.APIBaseURL)
	mailer.OnFailure = func(_ context.Context, to, subject, detail string) {
		discordText(fmt.Sprintf("[ERROR] :envelope: email %q to %s could not be sent: %s", subject, to, detail))
	}
	wooMailer := wooemail.New(shop, mailer)
	wooMailer.Notify = func(_ context.Context, emailID, to string, orderID uint64, sent bool) {
		state := "sent"
		if !sent {
			state = "NOT sent"
		}
		discordText(fmt.Sprintf("[EMAIL] :envelope: %s for order #%d %s to %s", emailID, orderID, state, to))
	}
	sealer, err := secrets.New(cfg.SecretKey)
	if err != nil {
		tel.Logger.Error("CWMERCH_SECRET_KEY cannot be used", "error", err)
		os.Exit(1)
	}
	if !sealer.Configured() {
		tel.Logger.Warn("no CWMERCH_SECRET_KEY: payment credentials can only come from the environment (STRIPE_SECRET_KEY, KONNECT_API_KEY, FLOUCI_PRIVATE_KEY...)")
	}
	pay := payments.New(shop, sealer, cfg.APIBaseURL, cfg.UIBaseURL)
	pay.OnPaid = func(_ context.Context, o woo.Order, provider, reference string) {
		discordText(fmt.Sprintf("[PAYMENT] :moneybag: order #%d paid %.2f %s via %s (%s)", o.ID, o.Total, o.Currency, provider, reference))
	}
	invoices := invoicing.New(shop)
	shop.SetFeeProvider(invoices.StampFee)
	shop.SetEvents(orderEvents{mailer: wooMailer, invoices: invoices, notify: discordText})

	// A Discord notice on every user action: every audited act goes out.
	audit.AddSink(func(_ context.Context, e store.AuditEvent, actorEmail string) {
		who := actorEmail
		if who == "" {
			who = "a visitor"
		}
		subject := e.SubjectType
		if e.SubjectID != "" {
			subject += " " + e.SubjectID
		}
		discordText(fmt.Sprintf("[ACTION] :bust_in_silhouette: %s → %s (%s)", who, e.Action, subject))
	})

	var banAlerter *middleware.BanAlerter
	var onServerError middleware.ErrorFunc
	if discord.Enabled() {
		banAlerter = middleware.NewBanAlerter(func(ctx context.Context, text string) { _ = discord.SendText(ctx, text) })
		onServerError = func(method, route string, status int, cid, detail string) {
			if detail == "" {
				detail = "(no detail)"
			}
			discordText(fmt.Sprintf("[ERROR] :fire: %d on %s %s, cid = %s : %s", status, method, route, cid, detail))
		}
	}

	rpID := cfg.UIBaseURL
	if u, err := url.Parse(cfg.UIBaseURL); err == nil && utils.IsNotBlank(u.Hostname()) {
		rpID = u.Hostname()
	}
	wa, err := webauthn.New(&webauthn.Config{RPID: rpID, RPDisplayName: mfaIssuer, RPOrigins: []string{cfg.UIBaseURL}})
	if err != nil {
		tel.Logger.Error("failed to configure WebAuthn", "error", err)
		os.Exit(1)
	}

	oidcProviders := oidc.BuildProviders(cfg)
	providerNames := func() []string {
		names := []string{}
		for _, p := range oidcProviders {
			names = append(names, string(p.Name))
		}
		return names
	}

	userHandler := handlers.NewUserHandler(users, webauthnCreds, audit, cfg.JWTSecret, cfg.MaxImageSize, cfg.ActivationMode, mailer,
		cfg.APIBaseURL, cfg.UIBaseURL, cfg.ConfirmationEmailTTL).WithConn(sessions, enricher, discord)
	userHandler.OnRegistered = func(ctx context.Context, u models.User) {
		if u.Role == models.GlobalRoleConfirmed {
			go wooMailer.SendNewAccount(context.WithoutCancel(ctx), u.Email, u.Name, u.Login, cfg.UIBaseURL+"/my-account")
		}
	}

	aiDownloads := aidownload.New(cfg.AIDownloadDir, cfg.AIDownloadTTL, cfg.AIDownloadSweep, cfg.AIDownloadMaxBytes, cfg.APIBaseURL, tel.Logger)
	go aiDownloads.StartSweeper(ctx)

	reports := handlers.NewReportsHandler(shop)
	renderer := seo.New(shop, cfg.UIInternalURL, cfg.ShopURL)
	contactClient := contact.New(cfg.CWCloudAPIURL, cfg.CWCloudContactFormID)

	h := router.Handlers{
		User:       userHandler,
		Admin:      handlers.NewAdminHandler(users, webauthnCreds, audit, cfg.MaxImageSize),
		MFA:        handlers.NewMFAHandler(users, webauthnCreds, audit, cfg.JWTSecret, cfg.ActivationMode, wa, mfaIssuer).WithConn(sessions, enricher, discord),
		ApiKey:     handlers.NewApiKeyHandler(apiKeys, audit),
		OIDC:       handlers.NewOIDCHandler(oidcProviders, users, webauthnCreds, audit, cfg.JWTSecret, cfg.APIBaseURL, cfg.UIBaseURL, cfg.OIDCKeycloakGroups, cfg.ActivationMode).WithConn(sessions, enricher, discord),
		Contact:    handlers.NewContactHandler(contactClient),
		AI:         handlers.NewAIHandler(aiStore, users, shop, fmt.Sprintf("http://127.0.0.1:%s", cfg.Port), aiDownloads, audit).WithNotifier(discord),
		AIDownload: handlers.NewAIDownloadHandler(aiDownloads),
		IPBan:      handlers.NewIPBanHandler(sessions).WithNotifier(discord),
		Shop: handlers.NewShopHandler(shop, users, wooMailer, renderer, audit, cfg.JWTSecret, cfg.ActivationMode, cfg.UIBaseURL,
			providerNames, contactClient.Configured()),
		ShopAdmin: handlers.NewShopAdminHandler(shop, users, wooMailer, reports, audit, cfg.MaxUploadSize),
		Reports:   reports,
		Reviews:   handlers.NewReviewsHandler(shop, audit),
		Customers: handlers.NewCustomersHandler(shop, audit),
		Commerce:  handlers.NewCommerceHandler(shop, pay, invoices, wooMailer, users, audit),
	}
	h.Shop.WithPayments(pay)
	userHandler.RegistrationAllowed = h.Commerce.RegistrationAllowed

	met, err := metrics.Setup(ctx, metrics.Config{Endpoint: cfg.OtelEndpoint, Proto: cfg.OtelProto, Version: cfg.Version}, shopStats{shop})
	if err != nil {
		tel.Logger.Error("failed to set up metrics", "error", err)
		os.Exit(1)
	}
	defer func() { _ = met.Shutdown(context.Background()) }()
	audit.SetObserver(met.RecordAudit)

	r := router.New(h, router.Config{
		Users: users, ApiKeys: apiKeys, JWTSecret: cfg.JWTSecret, ActivationMode: cfg.ActivationMode,
		CorsEnabled: cfg.CorsEnabled, CorsAllowedOrigins: cfg.CorsAllowedOrigins, Version: cfg.Version, ManifestPath: cfg.ManifestPath,
		Telemetry: tel, Observe: met.Observe, MetricsHandler: met.Handler, Sessions: sessions, Enricher: enricher,
		BanAlerter: banAlerter, OnServerError: onServerError, APIOnly: cfg.APIOnly,
	})

	mode := "shop + back office + AI agent"
	if cfg.APIOnly {
		mode = "API only (AI agent for WooCommerce)"
	}
	discordText(fmt.Sprintf("[START] :rocket: the cwmerch API is up, version %s (%s)", cfg.Version, mode))

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r, ReadHeaderTimeout: 15 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	tel.Logger.Info("server started", "port", cfg.Port, "mode", mode, "prefix", cfg.DBPrefix)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		tel.Logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// shopStats adapts the WooCommerce store to the metrics gauges.
type shopStats struct{ s *woo.Store }

func (a shopStats) CountProductsByStatus(ctx context.Context) (map[string]int64, error) {
	return a.s.CountProductsByStatus(ctx)
}
func (a shopStats) CountOrdersByStatus(ctx context.Context) (map[string]int64, error) {
	return a.s.CountOrdersByStatus(ctx)
}
func (a shopStats) CountUsersByRole(ctx context.Context) (map[string]int64, error) {
	return a.s.CountUsersByRole(ctx)
}

// orderEvents fans an order's events out: WooCommerce's emails, then the
// invoice when invoicing is set to issue it automatically (emailed to the
// customer with the PDF).
type orderEvents struct {
	mailer   *wooemail.Mailer
	invoices *invoicing.Service
	notify   func(string)
}

func (e orderEvents) OrderStatusChanged(ctx context.Context, o woo.Order, from, to string) {
	e.mailer.OrderStatusChanged(ctx, o, from, to)
	if inv, created := e.invoices.AutoIssue(context.WithoutCancel(ctx), o, to); created {
		e.notify(fmt.Sprintf("[INVOICE] :receipt: invoice %s issued for order #%d", inv.Number, o.ID))
		if pdf, err := e.invoices.PDF(context.WithoutCancel(ctx), inv); err == nil {
			e.mailer.SendInvoice(context.WithoutCancel(ctx), o, inv.Number, invoicing.Filename(inv), pdf)
		}
	}
}

func (e orderEvents) OrderCustomerNote(ctx context.Context, o woo.Order, note string) {
	e.mailer.OrderCustomerNote(ctx, o, note)
}

func (e orderEvents) OrderRefunded(ctx context.Context, o woo.Order, r woo.Refund, full bool) {
	e.mailer.OrderRefunded(ctx, o, r, full)
}
