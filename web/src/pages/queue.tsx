import { useState, useEffect, useCallback, useRef } from "preact/hooks";
import { call, redirectIfUnauthorized } from "../lib/api";
import { load as loadI18n, t } from "../lib/i18n";
import { formatTime } from "../lib/format";
import { Loading } from "../components/loading";

type QueueStats = {
  pending: number;
  running: number;
  done: number;
  failed: number;
  dead: number;
};


type Job = {
  id: string;
  type: string;
  payload: string;
  status: string;
  attempts: number;
  last_error: string;
  next_run_at: string;
  created_at: string;
  completed_at?: string;
};

const PAGE_SIZE = 50;

// extractHost extracts the destination host from a job payload.
// ジョブの payload から宛先ホストを抽出する。
export function extractHost(job: Job): string {
  try {
    const p = JSON.parse(job.payload);
    const uri = p.actor_uri || p.target_actor_uri || p.target_uri || "";
    if (uri) {
      const u = new URL(uri);
      return u.hostname;
    }
  } catch { /* ignore */ }
  return "";
}

// formatElapsed calculates elapsed time from created_at to now or completed_at.
// 経過時間を算出する (created_at から現在 or completed_at まで)。
function formatElapsed(job: Job): string {
  const start = new Date(job.created_at).getTime();
  const end = job.completed_at ? new Date(job.completed_at).getTime() : Date.now();
  const sec = Math.max(0, Math.floor((end - start) / 1000));
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m${sec % 60}s`;
  return `${Math.floor(sec / 3600)}h${Math.floor((sec % 3600) / 60)}m`;
}

export function QueuePage({ path }: { path?: string }) {
  const [stats, setStats] = useState<QueueStats | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState<string>("");
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [selectedJob, setSelectedJob] = useState<Job | null>(null);
  const tickRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  const fetchJobs = useCallback(async (status: string, cursor?: string) => {
    const params: Record<string, unknown> = { limit: PAGE_SIZE };
    if (status) params.status = status;
    if (cursor) params.cursor = cursor;
    const res = await call<Job[]>("queue.list", params);
    return res.result ?? [];
  }, []);

  const loadData = useCallback(async (status?: string) => {
    await loadI18n();
    const filterVal = status ?? filter;
    const [statsRes, fetched] = await Promise.all([
      call<QueueStats>("queue.stats"),
      fetchJobs(filterVal),
    ]);
    if (redirectIfUnauthorized(statsRes.error)) return;
    if (statsRes.result) setStats(statsRes.result);
    setJobs(fetched);
    setHasMore(fetched.length >= PAGE_SIZE);
    setReady(true);
  }, [filter, fetchJobs]);

  const loadMore = useCallback(async () => {
    if (jobs.length === 0) return;
    setLoadingMore(true);
    const cursor = jobs[jobs.length - 1].id;
    const fetched = await fetchJobs(filter, cursor);
    setJobs((prev) => [...prev, ...fetched]);
    setHasMore(fetched.length >= PAGE_SIZE);
    setLoadingMore(false);
  }, [jobs, filter, fetchJobs]);

  useEffect(() => {
    loadData();
    tickRef.current = setInterval(async () => {
      await call("queue.tick");
      loadData();
    }, 5000);
    return () => {
      if (tickRef.current) clearInterval(tickRef.current);
    };
  }, [loadData]);

  // Infinite scroll. / 無限スクロール。
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loadingMore) {
          loadMore();
        }
      },
      { rootMargin: "200px" }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasMore, loadingMore, loadMore]);

  const changeFilter = (f: string) => {
    setFilter(f);
    setJobs([]);
    setReady(false);
    loadData(f);
  };

  const handleRetry = async (id: string) => {
    setError("");
    const { error: err } = await call("queue.retry", { id });
    if (err) { setError(err.message); return; }
    setSelectedJob(null);
    loadData();
  };

  const handleDismiss = async (id: string) => {
    setError("");
    const { error: err } = await call("queue.dismiss", { id });
    if (err) { setError(err.message); return; }
    setSelectedJob(null);
    loadData();
  };


  if (!ready) return null;

  return (
    <div class="screen screen-wide">
      <h2 class="page-title">{t("my.queue.title") || "Job Queue"}</h2>

      {/* Stats / 統計 */}
      {stats && (
        <div class="card card-section">
          <div class="queue-stats">
            <div class="queue-stat-item">
              <span class="queue-stat-count">{stats.pending}</span>
              <span class="queue-stat-label">{t("my.queue.pending")}</span>
            </div>
            <div class="queue-stat-item">
              <span class="queue-stat-count">{stats.running}</span>
              <span class="queue-stat-label">{t("my.queue.running")}</span>
            </div>
            <div class="queue-stat-item">
              <span class="queue-stat-count">{stats.done}</span>
              <span class="queue-stat-label">{t("my.queue.done")}</span>
            </div>
            <div class="queue-stat-item">
              <span class="queue-stat-count queue-stat-failed">{stats.failed}</span>
              <span class="queue-stat-label">{t("my.queue.failed")}</span>
            </div>
            <div class="queue-stat-item">
              <span class="queue-stat-count queue-stat-failed">{stats.dead}</span>
              <span class="queue-stat-label">{t("my.queue.dead") || "Dead"}</span>
            </div>
          </div>
        </div>
      )}

      {/* Filter tabs / フィルタタブ */}
      <div class="tabs" style={{ marginTop: 16 }}>
        <a class={`tab${filter === "" ? " active" : ""}`} onClick={() => changeFilter("")}>
          {t("my.queue.all") || "All"}
        </a>
        {["pending", "running", "done", "failed", "dead"].map((f) => (
          <a key={f} class={`tab${filter === f ? " active" : ""}`} onClick={() => changeFilter(f)}>
            {t(`my.queue.${f}`) || f}
          </a>
        ))}
      </div>

      {error && <div class="error" style={{ margin: "8px 0" }}>{error}</div>}

      {/* Job list / ジョブ一覧 */}
      <div class="card" style={{ padding: 0 }}>
        {jobs.length === 0 ? (
          <p class="meta" style={{ textAlign: "center", padding: 16 }}>
            {t("my.queue.empty")}
          </p>
        ) : (
          <table class="queue-table">
            <thead>
              <tr>
                <th>{t("my.queue.type")}</th>
                <th>{t("my.queue.status")}</th>
                <th class="queue-desktop">{t("my.queue.attempts")}</th>
                <th class="queue-desktop">{t("my.queue.error")}</th>
                <th>{t("my.queue.created")}</th>
                <th>{t("my.queue.elapsed") || "Elapsed"}</th>
                <th class="queue-desktop"></th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((job) => (
                <tr key={job.id} class={job.status === "failed" || job.status === "dead" ? "row-failed" : ""} onClick={(e) => {
                  if (!(e.target as HTMLElement).closest("button")) setSelectedJob(job);
                }}>
                  <td><code>{job.type}</code></td>
                  <td>
                    <span class={`badge badge-${job.status}`}>{job.status}</span>
                  </td>
                  <td class="queue-desktop">{job.attempts}</td>
                  <td class={job.last_error ? "queue-desktop error-cell" : "queue-desktop"}>{job.last_error || ""}</td>
                  <td class="meta">{formatTime(job.created_at)}</td>
                  <td class="meta">{formatElapsed(job)}</td>
                  <td class="queue-desktop">
                    {(job.status === "failed" || job.status === "dead") && (
                      <div style={{ display: "flex", gap: 4 }}>
                        <button class="btn btn-outline btn-sm" onClick={() => handleRetry(job.id)}>
                          {t("my.queue.retry")}
                        </button>
                        <button class="btn btn-outline btn-sm" onClick={() => handleDismiss(job.id)}>
                          {t("my.queue.dismiss") || "Dismiss"}
                        </button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {loadingMore && <Loading />}
      <div ref={sentinelRef} />

      {/* Job detail dialog / ジョブ詳細ダイアログ */}
      {selectedJob && (
        <div class="overlay" onClick={(e) => {
          if ((e.target as HTMLElement).classList.contains("overlay")) setSelectedJob(null);
        }}>
          <div class="dialog">
            <div class="dialog-header">
              <h3 class="dialog-title">{t("my.queue.detail") || "Job Detail"}</h3>
              <button class="dialog-close" aria-label="Close" onClick={() => setSelectedJob(null)}>&times;</button>
            </div>
            <div class="dialog-body">
              <dl class="job-detail">
                <dt>{t("my.queue.type")}</dt>
                <dd><code>{selectedJob.type}</code></dd>
                <dt>{t("my.queue.status")}</dt>
                <dd><span class={`badge badge-${selectedJob.status}`}>{selectedJob.status}</span></dd>
                <dt>{t("my.queue.destination") || "Destination"}</dt>
                <dd>{extractHost(selectedJob) || "—"}</dd>
                <dt>{t("my.queue.attempts")}</dt>
                <dd>{selectedJob.attempts}</dd>
                <dt>{t("my.queue.created")}</dt>
                <dd>{formatTime(selectedJob.created_at)}</dd>
                <dt>{t("my.queue.elapsed") || "Elapsed"}</dt>
                <dd>{formatElapsed(selectedJob)}</dd>
                {selectedJob.completed_at && (<>
                  <dt>{t("my.queue.completed") || "Completed"}</dt>
                  <dd>{formatTime(selectedJob.completed_at)}</dd>
                </>)}
                <dt>{t("my.queue.next_run") || "Next run"}</dt>
                <dd>{selectedJob.next_run_at ? formatTime(selectedJob.next_run_at) : "—"}</dd>
                {selectedJob.last_error && (<>
                  <dt>{t("my.queue.error")}</dt>
                  <dd class="job-detail-error">{selectedJob.last_error}</dd>
                </>)}
                <dt>{t("my.queue.payload") || "Payload"}</dt>
                <dd><pre class="job-detail-payload">{selectedJob.payload}</pre></dd>
              </dl>
            </div>
            {(selectedJob.status === "failed" || selectedJob.status === "dead") && (
              <div class="dialog-footer">
                <button class="btn btn-outline btn-sm" onClick={() => handleRetry(selectedJob.id)}>
                  {t("my.queue.retry")}
                </button>
                <button class="btn btn-outline btn-sm" onClick={() => handleDismiss(selectedJob.id)}>
                  {t("my.queue.dismiss") || "Dismiss"}
                </button>
              </div>
            )}
          </div>
        </div>
      )}

    </div>
  );
}
