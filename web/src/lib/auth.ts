// Auth state — simple module-level flag set by app.tsx on mount.
// 認証状態 — app.tsx のマウント時にセットするモジュールレベルフラグ。

let loggedIn = false;

export function setLoggedIn(v: boolean): void {
  loggedIn = v;
}

export function isLoggedIn(): boolean {
  return loggedIn;
}
