import { Route, Routes } from "react-router-dom";
import AdminLayout from "../../layouts/AdminLayout";
import Assistant from "../ai/Assistant";
import Dashboard from "./Dashboard";
import Orders from "./Orders";
import OrderDetail from "./OrderDetail";
import Products from "./Products";
import ProductEditor from "./ProductEditor";
import { Attributes, AttributeTerms, Categories, Tags } from "./Taxonomies";
import { CouponEditor, Coupons } from "./Coupons";
import { CustomerDetail, Customers } from "./Customers";
import Reviews from "./Reviews";
import Reports from "./Reports";
import Accounting from "./Accounting";
import Appearance from "./Appearance";
import Settings from "./settings/Settings";
import { Audit, IpBans, SuperuserOnly, Users } from "./Superuser";
import NotFound from "../shop/NotFound";

const AdminRoutes = () => (
  <Routes>
    <Route element={<AdminLayout />}>
      <Route index element={<Dashboard />} />
      <Route path="orders" element={<Orders />} />
      <Route path="orders/:id" element={<OrderDetail />} />
      <Route path="products" element={<Products />} />
      <Route path="products/new" element={<ProductEditor />} />
      <Route path="products/:id" element={<ProductEditor />} />
      <Route path="categories" element={<Categories />} />
      <Route path="tags" element={<Tags />} />
      <Route path="attributes" element={<Attributes />} />
      <Route path="attributes/:id/terms" element={<AttributeTerms />} />
      <Route path="reviews" element={<Reviews />} />
      <Route path="coupons" element={<Coupons />} />
      <Route path="coupons/new" element={<CouponEditor />} />
      <Route path="coupons/:id" element={<CouponEditor />} />
      <Route path="customers" element={<Customers />} />
      <Route path="customers/:id" element={<CustomerDetail />} />
      <Route path="reports" element={<Reports />} />
      <Route path="accounting" element={<Accounting />} />
      <Route path="appearance" element={<Appearance />} />
      <Route path="settings" element={<Settings />} />
      <Route path="settings/:tab" element={<Settings />} />
      <Route path="assistant" element={<Assistant />} />
      <Route
        path="users"
        element={
          <SuperuserOnly>
            <Users />
          </SuperuserOnly>
        }
      />
      <Route
        path="audit"
        element={
          <SuperuserOnly>
            <Audit />
          </SuperuserOnly>
        }
      />
      <Route
        path="ip-bans"
        element={
          <SuperuserOnly>
            <IpBans />
          </SuperuserOnly>
        }
      />
      <Route path="*" element={<NotFound />} />
    </Route>
  </Routes>
);

export default AdminRoutes;
