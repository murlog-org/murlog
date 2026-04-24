import { useState, useEffect } from "preact/hooks";
import { route } from "preact-router";
import { call } from "../lib/api";
import { load as loadI18n, t } from "../lib/i18n";
import { Logo } from "../components/logo";

export function Login({ path }: { path?: string }) {
  const [password, setPassword] = useState("");
  const [totpCode, setTotpCode] = useState("");
  const [totpEnabled, setTotpEnabled] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    loadI18n().then(async () => {
      // Check if TOTP is enabled (public endpoint). / TOTP 有効状態を確認。
      const { result } = await call<{ enabled: boolean }>("totp.status");
      if (result) setTotpEnabled(result.enabled);
      setReady(true);
    });
  }, []);

  const handleSubmit = async (e: Event) => {
    e.preventDefault();
    setError("");
    setLoading(true);

    const params: Record<string, string> = { password };
    if (totpEnabled && totpCode) {
      params.totp_code = totpCode;
    }
    const { error: rpcErr } = await call<{ status: string }>(
      "auth.login",
      params
    );

    setLoading(false);

    if (rpcErr) {
      setError(rpcErr.message || t("login.failed"));
    } else {
      route("/my/");
    }
  };

  if (!ready) return null;

  return (
    <main class="login">
      <Logo size={48} />
      <h1>murlog</h1>
      <form onSubmit={handleSubmit}>
        <input
          class="input"
          type="password"
          value={password}
          onInput={(e) => setPassword((e.target as HTMLInputElement).value)}
          placeholder={t("login.password")}
          required
          autofocus
        />
        {totpEnabled && (
          <input
            class="input"
            type="text"
            inputMode="numeric"
            pattern="[0-9]{6}"
            maxLength={6}
            value={totpCode}
            onInput={(e) => setTotpCode((e.target as HTMLInputElement).value)}
            placeholder={t("login.totp_code")}
            autocomplete="one-time-code"
          />
        )}
        <button class="btn btn-primary" type="submit" disabled={loading}>
          {loading ? "..." : t("login.submit")}
        </button>
        {error && <p class="error">{error}</p>}
      </form>
    </main>
  );
}
