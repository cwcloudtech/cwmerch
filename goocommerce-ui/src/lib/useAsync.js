import { useCallback, useEffect, useRef, useState } from "react";

// useAsync runs `fn` when `deps` change and exposes {data, loading, error, reload, setData}.
export const useAsync = (fn, deps, initial = null) => {
  const [data, setData] = useState(initial);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const seq = useRef(0);

  const reload = useCallback(async () => {
    const id = ++seq.current;
    setLoading(true);
    setError(null);
    try {
      const result = await fnRef.current();
      if (id === seq.current) setData(result);
      return result;
    } catch (e) {
      if (id === seq.current) setError(e);
      return undefined;
    } finally {
      if (id === seq.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { data, loading, error, reload, setData };
};
