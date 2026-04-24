import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { call, redirectIfUnauthorized, uploadMedia } from "../lib/api";
import { load as loadI18n, t } from "../lib/i18n";
import { convertIfNeeded } from "../lib/image";

type CustomField = { name: string; value: string };

type Persona = {
	id: string;
	username: string;
	display_name: string;
	summary: string;
	fields: CustomField[];
	avatar_url: string;
	header_url: string;
	primary: boolean;
	locked: boolean;
	show_follows: boolean;
	discoverable: boolean;
};

export function SettingsPage({ path }: { path?: string }) {
	const [persona, setPersona] = useState<Persona | null>(null);
	const [displayName, setDisplayName] = useState("");
	const [summary, setSummary] = useState("");
	const [avatarUrl, setAvatarUrl] = useState("");
	const [avatarPath, setAvatarPath] = useState("");
	const [headerUrl, setHeaderUrl] = useState("");
	const [headerPath, setHeaderPath] = useState("");
	const [fields, setFields] = useState<CustomField[]>([]);
	const [locked, setLocked] = useState(false);
	const [showFollows, setShowFollows] = useState(true);
	const [discoverable, setDiscoverable] = useState(true);
	const [saving, setSaving] = useState(false);
	const [ready, setReady] = useState(false);
	const [profileError, setProfileError] = useState("");
	const [profileSuccess, setProfileSuccess] = useState("");
	const [pwError, setPwError] = useState("");
	const [pwSuccess, setPwSuccess] = useState("");
	const [currentPassword, setCurrentPassword] = useState("");
	const [newPassword, setNewPassword] = useState("");
	const [confirmPassword, setConfirmPassword] = useState("");
	const avatarRef = useRef<HTMLInputElement>(null);
	const headerRef = useRef<HTMLInputElement>(null);

	const loadPersona = useCallback(async () => {
		await loadI18n();
		const { result, error: err } = await call<Persona[]>("personas.list");
		if (redirectIfUnauthorized(err)) return;
		if (result && result.length > 0) {
			const p = result.find((x) => x.primary) || result[0];
			setPersona(p);
			setDisplayName(p.display_name);
			setSummary(p.summary || "");
			setFields(p.fields || []);
			setAvatarUrl(p.avatar_url || "");
			setHeaderUrl(p.header_url || "");
			setLocked(p.locked || false);
			setShowFollows(p.show_follows !== false);
			setDiscoverable(p.discoverable !== false);
		}
		setReady(true);
	}, []);

	useEffect(() => {
		loadPersona();
	}, [loadPersona]);

	if (!ready || !persona) return null;

	const handleImageUpload = async (
		e: Event,
		prefix: string,
		setUrl: (u: string) => void,
		setPath: (p: string) => void,
	) => {
		const input = e.target as HTMLInputElement;
		const files = input.files;
		if (!files || files.length === 0) return;

		setProfileError("");
		let file: File;
		try {
			file = await convertIfNeeded(files[0]);
		} catch {
			setProfileError(
				t("my.settings.heic_unsupported") ||
					"HEIC is not supported in this browser.",
			);
			return;
		}

		const { result, error: err } = await uploadMedia(file, undefined, prefix);
		if (err) {
			setProfileError(err);
			return;
		}
		if (result) {
			setUrl(result.url);
			setPath(result.path);
		}
		input.value = "";
	};

	const handleSave = async (e: Event) => {
		e.preventDefault();
		setSaving(true);
		setProfileError("");
		setProfileSuccess("");

		const params: Record<string, unknown> = {
			id: persona.id,
			display_name: displayName,
			summary: summary,
			fields: fields.filter((f) => f.name.trim() || f.value.trim()),
		};
		if (avatarPath) params.avatar_path = avatarPath;
		if (avatarUrl === "" && persona.avatar_url) params.avatar_path = "";
		if (headerPath) params.header_path = headerPath;
		if (headerUrl === "" && persona.header_url) params.header_path = "";
		params.locked = locked;
		params.show_follows = showFollows;
		params.discoverable = discoverable;

		const { error: err } = await call("personas.update", params);
		setSaving(false);
		if (err) {
			setProfileError(err.message);
		} else {
			setProfileSuccess(t("my.settings.saved") || "Saved!");
			loadPersona();
		}
	};

	return (
		<div class="screen">
			<h2 class="page-title">{t("my.settings") || "Settings"}</h2>

			<div class="card card-section">
				<form onSubmit={handleSave}>
					<h3 class="card-section-title">
						{t("my.settings.profile") || "Profile"}
					</h3>

					{/* Header image / ヘッダー画像 */}
					<div class="input-group">
						<label>{t("my.settings.header") || "Header Image"}</label>
						<div class="settings-header-preview">
							{headerUrl ? (
								<img src={headerUrl} alt="Header" class="settings-header-img" />
							) : (
								<div class="settings-header-placeholder" />
							)}
						</div>
						<div style={{ marginTop: 8, display: "flex", gap: 8 }}>
							<input
								ref={headerRef}
								type="file"
								accept="image/jpeg,image/png,image/gif,image/webp,image/heic,image/heif"
								style={{ display: "none" }}
								onChange={(e) =>
									handleImageUpload(e, "headers", setHeaderUrl, setHeaderPath)
								}
							/>
							<button
								type="button"
								class="btn btn-outline btn-sm"
								onClick={() => headerRef.current?.click()}
							>
								{t("my.settings.upload") || "Upload"}
							</button>
							{headerUrl && (
								<button
									type="button"
									class="btn btn-outline btn-sm"
									onClick={() => {
										setHeaderUrl("");
										setHeaderPath("");
									}}
								>
									{t("my.settings.remove") || "Remove"}
								</button>
							)}
						</div>
					</div>

					{/* Avatar / アイコン */}
					<div class="input-group">
						<label>{t("my.settings.avatar") || "Avatar"}</label>
						<div style={{ display: "flex", alignItems: "flex-end", gap: 12 }}>
							{avatarUrl ? (
								<img src={avatarUrl} alt="Avatar" class="settings-avatar-img" />
							) : (
								<div class="settings-avatar-placeholder" />
							)}
							<div style={{ display: "flex", gap: 8 }}>
								<input
									ref={avatarRef}
									type="file"
									accept="image/jpeg,image/png,image/gif,image/webp,image/heic,image/heif"
									style={{ display: "none" }}
									onChange={(e) =>
										handleImageUpload(e, "avatars", setAvatarUrl, setAvatarPath)
									}
								/>
								<button
									type="button"
									class="btn btn-outline btn-sm"
									onClick={() => avatarRef.current?.click()}
								>
									{t("my.settings.upload") || "Upload"}
								</button>
								{avatarUrl && (
									<button
										type="button"
										class="btn btn-outline btn-sm"
										onClick={() => {
											setAvatarUrl("");
											setAvatarPath("");
										}}
									>
										{t("my.settings.remove") || "Remove"}
									</button>
								)}
							</div>
						</div>
					</div>

					{/* Display Name / 表示名 */}
					<div class="input-group">
						<label>{t("my.settings.display_name") || "Display Name"}</label>
						<input
							type="text"
							value={displayName}
							onInput={(e) =>
								setDisplayName((e.target as HTMLInputElement).value)
							}
						/>
					</div>

					{/* Summary / 自己紹介 */}
					<div class="input-group">
						<label>{t("my.settings.summary") || "Bio"}</label>
						<textarea
							value={summary}
							rows={4}
							onInput={(e) =>
								setSummary((e.target as HTMLTextAreaElement).value)
							}
						/>
					</div>

					{/* Custom Fields / カスタムフィールド */}
					<div class="input-group">
						<label>{t("my.settings.fields") || "Custom Fields"}</label>
						{fields.map((f, i) => (
							<div key={i} class="custom-field-row">
								<input
									type="text"
									placeholder={t("my.settings.fields_name") || "Label"}
									value={f.name}
									onInput={(e) => {
										const next = [...fields];
										next[i] = {
											...next[i],
											name: (e.target as HTMLInputElement).value,
										};
										setFields(next);
									}}
								/>
								<input
									type="text"
									placeholder={t("my.settings.fields_value") || "Content"}
									value={f.value}
									onInput={(e) => {
										const next = [...fields];
										next[i] = {
											...next[i],
											value: (e.target as HTMLInputElement).value,
										};
										setFields(next);
									}}
								/>
								<button
									type="button"
									class="btn btn-outline btn-sm"
									onClick={() => setFields(fields.filter((_, j) => j !== i))}
								>
									{t("my.settings.fields_remove") || "Remove"}
								</button>
							</div>
						))}
						{fields.length < 4 && (
							<button
								type="button"
								class="btn btn-outline btn-sm"
								style={{ marginTop: 4 }}
								onClick={() => setFields([...fields, { name: "", value: "" }])}
							>
								{t("my.settings.fields_add") || "Add field"}
							</button>
						)}
					</div>

					{/* Locked account / 鍵アカウント */}
					<div class="input-group" style={{ marginTop: 16 }}>
						<label
							style={{
								display: "flex",
								alignItems: "center",
								gap: 8,
								cursor: "pointer",
							}}
						>
							<input
								type="checkbox"
								checked={locked}
								onChange={(e) =>
									setLocked((e.target as HTMLInputElement).checked)
								}
								style={{ width: "auto" }}
							/>
							{t("my.settings.locked") ||
								"Lock account (manually approve followers)"}
						</label>
						<p class="meta" style={{ marginTop: 4 }}>
							{t("my.settings.locked_desc") ||
								"When enabled, you must approve each follow request."}
						</p>
					</div>

					{/* Show follows / フォロー一覧公開 */}
					<div class="input-group" style={{ marginTop: 16 }}>
						<label
							style={{
								display: "flex",
								alignItems: "center",
								gap: 8,
								cursor: "pointer",
							}}
						>
							<input
								type="checkbox"
								checked={showFollows}
								onChange={(e) =>
									setShowFollows((e.target as HTMLInputElement).checked)
								}
								style={{ width: "auto" }}
							/>
							{t("my.settings.show_follows") ||
								"Show follow/follower lists on profile"}
						</label>
						<p class="meta" style={{ marginTop: 4 }}>
							{t("my.settings.show_follows_desc") ||
								"When disabled, only counts are visible. Lists are hidden from the public profile and ActivityPub."}
						</p>
					</div>

					{/* Discoverable / 検索可能 */}
					<div class="input-group" style={{ marginTop: 16 }}>
						<label
							style={{
								display: "flex",
								alignItems: "center",
								gap: 8,
								cursor: "pointer",
							}}
						>
							<input
								type="checkbox"
								checked={discoverable}
								onChange={(e) =>
									setDiscoverable((e.target as HTMLInputElement).checked)
								}
								style={{ width: "auto" }}
							/>
							{t("my.settings.discoverable") || "Appear in search results"}
						</label>
						<p class="meta" style={{ marginTop: 4 }}>
							{t("my.settings.discoverable_desc") ||
								"Allow other servers to discover your profile through search and directory listings."}
						</p>
					</div>

					{profileError && <div class="error">{profileError}</div>}
					{profileSuccess && (
						<div style={{ color: "var(--accent)", fontSize: 13, marginTop: 8 }}>
							{profileSuccess}
						</div>
					)}

					<div class="card-section-actions">
						<button class="btn btn-primary" type="submit" disabled={saving}>
							{t("my.settings.save") || "Save Changes"}
						</button>
					</div>
				</form>
			</div>

			{/* Password Change / パスワード変更 */}
			<div class="card card-section">
				<h3 class="card-section-title">
					{t("my.settings.password") || "Change Password"}
				</h3>
				<form
					onSubmit={async (e) => {
						e.preventDefault();
						setPwError("");
						setPwSuccess("");
						if (newPassword !== confirmPassword) {
							setPwError(
								t("my.settings.password_mismatch") || "Passwords do not match.",
							);
							return;
						}
						const { error: err } = await call("auth.change_password", {
							current_password: currentPassword,
							new_password: newPassword,
						});
						if (err) {
							setPwError(err.message);
						} else {
							setPwSuccess(
								t("my.settings.password_changed") || "Password changed!",
							);
							setCurrentPassword("");
							setNewPassword("");
							setConfirmPassword("");
						}
					}}
				>
					<div class="input-group">
						<label>
							{t("my.settings.current_password") || "Current Password"}
						</label>
						<input
							type="password"
							value={currentPassword}
							onInput={(e) =>
								setCurrentPassword((e.target as HTMLInputElement).value)
							}
						/>
					</div>
					<div class="input-group">
						<label>{t("my.settings.new_password") || "New Password"}</label>
						<input
							type="password"
							value={newPassword}
							onInput={(e) =>
								setNewPassword((e.target as HTMLInputElement).value)
							}
						/>
					</div>
					<div class="input-group">
						<label>
							{t("my.settings.confirm_password") || "Confirm New Password"}
						</label>
						<input
							type="password"
							value={confirmPassword}
							onInput={(e) =>
								setConfirmPassword((e.target as HTMLInputElement).value)
							}
						/>
					</div>
					{pwError && <div class="error">{pwError}</div>}
					{pwSuccess && (
						<div style={{ color: "var(--accent)", fontSize: 13, marginTop: 8 }}>
							{pwSuccess}
						</div>
					)}
					<div class="card-section-actions">
						<button class="btn btn-primary" type="submit">
							{t("my.settings.password_submit") || "Change Password"}
						</button>
					</div>
				</form>
			</div>

			{/* TOTP / 二要素認証 */}
			<TOTPSettings />

			{/* Crawler settings / クローラー設定 */}
			<CrawlerSettings />

			{/* DB Backup & Maintenance / DB バックアップ & メンテナンス */}
			<DBMaintenance />

			{/* Queue / キュー管理 */}
			<div class="card card-section">
				<h3 class="card-section-title">{t("my.queue") || "Queue"}</h3>
				<div style={{ display: "flex", gap: 8 }}>
					<a href="/my/queue" class="btn btn-outline">
						{t("my.settings.queue_manage") || "Manage Queue"}
					</a>
					<a href="/my/domain-failures" class="btn btn-outline">
						{t("my.settings.domain_failures") || "Domain Failures"}
					</a>
				</div>
			</div>
		</div>
	);
}

