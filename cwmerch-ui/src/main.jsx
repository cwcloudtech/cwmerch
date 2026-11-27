import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import "react-toastify/dist/ReactToastify.css";
import "./styles/global.css";
import App from "./App";
import { I18nProvider } from "./i18n/I18nContext";
import { AuthProvider } from "./context/AuthContext";
import { ShopProvider } from "./context/ShopContext";
import { CartProvider } from "./context/CartContext";

createRoot(document.getElementById("root")).render(
  <StrictMode>
    <BrowserRouter>
      <I18nProvider>
        <AuthProvider>
          <ShopProvider>
            <CartProvider>
              <App />
            </CartProvider>
          </ShopProvider>
        </AuthProvider>
      </I18nProvider>
    </BrowserRouter>
  </StrictMode>
);
