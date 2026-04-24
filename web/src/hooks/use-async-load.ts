// useAsyncLoad — manages loading / error / retry state for async operations.
// 非同期処理の loading / error / retry 状態を管理するフック。

import { useCallback, useRef, useState } from "preact/hooks";

type AsyncLoadState = {
	loading: boolean;
	error: boolean;
	ready: boolean;
	// Run an async function with automatic loading/error state management.
	// 非同期関数を loading/error 状態管理付きで実行。
	run: (fn: () => Promise<void>) => Promise<void>;
	// Retry the last run. / 最後の run をリトライ。
	retry: () => void;
};

export function useAsyncLoad(): AsyncLoadState {
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState(false);
	const [ready, setReady] = useState(false);
	const lastFn = useRef<(() => Promise<void>) | null>(null);

	const run = useCallback(async (fn: () => Promise<void>) => {
		lastFn.current = fn;
		setLoading(true);
		setError(false);
		try {
			await fn();
		} catch {
			setError(true);
		} finally {
			setLoading(false);
			setReady(true);
		}
	}, []);

	const retry = useCallback(() => {
		if (lastFn.current) run(lastFn.current);
	}, [run]);

	return { loading, error, ready, run, retry };
}