// TOTPSettings is the TOTP setup/disable component.
// TOTP セットアップ/無効化コンポーネント。
function TOTPSettings() {
	const [enabled, setEnabled] = useState(false);
	const [setupData, setSetupData] = useState<{
		secret: string;
		uri: string;
	} | null>(null);
	const [code, setCode] = useState("");
	const [error, setError] = useState("");
	const [success, setSuccess] = useState("");
	const canvasRef = useRef<HTMLCanvasElement>(null);

	useEffect(() => {
		call<{ enabled: boolean }>("totp.status").then(({ result }) => {
			if (result) setEnabled(result.enabled);
		});
	}, []);

	// Generate QR when setupData changes.
	// setupData が変わったら QR を生成。
	useEffect(() => {
		if (!setupData || !canvasRef.current) return;
		import("qrcode").then((QRCode) => {
			QRCode.toCanvas(canvasRef.current!, setupData.uri, { width: 200 });
		});
	}, [setupData]);

	const handleSetup = async () => {
		setError("");
		setSuccess("");
		const { result, error: err } = await call<{ secret: string; uri: string }>(
			"totp.setup",
		);
		if (err) {
			setError(err.message);
			return;
		}
		if (result) setSetupData(result);
	};

	const handleVerify = async (e: Event) => {
		e.preventDefault();
		setError("");
		const { error: err } = await call("totp.verify", { code });
		if (err) {
			setError(err.message);
			return;
		}
		setSuccess(t("my.settings.totp_verified") || "TOTP enabled!");
		setEnabled(true);
		setSetupData(null);
		setCode("");
	};

	const handleDisable = async () => {
		setError("");
		setSuccess("");
		const { error: err } = await call("totp.disable");
		if (err) {
			setError(err.message);
			return;
		}
		setSuccess(t("my.settings.totp_removed") || "TOTP disabled.");
		setEnabled(false);
	};

	return (
		<div class="card card-section">
			<h3 class="card-section-title">
				{t("my.settings.totp") || "Two-Factor Authentication"}
			</h3>
			<p class="meta card-section-desc">
				{enabled
					? `✅ ${t("my.settings.totp_enabled") || "Enabled"}`
					: t("my.settings.totp_disabled") || "Disabled"}
			</p>

			{!enabled && !setupData && (
				<button type="button" class="btn btn-outline" onClick={handleSetup}>
					{t("my.settings.totp_setup") || "Setup"}
				</button>
			)}

			{setupData && (
				<form onSubmit={handleVerify}>
					<p class="meta" style={{ marginBottom: 8 }}>
						{t("my.settings.totp_desc") ||
							"Scan the QR code with your authenticator app."}
					</p>
					<canvas
						ref={canvasRef}
						style={{ display: "block", margin: "12px 0" }}
					/>
					<div
						class="meta"
						style={{ fontSize: 11, wordBreak: "break-all", marginBottom: 8 }}
					>
						{setupData.secret}
					</div>
					<div class="input-group">
						<input
							type="text"
							inputMode="numeric"
							pattern="[0-9]{6}"
							maxLength={6}
							value={code}
							onInput={(e) => setCode((e.target as HTMLInputElement).value)}
							placeholder={t("my.settings.totp_code") || "6-digit code"}
							required
						/>
					</div>
					<button class="btn btn-primary" type="submit">
						{t("my.settings.totp_verify") || "Enable"}
					</button>
				</form>
			)}

			{enabled && (
				<button
					type="button"
					class="btn btn-outline"
					onClick={handleDisable}
					style={{ marginTop: 8 }}
				>
					{t("my.settings.totp_disable") || "Disable"}
				</button>
			)}

			{error && (
				<div class="error" style={{ marginTop: 8 }}>
					{error}
				</div>
			)}
			{success && (
				<div style={{ color: "var(--accent)", fontSize: 13, marginTop: 8 }}>
					{success}
				</div>
			)}
		</div>
	);
}

