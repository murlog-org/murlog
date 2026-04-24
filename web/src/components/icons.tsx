// SVG icon components (stroke-based, 24x24 viewBox).
// SVG アイコンコンポーネント (ストローク描画、24x24 viewBox)。

type Props = { size?: number; class?: string };

const paths: Record<string, string> = {
  reply:    "M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z",
  reblog:   "M17 1l4 4-4 4|M3 11V9a4 4 0 0 1 4-4h14|M7 23l-4-4 4-4|M21 13v2a4 4 0 0 1-4 4H3",
  heart:    "M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78L12 21.23l8.84-8.84a5.5 5.5 0 0 0 0-7.78z",
  pin:      "M12 17v5|M5 17h14v-1.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V6h1a2 2 0 0 0 0-4H8a2 2 0 0 0 0 4h1v4.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24z",
  share:    "M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8|M16 6 12 2 8 6|M12 2v13",
  attach:   "M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48",
  bookmark: "M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z",
};

// More icon uses filled circles instead of strokes.
// More アイコンは stroke ではなく fill の circle を使う。
const morePaths = [
  { cx: 5, cy: 12 },
  { cx: 12, cy: 12 },
  { cx: 19, cy: 12 },
];

function SvgIcon({ d, size = 16, ...rest }: { d: string; size?: number; [k: string]: unknown }) {
  return (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      style={{ verticalAlign: "-2px" }}
      aria-hidden="true"
      {...rest}
    >
      {d.split("|").map((p, i) => (
        <path key={i} d={p} />
      ))}
    </svg>
  );
}

export function Icon({ name, size = 16, ...rest }: Props & { name: string }) {
  if (name === "more") {
    return (
      <svg
        viewBox="0 0 24 24"
        width={size}
        height={size}
        fill="currentColor"
        style={{ verticalAlign: "-2px" }}
        aria-hidden="true"
        {...rest}
      >
        {morePaths.map((c, i) => (
          <circle key={i} cx={c.cx} cy={c.cy} r={1} />
        ))}
      </svg>
    );
  }
  const d = paths[name];
  if (!d) return null;
  return <SvgIcon d={d} size={size} {...rest} />;
}

// Convenience exports / 便利エクスポート
export const ReplyIcon = (p: Props) => <Icon name="reply" {...p} />;
export const ReblogIcon = (p: Props) => <Icon name="reblog" {...p} />;
export const HeartIcon = (p: Props) => <Icon name="heart" {...p} />;
export const ShareIcon = (p: Props) => <Icon name="share" {...p} />;
export const PinIcon = (p: Props) => <Icon name="pin" {...p} />;
export const MoreIcon = (p: Props) => <Icon name="more" {...p} />;
export const AttachIcon = (p: Props) => <Icon name="attach" {...p} />;
