import { useEffect, useState } from "react";

// The UI's build manifest (/manifest.json: {version, sha, details}, written by
// the delivery pipeline), fetched once per page load.
let manifestPromise = null;

const loadManifest = () => {
  if (!manifestPromise) {
    manifestPromise = fetch("/manifest.json", { cache: "no-cache" })
      .then((r) => (r.ok ? r.json() : null))
      .catch(() => null);
  }
  return manifestPromise;
};

// useVersion is the UI build's version, else the fallback (the API's).
export const useVersion = (fallback = "") => {
  const [version, setVersion] = useState("");
  useEffect(() => {
    let alive = true;
    loadManifest().then((m) => {
      if (alive && m && typeof m.version === "string") setVersion(m.version.trim());
    });
    return () => {
      alive = false;
    };
  }, []);
  return version || fallback || "";
};
