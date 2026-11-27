import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { ToastContainer } from "react-toastify";
import ShopLayout from "./layouts/ShopLayout";
import { Spinner } from "./components/ui";
import { useAuth } from "./context/AuthContext";
import { useI18n } from "./i18n/I18nContext";
import Home from "./pages/shop/Home";
import StorefrontRoute from "./pages/shop/StorefrontRoute";
import Cart from "./pages/shop/Cart";
import Checkout from "./pages/shop/Checkout";
import OrderReceived from "./pages/shop/OrderReceived";
import Payment from "./pages/shop/Payment";
import Contact from "./pages/shop/Contact";
import ForgotPassword from "./pages/shop/ForgotPassword";
import ResetPassword from "./pages/shop/ResetPassword";
import AccountLayout from "./pages/account/AccountLayout";
import AccountDashboard from "./pages/account/Dashboard";
import { Downloads, Orders as MyOrders, ViewOrder } from "./pages/account/Orders";
import Addresses from "./pages/account/Addresses";
import Details from "./pages/account/Details";
import Security from "./pages/account/Security";
import ApiKeys from "./pages/account/ApiKeys";
import OidcCallback from "./pages/account/OidcCallback";

// The assistant and the whole admin are split into their own chunks.
const Assistant = lazy(() => import("./pages/ai/Assistant"));
const Admin = lazy(() => import("./pages/admin/AdminRoutes"));

const RequireUser = ({ children }) => {
  const { user } = useAuth();
  return user ? children : <Navigate to="/my-account?redirect=/assistant" replace />;
};

const App = () => {
  const { dir } = useI18n();
  return (
    <>
      <Suspense fallback={<Spinner />}>
        <Routes>
          <Route path="/admin/*" element={<Admin />} />
          <Route element={<ShopLayout />}>
            <Route index element={<Home />} />
            <Route path="cart" element={<Cart />} />
            <Route path="checkout" element={<Checkout />} />
            <Route path="checkout/order-received/:id" element={<OrderReceived />} />
            <Route path="checkout/payment/:orderId" element={<Payment />} />
            <Route path="contact" element={<Contact />} />
            <Route path="forgot-password" element={<ForgotPassword />} />
            <Route path="reset-password" element={<ResetPassword />} />
            <Route path="oidc/callback" element={<OidcCallback />} />
            <Route
              path="assistant"
              element={
                <RequireUser>
                  <Assistant />
                </RequireUser>
              }
            />
            <Route path="my-account" element={<AccountLayout />}>
              <Route index element={<AccountDashboard />} />
              <Route path="orders" element={<MyOrders />} />
              <Route path="view-order/:id" element={<ViewOrder />} />
              <Route path="downloads" element={<Downloads />} />
              <Route path="edit-address" element={<Addresses />} />
              <Route path="edit-account" element={<Details />} />
              <Route path="security" element={<Security />} />
              <Route path="api-keys" element={<ApiKeys />} />
              <Route path="assistant" element={<Assistant embedded />} />
            </Route>
            {/* Shop, product, category and tag pages follow WooCommerce's permalinks. */}
            <Route path="*" element={<StorefrontRoute />} />
          </Route>
        </Routes>
      </Suspense>
      <ToastContainer position={dir === "rtl" ? "bottom-left" : "bottom-right"} rtl={dir === "rtl"} autoClose={4000} newestOnTop theme="colored" />
    </>
  );
};

export default App;
