/**
 * 投资账本品牌标：深蓝圆角 + 白字「投」
 * 用系统中文字体渲染，32px 仍清晰可辨。
 */
export default function BrandMark({ className }: { className?: string }) {
  return (
    <span className={className ? `brand-mark ${className}` : 'brand-mark'} aria-hidden="true">
      <span className="brand-mark-char">投</span>
    </span>
  );
}
