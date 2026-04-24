// Blocks page — manage actor blocks and domain blocks with tabs.
// ブロックページ — アクターブロックとドメインブロックの管理 (タブ切り替え)。

import { useState, useEffect, useCallback } from "preact/hooks";
import { call, redirectIfUnauthorized } from "../lib/api";
import { load as loadI18n, t } from "../lib/i18n";
import { Icon } from "../components/icons";

type Block = {
  id: string;
  actor_uri: string;
  created_at: string;
};

type DomainBlock = {
  id: string;
  domain: string;
  created_at: string;
};

export function BlocksPage({ path }: { path?: string }) {
  const [tab, setTab] = useState<"actors" | "domains">("actors");
  const [blocks, setBlocks] = useState<Block[]>([]);
  const [domainBlocks, setDomainBlocks] = useState<DomainBlock[]>([]);
  const [actorInput, setActorInput] = useState("");
  const [domainInput, setDomainInput] = useState("");
  const [error, setError] = useState("");
  const [ready, setReady] = useState(false);

  const loadData = useCallback(async () => {
    await loadI18n();
    const [bRes, dbRes] = await Promise.all([
      call<Block[]>("blocks.list"),
      call<DomainBlock[]>("domain_blocks.list"),
    ]);
    if (redirectIfUnauthorized(bRes.error)) return;
    if (bRes.result) setBlocks(bRes.result);
    if (dbRes.result) setDomainBlocks(dbRes.result);
    setReady(true);
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  if (!ready) return null;

  const handleBlockActor = async (e: Event) => {
    e.preventDefault();
    if (!actorInput.trim()) return;
    setError("");
    const { error: err } = await call("blocks.create", { actor_uri: actorInput.trim() });
    if (err) { setError(err.message); return; }
    setActorInput("");
    loadData();
  };

  const handleUnblockActor = async (actorURI: string) => {
    setError("");
    const { error: err } = await call("blocks.delete", { actor_uri: actorURI });
    if (err) { setError(err.message); return; }
    loadData();
  };

  const handleBlockDomain = async (e: Event) => {
    e.preventDefault();
    if (!domainInput.trim()) return;
    setError("");
    const { error: err } = await call("domain_blocks.create", { domain: domainInput.trim() });
    if (err) { setError(err.message); return; }
    setDomainInput("");
    loadData();
  };

  const handleUnblockDomain = async (domain: string) => {
    setError("");
    const { error: err } = await call("domain_blocks.delete", { domain });
    if (err) { setError(err.message); return; }
    loadData();
  };

  return (
    <div class="screen">
      {/* Tabs / タブ */}
      <div class="tabs">
        <a href="#" class={`tab${tab === "actors" ? " active" : ""}`} onClick={(e) => { e.preventDefault(); setTab("actors"); }}>
          {t("my.blocks.actors") || "Actors"} ({blocks.length})
        </a>
        <a href="#" class={`tab${tab === "domains" ? " active" : ""}`} onClick={(e) => { e.preventDefault(); setTab("domains"); }}>
          {t("my.blocks.domains") || "Domains"} ({domainBlocks.length})
        </a>
      </div>

      {error && (
        <div class="card">
          <p class="meta" style={{ color: "var(--danger)" }}>{error}</p>
        </div>
      )}

      {/* Actor blocks / アクターブロック */}
      {tab === "actors" && (
        <>
          <div class="card">
            <form onSubmit={handleBlockActor}>
              <div class="input-group">
                <label>{t("my.blocks.actor_label")}</label>
                <input
                  type="text"
                  value={actorInput}
                  onInput={(e) => setActorInput((e.target as HTMLInputElement).value)}
                  placeholder="user@example.com or https://..."
                />
              </div>
              <button class="btn btn-primary btn-sm" type="submit" disabled={!actorInput.trim()}>
                {t("my.blocks.block")}
              </button>
            </form>
          </div>

          {blocks.map((b) => (
            <div class="card user-card" key={b.id}>
              <div class="user-card-main">
                <div class="post-avatar" style={{ width: 40, height: 40, borderRadius: "50%", background: "var(--secondary)" }} />
                <div class="user-card-info">
                  <span class="post-author-name">{b.actor_uri.split("/").pop()}</span>
                  <span class="handle">{b.actor_uri}</span>
                </div>
              </div>
              <div class="user-card-actions">
                <button class="btn btn-outline btn-sm" onClick={() => handleUnblockActor(b.actor_uri)}>
                  {t("my.blocks.unblock")}
                </button>
              </div>
            </div>
          ))}
          {blocks.length === 0 && (
            <div class="card">
              <p class="meta" style={{ textAlign: "center", padding: 16 }}>{t("my.blocks.actors_empty")}</p>
            </div>
          )}
        </>
      )}

      {/* Domain blocks / ドメインブロック */}
      {tab === "domains" && (
        <>
          <div class="card">
            <form onSubmit={handleBlockDomain}>
              <div class="input-group">
                <label>{t("my.blocks.domain_label")}</label>
                <input
                  type="text"
                  value={domainInput}
                  onInput={(e) => setDomainInput((e.target as HTMLInputElement).value)}
                  placeholder="spam.example.com"
                />
              </div>
              <button class="btn btn-primary btn-sm" type="submit" disabled={!domainInput.trim()}>
                {t("my.blocks.block")}
              </button>
            </form>
          </div>

          {domainBlocks.map((b) => (
            <div class="card user-card" key={b.id}>
              <div class="user-card-main">
                <div class="domain-icon">
                  <Icon name="share" size={16} />
                </div>
                <div class="user-card-info">
                  <span class="post-author-name">{b.domain}</span>
                </div>
              </div>
              <div class="user-card-actions">
                <button class="btn btn-outline btn-sm" onClick={() => handleUnblockDomain(b.domain)}>
                  {t("my.blocks.unblock")}
                </button>
              </div>
            </div>
          ))}
          {domainBlocks.length === 0 && (
            <div class="card">
              <p class="meta" style={{ textAlign: "center", padding: 16 }}>{t("my.blocks.domains_empty")}</p>
            </div>
          )}
        </>
      )}
    </div>
  );
}