// DBMaintenance is the database backup and vacuum component.
// DB バックアップと VACUUM コンポーネント。
function DBMaintenance() {
	const [vacuuming, setVacuuming] = useState(false);
	const [result, setResult] = useState("");
	const [error, setError] = useState("");

	const handleVacuum = async () => {
		setVacuuming(true);
		setError("");
		setResult("");
		const { result: res, error: err } = await call<{ deleted: number }>(
			"queue.vacuum",
		);
		setVacuuming(false);
		if (err) {
			setError(err.message);
		} else if (res) {
			setResult(
				`${t("my.settings.vacuum_done") || "Cleanup complete"}: ${res.deleted} ${t("my.settings.vacuum_deleted") || "jobs deleted"}`,
			);
		}
	};

	return (
		<div class="card card-section">
			<h3 class="card-section-title">
				{t("my.settings.backup") || "Database Backup"}
			</h3>
			<div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
				<a
					href="/api/mur/v1/backup"
					class="btn btn-outline"
					target="_blank"
					download
					rel="noopener"
				>
					{t("my.settings.backup_download") || "Download"}
				</a>
				<button
					type="button"
					class="btn btn-outline"
					onClick={handleVacuum}
					disabled={vacuuming}
				>
					{vacuuming ? "..." : t("my.settings.vacuum") || "Cleanup & Vacuum"}
				</button>
			</div>
			{error && (
				<div class="error" style={{ marginTop: 8 }}>
					{error}
				</div>
			)}
			{result && (
				<div style={{ color: "var(--accent)", fontSize: 13, marginTop: 8 }}>
					{result}
				</div>
			)}
		</div>
	);
}

