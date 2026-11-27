import { useShop } from "../context/ShopContext";

// AuthLogo heads the sign-in / register / password pages: the shop's own logo
// when it has one, else the cwmerch logo (mascot + wordmark).
const AuthLogo = () => {
  const { appearance, shopName } = useShop();
  const logo = appearance?.logo?.src;
  return (
    <div className="auth-logo">
      {logo ? <img src={logo} alt={shopName} /> : <img src="/cwmerch-logo-small.png" alt="cwmerch" width="1229" height="590" />}
    </div>
  );
};

export default AuthLogo;
