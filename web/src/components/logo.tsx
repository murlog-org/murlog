// Dot constellation logo — size-adaptive dot radii.
// ドットコンステレーションロゴ — 表示サイズに応じたドット半径。

type Props = { size?: number };

// Dot radii per size tier: [top, mid-top, mid-bottom, bottom, accent]
const tiers: Record<string, number[]> = {
  lg: [6, 8, 10, 12, 14],   // >= 64px
  md: [8, 10, 12, 14, 16],  // >= 32px
  sm: [11, 13, 15, 17, 18], // >= 20px
  xs: [14, 16, 18, 20, 21], // < 20px
};

function radii(size: number): number[] {
  if (size >= 64) return tiers.lg;
  if (size >= 32) return tiers.md;
  if (size >= 20) return tiers.sm;
  return tiers.xs;
}

export function Logo({ size = 24 }: Props) {
  const [r1, r2, r3, r4, r5] = radii(size);
  return (
    <svg viewBox="0 0 240 240" width={size} height={size} xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <circle cx="56" cy="52" r={r1} fill="currentColor" opacity="0.35"/>
      <circle cx="120" cy="52" r={r1} fill="currentColor" opacity="0.35"/>
      <circle cx="184" cy="52" r={r1} fill="currentColor" opacity="0.35"/>
      <circle cx="56" cy="90" r={r2} fill="currentColor" opacity="0.5"/>
      <circle cx="88" cy="90" r={r2} fill="currentColor" opacity="0.5"/>
      <circle cx="120" cy="90" r={r2} fill="currentColor" opacity="0.5"/>
      <circle cx="152" cy="90" r={r2} fill="currentColor" opacity="0.5"/>
      <circle cx="184" cy="90" r={r2} fill="currentColor" opacity="0.5"/>
      <circle cx="56" cy="132" r={r3} fill="currentColor" opacity="0.7"/>
      <circle cx="120" cy="132" r={r3} fill="currentColor" opacity="0.7"/>
      <circle cx="184" cy="132" r={r3} fill="currentColor" opacity="0.7"/>
      <circle cx="56" cy="176" r={r4} fill="currentColor"/>
      <circle cx="120" cy="176" r={r4} fill="currentColor"/>
      <circle cx="184" cy="176" r={r4} fill="currentColor"/>
      <circle cx="184" cy="214" r={r5} fill="var(--accent, #3a7ca5)"/>
    </svg>
  );
}
