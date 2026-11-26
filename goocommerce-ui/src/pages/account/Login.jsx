import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { toast } from "react-toastify";
import { FcGoogle } from "react-icons/fc";
import { api, apiUrl, asArray } from "../../api/client";
import { useAuth } from "../../context/AuthContext";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { useAsync } from "../../lib/useAsync";
import { useSeo } from "../../lib/seo";
import { ErrorBox, TextField } from "../../components/ui";
import MfaChallenge from "./MfaChallenge";

export const OidcButtons = ({ redirectAfter }) => {
  const { t } = useI18n();
  const { data: providers } = useAsync(async () => asArray((await api.get("/v1/oidc/")).data), [], []);
  if (!providers.some((p) => p.name === "google")) return null;
  const start = () => {
    try {
      if (redirectAfter) sessionStorage.setItem("goocommerce.oidcRedirect", redirectAfter);
    } catch {
      /* ignore */
    }
    const redirect = `${window.location.origin}/oidc/callback`;
    window.location.assign(`${apiUrl()}/v1/oidc/google/login?redirect=${encodeURIComponent(redirect)}`);
  };
  return (
    <>
      <div className="divider">
        <span>{t("auth.or")}</span>
      </div>
      <button type="button" className="btn btn-secondary btn-block oidc-btn" onClick={start}>
        <FcGoogle aria-hidden="true" /> {t("auth.signInWithGoogle")}
      </button>
    </>
  );
};

const Login = () => {
  const { login, setSession, isAdmin } = useAuth();
  const { shopName } = useShop();
  const { t, errMsg } = useI18n();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const redirect = params.get("redirect") || "";
  const [loginForm, setLoginForm] = useState({ email: "", password: "" });
  const [reg, setReg] = useState({ name: "", surname: "", email: "", password: "" });
  const [challenge, setChallenge] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [regError, setRegError] = useState(null);
  useSeo({ title: `${t("auth.signIn")} – ${shopName}`, noindex: true });

  const afterLogin = (u) => {
    toast.success(t("auth.welcome", { name: u?.displayName || u?.name || "" }));
    if (redirect.startsWith("/")) navigate(redirect, { replace: true });
    else if (u && (u.role === "superuser" || u.role === "manager") && !isAdmin) navigate("/my-account", { replace: true });
  };

  const submitLogin = async (e) => {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const data = await login(loginForm.email.trim(), loginForm.password);
      if (data?.mfaRequired) setChallenge(data);
      else afterLogin(data);
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const submitRegister = async (e) => {
    e.preventDefault();
    setRegError(null);
    setBusy(true);
    try {
      const { data } = await api.post("/v1/users/", { ...reg, email: reg.email.trim() });
      if (data?.token) {
        setSession(data);
        afterLogin(data);
      } else {
        toast.success(t("auth.registered"));
      }
    } catch (err) {
      setRegError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  if (challenge) {
    return (
      <div className="narrow">
        <MfaChallenge challenge={challenge} onDone={afterLogin} onCancel={() => setChallenge(null)} />
      </div>
    );
  }

  return (
    <div className="auth-grid">
      <section className="card stack auth-card">
        <h2>{t("auth.signIn")}</h2>
        <form className="stack" onSubmit={submitLogin}>
          <TextField label={t("auth.emailOrUsername")} value={loginForm.email} onChange={(email) => setLoginForm({ ...loginForm, email })} required autoComplete="username" />
          <TextField label={t("auth.password")} type="password" value={loginForm.password} onChange={(password) => setLoginForm({ ...loginForm, password })} required autoComplete="current-password" />
          <ErrorBox error={error} />
          <button type="submit" className="btn btn-primary btn-block" disabled={busy}>
            {t("auth.signIn")}
          </button>
        </form>
        <Link to="/forgot-password" className="small">
          {t("auth.lostPassword")}
        </Link>
        <OidcButtons redirectAfter={redirect} />
      </section>
      <section className="card stack auth-card">
        <h2>{t("auth.register")}</h2>
        <form className="stack" onSubmit={submitRegister}>
          <div className="form-grid">
            <TextField label={t("auth.firstName")} value={reg.name} onChange={(name) => setReg({ ...reg, name })} required autoComplete="given-name" />
            <TextField label={t("auth.lastName")} value={reg.surname} onChange={(surname) => setReg({ ...reg, surname })} required autoComplete="family-name" />
          </div>
          <TextField label={t("auth.email")} type="email" value={reg.email} onChange={(email) => setReg({ ...reg, email })} required autoComplete="email" />
          <TextField label={t("auth.password")} type="password" value={reg.password} onChange={(password) => setReg({ ...reg, password })} required autoComplete="new-password" />
          <p className="small muted">{t("auth.privacyNotice")}</p>
          <ErrorBox error={regError} />
          <button type="submit" className="btn btn-secondary btn-block" disabled={busy}>
            {t("auth.register")}
          </button>
        </form>
      </section>
    </div>
  );
};

export default Login;
