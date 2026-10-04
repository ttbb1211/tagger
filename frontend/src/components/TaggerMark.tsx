import type {SVGProps} from 'react';

// 品牌小圆标：黑胶唱片（盘面 + 两道沟槽 + 红标 + 中孔）。
// 颜色全部走主题变量（--ink / --brand-groove / --brand-red / --paper-2），
// 所以午夜墨主题下会自动变成「米白盘面 + 浅灰沟槽 + 深色中孔」。
//
// ★ 沟槽必须带 vector-effect="non-scaling-stroke"：这个标在顶栏只显示 31px，
//   若按 viewBox 单位描边（0.7 单位 ≈ 0.34 设备像素）沟槽会整条糊掉看不见。
//   用了 non-scaling-stroke 后描边恒为 1 设备像素，16px 与 256px 下都清晰。
//   与 frontend/public/favicon.svg、brand/tagger-mark.svg 是同一套几何，改一处要三处同改。
export function TaggerMark(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      {...props}
      viewBox="0 0 64 64"
      fill="none"
      aria-hidden="true"
      focusable="false"
    >
      <circle cx="32" cy="32" r="26" fill="currentColor" />
      <circle
        cx="32"
        cy="32"
        r="23"
        fill="none"
        stroke="var(--brand-groove)"
        strokeWidth="1.2"
        vectorEffect="non-scaling-stroke"
      />
      <circle
        cx="32"
        cy="32"
        r="19.5"
        fill="none"
        stroke="var(--brand-groove)"
        strokeWidth="1.2"
        vectorEffect="non-scaling-stroke"
      />
      <circle cx="32" cy="32" r="11" fill="var(--brand-red)" />
      <circle cx="32" cy="32" r="4" fill="var(--paper-2)" />
    </svg>
  );
}
