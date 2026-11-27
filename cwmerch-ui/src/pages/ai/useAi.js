import { useCallback, useEffect, useState } from "react";
import { api, asArray } from "../../api/client";

const BASE = "/v1/ai";

// searchShareable powers the share picker's autocomplete: [{email, name, surname}].
export const searchShareable = async (q) => {
  try {
    return asArray((await api.get(`${BASE}/shareable-emails`, { params: { q } })).data);
  } catch {
    return [];
  }
};

// useAi ports cwclock's Redux AI slice to a hook: providers, connections
// (subscriptions; the first is the default), conversations, and the calls.
export const useAi = () => {
  const [providers, setProviders] = useState([]);
  const [connections, setConnections] = useState([]);
  const [conversations, setConversations] = useState([]);
  const [loaded, setLoaded] = useState(false);

  const loadConnections = useCallback(async () => {
    try {
      const { data } = await api.get(`${BASE}/connections/`);
      setConnections(asArray(data));
    } catch {
      setConnections([]);
    } finally {
      setLoaded(true);
    }
  }, []);

  const loadConversations = useCallback(async () => {
    try {
      const { data } = await api.get(`${BASE}/conversations/`);
      setConversations(asArray(data, "conversations"));
    } catch {
      setConversations([]);
    }
  }, []);

  const loadProviders = useCallback(async () => {
    try {
      const { data } = await api.get(`${BASE}/providers`);
      setProviders(asArray(data));
    } catch {
      setProviders([]);
    }
  }, []);

  useEffect(() => {
    loadConnections();
    loadConversations();
    loadProviders();
  }, [loadConnections, loadConversations, loadProviders]);

  const createConnection = async (fields) => {
    await api.post(`${BASE}/connections/`, fields);
    await loadConnections();
  };
  const updateConnection = async (id, fields) => {
    await api.put(`${BASE}/connections/${id}`, fields);
    await loadConnections();
  };
  const deleteConnection = async (id) => {
    await api.delete(`${BASE}/connections/${id}`);
    await loadConnections();
  };
  // reorder: the first id becomes the default subscription.
  const reorderConnections = async (ids) => {
    const { data } = await api.post(`${BASE}/connections/order`, { ids, connectionIds: ids });
    if (Array.isArray(data)) setConnections(data);
    else await loadConnections();
  };

  const listShares = async (id) => asArray((await api.get(`${BASE}/connections/${id}/shares`)).data);
  const share = async (id, email) => asArray((await api.post(`${BASE}/connections/${id}/shares`, { email, role: "user" })).data);
  const unshare = (id, userId) => api.delete(`${BASE}/connections/${id}/shares/${userId}`);

  const getConversation = async (id) => (await api.get(`${BASE}/conversations/${id}`)).data;
  const deleteConversations = async ({ ids, all }) => {
    await api.delete(`${BASE}/conversations/`, { data: all ? { all: true } : { ids } });
    await loadConversations();
  };
  const chat = async ({ message, conversationId, connectionId }) =>
    (
      await api.post(`${BASE}/chat`, {
        message,
        ...(conversationId ? { conversationId } : {}),
        ...(connectionId ? { connectionId } : {}),
      })
    ).data;

  return {
    providers,
    connections,
    conversations,
    loaded,
    loadConnections,
    loadConversations,
    createConnection,
    updateConnection,
    deleteConnection,
    reorderConnections,
    listShares,
    share,
    unshare,
    searchShareable,
    getConversation,
    deleteConversations,
    chat,
  };
};
