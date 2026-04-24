// DropdownMenu — reusable ... dropdown with outside-click-to-close.
// 汎用 ... ドロップダウンメニュー (外側クリックで閉じる)。

import { useState, useEffect, useRef } from "preact/hooks";
import { MoreIcon } from "./icons";

type Props = {
  children: preact.ComponentChildren;
  class?: string;
  iconSize?: number;
};

export function DropdownMenu({ children, class: className, iconSize = 16 }: Props) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("click", handler);
    return () => document.removeEventListener("click", handler);
  }, [open]);

  return (
    <div class={className || "dot-menu"} ref={ref}>
      <button class="dot-menu-btn" onClick={() => setOpen(!open)} aria-label="Menu">
        <MoreIcon size={iconSize} />
      </button>
      {open && (
        <div class="dot-menu-dropdown" style={{ display: "block" }} onClick={() => setOpen(false)}>
          {children}
        </div>
      )}
    </div>
  );
}
