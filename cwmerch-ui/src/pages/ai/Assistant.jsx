import { useEffect, useRef, useState } from "react";
import { toast } from "react-toastify";
import { FiChevronLeft, FiChevronRight, FiMessageSquare, FiPlus, FiSend, FiTrash2 } from "react-icons/fi";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";
import Markdown from "../../components/Markdown";
import { ConfirmModal, Empty, Spinner, Tabs } from "../../components/ui";
import { useAi } from "./useAi";
import Subscriptions from "./Subscriptions";

const HISTORY_KEY = "cwmerch.assistant.historyCollapsed";

// Assistant ports cwclock's AiAssistant: conversation rail, Markdown answers,
// subscription picker, history open/delete, and the subscriptions tab.
const Assistant = ({ embedded = false }) => {
  const { t, errMsg, formatDateTime } = useI18n();
  const ai = useAi();
  const { connections, conversations, loaded } = ai;
  const [tab, setTab] = useState("chat");
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState("");
  const [conversationId, setConversationId] = useState(null);
  const [connectionId, setConnectionId] = useState("");
  const [sending, setSending] = useState(false);
  const [confirm, setConfirm] = useState(null);
  const [ttl, setTtl] = useState(null);
  const [, setTick] = useState(0);
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem(HISTORY_KEY) === "1";
    } catch {
      return false;
    }
  });
  const scrollRef = useRef(null);
  useSeo({ title: t("ai.title"), noindex: true });

  // The selected subscription must still exist (deleted or recreated in the
  // subscriptions tab, or elsewhere): otherwise back to the default one.
  useEffect(() => {
    if (connections.length === 0) {
      if (connectionId) setConnectionId("");
      return;
    }
    if (!connections.some((c) => c.id === connectionId)) setConnectionId(connections[0].id);
  }, [connections, connectionId]);

  useEffect(() => {
    if (loaded && connections.length === 0) setTab("subscriptions");
  }, [loaded, connections.length]);

  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages, sending]);

  useEffect(() => {
    const id = setInterval(() => setTick((n) => n + 1), 30000);
    return () => clearInterval(id);
  }, []);

  const toggleHistory = () =>
    setCollapsed((c) => {
      try {
        localStorage.setItem(HISTORY_KEY, c ? "0" : "1");
      } catch {
        /* not remembered */
      }
      return !c;
    });

  const newChat = () => {
    setMessages([]);
    setConversationId(null);
  };

  const open = async (id) => {
    try {
      const conv = await ai.getConversation(id);
      if (conv.downloadTtlSeconds) setTtl(conv.downloadTtlSeconds);
      const loadedMsgs = [];
      (conv.exchanges || []).forEach((ex) => {
        loadedMsgs.push({ role: "user", content: ex.question });
        loadedMsgs.push({ role: "assistant", content: ex.answer, model: ex.model || ex.connection, createdAt: ex.createdAt });
      });
      setMessages(loadedMsgs);
      setConversationId(conv.id || id);
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  const send = async (e) => {
    e?.preventDefault?.();
    const text = input.trim();
    if (!text || sending) return;
    setInput("");
    setMessages((m) => [...m, { role: "user", content: text }]);
    setSending(true);
    try {
      let res;
      try {
        res = await ai.chat({ message: text, conversationId, connectionId });
      } catch (err) {
        // The subscription was deleted or recreated since the list was loaded
        // (another tab...): reload the list and ask the default one.
        if (!connectionId || err?.response?.status !== 404 || err?.response?.data?.i18n_code !== "errors.notFound") throw err;
        await ai.loadConnections();
        setConnectionId("");
        res = await ai.chat({ message: text, conversationId });
      }
      if (res.downloadTtlSeconds) setTtl(res.downloadTtlSeconds);
      setMessages((m) => [...m, { role: "assistant", content: res.message, model: res.model, createdAt: new Date().toISOString() }]);
      if (res.conversationId) setConversationId(res.conversationId);
      ai.loadConversations();
    } catch (err) {
      const message = errMsg(err);
      setMessages((m) => [...m, { role: "error", content: message }]);
    } finally {
      setSending(false);
    }
  };

  const runConfirm = async () => {
    const target = confirm;
    setConfirm(null);
    try {
      if (target.type === "all") {
        await ai.deleteConversations({ all: true });
        newChat();
      } else {
        await ai.deleteConversations({ ids: [target.id] });
        if (target.id === conversationId) newChat();
      }
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  return (
    <div className={`assistant-page ${embedded ? "" : "container"}`}>
      {!embedded && <h1>{t("ai.title")}</h1>}
      <Tabs
        tabs={[
          { id: "chat", label: t("ai.chat") },
          { id: "subscriptions", label: t("ai.subscriptions"), count: connections.length },
        ]}
        active={tab}
        onChange={setTab}
      />
      {tab === "subscriptions" ? (
        <Subscriptions ai={ai} />
      ) : !loaded ? (
        <Spinner />
      ) : connections.length === 0 ? (
        <Empty title={t("ai.noSubscriptionTitle")} body={t("ai.noSubscriptionBody")}>
          <button type="button" className="btn btn-primary" onClick={() => setTab("subscriptions")}>
            {t("ai.addSubscription")}
          </button>
        </Empty>
      ) : (
        <div className={`assistant ${collapsed ? "is-collapsed" : ""}`}>
          <aside className="assistant-rail" aria-label={t("ai.conversations")}>
            <div className="rail-head">
              <button type="button" className="icon-btn" onClick={toggleHistory} aria-label={collapsed ? t("ai.showHistory") : t("ai.hideHistory")} aria-expanded={!collapsed}>
                {collapsed ? <FiChevronRight className="flip-rtl" /> : <FiChevronLeft className="flip-rtl" />}
              </button>
              {!collapsed && <strong>{t("ai.conversations")}</strong>}
            </div>
            <button type="button" className="btn btn-secondary btn-sm btn-block" onClick={newChat} aria-label={t("ai.newChat")} title={t("ai.newChat")}>
              <FiPlus aria-hidden="true" /> {!collapsed && t("ai.newChat")}
            </button>
            {!collapsed && (
              <ul className="conversation-list">
                {conversations.length === 0 && <li className="muted small">{t("ai.noConversations")}</li>}
                {conversations.map((c) => (
                  <li key={c.id} className={c.id === conversationId ? "is-active" : ""}>
                    <button type="button" className="conversation-item" onClick={() => open(c.id)} title={c.title}>
                      <FiMessageSquare aria-hidden="true" /> <span>{c.title || t("ai.untitled")}</span>
                    </button>
                    <button type="button" className="icon-btn danger" onClick={() => setConfirm({ type: "one", id: c.id })} aria-label={t("ai.deleteConversation")}>
                      <FiTrash2 />
                    </button>
                  </li>
                ))}
              </ul>
            )}
            {!collapsed && conversations.length > 0 && (
              <button type="button" className="link-btn small danger" onClick={() => setConfirm({ type: "all" })}>
                {t("ai.clearHistory")}
              </button>
            )}
          </aside>
          <section className="assistant-chat">
            <div className="messages" ref={scrollRef} aria-live="polite">
              {messages.length === 0 && (
                <div className="welcome">
                  <h2>{t("ai.welcomeTitle")}</h2>
                  <p className="muted">{t("ai.welcomeBody")}</p>
                </div>
              )}
              {messages.map((m, i) => (
                <div key={i} className={`message message-${m.role}`}>
                  {m.role === "assistant" ? (
                    <>
                      <Markdown downloadExpiresAt={ttl && m.createdAt ? new Date(m.createdAt).getTime() + ttl * 1000 : null}>{m.content}</Markdown>
                      {(m.model || m.createdAt) && (
                        <p className="credit">
                          {m.model ? `${t("ai.generatedBy")} ${m.model}` : ""}
                          {m.model && m.createdAt ? " · " : ""}
                          {formatDateTime(m.createdAt)}
                        </p>
                      )}
                    </>
                  ) : (
                    <span className="pre-line">{m.content}</span>
                  )}
                </div>
              ))}
              {sending && (
                <div className="message message-assistant">
                  <span className="typing" aria-label={t("ai.thinking")}>
                    <span />
                    <span />
                    <span />
                  </span>
                </div>
              )}
            </div>
            <form className="composer" onSubmit={send}>
              <label htmlFor="ai-input" className="sr-only">
                {t("ai.askPlaceholder")}
              </label>
              <textarea
                id="ai-input"
                className="input"
                rows={2}
                value={input}
                placeholder={t("ai.askPlaceholder")}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && !e.shiftKey) {
                    e.preventDefault();
                    send();
                  }
                }}
              />
              {connections.length > 1 && (
                <>
                  <label htmlFor="ai-connection" className="sr-only">
                    {t("ai.model")}
                  </label>
                  <select id="ai-connection" className="input input-sm" value={connectionId} onChange={(e) => setConnectionId(e.target.value)}>
                    {connections.map((c) => (
                      <option key={c.id} value={c.id}>
                        {c.name || c.provider}
                      </option>
                    ))}
                  </select>
                </>
              )}
              <button type="submit" className="btn btn-primary" disabled={sending || !input.trim()} aria-label={t("ai.send")}>
                <FiSend aria-hidden="true" className="flip-rtl" />
              </button>
            </form>
          </section>
        </div>
      )}
      <ConfirmModal
        show={!!confirm}
        title={confirm?.type === "all" ? t("ai.clearHistoryTitle") : t("ai.deleteConversationTitle")}
        body={confirm?.type === "all" ? t("ai.clearHistoryBody") : t("ai.deleteConversationBody")}
        confirmLabel={t("common.delete")}
        onConfirm={runConfirm}
        onCancel={() => setConfirm(null)}
      />
    </div>
  );
};

export default Assistant;
