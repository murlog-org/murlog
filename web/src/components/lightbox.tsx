// Lightbox — fullscreen image overlay triggered by clicking post images.
// Moves the original <img> element into the overlay to avoid re-fetching.
// ライトボックス — 投稿画像クリックで全画面オーバーレイ表示。
// 元の <img> 要素をオーバーレイに移動して再フェッチを回避。

import { useCallback, useEffect, useRef, useState } from "preact/hooks";

type RestoreInfo = {
	img: HTMLImageElement;
	parent: Node;
	next: Node | null;
	origClass: string;
	origStyle: string;
};

let showFn: ((img: HTMLImageElement) => void) | null = null;

// openLightbox opens the lightbox with the given image element.
// 指定画像要素でライトボックスを開く。
export function openLightbox(img: HTMLImageElement): void {
	if (showFn) showFn(img);
}

export function Lightbox() {
	const [visible, setVisible] = useState(false);
	const overlayRef = useRef<HTMLDivElement>(null);
	const restoreRef = useRef<RestoreInfo | null>(null);

	const close = useCallback(() => {
		// Restore the image to its original position.
		// 画像を元の位置に戻す。
		const info = restoreRef.current;
		if (info) {
			info.img.className = info.origClass;
			info.img.setAttribute("style", info.origStyle);
			if (info.next) {
				info.parent.insertBefore(info.img, info.next);
			} else {
				info.parent.appendChild(info.img);
			}
			restoreRef.current = null;
		}
		setVisible(false);
	}, []);

	useEffect(() => {
		showFn = (img: HTMLImageElement) => {
			if (!overlayRef.current || !img.parentNode) return;

			// Save original position for restoration.
			// 復元用に元の位置を保存。
			restoreRef.current = {
				img,
				parent: img.parentNode,
				next: img.nextSibling,
				origClass: img.className,
				origStyle: img.getAttribute("style") || "",
			};

			// Move into overlay.
			// オーバーレイに移動。
			img.className = "lightbox-img";
			img.removeAttribute("style");
			overlayRef.current.appendChild(img);
			setVisible(true);
		};
		return () => {
			showFn = null;
		};
	}, []);

	// Close on Escape key. / Escape キーで閉じる。
	useEffect(() => {
		if (!visible) return;
		const handler = (e: KeyboardEvent) => {
			if (e.key === "Escape") close();
		};
		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, [visible, close]);

	return (
		<div
			class="lightbox-overlay"
			ref={overlayRef}
			style={{ display: visible ? "flex" : "none" }}
			onClick={close}
		/>
	);
}
