import fs from "node:fs";
import path from "node:path";

// Clean up data dir and config before each test run.
// テスト実行前にデータディレクトリと設定ファイルを削除する。
export default function globalSetup() {
  const root = path.resolve(__dirname, "../..");
  for (const name of ["murlog.ini", "murlog.toml"]) {
    const f = path.join(root, name);
    if (fs.existsSync(f)) fs.unlinkSync(f);
  }
  const dataDir = path.join(root, "data");
  if (fs.existsSync(dataDir)) fs.rmSync(dataDir, { recursive: true });
}
