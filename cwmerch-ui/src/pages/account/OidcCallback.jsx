import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "react-toastify";
import { api } from "../../api/client";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";
import { Spinner } from "../../components/ui";
import MfaChallenge from "./MfaChallenge";

// OidcCallback is where an OIDC (Google) login lands. The API redirects here
// with #token=... (or ?token=), ?error=, or ?mfaChallenge=&hasTotp=&hasWebAuthn=;
// a provider redirecting here directly with ?code=&state= is completed through
// GET /v1/oidc/callback. Ported from cwclock.
const readParams = () => {
  const merged = new URLSearchParams(window.location.search);
  const hash = window.location.hash.replace(/^#/, "");
  new URLSearchParams(hash).forEach((v, k) => merged.set(k, v));
  return merged;
};

const OidcCallback = () => {
  const { loginWithToken } = useAuth();
  const { t, errMsg } = useI18n();
  const navigate = useNavigate();
  const [challenge, setChallenge] = useState(null);
  useSeo({ title: t("auth.signIn"), noindex: true });

  const target = () => {
    try {
      const r = sessionStorage.getItem("cwmerch.oidcRedirect");
      sessionStorage.removeItem("cwmerch.oidcRedirect");
      if (r && r.startsWith("/")) return r;
    } catch {
      /* ignore */
    }
    return "/my-account";
  };

  useEffect(() => {
    const p = readParams();
    // Never leave the token in the address bar / history.
    window.history.replaceState(null, "", window.location.pathname);
    const fail = (err) => {
      toast.error(err ? errMsg(err) : t("errors.oidcFailed"));
      navigate("/my-account", { replace: true });
    };
    const finish = (token) =>
      loginWithToken(token)
        .then(() => navigate(target(), { replace: true }))
        .catch(fail);

    if (p.get("error") === "oidc_registration_disabled") {
      toast.error(t("errors.registrationDisabled"));
      navigate("/my-account", { replace: true });
      return;
    }
    if (p.get("error")) return fail();
    if (p.get("mfaChallenge")) {
      setChallenge({ challengeToken: p.get("mfaChallenge"), hasTotp: p.get("hasTotp") === "1", hasWebAuthn: p.get("hasWebAuthn") === "1" });
      return;
    }
    if (p.get("token")) {
      finish(p.get("token"));
      return;
    }
    if (p.get("code") && p.get("state")) {
      api
        .get("/v1/oidc/callback", { params: { code: p.get("code"), state: p.get("state") } })
        .then(({ data }) => {
          if (data?.mfaRequired) setChallenge(data);
          else if (data?.token) finish(data.token);
          else fail();
        })
        .catch(fail);
      return;
    }
    navigate("/my-account", { replace: true });
    // Runs once: the URL is only meaningful on the initial load.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (challenge) {
    return (
      <div className="container narrow">
        <h1 className="sr-only">{t("auth.mfaTitle")}</h1>
        <MfaChallenge challenge={challenge} onDone={() => navigate(target(), { replace: true })} onCancel={() => navigate("/my-account", { replace: true })} />
      </div>
    );
  }
  return (
    <div className="container">
      <h1 className="sr-only">{t("auth.signIn")}</h1>
      <Spinner />
    </div>
  );
};

export default OidcCallback;