// CrawlerSettings controls search/AI crawler indexing.
// 検索/AI クローラーのインデックス制御。
function CrawlerSettings() {
	const [noIndex, setNoIndex] = useState(false);
	const [noAI, setNoAI] = useState(false);
	const [saving, setSaving] = useState(false);
	const [success, setSuccess] = useState("");

	useEffect(() => {
		call<{ robots_noindex: boolean; robots_noai: boolean }>(
			"site.get_settings",
		).then(({ result }) => {
			if (result) {
				setNoIndex(result.robots_noindex);
				setNoAI(result.robots_noai);
			}
		});
	}, []);

	const handleSave = async () => {
		setSaving(true);
		setSuccess("");
		await call("site.update_settings", {
			robots_noindex: noIndex,
			robots_noai: noAI,
		});
		setSaving(false);
		setSuccess(t("my.settings.saved") || "Saved!");
	};

	return (
		<div class="card card-section">
			<h3 class="card-section-title">
				{t("my.settings.crawlers") || "Crawler Settings"}
			</h3>
			<div class="input-group">
				<label
					style={{
						display: "flex",
						alignItems: "center",
						gap: 8,
						cursor: "pointer",
					}}
				>
					<input
						type="checkbox"
						checked={noIndex}
						onChange={(e) => setNoIndex((e.target as HTMLInputElement).checked)}
						style={{ width: "auto" }}
					/>
					{t("my.settings.robots_noindex") || "Block search engine indexing"}
				</label>
				<p class="meta" style={{ marginTop: 4 }}>
					{t("my.settings.robots_noindex_desc") ||
						"Prevent search engines like Google from indexing your site."}
				</p>
			</div>
			<div class="input-group" style={{ marginTop: 12 }}>
				<label
					style={{
						display: "flex",
						alignItems: "center",
						gap: 8,
						cursor: "pointer",
					}}
				>
					<input
						type="checkbox"
						checked={noAI}
						onChange={(e) => setNoAI((e.target as HTMLInputElement).checked)}
						style={{ width: "auto" }}
					/>
					{t("my.settings.robots_noai") || "Block AI training crawlers"}
				</label>
				<p class="meta" style={{ marginTop: 4 }}>
					{t("my.settings.robots_noai_desc") ||
						"Block AI crawlers (GPTBot, CCBot, etc.) from using your content for training."}
				</p>
			</div>
			<div class="card-section-actions">
				<button
					type="button"
					class="btn btn-primary"
					onClick={handleSave}
					disabled={saving}
				>
					{t("my.settings.save") || "Save Changes"}
				</button>
			</div>
			{success && (
				<div style={{ color: "var(--accent)", fontSize: 13, marginTop: 8 }}>
					{success}
				</div>
			)}
		</div>
	);
}
