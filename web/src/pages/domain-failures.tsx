// Domain failures page — shows delivery failure domains with reset controls.
// ドメイン配送失敗ページ — 配送失敗ドメイン一覧とリセット操作。

import { useCallback, useEffect, useState } from "preact/hooks";
import { call, redirectIfUnauthorized } from "../lib/api";
import { formatTime } from "../lib/format";
import { load as loadI18n, t } from "../lib/i18n";

type DomainFailure = {
	domain: string;
	failure_count: number;
	last_error: string;
	dead: boolean;
	first_failure_at: string;
	last_failure_at: string;
};

export function DomainFailuresPage({ path }: { path?: string }) {
	const [failures, setFailures] = useState<DomainFailure[]>([]);
	const [ready, setReady] = useState(false);
	const [error, setError] = useState("");

	const load = useCallback(async () => {
		await loadI18n();
		const { result, error: err } = await call<DomainFailure[]>(
			"domains.list_failures",
		);
		if (redirectIfUnauthorized(err)) return;
		if (result) setFailures(result);
		setReady(true);
	}, []);

	useEffect(() => {
		load();
	}, [load]);

	const handleReset = async (domain: string) => {
		setError("");
		const { error: err } = await call("domains.reset_failure", { domain });
		if (err) {
			setError(err.message);
			return;
		}
		setFailures((prev) => prev.filter((f) => f.domain !== domain));
	};

	if (!ready) return null;

	return (
		<div class="screen screen-wide">
			<h2 class="page-title">
				{t("my.domain_failures.title") || "Domain Failures"}
			</h2>

			{error && (
				<div class="error" style={{ margin: "8px 0" }}>
					{error}
				</div>
			)}

			{failures.length === 0 ? (
				<div class="card">
					<p class="meta" style={{ textAlign: "center", padding: 16 }}>
						{t("my.domain_failures.empty") || "No domain failures."}
					</p>
				</div>
			) : (
				<div class="card" style={{ padding: 0 }}>
					<table class="queue-table" style={{ tableLayout: "fixed" }}>
						<colgroup>
							<col style={{ width: "25%" }} />
							<col style={{ width: "8%" }} />
							<col style={{ width: "10%" }} />
							<col class="queue-desktop" style={{ width: "30%" }} />
							<col class="queue-desktop" style={{ width: "15%" }} />
							<col style={{ width: "12%" }} />
						</colgroup>
						<thead>
							<tr>
								<th>{t("my.domain_failures.domain") || "Domain"}</th>
								<th>{t("my.domain_failures.count") || "Failures"}</th>
								<th>{t("my.domain_failures.status") || "Status"}</th>
								<th class="queue-desktop">
									{t("my.domain_failures.error") || "Last error"}
								</th>
								<th class="queue-desktop">
									{t("my.domain_failures.last_at") || "Last failure"}
								</th>
								<th></th>
							</tr>
						</thead>
						<tbody>
							{failures.map((f) => (
								<tr key={f.domain} class={f.dead ? "row-failed" : ""}>
									<td
										style={{
											overflow: "hidden",
											textOverflow: "ellipsis",
											whiteSpace: "nowrap",
										}}
									>
										<code>{f.domain}</code>
									</td>
									<td>{f.failure_count}</td>
									<td>
										<span
											class={`badge ${f.dead ? "badge-failed" : "badge-pending"}`}
										>
											{f.dead ? "dead" : "degraded"}
										</span>
									</td>
									<td class="queue-desktop error-cell">{f.last_error}</td>
									<td class="queue-desktop meta">
										{formatTime(f.last_failure_at)}
									</td>
									<td>
										<button
											type="button"
											class="btn btn-outline btn-sm"
											onClick={() => handleReset(f.domain)}
										>
											{t("my.domain_failures.reset") || "Reset"}
										</button>
									</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
			)}
		</div>
	);
}
