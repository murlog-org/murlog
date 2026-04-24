// Loading spinner component.
// ローディングスピナーコンポーネント。

type Props = {
	size?: number;
	text?: string;
};

export function Loading({ size = 20, text }: Props) {
	return (
		<div
			class="loading"
			style={{
				display: "flex",
				alignItems: "center",
				justifyContent: "center",
				gap: 8,
				padding: 16,
			}}
		>
			<svg
				width={size}
				height={size}
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2"
				stroke-linecap="round"
				class="loading-spinner"
			>
				<path d="M12 2a10 10 0 0 1 10 10" />
			</svg>
			{text && <span class="meta">{text}</span>}
		</div>
	);
}
