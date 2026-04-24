// Notifications page — notification list with polling and unread indicator.
// 通知ページ — ポーリング + 未読インジケーター付き通知一覧。

import { useState, useEffect, useCallback, useRef } from "preact/hooks";
import { call, redirectIfUnauthorized } from "../lib/api";
import { load as loadI18n, t } from "../lib/i18n";
import { formatTime } from "../lib/format";
import { Icon } from "../components/icons";

type Notification = {
  id: string;
  persona_id: string;
  type: string;
  actor_uri: string;
  post_id: string;
  read: boolean;
  created_at: string;
  // Enriched fields from API. / API から付与されるフィールド。
  actor_display_name?: string;
  actor_acct?: string;
  actor_avatar_url?: string;
  post_content?: string;
};

const typeIcon: Record<string, string> = {
  follow: "heart",
  mention: "reply",
  reblog: "reblog",
  favourite: "heart",
};

const POLL_INTERVAL = 30_000; // 30 seconds / 30 秒

export function NotificationsPage({ path }: { path?: string }) {
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [ready, setReady] = useState(false);
  const latestIdRef = useRef<string>("");

  const load = useCallback(async () => {
    await loadI18n();
    const { result, error } = await call<Notification[]>("notifications.list");
    if (redirectIfUnauthorized(error)) return;
    if (result) {
      setNotifications(result);
      if (result.length > 0) latestIdRef.current = result[0].id;
    }
    setReady(true);
  }, []);

  // Poll for new notifications. / 新着通知をポーリング。
  useEffect(() => {
    load();

    const poll = async () => {
      if (document.hidden) return;
      const since = latestIdRef.current;
      if (!since) return;
      const { result } = await call<Notification[]>("notifications.poll", { since });
      if (result && result.length > 0) {
        setNotifications((prev) => {
          const existingIds = new Set(prev.map((n) => n.id));
          const newItems = result.filter((n) => !existingIds.has(n.id));
          if (newItems.length === 0) return prev;
          const merged = [...newItems, ...prev];
          latestIdRef.current = merged[0].id;
          return merged;
        });
      }
    };

    const timer = setInterval(poll, POLL_INTERVAL);

    // Pause polling when tab is hidden. / タブ非アクティブ時にポーリングを一時停止。
    const handleVisibility = () => {
      if (!document.hidden) poll();
    };
    document.addEventListener("visibilitychange", handleVisibility);

    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", handleVisibility);
    };
  }, [load]);

  if (!ready) return null;

  const handleMarkRead = async (id: string) => {
    const { error } = await call("notifications.read", { id });
    if (!error) {
      setNotifications((prev) =>
        prev.map((n) => (n.id === id ? { ...n, read: true } : n))
      );
    }
  };

  const unreadCount = notifications.filter((n) => !n.read).length;

  return (
    <div class="screen">
      {/* Tabs / タブ */}
      <div class="tabs">
        <a href="/my/" class="tab">{t("my.home") || "Timeline"}</a>
        <a href="/my/notifications" class="tab active">
          {t("my.notifications") || "Notifications"}
          {unreadCount > 0 && <span class="tab-badge" />}
        </a>
      </div>

      {notifications.map((n) => (
        <div
          class={`card notif${n.read ? "" : " notif-unread"}`}
          key={n.id}
          onClick={() => !n.read && handleMarkRead(n.id)}
          style={{ cursor: n.read ? "default" : "pointer" }}
        >
          <div class="notif-header">
            <div class="notif-icon">
              <Icon name={typeIcon[n.type] || "heart"} size={16} />
            </div>
            <a href={`/users/${n.actor_acct || ""}`}>
              {n.actor_avatar_url ? (
                <img class="post-avatar" src={n.actor_avatar_url} alt="" width="36" height="36" />
              ) : (
                <div class="post-avatar" style={{ width: 36, height: 36, borderRadius: "50%", background: "var(--secondary)", flexShrink: 0 }} />
              )}
            </a>
            <div class="notif-body">
              <div class="notif-actor">
                <a href={`/users/${n.actor_acct || ""}`} class="post-author-name"><strong>{n.actor_display_name || n.actor_uri.split("/").pop()}</strong></a>{" "}
                {t(`my.notif.${n.type}`) || n.type}
              </div>
              <div class="handle">{n.actor_acct || n.actor_uri}</div>
            </div>
            <span class="meta">{formatTime(n.created_at)}</span>
          </div>
          {n.post_content && (
            <div class="notif-preview">
              <p>{n.post_content}</p>
            </div>
          )}
        </div>
      ))}

      {notifications.length === 0 && (
        <div class="card">
          <p class="meta" style={{ textAlign: "center", padding: 16 }}>
            {t("my.notif.empty")}
          </p>
        </div>
      )}
    </div>
  );
}
