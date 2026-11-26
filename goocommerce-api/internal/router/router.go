// Package router wires every goocommerce endpoint. The OpenAPI document is
// generated from this router after every route is registered, so it can
// never drift from the code; Swagger UI serves it at "/".
package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"goocommerce-api/internal/handlers"
	"goocommerce-api/internal/middleware"
	"goocommerce-api/internal/openapi"
	"goocommerce-api/internal/store"
	"goocommerce-api/internal/telemetry"
	"goocommerce-api/internal/woo"
)

// Handlers is every handler the router mounts.
type Handlers struct {
	User       *handlers.UserHandler
	Admin      *handlers.AdminHandler
	MFA        *handlers.MFAHandler
	ApiKey     *handlers.ApiKeyHandler
	OIDC       *handlers.OIDCHandler
	Contact    *handlers.ContactHandler
	AI         *handlers.AIHandler
	AIDownload *handlers.AIDownloadHandler
	IPBan      *handlers.IPBanHandler
	Shop       *handlers.ShopHandler
	ShopAdmin  *handlers.ShopAdminHandler
	Reports    *handlers.ReportsHandler
	Reviews    *handlers.ReviewsHandler
	Customers  *handlers.CustomersHandler
}

// Config is what the router needs besides the handlers.
type Config struct {
	Users              *store.UserStore
	ApiKeys            middleware.ApiKeyVerifier
	JWTSecret          string
	ActivationMode     string
	CorsEnabled        bool
	CorsAllowedOrigins []string
	Version            string
	ManifestPath       string
	Telemetry          *telemetry.Providers
	Observe            middleware.EndpointObserver
	MetricsHandler     http.Handler
	Sessions           middleware.VisitorStore
	Enricher           middleware.Enricher
	BanAlerter         *middleware.BanAlerter
	OnServerError      middleware.ErrorFunc
	// APIOnly disables the storefront/back-office routes: goocommerce then
	// only serves as an AI agent (+ API keys, MFA, accounting exports) next to
	// an unchanged WooCommerce.
	APIOnly bool
}

