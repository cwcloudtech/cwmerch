package woo

import "context"

// ShopConfig switches the public shop: cwmerch can run as an internal
// information system only (back office, accounting, AI) with no storefront.
type ShopConfig struct {
	ShopEnabled         bool   `json:"shopEnabled"`
	ClosedMessage       string `json:"closedMessage"`
	RegistrationEnabled bool   `json:"registrationEnabled"`
}

// ShopConfig reads the switch (open by default).
func (s *Store) ShopConfig(ctx context.Context) ShopConfig {
	c := ShopConfig{ShopEnabled: true, RegistrationEnabled: true}
	_, _, _ = s.Setting(ctx, "shop", &c)
	return c
}

// SaveShopConfig writes the switch.
func (s *Store) SaveShopConfig(ctx context.Context, c ShopConfig, by uint64) (ShopConfig, error) {
	if err := s.SaveSetting(ctx, "shop", c, by); err != nil {
		return ShopConfig{}, err
	}
	return s.ShopConfig(ctx), nil
}
