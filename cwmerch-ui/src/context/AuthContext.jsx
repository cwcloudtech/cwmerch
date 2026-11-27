import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { api, TOKEN_KEY, UNAUTHORIZED_EVENT } from "../api/client";
import { isAdminRole } from "../lib/util";

const USER_KEY = "cwmerch.user";

const readUser = () => {
  try {
    const raw = localStorage.getItem(USER_KEY);
    const token = localStorage.getItem(TOKEN_KEY);
    if (!raw || !token) return null;
    return { ...JSON.parse(raw), token };
  } catch {
    return null;
  }
};

const AuthContext = createContext(null);

export const AuthProvider = ({ children }) => {
  const [user, setUser] = useState(readUser);

  const setSession = useCallback((userResponse) => {
    if (!userResponse || !userResponse.token) return;
    try {
      localStorage.setItem(TOKEN_KEY, userResponse.token);
      const { token: _token, ...rest } = userResponse;
      localStorage.setItem(USER_KEY, JSON.stringify(rest));
    } catch {
      /* storage unavailable: session lives in memory only */
    }
    setUser(userResponse);
  }, []);

  const clearSession = useCallback(() => {
    try {
      localStorage.removeItem(TOKEN_KEY);
      localStorage.removeItem(USER_KEY);
    } catch {
      /* ignore */
    }
    setUser(null);
  }, []);

  useEffect(() => {
    const onUnauthorized = () => clearSession();
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
  }, [clearSession]);

  const refreshMe = useCallback(async () => {
    const { data } = await api.get("/v1/users/me");
    setUser((prev) => {
      if (!prev) return prev;
      const next = { ...prev, ...data, token: prev.token };
      try {
        const { token: _token, ...rest } = next;
        localStorage.setItem(USER_KEY, JSON.stringify(rest));
      } catch {
        /* ignore */
      }
      return next;
    });
    return data;
  }, []);

  // Refresh the profile once on load (a stale/revoked token 401s and clears).
  useEffect(() => {
    if (user?.token) refreshMe().catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // loginWithToken completes an OIDC / checkout-created session from a bare JWT.
  const loginWithToken = useCallback(
    async (token) => {
      const { data } = await api.get("/v1/users/me", { headers: { Authorization: `Bearer ${token}` } });
      setSession({ ...data, token });
      return data;
    },
    [setSession]
  );

  const login = useCallback(
    async (email, password) => {
      const { data } = await api.post("/v1/users/login", { email, password });
      if (data?.mfaRequired) return data;
      setSession(data);
      return data;
    },
    [setSession]
  );

  const logout = useCallback(async () => {
    try {
      await api.post("/v1/users/logout", {});
    } catch {
      /* audit only */
    }
    clearSession();
  }, [clearSession]);

  const value = useMemo(
    () => ({
      user,
      isAdmin: !!user && isAdminRole(user.role),
      isSuperuser: user?.role === "superuser",
      setSession,
      clearSession,
      login,
      loginWithToken,
      logout,
      refreshMe,
    }),
    [user, setSession, clearSession, login, loginWithToken, logout, refreshMe]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export const useAuth = () => useContext(AuthContext);