func New(h Handlers, c Config) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Correlation)
	r.Use(middleware.Instrument(c.Telemetry.Tracer, c.Telemetry.Logger, c.Observe, c.OnServerError))
	if c.CorsEnabled {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins: c.CorsAllowedOrigins,
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Authorization", "Content-Type", "X-Api-Key", "X-Cid"},
			ExposedHeaders: []string{"Content-Disposition", "X-Cid"},
		}))
	}
	if c.Sessions != nil {
		r.Use(middleware.BanIP(c.Sessions, c.Enricher, c.BanAlerter, c.JWTSecret, c.ApiKeys))
	}

	auth := middleware.Auth(c.JWTSecret, c.ApiKeys)
	optional := middleware.OptionalAuth(c.JWTSecret, c.ApiKeys)
	active := middleware.RequireActiveUser(c.Users, c.ActivationMode, c.BanAlerter)
	manager := middleware.RequireManager(c.Users)
	superuser := middleware.RequireSuperuser(c.Users)

	r.Route("/v1", func(r chi.Router) {
		r.Get("/health", handlers.Health)
		r.Get("/manifest", handlers.NewManifestHandler(c.ManifestPath))
		r.Get("/assets/logo.png", handlers.AssetsLogo)
		r.Post("/contact", h.Contact.Create)
		r.Get("/countries", h.Shop.AllCountries)
		r.Get("/currencies", h.Shop.Currencies)
		if h.AIDownload != nil {
			r.Get("/ai-downloads/{id}", h.AIDownload.Download)
		}

		r.Route("/oidc", func(r chi.Router) {
			r.Get("/", h.OIDC.ListProviders)
			r.Get("/callback", h.OIDC.FrontendCallback)
			r.Get("/{provider}/login", h.OIDC.Login)
			r.Get("/{provider}/callback", h.OIDC.Callback)
		})

		r.Get("/user/confirmation", h.User.Confirm)

		r.Route("/users", func(r chi.Router) {
			r.Post("/", h.User.Register)
			r.Post("/login", h.User.Login)
			r.Post("/forgot-password", h.User.ForgotPassword)
			r.Post("/reset-password", h.User.ResetPassword)
			r.Route("/login/mfa", func(r chi.Router) {
				r.Post("/totp", h.MFA.LoginTOTP)
				r.Post("/webauthn/begin", h.MFA.LoginWebAuthnBegin)
				r.Post("/webauthn/finish", h.MFA.LoginWebAuthnFinish)
			})
			r.Group(func(r chi.Router) {
				r.Use(auth)
				r.Get("/me", h.User.Me)
				r.Post("/logout", h.User.Logout)
				r.Group(func(r chi.Router) {
					r.Use(active)
					r.Put("/me", h.User.UpdateProfile)
					r.Put("/me/picture", h.User.UpdatePicture)
					r.Route("/me/api-keys", func(r chi.Router) {
						r.Get("/", h.ApiKey.List)
						r.Post("/", h.ApiKey.Create)
						r.Delete("/{id}", h.ApiKey.Delete)
					})
					r.Route("/me/mfa", func(r chi.Router) {
						r.Get("/", h.MFA.Status)
						r.Post("/totp/setup", h.MFA.TOTPSetup)
						r.Post("/totp/confirm", h.MFA.TOTPConfirm)
						r.Delete("/totp", h.MFA.TOTPDisable)
						r.Post("/webauthn/register/begin", h.MFA.WebAuthnRegisterBegin)
						r.Post("/webauthn/register/finish", h.MFA.WebAuthnRegisterFinish)
						r.Delete("/webauthn/{credentialId}", h.MFA.WebAuthnDelete)
					})
					if !c.APIOnly {
						r.Get("/me/addresses", h.Shop.MyAddresses)
						r.Put("/me/addresses", h.Shop.UpdateMyAddresses)
						r.Get("/me/orders/", h.Shop.MyOrders)
						r.Get("/me/orders/{id}", h.Shop.MyOrder)
						r.Get("/me/downloads", h.Shop.MyDownloads)
						r.Get("/me/downloads/{productId}/{downloadId}", h.Shop.Download)
					}
				})
			})
		})

		// --- public storefront ---
		r.Route("/shop", func(r chi.Router) {
			r.Get("/settings", h.Shop.Settings)
			r.Get("/countries", h.Shop.Countries)
			r.Get("/appearance", h.Shop.Appearance)
			r.Get("/themes", h.Shop.Themes)
			r.Get("/custom.css", h.Shop.CustomCSS)
			r.Get("/products", h.Shop.Products)
			r.Get("/products/{idOrSlug}", h.Shop.Product)
			r.Get("/products/{id}/reviews", h.Reviews.ShopList)
			r.With(optional).Post("/products/{id}/reviews", h.Reviews.ShopCreate)
			r.Get("/categories", h.Shop.Categories)
			r.Get("/tags", h.Shop.Tags)
			if !c.APIOnly {
				r.With(optional).Post("/cart", h.Shop.Cart)
				r.With(optional).Post("/checkout", h.Shop.Checkout)
				r.Get("/orders/{id}", h.Shop.OrderReceived)
			}
		})
		r.Get("/seo/html", h.Shop.SEOHTML)
		r.Get("/media/*", h.Shop.Media)

		// --- back office (shop managers and administrators) ---
		r.Route("/admin", func(r chi.Router) {
			r.Use(auth, active)
			r.Group(func(r chi.Router) {
				r.Use(manager)
				r.Get("/dashboard", h.ShopAdmin.Dashboard)
				r.Route("/reports", func(r chi.Router) {
					r.Get("/summary", h.Reports.Summary)
					r.Get("/sales", h.Reports.Sales)
					r.Get("/top-products", h.Reports.TopProducts)
					r.Get("/top-categories", h.Reports.TopCategories)
					r.Get("/stock", h.Reports.Stock)
					r.Get("/customers", h.Reports.Customers)
				})
				r.Route("/accounting", func(r chi.Router) {
					r.Get("/turnover", h.Reports.Turnover)
					r.Get("/balance", h.Reports.Balance)
					r.Get("/ledger", h.Reports.Ledger)
				})
				r.Route("/products", func(r chi.Router) {
					r.Get("/", h.ShopAdmin.Products)
					r.Post("/", h.ShopAdmin.CreateProduct)
					r.Get("/{id}", h.ShopAdmin.Product)
					r.Put("/{id}", h.ShopAdmin.UpdateProduct)
					r.Delete("/{id}", h.ShopAdmin.DeleteProduct)
					r.Post("/{id}/restore", h.ShopAdmin.RestoreProduct)
					r.Post("/{id}/duplicate", h.ShopAdmin.DuplicateProduct)
					r.Get("/{id}/variations/", h.ShopAdmin.Variations)
					r.Post("/{id}/variations/", h.ShopAdmin.CreateVariation)
					r.Post("/{id}/variations/generate", h.ShopAdmin.GenerateVariations)
					r.Put("/{id}/variations/{variationId}", h.ShopAdmin.UpdateVariation)
					r.Delete("/{id}/variations/{variationId}", h.ShopAdmin.DeleteVariation)
				})
				mountTerms := func(path, taxonomy string) {
					list, create, update, del := h.ShopAdmin.Terms(taxonomy)
					r.Route(path, func(r chi.Router) {
						r.Get("/", list)
						r.Post("/", create)
						r.Put("/{id}", update)
						r.Delete("/{id}", del)
					})
				}
				mountTerms("/categories", woo.TaxCategory)
				mountTerms("/tags", woo.TaxTag)
				mountTerms("/shipping/classes", woo.TaxShippingClass)
				r.Route("/attributes", func(r chi.Router) {
					r.Get("/", h.ShopAdmin.Attributes)
					r.Post("/", h.ShopAdmin.CreateAttribute)
					r.Put("/{id}", h.ShopAdmin.UpdateAttribute)
					r.Delete("/{id}", h.ShopAdmin.DeleteAttribute)
					r.Get("/{id}/terms/", h.ShopAdmin.AttributeTerms)
					r.Post("/{id}/terms/", h.ShopAdmin.AttributeTerms)
					r.Put("/{id}/terms/{termId}", h.ShopAdmin.AttributeTerms)
					r.Delete("/{id}/terms/{termId}", h.ShopAdmin.AttributeTerms)
				})
				r.Get("/media/", h.ShopAdmin.Media)
				r.Post("/media/", h.ShopAdmin.UploadMedia)
				r.Route("/orders", func(r chi.Router) {
					r.Get("/", h.ShopAdmin.Orders)
					r.Post("/", h.ShopAdmin.CreateOrder)
					r.Get("/{id}", h.ShopAdmin.Order)
					r.Put("/{id}", h.ShopAdmin.UpdateOrder)
					r.Delete("/{id}", h.ShopAdmin.DeleteOrder)
					r.Get("/{id}/notes/", h.ShopAdmin.Notes)
					r.Post("/{id}/notes/", h.ShopAdmin.AddNote)
					r.Delete("/{id}/notes/{noteId}", h.ShopAdmin.DeleteNote)
					r.Post("/{id}/refunds", h.ShopAdmin.Refund)
					r.Post("/{id}/emails", h.ShopAdmin.SendOrderEmail)
				})
				r.Route("/customers", func(r chi.Router) {
					r.Get("/", h.Customers.List)
					r.Get("/{id}", h.Customers.Get)
					r.Put("/{id}", h.Customers.Update)
					r.Get("/{id}/orders", h.Customers.Orders)
				})
				r.Route("/coupons", func(r chi.Router) {
					r.Get("/", h.ShopAdmin.Coupons)
					r.Post("/", h.ShopAdmin.CreateCoupon)
					r.Get("/{id}", h.ShopAdmin.Coupon)
					r.Put("/{id}", h.ShopAdmin.UpdateCoupon)
					r.Delete("/{id}", h.ShopAdmin.DeleteCoupon)
				})
				r.Route("/reviews", func(r chi.Router) {
					r.Get("/", h.Reviews.AdminList)
					r.Get("/{id}", h.Reviews.AdminGet)
					r.Put("/{id}", h.Reviews.AdminUpdate)
					r.Delete("/{id}", h.Reviews.AdminDelete)
					r.Post("/{id}/reply", h.Reviews.AdminReply)
				})
				r.Get("/settings/", h.ShopAdmin.Settings)
				r.Put("/settings/", h.ShopAdmin.UpdateSettings)
				r.Route("/taxes", func(r chi.Router) {
					r.Get("/", h.ShopAdmin.Taxes)
					r.Post("/", h.ShopAdmin.CreateTax)
					r.Put("/{id}", h.ShopAdmin.UpdateTax)
					r.Delete("/{id}", h.ShopAdmin.DeleteTax)
				})
				r.Route("/tax-classes", func(r chi.Router) {
					r.Get("/", h.ShopAdmin.TaxClasses)
					r.Post("/", h.ShopAdmin.CreateTaxClass)
					r.Delete("/{slug}", h.ShopAdmin.DeleteTaxClass)
				})
				r.Route("/shipping/zones", func(r chi.Router) {
					r.Get("/", h.ShopAdmin.Zones)
					r.Post("/", h.ShopAdmin.CreateZone)
					r.Put("/{id}", h.ShopAdmin.UpdateZone)
					r.Delete("/{id}", h.ShopAdmin.DeleteZone)
					r.Post("/{id}/methods/", h.ShopAdmin.AddMethod)
					r.Put("/{id}/methods/{instanceId}", h.ShopAdmin.UpdateMethod)
					r.Delete("/{id}/methods/{instanceId}", h.ShopAdmin.DeleteMethod)
				})
				r.Get("/payment-gateways/", h.ShopAdmin.Gateways)
				r.Put("/payment-gateways/{id}", h.ShopAdmin.UpdateGateway)
				r.Get("/emails/", h.ShopAdmin.Emails)
				r.Put("/emails/{id}", h.ShopAdmin.UpdateEmail)
				r.Get("/appearance/", h.ShopAdmin.Appearance)
				r.Put("/appearance/", h.ShopAdmin.UpdateAppearance)
			})

			// user administration, audit and IP bans: WordPress administrators only
			r.Group(func(r chi.Router) {
				r.Use(superuser)
				r.Route("/users", func(r chi.Router) {
					r.Get("/", h.Admin.ListUsers)
					r.Put("/{id}", h.Admin.UpdateUser)
					r.Delete("/{id}", h.Admin.DeleteUser)
					r.Post("/{id}/disable-mfa", h.Admin.DisableMFA)
					r.Get("/{id}/ips", h.IPBan.UserIPs)
				})
				r.Get("/audit", h.Admin.Audit)
				r.Route("/ip-bans", func(r chi.Router) {
					r.Get("/", h.IPBan.ListIPs)
					r.Get("/stats", h.IPBan.Stats)
					r.Put("/", h.IPBan.BanIP)
				})
			})
		})

		// --- AI assistant + CWCloud adapter ---
		r.Route("/ai", func(r chi.Router) {
			r.Use(auth, active)
			r.Get("/providers", h.AI.Providers)
			r.Get("/shareable-emails", h.AI.ShareableEmails)
			r.Route("/connections", func(r chi.Router) {
				r.Get("/", h.AI.List)
				r.Post("/", h.AI.Create)
				r.Post("/order", h.AI.Reorder)
				r.Put("/{id}", h.AI.Update)
				r.Delete("/{id}", h.AI.Delete)
				r.Get("/{id}/shares", h.AI.ListShares)
				r.Post("/{id}/shares", h.AI.Share)
				r.Delete("/{id}/shares/{userId}", h.AI.Unshare)
			})
			r.Post("/chat", h.AI.Chat)
			r.Post("/cwcloud/adapter", h.AI.CWCloudAdapter)
			r.Route("/conversations", func(r chi.Router) {
				r.Get("/", h.AI.ListConversations)
				r.Delete("/", h.AI.DeleteConversations)
				r.Get("/{id}", h.AI.GetConversation)
			})
		})
	})

	if c.MetricsHandler != nil {
		r.Get("/metrics", c.MetricsHandler.ServeHTTP)
		r.Get("/v1/metrics", c.MetricsHandler.ServeHTTP)
	}
	r.Get("/sitemap.xml", h.Shop.Sitemap)
	r.Get("/robots.txt", h.Shop.Robots)

	spec := openapi.Generate(r, "goocommerce API", c.Version, handlers.EndpointDocs())
	r.Get("/openapi.json", handlers.NewOpenAPIHandler(spec))
	r.Get("/", handlers.ServeSwaggerUI)
	return r
}
